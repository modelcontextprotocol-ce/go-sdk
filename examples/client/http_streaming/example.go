package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/pkg/client"
	"github.com/modelcontextprotocol/go-sdk/pkg/client/stream"
	"github.com/modelcontextprotocol/go-sdk/pkg/spec"
)

// HttpStreamingTransport extends the base StreamingTransport to use HTTP for communication
type HttpStreamingTransport struct {
	*stream.StreamingTransport
	serverURL string
	client    *http.Client
}

// NewHttpStreamingTransport creates a new HTTP-based streaming transport
func NewHttpStreamingTransport(serverURL string, debug bool) *HttpStreamingTransport {
	transport := &HttpStreamingTransport{
		StreamingTransport: stream.NewStreamingTransport(debug),
		serverURL:          serverURL,
		client:             &http.Client{Timeout: 60 * time.Second},
	}

	// Set up custom message sender
	transport.StreamingTransport.MessageSender = transport.httpSendMessage

	return transport
}

// httpSendMessage implements HTTP-based message sending
func (t *HttpStreamingTransport) httpSendMessage(ctx context.Context, message *spec.JSONRPCMessage) (*spec.JSONRPCMessage, error) {
	// If it's a streaming message, handle it specially
	if message.Method == spec.MethodContentCreateMessageStream {
		return t.handleCreateMessageStream(ctx, message)
	}

	// For other messages, send a regular HTTP request
	jsonData, err := json.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", t.serverURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Parse response
	var response spec.JSONRPCMessage
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

// handleCreateMessageStream handles streaming message creation requests over HTTP
func (t *HttpStreamingTransport) handleCreateMessageStream(ctx context.Context, message *spec.JSONRPCMessage) (*spec.JSONRPCMessage, error) {
	// Parse request to get metadata
	var request struct {
		Content  string                 `json:"content"`
		Metadata map[string]interface{} `json:"metadata"`
	}
	if err := json.Unmarshal(message.Params, &request); err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}

	// Extract requestId for streaming
	requestID, _ := request.Metadata["requestId"].(string)

	// Create initial response message
	initialContent := "Processing your request...\n\n"
	result := &spec.CreateMessageResult{
		Role:    "assistant",
		Content: initialContent,
		Metadata: map[string]interface{}{
			"requestId": requestID,
		},
	}

	resultBytes, _ := json.Marshal(result)
	response := &spec.JSONRPCMessage{
		JSONRPC: spec.JSONRPCVersion,
		ID:      message.ID,
		Result:  resultBytes,
	}

	// Start listening for SSE stream in a goroutine
	go t.startStreamingConnection(ctx, requestID)

	return response, nil
}

// startStreamingConnection establishes an HTTP connection for streaming results
func (t *HttpStreamingTransport) startStreamingConnection(ctx context.Context, requestID string) {
	// In a real implementation, you might open a Server-Sent Events (SSE) connection or WebSocket
	streamURL := fmt.Sprintf("%s/stream/%s", t.serverURL, requestID)

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", streamURL, nil)
	if err != nil {
		t.sendErrorStreamResult(requestID, fmt.Sprintf("failed to create stream request: %v", err))
		return
	}

	// Set headers for SSE
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "keep-alive")

	// For demonstration, we'll simulate streaming responses
	// In a real implementation, you would connect to the server and process the stream
	t.simulateStreamingResponses(ctx, requestID)
}

// sendErrorStreamResult sends an error through the streaming interface
func (t *HttpStreamingTransport) sendErrorStreamResult(requestID string, errorMessage string) {
	t.StreamingTransport.SendStreamingResult(requestID, "Error: "+errorMessage, true)
}

