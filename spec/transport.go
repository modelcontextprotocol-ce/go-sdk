package spec

import (
	"context"
)

// MessageHandler is a function that handles incoming JSON-RPC messages
type MessageHandler func(message *JSONRPCMessage) (*JSONRPCMessage, error)

// McpTransport defines the transport interface for MCP communication
type McpTransport interface {
	// Send sends a message over the transport
	Send(message []byte) error
	// Close closes the transport
	Close() error
}

// McpClientTransport defines the interface for a client-side transport.
type McpClientTransport interface {
	McpTransport

	// Connect establishes a connection to the server.
	// The handler will be called when a message is received from the server.
	Connect(ctx context.Context, handler MessageHandler) error

	// SendMessage sends a message to the server and returns the response.
	SendMessage(ctx context.Context, message *JSONRPCMessage) (*JSONRPCMessage, error)
}

// McpServerTransport defines the interface for a server-side transport.
type McpServerTransport interface {
	McpTransport

	// Listen starts listening for client connections.
	// The handler will be called when a message is received from a client.
	Listen(ctx context.Context, handler MessageHandler) error

	// SendMessage sends a message to a client.
	SendMessage(ctx context.Context, message *JSONRPCMessage) (*JSONRPCMessage, error)

	// ReceiveMessage receives a message from a client.
	// Returns the received message, the client session that sent the message, and any error.
	ReceiveMessage() (interface{}, McpClientSession, error)

	// Start starts the transport
	Start() error

	// Stop stops the transport immediately
	Stop() error

	// StopGracefully stops the transport after pending operations complete
	StopGracefully(ctx context.Context) error

	// Handler registration methods
	SetInitializeHandler(func(ctx context.Context, session McpClientSession, request *InitializeRequest) (*InitializeResult, error))
	SetPingHandler(func(ctx context.Context, session McpClientSession) error)
	SetToolsListHandler(func(ctx context.Context, session McpClientSession) (*ListToolsResult, error))
	SetToolsCallHandler(func(ctx context.Context, session McpClientSession, request *CallToolRequest) (*CallToolResult, error))
	SetResourcesListHandler(func(ctx context.Context, session McpClientSession) (*ListResourcesResult, error))
	SetResourcesReadHandler(func(ctx context.Context, session McpClientSession, request *ReadResourceRequest) (*ReadResourceResult, error))
	SetResourcesSubscribeHandler(func(ctx context.Context, session McpClientSession, request *SubscribeRequest) error)
	SetResourcesUnsubscribeHandler(func(ctx context.Context, session McpClientSession, request *UnsubscribeRequest) error)
	SetPromptsListHandler(func(ctx context.Context, session McpClientSession) (*ListPromptsResult, error))
	SetPromptGetHandler(func(ctx context.Context, session McpClientSession, request *GetPromptRequest) (*GetPromptResult, error))
	SetLoggingSetLevelHandler(func(ctx context.Context, session McpClientSession, request *SetLevelRequest) error)
	SetCreateMessageHandler(func(ctx context.Context, session McpClientSession, request *CreateMessageRequest) (*CreateMessageResponse, error))
}

// McpServerTransportProvider defines the interface for providing server transports.
type McpServerTransportProvider interface {
	// CreateTransport creates a new server transport.
	CreateTransport() (McpServerTransport, error)

	// GetTransport returns an existing transport or creates a new one if none exists.
	GetTransport() (McpServerTransport, error)
}

// ClientSessionOptions defines the options for a client session
type ClientSessionOptions struct {
	ClientInfo      Implementation     // Information about the client implementation
	Capabilities    ClientCapabilities // Client capabilities
	ProtocolVersion string             // Protocol version
	RequestTimeout  int64              // Request timeout in milliseconds
}

// ServerSessionOptions defines the options for a server session
type ServerSessionOptions struct {
	ServerInfo      Implementation     // Information about the server implementation
	Capabilities    ServerCapabilities // Server capabilities
	ProtocolVersion string             // Protocol version
	RequestTimeout  int64              // Request timeout in milliseconds
	Instructions    string             // Optional server instructions
}

// RootsProvider defines the interface for providing filesystem roots
type RootsProvider interface {
	// ListRoots lists the available filesystem roots
	ListRoots(ctx context.Context) ([]Root, error)
}

// ResourceProvider defines the interface for providing resources
type ResourceProvider interface {
	// ListResources lists the available resources
	ListResources(ctx context.Context, cursor string) (*ListResourcesResult, error)

	// ReadResource reads a resource's contents
	ReadResource(ctx context.Context, uri string) (*ReadResourceResult, error)

	// ListResourceTemplates lists the available resource templates
	ListResourceTemplates(ctx context.Context, cursor string) (*ListResourceTemplatesResult, error)

	// Subscribe subscribes to changes in a resource
	Subscribe(ctx context.Context, uri string) error

	// Unsubscribe unsubscribes from changes in a resource
	Unsubscribe(ctx context.Context, uri string) error
}

// ToolProvider defines the interface for providing tools
type ToolProvider interface {
	// ListTools lists the available tools
	ListTools(ctx context.Context, cursor string) (*ListToolsResult, error)

	// CallTool calls a tool with the given arguments
	CallTool(ctx context.Context, name string, args map[string]interface{}) (*CallToolResult, error)
}

// PromptProvider defines the interface for providing prompts
type PromptProvider interface {
	// ListPrompts lists the available prompts
	ListPrompts(ctx context.Context, cursor string) (*ListPromptsResult, error)

	// GetPrompt gets a prompt with the given arguments
	GetPrompt(ctx context.Context, name string, args map[string]interface{}) (*GetPromptResult, error)
}

// SamplingProvider defines the interface for providing message sampling
type SamplingProvider interface {
	// CreateMessage creates a message based on the request
	CreateMessage(ctx context.Context, request *CreateMessageRequest) (*CreateMessageResult, error)
}

// LoggingProvider defines the interface for providing logging capabilities
type LoggingProvider interface {
	// SetLevel sets the minimum logging level
	SetLevel(ctx context.Context, level LogLevel) error

	// Log logs a message with the specified level
	Log(level LogLevel, message string, logger string) error
}
