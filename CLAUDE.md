# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test

```bash
# Build the binary
go build -o ai-router .

# Run all tests
go test ./...

# Run a single test
go test -run TestEffortForwardingOpenAI ./...
go test -run TestRateLimitCooldownFailover ./...

# Run with a config file (requires config.yaml)
./ai-router --config config.yaml
```

Tests use `net/http/httptest` with mock upstream servers — no external dependencies needed.

## Architecture

**AI Router** is a multi-provider LLM gateway written in Go using Fiber v3. It accepts Anthropic-format (`/v1/messages`) and OpenAI-format (`/v1/chat/completions`) requests and routes them to upstream providers, translating formats on-the-fly.

### File Map

| File | Role |
|------|------|
| `main.go` | Flag parsing, Fiber app bootstrap, route registration (`/v1/messages`, `/v1/chat/completions`, `/v1/messages/count_tokens`, `/v1/models`) |
| `config.go` | YAML config loading/validation, provider + aggregation indexes, auth check |
| `handlers.go` | `App` struct (HTTP clients, cooldowns), all route handlers, dispatch logic with failover, streaming helpers |
| `router.go` | Model aggregation resolution: `failover`, `round_robin`, `weighted` strategies |
| `types.go` | `AnthropicRequest`/`AnthropicMessage`/`ContentBlock` types + `StringOrBlocks` polymorphic decoder |
| `transform.go` | Anthropic → OpenAI `chat/completions` request transformation + OpenAI → Anthropic response transformation |
| `transform_responses.go` | Anthropic → OpenAI `responses` request transformation + responses → Anthropic response transformation |
| `openai_inbound.go` | OpenAI `chat/completions` parsing into canonical `AnthropicRequest`, Anthropic → OpenAI streaming, OpenAI error format |
| `stream.go` | SSE event translation: OpenAI chat/completions → Anthropic events, OpenAI responses → Anthropic events, SSE helpers |
| `token.go` | tiktoken-based token counting for `/v1/messages/count_tokens` |
| `effort_test.go` | Tests for reasoning effort forwarding, rate-limit cooldown failover, reasoning_tokens passthrough |

### Data Flow

All requests pass through a single dispatch pipeline:

1. **Auth** — extract key from `Authorization: Bearer` or `x-api-key`, match against `client_keys` (disabled if empty)
2. **Resolve** — look up `model` in `model_aggregations` → ordered `[]Target` via strategy (`failover`, `round_robin`, `weighted`)
3. **Dispatch** — iterate targets in order, skipping providers on rate-limit cooldown
4. **Translate** — transform request body per target's `compatible` type (`openai`, `openai-responses`, `anthropic`)
5. **Fallback** — on 429 (marks cooldown), network error, or non-200, try next target
6. **Respond** — translate response back to the inbound format; for streaming, translate SSE events on-the-fly

### Key Patterns

- **`StringOrBlocks`** — polymorphic type that unmarshals either a JSON string or an array of content blocks (used for `system`, message `content` fields). Lives in `types.go`.
- **`effort()`** — reads reasoning effort from either `output_config.effort` (new) or `thinking.effort` (legacy). Defined on `AnthropicRequest` in `types.go`.
- **`dispatch()`** — builds the upstream HTTP request per provider dialect, handles auth headers (`x-api-key` for Anthropic, `Bearer` for OpenAI). Lives in `handlers.go`.
- **Rate-limit cooldown** — `sync.Map` keyed by provider name with expiry timestamps; per-provider or global default in seconds. 0 disables.
- **Streaming** — uses Fiber's `c.SendStreamWriter()` + `bufio.Writer` for SSE. Two SSE translators live in `stream.go` (OpenAI chat → Anthropic, OpenAI responses → Anthropic), and two in `openai_inbound.go` (Anthropic → OpenAI chat, OpenAI responses → OpenAI chat).
- **Test infrastructure** — `newTestGateway()` builds a full Fiber app with a mock upstream `httptest.Server`. Tests validate request bodies via captured slices.

### Adding a New Upstream Dialect

1. Add a `Compatible*` constant in `config.go`
2. Add the endpoint path in `Provider.endpoint()`
3. Add request translation (Anthropic → new format) in a new or existing transform file
4. Add response translation (new format → Anthropic) alongside it
5. Wire into `dispatch()` (request), `jsonResponse()`/`streamResponse()` (response, Anthropic inbound), `jsonResponseOpenAI()`/`streamResponseOpenAI()` (response, OpenAI inbound)
