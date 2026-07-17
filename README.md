# 🚀 AI Router (Go)

![AI Router Logo](Anthropic-Adapter.png "AI Router")

A **multi-provider LLM gateway** written in Go (Fiber v3). It accepts **both
Anthropic- and OpenAI-compatible** inbound requests — with **separate path
prefixes for each format** — and routes them to any number of upstream providers
(OpenAI-compatible, OpenAI Responses, or Anthropic-compatible), translating
message formats on the fly. Originally a Python/FastAPI single-provider adapter;
rewritten in Go as a configurable gateway.

## 🎯 Features

- ✅ **Dual-path API** — OpenAI at `/v1/*`, Anthropic at `/v1/anthropic/*`
- ✅ **Path-appropriate model list** — `/v1/models` returns OpenAI format, `/v1/anthropic/v1/models` returns Anthropic format
- ✅ **Multi-provider** — register many upstreams (`openai`, `openai-responses`, `anthropic`)
- ✅ **Model aggregation** — virtual model names routed by strategy: `failover`, `round_robin`, `weighted`
- ✅ **Automatic failover** — every strategy falls back to remaining targets on error
- ✅ **Format translation** — Anthropic ⇄ OpenAI (chat/completions & responses); Anthropic passthrough
- ✅ **Effort / reasoning forwarding** — `output_config.effort` / `thinking.effort` (Anthropic) ⇄ `reasoning_effort` / `reasoning.effort` (OpenAI); provider `reasoning_tokens` passed back to the client
- ✅ **Accurate streaming usage** — `output_tokens` in `message_delta` reflects real upstream usage (no hardcoded values)
- ✅ **Model metadata** — `context_window` / `max_output` per aggregation, exposed via both model-list endpoints
- ✅ **Client auth** — gateway-level API keys (`Authorization: Bearer` or `x-api-key`)
- ✅ **Rate-limit cooldown** — on `429`, skip the provider for a configurable window (default 10 min); `Retry-After` honored
- ✅ **Token counting** — `/v1/messages/count_tokens` (tiktoken)
- ✅ **Prompt compression** — Caveman + RTK-style compression saves 15–50%+ tokens before upstream dispatch (levels: `off`, `lite`, `standard`, `aggressive`)
- ✅ **MCP tool bridging** — connect MCP servers (Streamable HTTP or SSE); their tools are advertised to the model and optionally auto-executed in a tool-use loop (non-streaming)
- ✅ **YAML config**

## 📦 Build

Requires Go 1.26+.

```bash
go build -o ai-router .
```

## ⚙️ Configuration

Copy the example and edit it:

```bash
cp config.example.yaml config.yaml
```

```yaml
gateway:
  host: 127.0.0.1
  port: 8081
  debug: true
  rate_limit_cooldown: 600   # seconds; global default when a provider omits its own (0 = disabled)
  compression: standard      # off | lite | standard | aggressive (optional, saves tokens)

client_keys:              # callers must present one of these keys
  - key: ak-xxxxxxxx
    name: dev

providers:
  - name: oc1
    enabled: true
    compatible: openai    # openai | openai-responses | anthropic
    base_url: https://opencode.ai/zen/v1
    api_key: sk-...
    rate_limit_cooldown: 0  # optional per-provider override (seconds)

model_aggregations:
  - name: flash           # virtual model name used by clients
    strategy: round_robin # failover | round_robin | weighted
    models:
      - provider: oc1
        model: deepseek-v4-flash-free
        weight: 50

mcp_servers:              # optional; connect MCP tool servers
  - name: search
    type: http            # http (Streamable HTTP) | sse
    url: https://mcp.example.com/mcp
    bearer_token: mcp-secret
    prefix: true          # prefix tool names with `search_`
    auto_execute: true    # auto-run tool calls and re-dispatch
```

**`compatible` → upstream endpoint:**

| Value              | Endpoint                        | Translation                    |
| ------------------ | ------------------------------- | ------------------------------ |
| `openai`           | `{base_url}/chat/completions`   | Anthropic ⇄ OpenAI chat        |
| `openai-responses` | `{base_url}/responses`          | Anthropic ⇄ OpenAI responses   |
| `anthropic`        | `{base_url}/messages`           | passthrough (no translation)   |

