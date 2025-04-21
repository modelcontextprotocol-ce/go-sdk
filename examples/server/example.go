package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/modelcontextprotocol-ce/go-sdk/server"
	"github.com/modelcontextprotocol-ce/go-sdk/spec"
)

// SimpleServerTransport is a simple implementation of the McpServerTransport interface
// for demonstration purposes
type SimpleServerTransport struct {
	handler spec.MessageHandler
}

// Listen starts listening for client connections
func (t *SimpleServerTransport) Listen(ctx context.Context, handler spec.MessageHandler) error {
	t.handler = handler
	fmt.Println("Server listening via simulated transport")
	return nil
}

// ReceiveMessage simulates receiving a message from a client
func (t *SimpleServerTransport) ReceiveMessage() (interface{}, spec.McpClientSession, error) {
	// In a real implementation, this would wait for messages from clients
	// For demo purposes, we'll just return an error indicating no messages available
	return nil, nil, fmt.Errorf("no messages available")
}

// SendMessage simulates sending a message to a client
func (t *SimpleServerTransport) SendMessage(ctx context.Context, message *spec.JSONRPCMessage) (*spec.JSONRPCMessage, error) {
	fmt.Println("<- Sending message:", messageToString(message))
	return nil, nil
}

// Send sends raw message data to the client
func (t *SimpleServerTransport) Send(message []byte) error {
	fmt.Println("<- Sending raw message:", string(message))
	return nil
}

// Close closes the transport
func (t *SimpleServerTransport) Close() error {
	fmt.Println("Transport closed")
	return nil
}

// Start implements McpServerTransport
func (t *SimpleServerTransport) Start() error {
	fmt.Println("Transport started")
	return nil
}

// Stop implements McpServerTransport
func (t *SimpleServerTransport) Stop() error {
	fmt.Println("Transport stopped")
	return nil
}

// StopGracefully implements McpServerTransport
func (t *SimpleServerTransport) StopGracefully(ctx context.Context) error {
	fmt.Println("Transport gracefully stopped")
	return nil
}

// Handler registration methods
func (t *SimpleServerTransport) SetInitializeHandler(handler func(ctx context.Context, session spec.McpClientSession, request *spec.InitializeRequest) (*spec.InitializeResult, error)) {
	fmt.Println("Registered initialize handler")
}

func (t *SimpleServerTransport) SetPingHandler(handler func(ctx context.Context, session spec.McpClientSession) error) {
	fmt.Println("Registered ping handler")
}

func (t *SimpleServerTransport) SetToolsListHandler(handler func(ctx context.Context, session spec.McpClientSession) (*spec.ListToolsResult, error)) {
	fmt.Println("Registered tools list handler")
}

func (t *SimpleServerTransport) SetToolsCallHandler(handler func(ctx context.Context, session spec.McpClientSession, request *spec.CallToolRequest) (*spec.CallToolResult, error)) {
	fmt.Println("Registered tools call handler")
}

func (t *SimpleServerTransport) SetResourcesListHandler(handler func(ctx context.Context, session spec.McpClientSession) (*spec.ListResourcesResult, error)) {
	fmt.Println("Registered resources list handler")
}

func (t *SimpleServerTransport) SetResourcesReadHandler(handler func(ctx context.Context, session spec.McpClientSession, request *spec.ReadResourceRequest) (*spec.ReadResourceResult, error)) {
	fmt.Println("Registered resources read handler")
}

func (t *SimpleServerTransport) SetResourcesSubscribeHandler(handler func(ctx context.Context, session spec.McpClientSession, request *spec.SubscribeRequest) error) {
	fmt.Println("Registered resources subscribe handler")
}

func (t *SimpleServerTransport) SetResourcesUnsubscribeHandler(handler func(ctx context.Context, session spec.McpClientSession, request *spec.UnsubscribeRequest) error) {
	fmt.Println("Registered resources unsubscribe handler")
}

func (t *SimpleServerTransport) SetPromptsListHandler(handler func(ctx context.Context, session spec.McpClientSession) (*spec.ListPromptsResult, error)) {
	fmt.Println("Registered prompts list handler")
}

func (t *SimpleServerTransport) SetPromptGetHandler(handler func(ctx context.Context, session spec.McpClientSession, request *spec.GetPromptRequest) (*spec.GetPromptResult, error)) {
	fmt.Println("Registered prompt get handler")
}

func (t *SimpleServerTransport) SetLoggingSetLevelHandler(handler func(ctx context.Context, session spec.McpClientSession, request *spec.SetLevelRequest) error) {
	fmt.Println("Registered logging set level handler")
}

func (t *SimpleServerTransport) SetCreateMessageHandler(handler func(ctx context.Context, session spec.McpClientSession, request *spec.CreateMessageRequest) (*spec.CreateMessageResponse, error)) {
	fmt.Println("Registered create message handler")
}

// SimpleServerTransportProvider provides instances of SimpleServerTransport
type SimpleServerTransportProvider struct {
	transport spec.McpServerTransport
}

// CreateTransport creates a new server transport
func (p *SimpleServerTransportProvider) CreateTransport() (spec.McpServerTransport, error) {
	return &SimpleServerTransport{}, nil
}

// GetTransport returns an existing transport or creates a new one if none exists
func (p *SimpleServerTransportProvider) GetTransport() (spec.McpServerTransport, error) {
	if p.transport == nil {
		var err error
		p.transport, err = p.CreateTransport()
		if err != nil {
			return nil, err
		}
	}
	return p.transport, nil
}

