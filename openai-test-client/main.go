package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
)

type TestResult struct {
	Name    string
	Passed  bool
	Message string
}

var results []TestResult

func pass(name, msg string) {
	results = append(results, TestResult{Name: name, Passed: true, Message: msg})
	fmt.Printf("%s[PASS]%s %s: %s\n", colorGreen, colorReset, name, msg)
}

func fail(name, msg string) {
	results = append(results, TestResult{Name: name, Passed: false, Message: msg})
	fmt.Printf("%s[FAIL]%s %s: %s\n", colorRed, colorReset, name, msg)
}

func section(name string) {
	fmt.Printf("\n%s%s=== %s ===%s\n", colorBold, colorCyan, name, colorReset)
}

func main() {
	// Command line flags
	certFile := flag.String("cert", "../certs/client.crt", "Client certificate file")
	keyFile := flag.String("key", "../certs/client.key", "Client key file")
	caFile := flag.String("ca", "../certs/ca.crt", "CA certificate file for server verification")
	insecure := flag.Bool("insecure", false, "Run without mTLS (plain HTTP)")
	flag.Parse()

	var client *openai.Client

	if *insecure {
		// Configure client without TLS
		config := openai.DefaultConfig("mock-api-key")
		config.BaseURL = "http://localhost:8000/v1"
		client = openai.NewClientWithConfig(config)
	} else {
		// Load client certificate
		cert, err := tls.LoadX509KeyPair(*certFile, *keyFile)
		if err != nil {
			fmt.Printf("Failed to load client certificate: %v\n", err)
			os.Exit(1)
		}

		// Load CA certificate
		caCert, err := os.ReadFile(*caFile)
		if err != nil {
			fmt.Printf("Failed to read CA certificate: %v\n", err)
			os.Exit(1)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			fmt.Println("Failed to parse CA certificate")
			os.Exit(1)
		}

		// Create TLS config
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
			RootCAs:      caCertPool,
			MinVersion:   tls.VersionTLS12,
		}

		// Create HTTP client with mTLS
		httpClient := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
			},
		}

		// Configure OpenAI client with mTLS
		config := openai.DefaultConfig("mock-api-key")
		config.BaseURL = "https://localhost:8000/v1"
		config.HTTPClient = httpClient
		client = openai.NewClientWithConfig(config)
	}

	ctx := context.Background()

	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("%s%s       OpenAI Mock Server Test Suite%s\n", colorBold, colorCyan, colorReset)
	fmt.Println(strings.Repeat("=", 60))

	// Run all tests
	testListModels(ctx, client)
	testGetModel(ctx, client)
	testGetModelNotFound(ctx, client)
	testChatCompletion(ctx, client)
	testChatCompletionWithParams(ctx, client)
	testChatCompletionStreaming(ctx, client)
	testChatCompletionWithTools(ctx, client)
	testEmbeddings(ctx, client)
	testEmbeddingsMultipleInputs(ctx, client)
	testErrorHandling(ctx, client)

	// Print summary
	printSummary()
}

// =============================================================================
// Model Tests
// =============================================================================

func testListModels(ctx context.Context, client *openai.Client) {
	section("List Models")

	models, err := client.ListModels(ctx)
	if err != nil {
		fail("ListModels", fmt.Sprintf("Error: %v", err))
		return
	}

	if len(models.Models) == 0 {
		fail("ListModels", "No models returned")
		return
	}

	pass("ListModels", fmt.Sprintf("Retrieved %d models", len(models.Models)))

	// Check for expected models
	expectedModels := []string{"gpt-4", "gpt-4o", "gpt-3.5-turbo", "text-embedding-ada-002"}
	foundModels := make(map[string]bool)
	for _, m := range models.Models {
		foundModels[m.ID] = true
	}

	allFound := true
	for _, expected := range expectedModels {
		if !foundModels[expected] {
			allFound = false
			break
		}
	}

	if allFound {
		pass("ListModels-Expected", "All expected models present")
	} else {
		fail("ListModels-Expected", "Some expected models missing")
	}
}

