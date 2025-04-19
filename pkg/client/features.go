// Package client provides client implementations for the Model Context Protocol.
package client

import (
	"github.com/modelcontextprotocol-ce/go-sdk/pkg/spec"
)

// ClientFeatures encapsulates the feature set and capabilities of an MCP client.
// It defines how the client interacts with an MCP server, including supported
// capabilities, identity information, and event handlers.
type ClientFeatures struct {
	// ClientInfo contains information about the client implementation.
	ClientInfo spec.Implementation

	// Capabilities defines what features this client supports.
	Capabilities spec.ClientCapabilities

	// Roots defines the resource roots this client can access.
	Roots map[string]spec.Root

	// ToolsChangeHandlers are called when the available tools list changes.
	ToolsChangeHandlers []ToolsChangeHandler

	// ResourcesChangeHandlers are called when the available resources list changes.
	ResourcesChangeHandlers []ResourcesChangeHandler

	// ResourceChangeHandlers are called when a specific resource changes.
	ResourceChangeHandlers []ResourceChangeHandler

	// PromptsChangeHandlers are called when the available prompts list changes.
	PromptsChangeHandlers []PromptsChangeHandler

	// LoggingHandlers are called when a logging message is received.
	LoggingHandlers []LoggingHandler

	// SamplingHandler processes message creation requests before they are sent.
	SamplingHandler SamplingHandler

	// RootsChangeHandlers are called when the available roots list changes.
	RootsChangeHandlers []RootsChangeHandler
}

// ResourceChangeHandler defines a callback for a specific resource URI
type ResourceChangeHandler struct {
	URI      string
	Callback func([]spec.ResourceContents)
}

// RootsChangeHandler is called when the list of roots changes
type RootsChangeHandler func([]spec.Root)

// NewDefaultClientFeatures creates a new ClientFeatures instance with default settings.
func NewDefaultClientFeatures() *ClientFeatures {
	return &ClientFeatures{
		ClientInfo: spec.Implementation{
			Name:    "Go MCP Client",
			Version: "1.0.0",
		},
		Capabilities: spec.ClientCapabilities{
			ProtocolVersion: spec.LatestProtocolVersion,
			Sampling: &spec.SamplingCapabilities{
				Supported: true,
			},
			Tools: &spec.ToolsCapabilities{
				Execution: true,
			},
			Resources: &spec.ResourcesCapabilities{
				Access:      true,
				Subscribe:   true,
				ListChanged: true,
			},
			Prompts: &spec.PromptsCapabilities{
				Access:      true,
				ListChanged: true,
			},
			SupportedContentTypes: []string{"text", "image", "resource"},
			MaxToolParallelism:    5,
			ToolTimeout:           30, // 30 seconds default timeout
			MaxMessageParts:       100,
		},
		Roots:                   make(map[string]spec.Root),
		ToolsChangeHandlers:     []ToolsChangeHandler{},
		ResourcesChangeHandlers: []ResourcesChangeHandler{},
		ResourceChangeHandlers:  []ResourceChangeHandler{},
		PromptsChangeHandlers:   []PromptsChangeHandler{},
		LoggingHandlers:         []LoggingHandler{},
		RootsChangeHandlers:     []RootsChangeHandler{},
	}
}

// WithExperimental adds experimental capabilities
func (f *ClientFeatures) WithExperimental(experimental map[string]interface{}) *ClientFeatures {
	f.Capabilities.Experimental = experimental
	return f
}

// WithSupportedContentTypes sets the supported content types
func (f *ClientFeatures) WithSupportedContentTypes(types []string) *ClientFeatures {
	f.Capabilities.SupportedContentTypes = types
	return f
}

// WithProtocolVersion sets the protocol version
func (f *ClientFeatures) WithProtocolVersion(version string) *ClientFeatures {
	f.Capabilities.ProtocolVersion = version
	return f
}

// WithToolTimeout sets the tool timeout value
func (f *ClientFeatures) WithToolTimeout(timeout int) *ClientFeatures {
	f.Capabilities.ToolTimeout = timeout
	return f
}

// WithMaxToolParallelism sets the maximum tool parallelism
func (f *ClientFeatures) WithMaxToolParallelism(parallelism int) *ClientFeatures {
	f.Capabilities.MaxToolParallelism = parallelism
	return f
}

// WithMaxMessageParts sets the maximum message parts
func (f *ClientFeatures) WithMaxMessageParts(maxParts int) *ClientFeatures {
	f.Capabilities.MaxMessageParts = maxParts
	return f
}

// AddRoot adds a resource root to the client features.
func (f *ClientFeatures) AddRoot(root spec.Root) *ClientFeatures {
	f.Roots[root.URI] = root
	return f
}

