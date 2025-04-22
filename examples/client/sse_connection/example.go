package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/modelcontextprotocol-ce/go-sdk/client"
	"github.com/modelcontextprotocol-ce/go-sdk/client/stream"
	"github.com/modelcontextprotocol-ce/go-sdk/spec"
)

// Example that shows how to use the SSE connection with the MCP server
// This maintains a persistent connection and can be used for features like
// clipboard monitoring and other server-initiated events.
func SSEConnectionExample() error {
	fmt.Println("=== MCP Server SSE Connection Example ===")

	// Create HTTP transport with debug logging
	// In a real scenario, this would be your MCP server URL
	baseURL := "http://localhost:8080/jsonrpc"
	transport := stream.NewHTTPClientTransport(
		baseURL,
		stream.WithDebug(true),
		stream.WithRequestTimeout(30*time.Second),
	)

	// Create a client using the HTTP transport
	c := client.NewSync(transport).
		WithRequestTimeout(30 * time.Second).
		WithClientInfo(spec.Implementation{
			Name:    "Go MCP HTTP Client with SSE",
			Version: "1.0.0",
		}).(client.SyncBuilder).
		Build()

	// Initialize the client
	fmt.Println("Initializing client...")
	err := c.Initialize()
	if err != nil {
		return fmt.Errorf("failed to initialize client: %w", err)
	}
	fmt.Println("Client successfully initialized")

	// Get the underlying transport to access SSE-specific methods
	httpTransport, ok := c.GetTransport().(*stream.HTTPClientTransport)
	if !ok {
		return fmt.Errorf("unexpected transport type: %T", c.GetTransport())
	}

	// Connect to the default SSE endpoint
	fmt.Println("Connecting to SSE endpoint...")
	if err := httpTransport.ConnectToDefaultSSE(); err != nil {
		return fmt.Errorf("failed to connect to SSE endpoint: %w", err)
	}
	fmt.Println("Connected to SSE endpoint")

	// Wait a moment for the connection to establish and receive the session ID
	time.Sleep(2 * time.Second)

	// Get the session ID (should be received from the server)
	sessionID := httpTransport.GetSessionID()
	fmt.Printf("Session ID: %s\n", sessionID)

	// Create a signal channel to handle graceful termination
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("\nMaintaining SSE connection... Press Ctrl+C to exit")

	// Demonstrate basic clipboard operations if supported by the server
	go func() {
		// Wait a moment to ensure the connection is fully established
		time.Sleep(3 * time.Second)

		// Attempt clipboard operations
		// ctx := context.Background()

		// Example: Setting clipboard content
		fmt.Println("\nSetting clipboard content to 'Hello from MCP client!'")
		clipboardRequest := createClipboardUpdateRequest("Hello from MCP client!")
		var res interface{}
		err := c.ExecuteTool("clipboard_update", clipboardRequest, &res)
		if err != nil {
			fmt.Printf("Failed to set clipboard: %v\n", err)
		} else {
			fmt.Println("Clipboard content set successfully")
		}

		// Wait a moment
		time.Sleep(2 * time.Second)

		// Example: Getting clipboard content
		fmt.Println("\nGetting clipboard content")
		err = c.ExecuteTool("clipboard_get", json.RawMessage("{}"), &res)
		if err != nil {
			fmt.Printf("Failed to get clipboard: %v\n", err)
		} else {
			fmt.Printf("Clipboard content: %s\n", res)
		}
	}()

	// Wait for interrupt signal
	<-sigCh
	fmt.Println("\nReceived shutdown signal. Disconnecting...")

	// Disconnect from SSE endpoint
	httpTransport.DisconnectDefaultSSE()
	fmt.Println("Disconnected from SSE endpoint")

	return nil
}

// Helper function to create a clipboard update request
func createClipboardUpdateRequest(content string) json.RawMessage {
	req := struct {
		Content string `json:"content"`
	}{
		Content: content,
	}
	data, _ := json.Marshal(req)
	return data
}

// Main function
func main() {
	if err := SSEConnectionExample(); err != nil {
		log.Fatalf("Error in SSE connection example: %v", err)
	}
}
