package spec

import (
	"encoding/json"
	"fmt"
)

// JSONRPCVersion defines the JSON-RPC version used by MCP
const JSONRPCVersion = "2.0"

// LatestProtocolVersion defines the latest MCP protocol version
const LatestProtocolVersion = "2024-11-05"

// Error codes as defined by the JSON-RPC specification
const (
	ErrCodeParseError     = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternalError  = -32603
)

// Additional MCP-specific error codes
const (
	ErrCodeTimeoutError   = -32000
	ErrCodeTransportError = -32001
	ErrCodeSessionClosed  = -32002
)

// Additional MCP server error codes
const (
	// ErrCodeServerError represents a general server error
	ErrCodeServerError = -32099
	// ErrCodeClientError represents a client error
	ErrCodeClientError = -32098
)

// MCP method names
const (
	// Lifecycle Methods
	MethodInitialize              = "initialize"
	MethodNotificationInitialized = "notifications/initialized"
	MethodPing                    = "ping"

	// Tool Methods
	MethodToolsList                    = "tools/list"
	MethodToolsCall                    = "tools/call"
	MethodNotificationToolsListChanged = "notifications/tools/list_changed"

	// Resources Methods
	MethodResourcesList                    = "resources/list"
	MethodResourcesRead                    = "resources/read"
	MethodNotificationResourcesListChanged = "notifications/resources/list_changed"
	MethodNotificationResourceChanged      = "notifications/resources/changed"
	MethodResourcesTemplatesList           = "resources/templates/list"
	MethodResourcesSubscribe               = "resources/subscribe"
	MethodResourcesUnsubscribe             = "resources/unsubscribe"

	// Prompt Methods
	MethodPromptList                     = "prompts/list"
	MethodPromptGet                      = "prompts/get"
	MethodNotificationPromptsListChanged = "notifications/prompts/list_changed"

	// Logging Methods
	MethodLoggingSetLevel     = "logging/setLevel"
	MethodNotificationMessage = "notifications/message"

	// Roots Methods
	MethodRootsList                    = "roots/list"
	MethodNotificationRootsListChanged = "notifications/roots/list_changed"

	// Sampling Methods
	MethodSamplingCreateMessage       = "sampling/createMessage"
	MethodSamplingCreateMessageStream = "sampling/createMessageStream"
)

// JSONRPCMessage represents a JSON-RPC message
type JSONRPCMessage struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *JSONRPCError    `json:"error,omitempty"`
}

// IsRequest returns true if this is a request message
func (m *JSONRPCMessage) IsRequest() bool {
	return m != nil && m.Method != "" && m.ID != nil
}

// IsNotification returns true if this is a notification message
func (m *JSONRPCMessage) IsNotification() bool {
	return m != nil && m.Method != "" && m.ID == nil
}

// IsResponse returns true if this is a response message
func (m *JSONRPCMessage) IsResponse() bool {
	return m != nil && m.Method == "" && m.ID != nil
}

// HasError returns true if this message contains an error
func (m *JSONRPCMessage) HasError() bool {
	return m != nil && m.Error != nil
}

// JSONRPCError represents a JSON-RPC error
type JSONRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error returns the error message for JSON-RPC errors
func (e *JSONRPCError) Error() string {
	if e == nil {
		return "<nil error>"
	}
	return fmt.Sprintf("JSON-RPC error %d: %s", e.Code, e.Message)
}

// McpError implements the error interface for MCP-specific errors
type McpError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error returns the error message
func (e *McpError) Error() string {
	if e == nil {
		return "<nil error>"
	}
	return fmt.Sprintf("MCP error %d: %s", e.Code, e.Message)
}

// IsTimeoutError returns true if this is a timeout error
func (e *McpError) IsTimeoutError() bool {
	return e != nil && e.Code == ErrCodeTimeoutError
}

// IsTransportError returns true if this is a transport error
func (e *McpError) IsTransportError() bool {
	return e != nil && e.Code == ErrCodeTransportError
}

// Implementation represents information about the implementation of a client or server
type Implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ClientCapabilities represents the capabilities of an MCP client
type ClientCapabilities struct {
	Experimental          map[string]interface{} `json:"experimental,omitempty"`
	Roots                 *RootCapabilities      `json:"roots,omitempty"`
	Sampling              *SamplingCapabilities  `json:"sampling,omitempty"`
	Tools                 *ToolsCapabilities     `json:"tools,omitempty"`
	Resources             *ResourcesCapabilities `json:"resources,omitempty"`
	ProtocolVersion       string                 `json:"protocolVersion,omitempty"`
	ToolTimeout           int                    `json:"toolTimeout,omitempty"`
	MaxToolParallelism    int                    `json:"maxToolParallelism,omitempty"`
	MaxMessageParts       int                    `json:"maxMessageParts,omitempty"`
	SupportedContentTypes []string               `json:"supportedContentTypes,omitempty"`
	Prompts               *PromptsCapabilities   `json:"prompts,omitempty"`
}

// ClientCapabilitiesBuilder is used to build ClientCapabilities
type ClientCapabilitiesBuilder struct {
	cap ClientCapabilities
}

// NewClientCapabilitiesBuilder creates a new builder with default values
func NewClientCapabilitiesBuilder() *ClientCapabilitiesBuilder {
	return &ClientCapabilitiesBuilder{
		cap: ClientCapabilities{
			ProtocolVersion: LatestProtocolVersion,
		},
	}
}

// WithProtocolVersion sets the protocol version
func (b *ClientCapabilitiesBuilder) WithProtocolVersion(version string) *ClientCapabilitiesBuilder {
	b.cap.ProtocolVersion = version
	return b
}

