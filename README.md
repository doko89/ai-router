# 🚀 AI Router (Go)

![AI Router Logo](Anthropic-Adapter.png "AI Router")

A **multi-provider LLM gateway** written in Go (Fiber v3). It accepts **both
Anthropic- and OpenAI-compatible** inbound requests (`/v1/messages` and
`/v1/chat/completions`) and routes them to any number of upstream providers
(OpenAI-compatible, OpenAI Responses, or Anthropic-compatible), translating
message formats on the fly. Originally a Python/FastAPI single-provider adapter;
rewritten in Go as a configurable gateway.

## 🎯 Features

- ✅ **Dual inbound API** — Anthropic (`/v1/messages`) **and** OpenAI (`/v1/chat/completions`)
- ✅ **Multi-provider** — register many upstreams (`openai`, `openai-responses`, `anthropic`)
- ✅ **Model aggregation** — virtual model names routed by strategy: `failover`, `round_robin`, `weighted`
- ✅ **Automatic failover** — every strategy falls back to remaining targets on error
- ✅ **Format translation** — Anthropic ⇄ OpenAI (chat/completions & responses); Anthropic passthrough
- ✅ **Effort / reasoning forwarding** — `output_config.effort` / `thinking.effort` (Anthropic) ⇄ `reasoning_effort` / `reasoning.effort` (OpenAI); provider `reasoning_tokens` passed back to the client
- ✅ **Streaming** — SSE with correct Anthropic event framing
- ✅ **Client auth** — gateway-level API keys (`Authorization: Bearer` or `x-api-key`)
- ✅ **Rate-limit cooldown** — on `429`, skip the provider for a configurable window (default 10 min); `Retry-After` honored
- ✅ **`/v1/models`** — lists your model aggregations
- ✅ **Token counting** — `/v1/messages/count_tokens` (tiktoken)
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
```

**`compatible` → upstream endpoint:**

| Value              | Endpoint                        | Translation                    |
| ------------------ | ------------------------------- | ------------------------------ |
| `openai`           | `{base_url}/chat/completions`   | Anthropic ⇄ OpenAI chat        |
| `openai-responses` | `{base_url}/responses`          | Anthropic ⇄ OpenAI responses   |
| `anthropic`        | `{base_url}/messages`           | passthrough (no translation)   |

All three dialects are reachable as **both inbound and outbound**: the gateway
accepts Anthropic (`/v1/messages`) and OpenAI (`/v1/chat/completions`) requests
and translates them to whichever upstream dialect a target provider uses.

Disabled providers, and targets referencing unknown providers, are skipped when
building the candidate list. If `client_keys` is empty, auth is disabled.

## 🚀 Run

```bash
./ai-router --config config.yaml
```

## 🛠️ API

### `POST /v1/messages`
Anthropic `v1/messages` body. Set `model` to an **aggregation name**. Supports
`stream: true`. Auth required.

### `POST /v1/chat/completions`
OpenAI `chat/completions` body, also routed via an **aggregation name** in
`model`. Supports `stream: true`, `tool_calls`, and `reasoning_effort`. Auth
required.

### `POST /v1/messages/count_tokens`
Returns `{ "input_tokens": <n> }`.

### `GET /v1/models`
Lists model aggregations in OpenAI models format (`object: "model"`,
`owned_by: "ai-router"`).

## 🤝 Using with Claude Code

```bash
export ANTHROPIC_BASE_URL="http://localhost:8081"
export ANTHROPIC_AUTH_TOKEN="ak-xxxxxxxx"     # a gateway client key
export ANTHROPIC_API_KEY=""
export ANTHROPIC_DEFAULT_SONNET_MODEL="flash" # an aggregation name
export ANTHROPIC_DEFAULT_OPUS_MODEL="pro"
export ANTHROPIC_DEFAULT_HAIKU_MODEL="flash"
```

## 🔧 Architecture

| File                     | Responsibility                                       |
| ------------------------ | ---------------------------------------------------- |
| `main.go`                | flags, Fiber app, routes, bootstrap                  |
| `config.go`              | YAML config, providers, aggregations, auth           |
| `router.go`              | aggregation resolution + routing strategies          |
| `handlers.go`            | auth, dispatch, failover, rate-limit cooldown, `/v1/models` |
| `openai_inbound.go`      | OpenAI `chat/completions` parsing + response mapping |
| `types.go`               | Anthropic request types + polymorphic decoding       |
| `transform.go`           | `chat/completions` request/response transforms       |
| `transform_responses.go` | `responses` request/response transforms              |
| `stream.go`              | SSE streaming translation                            |
| `token.go`               | tiktoken token counting                              |

## 📄 License

MIT License.
