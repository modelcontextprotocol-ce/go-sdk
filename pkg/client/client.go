package client

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/pkg/spec"
)

// McpClient defines the common interface for MCP clients.
type McpClient interface {
	// Initialize initializes the client and connects to the server.
	Initialize() error

	// InitializeAsync initializes the client and connects to the server asynchronously.
	InitializeAsync() chan error

	// Close closes the client and releases any resources.
	Close() error

	// CloseGracefully closes the client gracefully, waiting for pending operations to complete.
	CloseGracefully(ctx context.Context) error

	// GetProtocolVersion returns the protocol version used by the server.
	GetProtocolVersion() string

	// GetServerInfo returns information about the server.
	GetServerInfo() spec.Implementation

	// GetServerCapabilities returns the capabilities of the server.
	GetServerCapabilities() spec.ServerCapabilities

	// GetClientCapabilities returns the capabilities of the client.
	GetClientCapabilities() spec.ClientCapabilities

	// GetTools returns the list of tools provided by the server.
	GetTools() ([]spec.Tool, error)

	// ExecuteTool executes a tool on the server.
	ExecuteTool(name string, params interface{}, resultPtr interface{}) error

	// GetResources returns the list of resources provided by the server.
	GetResources() ([]spec.Resource, error)

	// ReadResource reads a resource from the server.
	ReadResource(uri string) ([]spec.ResourceContents, error)

	// GetResourceTemplates returns the list of resource templates provided by the server.
	GetResourceTemplates() ([]spec.ResourceTemplate, error)

	// SubscribeToResource subscribes to changes in a resource.
	SubscribeToResource(uri string) error

	// UnsubscribeFromResource unsubscribes from changes in a resource.
	UnsubscribeFromResource(uri string) error

	// GetPrompts returns the list of prompts provided by the server.
	GetPrompts() ([]spec.Prompt, error)

	// GetPrompt gets a prompt from the server with the given arguments.
	GetPrompt(name string, args map[string]interface{}) (*spec.GetPromptResult, error)

	// CreateMessage sends a create message request to the server.
	CreateMessage(request *spec.CreateMessageRequest) (*spec.CreateMessageResult, error)

	// SetLoggingLevel sets the minimum level for logs from the server.
	SetLoggingLevel(level spec.LogLevel) error

	// Ping sends a ping to the server to check connectivity.
	Ping() error
}

// McpSyncClient defines the interface for a synchronous MCP client.
type McpSyncClient interface {
	McpClient
}

// McpAsyncClient defines the interface for an asynchronous MCP client.
type McpAsyncClient interface {
	McpClient

	// GetToolsAsync returns the list of tools provided by the server asynchronously.
	GetToolsAsync(ctx context.Context) (chan []spec.Tool, chan error)

	// ExecuteToolAsync executes a tool on the server asynchronously.
	ExecuteToolAsync(ctx context.Context, name string, params interface{}, resultType interface{}) (chan interface{}, chan error)

	// GetResourcesAsync returns the list of resources provided by the server asynchronously.
	GetResourcesAsync(ctx context.Context) (chan []spec.Resource, chan error)

	// ReadResourceAsync reads a resource from the server asynchronously.
	ReadResourceAsync(ctx context.Context, uri string) (chan []spec.ResourceContents, chan error)

	// GetResourceTemplatesAsync returns the list of resource templates provided by the server asynchronously.
	GetResourceTemplatesAsync(ctx context.Context) (chan []spec.ResourceTemplate, chan error)

	// SubscribeToResourceAsync subscribes to changes in a resource asynchronously.
	SubscribeToResourceAsync(ctx context.Context, uri string) chan error

	// UnsubscribeFromResourceAsync unsubscribes from changes in a resource asynchronously.
	UnsubscribeFromResourceAsync(ctx context.Context, uri string) chan error

	// GetPromptsAsync returns the list of prompts provided by the server asynchronously.
	GetPromptsAsync(ctx context.Context) (chan []spec.Prompt, chan error)

	// GetPromptAsync gets a prompt from the server with the given arguments asynchronously.
	GetPromptAsync(ctx context.Context, name string, args map[string]interface{}) (chan *spec.GetPromptResult, chan error)

	// CreateMessageAsync sends a create message request to the server asynchronously.
	CreateMessageAsync(ctx context.Context, request *spec.CreateMessageRequest) (chan *spec.CreateMessageResult, chan error)

	// SetLoggingLevelAsync sets the minimum level for logs from the server asynchronously.
	SetLoggingLevelAsync(ctx context.Context, level spec.LogLevel) chan error

	// PingAsync sends a ping to the server to check connectivity asynchronously.
	PingAsync(ctx context.Context) chan error
}

