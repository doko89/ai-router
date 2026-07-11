package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
)

// App holds shared dependencies for the HTTP handlers.
type App struct {
	cfg          *Config
	client       *http.Client
	streamClient *http.Client
}

// NewApp creates an App with configured HTTP clients.
func NewApp(cfg *Config) *App {
	return &App{
		cfg:          cfg,
		client:       &http.Client{Timeout: 120 * time.Second},
		streamClient: &http.Client{},
	}
}

// anthropicError builds an Anthropic-formatted error payload.
func anthropicError(errType, message string) map[string]any {
	return map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    errType,
			"message": message,
		},
	}
}

// clientKey extracts the caller key from Authorization: Bearer or x-api-key.
func clientKey(c fiber.Ctx) string {
	if h := c.Get("Authorization"); h != "" {
		if strings.HasPrefix(strings.ToLower(h), "bearer ") {
			return strings.TrimSpace(h[7:])
		}
		return strings.TrimSpace(h)
	}
	return c.Get("x-api-key")
}

// authenticate verifies the caller and returns the client name.
func (a *App) authenticate(c fiber.Ctx) (string, bool) {
	return a.cfg.authClient(clientKey(c))
}

// handleMessages implements POST /v1/messages with multi-provider routing.
func (a *App) handleMessages(c fiber.Ctx) error {
	name, ok := a.authenticate(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(
			anthropicError("authentication_error", "Invalid or missing client API key"))
	}

	bodyBytes := append([]byte(nil), c.Body()...)

	var req AnthropicRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			anthropicError("invalid_request_error", err.Error()))
	}

	candidates, exists := a.cfg.resolveCandidates(req.Model)
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(anthropicError("not_found_error",
			fmt.Sprintf("model %q not found. Available: %s", req.Model, strings.Join(a.cfg.aggregationNames(), ", "))))
	}
	if len(candidates) == 0 {
		return c.Status(fiber.StatusServiceUnavailable).JSON(anthropicError("overloaded_error",
			fmt.Sprintf("no enabled provider available for model %q", req.Model)))
	}

	if a.cfg.Gateway.Debug {
		log.Printf("[%s] model=%q stream=%v candidates=%d", name, req.Model, req.Stream, len(candidates))
	}

	var lastErr string
	for _, tgt := range candidates {
		resp, err := a.dispatch(tgt, bodyBytes, &req)
		if err != nil {
			lastErr = err.Error()
			if a.cfg.Gateway.Debug {
				log.Printf("  provider=%s model=%s dial error: %v", tgt.Provider.Name, tgt.Model, err)
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			data, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastErr = fmt.Sprintf("upstream %d from %s", resp.StatusCode, tgt.Provider.Name)
			if a.cfg.Gateway.Debug {
				log.Printf("  provider=%s model=%s status=%d body=%s", tgt.Provider.Name, tgt.Model, resp.StatusCode, string(data))
			}
			continue
		}

		if req.Stream {
			return a.streamResponse(c, tgt.Provider.Compatible, resp)
		}
		return a.jsonResponse(c, tgt.Provider.Compatible, resp)
	}

	return c.Status(fiber.StatusBadGateway).JSON(
		anthropicError("api_error", "all providers failed: "+lastErr))
}

