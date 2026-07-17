package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
)

// --- OpenAI inbound request types ---

type OpenAIChatRequest struct {
	Model              string          `json:"model"`
	Messages           []OpenAIMessage `json:"messages"`
	Stream             bool            `json:"stream"`
	Temperature        *float64        `json:"temperature"`
	TopP               *float64        `json:"top_p"`
	MaxTokens          *int            `json:"max_tokens"`
	MaxCompletionTokens *int           `json:"max_completion_tokens"`
	Stop               json.RawMessage `json:"stop"`
	PresencePenalty    *float64        `json:"presence_penalty"`
	FrequencyPenalty   *float64        `json:"frequency_penalty"`
	Tools              []OpenAITool    `json:"tools"`
	ToolChoice         json.RawMessage `json:"tool_choice"`
	ReasoningEffort    string          `json:"reasoning_effort"`
}

type OpenAIMessage struct {
	Role       string           `json:"role"`
	Content    json.RawMessage  `json:"content"`
	ToolCalls  []OpenAIToolCall `json:"tool_calls"`
	Name       string           `json:"name"`
	ToolCallID string           `json:"tool_call_id"`
}

type OpenAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type OpenAITool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

// parseOpenAIRequest converts an OpenAI chat/completions request into the
// canonical Anthropic request shape used by the rest of the gateway.
func parseOpenAIRequest(o *OpenAIChatRequest) (*AnthropicRequest, error) {
	req := &AnthropicRequest{Model: o.Model, Stream: o.Stream}

	if o.Temperature != nil {
		req.Temperature = o.Temperature
	}
	if o.TopP != nil {
		req.TopP = o.TopP
	}
	if o.PresencePenalty != nil {
		req.PresencePenalty = o.PresencePenalty
	}
	if o.FrequencyPenalty != nil {
		req.FrequencyPenalty = o.FrequencyPenalty
	}
	if o.MaxCompletionTokens != nil {
		req.MaxTokens = o.MaxCompletionTokens
	}
	if o.MaxTokens != nil {
		req.MaxTokens = o.MaxTokens
	}
	if req.MaxTokens == nil {
		d := 4096
		req.MaxTokens = &d
	}
	if o.ReasoningEffort != "" {
		req.OutputConfig = &OutputConfig{Effort: o.ReasoningEffort}
	}
	if stop, err := openAIStopToSequences(o.Stop); err != nil {
		return nil, err
	} else if len(stop) > 0 {
		req.StopSequences = stop
	}

	for _, t := range o.Tools {
		req.Tools = append(req.Tools, AnthropicTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: t.Function.Parameters,
		})
	}
	if len(o.ToolChoice) > 0 {
		tc, err := openAIToolChoice(o.ToolChoice)
		if err != nil {
			return nil, err
		}
		req.ToolChoice = tc
	}

	for _, m := range o.Messages {
		switch m.Role {
		case "system":
			c, err := openAIContentToBlocks(m.Content)
			if err != nil {
				return nil, err
			}
			if c != nil && (c.Str != "" || len(c.Blocks) > 0) {
				req.System = c
			}
		case "user":
			c, err := openAIContentToBlocks(m.Content)
			if err != nil {
				return nil, err
			}
			req.Messages = append(req.Messages, AnthropicMessage{Role: "user", Content: c})
		case "assistant":
			req.Messages = append(req.Messages, buildAnthropicAssistant(m))
		case "tool":
			req.Messages = appendToolResult(req.Messages, m)
		}
	}
	return req, nil
}

func openAIContentToBlocks(raw json.RawMessage) (*StringOrBlocks, error) {
	if len(raw) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return &StringOrBlocks{IsString: true, Str: ""}, nil
	}
	trimmed := bytes.TrimSpace(raw)
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return nil, err
		}
		return &StringOrBlocks{IsString: true, Str: s}, nil
	}
	var parts []map[string]any
	if err := json.Unmarshal(trimmed, &parts); err != nil {
		return nil, err
	}
	blocks := make([]ContentBlock, 0, len(parts))
	for _, p := range parts {
		switch p["type"] {
		case "text":
			if t, ok := p["text"].(string); ok {
				blocks = append(blocks, ContentBlock{Type: "text", Text: t})
			}
		case "image_url":
			url := nestedString(p["image_url"], "url")
			blocks = append(blocks, ContentBlock{Type: "image", Source: imageSourceFromURL(url)})
		}
	}
	if len(blocks) == 0 {
		return &StringOrBlocks{IsString: true, Str: ""}, nil
	}
	return &StringOrBlocks{Blocks: blocks}, nil
}

