package main

import (
	"bytes"
	"encoding/json"
)

// AnthropicRequest mirrors the Anthropic v1/messages request body.
type AnthropicRequest struct {
	Model            string             `json:"model"`
	Messages         []AnthropicMessage `json:"messages"`
	System           *StringOrBlocks    `json:"system,omitempty"`
	Tools            []AnthropicTool    `json:"tools,omitempty"`
	ToolChoice       *ToolChoice        `json:"tool_choice,omitempty"`
	MaxTokens        *int               `json:"max_tokens,omitempty"`
	Temperature      *float64           `json:"temperature,omitempty"`
	TopP             *float64           `json:"top_p,omitempty"`
	PresencePenalty  *float64           `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64           `json:"frequency_penalty,omitempty"`
	StopSequences    []string           `json:"stop_sequences,omitempty"`
	Stream           bool               `json:"stream,omitempty"`
}

// AnthropicMessage is a single message in the conversation.
type AnthropicMessage struct {
	Role    string          `json:"role"`
	Content *StringOrBlocks `json:"content"`
}

// AnthropicTool is a tool definition in Anthropic format.
type AnthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// ToolChoice mirrors Anthropic's tool_choice object.
type ToolChoice struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

// ContentBlock is a polymorphic Anthropic content block. Only the fields
// relevant to Type are populated.
type ContentBlock struct {
	Type string `json:"type"`

	// text
	Text string `json:"text,omitempty"`

	// image
	Source *ImageSource `json:"source,omitempty"`

	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// tool_result
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   *StringOrBlocks `json:"content,omitempty"`
}

// ImageSource is an Anthropic image source (base64 or url).
type ImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

// StringOrBlocks represents a JSON value that is either a plain string or an
// array of content blocks (Anthropic uses this for system, content, etc.).
type StringOrBlocks struct {
	IsString bool
	Str      string
	Blocks   []ContentBlock
}

// UnmarshalJSON decodes either a string or an array of ContentBlock.
func (s *StringOrBlocks) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	if trimmed[0] == '"' {
		s.IsString = true
		return json.Unmarshal(trimmed, &s.Str)
	}
	s.IsString = false
	return json.Unmarshal(trimmed, &s.Blocks)
}

// PlainText returns the flattened text of the value. For a string it returns
// the string; for blocks it joins the text of "text" blocks with sep.
func (s *StringOrBlocks) PlainText(sep string) string {
	if s == nil {
		return ""
	}
	if s.IsString {
		return s.Str
	}
	var parts []string
	for _, b := range s.Blocks {
		if b.Type == "text" {
			parts = append(parts, b.Text)
		}
	}
	return join(parts, sep)
}

func join(parts []string, sep string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += sep
		}
		out += p
	}
	return out
}
