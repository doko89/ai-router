package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// MCPServerConfig configures one MCP server connection for tool discovery and execution.
type MCPServerConfig struct {
	Name        string `yaml:"name"`
	Type        string `yaml:"type"`         // "sse" or "http" (Streamable HTTP)
	URL         string `yaml:"url"`
	BearerToken string `yaml:"bearer_token"`
	Prefix      bool   `yaml:"prefix"`        // prefix tool names with server name
	AutoExecute bool   `yaml:"auto_execute"` // execute MCP tools automatically when called by AI
}

type mcpConn struct {
	cfg     MCPServerConfig
	cli     client.MCPClient
	tools   []mcp.Tool
	toolMap map[string]int // prefixed tool name -> index in tools
}

// MCPServerManager manages connections to multiple MCP servers.
type MCPServerManager struct {
	conns    []*mcpConn
	toolConn map[string]*mcpConn // prefixed tool name -> connection
}

// NewMCPServerManager connects to configured MCP servers and discovers their tools.
// Connections that fail are logged and skipped — the manager still works with
// whatever servers succeeded.
func NewMCPServerManager(cfgs []MCPServerConfig) *MCPServerManager {
	m := &MCPServerManager{
		toolConn: make(map[string]*mcpConn),
	}
	if len(cfgs) == 0 {
		return m
	}

	for _, cfg := range cfgs {
		conn := &mcpConn{
			cfg:     cfg,
			toolMap: make(map[string]int),
		}

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)

		var err error
		conn.cli, err = dialMCP(ctx, cfg)
		if err != nil {
			cancel()
			log.Printf("[mcp] %s: dial error: %v", cfg.Name, err)
			continue
		}

		// Initialize MCP handshake
		initResult, err := conn.cli.Initialize(ctx, mcp.InitializeRequest{
			Params: mcp.InitializeParams{
				ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
				Capabilities:    mcp.ClientCapabilities{},
				ClientInfo: mcp.Implementation{
					Name:    "ai-router",
					Version: "0.3.3",
				},
			},
		})
		if err != nil {
			cancel()
			log.Printf("[mcp] %s: initialize error: %v", cfg.Name, err)
			conn.cli.Close()
			continue
		}
		_ = initResult
		cancel()

		// Discover tools
		listCtx, listCancel := context.WithTimeout(context.Background(), 10*time.Second)
		listResult, err := conn.cli.ListTools(listCtx, mcp.ListToolsRequest{})
		listCancel()
		if err != nil {
			log.Printf("[mcp] %s: list tools error: %v", cfg.Name, err)
			conn.cli.Close()
			continue
		}

		// Index tools by name (with optional prefix)
		conn.tools = listResult.Tools
		for i, tool := range listResult.Tools {
			name := tool.Name
			if cfg.Prefix {
				name = cfg.Name + "_" + tool.Name
			}
			conn.toolMap[name] = i
			m.toolConn[name] = conn
		}

		m.conns = append(m.conns, conn)
		log.Printf("[mcp] %s: connected, %d tools (prefix=%v, auto_execute=%v)",
			cfg.Name, len(listResult.Tools), cfg.Prefix, cfg.AutoExecute)
	}

	return m
}

func dialMCP(ctx context.Context, cfg MCPServerConfig) (client.MCPClient, error) {
	headers := map[string]string{
		"Content-Type": "application/json",
	}
	if cfg.BearerToken != "" {
		headers["Authorization"] = "Bearer " + cfg.BearerToken
	}

	switch cfg.Type {
	case "sse":
		opts := []transport.ClientOption{}
		if cfg.BearerToken != "" {
			opts = append(opts, transport.WithHeaders(headers))
		}
		return client.NewSSEMCPClient(cfg.URL, opts...)

	default: // "http" / "streamable-http"
		opts := []transport.StreamableHTTPCOption{}
		if cfg.BearerToken != "" {
			opts = append(opts, transport.WithHTTPHeaders(headers))
		}
		return client.NewStreamableHttpClient(cfg.URL, opts...)
	}
}

// Close closes all MCP client connections.
func (m *MCPServerManager) Close() {
	for _, conn := range m.conns {
		if err := conn.cli.Close(); err != nil {
			log.Printf("[mcp] %s: close error: %v", conn.cfg.Name, err)
		}
	}
}

// HasTools returns true if at least one MCP server connected successfully with tools.
func (m *MCPServerManager) HasTools() bool {
	return len(m.toolConn) > 0
}

// HasTool checks whether a tool name is managed by an MCP server.
func (m *MCPServerManager) HasTool(name string) bool {
	_, ok := m.toolConn[name]
	return ok
}

// ListAnthropicTools returns all discovered MCP tools in Anthropic format.
// Tool names are prefixed according to each server's config.
func (m *MCPServerManager) ListAnthropicTools() []AnthropicTool {
	var result []AnthropicTool
	seen := make(map[string]bool)

	for _, conn := range m.conns {
		for _, tool := range conn.tools {
			name := tool.Name
			if conn.cfg.Prefix {
				name = conn.cfg.Name + "_" + tool.Name
			}
			if seen[name] {
				continue
			}
			seen[name] = true

			schema, err := json.Marshal(tool.InputSchema)
			if err != nil {
				continue
			}
			result = append(result, AnthropicTool{
				Name:        name,
				Description: tool.Description,
				InputSchema: schema,
			})
		}
	}
	return result
}

