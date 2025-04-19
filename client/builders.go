package client

import (
	"time"

	"github.com/modelcontextprotocol-ce/go-sdk/spec"
)

// McpClientConfig holds the configuration for an MCP client
type McpClientConfig struct {
	RequestTimeout         time.Duration
	InitializationTimeout  time.Duration
	ClientCapabilities     spec.ClientCapabilities
	ClientInfo             spec.Implementation
	Roots                  []spec.Root
	ToolsChangeHandler     ToolsChangeHandler
	ResourcesChangeHandler ResourcesChangeHandler
	PromptsChangeHandler   PromptsChangeHandler
	LoggingHandler         LoggingHandler
}

// baseBuilder implements common builder functionality
type baseBuilder struct {
	config    McpClientConfig
	transport spec.McpClientTransport
}

// WithRequestTimeout sets the timeout for all requests
func (b *baseBuilder) WithRequestTimeout(timeout time.Duration) Builder {
	b.config.RequestTimeout = timeout
	return b
}

// WithInitializationTimeout sets the timeout for initialization
func (b *baseBuilder) WithInitializationTimeout(timeout time.Duration) Builder {
	b.config.InitializationTimeout = timeout
	return b
}

// WithCapabilities sets the client capabilities
func (b *baseBuilder) WithCapabilities(capabilities spec.ClientCapabilities) Builder {
	b.config.ClientCapabilities = capabilities
	return b
}

// WithClientInfo sets the client implementation information
func (b *baseBuilder) WithClientInfo(clientInfo spec.Implementation) Builder {
	b.config.ClientInfo = clientInfo
	return b
}

// WithRoots sets the resource roots the client can access
func (b *baseBuilder) WithRoots(roots ...spec.Root) Builder {
	b.config.Roots = roots
	return b
}

// WithToolsChangeHandler adds a handler for tool changes
func (b *baseBuilder) WithToolsChangeHandler(handler ToolsChangeHandler) Builder {
	b.config.ToolsChangeHandler = handler
	return b
}

// WithResourcesChangeHandler adds a handler for resource changes
func (b *baseBuilder) WithResourcesChangeHandler(handler ResourcesChangeHandler) Builder {
	b.config.ResourcesChangeHandler = handler
	return b
}

// WithPromptsChangeHandler adds a handler for prompt changes
func (b *baseBuilder) WithPromptsChangeHandler(handler PromptsChangeHandler) Builder {
	b.config.PromptsChangeHandler = handler
	return b
}

// WithLoggingHandler adds a handler for logging messages
func (b *baseBuilder) WithLoggingHandler(handler LoggingHandler) Builder {
	b.config.LoggingHandler = handler
	return b
}

// Default configuration values
var (
	DefaultRequestTimeout        = 30 * time.Second
	DefaultInitializationTimeout = 60 * time.Second
	DefaultClientInfo            = spec.Implementation{
		Name:    "Go MCP Client",
		Version: "1.0.0",
	}
)

// makeRootsMap converts a slice of spec.Root to a map keyed by URI
func makeRootsMap(roots []spec.Root) map[string]spec.Root {
	rootMap := make(map[string]spec.Root)
	for _, root := range roots {
		rootMap[root.URI] = root
	}
	return rootMap
}

// syncBuilder implements SyncBuilder
type syncBuilder struct {
	baseBuilder
	samplingHandler SamplingHandler
}

// newSyncBuilder creates a new syncBuilder with default values
func newSyncBuilder(transport spec.McpClientTransport) *syncBuilder {
	return &syncBuilder{
		baseBuilder: baseBuilder{
			transport: transport,
			config: McpClientConfig{
				RequestTimeout:        DefaultRequestTimeout,
				InitializationTimeout: DefaultInitializationTimeout,
				ClientInfo:            DefaultClientInfo,
				ClientCapabilities:    spec.NewClientCapabilitiesBuilder().Build(),
			},
		},
	}
}

// WithSampling sets the sampling handler
func (b *syncBuilder) WithSampling(handler SamplingHandler) SyncBuilder {
	b.samplingHandler = handler
	return b
}

// Build creates a new McpSyncClient with the configured settings
func (b *syncBuilder) Build() McpSyncClient {
	return &syncClientImpl{
		transport:             b.transport,
		requestTimeout:        b.config.RequestTimeout,
		initializationTimeout: b.config.InitializationTimeout,
		features: &ClientFeatures{
			ClientInfo:   b.config.ClientInfo,
			Capabilities: b.config.ClientCapabilities,
			Roots:        makeRootsMap(b.config.Roots),
			ToolsChangeHandlers: []ToolsChangeHandler{
				b.config.ToolsChangeHandler,
			},
			ResourcesChangeHandlers: []ResourcesChangeHandler{
				b.config.ResourcesChangeHandler,
			},
			ResourceChangeHandlers: []ResourceChangeHandler{},
			PromptsChangeHandlers: []PromptsChangeHandler{
				b.config.PromptsChangeHandler,
			},
			LoggingHandlers: []LoggingHandler{
				b.config.LoggingHandler,
			},
			SamplingHandler: b.samplingHandler,
		},
		session:         nil,
		initialized:     false,
		samplingHandler: b.samplingHandler,
	}
}

// asyncBuilder implements AsyncBuilder
type asyncBuilder struct {
	baseBuilder
	samplingHandler AsyncSamplingHandler
}

// newAsyncBuilder creates a new asyncBuilder with default values
func newAsyncBuilder(transport spec.McpClientTransport) *asyncBuilder {
	return &asyncBuilder{
		baseBuilder: baseBuilder{
			transport: transport,
			config: McpClientConfig{
				RequestTimeout:        DefaultRequestTimeout,
				InitializationTimeout: DefaultInitializationTimeout,
				ClientInfo:            DefaultClientInfo,
				ClientCapabilities:    spec.NewClientCapabilitiesBuilder().Build(),
			},
		},
	}
}

// WithSampling sets the sampling handler
func (b *asyncBuilder) WithSampling(handler AsyncSamplingHandler) AsyncBuilder {
	b.samplingHandler = handler
	return b
}

// Build creates a new McpAsyncClient with the configured settings
func (b *asyncBuilder) Build() McpAsyncClient {
	return newAsyncClient(b.transport, b.config, b.samplingHandler)
}

// Various handlers for client functionality
type (
	// ToolsChangeHandler is called when the available tools change
	ToolsChangeHandler func([]spec.Tool)

	// ResourcesChangeHandler is called when the available resources change
	ResourcesChangeHandler func([]spec.Resource)

	// PromptsChangeHandler is called when the available prompts change
	PromptsChangeHandler func([]spec.Prompt)

	// LoggingHandler is called when a log message is received
	LoggingHandler func(spec.LoggingMessage)

	// SamplingHandler is called to handle sampling operations
	SamplingHandler interface {
		CreateMessage(request *spec.CreateMessageRequest) (*spec.CreateMessageResult, error)
	}

	// AsyncSamplingHandler is called to handle asynchronous sampling operations
	AsyncSamplingHandler interface {
		CreateMessageAsync(request *spec.CreateMessageRequest) (chan *spec.CreateMessageResult, chan error)
	}
)