// McpClientFeatures defines optional features that can be implemented by clients.
type McpClientFeatures interface {
	// ListRoots lists the available filesystem roots.
	ListRoots(ctx context.Context) ([]spec.Root, error)

	// OnRootsChanged registers a callback to be notified when roots change.
	OnRootsChanged(callback func([]spec.Root))

	// OnToolsChanged registers a callback to be notified when tools change.
	OnToolsChanged(callback func([]spec.Tool))

	// OnResourcesChanged registers a callback to be notified when resources change.
	OnResourcesChanged(callback func([]spec.Resource))

	// OnPromptsChanged registers a callback to be notified when prompts change.
	OnPromptsChanged(callback func([]spec.Prompt))

	// OnResourceChanged registers a callback to be notified when a specific resource changes.
	OnResourceChanged(uri string, callback func([]spec.ResourceContents))

	// OnLogMessage registers a callback to be notified when a log message is received.
	OnLogMessage(callback func(spec.LoggingMessage))
}

// Builder interface for creating clients
type Builder interface {
	// WithRequestTimeout sets the timeout for all requests.
	WithRequestTimeout(timeout time.Duration) Builder

	// WithInitializationTimeout sets the timeout for initialization.
	WithInitializationTimeout(timeout time.Duration) Builder

	// WithCapabilities sets the client capabilities.
	WithCapabilities(capabilities spec.ClientCapabilities) Builder

	// WithClientInfo sets the client implementation information.
	WithClientInfo(clientInfo spec.Implementation) Builder

	// WithRoots sets the resource roots the client can access.
	WithRoots(roots ...spec.Root) Builder

	// WithToolsChangeHandler adds a handler for tool changes.
	WithToolsChangeHandler(handler ToolsChangeHandler) Builder

	// WithResourcesChangeHandler adds a handler for resource changes.
	WithResourcesChangeHandler(handler ResourcesChangeHandler) Builder

	// WithPromptsChangeHandler adds a handler for prompt changes.
	WithPromptsChangeHandler(handler PromptsChangeHandler) Builder

	// WithLoggingHandler adds a handler for logging messages.
	WithLoggingHandler(handler LoggingHandler) Builder
}

// SyncBuilder extends Builder with synchronous client specific methods
type SyncBuilder interface {
	Builder

	// WithSampling sets the sampling handler.
	WithSampling(handler SamplingHandler) SyncBuilder

	// Build creates a new McpSyncClient with the configured settings.
	Build() McpSyncClient
}

// AsyncBuilder extends Builder with asynchronous client specific methods
type AsyncBuilder interface {
	Builder

	// WithSampling sets the sampling handler.
	WithSampling(handler AsyncSamplingHandler) AsyncBuilder

	// Build creates a new McpAsyncClient with the configured settings.
	Build() McpAsyncClient
}

// NewSync creates a new synchronous client builder.
func NewSync(transport spec.McpClientTransport) SyncBuilder {
	return newSyncBuilder(transport)
}

// NewAsync creates a new asynchronous client builder.
func NewAsync(transport spec.McpClientTransport) AsyncBuilder {
	return newAsyncBuilder(transport)
}
