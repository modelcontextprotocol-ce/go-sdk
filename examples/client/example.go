package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/modelcontextprotocol-ce/go-sdk/client"
	"github.com/modelcontextprotocol-ce/go-sdk/spec"
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

// StreamingExample demonstrates using a streaming tool
func StreamingExample(c client.McpSyncClient) error {
	fmt.Println("\n--- Streaming Tool Example ---")

	// Create a context with timeout for the streaming operation
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Define parameters for the counting stream tool
	params := map[string]interface{}{
		"count": 10,  // Stream 10 numbers
		"delay": 300, // 300ms delay between each number
	}

	fmt.Println("Executing streaming tool 'counting_stream'...")

	// Execute the streaming tool
	contentCh, errCh := c.ExecuteToolStream(ctx, "counting_stream", params)

	// Process the streaming results
	for {
		select {
		case content, ok := <-contentCh:
			if !ok {
				// Channel closed, streaming is complete
				fmt.Println("Streaming complete!")
				return nil
			}

			// Print each content item received
			for _, item := range content {
				if textContent, ok := item.(*spec.TextContent); ok {
					fmt.Printf("Received: %s\n", textContent.Text)
				} else {
					fmt.Printf("Received non-text content: %v\n", item)
				}
			}

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

// ModelPreferencesExample demonstrates how to use model preferences
// when creating messages with the MCP SDK.
func ModelPreferencesExample(client client.McpClient) {
	fmt.Println("=== Model Preferences Example ===")

	// Example 1: Use a specific model by name
	fmt.Println("Creating a message with a specific model hint:")
	result, err := client.CreateMessageWithModel("Generate a summary of the solar system", "gpt-4")
	if err != nil {
		fmt.Printf("Error creating message with specific model: %v\n", err)
	} else {
		fmt.Printf("Response using specific model: %s\n\n", result.Content)
	}

	// Example 2: Create advanced model preferences with multiple parameters
	fmt.Println("Creating a message with advanced model preferences:")
	prefs := spec.NewModelPreferencesBuilder().
		AddHint("gpt-4-turbo").
		AddHint("claude-3-opus").
		CostPriority(0.2).         // Lower priority on cost (willing to use more expensive models)
		SpeedPriority(0.5).        // Medium priority on speed
		IntelligencePriority(0.8). // High priority on intelligence/quality
		Build()

	result, err = client.CreateMessageWithModelPreferences(
		"Explain quantum computing in simple terms",
		&prefs)
	if err != nil {
		fmt.Printf("Error creating message with advanced preferences: %v\n", err)
	} else {
		fmt.Printf("Response using advanced preferences: %s\n\n", result.Content)
	}

	// Example 3: Using the CreateMessageRequest builder directly
	fmt.Println("Creating a message using the request builder:")
	req := spec.NewCreateMessageRequestBuilder().
		Content("Describe three benefits of renewable energy").
		ModelPreferences(&spec.ModelPreferences{
			Hints: []spec.ModelHint{
				{Name: "gpt-4-turbo"},
			},
			SpeedPriority: 0.7, // Prioritize faster responses
		}).
		MaxTokens(300).   // Limit the response length
		Temperature(0.7). // Add some creativity
		Build()

	result, err = client.CreateMessage(&req)
	if err != nil {
		fmt.Printf("Error creating message with request builder: %v\n", err)
	} else {
		fmt.Printf("Response using request builder: %s\n\n", result.Content)
	}
}

// RootManagementExample demonstrates how to use root management operations
// with the MCP SDK.
func RootManagementExample(client client.McpClient) {
	fmt.Println("=== Root Management Example ===")

	// Example 1: List all available roots
	fmt.Println("Listing available roots:")
	roots, err := client.GetRoots()
	if err != nil {
		fmt.Printf("Error getting roots: %v\n", err)
	} else {
		fmt.Printf("Found %d roots\n", len(roots))
		for _, root := range roots {
			fmt.Printf("- %s (%s)\n", root.URI, root.Name)
		}
	}

	// Example 2: Register for root change notifications
	client.OnRootsChanged(func(roots []spec.Root) {
		fmt.Println("Root list changed! New roots:")
		for _, root := range roots {
			fmt.Printf("- %s (%s)\n", root.URI, root.Name)
		}
	})

	// Example 3: Create a new root
	fmt.Println("\nCreating a new root:")
	newRoot := spec.Root{
		URI:  "file:///home/user/documents",
		Name: "User Documents",
	}

	createdRoot, err := client.CreateRoot(newRoot)
	if err != nil {
		fmt.Printf("Error creating root: %v\n", err)
	} else {
		fmt.Printf("Created root: %s (%s)\n", createdRoot.URI, createdRoot.Name)
	}

	// Example 4: Update an existing root
	fmt.Println("\nUpdating a root:")
	if createdRoot != nil {
		updatedRootData := *createdRoot
		updatedRootData.Name = "Updated Documents Root"

		updatedRoot, err := client.UpdateRoot(updatedRootData)
		if err != nil {
			fmt.Printf("Error updating root: %v\n", err)
		} else {
			fmt.Printf("Updated root: %s (%s)\n", updatedRoot.URI, updatedRoot.Name)
		}
	}

	// Example 5: Delete a root
	fmt.Println("\nDeleting a root:")
	if createdRoot != nil {
		err := client.DeleteRoot(createdRoot.URI)
		if err != nil {
			fmt.Printf("Error deleting root: %v\n", err)
		} else {
			fmt.Printf("Successfully deleted root: %s\n", createdRoot.URI)
		}
	}

	// Example 6: List roots again to verify changes
	fmt.Println("\nListing roots after modifications:")
	roots, err = client.GetRoots()
	if err != nil {
		fmt.Printf("Error getting roots: %v\n", err)
	} else {
		fmt.Printf("Found %d roots\n", len(roots))
		for _, root := range roots {
			fmt.Printf("- %s (%s)\n", root.URI, root.Name)
		}
	}
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

	// Demonstrate streaming tool usage
	err = StreamingExample(c)
	if err != nil {
		log.Fatalf("Streaming example failed: %v", err)
	}

	// Demonstrate model preferences usage
	ModelPreferencesExample(c)

	// Demonstrate root management usage
	RootManagementExample(c)

	// Close the client
	err = c.Close()
	if err != nil {
		log.Fatalf("Failed to close client: %v", err)
	}

	fmt.Println("Client closed successfully")
}