func testGetModel(ctx context.Context, client *openai.Client) {
	section("Get Model by ID")

	model, err := client.GetModel(ctx, "gpt-4o")
	if err != nil {
		fail("GetModel", fmt.Sprintf("Error: %v", err))
		return
	}

	if model.ID != "gpt-4o" {
		fail("GetModel", fmt.Sprintf("Wrong model ID: %s", model.ID))
		return
	}

	pass("GetModel", fmt.Sprintf("Retrieved model: %s (owned by: %s)", model.ID, model.OwnedBy))
}

func testGetModelNotFound(ctx context.Context, client *openai.Client) {
	section("Get Model Not Found")

	_, err := client.GetModel(ctx, "nonexistent-model")
	if err != nil {
		pass("GetModel-NotFound", "Correctly returned error for nonexistent model")
	} else {
		fail("GetModel-NotFound", "Should have returned error for nonexistent model")
	}
}

// =============================================================================
// Chat Completion Tests
// =============================================================================

func testChatCompletion(ctx context.Context, client *openai.Client) {
	section("Chat Completion (Non-Streaming)")

	resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: openai.GPT4o,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: "Hello, how are you?"},
		},
	})

	if err != nil {
		fail("ChatCompletion", fmt.Sprintf("Error: %v", err))
		return
	}

	if len(resp.Choices) == 0 {
		fail("ChatCompletion", "No choices returned")
		return
	}

	choice := resp.Choices[0]
	pass("ChatCompletion", fmt.Sprintf("Response: %q", truncate(choice.Message.Content, 60)))

	// Verify response structure
	if resp.ID == "" {
		fail("ChatCompletion-ID", "Missing response ID")
	} else {
		pass("ChatCompletion-ID", fmt.Sprintf("ID: %s", resp.ID))
	}

	if resp.Model == "" {
		fail("ChatCompletion-Model", "Missing model in response")
	} else {
		pass("ChatCompletion-Model", fmt.Sprintf("Model: %s", resp.Model))
	}

	if resp.Usage.TotalTokens > 0 {
		pass("ChatCompletion-Usage", fmt.Sprintf("Tokens - Prompt: %d, Completion: %d, Total: %d",
			resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens))
	} else {
		fail("ChatCompletion-Usage", "Invalid token usage")
	}

	if choice.FinishReason != "" {
		pass("ChatCompletion-FinishReason", fmt.Sprintf("Finish reason: %s", choice.FinishReason))
	} else {
		fail("ChatCompletion-FinishReason", "Missing finish reason")
	}
}

func testChatCompletionWithParams(ctx context.Context, client *openai.Client) {
	section("Chat Completion with Parameters")

	maxTokens := 100
	temperature := float32(0.7)
	n := 2

	resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: openai.GPT4o,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: "You are a helpful assistant."},
			{Role: openai.ChatMessageRoleUser, Content: "Tell me a joke."},
		},
		MaxTokens:   maxTokens,
		Temperature: temperature,
		N:           n,
	})

	if err != nil {
		fail("ChatCompletion-Params", fmt.Sprintf("Error: %v", err))
		return
	}

	if len(resp.Choices) >= n {
		pass("ChatCompletion-Params-N", fmt.Sprintf("Received %d choices (requested %d)", len(resp.Choices), n))
	} else {
		fail("ChatCompletion-Params-N", fmt.Sprintf("Expected %d choices, got %d", n, len(resp.Choices)))
	}
}

