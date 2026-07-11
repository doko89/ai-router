package main

import (
	"os"
	"strconv"
	"strings"
)

// APIType identifies which upstream OpenAI-compatible API dialect is targeted.
type APIType string

const (
	APIChatCompletions APIType = "v1_chat_completions"
	APIResponses       APIType = "v1_responses"
)

// Config holds runtime configuration for the adapter.
type Config struct {
	OpenAIBaseURL    string
	OpenAIAPIKey     string
	TiktokenEncoding string
	Host             string
	Port             int
	APIType          APIType
}

// LoadConfig builds a Config from environment variables with sensible defaults.
func LoadConfig() *Config {
	c := &Config{
		OpenAIBaseURL:    getenv("OPENAI_BASE_URL", "https://api.openai.com/v1/chat/completions"),
		OpenAIAPIKey:     os.Getenv("OPENAI_API_KEY"),
		TiktokenEncoding: getenv("TIKTOKEN_ENCODING", "cl100k_base"),
		Host:             getenv("HOST", "0.0.0.0"),
		Port:             getenvInt("PORT", 8000),
	}
	c.detectAPIType()
	return c
}

// detectAPIType auto-detects the upstream API dialect from the base URL.
func (c *Config) detectAPIType() {
	switch {
	case strings.Contains(c.OpenAIBaseURL, "/v1/responses"):
		c.APIType = APIResponses
	default:
		c.APIType = APIChatCompletions
	}
}

// Update applies runtime overrides (builder/CLI pattern) and re-detects API type.
func (c *Config) Update(baseURL, apiKey string) {
	if baseURL != "" {
		c.OpenAIBaseURL = baseURL
		c.detectAPIType()
	}
	if apiKey != "" {
		c.OpenAIAPIKey = apiKey
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