// CallTool calls a specific MCP tool and returns the result.
func (m *MCPServerManager) CallTool(ctx context.Context, toolName string, args map[string]any) (*mcp.CallToolResult, error) {
	conn, ok := m.toolConn[toolName]
	if !ok {
		return nil, fmt.Errorf("mcp: tool %q not found", toolName)
	}

	// Resolve original tool name (strip prefix if used)
	callName := toolName
	if conn.cfg.Prefix {
		callName = strings.TrimPrefix(toolName, conn.cfg.Name+"_")
	}

	callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	return conn.cli.CallTool(callCtx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      callName,
			Arguments: args,
		},
	})
}

// ToolAutoExecute reports whether the server owning toolName has auto_execute enabled.
// Returns false for unknown tools.
func (m *MCPServerManager) ToolAutoExecute(toolName string) bool {
	conn, ok := m.toolConn[toolName]
	return ok && conn.cfg.AutoExecute
}

// toolUseBlock represents a tool_use found in an upstream response.
type toolUseBlock struct {
	ID    string
	Name  string
	Input map[string]any
}

// --- Response parsing helpers (Anthropic format) ---

// parseContentBlocks extracts ContentBlock slices from an Anthropic-style content array.
func parseContentBlocks(raw []any) []ContentBlock {
	blocks := make([]ContentBlock, 0, len(raw))
	for _, r := range raw {
		m, ok := r.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := m["type"].(string)
		block := ContentBlock{Type: typ}
		switch typ {
		case "text":
			block.Text, _ = m["text"].(string)
		case "tool_use":
			block.ID, _ = m["id"].(string)
			block.Name, _ = m["name"].(string)
			if input, ok := m["input"]; ok {
				if b, err := json.Marshal(input); err == nil {
					block.Input = b
				}
			}
		}
		blocks = append(blocks, block)
	}
	return blocks
}

// findToolUseFromResponses parses a v1/responses response and returns its text
// and function_call items as Anthropic-shaped content blocks and tool_use entries.
func findToolUseFromResponses(data []byte) ([]ContentBlock, []toolUseBlock) {
	var resp V1ResponsesResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, nil
	}

	var blocks []ContentBlock
	var uses []toolUseBlock
	for _, item := range resp.Output {
		switch item.Type {
		case "message":
			for _, content := range item.Content {
				if content.Type == "output_text" && content.Text != "" {
					blocks = append(blocks, ContentBlock{Type: "text", Text: content.Text})
				}
			}
		case "function_call":
			id := item.CallID
			if id == "" {
				id = item.ID
			}
			var input map[string]any
			if item.Args != "" {
				json.Unmarshal([]byte(item.Args), &input)
			}
			raw, _ := json.Marshal(input)
			blocks = append(blocks, ContentBlock{
				Type:  "tool_use",
				ID:    id,
				Name:  item.Name,
				Input: raw,
			})
			uses = append(uses, toolUseBlock{ID: id, Name: item.Name, Input: input})
		}
	}
	return blocks, uses
}
func findToolUseFromAnthropic(data []byte) ([]ContentBlock, []toolUseBlock) {
	var resp map[string]any
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, nil
	}

	contentRaw, _ := resp["content"].([]any)
	if len(contentRaw) == 0 {
		return nil, nil
	}

	blocks := parseContentBlocks(contentRaw)
	var uses []toolUseBlock
	for _, b := range blocks {
		if b.Type != "tool_use" {
			continue
		}
		var input map[string]any
		if len(b.Input) > 0 {
			json.Unmarshal(b.Input, &input)
		}
		uses = append(uses, toolUseBlock{
			ID:    b.ID,
			Name:  b.Name,
			Input: input,
		})
	}
	return blocks, uses
}

// findToolUseFromOpenAI parses an OpenAI Chat Completions response and returns
// all tool_calls from the first choice, plus the assistant's text. The returned
// blocks are normalized to Anthropic content-block shape so handleMCPStep can
// append them to the message history uniformly.
func findToolUseFromOpenAI(data []byte) ([]ContentBlock, []toolUseBlock) {
	var resp struct {
		Choices []struct {
			Message struct {
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
	}
	if err := json.Unmarshal(data, &resp); err != nil || len(resp.Choices) == 0 {
		return nil, nil
	}

	msg := resp.Choices[0].Message
	var blocks []ContentBlock
	if msg.Content != "" {
		blocks = append(blocks, ContentBlock{Type: "text", Text: msg.Content})
	}

	var uses []toolUseBlock
	for _, tc := range msg.ToolCalls {
		var input map[string]any
		if tc.Function.Arguments != "" {
			json.Unmarshal([]byte(tc.Function.Arguments), &input)
		}
		raw, _ := json.Marshal(input)
		blocks = append(blocks, ContentBlock{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: raw,
		})
		uses = append(uses, toolUseBlock{
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: input,
		})
	}
	return blocks, uses
}
