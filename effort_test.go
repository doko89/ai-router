package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func newTestGateway(t *testing.T, compatible Compatible, captured *[]string) *fiber.App {
	var mu sync.Mutex
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		*captured = append(*captured, string(b))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"x","object":"chat.completion","created":1,"model":"mock-model","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	t.Cleanup(mock.Close)

	cfg := &Config{
		Gateway:    GatewayConfig{Host: "127.0.0.1", Port: 0, TiktokenEncoding: "cl100k_base"},
		ClientKeys: []ClientKey{{Key: "test", Name: "test"}},
		Providers:  []Provider{{Name: "mock", Enabled: true, Compatible: compatible, BaseURL: mock.URL, APIKey: "x"}},
		ModelAggregations: []ModelAggregation{{
			Name: "m", Strategy: "failover",
			Models: []AggModel{{Provider: "mock", Model: "mock-model"}},
		}},
	}
	cfg.buildIndexes()
	return buildApp(cfg)
}

func doReq(t *testing.T, app *fiber.App, path, auth, body string) {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

func TestEffortForwardingOpenAI(t *testing.T) {
	var captured []string
	app := newTestGateway(t, CompatibleOpenAI, &captured)

	// OpenAI inbound reasoning_effort
	doReq(t, app, "/v1/chat/completions", "Bearer test",
		`{"model":"m","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"high"}`)
	if len(captured) == 0 {
		t.Fatal("no request captured")
	}
	var m map[string]any
	json.Unmarshal([]byte(captured[0]), &m)
	if m["reasoning_effort"] != "high" {
		t.Fatalf("openai inbound: want reasoning_effort=high, got %v (body=%s)", m["reasoning_effort"], captured[0])
	}

	// Anthropic inbound output_config.effort -> reasoning_effort
	doReq(t, app, "/v1/messages", "Bearer test",
		`{"model":"m","max_tokens":10,"messages":[{"role":"user","content":"hi"}],"output_config":{"effort":"low"}}`)
	var m2 map[string]any
	json.Unmarshal([]byte(captured[1]), &m2)
	if m2["reasoning_effort"] != "low" {
		t.Fatalf("anthropic inbound output_config: want reasoning_effort=low, got %v (body=%s)", m2["reasoning_effort"], captured[1])
	}

	// Anthropic inbound thinking.effort -> reasoning_effort
	doReq(t, app, "/v1/messages", "Bearer test",
		`{"model":"m","max_tokens":10,"messages":[{"role":"user","content":"hi"}],"thinking":{"type":"adaptive","effort":"medium"}}`)
	var m3 map[string]any
	json.Unmarshal([]byte(captured[2]), &m3)
	if m3["reasoning_effort"] != "medium" {
		t.Fatalf("anthropic inbound thinking: want reasoning_effort=medium, got %v (body=%s)", m3["reasoning_effort"], captured[2])
	}
}

func TestEffortForwardingResponses(t *testing.T) {
	var captured []string
	app := newTestGateway(t, CompatibleOpenAIResponses, &captured)

	doReq(t, app, "/v1/chat/completions", "Bearer test",
		`{"model":"m","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"xhigh"}`)
	var m map[string]any
	json.Unmarshal([]byte(captured[0]), &m)
	reasoning, ok := m["reasoning"].(map[string]any)
	if !ok || reasoning["effort"] != "xhigh" {
		t.Fatalf("responses provider: want reasoning.effort=xhigh, got %v (body=%s)", m["reasoning"], captured[0])
	}
}

func TestEffortOmittedWhenUnset(t *testing.T) {
	var captured []string
	app := newTestGateway(t, CompatibleOpenAI, &captured)

	doReq(t, app, "/v1/chat/completions", "Bearer test",
		`{"model":"m","messages":[{"role":"user","content":"hi"}]}`)
	var m map[string]any
	json.Unmarshal([]byte(captured[0]), &m)
	if _, ok := m["reasoning_effort"]; ok {
		t.Fatalf("effort should be omitted when unset, body=%s", captured[0])
	}
}
