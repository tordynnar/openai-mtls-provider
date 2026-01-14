# OpenAI Mock Server & Test Suite

A complete mock OpenAI API server written in Go, along with a test client and OpenCode configuration for local development and testing. Supports mTLS (mutual TLS) authentication.

## Project Structure

```
./
├── README.md                 # This file
├── opencode.json             # OpenCode configuration for mock server
├── certs/                    # TLS certificates
│   └── generate.sh           # Script to generate CA, server, and client certs
├── openai-mock-server/       # Mock OpenAI API server (Go)
│   ├── main.go
│   ├── go.mod
│   └── go.sum
├── openai-test-client/       # Test client (Go)
│   ├── main.go
│   ├── go.mod
│   └── go.sum
├── openai-mtls-provider/     # Custom mTLS provider for OpenCode (TypeScript)
│   ├── src/index.ts
│   ├── package.json
│   └── tsconfig.json
└── http-proxy/               # HTTP proxy server with SSE support (Go)
    ├── main.go
    └── go.mod
```

## Quick Start

```bash
# 1. Generate certificates
cd certs && ./generate.sh && cd ..

# 2. Build and start the mock server (with mTLS)
cd openai-mock-server
go build -o openai-mock-server .
./openai-mock-server

# 3. Run tests (in another terminal)
cd openai-test-client
go build -o openai-test-client .
./openai-test-client
```

## mTLS Authentication

The server and client support mutual TLS authentication by default. Both parties verify each other's certificates.

### Generating Certificates

Run the certificate generation script:

```bash
cd certs
./generate.sh
```

This creates:
- `ca.crt` / `ca.key` - Certificate Authority
- `server.crt` / `server.key` - Server certificate (CN=localhost)
- `client.crt` / `client.key` - Client certificate (CN=test-client)

### Server Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-port` | `8000` | Port to listen on |
| `-cert` | `../certs/server.crt` | Server certificate file |
| `-key` | `../certs/server.key` | Server key file |
| `-ca` | `../certs/ca.crt` | CA certificate for client verification |
| `-insecure` | `false` | Run without mTLS (plain HTTP) |

### Client Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-cert` | `../certs/client.crt` | Client certificate file |
| `-key` | `../certs/client.key` | Client key file |
| `-ca` | `../certs/ca.crt` | CA certificate for server verification |
| `-insecure` | `false` | Run without mTLS (plain HTTP) |

### Running Without mTLS

```bash
# Server (insecure mode)
./openai-mock-server -insecure

# Client (insecure mode)
./openai-test-client -insecure
```

## OpenAI Mock Server

A fully-featured mock OpenAI API server that simulates OpenAI's API responses for testing and development without incurring API costs.

### Endpoints

- **GET /v1/models** - List available models
- **GET /v1/models/{id}** - Get model by ID
- **POST /v1/chat/completions** - Chat completions (streaming & non-streaming)
- **POST /v1/embeddings** - Generate embeddings

### Features

| Feature | Description |
|---------|-------------|
| mTLS Authentication | Mutual TLS with client certificate verification |
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

### Example Requests (with mTLS)

**List Models:**
```bash
curl --cacert certs/ca.crt \
     --cert certs/client.crt \
     --key certs/client.key \
     https://localhost:8000/v1/models
```

**Chat Completion:**
```bash
curl --cacert certs/ca.crt \
     --cert certs/client.crt \
     --key certs/client.key \
     https://localhost:8000/v1/chat/completions \
     -H "Content-Type: application/json" \
     -d '{
       "model": "gpt-4o",
       "messages": [{"role": "user", "content": "Hello!"}]
     }'
```

**Chat Completion (Streaming):**
```bash
curl -N --cacert certs/ca.crt \
     --cert certs/client.crt \
     --key certs/client.key \
     https://localhost:8000/v1/chat/completions \
     -H "Content-Type: application/json" \
     -d '{
       "model": "gpt-4o",
       "messages": [{"role": "user", "content": "Hello!"}],
       "stream": true
     }'
```

**Embeddings:**
```bash
curl --cacert certs/ca.crt \
     --cert certs/client.crt \
     --key certs/client.key \
     https://localhost:8000/v1/embeddings \
     -H "Content-Type: application/json" \
     -d '{
       "model": "text-embedding-ada-002",
       "input": "Hello world"
     }'
```

### Example Requests (Insecure Mode)

When running with `-insecure`:

```bash
curl http://localhost:8000/v1/models
```

## Test Client

