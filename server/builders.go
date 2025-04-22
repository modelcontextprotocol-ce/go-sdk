package server

import (
	"fmt"
	"time"

	"github.com/modelcontextprotocol-ce/go-sdk/server/stream"
	"github.com/modelcontextprotocol-ce/go-sdk/spec"
	"github.com/modelcontextprotocol-ce/go-sdk/util"
)

// Builder interface for creating servers
type Builder interface {
	// WithRequestTimeout sets the timeout for all requests.
	WithRequestTimeout(timeout time.Duration) Builder

	// WithServerInfo sets the server implementation information.
	WithServerInfo(serverInfo spec.Implementation) Builder

	// WithCapabilities sets the server capabilities.
	WithCapabilities(capabilities spec.ServerCapabilities) Builder

	// WithTools sets the tools available on the server.
	WithTools(tools ...spec.Tool) Builder

	// WithResources sets the resources available on the server.
	WithResources(resources ...spec.Resource) Builder

	// WithPrompts sets the prompt templates available on the server.
	WithPrompts(prompts ...spec.Prompt) Builder

	// WithAPIToken sets an API token for authentication
	WithAPIToken(token string) Builder

	// WithAPITokenFromConfig loads the API token from the config file
	WithAPITokenFromConfig() Builder
}

// SyncBuilder extends Builder with synchronous server specific methods
type SyncBuilder interface {
	Builder

	// WithToolHandler sets the handler for a specific tool.
	WithToolHandler(name string, handler ToolHandler) SyncBuilder

	// WithStreamingToolHandler sets a streaming handler for a specific tool.
	WithStreamingToolHandler(name string, handler StreamingToolHandler) SyncBuilder

	// WithResourceHandler sets the handler for resources.
	WithResourceHandler(handler ResourceHandler) SyncBuilder

	// WithPromptHandler sets the handler for prompts.
	WithPromptHandler(handler PromptHandler) SyncBuilder

	// WithCreateMessageHandler sets the handler for creating messages.
	WithCreateMessageHandler(handler CreateMessageHandler) SyncBuilder

	// Build creates a new McpSyncServer with the configured settings.
	Build() McpSyncServer
}

// AsyncBuilder extends Builder with asynchronous server specific methods
type AsyncBuilder interface {
	Builder

	// WithToolHandler sets the handler for a specific tool.
	WithToolHandler(name string, handler AsyncToolHandler) AsyncBuilder

	// WithResourceHandler sets the handler for resources.
	WithResourceHandler(handler AsyncResourceHandler) AsyncBuilder

	// WithPromptHandler sets the handler for prompts.
	WithPromptHandler(handler AsyncPromptHandler) AsyncBuilder

	// WithCreateMessageHandler sets the handler for creating messages.
	WithCreateMessageHandler(handler AsyncCreateMessageHandler) AsyncBuilder

	// Build creates a new McpAsyncServer with the configured settings.
	Build() McpAsyncServer
}

// syncServerBuilder implements the SyncBuilder interface for building synchronous MCP servers
type syncServerBuilder struct {
	transportProvider     spec.McpServerTransportProvider
	requestTimeout        time.Duration
	features              *ServerFeatures
	createMessageHandler  CreateMessageHandler
	toolHandlers          map[string]ToolHandler
	streamingToolHandlers map[string]StreamingToolHandler
	resourceHandler       ResourceHandler
	promptHandler         PromptHandler
	apiToken              string
}

// asyncServerBuilder implements the AsyncBuilder interface for building asynchronous MCP servers
type asyncServerBuilder struct {
	transportProvider    spec.McpServerTransportProvider
	requestTimeout       time.Duration
	features             *ServerFeatures
	createMessageHandler AsyncCreateMessageHandler
	toolHandlers         map[string]interface{}
	resourceHandler      AsyncResourceHandler
	promptHandler        AsyncPromptHandler
	apiToken             string
}

// newSyncServerBuilder creates a new syncServerBuilder with default settings
func newSyncServerBuilder(transportProvider spec.McpServerTransportProvider) *syncServerBuilder {
	util.AssertNotNil(transportProvider, "Transport provider must not be nil")

	return &syncServerBuilder{
		transportProvider: transportProvider,
		requestTimeout:    20 * time.Second, // Default timeout
		features:          NewDefaultServerFeatures(),
		toolHandlers:      make(map[string]ToolHandler),
	}
}

