package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/modelcontextprotocol-ce/go-sdk/spec"
)

// SimpleStdioTransport is a simple implementation of the McpClientTransport
// interface that simulates communication over stdio for demonstration purposes.
// This is a simplified transport suitable for examples and testing.
type SimpleStdioTransport struct {
	handler     spec.MessageHandler
	mu          sync.Mutex
	closed      bool
	requestID   int64
	streamingID string // For tracking streaming sessions
	debug       bool   // Enable debug output
}

// NewSimpleStdioTransport creates a new SimpleStdioTransport instance.
func NewSimpleStdioTransport(debug bool) *SimpleStdioTransport {
	return &SimpleStdioTransport{
		debug: debug,
	}
}

// GetHandler returns the current message handler.
func (t *SimpleStdioTransport) GetHandler() spec.MessageHandler {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.handler
}

// Connect establishes a simulated connection and registers the message handler.
func (t *SimpleStdioTransport) Connect(ctx context.Context, handler spec.MessageHandler) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.handler = handler
	if t.debug {
		fmt.Println("Transport connected")
	}
	return nil
}

// Send simulates sending raw message data.
func (t *SimpleStdioTransport) Send(message []byte) error {
	if t.closed {
		return fmt.Errorf("transport closed")
	}

	if t.debug {
		fmt.Printf("DEBUG: Sent raw message: %s\n", string(message))
	}
	return nil
}

// SendMessage simulates sending a JSON-RPC message and returns a simulated response.
func (t *SimpleStdioTransport) SendMessage(ctx context.Context, message *spec.JSONRPCMessage) (*spec.JSONRPCMessage, error) {
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

	// Handle different method types
	switch message.Method {
	case spec.MethodInitialize:
		return t.handleInitialize(message)
	case spec.MethodContentCreateMessageStream:
		return t.handleCreateMessageStream(ctx, message)
	case spec.MethodSamplingCreateMessage:
		return t.handleCreateMessage(message)
	case spec.MethodToolsList:
		return t.handleToolsList(message)
	case spec.MethodResourcesList:
		return t.handleResourcesList(message)
	case spec.MethodPromptList:
		return t.handlePromptsList(message)
	case spec.MethodPing:
		return t.handlePing(message)
	default:
		// Default empty response for other methods
		return &spec.JSONRPCMessage{
			JSONRPC: spec.JSONRPCVersion,
			ID:      message.ID,
			Result:  []byte(`{}`),
		}, nil
	}
}

// Close closes the transport.
func (t *SimpleStdioTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.closed = true
	if t.debug {
		fmt.Println("Transport closed")
	}
	return nil
}

