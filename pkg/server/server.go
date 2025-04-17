// Package server provides server implementations for the Model Context Protocol.
package server

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/pkg/spec"
)

// McpServer defines the common interface for all MCP servers.
type McpServer interface {
	// Start starts the server.
	Start() error

	// Stop stops the server.
	Stop() error

	// StopGracefully stops the server gracefully, waiting for pending operations to complete.
	StopGracefully(ctx context.Context) error

	// GetProtocolVersion returns the protocol version used by the server.
	GetProtocolVersion() string

	// GetServerInfo returns information about the server.
	GetServerInfo() spec.Implementation

	// GetServerCapabilities returns the capabilities of the server.
	GetServerCapabilities() spec.ServerCapabilities

	// BroadcastToolsChanged notifies all connected clients that tools have changed.
	BroadcastToolsChanged() error

	// BroadcastResourcesChanged notifies all connected clients that resources have changed.
	BroadcastResourcesChanged() error

	// BroadcastPromptsChanged notifies all connected clients that prompts have changed.
	BroadcastPromptsChanged() error

	// BroadcastResourceChanged notifies subscribers that a specific resource has changed.
	BroadcastResourceChanged(uri string, contents []byte) error

	// SendLogMessage sends a log message to connected clients.
	SendLogMessage(level spec.LogLevel, message string, logger string) error
}

// McpSyncServer defines a synchronous MCP server interface.
// It provides blocking methods for handling MCP requests.
type McpSyncServer interface {
	McpServer

	// RegisterToolHandler registers a handler for a specific tool.
	// The handler will be called when a client requests to execute the tool.
	RegisterToolHandler(name string, handler ToolHandler) error

	// RegisterResourceHandler registers a handler for accessing resources.
	RegisterResourceHandler(handler ResourceHandler) error

	// RegisterPromptHandler registers a handler for applying prompt templates.
	RegisterPromptHandler(handler PromptHandler) error

	// SetCreateMessageHandler sets the handler for creating messages.
	SetCreateMessageHandler(handler CreateMessageHandler) error

	// AddTool adds a tool to the list of available tools.
	AddTool(tool spec.Tool) error

	// AddResource adds a resource to the list of available resources.
	AddResource(resource spec.Resource) error

	// AddPrompt adds a prompt to the list of available prompts.
	AddPrompt(prompt spec.Prompt) error

	// RemoveTool removes a tool from the list of available tools.
	RemoveTool(name string) error

	// RemoveResource removes a resource from the list of available resources.
	RemoveResource(uri string) error

	// RemovePrompt removes a prompt from the list of available prompts.
	RemovePrompt(name string) error
}

// McpAsyncServer defines an asynchronous MCP server interface.
// It provides non-blocking methods for handling MCP requests.
type McpAsyncServer interface {
	McpServer

	// RegisterToolHandler registers a handler for a specific tool.
	// The handler will be called when a client requests to execute the tool.
	RegisterToolHandler(name string, handler AsyncToolHandler) error

	// RegisterResourceHandler registers a handler for accessing resources.
	RegisterResourceHandler(handler AsyncResourceHandler) error

	// RegisterPromptHandler registers a handler for applying prompt templates.
	RegisterPromptHandler(handler AsyncPromptHandler) error

	// SetCreateMessageHandler sets the handler for creating messages.
	SetCreateMessageHandler(handler AsyncCreateMessageHandler) error

	// AddTool adds a tool to the list of available tools.
	AddTool(tool spec.Tool) error

	// AddResource adds a resource to the list of available resources.
	AddResource(resource spec.Resource) error

	// AddPrompt adds a prompt to the list of available prompts.
	AddPrompt(prompt spec.Prompt) error

	// RemoveTool removes a tool from the list of available tools.
	RemoveTool(name string) error

	// RemoveResource removes a resource from the list of available resources.
	RemoveResource(uri string) error

	// RemovePrompt removes a prompt from the list of available prompts.
	RemovePrompt(name string) error
}

// CreateMessageHandler processes message creation requests synchronously.
type CreateMessageHandler func(ctx context.Context, request spec.CreateMessageRequest) (*spec.CreateMessageResponse, error)

// ToolHandler processes tool execution requests synchronously.
type ToolHandler func(ctx context.Context, params []byte) (interface{}, error)

// ResourceHandler processes resource access requests synchronously.
type ResourceHandler func(ctx context.Context, uri string) ([]byte, error)

// PromptHandler processes prompt template requests synchronously.
type PromptHandler func(ctx context.Context, id string, variables map[string]interface{}) (string, error)

// AsyncCreateMessageHandler processes message creation requests asynchronously.
type AsyncCreateMessageHandler func(ctx context.Context, request spec.CreateMessageRequest) (chan *spec.CreateMessageResponse, chan error)

// AsyncToolHandler processes tool execution requests asynchronously.
type AsyncToolHandler func(ctx context.Context, params []byte) (chan interface{}, chan error)

