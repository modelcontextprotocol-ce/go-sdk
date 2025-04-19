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

	// ExecuteToolsParallel executes multiple tools on the server in parallel.
	// The toolCalls parameter is a map where keys are tool names and values are the parameters to pass to each tool.
	// The results parameter is a map where keys are tool names and values are pointers to store the results.
	// Returns a map of tool names to errors for any failed tool executions.
	ExecuteToolsParallel(ctx context.Context, toolCalls map[string]interface{}, results map[string]interface{}) map[string]error

	// ExecuteToolStream executes a tool on the server with streaming support.
	// It returns a channel that receives content as it becomes available and a channel for errors.
	// The contents channel will be closed when streaming is complete.
	ExecuteToolStream(ctx context.Context, name string, params interface{}) (chan []spec.Content, chan error)

	// GetResources returns the list of resources provided by the server.
	GetResources() ([]spec.Resource, error)

	// ReadResource reads a resource from the server.
	ReadResource(uri string) ([]spec.ResourceContents, error)

	// CreateResource creates a new resource on the server.
	CreateResource(resource spec.Resource, contents []byte) error

	// UpdateResource updates an existing resource on the server.
	UpdateResource(resource spec.Resource, contents []byte) error

	// DeleteResource deletes a resource from the server.
	DeleteResource(uri string) error

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

	// CreatePrompt creates a new prompt on the server.
	CreatePrompt(prompt spec.Prompt, messages []spec.PromptMessage) error

	// UpdatePrompt updates an existing prompt on the server.
	UpdatePrompt(prompt spec.Prompt, messages []spec.PromptMessage) error

	// DeletePrompt deletes a prompt from the server.
	DeletePrompt(name string) error

	// CreateMessage sends a create message request to the server.
	CreateMessage(request *spec.CreateMessageRequest) (*spec.CreateMessageResult, error)

	// CreateMessageStream sends a create message request to the server and streams the responses.
	// The returned channel will receive partial results as they become available.
	CreateMessageStream(ctx context.Context, request *spec.CreateMessageRequest) (<-chan *spec.CreateMessageResult, <-chan error)

	// CreateMessageWithModelPreferences creates a new message with specified model preferences
	CreateMessageWithModelPreferences(content string, modelPreferences *spec.ModelPreferences) (*spec.CreateMessageResult, error)

	// CreateMessageWithModel creates a new message with a specific model hint
	CreateMessageWithModel(content string, modelName string) (*spec.CreateMessageResult, error)

	// SetLoggingLevel sets the minimum level for logs from the server.
	SetLoggingLevel(level spec.LogLevel) error

	// Ping sends a ping to the server to check connectivity.
	Ping() error

	// GetRoots returns the list of roots available on the server.
	GetRoots() ([]spec.Root, error)

	// CreateRoot creates a new root on the server.
	CreateRoot(root spec.Root) (*spec.Root, error)

	// UpdateRoot updates an existing root on the server.
	UpdateRoot(root spec.Root) (*spec.Root, error)

	// DeleteRoot deletes a root from the server.
	DeleteRoot(uri string) error

	// OnRootsChanged registers a callback to be notified when roots change.
	OnRootsChanged(callback func([]spec.Root))
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

	// ExecuteToolsParallelAsync executes multiple tools on the server in parallel asynchronously.
	// The toolCalls parameter is a map where keys are tool names and values are the parameters to pass to each tool.
	// Returns two channels: one for results (map of tool names to results) and one for errors.
	ExecuteToolsParallelAsync(ctx context.Context, toolCalls map[string]interface{}, resultTypes map[string]interface{}) (chan map[string]interface{}, chan error)

	// GetResourcesAsync returns the list of resources provided by the server asynchronously.
	GetResourcesAsync(ctx context.Context) (chan []spec.Resource, chan error)

	// ReadResourceAsync reads a resource from the server asynchronously.
	ReadResourceAsync(ctx context.Context, uri string) (chan []spec.ResourceContents, chan error)

	// CreateResourceAsync creates a new resource on the server asynchronously.
	CreateResourceAsync(ctx context.Context, resource spec.Resource, contents []byte) chan error

	// UpdateResourceAsync updates an existing resource on the server asynchronously.
	UpdateResourceAsync(ctx context.Context, resource spec.Resource, contents []byte) chan error

	// DeleteResourceAsync deletes a resource from the server asynchronously.
	DeleteResourceAsync(ctx context.Context, uri string) chan error

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

	// CreatePromptAsync creates a new prompt on the server asynchronously.
	CreatePromptAsync(ctx context.Context, prompt spec.Prompt, messages []spec.PromptMessage) chan error

	// UpdatePromptAsync updates an existing prompt on the server asynchronously.
	UpdatePromptAsync(ctx context.Context, prompt spec.Prompt, messages []spec.PromptMessage) chan error

	// DeletePromptAsync deletes a prompt from the server asynchronously.
	DeletePromptAsync(ctx context.Context, name string) chan error

	// CreateMessageAsync sends a create message request to the server asynchronously.
	CreateMessageAsync(ctx context.Context, request *spec.CreateMessageRequest) (chan *spec.CreateMessageResult, chan error)

	// CreateMessageStreamAsync sends a create message request to the server and streams the responses asynchronously.
	// The returned channels will receive partial results and errors as they become available.
	CreateMessageStreamAsync(ctx context.Context, request *spec.CreateMessageRequest) (chan *spec.CreateMessageResult, chan error)

	// SetLoggingLevelAsync sets the minimum level for logs from the server asynchronously.
	SetLoggingLevelAsync(ctx context.Context, level spec.LogLevel) chan error

	// PingAsync sends a ping to the server to check connectivity asynchronously.
	PingAsync(ctx context.Context) chan error

	// GetRootsAsync returns the list of roots available on the server asynchronously.
	GetRootsAsync(ctx context.Context) (chan []spec.Root, chan error)

	// CreateRootAsync creates a new root on the server asynchronously.
	CreateRootAsync(ctx context.Context, root spec.Root) (chan *spec.Root, chan error)

	// UpdateRootAsync updates an existing root on the server asynchronously.
	UpdateRootAsync(ctx context.Context, root spec.Root) (chan *spec.Root, chan error)

	// DeleteRootAsync deletes a root from the server asynchronously.
	DeleteRootAsync(ctx context.Context, uri string) chan error

	// CancelOperation cancels a specific operation by ID
	CancelOperation(operationID string) bool

	// CancelCreateMessage cancels an ongoing message creation operation
	CancelCreateMessage(messageID string) bool

	// CancelToolExecution cancels an ongoing tool execution operation
	CancelToolExecution(toolID string) bool

	// GetOngoingOperations returns the IDs of all ongoing operations
	GetOngoingOperations() []string
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