A comprehensive Go test client using the `sashabaranov/go-openai` library that validates all mock server endpoints.

### Running Tests

```bash
cd openai-test-client
./openai-test-client
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

The `opencode.json` configuration allows you to use the mock server with [OpenCode](https://opencode.ai) using mTLS authentication.

### Custom mTLS Provider

A custom OpenAI-compatible provider (`openai-mtls-provider`) enables OpenCode to use mTLS authentication. The provider uses Bun's native TLS options to present client certificates.

### Setup

1. Build the mTLS provider:
   ```bash
   cd openai-mtls-provider
   npm install
   npm run build
   cd ..
   ```

2. Generate certificates (if not already done):
   ```bash
   cd certs && ./generate.sh && cd ..
   ```

3. Start the mock server with mTLS:
   ```bash
   cd openai-mock-server && ./openai-mock-server
   ```

4. Run OpenCode:
   ```bash
   opencode run "Your message here"
   ```

### Configuration

The `opencode.json` configuration specifies the custom provider with mTLS certificates. It uses OpenCode's `{env:VAR}` syntax for environment variable expansion:

```json
{
  "provider": {
    "mock-openai-mtls": {
      "npm": "file://{env:PWD}/openai-mtls-provider",
      "options": {
        "baseURL": "https://localhost:8000/v1",
        "clientCert": "./certs/client.crt",
        "clientKey": "./certs/client.key",
        "caCert": "./certs/ca.crt"
      },
      "models": { ... }
    }
  },
  "model": "mock-openai-mtls/gpt-4o"
}
```

- `{env:PWD}` expands to the current working directory at runtime
- Certificate paths are relative to the project root

### Available Models in OpenCode

| Model | Context | Output |
|-------|---------|--------|
| mock-openai-mtls/gpt-4o | 128K | 16K |
| mock-openai-mtls/gpt-4o-mini | 128K | 16K |
| mock-openai-mtls/gpt-4-turbo | 128K | 4K |
| mock-openai-mtls/gpt-4 | 8K | 4K |
| mock-openai-mtls/gpt-3.5-turbo | 16K | 4K |

## mTLS Provider

The `openai-mtls-provider` is a custom AI SDK provider that adds mTLS and HTTP proxy support.

### Features

- Compatible with `@ai-sdk/openai-compatible`
- Uses Bun's native TLS options for client certificate authentication
- HTTP proxy support via CONNECT tunneling
- Configurable certificate paths in `opencode.json`
- Falls back to standard fetch when no certificates are provided

### Options

| Option | Type | Description |
|--------|------|-------------|
| `baseURL` | string | Base URL for the OpenAI-compatible API |
| `clientCert` | string | Path to client certificate (PEM format) |
| `clientKey` | string | Path to client private key (PEM format) |
| `caCert` | string | Path to CA certificate (PEM format) |
| `apiKey` | string | Optional API key for authentication |
| `headers` | object | Optional custom headers |
| `proxy` | string | Optional HTTP proxy URL (e.g., `http://localhost:8080`) |

## HTTP Proxy Server

A generic HTTP proxy server with support for HTTPS tunneling and SSE streaming.

### Features

- HTTP and HTTPS proxy support
- CONNECT method for HTTPS tunneling
- SSE/streaming support (unbuffered responses)
- Request logging
- Verbose mode for debugging

### Usage

```bash
cd http-proxy
go build -o http-proxy .
./http-proxy [-port 8080] [-verbose]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-port` | `8080` | Port to listen on |
| `-verbose` | `false` | Enable verbose logging |

### Using with OpenCode

Add the proxy to your `opencode.json`:

```json
{
  "options": {
    "proxy": "http://localhost:8080"
  }
}
```

## Building from Source

```bash
# Generate certificates
cd certs && ./generate.sh && cd ..

# Build mock server
cd openai-mock-server
go build -o openai-mock-server .

# Build test client
cd ../openai-test-client
go build -o openai-test-client .

# Build HTTP proxy
cd ../http-proxy
go build -o http-proxy .

# Build mTLS provider
cd ../openai-mtls-provider
npm install && npm run build
```

## Dependencies

### Mock Server
- Go 1.21+
- `github.com/google/uuid`

### Test Client
- Go 1.21+
- `github.com/sashabaranov/go-openai`

### mTLS Provider
- Node.js 18+ / Bun
- `@ai-sdk/openai-compatible` ^1.0.30

### HTTP Proxy
- Go 1.21+
- No external dependencies

## License

MIT
