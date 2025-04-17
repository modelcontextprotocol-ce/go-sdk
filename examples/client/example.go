package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/modelcontextprotocol/go-sdk/pkg/client"
	"github.com/modelcontextprotocol/go-sdk/pkg/spec"
)

// SimpleStdioTransport is a simple implementation of the McpClientTransport interface
// for demonstration purposes. In a real application, you would implement a proper
// transport layer (HTTP, websockets, stdio, etc.)
type SimpleStdioTransport struct {
	handler spec.MessageHandler
}

// Connect establishes a simulated connection to the server
func (t *SimpleStdioTransport) Connect(ctx context.Context, handler spec.MessageHandler) error {
	t.handler = handler
	fmt.Println("Connected to server via simulated transport")
	return nil
}

// SendMessage simulates sending a message to the server
func (t *SimpleStdioTransport) SendMessage(ctx context.Context, message *spec.JSONRPCMessage) (*spec.JSONRPCMessage, error) {
	fmt.Println("-> Sending message:", messageToString(message))

	// For demonstration purposes, we'll simulate some server responses
	if message.IsRequest() && message.Method == "getTools" {
		// Simulate a getTools response
		idCopy := *message.ID

		// Create a sample tool
		tools := []spec.Tool{
			{
				Name:        "echo",
				Description: "Echoes back the input",
				InputSchema: []byte(`{"type":"object","properties":{"message":{"type":"string"}},"required":["message"]}`),
			},
		}

		// Marshal the tools
		toolsJSON, err := json.Marshal(tools)
		if err != nil {
			return nil, err
		}

		return &spec.JSONRPCMessage{
			JSONRPC: spec.JSONRPCVersion,
			ID:      &idCopy,
			Result:  toolsJSON,
		}, nil
	}

	// For any other request, echo back a success response
	if message.IsRequest() {
		idCopy := *message.ID
		return &spec.JSONRPCMessage{
			JSONRPC: spec.JSONRPCVersion,
			ID:      &idCopy,
			Result:  []byte(`{"status":"success"}`),
		}, nil
	}

	return nil, nil
}

// Send sends raw message data to the client
func (t *SimpleStdioTransport) Send(message []byte) error {
	fmt.Println("<- Sending raw message:", string(message))
	return nil
}

// Close closes the transport
func (t *SimpleStdioTransport) Close() error {
	fmt.Println("Transport closed")
	return nil
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

func main() {
	// Create a transport
	transport := &SimpleStdioTransport{}

	// Create a client
	c := client.NewSync(transport).
		WithRequestTimeout(5 * time.Second).
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

	// Get the available tools
	fmt.Println("Getting available tools...")
	tools, err := c.GetTools()
	if err != nil {
		log.Fatalf("Failed to get tools: %v", err)
	}

	fmt.Println("Available tools:")
	for _, tool := range tools {
		fmt.Printf("- %s: %s\n", tool.Name, tool.Description)
	}

	// Execute a tool
	if len(tools) > 0 {
		fmt.Printf("Executing tool '%s'...\n", tools[0].Name)

		// Create the tool parameters
		params := map[string]interface{}{
			"message": "Hello, MCP!",
		}

		// Execute the tool
		var result map[string]interface{}
		err = c.ExecuteTool(tools[0].Name, params, &result)
		if err != nil {
			log.Fatalf("Failed to execute tool: %v", err)
		}

		fmt.Println("Tool execution result:", result)
	}

	// Create a message
	fmt.Println("Creating a message...")
	request := spec.CreateMessageRequest{
		Role:    "user",
		Content: "Hello, AI!",
	}

	result, err := c.CreateMessage(&request)
	if err != nil {
		log.Fatalf("Failed to create message: %v", err)
	}

	fmt.Printf("Message created: role=%s, content=%s\n", result.Role, result.Content)

	// Close the client
	err = c.Close()
	if err != nil {
		log.Fatalf("Failed to close client: %v", err)
	}

	fmt.Println("Client closed successfully")
}
