package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
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
		client:       &http.Client{Timeout: 60 * time.Second},
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

// handleMessages implements POST /v1/messages.
func (a *App) handleMessages(c fiber.Ctx) error {
	targetAPIKey := c.Get("x-api-key")
	if targetAPIKey == "" {
		targetAPIKey = a.cfg.OpenAIAPIKey
	}
	if targetAPIKey == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(
			anthropicError("authentication_error", "Missing API Key. Provide via x-api-key header or .env"))
	}

	var req AnthropicRequest
	if err := json.Unmarshal(c.Body(), &req); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(
			anthropicError("internal_server_error", err.Error()))
	}

	var targetBody map[string]any
	if a.cfg.APIType == APIResponses {
		log.Println("Using v1/responses API transformation")
		targetBody = transformRequestBodyV1Responses(&req)
	} else {
		log.Println("Using v1/chat/completions API transformation")
		targetBody = transformRequestBody(&req)
	}

	stream, _ := targetBody["stream"].(bool)

	if stream {
		return a.proxyStream(c, targetBody, targetAPIKey)
	}
	return a.proxyOnce(c, targetBody, targetAPIKey)
}

// proxyOnce forwards a non-streaming request and transforms the response.
func (a *App) proxyOnce(c fiber.Ctx, targetBody map[string]any, apiKey string) error {
	resp, err := a.forward(a.client, targetBody, apiKey)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(
			anthropicError("internal_server_error", err.Error()))
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		log.Printf("⚠️  Upstream returned %d from %s", resp.StatusCode, a.cfg.OpenAIBaseURL)
		var errResp struct {
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(data, &errResp)
		errType := errResp.Error.Type
		if errType == "" {
			errType = "invalid_request_error"
		}
		message := errResp.Error.Message
		if message == "" {
			message = "Upstream returned non-200 status"
		}
		return c.Status(resp.StatusCode).JSON(anthropicError(errType, message))
	}

	var anthropicResp map[string]any
	if a.cfg.APIType == APIResponses {
		var parsed V1ResponsesResponse
		if err := json.Unmarshal(data, &parsed); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(
				anthropicError("internal_server_error", err.Error()))
		}
		anthropicResp = transformV1ResponsesResponse(&parsed)
	} else {
		var parsed OpenAIResponse
		if err := json.Unmarshal(data, &parsed); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(
				anthropicError("internal_server_error", err.Error()))
		}
		anthropicResp = transformOpenAIResponse(&parsed)
	}

	return c.JSON(anthropicResp)
}

// proxyStream forwards a streaming request and translates the SSE stream.
func (a *App) proxyStream(c fiber.Ctx, targetBody map[string]any, apiKey string) error {
	resp, err := a.forward(a.streamClient, targetBody, apiKey)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(
			anthropicError("internal_server_error", err.Error()))
	}

	c.Set(fiber.HeaderContentType, "text/event-stream")
	c.Set(fiber.HeaderCacheControl, "no-cache")
	c.Set(fiber.HeaderConnection, "keep-alive")

	return c.SendStreamWriter(func(w *bufio.Writer) {
		defer resp.Body.Close()
		if a.cfg.APIType == APIResponses {
			streamV1ResponsesToAnthropic(w, resp.Body)
		} else {
			streamOpenAIToAnthropic(w, resp.Body)
		}
	})
}

// forward sends the transformed body to the upstream API.
func (a *App) forward(client *http.Client, body map[string]any, apiKey string) (*http.Response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, a.cfg.OpenAIBaseURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	return client.Do(req)
}

// handleCountTokens implements POST /v1/messages/count_tokens.
func (a *App) handleCountTokens(c fiber.Ctx) error {
	var req AnthropicRequest
	if err := json.Unmarshal(c.Body(), &req); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(map[string]any{"error": err.Error()})
	}
	openaiBody := transformRequestBody(&req)
	tokenCount := countOpenAITokens(openaiBody, a.cfg.TiktokenEncoding)
	return c.JSON(map[string]any{"input_tokens": tokenCount})
}