// messageToString converts a JSON-RPC message to a string for display
func messageToString(message *spec.JSONRPCMessage) string {
	if message == nil {
		return "nil"
	}

	if message.IsRequest() {
		return fmt.Sprintf("Request: method=%s", message.Method)
	}

	if message.IsNotification() {
		return fmt.Sprintf("Notification: method=%s", message.Method)
	}

	if message.IsResponse() {
		if message.Error != nil {
			return fmt.Sprintf("Response: error=%s", message.Error.Message)
		}
		return "Response: success"
	}

	return "Unknown message type"
}

// EchoToolHandler handles the 'echo' tool execution
func EchoToolHandler(ctx context.Context, params []byte) (interface{}, error) {
	var p map[string]interface{}

	// Parse the JSON parameters
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}

	// Extract the message
	message, ok := p["message"]
	if !ok {
		return nil, fmt.Errorf("message parameter is required")
	}

	// Return the echo response
	return map[string]interface{}{
		"echoed": message,
	}, nil
}

// CountingStreamToolHandler is an example of a streaming tool handler that streams numbers
func CountingStreamToolHandler(ctx context.Context, params []byte) (chan interface{}, chan error) {
	resultCh := make(chan interface{}, 5)
	errCh := make(chan error, 1)

	// Parse parameters
	var p map[string]interface{}
	if err := json.Unmarshal(params, &p); err != nil {
		errCh <- err
		close(resultCh)
		return resultCh, errCh
	}

	// Get the count parameter or use default
	count := 5
	if countParam, ok := p["count"]; ok {
		if countFloat, ok := countParam.(float64); ok {
			count = int(countFloat)
		}
	}

	// Get the delay parameter or use default
	delay := 500 * time.Millisecond
	if delayParam, ok := p["delay"]; ok {
		if delayFloat, ok := delayParam.(float64); ok {
			delay = time.Duration(delayFloat) * time.Millisecond
		}
	}

	// Start streaming in a goroutine
	go func() {
		defer close(resultCh)
		defer close(errCh)

		for i := 1; i <= count; i++ {
			select {
			case <-ctx.Done():
				// Context cancelled
				errCh <- ctx.Err()
				return
			case <-time.After(delay):
				// Send the current number as a text content
				resultCh <- &spec.TextContent{
					Text: fmt.Sprintf("Number: %d", i),
				}
			}
		}
	}()

	return resultCh, errCh
}

// SimpleResourceHandler handles resource access requests
func SimpleResourceHandler(ctx context.Context, uri string) ([]byte, error) {
	// For demonstration purposes, we'll just return some hardcoded data
	switch uri {
	case "file://example/hello.txt":
		return []byte("Hello, world!"), nil
	default:
		return nil, fmt.Errorf("resource not found: %s", uri)
	}
}

// SimplePromptHandler handles prompt template requests
func SimplePromptHandler(ctx context.Context, id string, variables map[string]interface{}) (string, error) {
	// For demonstration purposes, we'll just handle a simple prompt
	switch id {
	case "greeting":
		name, ok := variables["name"].(string)
		if !ok {
			return "", fmt.Errorf("name parameter is required")
		}
		return fmt.Sprintf("Hello, %s!", name), nil
	default:
		return "", fmt.Errorf("prompt template not found: %s", id)
	}
}

// SimpleCreateMessageHandler handles message creation requests
func SimpleCreateMessageHandler(ctx context.Context, request spec.CreateMessageRequest) (*spec.CreateMessageResponse, error) {
	// For demonstration purposes, we'll just echo back the message with a different role
	result := &spec.CreateMessageResult{
		Role:    "assistant",
		Content: "You said: " + request.Content,
	}

	// Wrap the result in a response
	return &spec.CreateMessageResponse{
		Result: result,
	}, nil
}

func main() {
	// Create a transport provider
	transportProvider := &SimpleServerTransportProvider{}

	// Create a server
	s := server.NewSync(transportProvider).
		WithRequestTimeout(5*time.Second).
		WithServerInfo(spec.Implementation{
			Name:    "Go MCP Example Server",
			Version: "1.0.0",
		}).
		WithTools(
			spec.Tool{
				Name:        "echo",
				Description: "Echoes back the input",
				InputSchema: []byte(`{"type":"object","properties":{"message":{"type":"string"}},"required":["message"]}`),
				Streaming:   false,
			},
			spec.Tool{
				Name:        "counting_stream",
				Description: "Streams numbers up to a count",
				InputSchema: []byte(`{"type":"object","properties":{"count":{"type":"integer"},"delay":{"type":"integer"}}}`),
				Streaming:   true,
			},
		).
		WithResources(
			spec.Resource{
				URI:         "file://example/hello.txt",
				MimeType:    "text/plain",
				Description: "A simple hello world file",
			},
		).
		WithPrompts(
			spec.Prompt{
				Name:        "greeting",
				Description: "A simple greeting prompt",
				Arguments: []spec.PromptArgument{
					{
						Name:        "name",
						Description: "The name to greet",
						Required:    true,
					},
				},
			},
		).(server.SyncBuilder).
		WithCreateMessageHandler(SimpleCreateMessageHandler).
		WithToolHandler("echo", EchoToolHandler).
		WithStreamingToolHandler("counting_stream", CountingStreamToolHandler).
		WithResourceHandler(SimpleResourceHandler).
		WithPromptHandler(SimplePromptHandler).
		Build()

	// Start the server
	fmt.Println("Starting server...")
	err := s.Start()
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	fmt.Println("Server started successfully")

	// In a real application, we would keep the server running
	// For this example, we'll just simulate some activity and then stop

	fmt.Println("Server running... (press Ctrl+C to stop)")

	// Wait for a while to simulate server activity
	time.Sleep(60 * time.Second)

	// Stop the server gracefully
	fmt.Println("Stopping server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = s.StopGracefully(ctx)
	if err != nil {
		log.Fatalf("Failed to stop server gracefully: %v", err)
	}

	fmt.Println("Server stopped successfully")
}
