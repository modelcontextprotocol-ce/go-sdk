package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/modelcontextprotocol-ce/go-sdk/client"
	"github.com/modelcontextprotocol-ce/go-sdk/client/stream"
	"github.com/modelcontextprotocol-ce/go-sdk/spec"
)

// HTTPTransportExample demonstrates using the HTTP transport with SSE streaming
func HTTPTransportExample() error {
	fmt.Println("=== HTTP Transport with SSE Streaming Example ===")

	// Create HTTP transport
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
			Name:    "Go MCP HTTP Client",
			Version: "1.0.0",
		}).(client.SyncBuilder).
		Build()

	// Initialize the client
	// Note: In this example, we're assuming a server is running locally
	fmt.Println("Initializing client...")
	err := c.Initialize()
	if err != nil {
		return fmt.Errorf("failed to initialize client: %w", err)
	}
	fmt.Println("Client successfully initialized")

	// Get tools (optional)
	fmt.Println("Getting available tools...")
	tools, err := c.GetTools()
	if err != nil {
		fmt.Printf("Warning: Failed to get tools: %v\n", err)
	} else {
		fmt.Printf("Found %d tools\n", len(tools))
		for _, tool := range tools {
			fmt.Printf("  - %s: %s\n", tool.Name, tool.Description)
		}
	}

	// Create a context with timeout for our streaming request
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create a message creation request with streaming
	fmt.Println("\nCreating a streaming message...")
	request := spec.NewCreateMessageRequestBuilder().
		Content("Explain quantum computing and its real-world applications in detail").
		ModelPreferences(&spec.ModelPreferences{
			Hints: []spec.ModelHint{
				{Name: "gpt-4-turbo"},
			},
		}).
		MaxTokens(2000).
		Temperature(0.7).
		Build()

	// Call the streaming API
	resultCh, errCh := c.CreateMessageStream(ctx, &request)

	// Process the streaming responses
	var fullContent string
	fmt.Println("\nStreaming response:")
	for {
		select {
		case result, ok := <-resultCh:
			if !ok {
				// Channel closed, streaming complete
				fmt.Println("\n\nStreaming complete. Final content length:", len(fullContent))
				return nil
			}

			// Print the new content and add it to the full content
			fmt.Print(result.Content)
			fullContent += result.Content

			// Check if this is a partial or complete response
			if result.StopReason != "" {
				fmt.Printf("\n\nMessage generation completed with reason: %s\n", result.StopReason)
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

func main() {
	if err := HTTPTransportExample(); err != nil {
		log.Fatalf("Error in HTTP transport example: %v", err)
	}
}
