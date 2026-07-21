package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/mark3labs/mcp-go/mcp"
)

// App holds shared dependencies for the HTTP handlers.
type App struct {
	cfg          *Config
	client       *http.Client
	streamClient *http.Client
	mcp          *MCPServerManager

	// cooldowns maps a cooldown key to the time until which it should be
	// skipped (rate-limited). The key is either "provider" or "provider:model"
	// depending on the rate_limit_scope config. Read/written concurrently.
	cooldowns sync.Map
}

// NewApp creates an App with configured HTTP clients and MCP manager.
func NewApp(cfg *Config) *App {
	return &App{
		cfg:          cfg,
		client:       &http.Client{Timeout: 120 * time.Second},
		streamClient: &http.Client{},
		mcp:          NewMCPServerManager(cfg.MCPServers),
	}
}

// effectiveCooldown returns the configured cooldown seconds for a provider
// (per-provider override wins, else gateway default). 0 means disabled.
func (a *App) effectiveCooldown(p *Provider) int {
	if p.RateLimitCooldown > 0 {
		return p.RateLimitCooldown
	}
	return a.cfg.Gateway.RateLimitCooldown
}

// cooldownKey builds the map key based on the configured scope.
// "provider" → provider name; "model" → "provider:model".
func (a *App) cooldownKey(p *Provider, model string) string {
	if a.cfg.Gateway.RateLimitScope == "model" {
		return p.Name + ":" + model
	}
	return p.Name
}

// isCooling reports whether the provider (or provider+model) is temporarily skipped.
func (a *App) isCooling(p *Provider, model string) bool {
	v, ok := a.cooldowns.Load(a.cooldownKey(p, model))
	if !ok {
		return false
	}
	until, ok := v.(time.Time)
	return ok && time.Now().Before(until)
}

// markCooldown records a cooldown, honoring a Retry-After header when present,
// otherwise the configured cooldown. Returns false if cooling is disabled or
// could not be determined.
func (a *App) markCooldown(p *Provider, model string, retryAfter string) bool {
	secs := parseRetryAfter(retryAfter)
	if secs <= 0 {
		secs = a.effectiveCooldown(p)
	}
	if secs <= 0 {
		return false
	}
	a.cooldowns.Store(a.cooldownKey(p, model), time.Now().Add(time.Duration(secs)*time.Second))
	return true
}

// retryAfterFor returns the largest remaining cooldown (seconds) among the
// given candidates, or 0 if none are cooling.
func (a *App) retryAfterFor(candidates []Target) int {
	max := 0
	for _, tgt := range candidates {
		v, ok := a.cooldowns.Load(a.cooldownKey(tgt.Provider, tgt.Model))
		if !ok {
			continue
		}
		until, ok := v.(time.Time)
		if !ok {
			continue
		}
		rem := int(time.Until(until).Seconds())
		if rem < 0 {
			rem = 0
		}
		if rem > max {
			max = rem
		}
	}
	return max
}