func imageSourceFromURL(url string) *ImageSource {
	if strings.HasPrefix(url, "data:") {
		comma := strings.Index(url, ",")
		if comma < 0 {
			return &ImageSource{Type: "url", URL: url}
		}
		meta := url[5:comma]
		parts := strings.SplitN(meta, ";", 2)
		mediaType := parts[0]
		data := url[comma+1:]
		return &ImageSource{Type: "base64", MediaType: mediaType, Data: data}
	}
	return &ImageSource{Type: "url", URL: url}
}

func nestedString(v any, key string) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

func buildAnthropicAssistant(m OpenAIMessage) AnthropicMessage {
	if len(m.ToolCalls) == 0 {
		c, _ := openAIContentToBlocks(m.Content)
		return AnthropicMessage{Role: "assistant", Content: c}
	}
	var blocks []ContentBlock
	if len(m.Content) > 0 {
		if c, _ := openAIContentToBlocks(m.Content); c != nil {
			if c.IsString && c.Str != "" {
				blocks = append(blocks, ContentBlock{Type: "text", Text: c.Str})
			} else if len(c.Blocks) > 0 {
				blocks = append(blocks, c.Blocks...)
			}
		}
	}
	for _, tc := range m.ToolCalls {
		blocks = append(blocks, ContentBlock{
			Type: "tool_use",
			ID:   tc.ID,
			Name: tc.Function.Name,
			Input: rawJSON([]byte(tc.Function.Arguments)),
		})
	}
	if len(blocks) == 0 {
		blocks = []ContentBlock{{Type: "text", Text: ""}}
	}
	return AnthropicMessage{Role: "assistant", Content: &StringOrBlocks{Blocks: blocks}}
}

func appendToolResult(messages []AnthropicMessage, m OpenAIMessage) []AnthropicMessage {
	text := ""
	if c, _ := openAIContentToBlocks(m.Content); c != nil {
		if c.IsString {
			text = c.Str
		} else {
			text = c.PlainText(" ")
		}
	}
	if text == "" {
		text = "Success"
	}
	block := ContentBlock{
		Type:       "tool_result",
		ToolUseID:  m.ToolCallID,
		Content:    &StringOrBlocks{IsString: true, Str: text},
	}
	if len(messages) > 0 {
		last := &messages[len(messages)-1]
		if last.Role == "user" && last.Content != nil && !last.Content.IsString {
			last.Content.Blocks = append(last.Content.Blocks, block)
			return messages
		}
	}
	return append(messages, AnthropicMessage{
		Role:    "user",
		Content: &StringOrBlocks{Blocks: []ContentBlock{block}},
	})
}

func openAIStopToSequences(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil, nil
	}
	trimmed := bytes.TrimSpace(raw)
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return nil, err
		}
		return []string{s}, nil
	}
	var arr []string
	if err := json.Unmarshal(trimmed, &arr); err != nil {
		return nil, err
	}
	return arr, nil
}

func openAIToolChoice(raw json.RawMessage) (*ToolChoice, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, nil
	}
	if trimmed[0] == '"' {
		var s string
		json.Unmarshal(trimmed, &s)
		switch s {
		case "required":
			return &ToolChoice{Type: "any"}, nil
		case "none":
			return &ToolChoice{Type: "auto"}, nil
		default:
			return &ToolChoice{Type: "auto"}, nil
		}
	}
	var obj struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return nil, err
	}
	if obj.Type == "function" {
		return &ToolChoice{Type: "tool", Name: obj.Function.Name}, nil
	}
	return &ToolChoice{Type: "auto"}, nil
}

// --- Response conversion (provider -> OpenAI) ---

