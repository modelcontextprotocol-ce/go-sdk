package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/modelcontextprotocol-ce/go-sdk/client"
	"github.com/modelcontextprotocol-ce/go-sdk/spec"
)

// StreamingTransport is a new implementation of the McpClientTransport
// interface that simulates communication for demonstration purposes.
type StreamingTransport struct {
	handler     spec.MessageHandler
	mu          sync.Mutex
	closed      bool
	streamingID string // For tracking streaming sessions
	debug       bool   // Enable debug logging
}

// NewStreamingTransport creates a new instance of StreamingTransport.
func NewStreamingTransport(debug bool) *StreamingTransport {
	return &StreamingTransport{debug: debug}
}

// Connect establishes a simulated connection and registers the message handler.
func (t *StreamingTransport) Connect(ctx context.Context, handler spec.MessageHandler) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.handler = handler
	if t.debug {
		fmt.Println("Transport connected")
	}
	return nil
}

// Send simulates sending raw message data.
func (t *StreamingTransport) Send(message []byte) error {
	if t.closed {
		return fmt.Errorf("transport closed")
	}

	if t.debug {
		fmt.Printf("DEBUG: Sent raw message: %s\n", string(message))
	}
	return nil
}

// SendMessage simulates sending a JSON-RPC message and returns a simulated response.
func (t *StreamingTransport) SendMessage(ctx context.Context, message *spec.JSONRPCMessage) (*spec.JSONRPCMessage, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil, fmt.Errorf("transport closed")
	}

	// Print the message for debugging
	if t.debug {
		jsonBytes, _ := json.MarshalIndent(message, "", "  ")
		fmt.Printf("DEBUG: Sending message: %s\n", string(jsonBytes))
	}

	// For streaming message creation, simulate streaming responses
	if message.Method == spec.MethodContentCreateMessageStream {
		// Parse request to get metadata
		var request struct {
			Metadata map[string]interface{} `json:"metadata"`
		}
		if err := json.Unmarshal(message.Params, &request); err != nil {
			return nil, fmt.Errorf("failed to parse request: %w", err)
		}

		// Extract requestId for streaming
		requestID, _ := request.Metadata["requestId"].(string)
		t.streamingID = requestID

		// Create initial response
		initialContent := "Starting garden guide...\n\n"
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

		// Simulate streaming responses in background
		go t.simulateStreamingResponses(ctx, requestID)

		return response, nil
	} else if message.Method == spec.MethodSamplingCreateMessage {
		// For non-streaming, send complete response immediately
		result := &spec.CreateMessageResult{
			Role:    "assistant",
			Content: "Here's a complete guide on creating a garden...",
			Metadata: map[string]interface{}{
				"completed": true,
			},
			StopReason: spec.StopReasonEndTurn,
		}

		resultBytes, _ := json.Marshal(result)
		response := &spec.JSONRPCMessage{
			JSONRPC: spec.JSONRPCVersion,
			ID:      message.ID,
			Result:  resultBytes,
		}

		return response, nil
	} else if message.Method == spec.MethodInitialize {
		// Handle initialization
		serverInfo := spec.Implementation{
			Name:    "Simple Server",
			Version: "1.0.0",
		}

		capabilities := spec.ServerCapabilities{
			Tools: &spec.ToolsCapabilities{
				Execution:   true,
				ListChanged: true,
				Streaming:   true,
				Parallel:    true,
			},
		}

		initResult := spec.InitializeResult{
			ProtocolVersion: spec.LatestProtocolVersion,
			ServerInfo:      serverInfo,
			Capabilities:    capabilities,
		}

		resultBytes, _ := json.Marshal(initResult)
		return &spec.JSONRPCMessage{
			JSONRPC: spec.JSONRPCVersion,
			ID:      message.ID,
			Result:  resultBytes,
		}, nil
	}

	// Default response for other methods
	return &spec.JSONRPCMessage{
		JSONRPC: spec.JSONRPCVersion,
		ID:      message.ID,
		Result:  []byte(`{}`),
	}, nil
}

// Close closes the transport.
func (t *StreamingTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.closed = true
	if t.debug {
		fmt.Println("Transport closed")
	}
	return nil
}

// simulateStreamingResponses simulates a streaming response by sending content chunks
func (t *StreamingTransport) simulateStreamingResponses(ctx context.Context, requestID string) {
	// Content chunks to simulate streaming
	chunks := []string{
		"# Garden Creation Guide\n\n",
		"## Step 1: Planning Your Garden\n\n",
		"1. Choose a suitable location with adequate sunlight (most vegetables need 6-8 hours).\n",
		"2. Measure your space and sketch a layout.\n",
		"3. Decide what plants you want to grow based on your climate and season.\n\n",
		"## Step 2: Preparing the Soil\n\n",
		"1. Clear the area of weeds, rocks, and debris.\n",
		"2. Test your soil pH and composition.\n",
		"3. Add compost or organic matter to enrich the soil.\n",
		"4. Consider raised beds for better drainage and soil control.\n\n",
		"## Step 3: Planting\n\n",
		"1. Follow seed packet instructions for spacing and depth.\n",
		"2. Plant taller crops on the north side to avoid shading.\n",
		"3. Consider companion planting to deter pests naturally.\n\n",
		"## Step 4: Maintenance\n\n",
		"1. Water regularly, preferably in the morning.\n",
		"2. Mulch to retain moisture and prevent weeds.\n",
		"3. Monitor for pests and diseases.\n",
		"4. Fertilize as needed based on plant requirements.\n\n",
		"Enjoy your garden and the fresh produce it provides!",
	}

	// Send chunks with delays to simulate streaming
	for i, chunk := range chunks {
		select {
		case <-ctx.Done():
			return // Context cancelled
		case <-time.After(500 * time.Millisecond): // Delay between chunks
			if t.handler == nil {
				return
			}

			isLast := i == len(chunks)-1
			result := &spec.CreateMessageResult{
				Role:    "assistant",
				Content: chunk,
				Metadata: map[string]interface{}{
					"requestId": requestID,
				},
			}

			// Set stop reason if this is the last chunk
			if isLast {
				result.StopReason = spec.StopReasonEndTurn
			}

			// Create notification message
			notification := &spec.JSONRPCMessage{
				JSONRPC: spec.JSONRPCVersion,
				Method:  spec.MethodMessageStreamResult,
			}

			// Marshal the parameters
			params, _ := json.Marshal(result)
			notification.Params = params

			// Send the notification
			t.handler(notification)
		}
	}
}

// StreamingMessageExample demonstrates using the streaming message creation API
func StreamingMessageExample(c client.McpSyncClient) error {
	fmt.Println("=== Streaming Message Creation Example ===")

	// Create a context with a timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a request with the message content
	request := spec.NewCreateMessageRequestBuilder().
		Content("Generate a step-by-step guide on how to create a garden. Include detailed steps for preparation, planting, and maintenance.").
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
	// Create a transport with debug logging enabled
	transport := NewStreamingTransport(true)

	// Create a client
	c := client.NewSync(transport).
		WithRequestTimeout(30 * time.Second).
		WithClientInfo(spec.Implementation{
			Name:    "Go MCP Example Client",
			Version: "1.0.0",
		}).(client.SyncBuilder).
		Build()

	// Initialize the client
	err := c.Initialize()
	if err != nil {
		log.Fatalf("Failed to initialize client: %v", err)
	}

	// Run the streaming message example
	if err := StreamingMessageExample(c); err != nil {
		log.Fatalf("Error in streaming example: %v", err)
	}
}