// dispatch builds and sends the upstream request for a target.
func (a *App) dispatch(tgt Target, bodyBytes []byte, req *AnthropicRequest) (*http.Response, error) {
	var payload []byte
	switch tgt.Provider.Compatible {
	case CompatibleAnthropic:
		m := map[string]any{}
		if err := json.Unmarshal(bodyBytes, &m); err != nil {
			return nil, err
		}
		m["model"] = tgt.Model
		payload, _ = json.Marshal(m)
	case CompatibleOpenAIResponses:
		body := transformRequestBodyV1Responses(req)
		body["model"] = tgt.Model
		payload, _ = json.Marshal(body)
	default: // openai
		body := transformRequestBody(req)
		body["model"] = tgt.Model
		payload, _ = json.Marshal(body)
	}

	httpReq, err := http.NewRequest(http.MethodPost, tgt.Provider.endpoint(), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if tgt.Provider.Compatible == CompatibleAnthropic {
		httpReq.Header.Set("x-api-key", tgt.Provider.APIKey)
		httpReq.Header.Set("anthropic-version", "2023-06-01")
	} else {
		httpReq.Header.Set("Authorization", "Bearer "+tgt.Provider.APIKey)
	}

	client := a.client
	if req.Stream {
		client = a.streamClient
	}
	return client.Do(httpReq)
}

// jsonResponse transforms and returns a non-streaming upstream response.
func (a *App) jsonResponse(c fiber.Ctx, compat Compatible, resp *http.Response) error {
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	switch compat {
	case CompatibleAnthropic:
		c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
		return c.Send(data)
	case CompatibleOpenAIResponses:
		var parsed V1ResponsesResponse
		if err := json.Unmarshal(data, &parsed); err != nil {
			return c.Status(fiber.StatusBadGateway).JSON(anthropicError("api_error", err.Error()))
		}
		return c.JSON(transformV1ResponsesResponse(&parsed))
	default:
		var parsed OpenAIResponse
		if err := json.Unmarshal(data, &parsed); err != nil {
			return c.Status(fiber.StatusBadGateway).JSON(anthropicError("api_error", err.Error()))
		}
		return c.JSON(transformOpenAIResponse(&parsed))
	}
}

// streamResponse transforms and streams an upstream SSE response.
func (a *App) streamResponse(c fiber.Ctx, compat Compatible, resp *http.Response) error {
	c.Set(fiber.HeaderContentType, "text/event-stream")
	c.Set(fiber.HeaderCacheControl, "no-cache")
	c.Set(fiber.HeaderConnection, "keep-alive")

	return c.SendStreamWriter(func(w *bufio.Writer) {
		defer resp.Body.Close()
		switch compat {
		case CompatibleAnthropic:
			copyStream(w, resp.Body)
		case CompatibleOpenAIResponses:
			streamV1ResponsesToAnthropic(w, resp.Body)
		default:
			streamOpenAIToAnthropic(w, resp.Body)
		}
	})
}

// copyStream forwards an already-Anthropic SSE stream verbatim, flushing often.
func copyStream(w *bufio.Writer, body io.Reader) {
	buf := make([]byte, 8*1024)
	for {
		n, err := body.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
			w.Flush()
		}
		if err != nil {
			return
		}
	}
}

// handleCountTokens implements POST /v1/messages/count_tokens.
func (a *App) handleCountTokens(c fiber.Ctx) error {
	if _, ok := a.authenticate(c); !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(
			anthropicError("authentication_error", "Invalid or missing client API key"))
	}
	var req AnthropicRequest
	if err := json.Unmarshal(c.Body(), &req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{"error": err.Error()})
	}
	openaiBody := transformRequestBody(&req)
	tokenCount := countOpenAITokens(openaiBody, a.cfg.Gateway.TiktokenEncoding)
	return c.JSON(map[string]any{"input_tokens": tokenCount})
}

// handleListModels implements GET /v1/models, listing model aggregations.
func (a *App) handleListModels(c fiber.Ctx) error {
	if _, ok := a.authenticate(c); !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(
			anthropicError("authentication_error", "Invalid or missing client API key"))
	}

	created := time.Now().UTC().Format(time.RFC3339)
	names := a.cfg.aggregationNames()
	data := make([]map[string]any, 0, len(names))
	for _, n := range names {
		data = append(data, map[string]any{
			"type":         "model",
			"id":           n,
			"display_name": n,
			"created_at":   created,
		})
	}

	resp := map[string]any{
		"data":     data,
		"has_more": false,
	}
	if len(names) > 0 {
		resp["first_id"] = names[0]
		resp["last_id"] = names[len(names)-1]
	}
	return c.JSON(resp)
}