func transformAnthropicToOpenAIMap(resp map[string]any, model string) map[string]any {
	var textParts []string
	var toolCalls []map[string]any
	if content, ok := resp["content"].([]any); ok {
		for _, cb := range content {
			b, _ := cb.(map[string]any)
			if b == nil {
				continue
			}
			switch b["type"] {
			case "text":
				if t, ok := b["text"].(string); ok {
					textParts = append(textParts, t)
				}
			case "tool_use":
				input := b["input"]
				args, _ := json.Marshal(input)
				toolCalls = append(toolCalls, map[string]any{
					"id":   b["id"],
					"type": "function",
					"function": map[string]any{
						"name":      b["name"],
						"arguments": string(args),
					},
				})
			}
		}
	}

	fr := "stop"
	switch resp["stop_reason"] {
	case "tool_use":
		fr = "tool_calls"
	case "max_tokens", "length":
		fr = "length"
	}

	var inTok, outTok, reasoningTok int
	if usage, ok := resp["usage"].(map[string]any); ok {
		inTok = int(toFloat(usage["input_tokens"]))
		outTok = int(toFloat(usage["output_tokens"]))
		reasoningTok = int(toFloat(usage["reasoning_tokens"]))
	}

	msg := map[string]any{"role": "assistant", "content": join(textParts, "\n")}
	if len(textParts) == 0 {
		msg["content"] = ""
	}
	if len(toolCalls) > 0 {
		msg["tool_calls"] = toolCalls
	}

	usageOut := map[string]any{
		"prompt_tokens":     inTok,
		"completion_tokens": outTok,
		"total_tokens":      inTok + outTok,
	}
	if reasoningTok > 0 {
		usageOut["reasoning_tokens"] = reasoningTok
		usageOut["completion_tokens_details"] = map[string]any{"reasoning_tokens": reasoningTok}
	}

	return map[string]any{
		"id":      "chatcmpl-" + uuid.New().String(),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{{
			"index":         0,
			"message":       msg,
			"finish_reason": fr,
		}},
		"usage": usageOut,
	}
}

// --- Streaming (provider SSE -> OpenAI SSE) ---

func writeOpenAISSE(w *bufio.Writer, chunk map[string]any) {
	w.WriteString("data: ")
	w.WriteString(jsonString(chunk))
	w.WriteString("\n\n")
	w.Flush()
}

func streamAnthropicToOpenAI(w *bufio.Writer, body io.Reader, model string) {
	var promptTokens, completionTokens int

	emit := func(delta map[string]any, finish string) {
		chunk := map[string]any{
			"id":      "chatcmpl-" + uuid.New().String(),
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   model,
			"choices": []map[string]any{{"index": 0, "delta": delta, "finish_reason": finish}},
		}
		if finish != "" && (promptTokens > 0 || completionTokens > 0) {
			chunk["usage"] = map[string]any{
				"prompt_tokens":     promptTokens,
				"completion_tokens": completionTokens,
				"total_tokens":      promptTokens + completionTokens,
			}
		}
		writeOpenAISSE(w, chunk)
	}

	sc := newScanner(body)
	emittedRole := false
	toolIndexByBlock := map[int]int{}
	nextToolIndex := 0

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimSpace(line[6:])
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			continue
		}
		switch ev["type"] {
		case "message_start":
			if !emittedRole {
				emit(map[string]any{"role": "assistant"}, "")
				emittedRole = true
			}
			if msg, ok := ev["message"].(map[string]any); ok {
				if u, ok := msg["usage"].(map[string]any); ok {
					if pt, ok := u["input_tokens"]; ok {
						promptTokens = int(toFloat(pt))
					}
				}
			}
		case "content_block_start":
			idx := int(toFloat(ev["index"]))
			blk, _ := ev["content_block"].(map[string]any)
			if blk != nil && blk["type"] == "tool_use" {
				ti := nextToolIndex
				nextToolIndex++
				toolIndexByBlock[idx] = ti
				emit(map[string]any{"tool_calls": []map[string]any{{
					"index":    ti,
					"id":       blk["id"],
					"type":     "function",
					"function": map[string]any{"name": blk["name"], "arguments": ""},
				}}}, "")
			}
		case "content_block_delta":
			idx := int(toFloat(ev["index"]))
			delta, _ := ev["delta"].(map[string]any)
			if delta == nil {
				continue
			}
			switch delta["type"] {
			case "text_delta":
				if txt, ok := delta["text"].(string); ok && txt != "" {
					emit(map[string]any{"content": txt}, "")
				}
			case "input_json_delta":
				ti, ok := toolIndexByBlock[idx]
				if !ok {
					ti = 0
				}
				partial, _ := delta["partial_json"].(string)
				emit(map[string]any{"tool_calls": []map[string]any{{
					"index":    ti,
					"function": map[string]any{"arguments": partial},
				}}}, "")
			}
		case "message_delta":
			if u, ok := ev["usage"].(map[string]any); ok {
				if ct, ok := u["output_tokens"]; ok {
					completionTokens = int(toFloat(ct))
				}
			}
			fr := "stop"
			switch ev["stop_reason"] {
			case "tool_use":
				fr = "tool_calls"
			case "max_tokens", "length":
				fr = "length"
			}
			emit(map[string]any{}, fr)
		}
	}
	writeOpenAISSE(w, map[string]any{"data": "[DONE]"})
}

