package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// Types
// ============================================================================

// Models
type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type ModelsResponse struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

// Chat Completions

// ContentPart represents a part of a multi-part content message
type ContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL *struct {
		URL    string `json:"url"`
		Detail string `json:"detail,omitempty"`
	} `json:"image_url,omitempty"`
}

// MessageContent can be either a string or an array of ContentParts
type MessageContent struct {
	Text  string
	Parts []ContentPart
}

func (mc *MessageContent) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as a string first
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		mc.Text = text
		mc.Parts = nil
		return nil
	}

	// Try to unmarshal as an array of ContentParts
	var parts []ContentPart
	if err := json.Unmarshal(data, &parts); err == nil {
		mc.Parts = parts
		mc.Text = ""
		return nil
	}

	// If neither works, return an error
	return fmt.Errorf("content must be a string or array of content parts")
}

func (mc MessageContent) MarshalJSON() ([]byte, error) {
	if len(mc.Parts) > 0 {
		return json.Marshal(mc.Parts)
	}
	return json.Marshal(mc.Text)
}

// GetText returns the text content, extracting from parts if necessary
func (mc *MessageContent) GetText() string {
	if mc.Text != "" {
		return mc.Text
	}
	// Extract text from parts
	var texts []string
	for _, part := range mc.Parts {
		if part.Type == "text" && part.Text != "" {
			texts = append(texts, part.Text)
		}
	}
	return strings.Join(texts, " ")
}

type ChatMessage struct {
	Role       string         `json:"role"`
	Content    MessageContent `json:"content,omitempty"`
	ToolCalls  []ToolCall     `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Name       string         `json:"name,omitempty"`
}

// ResponseMessage is used for responses (always string content)
type ResponseMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type Tool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string                 `json:"name"`
		Description string                 `json:"description,omitempty"`
		Parameters  map[string]interface{} `json:"parameters,omitempty"`
	} `json:"function"`
}

type ChatCompletionRequest struct {
	Model            string        `json:"model"`
	Messages         []ChatMessage `json:"messages"`
	MaxTokens        *int          `json:"max_tokens,omitempty"`
	Temperature      *float64      `json:"temperature,omitempty"`
	TopP             *float64      `json:"top_p,omitempty"`
	N                *int          `json:"n,omitempty"`
	Stream           bool          `json:"stream,omitempty"`
	Stop             interface{}   `json:"stop,omitempty"`
	PresencePenalty  *float64      `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64      `json:"frequency_penalty,omitempty"`
	User             string        `json:"user,omitempty"`
	Tools            []Tool        `json:"tools,omitempty"`
	ToolChoice       interface{}   `json:"tool_choice,omitempty"`
}

type ChatChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ChatCompletionResponse struct {
	ID                string       `json:"id"`
	Object            string       `json:"object"`
	Created           int64        `json:"created"`
	Model             string       `json:"model"`
	Choices           []ChatChoice `json:"choices"`
	Usage             Usage        `json:"usage"`
	SystemFingerprint string       `json:"system_fingerprint,omitempty"`
}

// Streaming types
type StreamDelta struct {
	Role    *string `json:"role,omitempty"`
	Content *string `json:"content,omitempty"`
}

type StreamChoice struct {
	Index        int          `json:"index"`
	Delta        StreamDelta  `json:"delta"`
	FinishReason *string      `json:"finish_reason"`
}

type ChatCompletionChunk struct {
	ID                string         `json:"id"`
	Object            string         `json:"object"`
	Created           int64          `json:"created"`
	Model             string         `json:"model"`
	SystemFingerprint string         `json:"system_fingerprint,omitempty"`
	Choices           []StreamChoice `json:"choices"`
}

// Embeddings
type EmbeddingsRequest struct {
	Model          string `json:"model"`
	Input          any    `json:"input"`
	EncodingFormat string `json:"encoding_format,omitempty"`
	Dimensions     *int   `json:"dimensions,omitempty"`
	User           string `json:"user,omitempty"`
}

type EmbeddingData struct {
	Object    string    `json:"object"`
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}