// parseRetryAfter parses an HTTP Retry-After value (delta-seconds or HTTP-date)
// into seconds. Returns 0 if unparseable.
func parseRetryAfter(header string) int {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0
	}
	if v, err := strconv.Atoi(header); err == nil && v > 0 {
		return v
	}
	if t, err := http.ParseTime(header); err == nil {
		secs := int(time.Until(t).Seconds())
		if secs > 0 {
			return secs
		}
	}
	return 0
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

	if a.cfg.Gateway.Compression != "" && a.cfg.Gateway.Compression != CompressionOff {
		level := a.cfg.Gateway.Compression
		var origBody []byte
		origBody, bodyBytes = bodyBytes, nil
		compressAnthropicRequest(&req, level)
		bodyBytes, _ = json.Marshal(req)
		if len(bodyBytes) > len(origBody) {
			bodyBytes = origBody
			if a.cfg.Gateway.Debug {
				log.Printf("[%s] compression inflated %d→%d bytes, discarded level=%s", name, len(origBody), len(bodyBytes), normalizeLevel(level))
			}
		} else if a.cfg.Gateway.Debug {
			saved := len(origBody) - len(bodyBytes)
			pct := 100 * saved / len(origBody)
			log.Printf("[%s] compression saved %d%% (%d→%d, -%d) level=%s", name, pct, len(origBody), len(bodyBytes), saved, normalizeLevel(level))
		}
	}

	// MCP tool merging: append MCP tools to the user's tool definitions.
	if a.mcp != nil && a.mcp.HasTools() && !req.Stream {
		mcpTools := a.mcp.ListAnthropicTools()
		if len(mcpTools) > 0 {
			// Avoid duplicates — MCP tool names are prefixed so conflicts are
			// unlikely, but check anyway.
			existing := make(map[string]bool, len(req.Tools))
			for _, t := range req.Tools {
				existing[t.Name] = true
			}
			for _, mt := range mcpTools {
				if !existing[mt.Name] {
					req.Tools = append(req.Tools, mt)
					existing[mt.Name] = true
				}
			}
			bodyBytes, _ = json.Marshal(req)
			if a.cfg.Gateway.Debug {
				log.Printf("[%s] merged %d MCP tools (%d total)", name, len(mcpTools), len(req.Tools))
			}
		}
	}

	// Auto-execute loop for non-streaming requests with MCP tools.
	// Each successful response is checked for MCP tool_use blocks; if found,
	// the tool is executed, results appended to messages, and the request
	// re-dispatched (up to 9 additional rounds).
	maxMCPSteps := 10
	isStream := req.Stream

	for mcpStep := 0; mcpStep < maxMCPSteps; mcpStep++ {
		candidates, exists := a.cfg.resolveCandidates(req.Model)
		if !exists {
			return c.Status(fiber.StatusNotFound).JSON(anthropicError("not_found_error",
				fmt.Sprintf("model %q not found. Available: %s", req.Model, strings.Join(a.cfg.aggregationNames(), ", "))))
		}
		if len(candidates) == 0 {
			return c.Status(fiber.StatusServiceUnavailable).JSON(anthropicError("overloaded_error",
				fmt.Sprintf("no enabled provider available for model %q", req.Model)))
		}

		if a.cfg.Gateway.Debug && mcpStep == 0 {
			log.Printf("[%s] model=%q stream=%v candidates=%d", name, req.Model, req.Stream, len(candidates))
		}

		var lastErr string
		rateLimited := false
		dispatched := false

	for _, tgt := range candidates {
		if a.isCooling(tgt.Provider, tgt.Model) {
			if a.cfg.Gateway.Debug {
				log.Printf("  provider=%s model=%s skipped (rate-limit cooldown)", tgt.Provider.Name, tgt.Model)
			}
			continue
		}
		resp, err := a.dispatch(tgt, bodyBytes, &req)
		if err != nil {
			lastErr = err.Error()
			if a.cfg.Gateway.Debug {
				log.Printf("  provider=%s model=%s dial error: %v", tgt.Provider.Name, tgt.Model, err)
			}
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			rateLimited = true
			a.markCooldown(tgt.Provider, tgt.Model, resp.Header.Get("Retry-After"))
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if a.cfg.Gateway.Debug {
					log.Printf("  provider=%s model=%s rate limited; cooling down", tgt.Provider.Name, tgt.Model)
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

			dispatched = true

			if a.cfg.Gateway.Debug {
				log.Printf("[%s] → provider=%s model=%s compatible=%s stream=%v (step %d)", name, tgt.Provider.Name, tgt.Model, tgt.Provider.Compatible, isStream, mcpStep)
			}

			if isStream {
				return a.streamResponse(c, tgt.Provider.Compatible, resp)
			}

			// Non-streaming: read response and check for MCP tool_use.
			data, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			if a.mcp != nil && mcpStep < maxMCPSteps-1 {
				modified, newBody, err := a.handleMCPStep(data, &req, tgt.Provider.Compatible, name)
				if err != nil {
					return c.Status(fiber.StatusBadGateway).JSON(anthropicError("api_error",
						fmt.Sprintf("mcp execution error: %v", err)))
				}
				if modified {
					bodyBytes = newBody
					if a.cfg.Gateway.Debug {
						log.Printf("[%s] mcp tool_use handled, re-dispatching (step %d)", name, mcpStep+1)
					}
					break // out of candidate loop → continue outer loop
				}
			}

			// No MCP tool_use → return response to caller.
			return a.sendJSONResponse(c, tgt.Provider.Compatible, data)
		}

		if !dispatched {
			// All candidates exhausted.
			if rateLimited {
				retryAfter := a.retryAfterFor(candidates)
				if retryAfter <= 0 {
					retryAfter = 1
				}
				c.Set(fiber.HeaderRetryAfter, fmt.Sprintf("%d", retryAfter))
				return c.Status(fiber.StatusTooManyRequests).JSON(
					anthropicError("rate_limit_error", fmt.Sprintf("all providers rate limited; retry after %d seconds", retryAfter)))
			}
			return c.Status(fiber.StatusBadGateway).JSON(
				anthropicError("api_error", "all providers failed: "+lastErr))
		}
	}

	return c.Status(fiber.StatusInternalServerError).JSON(
		anthropicError("overloaded_error", "maximum MCP auto-execute steps reached"))
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

// sendJSONResponse sends a non-streaming response from already-read data.
func (a *App) sendJSONResponse(c fiber.Ctx, compat Compatible, data []byte) error {
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

// handleMCPStep checks a non-streaming upstream response for MCP tool_use blocks.
// When MCP tool_use is found with auto_execute enabled:
//   - The full assistant message (text + tool_use) is appended to req.Messages
//   - Each MCP tool is executed via CallTool
//   - The results are appended as a user message with tool_result blocks
//   - The request is re-marshaled into newBody for re-dispatch
//
// Returns (modified=true, newBody, nil) when MCP tools were executed.
// Returns (modified=false, nil, nil) when no MCP tool_use is found.
func (a *App) handleMCPStep(data []byte, req *AnthropicRequest, compat Compatible, name string) (bool, []byte, error) {
	// Parse tool_use from the upstream response in its native dialect. The
	// request we re-dispatch is always Anthropic-shaped (the inbound format),
	// but the upstream response follows the target provider's dialect.
	var blocks []ContentBlock
	var uses []toolUseBlock
	switch compat {
	case CompatibleAnthropic:
		blocks, uses = findToolUseFromAnthropic(data)
	case CompatibleOpenAIResponses:
		blocks, uses = findToolUseFromResponses(data)
	default: // openai chat completions
		blocks, uses = findToolUseFromOpenAI(data)
	}
	if len(uses) == 0 {
		return false, nil, nil
	}

	// Partition tool_use blocks: MCP-managed tools with auto_execute vs. the rest.
	// Auto-execute only when EVERY tool_use is MCP-auto — a mixed turn would leave
	// a non-MCP tool_use without a matching tool_result on re-dispatch, which the
	// upstream API rejects. Mixed or non-auto turns are returned to the client as-is.
	var mcpUses, otherUses []toolUseBlock
	for _, u := range uses {
		if a.mcp.ToolAutoExecute(u.Name) {
			mcpUses = append(mcpUses, u)
		} else {
			otherUses = append(otherUses, u)
		}
	}
	if len(mcpUses) == 0 || len(otherUses) > 0 {
		return false, nil, nil
	}

	if a.cfg.Gateway.Debug {
		log.Printf("[%s] mcp: %d tool_use blocks to auto-execute", name, len(mcpUses))
	}

	// Append assistant message with ALL original content blocks (text + tool_use).
	req.Messages = append(req.Messages, AnthropicMessage{
		Role:    "assistant",
		Content: &StringOrBlocks{Blocks: blocks},
	})

	// Execute each MCP tool and build tool_result blocks.
	ctx := context.Background()
	var toolResults []ContentBlock
	for _, u := range mcpUses {
		result, err := a.mcp.CallTool(ctx, u.Name, u.Input)
		text := "Success"
		if err != nil {
			text = "Error: " + err.Error()
			if a.cfg.Gateway.Debug {
				log.Printf("[%s] mcp: tool %q error: %v", name, u.Name, err)
			}
		} else if result != nil {
			text = formatToolResult(result)
		}

		toolResults = append(toolResults, ContentBlock{
			Type:      "tool_result",
			ToolUseID: u.ID,
			Content:   &StringOrBlocks{IsString: true, Str: text},
		})
	}

	// Append user message with all tool results.
	req.Messages = append(req.Messages, AnthropicMessage{
		Role:    "user",
		Content: &StringOrBlocks{Blocks: toolResults},
	})

	// Re-marshal request body.
	newBody, err := json.Marshal(req)
	if err != nil {
		return false, nil, fmt.Errorf("re-marshal after MCP tool_use: %w", err)
	}

	return true, newBody, nil
}

// formatToolResult extracts text from an MCP CallToolResult.
func formatToolResult(result *mcp.CallToolResult) string {
	var texts []string
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			texts = append(texts, tc.Text)
		}
	}
	if len(texts) == 0 {
		return "Success"
	}
	return strings.Join(texts, "\n")
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

// handleChatCompletions implements POST /v1/chat/completions (OpenAI inbound).
// The request is converted to the canonical Anthropic shape, routed through the
// same dispatch path, and the provider response is translated back to OpenAI.
func (a *App) handleChatCompletions(c fiber.Ctx) error {
	name, ok := a.authenticate(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(
			openAIError("authentication_error", "Invalid or missing client API key"))
	}

	var oreq OpenAIChatRequest
	if err := json.Unmarshal(c.Body(), &oreq); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(openAIError("invalid_request_error", err.Error()))
	}

	req, err := parseOpenAIRequest(&oreq)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(openAIError("invalid_request_error", err.Error()))
	}

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(openAIError("api_error", err.Error()))
	}

	if a.cfg.Gateway.Compression != "" && a.cfg.Gateway.Compression != CompressionOff {
		level := a.cfg.Gateway.Compression
		var origBody []byte
		origBody, bodyBytes = bodyBytes, nil
		compressAnthropicRequest(req, level)
		bodyBytes, _ = json.Marshal(req)
		if len(bodyBytes) > len(origBody) {
			bodyBytes = origBody
		}
		if a.cfg.Gateway.Debug {
			saved := len(origBody) - len(bodyBytes)
			pct := 100 * saved / len(origBody)
			log.Printf("[%s|openai] compression saved %d%% (%d→%d, -%d) level=%s", name, pct, len(origBody), len(bodyBytes), saved, normalizeLevel(level))
		}
	}

	candidates, exists := a.cfg.resolveCandidates(req.Model)
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(openAIError("not_found_error",
			fmt.Sprintf("model %q not found. Available: %s", req.Model, strings.Join(a.cfg.aggregationNames(), ", "))))
	}
	if len(candidates) == 0 {
		return c.Status(fiber.StatusServiceUnavailable).JSON(openAIError("unavailable_error",
			fmt.Sprintf("no enabled provider available for model %q", req.Model)))
	}

	if a.cfg.Gateway.Debug {
		log.Printf("[%s|openai] model=%q stream=%v candidates=%d", name, req.Model, req.Stream, len(candidates))
	}

	var lastErr string
	rateLimited := false
	for _, tgt := range candidates {
		if a.isCooling(tgt.Provider, tgt.Model) {
			if a.cfg.Gateway.Debug {
				log.Printf("  provider=%s model=%s skipped (rate-limit cooldown)", tgt.Provider.Name, tgt.Model)
			}
			continue
		}
		resp, err := a.dispatch(tgt, bodyBytes, req)
		if err != nil {
			lastErr = err.Error()
			if a.cfg.Gateway.Debug {
				log.Printf("  provider=%s model=%s dial error: %v", tgt.Provider.Name, tgt.Model, err)
			}
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			rateLimited = true
			a.markCooldown(tgt.Provider, tgt.Model, resp.Header.Get("Retry-After"))
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if a.cfg.Gateway.Debug {
				log.Printf("  provider=%s model=%s rate limited; cooling down", tgt.Provider.Name, tgt.Model)
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

		if a.cfg.Gateway.Debug {
			log.Printf("[%s|openai] → provider=%s model=%s compatible=%s stream=%v", name, tgt.Provider.Name, tgt.Model, tgt.Provider.Compatible, req.Stream)
		}
		if req.Stream {
			return a.streamResponseOpenAI(c, tgt.Provider.Compatible, resp, req.Model)
		}
		return a.jsonResponseOpenAI(c, tgt.Provider.Compatible, resp, req.Model)
	}

	if rateLimited {
		retryAfter := a.retryAfterFor(candidates)
		if retryAfter <= 0 {
			retryAfter = 1
		}
		c.Set(fiber.HeaderRetryAfter, fmt.Sprintf("%d", retryAfter))
		return c.Status(fiber.StatusTooManyRequests).JSON(openAIError("rate_limit_error", fmt.Sprintf("all providers rate limited; retry after %d seconds", retryAfter)))
	}

	return c.Status(fiber.StatusBadGateway).JSON(openAIError("api_error", "all providers failed: "+lastErr))
}

// jsonResponseOpenAI transforms and returns a non-streaming upstream response in
// OpenAI chat/completions shape.
func (a *App) jsonResponseOpenAI(c fiber.Ctx, compat Compatible, resp *http.Response, model string) error {
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	switch compat {
	case CompatibleAnthropic:
		var parsed map[string]any
		if err := json.Unmarshal(data, &parsed); err != nil {
			return c.Status(fiber.StatusBadGateway).JSON(openAIError("api_error", err.Error()))
		}
		return c.JSON(transformAnthropicToOpenAIMap(parsed, model))
	case CompatibleOpenAIResponses:
		var parsed V1ResponsesResponse
		if err := json.Unmarshal(data, &parsed); err != nil {
			return c.Status(fiber.StatusBadGateway).JSON(openAIError("api_error", err.Error()))
		}
		return c.JSON(transformAnthropicToOpenAIMap(transformV1ResponsesResponse(&parsed), model))
	default:
		var parsed map[string]any
		if err := json.Unmarshal(data, &parsed); err != nil {
			return c.Status(fiber.StatusBadGateway).JSON(openAIError("api_error", err.Error()))
		}
		parsed["model"] = model
		return c.JSON(parsed)
	}
}

// streamResponseOpenAI transforms and streams an upstream SSE response in
// OpenAI chat/completions shape.
func (a *App) streamResponseOpenAI(c fiber.Ctx, compat Compatible, resp *http.Response, model string) error {
	c.Set(fiber.HeaderContentType, "text/event-stream")
	c.Set(fiber.HeaderCacheControl, "no-cache")
	c.Set(fiber.HeaderConnection, "keep-alive")

	return c.SendStreamWriter(func(w *bufio.Writer) {
		defer resp.Body.Close()
		switch compat {
		case CompatibleAnthropic:
			streamAnthropicToOpenAI(w, resp.Body, model)
		case CompatibleOpenAIResponses:
			streamV1ResponsesToOpenAI(w, resp.Body, model)
		default:
			copyStream(w, resp.Body)
		}
	})
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

// handleListModelsOpenAI implements GET /v1/models in OpenAI format.
func (a *App) handleListModelsOpenAI(c fiber.Ctx) error {
	if _, ok := a.authenticate(c); !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(openAIError("authentication_error", "Invalid or missing client API key"))
	}

	created := time.Now().Unix()
	names := a.cfg.aggregationNames()
	data := make([]map[string]any, 0, len(names))
	for _, n := range names {
		entry := map[string]any{
			"id":        n,
			"object":    "model",
			"created":   created,
			"owned_by":  "ai-router",
		}
		if meta, ok := a.cfg.aggregationMetadata(n); ok {
			if meta.ContextWindow > 0 {
				entry["context_window"] = meta.ContextWindow
			}
			if meta.MaxOutput > 0 {
				entry["max_output"] = meta.MaxOutput
			}
		}
		data = append(data, entry)
	}

	return c.JSON(map[string]any{
		"object": "list",
		"data":   data,
	})
}

// handleListModelsAnthropic implements GET /v1/models in Anthropic format.
func (a *App) handleListModelsAnthropic(c fiber.Ctx) error {
	if _, ok := a.authenticate(c); !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(anthropicError("authentication_error", "Invalid or missing client API key"))
	}

	created := time.Now().UTC().Format(time.RFC3339)
	names := a.cfg.aggregationNames()
	data := make([]map[string]any, 0, len(names))
	for _, n := range names {
		entry := map[string]any{
			"type":         "model",
			"id":           n,
			"display_name": n,
			"created_at":   created,
		}
		if meta, ok := a.cfg.aggregationMetadata(n); ok {
			if meta.ContextWindow > 0 {
				entry["context_window"] = meta.ContextWindow
			}
			if meta.MaxOutput > 0 {
				entry["max_output"] = meta.MaxOutput
			}
		}
		data = append(data, entry)
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