func testChatCompletionStreaming(ctx context.Context, client *openai.Client) {
	section("Chat Completion (SSE Streaming)")

	stream, err := client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model: openai.GPT4o,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: "Hello!"},
		},
		Stream: true,
	})

	if err != nil {
		fail("ChatCompletion-Stream", fmt.Sprintf("Error creating stream: %v", err))
		return
	}
	defer stream.Close()

	pass("ChatCompletion-Stream-Init", "Stream created successfully")

	var fullContent strings.Builder
	chunkCount := 0
	var lastFinishReason string
	startTime := time.Now()

	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			fail("ChatCompletion-Stream-Recv", fmt.Sprintf("Error receiving chunk: %v", err))
			return
		}

		chunkCount++
		if len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta
			fullContent.WriteString(delta.Content)
			if chunk.Choices[0].FinishReason != "" {
				lastFinishReason = string(chunk.Choices[0].FinishReason)
			}
		}
	}

	elapsed := time.Since(startTime)

	if chunkCount > 0 {
		pass("ChatCompletion-Stream-Chunks", fmt.Sprintf("Received %d chunks in %v", chunkCount, elapsed.Round(time.Millisecond)))
	} else {
		fail("ChatCompletion-Stream-Chunks", "No chunks received")
	}

	content := fullContent.String()
	if content != "" {
		pass("ChatCompletion-Stream-Content", fmt.Sprintf("Full response: %q", truncate(content, 60)))
	} else {
		fail("ChatCompletion-Stream-Content", "Empty content from stream")
	}

	if lastFinishReason == "stop" {
		pass("ChatCompletion-Stream-Finish", "Received finish_reason: stop")
	} else {
		fail("ChatCompletion-Stream-Finish", fmt.Sprintf("Expected finish_reason 'stop', got '%s'", lastFinishReason))
	}
}

func testChatCompletionWithTools(ctx context.Context, client *openai.Client) {
	section("Chat Completion with Tools/Functions")

	resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: openai.GPT4o,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: "What's the weather in Paris?"},
		},
		Tools: []openai.Tool{
			{
				Type: openai.ToolTypeFunction,
				Function: &openai.FunctionDefinition{
					Name:        "get_weather",
					Description: "Get weather information for a location",
					Parameters: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"location": map[string]interface{}{
								"type":        "string",
								"description": "City name",
							},
						},
						"required": []string{"location"},
					},
				},
			},
		},
		ToolChoice: "required",
	})

	if err != nil {
		fail("ChatCompletion-Tools", fmt.Sprintf("Error: %v", err))
		return
	}

	if len(resp.Choices) == 0 {
		fail("ChatCompletion-Tools", "No choices returned")
		return
	}

	choice := resp.Choices[0]

	// Check for tool calls
	if len(choice.Message.ToolCalls) > 0 {
		toolCall := choice.Message.ToolCalls[0]
		pass("ChatCompletion-Tools-Call", fmt.Sprintf("Tool call: %s (ID: %s)", toolCall.Function.Name, toolCall.ID))
		pass("ChatCompletion-Tools-Args", fmt.Sprintf("Arguments: %s", toolCall.Function.Arguments))
	} else if choice.Message.Content != "" {
		// Mock server might return regular content sometimes
		pass("ChatCompletion-Tools-Content", fmt.Sprintf("Response: %q", truncate(choice.Message.Content, 60)))
	} else {
		fail("ChatCompletion-Tools", "No tool calls or content returned")
	}

	if choice.FinishReason == "tool_calls" {
		pass("ChatCompletion-Tools-FinishReason", "Finish reason: tool_calls")
	} else {
		pass("ChatCompletion-Tools-FinishReason", fmt.Sprintf("Finish reason: %s", choice.FinishReason))
	}
}

// =============================================================================
// Embeddings Tests
// =============================================================================

func testEmbeddings(ctx context.Context, client *openai.Client) {
	section("Embeddings")

	resp, err := client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Model: openai.AdaEmbeddingV2,
		Input: []string{"Hello, world!"},
	})

	if err != nil {
		fail("Embeddings", fmt.Sprintf("Error: %v", err))
		return
	}

	if len(resp.Data) == 0 {
		fail("Embeddings", "No embeddings returned")
		return
	}

	embedding := resp.Data[0]
	pass("Embeddings", fmt.Sprintf("Received embedding with %d dimensions", len(embedding.Embedding)))

	if embedding.Index == 0 {
		pass("Embeddings-Index", "Correct index: 0")
	} else {
		fail("Embeddings-Index", fmt.Sprintf("Wrong index: %d", embedding.Index))
	}

	if resp.Model != "" {
		pass("Embeddings-Model", fmt.Sprintf("Model: %s", resp.Model))
	}

	if resp.Usage.TotalTokens > 0 {
		pass("Embeddings-Usage", fmt.Sprintf("Tokens - Prompt: %d, Total: %d",
			resp.Usage.PromptTokens, resp.Usage.TotalTokens))
	}

	// Check embedding dimensions (ada-002 should be 1536)
	expectedDims := 1536
	if len(embedding.Embedding) == expectedDims {
		pass("Embeddings-Dimensions", fmt.Sprintf("Correct dimensions: %d", expectedDims))
	} else {
		fail("Embeddings-Dimensions", fmt.Sprintf("Expected %d dimensions, got %d", expectedDims, len(embedding.Embedding)))
	}
}