// AddToolsChangeHandler adds a handler for tool changes.
func (f *ClientFeatures) AddToolsChangeHandler(handler ToolsChangeHandler) *ClientFeatures {
	f.ToolsChangeHandlers = append(f.ToolsChangeHandlers, handler)
	return f
}

// AddResourcesChangeHandler adds a handler for resource changes.
func (f *ClientFeatures) AddResourcesChangeHandler(handler ResourcesChangeHandler) *ClientFeatures {
	f.ResourcesChangeHandlers = append(f.ResourcesChangeHandlers, handler)
	return f
}

// AddResourceChangeHandler adds a handler for a specific resource change.
func (f *ClientFeatures) AddResourceChangeHandler(uri string, handler func([]spec.ResourceContents)) *ClientFeatures {
	f.ResourceChangeHandlers = append(f.ResourceChangeHandlers, ResourceChangeHandler{
		URI:      uri,
		Callback: handler,
	})
	return f
}

// AddPromptsChangeHandler adds a handler for prompt changes.
func (f *ClientFeatures) AddPromptsChangeHandler(handler PromptsChangeHandler) *ClientFeatures {
	f.PromptsChangeHandlers = append(f.PromptsChangeHandlers, handler)
	return f
}

// AddLoggingHandler adds a handler for logging messages.
func (f *ClientFeatures) AddLoggingHandler(handler LoggingHandler) *ClientFeatures {
	f.LoggingHandlers = append(f.LoggingHandlers, handler)
	return f
}

// AddRootsChangeHandler adds a handler for root changes.
func (f *ClientFeatures) AddRootsChangeHandler(handler RootsChangeHandler) *ClientFeatures {
	f.RootsChangeHandlers = append(f.RootsChangeHandlers, handler)
	return f
}

// WithSamplingHandler sets the sampling handler.
func (f *ClientFeatures) WithSamplingHandler(handler SamplingHandler) *ClientFeatures {
	f.SamplingHandler = handler
	return f
}

// GetToolsChangeHandlers returns the tools change handlers.
func (f *ClientFeatures) GetToolsChangeHandlers() []ToolsChangeHandler {
	return f.ToolsChangeHandlers
}

// GetResourcesChangeHandlers returns the resources change handlers.
func (f *ClientFeatures) GetResourcesChangeHandlers() []ResourcesChangeHandler {
	return f.ResourcesChangeHandlers
}

// GetResourceChangeHandlers returns the resource change handlers.
func (f *ClientFeatures) GetResourceChangeHandlers() []ResourceChangeHandler {
	return f.ResourceChangeHandlers
}

// GetPromptsChangeHandlers returns the prompts change handlers.
func (f *ClientFeatures) GetPromptsChangeHandlers() []PromptsChangeHandler {
	return f.PromptsChangeHandlers
}

// GetLoggingHandlers returns the logging handlers.
func (f *ClientFeatures) GetLoggingHandlers() []LoggingHandler {
	return f.LoggingHandlers
}

// GetRootsChangeHandlers returns the roots change handlers.
func (f *ClientFeatures) GetRootsChangeHandlers() []RootsChangeHandler {
	return f.RootsChangeHandlers
}

// GetSamplingHandler returns the sampling handler.
func (f *ClientFeatures) GetSamplingHandler() SamplingHandler {
	return f.SamplingHandler
}

// GetRoots returns the resource roots.
func (f *ClientFeatures) GetRoots() map[string]spec.Root {
	return f.Roots
}

// HasSamplingSupport returns true if the client supports sampling capabilities.
func (f *ClientFeatures) HasSamplingSupport() bool {
	return f.Capabilities.Sampling != nil && f.Capabilities.Sampling.Supported
}

// HasToolsSupport returns true if the client supports tool execution.
func (f *ClientFeatures) HasToolsSupport() bool {
	return f.Capabilities.Tools != nil && f.Capabilities.Tools.Execution
}

// HasResourcesSupport returns true if the client supports resource access.
func (f *ClientFeatures) HasResourcesSupport() bool {
	return f.Capabilities.Resources != nil && f.Capabilities.Resources.Access
}

// HasResourcesSubscription returns true if the client supports resource subscription.
func (f *ClientFeatures) HasResourcesSubscription() bool {
	return f.Capabilities.Resources != nil && f.Capabilities.Resources.Subscribe
}

// HasPromptsSupport returns true if the client supports prompt access.
func (f *ClientFeatures) HasPromptsSupport() bool {
	return f.Capabilities.Prompts != nil && f.Capabilities.Prompts.Access
}