// simulateStreamingResponses simulates a streaming response sequence
func (t *HttpStreamingTransport) simulateStreamingResponses(ctx context.Context, requestID string) {
	// Sample content chunks - in a real implementation this would come from the HTTP stream
	chunks := []string{
		"# RESTful API Implementation Guide\n\n",
		"## Step 1: Define Your Resources\n\n",
		"1. Identify the key resources your API will manage\n",
		"2. Determine resource relationships\n",
		"3. Plan your URI structure (e.g., /api/v1/resources)\n\n",
		"## Step 2: Choose HTTP Methods Appropriately\n\n",
		"- GET: Retrieve resources\n",
		"- POST: Create new resources\n",
		"- PUT: Update resources (full update)\n",
		"- PATCH: Partial update of resources\n",
		"- DELETE: Remove resources\n\n",
		"## Step 3: Design Request and Response Formats\n\n",
		"1. Choose a format (usually JSON)\n",
		"2. Design consistent structures\n",
		"3. Include necessary metadata\n\n",
		"## Step 4: Implement Error Handling\n\n",
		"1. Use appropriate HTTP status codes\n",
		"2. Provide descriptive error messages\n",
		"3. Include error codes for client parsing\n\n",
		"## Step 5: Add Authentication and Authorization\n\n",
		"1. Choose an auth strategy (e.g., JWT, OAuth)\n",
		"2. Implement secure token handling\n",
		"3. Define permission levels\n\n",
		"## Step 6: Versioning Strategy\n\n",
		"1. Include version in URL or header\n",
		"2. Plan for backward compatibility\n",
		"3. Document version changes\n\n",
		"## Step 7: Implement Pagination and Filtering\n\n",
		"1. Use query parameters for pagination\n",
		"2. Add sorting options\n",
		"3. Support filtering by resource attributes\n\n",
		"## Step 8: Documentation\n\n",
		"1. Create comprehensive API docs\n",
		"2. Include examples for each endpoint\n",
		"3. Consider using tools like Swagger/OpenAPI\n\n",
		"## Step 9: Testing\n\n",
		"1. Write unit and integration tests\n",
		"2. Test error scenarios\n",
		"3. Implement performance testing\n\n",
		"## Step 10: Deployment and Monitoring\n\n",
		"1. Set up CI/CD pipeline\n",
		"2. Implement logging and monitoring\n",
		"3. Track API usage metrics\n\n",
		"Best of luck with your RESTful API implementation!",
	}

	// Send chunks with delays to simulate streaming
	for i, chunk := range chunks {
		select {
		case <-ctx.Done():
			// Context canceled, stop streaming
			return
		case <-time.After(300 * time.Millisecond):
			// Send this chunk
			isLast := i == len(chunks)-1
			t.StreamingTransport.SendStreamingResult(requestID, chunk, isLast)
		}
	}
}

// HttpStreamingExample demonstrates using the streaming transport with HTTP
func HttpStreamingExample() error {
	fmt.Println("=== HTTP Streaming Message Creation Example ===")

	// For demonstration, we'll use a mock URL
	// In a real scenario, replace with actual server URL
	serverURL := "https://mcp-server.example.com/jsonrpc"

	// Create the HTTP transport
	transport := NewHttpStreamingTransport(serverURL, true)

	// Create a client
	c := client.NewSync(transport).
		WithRequestTimeout(30 * time.Second).
		WithClientInfo(spec.Implementation{
			Name:    "Go MCP HTTP Streaming Client",
			Version: "1.0.0",
		}).(client.SyncBuilder).
		Build()

	// Initialize the client
	// Note: In this example, we're using a mock transport so initialization will succeed
	// In a real scenario, this would connect to the actual server
	err := c.Initialize()
	if err != nil {
		return fmt.Errorf("failed to initialize client: %w", err)
	}

	// Create a context with a timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a request with the message content
	request := spec.NewCreateMessageRequestBuilder().
		Content("Generate a step-by-step guide for implementing a RESTful API.").
		ModelPreferences(&spec.ModelPreferences{
			Hints: []spec.ModelHint{
				{Name: "gpt-4"},
			},
		}).
		MaxTokens(500).
		Temperature(0.7).
		Build()

	// Call the streaming API
	fmt.Println("Starting streaming message generation...")
	resultCh, errCh := c.CreateMessageStream(ctx, &request)

	// Process the streaming responses
	var fullContent string
	for {
		select {
		case result, ok := <-resultCh:
			if !ok {
				// Channel closed, streaming complete
				fmt.Println("\nStreaming complete. Final content:")
				fmt.Println(fullContent)
				return nil
			}

			// Check if this is a partial or complete response
			if result.StopReason != "" {
				fmt.Printf("\nMessage generation completed with reason: %s\n", result.StopReason)
			}

			// Print the new content and add it to the full content
			fmt.Printf("%s", result.Content)
			fullContent += result.Content

		case err, ok := <-errCh:
			if !ok {
				continue // Error channel closed without errors
			}
			return fmt.Errorf("streaming error: %v", err)

		case <-ctx.Done():
			return fmt.Errorf("streaming timed out: %v", ctx.Err())
		}
	}
}

func main() {
	if err := HttpStreamingExample(); err != nil {
		log.Fatalf("Error in HTTP streaming example: %v", err)
	}
}