// WithToolTimeout sets the maximum time a tool call should take
func (b *ClientCapabilitiesBuilder) WithToolTimeout(timeout int) *ClientCapabilitiesBuilder {
	b.cap.ToolTimeout = timeout
	return b
}

// WithMaxToolParallelism sets the maximum number of tools that can be executed in parallel
func (b *ClientCapabilitiesBuilder) WithMaxToolParallelism(parallelism int) *ClientCapabilitiesBuilder {
	b.cap.MaxToolParallelism = parallelism
	return b
}

// WithMaxMessageParts sets the maximum number of message parts that can be handled
func (b *ClientCapabilitiesBuilder) WithMaxMessageParts(maxParts int) *ClientCapabilitiesBuilder {
	b.cap.MaxMessageParts = maxParts
	return b
}

// WithSupportedContentTypes sets the content types supported by the client
func (b *ClientCapabilitiesBuilder) WithSupportedContentTypes(contentTypes []string) *ClientCapabilitiesBuilder {
	b.cap.SupportedContentTypes = contentTypes
	return b
}

// WithSamplingCapabilities sets the sampling capabilities
func (b *ClientCapabilitiesBuilder) WithSamplingCapabilities(supported bool) *ClientCapabilitiesBuilder {
	b.cap.Sampling = &SamplingCapabilities{
		Supported: supported,
	}
	return b
}

// WithAdvancedSamplingCapabilities sets advanced sampling capabilities
func (b *ClientCapabilitiesBuilder) WithAdvancedSamplingCapabilities(supported bool, maxTokens int, streamingSupported bool, temperatureSupport bool, supportedModels []string, contextInclusionMax int) *ClientCapabilitiesBuilder {
	b.cap.Sampling = &SamplingCapabilities{
		Supported:           supported,
		MaxTokens:           maxTokens,
		StreamingSupported:  streamingSupported,
		TemperatureSupport:  temperatureSupport,
		SupportedModels:     supportedModels,
		ContextInclusionMax: contextInclusionMax,
	}
	return b
}

// WithPromptsCapabilities sets the prompts capabilities
func (b *ClientCapabilitiesBuilder) WithPromptsCapabilities(access bool, listChanged bool) *ClientCapabilitiesBuilder {
	b.cap.Prompts = &PromptsCapabilities{
		Access:      access,
		ListChanged: listChanged,
	}
	return b
}

// WithRootCapabilities sets the root capabilities
func (b *ClientCapabilitiesBuilder) WithRootCapabilities(listChanged bool) *ClientCapabilitiesBuilder {
	b.cap.Roots = &RootCapabilities{
		ListChanged: listChanged,
	}
	return b
}

// WithToolsCapabilities sets the basic tools capabilities
func (b *ClientCapabilitiesBuilder) WithToolsCapabilities(execution bool) *ClientCapabilitiesBuilder {
	b.cap.Tools = &ToolsCapabilities{
		Execution: execution,
	}
	return b
}

// WithAdvancedToolsCapabilities sets advanced tools capabilities
func (b *ClientCapabilitiesBuilder) WithAdvancedToolsCapabilities(execution bool, listChanged bool, streaming bool, parallel bool) *ClientCapabilitiesBuilder {
	b.cap.Tools = &ToolsCapabilities{
		Execution:   execution,
		ListChanged: listChanged,
		Streaming:   streaming,
		Parallel:    parallel,
	}
	return b
}

// WithResourcesCapabilities sets the basic resources capabilities
func (b *ClientCapabilitiesBuilder) WithResourcesCapabilities(access bool, subscribe bool) *ClientCapabilitiesBuilder {
	b.cap.Resources = &ResourcesCapabilities{
		Access:    access,
		Subscribe: subscribe,
	}
	return b
}

// WithAdvancedResourcesCapabilities sets advanced resources capabilities
func (b *ClientCapabilitiesBuilder) WithAdvancedResourcesCapabilities(access bool, subscribe bool, listChanged bool, templates bool) *ClientCapabilitiesBuilder {
	b.cap.Resources = &ResourcesCapabilities{
		Access:      access,
		Subscribe:   subscribe,
		ListChanged: listChanged,
		Templates:   templates,
	}
	return b
}

// Build creates the ClientCapabilities instance
func (b *ClientCapabilitiesBuilder) Build() ClientCapabilities {
	return b.cap
}

// RootCapabilities represents the capabilities related to filesystem roots
type RootCapabilities struct {
	ListChanged bool `json:"listChanged"`
}

// SamplingCapabilities represents the capabilities related to LLM sampling
type SamplingCapabilities struct {
	Supported           bool     `json:"supported"`
	MaxTokens           int      `json:"maxTokens,omitempty"`
	StreamingSupported  bool     `json:"streamingSupported,omitempty"`
	TemperatureSupport  bool     `json:"temperatureSupport,omitempty"`
	SupportedModels     []string `json:"supportedModels,omitempty"`
	ContextInclusionMax int      `json:"contextInclusionMax,omitempty"`
}

// ServerCapabilities represents the capabilities of an MCP server
type ServerCapabilities struct {
	Experimental map[string]interface{} `json:"experimental,omitempty"`
	Logging      *LoggingCapabilities   `json:"logging,omitempty"`
	Prompts      *PromptCapabilities    `json:"prompts,omitempty"`
	Resources    *ResourcesCapabilities `json:"resources,omitempty"`
	Tools        *ToolsCapabilities     `json:"tools,omitempty"`
}

