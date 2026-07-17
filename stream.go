package main

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
	"strings"

	"github.com/google/uuid"
)

// writeSSE writes a single Server-Sent Event and flushes the writer.
func writeSSE(w *bufio.Writer, event string, data map[string]any) {
	w.WriteString("event: ")
	w.WriteString(event)
	w.WriteString("\ndata: ")
	w.WriteString(jsonString(data))
	w.WriteString("\n\n")
	w.Flush()
}

// newScanner returns a line scanner with a large buffer to handle big SSE lines.
func newScanner(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	return sc
}

// streamOpenAIToAnthropic transforms an OpenAI chat/completions SSE stream into
// the Anthropic v1/messages event stream.
func streamOpenAIToAnthropic(w *bufio.Writer, body io.Reader) {
	msgID := "msg_" + uuid.New().String()

	var completionTokens int
	var inputTokens int
	modelName := "proxy"

	writeSSE(w, "message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id": msgID, "type": "message", "role": "assistant",
			"content": []any{}, "model": modelName,
			"stop_reason": nil, "stop_sequence": nil,
			"usage": map[string]any{"input_tokens": 0, "output_tokens": 0},
		},
	})

	currentBlockIndex := 0
	toolCallIDByIndex := map[int]string{}
	toolCallNameByIndex := map[int]string{}
	writeSSE(w, "content_block_start", map[string]any{
		"type": "content_block_start", "index": currentBlockIndex,
		"content_block": map[string]any{"type": "text", "text": ""},
	})

	sc := newScanner(body)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line == "data: [DONE]" {
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		var chunk map[string]any
		if err := json.Unmarshal([]byte(line[6:]), &chunk); err != nil {
			log.Printf("Streaming Error: %v", err)
			continue
		}

		// Track model name from first chunk that has it.
		if m, ok := chunk["model"].(string); ok && m != "" && modelName == "proxy" {
			modelName = m
		}

		// Track usage from any chunk (OpenAI sends it in the final chunk).
		if u, ok := chunk["usage"].(map[string]any); ok {
			if ct, ok := u["completion_tokens"]; ok {
				completionTokens = int(toFloat(ct))
			}
			if pt, ok := u["prompt_tokens"]; ok {
				inputTokens = int(toFloat(pt))
			}
		}

		choices, _ := chunk["choices"].([]any)
		if len(choices) == 0 {
			continue
		}
		choice, _ := choices[0].(map[string]any)
		delta, _ := choice["delta"].(map[string]any)

		// Text content.
		if content, ok := delta["content"].(string); ok && content != "" {
			writeSSE(w, "content_block_delta", map[string]any{
				"type": "content_block_delta", "index": 0,
				"delta": map[string]any{"type": "text_delta", "text": content},
			})
		}

		// Tool calls.
		if tcs, ok := delta["tool_calls"].([]any); ok && len(tcs) > 0 {
			for _, tcRaw := range tcs {
				tc, _ := tcRaw.(map[string]any)
				tcIndex := int(toFloat(tc["index"]))
				targetIndex := tcIndex + 1

				// Track tool call id and name across chunks (OpenAI sends them
				// in the first chunk for each index, then arguments separately).
				if id, ok := tc["id"].(string); ok && id != "" {
					toolCallIDByIndex[tcIndex] = id
				}
				if fn, ok := tc["function"].(map[string]any); ok {
					if name, ok := fn["name"].(string); ok && name != "" {
						toolCallNameByIndex[tcIndex] = name
					}
				}

				if targetIndex != currentBlockIndex {
					writeSSE(w, "content_block_stop", map[string]any{
						"type": "content_block_stop", "index": currentBlockIndex,
					})
					currentBlockIndex = targetIndex
					writeSSE(w, "content_block_start", map[string]any{
						"type": "content_block_start", "index": currentBlockIndex,
						"content_block": map[string]any{
							"type": "tool_use",
							"id":   toolCallIDByIndex[tcIndex],
							"name": toolCallNameByIndex[tcIndex],
							"input": map[string]any{},
						},
					})
				}

				if fn, ok := tc["function"].(map[string]any); ok {
					if args, ok := fn["arguments"].(string); ok && args != "" {
						writeSSE(w, "content_block_delta", map[string]any{
							"type": "content_block_delta", "index": currentBlockIndex,
							"delta": map[string]any{"type": "input_json_delta", "partial_json": args},
						})
					}
				}
			}
		}

		// Stop reason.
		if fr, ok := choice["finish_reason"].(string); ok && fr != "" {
			anthropicReason := "end_turn"
			switch fr {
			case "tool_calls":
				anthropicReason = "tool_use"
			case "length":
				anthropicReason = "max_tokens"
			}
			writeSSE(w, "content_block_stop", map[string]any{
				"type": "content_block_stop", "index": currentBlockIndex,
			})
			writeSSE(w, "message_delta", map[string]any{
				"type":  "message_delta",
				"delta": map[string]any{"stop_reason": anthropicReason, "stop_sequence": nil},
				"usage": map[string]any{"output_tokens": completionTokens, "input_tokens": inputTokens},
			})
		}
	}

	writeSSE(w, "message_stop", map[string]any{"type": "message_stop"})
}

