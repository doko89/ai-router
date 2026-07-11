package main

import (
	"encoding/json"

	"github.com/google/uuid"
)

// transformRequestBodyV1Responses translates an Anthropic v1/messages request
// into an OpenAI v1/responses request body.
func transformRequestBodyV1Responses(req *AnthropicRequest) map[string]any {
	inputItems := make([]map[string]any, 0, len(req.Messages))

	// 1. System prompt -> instructions
	var instructions string
	if req.System != nil {
		instructions = req.System.PlainText("\n")
	}

	// 2. Message history -> input items
	for _, msg := range req.Messages {
		switch msg.Role {
		case "user":
			inputItems = appendResponsesUser(inputItems, msg)
		case "assistant":
			inputItems = appendResponsesAssistant(inputItems, msg)
		}
	}

	// 3. Tools
	tools := make([]map[string]any, 0, len(req.Tools))
	for _, t := range req.Tools {
		tools = append(tools, map[string]any{
			"type":        "function",
			"name":        t.Name,
			"description": t.Description,
			"parameters":  rawOrNull(t.InputSchema),
		})
	}

	// 4. Final body
	body := map[string]any{
		"model":  req.Model,
		"input":  inputItems,
		"stream": req.Stream,
	}

	if req.MaxTokens != nil {
		body["max_output_tokens"] = *req.MaxTokens
	}
	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	if instructions != "" {
		body["instructions"] = instructions
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

func appendResponsesUser(items []map[string]any, msg AnthropicMessage) []map[string]any {
	c := msg.Content
	if c == nil {
		return items
	}
	if c.IsString {
		return append(items, map[string]any{
			"type":    "message",
			"role":    "user",
			"content": []map[string]any{{"type": "input_text", "text": c.Str}},
		})
	}

	if hasToolResult(c.Blocks) {
		for _, block := range c.Blocks {
			if block.Type != "tool_result" {
				continue
			}
			out := block.Content.PlainText(" ")
			if out == "" {
				out = "Success"
			}
			items = append(items, map[string]any{
				"type":    "custom_tool_call_output",
				"call_id": block.ToolUseID,
				"output":  out,
			})
		}
		return items
	}

	contentBlocks := make([]map[string]any, 0, len(c.Blocks))
	for _, block := range c.Blocks {
		switch block.Type {
		case "text":
			contentBlocks = append(contentBlocks, map[string]any{"type": "input_text", "text": block.Text})
		case "image":
			contentBlocks = append(contentBlocks, map[string]any{
				"type":      "input_image",
				"image_url": convertImageSource(block.Source),
			})
		}
	}
	return append(items, map[string]any{
		"type":    "message",
		"role":    "user",
		"content": contentBlocks,
	})
}

func appendResponsesAssistant(items []map[string]any, msg AnthropicMessage) []map[string]any {
	c := msg.Content
	if c == nil {
		return items
	}
	if c.IsString {
		return append(items, map[string]any{
			"type":    "message",
			"role":    "assistant",
			"content": []map[string]any{{"type": "output_text", "text": c.Str}},
		})
	}

	contentBlocks := make([]map[string]any, 0, len(c.Blocks))
	for _, block := range c.Blocks {
		switch block.Type {
		case "text":
			contentBlocks = append(contentBlocks, map[string]any{"type": "output_text", "text": block.Text})
		case "tool_use":
			items = append(items, map[string]any{
				"type":      "function_call",
				"call_id":   block.ID,
				"name":      block.Name,
				"arguments": string(rawJSON(block.Input)),
			})
		}
	}
	if len(contentBlocks) > 0 {
		items = append(items, map[string]any{
			"type":    "message",
			"role":    "assistant",
			"content": contentBlocks,
		})
	}
	return items
}

// V1ResponsesResponse is the non-streaming v1/responses response shape.
type V1ResponsesResponse struct {
	Model  string `json:"model"`
	Output []struct {
		Type    string `json:"type"`
		ID      string `json:"id"`
		CallID  string `json:"call_id"`
		Name    string `json:"name"`
		Args    string `json:"arguments"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// transformV1ResponsesResponse converts a v1/responses response into an
// Anthropic message.
func transformV1ResponsesResponse(resp *V1ResponsesResponse) map[string]any {
	contentBlocks := make([]map[string]any, 0, 2)
	stopReason := "end_turn"

	for _, item := range resp.Output {
		switch item.Type {
		case "message":
			for _, content := range item.Content {
				if content.Type == "output_text" {
					contentBlocks = append(contentBlocks, map[string]any{
						"type": "text",
						"text": content.Text,
					})
				}
			}
		case "function_call":
			id := item.CallID
			if id == "" {
				id = item.ID
			}
			contentBlocks = append(contentBlocks, map[string]any{
				"type":  "tool_use",
				"id":    id,
				"name":  item.Name,
				"input": parseJSONObject(item.Args),
			})
			stopReason = "tool_use"
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
			"input_tokens":  resp.Usage.InputTokens,
			"output_tokens": resp.Usage.OutputTokens,
		},
	}
}

// jsonString marshals a value to a compact JSON string, ignoring errors.
func jsonString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