type EmbeddingsResponse struct {
	Object string          `json:"object"`
	Data   []EmbeddingData `json:"data"`
	Model  string          `json:"model"`
	Usage  struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// Error response
type ErrorDetail struct {
	Message string  `json:"message"`
	Type    string  `json:"type"`
	Param   *string `json:"param"`
	Code    *string `json:"code"`
}

type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ============================================================================
// Mock Data
// ============================================================================

var mockModels = []Model{
	{ID: "gpt-4", Object: "model", Created: 1687882411, OwnedBy: "openai"},
	{ID: "gpt-4-turbo", Object: "model", Created: 1712361441, OwnedBy: "openai"},
	{ID: "gpt-4-turbo-preview", Object: "model", Created: 1706037777, OwnedBy: "openai"},
	{ID: "gpt-4o", Object: "model", Created: 1715367049, OwnedBy: "openai"},
	{ID: "gpt-4o-mini", Object: "model", Created: 1721172741, OwnedBy: "openai"},
	{ID: "gpt-3.5-turbo", Object: "model", Created: 1677610602, OwnedBy: "openai"},
	{ID: "gpt-3.5-turbo-16k", Object: "model", Created: 1683758102, OwnedBy: "openai"},
	{ID: "text-embedding-ada-002", Object: "model", Created: 1671217299, OwnedBy: "openai-internal"},
	{ID: "text-embedding-3-small", Object: "model", Created: 1705948997, OwnedBy: "openai"},
	{ID: "text-embedding-3-large", Object: "model", Created: 1705953180, OwnedBy: "openai"},
}

var mockResponses = []string{
	"Hello! I'm a mock OpenAI server. How can I help you today?",
	"I'm here to assist with your testing needs. This is a simulated response.",
	"This is a mock response from the OpenAI-compatible server. Everything is working correctly!",
	"Greetings! I'm a test server simulating OpenAI's API. Feel free to experiment!",
	"Mock response generated successfully. Your API integration is working!",
}

// ============================================================================
// Helpers
// ============================================================================

func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, DELETE, PUT, PATCH")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

func sendError(w http.ResponseWriter, status int, message, errType string, param, code *string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{
		Error: ErrorDetail{
			Message: message,
			Type:    errType,
			Param:   param,
			Code:    code,
		},
	})
}

func estimateTokens(text string) int {
	// Rough approximation: ~4 chars per token
	return len(text) / 4
}

func generateFingerprint() string {
	return fmt.Sprintf("fp_%s", uuid.New().String()[:12])
}

// ============================================================================
// Handlers
// ============================================================================

func modelsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed", "invalid_request_error", nil, nil)
		return
	}

	response := ModelsResponse{
		Object: "list",
		Data:   mockModels,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func modelByIDHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed", "invalid_request_error", nil, nil)
		return
	}

	// Extract model ID from path: /v1/models/{model_id}
	path := strings.TrimPrefix(r.URL.Path, "/v1/models/")
	modelID := strings.TrimSuffix(path, "/")

	for _, model := range mockModels {
		if model.ID == modelID {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(model)
			return
		}
	}

	code := "model_not_found"
	sendError(w, http.StatusNotFound, fmt.Sprintf("The model '%s' does not exist", modelID), "invalid_request_error", nil, &code)
}

func chatCompletionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed", "invalid_request_error", nil, nil)
		return
	}

	var req ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		param := "body"
		sendError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err), "invalid_request_error", &param, nil)
		return
	}

	// Validate required fields
	if req.Model == "" {
		param := "model"
		sendError(w, http.StatusBadRequest, "Missing required parameter: 'model'", "invalid_request_error", &param, nil)
		return
	}

	if len(req.Messages) == 0 {
		param := "messages"
		sendError(w, http.StatusBadRequest, "Missing required parameter: 'messages'", "invalid_request_error", &param, nil)
		return
	}

	// Handle streaming
	if req.Stream {
		handleStreamingChat(w, req)
		return
	}

	// Handle tool calls if tools are provided
	var responseMessage ChatMessage
	finishReason := "stop"

	if len(req.Tools) > 0 && shouldUseTool(req) {
		// Simulate a tool call response
		tool := req.Tools[0]
		responseMessage = ChatMessage{
			Role: "assistant",
			ToolCalls: []ToolCall{
				{
					ID:   "call_" + uuid.New().String()[:8],
					Type: "function",
					Function: struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}{
						Name:      tool.Function.Name,
						Arguments: `{"mock": "arguments"}`,
					},
				},
			},
		}
		finishReason = "tool_calls"
	} else {
		// Regular response
		mockContent := mockResponses[rand.Intn(len(mockResponses))]
		responseMessage = ChatMessage{
			Role:    "assistant",
			Content: MessageContent{Text: mockContent},
		}
	}

	// Calculate tokens
	promptTokens := 0
	for _, msg := range req.Messages {
		promptTokens += estimateTokens(msg.Content.GetText())
	}
	completionTokens := estimateTokens(responseMessage.Content.GetText())

	// Determine number of choices
	n := 1
	if req.N != nil && *req.N > 0 {
		n = *req.N
	}

	choices := make([]ChatChoice, n)
	for i := 0; i < n; i++ {
		choices[i] = ChatChoice{
			Index:        i,
			Message:      responseMessage,
			FinishReason: finishReason,
		}
	}

	response := ChatCompletionResponse{
		ID:                "chatcmpl-" + uuid.New().String()[:24],
		Object:            "chat.completion",
		Created:           time.Now().Unix(),
		Model:             req.Model,
		Choices:           choices,
		Usage: Usage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens * n,
			TotalTokens:      promptTokens + completionTokens*n,
		},
		SystemFingerprint: generateFingerprint(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func shouldUseTool(req ChatCompletionRequest) bool {
	// Check if tool_choice forces tool use
	if req.ToolChoice != nil {
		switch v := req.ToolChoice.(type) {
		case string:
			return v == "required" || v == "auto"
		case map[string]interface{}:
			return true
		}
	}
	return rand.Float32() < 0.3 // 30% chance to use tool when available
}

func handleStreamingChat(w http.ResponseWriter, req ChatCompletionRequest) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		sendError(w, http.StatusInternalServerError, "Streaming not supported", "server_error", nil, nil)
		return
	}

	completionID := "chatcmpl-" + uuid.New().String()[:24]
	created := time.Now().Unix()
	fingerprint := generateFingerprint()

	// Generate response content
	mockContent := mockResponses[rand.Intn(len(mockResponses))]
	words := strings.Fields(mockContent)

	// Send initial chunk with role
	assistantRole := "assistant"
	initialChunk := ChatCompletionChunk{
		ID:                completionID,
		Object:            "chat.completion.chunk",
		Created:           created,
		Model:             req.Model,
		SystemFingerprint: fingerprint,
		Choices: []StreamChoice{
			{
				Index: 0,
				Delta: StreamDelta{Role: &assistantRole},
			},
		},
	}
	sendSSEChunk(w, flusher, initialChunk)

	// Stream content word by word
	for i, word := range words {
		time.Sleep(50 * time.Millisecond) // Simulate typing delay

		content := word
		if i < len(words)-1 {
			content += " "
		}

		chunk := ChatCompletionChunk{
			ID:                completionID,
			Object:            "chat.completion.chunk",
			Created:           created,
			Model:             req.Model,
			SystemFingerprint: fingerprint,
			Choices: []StreamChoice{
				{
					Index: 0,
					Delta: StreamDelta{Content: &content},
				},
			},
		}
		sendSSEChunk(w, flusher, chunk)
	}

	// Send final chunk with finish_reason
	finishReason := "stop"
	finalChunk := ChatCompletionChunk{
		ID:                completionID,
		Object:            "chat.completion.chunk",
		Created:           created,
		Model:             req.Model,
		SystemFingerprint: fingerprint,
		Choices: []StreamChoice{
			{
				Index:        0,
				Delta:        StreamDelta{},
				FinishReason: &finishReason,
			},
		},
	}
	sendSSEChunk(w, flusher, finalChunk)

	// Send [DONE] message
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func sendSSEChunk(w http.ResponseWriter, flusher http.Flusher, chunk ChatCompletionChunk) {
	data, _ := json.Marshal(chunk)
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

func embeddingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed", "invalid_request_error", nil, nil)
		return
	}

	var req EmbeddingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		param := "body"
		sendError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err), "invalid_request_error", &param, nil)
		return
	}

	// Validate required fields
	if req.Model == "" {
		param := "model"
		sendError(w, http.StatusBadRequest, "Missing required parameter: 'model'", "invalid_request_error", &param, nil)
		return
	}

	if req.Input == nil {
		param := "input"
		sendError(w, http.StatusBadRequest, "Missing required parameter: 'input'", "invalid_request_error", &param, nil)
		return
	}

	// Determine embedding dimensions
	dimensions := 1536 // default for ada-002 and 3-small
	if req.Model == "text-embedding-3-large" {
		dimensions = 3072
	}
	// Allow custom dimensions for v3 models
	if req.Dimensions != nil && (req.Model == "text-embedding-3-small" || req.Model == "text-embedding-3-large") {
		dimensions = *req.Dimensions
	}

	// Parse inputs
	var inputs []string
	switch v := req.Input.(type) {
	case string:
		inputs = []string{v}
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok {
				inputs = append(inputs, s)
			}
		}
	}

	// Generate embeddings
	totalTokens := 0
	data := make([]EmbeddingData, len(inputs))
	for i, input := range inputs {
		totalTokens += estimateTokens(input)

		// Generate normalized random embedding
		embedding := make([]float64, dimensions)
		var sumSq float64
		for j := range embedding {
			embedding[j] = rand.NormFloat64()
			sumSq += embedding[j] * embedding[j]
		}
		// Normalize to unit vector
		norm := 1.0 / (sumSq + 1e-10)
		for j := range embedding {
			embedding[j] *= norm
		}

		data[i] = EmbeddingData{
			Object:    "embedding",
			Embedding: embedding,
			Index:     i,
		}
	}

	response := EmbeddingsResponse{
		Object: "list",
		Data:   data,
		Model:  req.Model,
	}
	response.Usage.PromptTokens = totalTokens
	response.Usage.TotalTokens = totalTokens

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ============================================================================
// Router
// ============================================================================

