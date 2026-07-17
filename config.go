package main

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	"gopkg.in/yaml.v3"
)

// Compatible identifies the upstream API dialect of a provider.
type Compatible string

const (
	CompatibleOpenAI          Compatible = "openai"           // {base_url}/chat/completions
	CompatibleOpenAIResponses Compatible = "openai-responses" // {base_url}/responses
	CompatibleAnthropic       Compatible = "anthropic"        // {base_url}/messages (passthrough)
)

// GatewayConfig holds server-level settings.
type GatewayConfig struct {
	Host              string `yaml:"host"`
	Port              int    `yaml:"port"`
	Debug             bool   `yaml:"debug"`
	TiktokenEncoding  string `yaml:"tiktoken_encoding"`
	RateLimitCooldown int    `yaml:"rate_limit_cooldown"` // seconds; default 600 (10 min), 0 disables
	Compression       string `yaml:"compression"`         // off, lite, standard, aggressive; empty = off
}

// ClientKey is a caller-facing API key accepted by the gateway.
type ClientKey struct {
	Key  string `yaml:"key"`
	Name string `yaml:"name"`
}

// Provider is an upstream LLM endpoint.
type Provider struct {
	Name              string     `yaml:"name"`
	Enabled           bool       `yaml:"enabled"`
	Compatible        Compatible `yaml:"compatible"`
	BaseURL           string     `yaml:"base_url"`
	APIKey            string     `yaml:"api_key"`
	RateLimitCooldown int        `yaml:"rate_limit_cooldown"` // per-provider override; 0 = use gateway default
	ContextWindow     int        `yaml:"context_window"`      // max input tokens; 0 = unknown
	MaxOutput         int        `yaml:"max_output"`          // max output tokens; 0 = unknown
}

// AggModel is one routable target inside an aggregation.
type AggModel struct {
	Provider      string `yaml:"provider"`
	Model         string `yaml:"model"`
	Weight        int    `yaml:"weight"`
	ContextWindow int    `yaml:"context_window"` // per-model override; 0 = inherit from provider
	MaxOutput     int    `yaml:"max_output"`      // per-model override; 0 = inherit from provider
}

// ModelAggregation maps a virtual model name to one or more provider targets
// selected via a routing strategy.
type ModelAggregation struct {
	Name          string     `yaml:"name"`
	Strategy      string     `yaml:"strategy"`
	Models        []AggModel `yaml:"models"`
	ContextWindow int        `yaml:"context_window"` // override for entire aggregation; 0 = resolve from models
	MaxOutput     int        `yaml:"max_output"`      // override for entire aggregation; 0 = resolve from models

	rr atomic.Uint64 // round-robin counter
}

// Config is the full gateway configuration.
type Config struct {
	Gateway           GatewayConfig      `yaml:"gateway"`
	ClientKeys        []ClientKey        `yaml:"client_keys"`
	Providers         []Provider         `yaml:"providers"`
	ModelAggregations []ModelAggregation `yaml:"model_aggregations"`

	providerByName map[string]*Provider
	aggByName      map[string]*ModelAggregation
}

// LoadConfig reads and validates a YAML configuration file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	c.applyDefaults()
	c.buildIndexes()
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) applyDefaults() {
	if c.Gateway.Host == "" {
		c.Gateway.Host = "127.0.0.1"
	}
	if c.Gateway.Port == 0 {
		c.Gateway.Port = 8081
	}
	if c.Gateway.TiktokenEncoding == "" {
		c.Gateway.TiktokenEncoding = "cl100k_base"
	}
	if c.Gateway.RateLimitCooldown == 0 {
		c.Gateway.RateLimitCooldown = 600
	}
	for i := range c.Providers {
		if c.Providers[i].Compatible == "" {
			c.Providers[i].Compatible = CompatibleOpenAI
		}
	}
	for i := range c.ModelAggregations {
		if c.ModelAggregations[i].Strategy == "" {
			c.ModelAggregations[i].Strategy = "failover"
		}
	}
}

func (c *Config) buildIndexes() {
	c.providerByName = make(map[string]*Provider, len(c.Providers))
	for i := range c.Providers {
		c.providerByName[c.Providers[i].Name] = &c.Providers[i]
	}
	c.aggByName = make(map[string]*ModelAggregation, len(c.ModelAggregations))
	for i := range c.ModelAggregations {
		c.aggByName[c.ModelAggregations[i].Name] = &c.ModelAggregations[i]
	}
}

func (c *Config) validate() error {
	if len(c.ModelAggregations) == 0 {
		return fmt.Errorf("no model_aggregations defined")
	}
	for i := range c.ModelAggregations {
		agg := &c.ModelAggregations[i]
		if len(agg.Models) == 0 {
			return fmt.Errorf("aggregation %q has no models", agg.Name)
		}
		switch agg.Strategy {
		case "failover", "round_robin", "weighted":
		default:
			return fmt.Errorf("aggregation %q has unknown strategy %q", agg.Name, agg.Strategy)
		}
	}
	return nil
}

// providerEndpoint returns the upstream URL for a provider based on its dialect.
func (p *Provider) endpoint() string {
	base := strings.TrimRight(p.BaseURL, "/")
	switch p.Compatible {
	case CompatibleOpenAIResponses:
		return base + "/responses"
	case CompatibleAnthropic:
		return base + "/messages"
	default:
		return base + "/chat/completions"
	}
}

// EffectiveMetadata holds resolved context window and max output for an aggregation.
type EffectiveMetadata struct {
	ContextWindow int
	MaxOutput     int
}

// aggregationMetadata computes the effective (context_window, max_output) for an
// aggregation. Hierarchy (first wins):
//  1. aggregation-level override (ModelAggregation.ContextWindow / MaxOutput)
//  2. min of all valid model candidates (each: AggModel override → Provider value → 0)
//  3. 0 (unknown) if no value can be determined
func (c *Config) aggregationMetadata(name string) (EffectiveMetadata, bool) {
	agg, ok := c.aggByName[name]
	if !ok {
		return EffectiveMetadata{}, false
	}

	// Aggregation-level override — strongest.
	if agg.ContextWindow > 0 || agg.MaxOutput > 0 {
		return EffectiveMetadata{ContextWindow: agg.ContextWindow, MaxOutput: agg.MaxOutput}, true
	}

	// Otherwise compute min across all enabled models.
	minCtx := -1
	minOut := -1
	for _, m := range agg.Models {
		p, ok := c.providerByName[m.Provider]
		if !ok || !p.Enabled {
			continue
		}
		ctx := m.ContextWindow
		if ctx <= 0 {
			ctx = p.ContextWindow
		}
		out := m.MaxOutput
		if out <= 0 {
			out = p.MaxOutput
		}
		if ctx > 0 && (minCtx < 0 || ctx < minCtx) {
			minCtx = ctx
		}
		if out > 0 && (minOut < 0 || out < minOut) {
			minOut = out
		}
	}

	meta := EffectiveMetadata{}
	if minCtx > 0 {
		meta.ContextWindow = minCtx
	}
	if minOut > 0 {
		meta.MaxOutput = minOut
	}
	return meta, true
}

// authClient returns (client name, true) if the given key is authorized. When
// no client keys are configured, all requests are allowed.
func (c *Config) authClient(key string) (string, bool) {
	if len(c.ClientKeys) == 0 {
		return "anonymous", true
	}
	for _, ck := range c.ClientKeys {
		if ck.Key == key && key != "" {
			return ck.Name, true
		}
	}
	return "", false
}
