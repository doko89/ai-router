package main

import (
	"encoding/json"

	"github.com/google/uuid"
)

// convertImageSource converts an Anthropic image source to an OpenAI data URI.
func convertImageSource(src *ImageSource) string {
	if src == nil {
		return ""
	}
	if src.Type == "base64" {
		return "data:" + src.MediaType + ";base64," + src.Data
	}
	return src.URL
}

// transformRequestBody translates an Anthropic v1/messages request into an
// OpenAI v1/chat/completions request body.
func transformRequestBody(req *AnthropicRequest) map[string]any {
	openaiMessages := make([]map[string]any, 0, len(req.Messages)+1)

	// 1. System prompt
	if req.System != nil {
		openaiMessages = append(openaiMessages, map[string]any{
			"role":    "system",
			"content": req.System.PlainText("\n"),
		})
	}

	// 2. Message history
	for _, msg := range req.Messages {
		switch msg.Role {
		case "user":
			openaiMessages = appendUserMessage(openaiMessages, msg)
		case "assistant":
			openaiMessages = append(openaiMessages, buildAssistantMessage(msg))
		}
	}

	// 3. Tools
	tools := make([]map[string]any, 0, len(req.Tools))
	for _, t := range req.Tools {
		tools = append(tools, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  rawOrNull(t.InputSchema),
			},
		})
	}

	// 4. Final body
	body := map[string]any{
		"model":    req.Model,
		"messages": openaiMessages,
		"stream":   req.Stream,
	}

	if req.MaxTokens != nil {
		body["max_completion_tokens"] = *req.MaxTokens
	}
	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}

	if n := len(openaiMessages); n > 0 && openaiMessages[n-1]["role"] == "assistant" {
		body["continue_final_message"] = true
		body["add_generation_prompt"] = false
	}

	if len(req.StopSequences) > 0 {
		body["stop"] = req.StopSequences
	}
	if req.TopP != nil {
		body["top_p"] = *req.TopP
	}
	if req.PresencePenalty != nil {
		body["presence_penalty"] = *req.PresencePenalty
	}
	if req.FrequencyPenalty != nil {
		body["frequency_penalty"] = *req.FrequencyPenalty
	}

	if len(tools) > 0 {
		body["tools"] = tools
		if req.ToolChoice != nil {
			switch req.ToolChoice.Type {
			case "any":
				body["tool_choice"] = "required"
			case "auto":
				body["tool_choice"] = "auto"
			case "tool":
				body["tool_choice"] = map[string]any{
					"type":     "function",
					"function": map[string]any{"name": req.ToolChoice.Name},
				}
			}
		}
	}

	return body
}

// appendUserMessage handles a user message, splitting tool results into
// separate OpenAI "tool" role messages when present.
func appendUserMessage(messages []map[string]any, msg AnthropicMessage) []map[string]any {
	c := msg.Content
	if c == nil {
		return append(messages, map[string]any{"role": "user", "content": ""})
	}
	if c.IsString {
		return append(messages, map[string]any{"role": "user", "content": c.Str})
	}

	if hasToolResult(c.Blocks) {
		for _, block := range c.Blocks {
			if block.Type != "tool_result" {
				continue
			}
			content := block.Content.PlainText(" ")
			if content == "" {
				content = "Success"
			}
			messages = append(messages, map[string]any{
				"role":         "tool",
				"tool_call_id": block.ToolUseID,
				"content":      content,
			})
		}
		return messages
	}

	// Standard multimodal user message.
	parts := make([]map[string]any, 0, len(c.Blocks))
	for _, block := range c.Blocks {
		switch block.Type {
		case "text":
			parts = append(parts, map[string]any{"type": "text", "text": block.Text})
		case "image":
			parts = append(parts, map[string]any{
				"type":      "image_url",
				"image_url": map[string]any{"url": convertImageSource(block.Source)},
			})
		}
	}
	return append(messages, map[string]any{"role": "user", "content": parts})
}

// buildAssistantMessage converts an assistant message, extracting text and
// tool calls.
func buildAssistantMessage(msg AnthropicMessage) map[string]any {
	out := map[string]any{"role": "assistant"}
	c := msg.Content
	if c == nil {
		return out
	}
	if c.IsString {
		out["content"] = c.Str
		return out
	}

	var textParts []string
	var toolCalls []map[string]any
	for _, block := range c.Blocks {
		switch block.Type {
		case "text":
			textParts = append(textParts, block.Text)
		case "tool_use":
			toolCalls = append(toolCalls, map[string]any{
				"id":   block.ID,
				"type": "function",
				"function": map[string]any{
					"name":      block.Name,
					"arguments": string(rawJSON(block.Input)),
				},
			})
		}
	}
	if len(textParts) > 0 {
		out["content"] = join(textParts, "\n")
	}
	if len(toolCalls) > 0 {
		out["tool_calls"] = toolCalls
	}
	return out
}

func hasToolResult(blocks []ContentBlock) bool {
	for _, b := range blocks {
		if b.Type == "tool_result" {
			return true
		}
	}
	return false
}

// --- Response transformation (OpenAI -> Anthropic) ---

// OpenAIResponse is the non-streaming chat/completions response shape.
type OpenAIResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// transformOpenAIResponse converts an OpenAI response into an Anthropic message.
func transformOpenAIResponse(resp *OpenAIResponse) map[string]any {
	contentBlocks := make([]map[string]any, 0, 2)
	stopReason := "end_turn"

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		if choice.Message.Content != "" {
			contentBlocks = append(contentBlocks, map[string]any{
				"type": "text",
				"text": choice.Message.Content,
			})
		}
		for _, tc := range choice.Message.ToolCalls {
			contentBlocks = append(contentBlocks, map[string]any{
				"type":  "tool_use",
				"id":    tc.ID,
				"name":  tc.Function.Name,
				"input": parseJSONObject(tc.Function.Arguments),
			})
		}
		switch choice.FinishReason {
		case "tool_calls":
			stopReason = "tool_use"
		case "length":
			stopReason = "max_tokens"
		}
	}

	model := resp.Model
	if model == "" {
		model = "unknown"
	}

	return map[string]any{
		"id":            "msg_" + uuid.New().String(),
		"type":          "message",
		"role":          "assistant",
		"content":       contentBlocks,
		"model":         model,
		"stop_reason":   stopReason,
		"stop_sequence": nil,
		"usage": map[string]any{
			"input_tokens":  resp.Usage.PromptTokens,
			"output_tokens": resp.Usage.CompletionTokens,
		},
	}
}

// --- JSON helpers ---

func rawOrNull(r json.RawMessage) any {
	if len(r) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(r, &v); err != nil {
		return nil
	}
	return v
}

func rawJSON(r json.RawMessage) json.RawMessage {
	if len(r) == 0 {
		return json.RawMessage("{}")
	}
	return r
}

func parseJSONObject(s string) any {
	if s == "" {
		return map[string]any{}
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return map[string]any{}
	}
	return v
}
