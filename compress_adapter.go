package main

import "anthropic-adapter/compression"

// compressRequest runs the compression pipeline over an AnthropicRequest in-place.
// It replaces the old compressAnthropicRequest, using the modular compression package.
func compressRequest(req *AnthropicRequest, level string) {
	pipeline := compression.NewDefaultPipeline(level)

	if req.System != nil {
		if req.System.IsString {
			req.System.Str = pipeline.CompressText(req.System.Str, level, "system")
		} else {
			for i := range req.System.Blocks {
				if req.System.Blocks[i].Type == "text" {
					req.System.Blocks[i].Text = pipeline.CompressText(req.System.Blocks[i].Text, level, "system")
				}
			}
		}
	}

	for i := range req.Messages {
		role := req.Messages[i].Role
		compressContent(pipeline, req.Messages[i].Content, level, role)
	}
}

func compressContent(pipeline *compression.Pipeline, sc *StringOrBlocks, level, role string) {
	if sc == nil {
		return
	}
	if sc.IsString {
		sc.Str = pipeline.CompressText(sc.Str, level, role)
		return
	}
	for i := range sc.Blocks {
		b := &sc.Blocks[i]
		switch b.Type {
		case "text":
			b.Text = pipeline.CompressText(b.Text, level, role)
		case "tool_result":
			compressToolResultContent(pipeline, b.Content, level)
		}
	}
}

func compressToolResultContent(pipeline *compression.Pipeline, sc *StringOrBlocks, level string) {
	if sc == nil {
		return
	}
	if sc.IsString {
		sc.Str = pipeline.CompressToolResult(sc.Str, level)
		return
	}
	for i := range sc.Blocks {
		if sc.Blocks[i].Type == "text" {
			sc.Blocks[i].Text = pipeline.CompressToolResult(sc.Blocks[i].Text, level)
		}
	}
}