// streamV1ResponsesToAnthropic transforms an OpenAI v1/responses SSE stream into
// the Anthropic v1/messages event stream.
func streamV1ResponsesToAnthropic(w *bufio.Writer, body io.Reader) {
	msgID := "msg_" + uuid.New().String()
	messageStarted := false
	currentContentIndex := 0
	eventType := ""

	sc := newScanner(body)
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

		var chunk map[string]any
		if err := json.Unmarshal([]byte(line[6:]), &chunk); err != nil {
			log.Printf("Streaming Error (responses): %v", err)
			continue
		}

		switch eventType {
		case "response.output_text.delta":
			if !messageStarted {
				messageStarted = true
				writeSSE(w, "message_start", map[string]any{
					"type": "message_start",
					"message": map[string]any{
						"id": msgID, "type": "message", "role": "assistant",
						"content": []any{}, "model": "proxy",
						"stop_reason": nil, "stop_sequence": nil,
						"usage": map[string]any{"input_tokens": 0, "output_tokens": 0},
					},
				})
				writeSSE(w, "content_block_start", map[string]any{
					"type": "content_block_start", "index": 0,
					"content_block": map[string]any{"type": "text", "text": ""},
				})
			}
			if delta, ok := chunk["delta"].(string); ok && delta != "" {
				writeSSE(w, "content_block_delta", map[string]any{
					"type": "content_block_delta", "index": 0,
					"delta": map[string]any{"type": "text_delta", "text": delta},
				})
			}

		case "response.output_text.done":
			if !messageStarted {
				messageStarted = true
				writeSSE(w, "message_start", map[string]any{
					"type": "message_start",
					"message": map[string]any{
						"id": msgID, "type": "message", "role": "assistant",
						"content": []any{}, "model": "proxy",
						"stop_reason": nil, "stop_sequence": nil,
						"usage": map[string]any{"input_tokens": 0, "output_tokens": 0},
					},
				})
			}
			writeSSE(w, "content_block_stop", map[string]any{
				"type": "content_block_stop", "index": currentContentIndex,
			})

		case "response.completed":
			responseData, _ := chunk["response"].(map[string]any)
			usage, _ := responseData["usage"].(map[string]any)
			output, _ := responseData["output"].([]any)

			stopReason := "end_turn"
			for _, it := range output {
				if m, ok := it.(map[string]any); ok {
					if t, _ := m["type"].(string); t == "function_call" {
						stopReason = "tool_use"
						break
					}
				}
			}

			outputTokens := 0
			if usage != nil {
				outputTokens = int(toFloat(usage["output_tokens"]))
			}
			writeSSE(w, "message_delta", map[string]any{
				"type":  "message_delta",
				"delta": map[string]any{"stop_reason": stopReason, "stop_sequence": nil},
				"usage": map[string]any{"output_tokens": outputTokens},
			})
			writeSSE(w, "message_stop", map[string]any{"type": "message_stop"})
		}
	}

	if messageStarted {
		writeSSE(w, "message_stop", map[string]any{"type": "message_stop"})
	}
}

// toFloat coerces a JSON-decoded numeric value to float64.
func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	}
	return 0
}
