# Go MCP SDK

This is a Go implementation of the Model Context Protocol (MCP) SDK, which provides a framework for communication between AI models and external environments using a standardized protocol.

## Overview

The Model Context Protocol enables AI models to interact with external environments through a standardized JSON-RPC based protocol. This SDK provides both client and server implementations of the protocol in Go, allowing developers to easily integrate MCP into their applications.

## Features

- Complete implementation of the MCP client and server components
- Support for both synchronous and asynchronous communication patterns
- Comprehensive error handling and timeout management
- Flexible architecture that can work with various transport implementations
- Full support for MCP features including tools, resources, and prompts

## Getting Started

### Installation

```bash
go get github.com/modelcontextprotocol-ce/go-sdk
```

### Client Example

```go
import (
    "github.com/modelcontextprotocol-ce/go-sdk/client"
    "github.com/modelcontextprotocol-ce/go-sdk/spec"
)

func main() {
    // Create a transport implementation
    transport := YourTransportImplementation()
    
    // Create a client using the builder pattern
    c := client.NewSync(transport).
        WithRequestTimeout(5 * time.Second).
        WithClientInfo(spec.Implementation{
            Name:    "My MCP Client",
            Version: "1.0.0",
        }).
        Build()
    
    // Initialize the client
    err := c.Initialize()
    if err != nil {
        log.Fatalf("Failed to initialize client: %v", err)
    }
    
    // Get available tools
    tools, err := c.GetTools()
    if err != nil {
        log.Fatalf("Failed to get tools: %v", err)
    }
    
    // Execute a tool
    var result YourResultType
    err = c.ExecuteTool("tool-name", params, &result)
    if err != nil {
        log.Fatalf("Failed to execute tool: %v", err)
    }
    
    // Close the client
    c.Close()
}
```

### Server Example

```go
import (
    "github.com/modelcontextprotocol-ce/go-sdk/server"
    "github.com/modelcontextprotocol-ce/go-sdk/spec"
)

// Tool handler function
func MyToolHandler(params interface{}) (interface{}, error) {
    // Implement your tool logic here
    return result, nil
}

func main() {
    // Create a transport provider
    transportProvider := YourTransportProviderImplementation()
    
    // Create a server using the builder pattern
    s := server.NewSync(transportProvider).
        WithRequestTimeout(5 * time.Second).
        WithServerInfo(spec.Implementation{
            Name:    "My MCP Server",
            Version: "1.0.0",
        }).
        WithTools(
            spec.Tool{
                Name:        "my-tool",
                Description: "My amazing tool",
                Parameters:  []byte(`{"type":"object","properties":{}}`),
            },
        ).
        WithAPIToken("<X-API-Token>").(server.SyncBuilder).
        WithToolHandler("my-tool", MyToolHandler).
        Build()
    
    // Start the server
    err := s.Start()
    if err != nil {
        log.Fatalf("Failed to start server: %v", err)
    }
    
    // Keep the server running (in a real application)
    select {}
    
    // Or stop the server gracefully
    s.StopGracefully(context.Background())
}
```

## Asynchronous Usage

The SDK provides asynchronous variants of both client and server:

```go
// Async client
c := client.NewAsync(transport).
    WithRequestTimeout(5 * time.Second).
    Build()

// Get tools asynchronously
toolsCh, errCh := c.GetTools(ctx)
select {
case tools := <-toolsCh:
    // Process tools
case err := <-errCh:
    // Handle error
}

// Async server
s := server.NewAsync(transportProvider).
    WithRequestTimeout(5 * time.Second).
    Build()
```

## Transport Implementation

The SDK is transport-agnostic, allowing you to implement the transport layer according to your needs (HTTP, WebSockets, stdio, etc.). You just need to implement the `McpClientTransport` or `McpServerTransport` interfaces:

```go
// Client transport
type MyClientTransport struct {}

func (t *MyClientTransport) Connect(ctx context.Context, handler spec.MessageHandler) error {
    // Implement connection logic
}

func (t *MyClientTransport) SendMessage(ctx context.Context, message *spec.JSONRPCMessage) (*spec.JSONRPCMessage, error) {
    // Implement message sending logic
}

func (t *MyClientTransport) Close() error {
    // Implement close logic
}

// Server transport provider
type MyServerTransportProvider struct {}

func (p *MyServerTransportProvider) CreateTransport() (spec.McpServerTransport, error) {
    return &MyServerTransport{}, nil
}
```

## Examples

Check out the `/examples` directory for complete examples of client and server implementations.

## License

[MIT License](../LICENSE)