// ServerCapabilitiesBuilder allows for fluent construction of ServerCapabilities
type ServerCapabilitiesBuilder struct {
	capabilities ServerCapabilities
}

// NewServerCapabilitiesBuilder creates a new ServerCapabilitiesBuilder
func NewServerCapabilitiesBuilder() *ServerCapabilitiesBuilder {
	return &ServerCapabilitiesBuilder{
		capabilities: ServerCapabilities{},
	}
}

// Experimental sets the experimental capabilities
func (b *ServerCapabilitiesBuilder) Experimental(experimental map[string]interface{}) *ServerCapabilitiesBuilder {
	b.capabilities.Experimental = experimental
	return b
}

// Logging adds basic logging capabilities
func (b *ServerCapabilitiesBuilder) Logging() *ServerCapabilitiesBuilder {
	b.capabilities.Logging = &LoggingCapabilities{
		Supported: true,
	}
	return b
}

// LoggingWithOptions adds logging capabilities with custom configuration
func (b *ServerCapabilitiesBuilder) LoggingWithOptions(defaultLevel LogLevel, supportedLevels []LogLevel) *ServerCapabilitiesBuilder {
	b.capabilities.Logging = &LoggingCapabilities{
		Supported:       true,
		DefaultLevel:    defaultLevel,
		SupportedLevels: supportedLevels,
	}
	return b
}

// Prompts sets the prompts capabilities
func (b *ServerCapabilitiesBuilder) Prompts(access bool, listChanged bool) *ServerCapabilitiesBuilder {
	b.capabilities.Prompts = &PromptCapabilities{
		Access:      access,
		ListChanged: listChanged,
	}
	return b
}

// PromptsWithAdvanced sets the prompts capabilities with advanced options
func (b *ServerCapabilitiesBuilder) PromptsWithAdvanced(access bool, listChanged bool, create bool, update bool, delete bool) *ServerCapabilitiesBuilder {
	b.capabilities.Prompts = &PromptCapabilities{
		Access:      access,
		ListChanged: listChanged,
		Create:      create,
		Update:      update,
		Delete:      delete,
	}
	return b
}

// Resources sets the basic resources capabilities
func (b *ServerCapabilitiesBuilder) Resources(access bool, subscribe bool, listChanged bool) *ServerCapabilitiesBuilder {
	b.capabilities.Resources = &ResourcesCapabilities{
		Access:      access,
		Subscribe:   subscribe,
		ListChanged: listChanged,
	}
	return b
}

// ResourcesWithAdvanced sets the resources capabilities with advanced options
func (b *ServerCapabilitiesBuilder) ResourcesWithAdvanced(access bool, subscribe bool, listChanged bool, templates bool, create bool, update bool, delete bool) *ServerCapabilitiesBuilder {
	b.capabilities.Resources = &ResourcesCapabilities{
		Access:      access,
		Subscribe:   subscribe,
		ListChanged: listChanged,
		Templates:   templates,
		Create:      create,
		Update:      update,
		Delete:      delete,
	}
	return b
}

// Tools sets the basic tools capabilities
func (b *ServerCapabilitiesBuilder) Tools(execution bool, listChanged bool) *ServerCapabilitiesBuilder {
	b.capabilities.Tools = &ToolsCapabilities{
		Execution:   execution,
		ListChanged: listChanged,
	}
	return b
}

// ToolsWithAdvanced sets the tools capabilities with advanced options
func (b *ServerCapabilitiesBuilder) ToolsWithAdvanced(execution bool, listChanged bool, streaming bool, parallel bool) *ServerCapabilitiesBuilder {
	b.capabilities.Tools = &ToolsCapabilities{
		Execution:   execution,
		ListChanged: listChanged,
		Streaming:   streaming,
		Parallel:    parallel,
	}
	return b
}

// Build constructs the final ServerCapabilities object
func (b *ServerCapabilitiesBuilder) Build() ServerCapabilities {
	return b.capabilities
}

// LoggingCapabilities represents the capabilities related to logging
type LoggingCapabilities struct {
	Supported       bool       `json:"supported,omitempty"`
	DefaultLevel    LogLevel   `json:"defaultLevel,omitempty"`
	SupportedLevels []LogLevel `json:"supportedLevels,omitempty"`
}