// AsyncResourceHandler processes resource access requests asynchronously.
type AsyncResourceHandler func(ctx context.Context, uri string) (chan []byte, chan error)

// AsyncPromptHandler processes prompt template requests asynchronously.
type AsyncPromptHandler func(ctx context.Context, id string, variables map[string]interface{}) (chan string, chan error)

// ServerFeatures encapsulates the feature set and capabilities of an MCP server.
// It defines how the server interacts with MCP clients, including supported
// capabilities, identity information, and tool/resource/prompt management.
type ServerFeatures struct {
	// ServerInfo contains information about the server implementation.
	ServerInfo spec.Implementation

	// ServerCapabilities defines the capabilities of the server.
	ServerCapabilities spec.ServerCapabilities

	// ProtocolVersion is the version of the MCP protocol used by this server.
	ProtocolVersion string

	// Instructions are optional instructions provided to clients during initialization.
	Instructions string

	// AvailableTools is the list of tools that the server exposes to clients.
	AvailableTools []spec.Tool

	// AvailableResources is the list of resources that the server exposes to clients.
	AvailableResources []spec.Resource

	// AvailablePrompts is the list of prompt templates that the server exposes to clients.
	AvailablePrompts []spec.Prompt

	// RequestTimeout is the default timeout for requests.
	RequestTimeout time.Duration
}

// NewDefaultServerFeatures creates a new ServerFeatures instance with default settings.
func NewDefaultServerFeatures() *ServerFeatures {
	return &ServerFeatures{
		ServerInfo: spec.Implementation{
			Name:    "Go MCP Server",
			Version: "1.0.0",
		},
		ServerCapabilities: spec.ServerCapabilities{
			Logging: &spec.LoggingCapabilities{},
			Prompts: &spec.PromptCapabilities{
				ListChanged: true,
			},
			Resources: &spec.ResourcesCapabilities{
				Subscribe:   true,
				ListChanged: true,
				Access:      true,
			},
			Tools: &spec.ToolsCapabilities{
				ListChanged: true,
				Execution:   true,
			},
		},
		ProtocolVersion:    spec.LatestProtocolVersion,
		AvailableTools:     []spec.Tool{},
		AvailableResources: []spec.Resource{},
		AvailablePrompts:   []spec.Prompt{},
		RequestTimeout:     30 * time.Second,
	}
}

// NewSync creates a new synchronous server builder.
func NewSync(transportProvider spec.McpServerTransportProvider) SyncBuilder {
	return newSyncServerBuilder(transportProvider)
}

// NewAsync creates a new asynchronous server builder.
func NewAsync(transportProvider spec.McpServerTransportProvider) AsyncBuilder {
	return newAsyncServerBuilder(transportProvider)
}

// ToolExecutionContext represents the context for tool execution
type ToolExecutionContext struct {
	Request       interface{}
	ToolID        string
	ToolName      string
	ToolArguments map[string]interface{}
	Callback      ToolExecutionCallback
}

// ToolExecutionCallback is a callback for when tool execution is complete
type ToolExecutionCallback interface {
	OnToolResult(toolResult *spec.ToolResult) error
	OnToolError(err error) error
}

// ToolExecutionHandler handles tool execution requests
type ToolExecutionHandler interface {
	ExecuteTool(ctx context.Context, executionContext ToolExecutionContext) error
}

// DefaultToolExecutionHandler provides a default implementation of ToolExecutionHandler
type DefaultToolExecutionHandler struct {
	// Configuration and dependencies can be added here
}

// NewDefaultToolExecutionHandler creates a new default tool execution handler
func NewDefaultToolExecutionHandler() *DefaultToolExecutionHandler {
	return &DefaultToolExecutionHandler{}
}

// ExecuteTool executes a tool based on the provided context
func (h *DefaultToolExecutionHandler) ExecuteTool(ctx context.Context, executionContext ToolExecutionContext) error {
	// This is a default implementation that should be overridden by actual implementations
	return executionContext.Callback.OnToolError(
		fmt.Errorf("tool execution not implemented for tool: %s", executionContext.ToolName))
}

// ServerConfig represents the configuration for an MCP server
type ServerConfig struct {
	ToolExecutionHandler ToolExecutionHandler
	// Add other configuration options as needed
}

// ServerBuilder is a builder for creating server configurations
type ServerBuilder struct {
	config ServerConfig
}

// NewServerBuilder creates a new server builder
func NewServerBuilder() *ServerBuilder {
	return &ServerBuilder{
		config: ServerConfig{
			ToolExecutionHandler: NewDefaultToolExecutionHandler(),
		},
	}
}

// WithToolExecutionHandler sets the tool execution handler
func (b *ServerBuilder) WithToolExecutionHandler(handler ToolExecutionHandler) *ServerBuilder {
	b.config.ToolExecutionHandler = handler
	return b
}

// Build creates the server configuration
func (b *ServerBuilder) Build() ServerConfig {
	return b.config
}