func streamV1ResponsesToOpenAI(w *bufio.Writer, body io.Reader, model string) {
	var promptTokens, completionTokens int

	emit := func(delta map[string]any, finish string) {
		chunk := map[string]any{
			"id":      "chatcmpl-" + uuid.New().String(),
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   model,
			"choices": []map[string]any{{"index": 0, "delta": delta, "finish_reason": finish}},
		}
		if finish != "" && (promptTokens > 0 || completionTokens > 0) {
			chunk["usage"] = map[string]any{
				"prompt_tokens":     promptTokens,
				"completion_tokens": completionTokens,
				"total_tokens":      promptTokens + completionTokens,
			}
		}
		writeOpenAISSE(w, chunk)
	}

	sc := newScanner(body)
	emittedRole := false
	toolIndexByOutput := map[int]int{}
	nextToolIndex := 0
	eventType := ""

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimSpace(line[7:])
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimSpace(line[6:])
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var chunk map[string]any
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		switch eventType {
		case "response.created":
			if !emittedRole {
				emit(map[string]any{"role": "assistant"}, "")
				emittedRole = true
			}
		case "response.output_item.added":
			item, _ := chunk["item"].(map[string]any)
			if item == nil || item["type"] != "function_call" {
				continue
			}
			outputIndex := int(toFloat(chunk["output_index"]))
			ti := nextToolIndex
			nextToolIndex++
			toolIndexByOutput[outputIndex] = ti
			id, _ := item["call_id"].(string)
			if id == "" {
				id, _ = item["id"].(string)
			}
			emit(map[string]any{"tool_calls": []map[string]any{{
				"index":    ti,
				"id":       id,
				"type":     "function",
				"function": map[string]any{"name": item["name"], "arguments": ""},
			}}}, "")
		case "response.output_text.delta":
			if delta, ok := chunk["delta"].(string); ok && delta != "" {
				emit(map[string]any{"content": delta}, "")
			}
		case "response.function_call_delta":
			outputIndex := int(toFloat(chunk["output_index"]))
			ti, ok := toolIndexByOutput[outputIndex]
			if !ok {
				ti = 0
			}
			if delta, ok := chunk["delta"].(map[string]any); ok {
				if a, ok := delta["arguments"].(string); ok && a != "" {
					emit(map[string]any{"tool_calls": []map[string]any{{
						"index":    ti,
						"function": map[string]any{"arguments": a},
					}}}, "")
				}
			}
		case "response.completed":
			if resp, ok := chunk["response"].(map[string]any); ok {
				if u, ok := resp["usage"].(map[string]any); ok {
					if pt, ok := u["input_tokens"]; ok {
						promptTokens = int(toFloat(pt))
					}
					if ct, ok := u["output_tokens"]; ok {
						completionTokens = int(toFloat(ct))
					}
				}
				fr := "stop"
				if out, ok := resp["output"].([]any); ok {
					for _, it := range out {
						if m, ok := it.(map[string]any); ok {
							if t, _ := m["type"].(string); t == "function_call" {
								fr = "tool_calls"
								break
							}
						}
					}
				}
				emit(map[string]any{}, fr)
			}
		}
	}
	writeOpenAISSE(w, map[string]any{"data": "[DONE]"})
}

// openAIError builds an OpenAI-formatted error payload.
func openAIError(errType, message string) map[string]any {
	return map[string]any{
		"error": map[string]any{
			"type":    errType,
			"message": message,
		},
	}
}
