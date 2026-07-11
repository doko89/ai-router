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
	RateLimitCooldown int    `yaml:"rate_limit_cooldown"` // seconds; 0 disables
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
}

// AggModel is one routable target inside an aggregation.
type AggModel struct {
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
	Weight   int    `yaml:"weight"`
}

// ModelAggregation maps a virtual model name to one or more provider targets
// selected via a routing strategy.
type ModelAggregation struct {
	Name     string     `yaml:"name"`
	Strategy string     `yaml:"strategy"`
	Models   []AggModel `yaml:"models"`

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