All three dialects are reachable as **both inbound and outbound**: the gateway
accepts Anthropic-format requests at `POST /v1/anthropic/v1/messages` and
OpenAI-format requests at `POST /v1/chat/completions`, and translates them
to whichever upstream dialect a target provider uses.

Disabled providers, and targets referencing unknown providers, are skipped when
building the candidate list. If `client_keys` is empty, auth is disabled.

## 🚀 Run

```bash
./ai-router --config config.yaml
```

## 🛠️ API

### OpenAI — `/v1/*`

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/chat/completions` | OpenAI `chat/completions` body. `model` = aggregation name. Supports `stream`, `tool_calls`, `reasoning_effort`. |
| `GET` | `/v1/models` | Lists aggregations in **OpenAI format** (`object: "model"`, `created`, `owned_by`). |

### Anthropic — `/v1/anthropic/*`

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/anthropic/v1/messages` | Anthropic `messages` body. `model` = aggregation name. Supports `stream: true`. |
| `POST` | `/v1/anthropic/v1/messages/count_tokens` | Returns `{ "input_tokens": <n> }`. |
| `GET` | `/v1/anthropic/v1/models` | Lists aggregations in **Anthropic format** (`type: "model"`, `created_at`, `display_name`). |

Both model-list endpoints include `context_window` and `max_output` per model when configured.

## 🤝 Using with Claude Code

```bash
export ANTHROPIC_BASE_URL="http://localhost:8081/v1/anthropic"
export ANTHROPIC_AUTH_TOKEN="ak-xxxxxxxx"     # a gateway client key
export ANTHROPIC_API_KEY=""
export ANTHROPIC_DEFAULT_SONNET_MODEL="flash" # an aggregation name
export ANTHROPIC_DEFAULT_OPUS_MODEL="pro"
export ANTHROPIC_DEFAULT_HAIKU_MODEL="flash"
```

For OpenAI-compatible clients (OpenCode, any OpenAI SDK):

```bash
export OPENAI_BASE_URL="http://localhost:8081/v1"
export OPENAI_API_KEY="ak-xxxxxxxx"
```

## 🧩 MCP tools

Optionally connect one or more MCP (Model Context Protocol) servers under
`mcp_servers`. At startup the gateway connects to each, lists its tools, and
advertises them to the model alongside the caller's own tools (non-streaming
requests only).

| Field           | Description                                                          |
| --------------- | ------------------------------------------------------------------- |
| `name`          | server label (also the prefix namespace when `prefix: true`)         |
| `type`          | `http` (Streamable HTTP) or `sse`                                    |
| `url`           | MCP server endpoint                                                  |
| `bearer_token`  | optional, sent as `Authorization: Bearer <token>`                    |
| `prefix`        | prefix tool names with `<name>_` (recommended for multiple servers)  |
| `auto_execute`  | auto-run tool calls; if `false`, tool_use is returned to the caller  |

When `auto_execute: true` and the model calls one of these tools, the gateway
runs it, appends the result, and re-dispatches — repeating up to 10 rounds
until the model stops calling tools. Auto-execute only fires when **every**
tool_use in a turn belongs to an `auto_execute` server; a mixed turn (MCP +
caller-defined tools) is returned to the caller unchanged.

Tools are discovered once at startup; a server that fails to connect is logged
and skipped, and the gateway continues with the rest.

## 🔧 Architecture

| File                     | Responsibility                                       |
| ------------------------ | ---------------------------------------------------- |
| `main.go`                | flags, Fiber app, routes, bootstrap                  |
| `config.go`              | YAML config, providers, aggregations, auth           |
| `router.go`              | aggregation resolution + routing strategies          |
| `handlers.go`            | auth, dispatch, failover, rate-limit cooldown, `/v1/models`, MCP tool-use loop |
| `openai_inbound.go`      | OpenAI `chat/completions` parsing + response mapping |
| `types.go`               | Anthropic request types + polymorphic decoding       |
| `transform.go`           | `chat/completions` request/response transforms       |
| `transform_responses.go` | `responses` request/response transforms              |
| `stream.go`              | SSE streaming translation                            |
| `compress.go`            | prompt compression (Caveman + RTK-style, EN + ID)    |
| `mcp_client.go`          | MCP server connections, tool discovery, auto-execution |
| `token.go`               | tiktoken token counting                              |

## 📄 License

MIT License.