// newAsyncServerBuilder creates a new asyncServerBuilder with default settings
func newAsyncServerBuilder(transportProvider spec.McpServerTransportProvider) *asyncServerBuilder {
	util.AssertNotNil(transportProvider, "Transport provider must not be nil")

	return &asyncServerBuilder{
		transportProvider: transportProvider,
		requestTimeout:    20 * time.Second, // Default timeout
		features:          NewDefaultServerFeatures(),
		toolHandlers:      make(map[string]interface{}),
	}
}

// WithRequestTimeout sets the timeout for all requests
func (b *syncServerBuilder) WithRequestTimeout(timeout time.Duration) Builder {
	util.AssertNotNil(timeout, "Request timeout must not be nil")
	b.requestTimeout = timeout
	return b
}

// WithServerInfo sets the server implementation information
func (b *syncServerBuilder) WithServerInfo(serverInfo spec.Implementation) Builder {
	util.AssertNotNil(serverInfo, "Server info must not be nil")
	b.features.ServerInfo = serverInfo
	return b
}

// WithCapabilities sets the server capabilities
func (b *syncServerBuilder) WithCapabilities(capabilities spec.ServerCapabilities) Builder {
	util.AssertNotNil(capabilities, "Server capabilities must not be nil")
	b.features.ServerCapabilities = capabilities
	return b
}

// WithTools sets the tools available on the server
func (b *syncServerBuilder) WithTools(tools ...spec.Tool) Builder {
	util.AssertNotNil(tools, "Tools must not be nil")
	b.features.AvailableTools = tools
	return b
}

// WithResources sets the resources available on the server
func (b *syncServerBuilder) WithResources(resources ...spec.Resource) Builder {
	util.AssertNotNil(resources, "Resources must not be nil")
	b.features.AvailableResources = resources
	return b
}

// WithPrompts sets the prompt templates available on the server
func (b *syncServerBuilder) WithPrompts(prompts ...spec.Prompt) Builder {
	util.AssertNotNil(prompts, "Prompts must not be nil")
	b.features.AvailablePrompts = prompts
	return b
}

// WithAPIToken sets an API token for authentication
func (b *syncServerBuilder) WithAPIToken(token string) Builder {
	b.apiToken = token
	return b
}

// WithAPITokenFromConfig loads the API token from the config file
func (b *syncServerBuilder) WithAPITokenFromConfig() Builder {
	config, err := util.LoadConfig("")
	if err != nil {
		// Log error but continue without token
		fmt.Printf("Error loading API token from config: %v\n", err)
		return b
	}

	if config.APIToken != "" {
		b.apiToken = config.APIToken
	}
	return b
}

// WithToolHandler sets the handler for a specific tool
func (b *syncServerBuilder) WithToolHandler(name string, handler ToolHandler) SyncBuilder {
	util.AssertNotNil(name, "Tool name must not be nil")
	util.AssertNotNil(handler, "Tool handler must not be nil")
	if b.toolHandlers == nil {
		b.toolHandlers = make(map[string]ToolHandler)
	}
	b.toolHandlers[name] = handler
	return b
}

// WithStreamingToolHandler sets a streaming handler for a specific tool
func (b *syncServerBuilder) WithStreamingToolHandler(name string, handler StreamingToolHandler) SyncBuilder {
	util.AssertNotNil(name, "Tool name must not be nil")
	util.AssertNotNil(handler, "Streaming tool handler must not be nil")
	if b.streamingToolHandlers == nil {
		b.streamingToolHandlers = make(map[string]StreamingToolHandler)
	}
	b.streamingToolHandlers[name] = handler
	return b
}

// WithResourceHandler sets the handler for resources
func (b *syncServerBuilder) WithResourceHandler(handler ResourceHandler) SyncBuilder {
	util.AssertNotNil(handler, "Resource handler must not be nil")
	b.resourceHandler = handler
	return b
}

// WithPromptHandler sets the handler for prompts
func (b *syncServerBuilder) WithPromptHandler(handler PromptHandler) SyncBuilder {
	util.AssertNotNil(handler, "Prompt handler must not be nil")
	b.promptHandler = handler
	return b
}

// WithCreateMessageHandler sets the handler for creating messages
func (b *syncServerBuilder) WithCreateMessageHandler(handler CreateMessageHandler) SyncBuilder {
	util.AssertNotNil(handler, "Create message handler must not be nil")
	b.createMessageHandler = handler
	return b
}

