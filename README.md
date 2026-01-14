# OpenAI Mock Server & Test Suite

A complete mock OpenAI API server written in Go, along with a test client and OpenCode configuration for local development and testing.

## Project Structure

```
./
├── README.md                 # This file
├── opencode.json             # OpenCode configuration for mock server
├── openai-mock-server/       # Mock OpenAI API server (Go)
│   ├── main.go
│   ├── go.mod
│   ├── go.sum
│   └── openai-mock-server    # Compiled binary
└── openai-test-client/       # Test client (Go)
    ├── main.go
    ├── go.mod
    └── go.sum
```

## OpenAI Mock Server

A fully-featured mock OpenAI API server that simulates OpenAI's API responses for testing and development without incurring API costs.

### Features

- **GET /v1/models** - List available models
- **GET /v1/models/{id}** - Get model by ID
- **POST /v1/chat/completions** - Chat completions (streaming & non-streaming)
- **POST /v1/embeddings** - Generate embeddings

### Additional Capabilities

| Feature | Description |
|---------|-------------|
| SSE Streaming | Real-time word-by-word streaming via Server-Sent Events |
| Tool/Function Calling | Supports `tools` parameter with mock tool call responses |
| CORS | Full CORS support for browser-based clients |
| Error Responses | OpenAI-compatible error format with `type`, `param`, `code` |
| Multiple Models | GPT-4, GPT-4o, GPT-3.5-turbo, embedding models |

### Supported Models

| Model ID | Type |
|----------|------|
| gpt-4 | Chat |
| gpt-4-turbo | Chat |
| gpt-4-turbo-preview | Chat |
| gpt-4o | Chat |
| gpt-4o-mini | Chat |
| gpt-3.5-turbo | Chat |
| gpt-3.5-turbo-16k | Chat |
| text-embedding-ada-002 | Embedding (1536 dims) |
| text-embedding-3-small | Embedding (1536 dims) |
| text-embedding-3-large | Embedding (3072 dims) |

### Running the Server

```bash
cd openai-mock-server
go build -o openai-mock-server .
./openai-mock-server
```

The server starts on `http://localhost:8000`.

### Example Requests

**List Models:**
```bash
curl http://localhost:8000/v1/models
```

**Chat Completion:**
```bash
curl http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

**Chat Completion (Streaming):**
```bash
curl -N http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": true
  }'
```

**Embeddings:**
```bash
curl http://localhost:8000/v1/embeddings \
  -H "Content-Type: application/json" \
  -d '{
    "model": "text-embedding-ada-002",
    "input": "Hello world"
  }'
```

**With Tools/Functions:**
```bash
curl http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "What is the weather?"}],
    "tools": [{
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "Get weather for a location"
      }
    }],
    "tool_choice": "required"
  }'
```

## Test Client

A comprehensive Go test client using the official `sashabaranov/go-openai` library that validates all mock server endpoints.

### Running Tests

```bash
cd openai-test-client
go run main.go
```

### Test Coverage (26 Tests)

| Category | Tests | Description |
|----------|-------|-------------|
| List Models | 2 | Retrieves models, validates expected models present |
| Get Model | 2 | Fetch by ID, 404 handling for missing models |
| Chat Completion | 5 | Response structure, ID, model, usage, finish_reason |
| Chat with Params | 1 | Temperature, max_tokens, N choices |
| SSE Streaming | 4 | Stream init, chunk count, content assembly, finish |
| Tool Calling | 3 | Tool calls, arguments, finish_reason |
| Embeddings | 5 | Dimensions, index, model, usage |
| Multi Embeddings | 2 | Batch processing, index ordering |
| Error Handling | 2 | Missing model, empty messages |

### Sample Output

```
============================================================
       OpenAI Mock Server Test Suite
============================================================

=== List Models ===
[PASS] ListModels: Retrieved 10 models
[PASS] ListModels-Expected: All expected models present

=== Chat Completion (SSE Streaming) ===
[PASS] ChatCompletion-Stream-Init: Stream created successfully
[PASS] ChatCompletion-Stream-Chunks: Received 14 chunks in 614ms
[PASS] ChatCompletion-Stream-Content: Full response: "Greetings!..."
[PASS] ChatCompletion-Stream-Finish: Received finish_reason: stop

...

Total Tests: 26
Passed: 26
Failed: 0

All tests passed!
```

## OpenCode Integration

The `opencode.json` configuration allows you to use the mock server with [OpenCode](https://opencode.ai).

### Configuration

The config defines a custom provider `mock-openai` pointing to the local server:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "mock-openai": {
      "npm": "@ai-sdk/openai-compatible",
      "name": "Mock OpenAI Server",
      "options": {
        "baseURL": "http://localhost:8000/v1"
      },
      "models": {
        "gpt-4o": { "name": "GPT-4o (Mock)", ... },
        "gpt-4o-mini": { "name": "GPT-4o Mini (Mock)", ... },
        ...
      }
    }
  },
  "model": "mock-openai/gpt-4o"
}
```

### Usage with OpenCode

1. Start the mock server:
   ```bash
   cd openai-mock-server && ./openai-mock-server
   ```

2. Run OpenCode from the `ai` directory (where `opencode.json` is located):
   ```bash
   cd .
   opencode run "Your message here"
   ```

   Or start the TUI:
   ```bash
   opencode
   ```

### Available Models in OpenCode

| Model | Context | Output |
|-------|---------|--------|
| mock-openai/gpt-4o | 128K | 16K |
| mock-openai/gpt-4o-mini | 128K | 16K |
| mock-openai/gpt-4-turbo | 128K | 4K |
| mock-openai/gpt-4 | 8K | 4K |
| mock-openai/gpt-3.5-turbo | 16K | 4K |

## Quick Start

```bash
# Terminal 1: Start the mock server
cd ./openai-mock-server
./openai-mock-server

# Terminal 2: Run tests
cd ./openai-test-client
go run main.go

# Terminal 3: Use with OpenCode
cd .
opencode run "Hello, world!"
```

## Dependencies

### Mock Server
- Go 1.21+
- `github.com/google/uuid`

### Test Client
- Go 1.21+
- `github.com/sashabaranov/go-openai`

## Building from Source

```bash
# Build mock server
cd openai-mock-server
go build -o openai-mock-server .

# Build test client (optional, can run with go run)
cd openai-test-client
go build -o openai-test-client .
```

## License

MIT
