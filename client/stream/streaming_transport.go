// Package stream provides implementations for streaming message creation
package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/modelcontextprotocol-ce/go-sdk/spec"
	"github.com/modelcontextprotocol-ce/go-sdk/util"
)

// StreamingTransport implements the McpClientTransport interface
// with built-in support for streaming message creation.
// It can be extended or wrapped to provide actual transport functionality
// like HTTP, WebSockets, etc.
type StreamingTransport struct {
	handler     spec.MessageHandler
	mu          sync.Mutex
	closed      bool
	streamingID string // For tracking streaming sessions
	debug       bool   // Enable debug logging

	// Extension points for concrete implementations
	Connector       func(ctx context.Context, handler spec.MessageHandler) error
	MessageSender   func(ctx context.Context, message *spec.JSONRPCMessage) (*spec.JSONRPCMessage, error)
	RawSender       func(message []byte) error
	Closer          func() error
	StreamResponder func(ctx context.Context, requestID string, results chan *spec.CreateMessageResult)
}

// NewStreamingTransport creates a new streaming transport instance.
func NewStreamingTransport(debug bool) *StreamingTransport {
	return &StreamingTransport{
		debug: debug,
	}
}

// Connect establishes a connection and registers the message handler.
func (t *StreamingTransport) Connect(ctx context.Context, handler spec.MessageHandler) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.handler = handler
	if t.debug {
		fmt.Println("Transport connected")
	}

	// If a custom connector is provided, use it
	if t.Connector != nil {
		return t.Connector(ctx, handler)
	}

	return nil
}

// Send sends raw message data over the transport.
func (t *StreamingTransport) Send(message []byte) error {
	if t.closed {
		return fmt.Errorf("transport closed")
	}

	if t.debug {
		fmt.Printf("DEBUG: Sent raw message: %s\n", string(message))
	}

	// If a custom raw sender is provided, use it
	if t.RawSender != nil {
		return t.RawSender(message)
	}

	return nil
}

// SendMessage sends a JSON-RPC message and returns the response.
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

	// If a custom message sender is provided, use it
	if t.MessageSender != nil {
		return t.MessageSender(ctx, message)
	}

	// For streaming message creation, handle specially
	if message.Method == spec.MethodContentCreateMessageStream {
		return t.handleCreateMessageStream(ctx, message)
	}

	// For initialization, return a default response
	if message.Method == spec.MethodInitialize {
		return t.handleInitialize(message)
	}

	// Default empty response for other methods
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

	// If a custom closer is provided, use it
	if t.Closer != nil {
		return t.Closer()
	}

	return nil
}

// handleInitialize handles the initialize request with streaming capabilities.
func (t *StreamingTransport) handleInitialize(message *spec.JSONRPCMessage) (*spec.JSONRPCMessage, error) {
	serverInfo := spec.Implementation{
		Name:    "Streaming Transport",
		Version: "1.0.0",
	}

	capabilities := spec.ServerCapabilities{
		Tools: &spec.ToolsCapabilities{
			Execution:   true,
			ListChanged: false,
			Streaming:   true, // Important for streaming support
			Parallel:    false,
		},
		Sampling: &spec.SamplingCapabilities{
			MessageCreation: true,
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
func (t *StreamingTransport) handleCreateMessageStream(ctx context.Context, message *spec.JSONRPCMessage) (*spec.JSONRPCMessage, error) {
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
	if requestID == "" {
		requestID = util.GenerateUUID()
	}
	t.streamingID = requestID

	// Create initial response
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

	// Create a channel for streaming results
	resultsChan := make(chan *spec.CreateMessageResult, 10)

	// Start streaming in background
	if t.StreamResponder != nil {
		go t.StreamResponder(ctx, requestID, resultsChan)
	} else {
		// Default implementation if no custom responder is provided
		go t.defaultStreamResponder(ctx, requestID, resultsChan)
	}

	return response, nil
}

// defaultStreamResponder is the default implementation for streaming responses
func (t *StreamingTransport) defaultStreamResponder(ctx context.Context, requestID string, results chan *spec.CreateMessageResult) {
	defer close(results)

	// Just return a simple response for default implementation
	select {
	case <-ctx.Done():
		return // Context cancelled
	case <-time.After(500 * time.Millisecond):
		if t.handler == nil {
			return
		}

		result := &spec.CreateMessageResult{
			Role:    "assistant",
			Content: "This is a placeholder response. Implement StreamResponder for actual functionality.",
			Metadata: map[string]interface{}{
				"requestId": requestID,
			},
			StopReason: spec.StopReasonEndTurn,
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

// SendStreamingResult sends a streaming result through the transport
// This is a helper method that can be used by implementations to send streaming results
func (t *StreamingTransport) SendStreamingResult(requestID string, content string, isFinal bool) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed || t.handler == nil {
		return fmt.Errorf("transport closed or no handler registered")
	}

	// Create the result
	result := &spec.CreateMessageResult{
		Role:    "assistant",
		Content: content,
		Metadata: map[string]interface{}{
			"requestId": requestID,
		},
	}

	// Set stop reason if this is the final result
	if isFinal {
		result.StopReason = spec.StopReasonEndTurn
	}

	// Create notification message
	notification := &spec.JSONRPCMessage{
		JSONRPC: spec.JSONRPCVersion,
		Method:  spec.MethodMessageStreamResult,
	}

	// Marshal the parameters
	params, err := json.Marshal(result)
	if err != nil {
		return err
	}
	notification.Params = params

	// Send the notification
	t.handler(notification)
	return nil
}