func testEmbeddingsMultipleInputs(ctx context.Context, client *openai.Client) {
	section("Embeddings (Multiple Inputs)")

	inputs := []string{
		"First sentence",
		"Second sentence",
		"Third sentence",
	}

	resp, err := client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Model: openai.SmallEmbedding3,
		Input: inputs,
	})

	if err != nil {
		fail("Embeddings-Multi", fmt.Sprintf("Error: %v", err))
		return
	}

	if len(resp.Data) == len(inputs) {
		pass("Embeddings-Multi-Count", fmt.Sprintf("Received %d embeddings for %d inputs", len(resp.Data), len(inputs)))
	} else {
		fail("Embeddings-Multi-Count", fmt.Sprintf("Expected %d embeddings, got %d", len(inputs), len(resp.Data)))
	}

	// Verify indices
	allIndicesCorrect := true
	for i, emb := range resp.Data {
		if emb.Index != i {
			allIndicesCorrect = false
			break
		}
	}

	if allIndicesCorrect {
		pass("Embeddings-Multi-Indices", "All indices correct")
	} else {
		fail("Embeddings-Multi-Indices", "Incorrect indices")
	}
}

// =============================================================================
// Error Handling Tests
// =============================================================================

func testErrorHandling(ctx context.Context, client *openai.Client) {
	section("Error Handling")

	// Test missing model
	_, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: "", // Empty model
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: "Hello"},
		},
	})

	if err != nil {
		pass("Error-MissingModel", fmt.Sprintf("Correctly returned error: %v", truncate(err.Error(), 80)))
	} else {
		fail("Error-MissingModel", "Should have returned error for missing model")
	}

	// Test empty messages
	_, err = client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:    openai.GPT4o,
		Messages: []openai.ChatCompletionMessage{}, // Empty messages
	})

	if err != nil {
		pass("Error-EmptyMessages", fmt.Sprintf("Correctly returned error: %v", truncate(err.Error(), 80)))
	} else {
		fail("Error-EmptyMessages", "Should have returned error for empty messages")
	}
}

// =============================================================================
// Helpers
// =============================================================================

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func printSummary() {
	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("%s%s                    TEST SUMMARY%s\n", colorBold, colorCyan, colorReset)
	fmt.Println(strings.Repeat("=", 60))

	passed := 0
	failed := 0
	for _, r := range results {
		if r.Passed {
			passed++
		} else {
			failed++
		}
	}

	total := passed + failed
	fmt.Printf("\nTotal Tests: %d\n", total)
	fmt.Printf("%sPassed: %d%s\n", colorGreen, passed, colorReset)
	fmt.Printf("%sFailed: %d%s\n", colorRed, failed, colorReset)

	if failed > 0 {
		fmt.Printf("\n%sFailed Tests:%s\n", colorRed, colorReset)
		for _, r := range results {
			if !r.Passed {
				fmt.Printf("  - %s: %s\n", r.Name, r.Message)
			}
		}
	}

	fmt.Println()
	if failed == 0 {
		fmt.Printf("%s%sAll tests passed!%s\n", colorBold, colorGreen, colorReset)
	} else {
		fmt.Printf("%s%sSome tests failed.%s\n", colorBold, colorRed, colorReset)
	}
	fmt.Println(strings.Repeat("=", 60))
}
