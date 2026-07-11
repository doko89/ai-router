package main

import (
	"github.com/pkoukk/tiktoken-go"
)

// countOpenAITokens estimates the token count of an OpenAI-formatted request
// body, mirroring the reference tiktoken heuristic.
func countOpenAITokens(body map[string]any, encodingName string) int {
	enc, err := tiktoken.GetEncoding(encodingName)
	if err != nil {
		enc, err = tiktoken.GetEncoding("cl100k_base")
		if err != nil {
			return 0
		}
	}

	numTokens := 0
	if messages, ok := body["messages"].([]map[string]any); ok {
		for _, message := range messages {
			numTokens += 3
			if content, ok := message["content"].(string); ok {
				numTokens += len(enc.Encode(content, nil, nil))
			}
			if toolCalls, ok := message["tool_calls"].([]map[string]any); ok {
				for _, tc := range toolCalls {
					if fn, ok := tc["function"].(map[string]any); ok {
						if name, ok := fn["name"].(string); ok {
							numTokens += len(enc.Encode(name, nil, nil))
						}
						if args, ok := fn["arguments"].(string); ok {
							numTokens += len(enc.Encode(args, nil, nil))
						}
					}
				}
			}
		}
	}

	numTokens += 3
	if tools, ok := body["tools"]; ok {
		numTokens += len(enc.Encode(jsonString(tools), nil, nil))
	}

	return numTokens
}