// handleInitialize handles the initialize request.
func (t *SimpleStdioTransport) handleInitialize(message *spec.JSONRPCMessage) (*spec.JSONRPCMessage, error) {
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
		Resources: &spec.ResourcesCapabilities{
			Access:      true,
			Subscribe:   true,
			ListChanged: true,
		},
		Prompts: &spec.PromptCapabilities{
			Access:      true,
			ListChanged: true,
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

// handleCreateMessageStream handles streaming message creation requests.
func (t *SimpleStdioTransport) handleCreateMessageStream(ctx context.Context, message *spec.JSONRPCMessage) (*spec.JSONRPCMessage, error) {
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
	t.streamingID = requestID

	// Create initial response
	initialContent := "Starting response...\n\n"
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

	// Start streaming in background
	go t.simulateResponse(ctx, requestID, request.Content)

	return response, nil
}

// handleCreateMessage handles non-streaming message creation.
func (t *SimpleStdioTransport) handleCreateMessage(message *spec.JSONRPCMessage) (*spec.JSONRPCMessage, error) {
	var request struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(message.Params, &request); err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}

	// Create a simple response based on the request content
	responseContent := fmt.Sprintf("Response to: %s\n\nHere's a complete answer...", request.Content)

	result := &spec.CreateMessageResult{
		Role:    "assistant",
		Content: responseContent,
		Metadata: map[string]interface{}{
			"completed": true,
		},
		StopReason: spec.StopReasonEndTurn,
	}

	resultBytes, _ := json.Marshal(result)
	return &spec.JSONRPCMessage{
		JSONRPC: spec.JSONRPCVersion,
		ID:      message.ID,
		Result:  resultBytes,
	}, nil
}

// handleToolsList handles the tools list request.
func (t *SimpleStdioTransport) handleToolsList(message *spec.JSONRPCMessage) (*spec.JSONRPCMessage, error) {
	tools := []spec.Tool{
		{
			Name:        "echo",
			Description: "Echoes back the input",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"message":{"type":"string"}},"required":["message"]}`),
		},
		{
			Name:        "calculator",
			Description: "Performs simple calculations",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"expression":{"type":"string"}},"required":["expression"]}`),
		},
		{
			Name:        "weather",
			Description: "Gets the weather for a location",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}`),
			Streaming:   true,
		},
	}

	result := spec.ListToolsResult{
		Tools: tools,
	}

	resultBytes, _ := json.Marshal(result)
	return &spec.JSONRPCMessage{
		JSONRPC: spec.JSONRPCVersion,
		ID:      message.ID,
		Result:  resultBytes,
	}, nil
}

// handleResourcesList handles the resources list request.
func (t *SimpleStdioTransport) handleResourcesList(message *spec.JSONRPCMessage) (*spec.JSONRPCMessage, error) {
	resources := []spec.Resource{
		{
			URI:         "file:///examples/sample.txt",
			Name:        "Sample Text",
			Description: "A sample text file",
			MimeType:    "text/plain",
		},
		{
			URI:         "file:///examples/image.jpg",
			Name:        "Sample Image",
			Description: "A sample image file",
			MimeType:    "image/jpeg",
		},
	}

	result := spec.ListResourcesResult{
		Resources: resources,
	}

	resultBytes, _ := json.Marshal(result)
	return &spec.JSONRPCMessage{
		JSONRPC: spec.JSONRPCVersion,
		ID:      message.ID,
		Result:  resultBytes,
	}, nil
}

// handlePromptsList handles the prompts list request.
func (t *SimpleStdioTransport) handlePromptsList(message *spec.JSONRPCMessage) (*spec.JSONRPCMessage, error) {
	prompts := []spec.Prompt{
		{
			Name:        "greeting",
			Description: "A friendly greeting prompt",
			Arguments: []spec.PromptArgument{
				{
					Name:        "name",
					Description: "The name of the person to greet",
					Required:    true,
				},
			},
		},
		{
			Name:        "summary",
			Description: "Summarizes the provided text",
			Arguments: []spec.PromptArgument{
				{
					Name:        "text",
					Description: "The text to summarize",
					Required:    true,
				},
				{
					Name:        "length",
					Description: "The desired length of the summary",
					Required:    false,
				},
			},
		},
	}

	result := spec.ListPromptsResult{
		Prompts: prompts,
	}

	resultBytes, _ := json.Marshal(result)
	return &spec.JSONRPCMessage{
		JSONRPC: spec.JSONRPCVersion,
		ID:      message.ID,
		Result:  resultBytes,
	}, nil
}

// handlePing handles the ping request.
func (t *SimpleStdioTransport) handlePing(message *spec.JSONRPCMessage) (*spec.JSONRPCMessage, error) {
	return &spec.JSONRPCMessage{
		JSONRPC: spec.JSONRPCVersion,
		ID:      message.ID,
		Result:  []byte(`{}`),
	}, nil
}

// simulateResponse simulates sending a response based on the request content.
// Override this method in specialized transports to provide custom responses.
func (t *SimpleStdioTransport) simulateResponse(ctx context.Context, requestID string, content string) {
	// Default implementation - subclasses should override this
}