// PromptCapabilities represents the capabilities related to prompts
type PromptCapabilities struct {
	Access      bool `json:"access,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
	Create      bool `json:"create,omitempty"`
	Update      bool `json:"update,omitempty"`
	Delete      bool `json:"delete,omitempty"`
}

// ResourcesCapabilities represents the capabilities related to resources
type ResourcesCapabilities struct {
	Access      bool `json:"access"`
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
	Templates   bool `json:"templates,omitempty"`
	Create      bool `json:"create,omitempty"`
	Update      bool `json:"update,omitempty"`
	Delete      bool `json:"delete,omitempty"`
}

// ToolsCapabilities represents the capabilities related to tools
type ToolsCapabilities struct {
	Execution   bool `json:"execution"`
	ListChanged bool `json:"listChanged,omitempty"`
	Streaming   bool `json:"streaming,omitempty"`
	Parallel    bool `json:"parallel,omitempty"`
}

// PromptsCapabilities represents the capabilities related to prompts (client side)
type PromptsCapabilities struct {
	Access      bool `json:"access"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// Root represents a resource root
type Root struct {
	URI  string `json:"uri"`
	Name string `json:"name,omitempty"`
}

// ListRootsResult represents the result of listing roots
type ListRootsResult struct {
	Roots []Root `json:"roots"`
}

// Annotations provides optional annotations for clients
type Annotations struct {
	Audience []Role  `json:"audience,omitempty"`
	Priority float64 `json:"priority,omitempty"`
}

// Tool represents a tool that can be executed by an MCP client
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// JsonSchema represents a JSON schema
type JsonSchema struct {
	Type                 string                 `json:"type"`
	Properties           map[string]interface{} `json:"properties,omitempty"`
	Required             []string               `json:"required,omitempty"`
	AdditionalProperties bool                   `json:"additionalProperties,omitempty"`
}

// ContentType defines the type of content
type ContentType string

const (
	// ContentTypeText represents text content
	ContentTypeText ContentType = "text"
	// ContentTypeImage represents image content
	ContentTypeImage ContentType = "image"
	// ContentTypeResource represents embedded resource content
	ContentTypeResource ContentType = "resource"
)

// Content is an interface for different types of content
type Content interface {
	GetType() ContentType
}

// BaseContent provides common fields for all content types
type BaseContent struct {
	Audience []Role  `json:"audience,omitempty"`
	Priority float64 `json:"priority,omitempty"`
}

// TextContent represents text content
type TextContent struct {
	BaseContent
	Text string `json:"text"`
}

// GetType returns the type of content
func (t *TextContent) GetType() ContentType {
	return ContentTypeText
}

// NewTextContent creates a new TextContent with the given text
func NewTextContent(text string) *TextContent {
	return &TextContent{Text: text}
}

// ImageContent represents image content
type ImageContent struct {
	BaseContent
	Data     string `json:"data"`
	MimeType string `json:"mimeType"`
}

// GetType returns the type of content
func (i *ImageContent) GetType() ContentType {
	return ContentTypeImage
}

// NewImageContent creates a new ImageContent with the given data and MIME type
func NewImageContent(data string, mimeType string) *ImageContent {
	return &ImageContent{Data: data, MimeType: mimeType}
}

// EmbeddedResource represents embedded resource content
type EmbeddedResource struct {
	BaseContent
	Resource ResourceContents `json:"resource"`
}

// GetType returns the type of content
func (e *EmbeddedResource) GetType() ContentType {
	return ContentTypeResource
}

// NewEmbeddedResource creates a new EmbeddedResource with the given ResourceContents
func NewEmbeddedResource(resource ResourceContents) *EmbeddedResource {
	return &EmbeddedResource{Resource: resource}
}

// MarshalJSON implements custom JSON marshaling for Content
func (t *TextContent) MarshalJSON() ([]byte, error) {
	type Alias TextContent
	return json.Marshal(&struct {
		Type string `json:"type"`
		*Alias
	}{
		Type:  string(ContentTypeText),
		Alias: (*Alias)(t),
	})
}

// MarshalJSON implements custom JSON marshaling for ImageContent
func (i *ImageContent) MarshalJSON() ([]byte, error) {
	type Alias ImageContent
	return json.Marshal(&struct {
		Type string `json:"type"`
		*Alias
	}{
		Type:  string(ContentTypeImage),
		Alias: (*Alias)(i),
	})
}

// MarshalJSON implements custom JSON marshaling for EmbeddedResource
func (e *EmbeddedResource) MarshalJSON() ([]byte, error) {
	type Alias EmbeddedResource
	return json.Marshal(&struct {
		Type string `json:"type"`
		*Alias
	}{
		Type:  string(ContentTypeResource),
		Alias: (*Alias)(e),
	})
}

// Resource represents a resource that can be accessed by an MCP client
type Resource struct {
	URI         string      `json:"uri"`
	Name        string      `json:"name,omitempty"`
	Description string      `json:"description,omitempty"`
	MimeType    string      `json:"mimeType,omitempty"`
	Annotations Annotations `json:"annotations,omitempty"`
}

// ResourceTemplate represents a template for parameterized resources
type ResourceTemplate struct {
	URITemplate string      `json:"uriTemplate"`
	Name        string      `json:"name,omitempty"`
	Description string      `json:"description,omitempty"`
	MimeType    string      `json:"mimeType,omitempty"`
	Annotations Annotations `json:"annotations,omitempty"`
}

// ListResourcesResult represents the result of listing resources
type ListResourcesResult struct {
	Resources  []Resource `json:"resources"`
	NextCursor string     `json:"nextCursor,omitempty"`
}

// ListResourceTemplatesResult represents the result of listing resource templates
type ListResourceTemplatesResult struct {
	ResourceTemplates []ResourceTemplate `json:"resourceTemplates"`
	NextCursor        string             `json:"nextCursor,omitempty"`
}

// ReadResourceRequest represents a request to read a resource
type ReadResourceRequest struct {
	URI string `json:"uri"`
}

// ReadResourceResult represents the result of reading a resource
type ReadResourceResult struct {
	Contents []ResourceContents `json:"contents"`
}

// SubscribeRequest represents a request to subscribe to changes in a resource
type SubscribeRequest struct {
	BaseRequest
	URI string `json:"uri"`
}

// UnsubscribeRequest represents a request to unsubscribe from changes in a resource
type UnsubscribeRequest struct {
	BaseRequest
	URI string `json:"uri"`
}

// ResourceContents is an interface for different types of resource contents
type ResourceContents interface {
	GetURI() string
	GetMimeType() string
	GetType() string
}

// ResourceContentsType defines the type of resource contents
type ResourceContentsType string

const (
	// ResourceContentsTypeText represents text resource contents
	ResourceContentsTypeText ResourceContentsType = "text"
	// ResourceContentsTypeBlob represents binary resource contents
	ResourceContentsTypeBlob ResourceContentsType = "blob"
)

// TextResourceContents represents text resource contents
type TextResourceContents struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
}