// Global verbose flag
var verbose bool

func logRequest(r *http.Request) {
	if !verbose {
		return
	}

	log.Printf("[%s] %s", r.Method, r.URL.Path)

	// Log custom headers (X-* headers)
	for name, values := range r.Header {
		if strings.HasPrefix(name, "X-") {
			for _, v := range values {
				log.Printf("  Header: %s: %s", name, v)
			}
		}
	}

	// Log Authorization header (masked)
	if auth := r.Header.Get("Authorization"); auth != "" {
		if len(auth) > 20 {
			log.Printf("  Header: Authorization: %s...%s", auth[:10], auth[len(auth)-4:])
		} else {
			log.Printf("  Header: Authorization: %s", auth)
		}
	}
}

func router(w http.ResponseWriter, r *http.Request) {
	logRequest(r)

	path := r.URL.Path

	switch {
	case path == "/v1/models":
		modelsHandler(w, r)
	case strings.HasPrefix(path, "/v1/models/"):
		modelByIDHandler(w, r)
	case path == "/v1/chat/completions":
		chatCompletionsHandler(w, r)
	case path == "/v1/embeddings":
		embeddingsHandler(w, r)
	default:
		code := "unknown_url"
		sendError(w, http.StatusNotFound, fmt.Sprintf("Unknown request URL: %s", path), "invalid_request_error", nil, &code)
	}
}

// ============================================================================
// Main
// ============================================================================

func main() {
	rand.Seed(time.Now().UnixNano())

	// Command line flags
	port := flag.String("port", "8000", "Port to listen on")
	certFile := flag.String("cert", "../certs/server.crt", "Server certificate file")
	keyFile := flag.String("key", "../certs/server.key", "Server key file")
	caFile := flag.String("ca", "../certs/ca.crt", "CA certificate file for client verification")
	insecure := flag.Bool("insecure", false, "Run without mTLS (plain HTTP)")
	verboseFlag := flag.Bool("verbose", false, "Enable verbose logging (shows headers)")
	flag.Parse()

	verbose = *verboseFlag

	http.HandleFunc("/", corsMiddleware(router))

	addr := ":" + *port

	fmt.Println("========================================")
	fmt.Println("       OpenAI Mock Server v3.0")
	fmt.Println("========================================")

	if *insecure {
		fmt.Printf("Server running on http://localhost%s\n\n", addr)
		fmt.Println("WARNING: Running in insecure mode (no TLS)")
	} else {
		fmt.Printf("Server running on https://localhost%s\n\n", addr)
		fmt.Println("mTLS Authentication: ENABLED")
		fmt.Printf("  CA:   %s\n", *caFile)
		fmt.Printf("  Cert: %s\n", *certFile)
		fmt.Printf("  Key:  %s\n", *keyFile)
	}

	fmt.Println("")
	fmt.Println("Supported endpoints:")
	fmt.Println("  GET  /v1/models              - List models")
	fmt.Println("  GET  /v1/models/{id}         - Get model by ID")
	fmt.Println("  POST /v1/chat/completions    - Chat (supports streaming)")
	fmt.Println("  POST /v1/embeddings          - Generate embeddings")
	fmt.Println("")
	fmt.Println("Features:")
	fmt.Println("  - SSE streaming support")
	fmt.Println("  - Tool/function calling")
	fmt.Println("  - CORS enabled")
	fmt.Println("  - OpenAI-compatible error responses")
	if !*insecure {
		fmt.Println("  - mTLS client authentication")
	}
	if verbose {
		fmt.Println("  - Verbose logging ENABLED")
	}
	fmt.Println("========================================")

	if *insecure {
		log.Fatal(http.ListenAndServe(addr, nil))
	} else {
		// Load CA certificate for client verification
		caCert, err := os.ReadFile(*caFile)
		if err != nil {
			log.Fatalf("Failed to read CA certificate: %v", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			log.Fatal("Failed to parse CA certificate")
		}

		// Configure TLS with mTLS
		tlsConfig := &tls.Config{
			ClientCAs:  caCertPool,
			ClientAuth: tls.RequireAndVerifyClientCert,
			MinVersion: tls.VersionTLS12,
		}

		server := &http.Server{
			Addr:      addr,
			TLSConfig: tlsConfig,
		}

		log.Fatal(server.ListenAndServeTLS(*certFile, *keyFile))
	}
}
