# 🚀 Anthropic Adapter (Go)

![Anthropic Adapter Logo](Anthropic-Adapter.png "Anthropic Adapter")

An open-source API adapter that translates between the **Anthropic** and **OpenAI** message formats, enabling seamless interoperability. This is a Go rewrite (Fiber v3) of the original Python/FastAPI project.

## 📋 Overview

The Anthropic Adapter is a lightweight proxy server that bridges the gap between the **Anthropic API** (`v1/messages`) and **OpenAI-compatible APIs** (`v1/chat/completions` and `v1/responses`). It transforms requests and responses between the two formats, letting you use any OpenAI-compatible LLM provider with Anthropic SDKs (including Claude Code).

## 🎯 Features

- ✅ **Format Translation** — Anthropic ⇄ OpenAI message conversion
- ✅ **Dual API Support** — works with both `v1/chat/completions` and `v1/responses`
- ✅ **Auto-Detection** — picks the transformation based on your base URL
- ✅ **Streaming** — real-time SSE streaming with correct Anthropic event framing
- ✅ **Token Counting** — `count_tokens` endpoint backed by tiktoken
- ✅ **Multimodal** — text, images, and tool calls
- ✅ **Configurable** — via environment variables or CLI flags
- ✅ **Error Handling** — upstream errors mapped to Anthropic error format
- ✅ **CORS** — permissive by default

## 📦 Installation

### Prerequisites

- Go 1.26+

### Build

```bash
go build -o anthropic-adapter .
```

## 🚀 Usage

### CLI

```bash
./anthropic-adapter --port 9000 --base-url "http://localhost:1234/v1/chat/completions" --api-key "sk-..."
```

Available flags:

| Flag         | Default                                           | Description                                    |
| ------------ | ------------------------------------------------- | ---------------------------------------------- |
| `--base-url` | `https://api.openai.com/v1/chat/completions`      | Target OpenAI-compatible API URL               |
| `--api-key`  | (from env)                                        | Upstream API key (or pass via `x-api-key`)     |
| `--host`     | `0.0.0.0`                                          | Bind host                                      |
| `--port`     | `8000`                                             | Bind port                                      |

### Environment Variables

Create a `.env` file (see `.env.example`):

```bash
# For v1/chat/completions (default)
OPENAI_BASE_URL=https://api.openai.com/v1/chat/completions
# For v1/responses (newer API)
# OPENAI_BASE_URL=https://api.openai.com/v1/responses

OPENAI_API_KEY=your-api-key-here
HOST=0.0.0.0
PORT=8000
TIKTOKEN_ENCODING=cl100k_base
```

The adapter automatically detects which API format to use based on `OPENAI_BASE_URL`.

### Using with Claude Code

```bash
export ANTHROPIC_BASE_URL="http://localhost:8000"
export ANTHROPIC_AUTH_TOKEN="your-api-key-here"
export ANTHROPIC_API_KEY=""  # Must be explicitly empty
```

Override model aliases to point at your provider's models:

```bash
export ANTHROPIC_DEFAULT_SONNET_MODEL="<MODEL_1>"
export ANTHROPIC_DEFAULT_OPUS_MODEL="<MODEL_2>"
export ANTHROPIC_DEFAULT_HAIKU_MODEL="<MODEL_3>"
```

## 🔧 Architecture

| File                     | Responsibility                                             |
| ------------------------ | --------------------------------------------------------- |
| `main.go`                | CLI flags, Fiber app, routes, server bootstrap            |
| `config.go`              | Configuration + API-type auto-detection                   |
| `types.go`               | Anthropic request types + polymorphic JSON decoding       |
| `transform.go`           | `v1/chat/completions` request/response transforms         |
| `transform_responses.go` | `v1/responses` request/response transforms                |
| `stream.go`              | SSE streaming translation for both APIs                   |
| `token.go`               | tiktoken-based token counting                             |
| `handlers.go`            | HTTP handlers + upstream forwarding                       |

### Data Flow

```
Anthropic → [Transform] → OpenAI → [Forward] → Upstream API → [Response] → [Transform] → Anthropic
```

## 🛠️ API Endpoints

### `POST /v1/messages`

Message creation and streaming.

- **Headers:** `x-api-key` — your upstream API key (or set via env)
- **Body:** standard Anthropic `v1/messages` JSON; set `stream: true` for SSE
- **Response:** Anthropic message format (SSE stream when streaming)

### `POST /v1/messages/count_tokens`

- **Body:** Anthropic-format request
- **Response:** `{ "input_tokens": <n> }`

## 📄 License

MIT License.