// GetURI returns the URI of the resource
func (t *TextResourceContents) GetURI() string {
	return t.URI
}

// GetMimeType returns the MIME type of the resource
func (t *TextResourceContents) GetMimeType() string {
	return t.MimeType
}

// GetType returns the type of resource content
func (t *TextResourceContents) GetType() string {
	return string(ResourceContentsTypeText)
}

// BlobResourceContents represents binary resource contents
type BlobResourceContents struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType"`
	Blob     string `json:"blob"`
}

// GetURI returns the URI of the resource
func (b *BlobResourceContents) GetURI() string {
	return b.URI
}

// GetMimeType returns the MIME type of the resource
func (b *BlobResourceContents) GetMimeType() string {
	return b.MimeType
}

// GetType returns the type of resource content
func (b *BlobResourceContents) GetType() string {
	return string(ResourceContentsTypeBlob)
}

// Prompt represents a prompt template that can be applied by an MCP client
type Prompt struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

// PromptArgument represents an argument for a prompt template
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// ListPromptsResult represents the result of listing prompts
type ListPromptsResult struct {
	Prompts    []Prompt `json:"prompts"`
	NextCursor string   `json:"nextCursor,omitempty"`
}

// GetPromptRequest represents a request to get a prompt
type GetPromptRequest struct {
	BaseRequest
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// GetPromptResult represents the result of getting a prompt
type GetPromptResult struct {
	Description string          `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages"`
}

// PromptMessage represents a message in a prompt result
type PromptMessage struct {
	Role    Role    `json:"role"`
	Content Content `json:"content"`
}

// Role defines the role of a message
type Role string

// Role constants
const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
	RoleTool      Role = "tool"
)

