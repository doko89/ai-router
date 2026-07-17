# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repo.

## Commands

- **Build:** `go build -o ai-router .`
- **Test all:** `go test ./...`
- **Lint:** `go vet ./...`
- **Run with config:** `go run . -config config.yaml`
- **Docker:** `docker build -t ai-router .`

## Project Structure

Single `package main` repo (no `internal/` packages) — all source files at the root.

### File Map

| File | Responsibility |
|------|----------------|
| `main.go` | Flag parsing, Fiber app bootstrap, route registration |
| `config.go` | YAML config loading/validation, provider + aggregation indexes, auth check |
| `handlers.go` | `App` struct (clients, cooldowns), all route handlers, dispatch with failover, streaming helpers |
| `router.go` | Aggregation resolution + routing strategies (failover, round_robin, weighted) |
| `types.go` | `AnthropicRequest` types, `StringOrBlocks` polymorphic decoder, response builders |
| `transform.go` | Anthropic ↔ OpenAI `chat/completions` request/response transforms |
| `transform_responses.go` | Anthropic ↔ OpenAI `responses` request/response transforms |
| `openai_inbound.go` | OpenAI `chat/completions` inbound parsing, Anthropic→OpenAI streaming, OpenAI error format |
| `stream.go` | SSE event translation: OpenAI chat↔Anthropic, OpenAI responses↔Anthropic |
| `compress.go` | Prompt compression: Caveman + RTK-style rules (EN + ID) at 4 levels |
| `token.go` | tiktoken-based token counting for `/v1/messages/count_tokens` |
| `effort_test.go` | Tests for reasoning effort forwarding, rate-limit cooldown failover, reasoning_tokens passthrough |

### Architecture

All requests pass through a single dispatch pipeline:

1. **Auth** — extract key from `Authorization: Bearer` or `x-api-key`, match against `client_keys` (disabled if empty list)
2. **Resolve** — look up `model` in `model_aggregations` → ordered `[]Target` via strategy (`failover`, `round_robin`, `weighted`)
3. **Compress** (optional) — if `gateway.compression` is set, prompt text is compressed before dispatch to save tokens
4. **Translate** — transform request body per target's `compatible` type (`openai`, `openai-responses`, `anthropic`)
5. **Fallback** — iterate targets in order; on 429 (marks cooldown), network error, or non-200, try next target
6. **Respond** — translate response back to inbound format; for streaming, translate SSE events on-the-fly

### Route Registration (`main.go:buildApp`)

- `POST /v1/chat/completions` → OpenAI inbound handler
- `GET /v1/models` → OpenAI-format model list
- `POST /v1/anthropic/v1/messages` → Anthropic inbound handler
- `POST /v1/anthropic/v1/messages/count_tokens` → token counting
- `GET /v1/anthropic/v1/models` → Anthropic-format model list

### Key Types (`config.go`)

- `Config` — top-level: `Gateway`, `ClientKeys`, `Providers`, `ModelAggregations`
- `Provider` — `Name`, `Enabled`, `Compatible` (`openai` | `openai-responses` | `anthropic`), `BaseURL`, `APIKey`, optional `RateLimitCooldown`, `ContextWindow`, `MaxOutput`
- `ModelAggregation` — virtual model with `Strategy` (`failover` | `round_robin` | `weighted`), list of `AggModel` refs to providers
- `Target` (`router.go`) — resolved `Provider` + `Model` pair ready for dispatch
- `GatewayConfig` — `Host`, `Port`, `Debug`, `RateLimitCooldown`, `Compression`, `TiktokenEncoding`

### Key Patterns

- **`StringOrBlocks`** (`types.go`) — polymorphic type that unmarshals either a JSON string or `[]ContentBlock`. Used for `system` and message `content` fields. Methods: `PlainText(sep)` flattens to string.
- **`effort()`** (`types.go` on `AnthropicRequest`) — reads reasoning effort from `output_config.effort` (new) or `thinking.effort` (legacy).
- **`dispatch()`** (`handlers.go`) — builds the upstream HTTP request, adds auth headers (`x-api-key` for Anthropic, `Bearer` for OpenAI), sends, reads response.
- **`streamResponse()` / `streamResponseOpenAI()`** (`handlers.go`) — Fiber `c.SendStreamWriter()` + `bufio.Writer` for SSE. Two SSE translators in `stream.go` and one in `openai_inbound.go`.
- **Rate-limit cooldown** — `sync.Map` keyed by provider name with expiry timestamps. Per-provider or global default in seconds. 0 disables. Retry-After header is honored.
- **Error formats** — `anthropicError(type, msg)` returns Anthropic-shaped error JSON; `openAIError(msg)` returns OpenAI-shaped. Both in `handlers.go`.
- **Compression** (`compress.go`) — levels: `off`, `lite`, `standard`, `aggressive`. Applies regex-based Caveman + RTK-style rules to text content blocks before upstream dispatch. Spelling corrections for `agressive`/`agresive` → `aggressive`, `standart`/`standar` → `standard`.
- **Effective metadata** (`config.go:aggregationMetadata`) — resolves `context_window` / `max_output` per aggregation: aggregation-level override → min of model candidates → 0. Exposed via both model-list endpoints.

### Testing Patterns (`effort_test.go`)

- `newTestGateway(t, compatible, captured)` — builds a full Fiber app with a mock upstream `httptest.Server`. Pass the `Compatible` type to test each dialect. Tests validate request bodies via captured slices.
- `doReq(t, app, path, auth, body)` — convenience helper for making requests and discarding the response body.
- Tests use `httptest.NewRequest` + `app.Test()` — no real HTTP server needed.
- Mock upstream responses are hand-crafted JSON matching the expected response format for the dialect being tested.

### Dispatch Logic (`handlers.go:handleMessages`)

The core failover loop pattern:
1. Resolve candidates from aggregation name
2. For each candidate, skip if provider is on cooldown
3. Compress request body if enabled
4. Transform request body for the target's dialect
5. Call `dispatch()` which sends the HTTP request
6. On success → translate response and return
7. On 429 → mark cooldown, continue to next candidate
8. On error (network/non-200) → continue to next candidate
9. If all exhausted → return 429 with "overloaded" error + Retry-After hint

### Adding a New Upstream Dialect

1. Add a `Compatible*` constant in `config.go`
2. Add the endpoint path in `handlers.go dispatch()` or wire a new one
3. Add request translation (Anthropic → new format) in a new or existing transform file
4. Add response translation (new format → Anthropic) alongside it
5. Wire into `dispatch()` (request), `jsonResponse()`/`streamResponse()` (response, Anthropic inbound), `jsonResponseOpenAI()`/`streamResponseOpenAI()` (response, OpenAI inbound)
6. Add a compatible case in `newTestGateway` for test coverage