// Build creates a new McpSyncServer with the configured settings
func (b *syncServerBuilder) Build() McpSyncServer {
	// If using HTTP server transport, pass the API token
	if httpProvider, ok := b.transportProvider.(*stream.HTTPServerTransportProvider); ok && b.apiToken != "" {
		// Add API token option to the transport provider
		httpProvider.WithAPIToken(b.apiToken)
	}

	return &syncServerImpl{
		transportProvider:    b.transportProvider,
		requestTimeout:       b.requestTimeout,
		features:             b.features,
		createMessageHandler: b.createMessageHandler,
		toolHandlers:         b.toolHandlers,
		resourceHandler:      b.resourceHandler,
		promptHandler:        b.promptHandler,
	}
}

// WithRequestTimeout sets the timeout for all requests
func (b *asyncServerBuilder) WithRequestTimeout(timeout time.Duration) Builder {
	util.AssertNotNil(timeout, "Request timeout must not be nil")
	b.requestTimeout = timeout
	return b
}

// WithServerInfo sets the server implementation information
func (b *asyncServerBuilder) WithServerInfo(serverInfo spec.Implementation) Builder {
	util.AssertNotNil(serverInfo, "Server info must not be nil")
	b.features.ServerInfo = serverInfo
	return b
}

// WithCapabilities sets the server capabilities
func (b *asyncServerBuilder) WithCapabilities(capabilities spec.ServerCapabilities) Builder {
	util.AssertNotNil(capabilities, "Server capabilities must not be nil")
	b.features.ServerCapabilities = capabilities
	return b
}

// WithTools sets the tools available on the server
func (b *asyncServerBuilder) WithTools(tools ...spec.Tool) Builder {
	util.AssertNotNil(tools, "Tools must not be nil")
	b.features.AvailableTools = tools
	return b
}

// WithResources sets the resources available on the server
func (b *asyncServerBuilder) WithResources(resources ...spec.Resource) Builder {
	util.AssertNotNil(resources, "Resources must not be nil")
	b.features.AvailableResources = resources
	return b
}

// WithPrompts sets the prompt templates available on the server
func (b *asyncServerBuilder) WithPrompts(prompts ...spec.Prompt) Builder {
	util.AssertNotNil(prompts, "Prompts must not be nil")
	b.features.AvailablePrompts = prompts
	return b
}

// WithAPIToken sets an API token for authentication
func (b *asyncServerBuilder) WithAPIToken(token string) Builder {
	b.apiToken = token
	return b
}

// WithAPITokenFromConfig loads the API token from the config file
func (b *asyncServerBuilder) WithAPITokenFromConfig() Builder {
	config, err := util.LoadConfig("")
	if err != nil {
		// Log error but continue without token
		fmt.Printf("Error loading API token from config: %v\n", err)
		return b
	}

	if config.APIToken != "" {
		b.apiToken = config.APIToken
	}
	return b
}

// WithToolHandler sets the handler for a specific tool
func (b *asyncServerBuilder) WithToolHandler(name string, handler AsyncToolHandler) AsyncBuilder {
	util.AssertNotNil(name, "Tool name must not be nil")
	util.AssertNotNil(handler, "Tool handler must not be nil")
	if b.toolHandlers == nil {
		b.toolHandlers = make(map[string]interface{})
	}
	b.toolHandlers[name] = handler
	return b
}

// WithResourceHandler sets the handler for resources
func (b *asyncServerBuilder) WithResourceHandler(handler AsyncResourceHandler) AsyncBuilder {
	util.AssertNotNil(handler, "Resource handler must not be nil")
	b.resourceHandler = handler
	return b
}

// WithPromptHandler sets the handler for prompts
func (b *asyncServerBuilder) WithPromptHandler(handler AsyncPromptHandler) AsyncBuilder {
	util.AssertNotNil(handler, "Prompt handler must not be nil")
	b.promptHandler = handler
	return b
}

// WithCreateMessageHandler sets the handler for creating messages
func (b *asyncServerBuilder) WithCreateMessageHandler(handler AsyncCreateMessageHandler) AsyncBuilder {
	util.AssertNotNil(handler, "Create message handler must not be nil")
	b.createMessageHandler = handler
	return b
}

// Build creates a new McpAsyncServer with the configured settings
func (b *asyncServerBuilder) Build() McpAsyncServer {
	// If using HTTP server transport, pass the API token
	if httpProvider, ok := b.transportProvider.(*stream.HTTPServerTransportProvider); ok && b.apiToken != "" {
		// Add API token option to the transport provider
		httpProvider.WithAPIToken(b.apiToken)
	}

	return &asyncServerImpl{
		transportProvider:    b.transportProvider,
		requestTimeout:       b.requestTimeout,
		features:             b.features,
		createMessageHandler: b.createMessageHandler,
		toolHandlers:         b.toolHandlers,
		resourceHandler:      b.resourceHandler,
		promptHandler:        b.promptHandler,
	}
}