// CallToolRequest represents a request to call a tool
type CallToolRequest struct {
	BaseRequest
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// CallToolRequestBuilder provides a fluent builder for CallToolRequest
type CallToolRequestBuilder struct {
	request CallToolRequest
}

// NewCallToolRequestBuilder creates a new CallToolRequestBuilder
func NewCallToolRequestBuilder(name string) *CallToolRequestBuilder {
	return &CallToolRequestBuilder{
		request: CallToolRequest{
			Name:      name,
			Arguments: make(map[string]interface{}),
		},
	}
}

// Argument adds an argument to the request
func (b *CallToolRequestBuilder) Argument(name string, value interface{}) *CallToolRequestBuilder {
	b.request.Arguments[name] = value
	return b
}

// Arguments sets all arguments at once
func (b *CallToolRequestBuilder) Arguments(args map[string]interface{}) *CallToolRequestBuilder {
	b.request.Arguments = args
	return b
}

// Build constructs the final CallToolRequest object
func (b *CallToolRequestBuilder) Build() CallToolRequest {
	return b.request
}

// CallToolResult represents the result of calling a tool
type CallToolResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

// CallToolResultBuilder provides a fluent builder for CallToolResult
type CallToolResultBuilder struct {
	result CallToolResult
}

// NewCallToolResultBuilder creates a new CallToolResultBuilder
func NewCallToolResultBuilder() *CallToolResultBuilder {
	return &CallToolResultBuilder{
		result: CallToolResult{
			Content: []Content{},
		},
	}
}

// AddContent adds content to the result
func (b *CallToolResultBuilder) AddContent(content Content) *CallToolResultBuilder {
	b.result.Content = append(b.result.Content, content)
	return b
}

// IsError sets the error flag
func (b *CallToolResultBuilder) IsError(isError bool) *CallToolResultBuilder {
	b.result.IsError = isError
	return b
}

// Build constructs the final CallToolResult object
func (b *CallToolResultBuilder) Build() CallToolResult {
	return b.result
}

// CreateMessageRequest represents a request to create a message
type CreateMessageRequest struct {
	BaseRequest
	Messages         []SamplingMessage        `json:"messages,omitempty"`
	ModelPreferences *ModelPreferences        `json:"modelPreferences,omitempty"`
	SystemPrompt     string                   `json:"systemPrompt,omitempty"`
	IncludeContext   ContextInclusionStrategy `json:"includeContext,omitempty"`
	Temperature      float64                  `json:"temperature,omitempty"`
	MaxTokens        int                      `json:"maxTokens,omitempty"`
	StopSequences    []string                 `json:"stopSequences,omitempty"`
	Metadata         map[string]interface{}   `json:"metadata,omitempty"`
	Role             string                   `json:"role,omitempty"`
	Content          string                   `json:"content,omitempty"`
	ToolResponses    []ToolResponse           `json:"toolResponses,omitempty"`
}

// CreateMessageRequestBuilder provides a fluent builder for CreateMessageRequest
type CreateMessageRequestBuilder struct {
	request CreateMessageRequest
}

// NewCreateMessageRequestBuilder creates a new CreateMessageRequestBuilder
func NewCreateMessageRequestBuilder() *CreateMessageRequestBuilder {
	return &CreateMessageRequestBuilder{
		request: CreateMessageRequest{
			Messages:      []SamplingMessage{},
			StopSequences: []string{},
			ToolResponses: []ToolResponse{},
		},
	}
}

// AddMessage adds a message to the request
func (b *CreateMessageRequestBuilder) AddMessage(role Role, content Content) *CreateMessageRequestBuilder {
	b.request.Messages = append(b.request.Messages, SamplingMessage{
		Role:    role,
		Content: content,
	})
	return b
}

// ModelPreferences sets the model preferences
func (b *CreateMessageRequestBuilder) ModelPreferences(prefs *ModelPreferences) *CreateMessageRequestBuilder {
	b.request.ModelPreferences = prefs
	return b
}

// SystemPrompt sets the system prompt
func (b *CreateMessageRequestBuilder) SystemPrompt(systemPrompt string) *CreateMessageRequestBuilder {
	b.request.SystemPrompt = systemPrompt
	return b
}

// IncludeContext sets the context inclusion strategy
func (b *CreateMessageRequestBuilder) IncludeContext(strategy ContextInclusionStrategy) *CreateMessageRequestBuilder {
	b.request.IncludeContext = strategy
	return b
}

// Temperature sets the temperature
func (b *CreateMessageRequestBuilder) Temperature(temperature float64) *CreateMessageRequestBuilder {
	b.request.Temperature = temperature
	return b
}

// MaxTokens sets the maximum tokens
func (b *CreateMessageRequestBuilder) MaxTokens(maxTokens int) *CreateMessageRequestBuilder {
	b.request.MaxTokens = maxTokens
	return b
}

// AddStopSequence adds a stop sequence
func (b *CreateMessageRequestBuilder) AddStopSequence(stopSequence string) *CreateMessageRequestBuilder {
	b.request.StopSequences = append(b.request.StopSequences, stopSequence)
	return b
}

// Metadata sets the metadata
func (b *CreateMessageRequestBuilder) Metadata(metadata map[string]interface{}) *CreateMessageRequestBuilder {
	b.request.Metadata = metadata
	return b
}

// Role sets the role
func (b *CreateMessageRequestBuilder) Role(role string) *CreateMessageRequestBuilder {
	b.request.Role = role
	return b
}

// Content sets the content
func (b *CreateMessageRequestBuilder) Content(content string) *CreateMessageRequestBuilder {
	b.request.Content = content
	return b
}

// AddToolResponse adds a tool response
func (b *CreateMessageRequestBuilder) AddToolResponse(response ToolResponse) *CreateMessageRequestBuilder {
	b.request.ToolResponses = append(b.request.ToolResponses, response)
	return b
}

// Build constructs the final CreateMessageRequest object
func (b *CreateMessageRequestBuilder) Build() CreateMessageRequest {
	return b.request
}

// StopReason defines why a message creation stopped
type StopReason string

// Stop reason constants
const (
	StopReasonEndTurn      StopReason = "endTurn"
	StopReasonStopSequence StopReason = "stopSequence"
	StopReasonMaxTokens    StopReason = "maxTokens"
	StopReasonError        StopReason = "error"
	StopReasonCancelled    StopReason = "cancelled"
)

// CreateMessageResult represents the result of creating a message
type CreateMessageResult struct {
	Role       string                 `json:"role"`
	Content    string                 `json:"content"`
	ToolCalls  []ToolCall             `json:"toolCalls,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	Model      string                 `json:"model,omitempty"`
	StopReason StopReason             `json:"stopReason,omitempty"`
	Error      *McpError              `json:"error,omitempty"`
}

// CreateMessageResponse represents the response to a create message request
type CreateMessageResponse struct {
	Result *CreateMessageResult `json:"result,omitempty"`
	Error  *McpError            `json:"error,omitempty"`
}

// ToolCall represents a call to a tool
type ToolCall struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Params json.RawMessage `json:"params"`
}

// ToolResponse represents a response from a tool
type ToolResponse struct {
	CallID string          `json:"callId"`
	Result json.RawMessage `json:"result"`
	Error  *McpError       `json:"error,omitempty"`
}

// SetLevelRequest represents a request to set the logging level
type SetLevelRequest struct {
	Level LogLevel `json:"level"`
}

// LogLevel defines the level of a log message
type LogLevel string

// Log level constants
const (
	LogLevelDebug     LogLevel = "debug"
	LogLevelInfo      LogLevel = "info"
	LogLevelNotice    LogLevel = "notice"
	LogLevelWarning   LogLevel = "warning"
	LogLevelError     LogLevel = "error"
	LogLevelCritical  LogLevel = "critical"
	LogLevelAlert     LogLevel = "alert"
	LogLevelEmergency LogLevel = "emergency"
)

// LoggingMessage represents a logging message notification
type LoggingMessage struct {
	Level   LogLevel `json:"level"`
	Message string   `json:"message"`
	Logger  string   `json:"logger,omitempty"`
}

// InitializeRequest represents a request to initialize the protocol
type InitializeRequest struct {
	BaseRequest
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ClientCapabilities `json:"capabilities"`
	ClientInfo      Implementation     `json:"clientInfo"`
}

// InitializeResult represents the result of initializing the protocol
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      Implementation     `json:"serverInfo"`
	Instructions    string             `json:"instructions,omitempty"`
}

// ListToolsResult represents the result of listing tools
type ListToolsResult struct {
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// Request is a marker interface for all MCP requests
type Request interface {
	IsRequest() bool
}

// BaseRequest provides a common implementation of the Request interface
type BaseRequest struct{}

// IsRequest identifies this as an MCP request
func (b *BaseRequest) IsRequest() bool {
	return true
}

// SamplingMessage represents a message for sampling
type SamplingMessage struct {
	Role    Role    `json:"role"`
	Content Content `json:"content"`
}

// ContextInclusionStrategy defines how context should be included
type ContextInclusionStrategy string

// Context inclusion strategy constants
const (
	ContextInclusionStrategyAuto     ContextInclusionStrategy = "auto"
	ContextInclusionStrategyNone     ContextInclusionStrategy = "none"
	ContextInclusionStrategyExplicit ContextInclusionStrategy = "explicit"
)

// ModelPreferences represents preferences for model selection
type ModelPreferences struct {
	Hints                []ModelHint `json:"hints,omitempty"`
	CostPriority         float64     `json:"costPriority,omitempty"`
	SpeedPriority        float64     `json:"speedPriority,omitempty"`
	IntelligencePriority float64     `json:"intelligencePriority,omitempty"`
}

// ModelHint provides a hint for model selection
type ModelHint struct {
	Name string `json:"name"`
}

// ModelPreferencesBuilder allows for fluent construction of ModelPreferences
type ModelPreferencesBuilder struct {
	preferences ModelPreferences
}

// NewModelPreferencesBuilder creates a new ModelPreferencesBuilder
func NewModelPreferencesBuilder() *ModelPreferencesBuilder {
	return &ModelPreferencesBuilder{
		preferences: ModelPreferences{},
	}
}

// AddHint adds a model hint
func (b *ModelPreferencesBuilder) AddHint(name string) *ModelPreferencesBuilder {
	b.preferences.Hints = append(b.preferences.Hints, ModelHint{Name: name})
	return b
}

// CostPriority sets the cost priority
func (b *ModelPreferencesBuilder) CostPriority(priority float64) *ModelPreferencesBuilder {
	b.preferences.CostPriority = priority
	return b
}

// SpeedPriority sets the speed priority
func (b *ModelPreferencesBuilder) SpeedPriority(priority float64) *ModelPreferencesBuilder {
	b.preferences.SpeedPriority = priority
	return b
}

// IntelligencePriority sets the intelligence priority
func (b *ModelPreferencesBuilder) IntelligencePriority(priority float64) *ModelPreferencesBuilder {
	b.preferences.IntelligencePriority = priority
	return b
}

// Build constructs the final ModelPreferences object
func (b *ModelPreferencesBuilder) Build() ModelPreferences {
	return b.preferences
}

// ToolExecutionContext contains information about the tool execution environment
type ToolExecutionContext struct {
	ID         string                 `json:"id,omitempty"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
	Timeout    int                    `json:"timeout,omitempty"`
	MaxRetries int                    `json:"maxRetries,omitempty"`
}

// ToolExecutionContextBuilder creates a ToolExecutionContext
type ToolExecutionContextBuilder struct {
	context ToolExecutionContext
}

// NewToolExecutionContextBuilder creates a new builder for ToolExecutionContext
func NewToolExecutionContextBuilder() *ToolExecutionContextBuilder {
	return &ToolExecutionContextBuilder{
		context: ToolExecutionContext{
			Parameters: make(map[string]interface{}),
		},
	}
}

// WithID sets the ID of the tool execution
func (b *ToolExecutionContextBuilder) WithID(id string) *ToolExecutionContextBuilder {
	b.context.ID = id
	return b
}

// WithParameter adds a parameter to the tool execution context
func (b *ToolExecutionContextBuilder) WithParameter(key string, value interface{}) *ToolExecutionContextBuilder {
	b.context.Parameters[key] = value
	return b
}

// WithParameters sets all parameters for the tool execution context
func (b *ToolExecutionContextBuilder) WithParameters(params map[string]interface{}) *ToolExecutionContextBuilder {
	b.context.Parameters = params
	return b
}

// WithTimeout sets the timeout for the tool execution
func (b *ToolExecutionContextBuilder) WithTimeout(timeout int) *ToolExecutionContextBuilder {
	b.context.Timeout = timeout
	return b
}

// WithMaxRetries sets the maximum number of retries for the tool execution
func (b *ToolExecutionContextBuilder) WithMaxRetries(maxRetries int) *ToolExecutionContextBuilder {
	b.context.MaxRetries = maxRetries
	return b
}

// Build creates the ToolExecutionContext instance
func (b *ToolExecutionContextBuilder) Build() ToolExecutionContext {
	return b.context
}

// ToolResultStatus represents the status of a tool execution result
type ToolResultStatus string

const (
	// ToolResultStatusSuccess indicates a successful tool execution
	ToolResultStatusSuccess ToolResultStatus = "success"

	// ToolResultStatusError indicates an error during tool execution
	ToolResultStatusError ToolResultStatus = "error"

	// ToolResultStatusCancelled indicates a cancelled tool execution
	ToolResultStatusCancelled ToolResultStatus = "cancelled"
)

// ErrorDetails represents the error detail information
type ErrorDetails struct{}

// ToolResult represents the result of a tool execution
type ToolResult struct {
	ID           string                 `json:"id,omitempty"`
	Status       ToolResultStatus       `json:"status"`
	ContentType  string                 `json:"contentType,omitempty"`
	Content      interface{}            `json:"content,omitempty"`
	ErrorDetails *ErrorDetails          `json:"errorDetails,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// ToolResultBuilder creates a ToolResult
type ToolResultBuilder struct {
	result ToolResult
}

// NewToolResultBuilder creates a new builder for ToolResult
func NewToolResultBuilder() *ToolResultBuilder {
	return &ToolResultBuilder{
		result: ToolResult{
			Status:   ToolResultStatusSuccess,
			Metadata: make(map[string]interface{}),
		},
	}
}

// WithID sets the ID of the tool result
func (b *ToolResultBuilder) WithID(id string) *ToolResultBuilder {
	b.result.ID = id
	return b
}

// WithStatus sets the status of the tool result
func (b *ToolResultBuilder) WithStatus(status ToolResultStatus) *ToolResultBuilder {
	b.result.Status = status
	return b
}

// WithContentType sets the content type of the tool result
func (b *ToolResultBuilder) WithContentType(contentType string) *ToolResultBuilder {
	b.result.ContentType = contentType
	return b
}

// WithContent sets the content of the tool result
func (b *ToolResultBuilder) WithContent(content interface{}) *ToolResultBuilder {
	b.result.Content = content
	return b
}

// WithErrorDetails sets the error details of the tool result
func (b *ToolResultBuilder) WithErrorDetails(details *ErrorDetails) *ToolResultBuilder {
	b.result.ErrorDetails = details
	b.result.Status = ToolResultStatusError
	return b
}

// WithMetadata adds metadata to the tool result
func (b *ToolResultBuilder) WithMetadata(key string, value interface{}) *ToolResultBuilder {
	b.result.Metadata[key] = value
	return b
}

// Build creates the ToolResult instance
func (b *ToolResultBuilder) Build() ToolResult {
	return b.result
}

// TextAnnotation represents annotations on text content
type TextAnnotation struct {
	Start      int                    `json:"start"`
	End        int                    `json:"end"`
	Type       string                 `json:"type"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

// TextAnnotationBuilder creates a TextAnnotation
type TextAnnotationBuilder struct {
	annotation TextAnnotation
}

// NewTextAnnotationBuilder creates a new builder for TextAnnotation
func NewTextAnnotationBuilder() *TextAnnotationBuilder {
	return &TextAnnotationBuilder{
		annotation: TextAnnotation{
			Attributes: make(map[string]interface{}),
		},
	}
}

// WithRange sets the start and end position of the annotation
func (b *TextAnnotationBuilder) WithRange(start, end int) *TextAnnotationBuilder {
	b.annotation.Start = start
	b.annotation.End = end
	return b
}

// WithType sets the type of the annotation
func (b *TextAnnotationBuilder) WithType(annotationType string) *TextAnnotationBuilder {
	b.annotation.Type = annotationType
	return b
}

// WithAttribute adds an attribute to the annotation
func (b *TextAnnotationBuilder) WithAttribute(key string, value interface{}) *TextAnnotationBuilder {
	b.annotation.Attributes[key] = value
	return b
}

// Build creates the TextAnnotation instance
func (b *TextAnnotationBuilder) Build() TextAnnotation {
	return b.annotation
}

// TextAnnotations is a list of TextAnnotation with helper methods
type TextAnnotations []TextAnnotation

// NewTextAnnotations creates a new empty TextAnnotations list
func NewTextAnnotations() TextAnnotations {
	return TextAnnotations{}
}

// Add adds a new annotation to the list
func (annotations *TextAnnotations) Add(annotation TextAnnotation) {
	*annotations = append(*annotations, annotation)
}

// AddAll adds all annotations from another list
func (annotations *TextAnnotations) AddAll(other TextAnnotations) {
	*annotations = append(*annotations, other...)
}

// Size returns the number of annotations
func (annotations TextAnnotations) Size() int {
	return len(annotations)
}

// Get returns the annotation at the specified index
func (annotations TextAnnotations) Get(index int) TextAnnotation {
	return annotations[index]
}

// ToList returns the annotations as a standard Go slice
func (annotations TextAnnotations) ToList() []TextAnnotation {
	return annotations
}

// ContentRequest represents a request to create content asynchronously
type ContentRequest struct {
	BaseRequest
	RequestID        string                 `json:"requestId"`
	Messages         []SamplingMessage      `json:"messages,omitempty"`
	ModelPreferences *ModelPreferences      `json:"modelPreferences,omitempty"`
	SystemPrompt     string                 `json:"systemPrompt,omitempty"`
	IncludeContext   bool                   `json:"includeContext,omitempty"`
	Temperature      float64                `json:"temperature,omitempty"`
	MaxTokens        int                    `json:"maxTokens,omitempty"`
	StopSequences    []string               `json:"stopSequences,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// ContentEvent represents an event with content
type ContentEvent struct {
	RequestID string  `json:"requestId"`
	Content   Content `json:"content"`
}

// ContentCompleteEvent signals that content streaming is complete
type ContentCompleteEvent struct {
	RequestID string `json:"requestId"`
}

// ContentErrorEvent signals an error in content streaming
type ContentErrorEvent struct {
	RequestID string    `json:"requestId"`
	Error     *McpError `json:"error"`
}

// ToolExecutionRequest represents a request to execute a tool asynchronously
type ToolExecutionRequest struct {
	BaseRequest
	RequestID      string                 `json:"requestId"`
	ToolID         string                 `json:"toolId"`
	ToolName       string                 `json:"toolName"`
	ToolArguments  map[string]interface{} `json:"toolArguments"`
	ExecutionFlags []string               `json:"executionFlags,omitempty"`
}

// ToolExecutionResponse represents a response from tool execution
type ToolExecutionResponse struct {
	ToolResult *ToolResult `json:"toolResult"`
}

// ResourceRequest represents a request to access a resource asynchronously
type ResourceRequest struct {
	BaseRequest
	RequestID string `json:"requestId"`
	URI       string `json:"uri"`
}

// PromptRequest represents a request to access a prompt asynchronously
type PromptRequest struct {
	BaseRequest
	RequestID  string                 `json:"requestId"`
	Name       string                 `json:"name"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

// CloseSessionRequest represents a request to close the session
type CloseSessionRequest struct {
	BaseRequest
}

// CloseSessionResponse represents the response to close a session
type CloseSessionResponse struct {
	// No fields required for this response
}

// ErrorResponse represents an error response to a request
type ErrorResponse struct {
	Error *McpError `json:"error"`
}
