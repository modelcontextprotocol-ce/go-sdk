package client

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/pkg/spec"
	"github.com/modelcontextprotocol/go-sdk/pkg/util"
)

// syncClientImpl implements the McpSyncClient interface
type syncClientImpl struct {
	transport             spec.McpClientTransport
	requestTimeout        time.Duration
	initializationTimeout time.Duration
	features              *ClientFeatures
	session               spec.McpClientSession
	clientInfo            spec.Implementation
	serverInfo            spec.Implementation
	clientCapabilities    spec.ClientCapabilities
	serverCapabilities    spec.ServerCapabilities
	protocolVersion       string
	initialized           bool
	samplingHandler       SamplingHandler
}

// Initialize initializes the client connection and negotiates capabilities
func (c *syncClientImpl) Initialize() error {
	ctx, cancel := context.WithTimeout(context.Background(), c.initializationTimeout)
	defer cancel()

	// Create a new session if not already done
	if c.session == nil {
		session, err := spec.NewClientSession(c.transport)
		if err != nil {
			return err
		}
		c.session = session
	}

	// Create initialize request
	initRequest := &spec.InitializeRequest{
		ProtocolVersion: c.clientInfo.Version,
		Capabilities:    c.clientCapabilities,
		ClientInfo:      c.clientInfo,
	}

	// Send initialize request
	var result spec.InitializeResult
	err := c.session.SendRequest(ctx, spec.MethodInitialize, initRequest, &result)
	if err != nil {
		return err
	}

	// Store server information
	c.serverInfo = result.ServerInfo
	c.serverCapabilities = result.Capabilities
	c.protocolVersion = result.ProtocolVersion

	c.initialized = true
	return nil
}

// InitializeAsync initializes the client and connects to the server asynchronously.
func (c *syncClientImpl) InitializeAsync() chan error {
	errCh := make(chan error, 1)

	go func() {
		defer close(errCh)
		errCh <- c.Initialize()
	}()

	return errCh
}

// GetProtocolVersion returns the protocol version used by the server.
func (c *syncClientImpl) GetProtocolVersion() string {
	return c.protocolVersion
}

// GetServerInfo returns information about the server.
func (c *syncClientImpl) GetServerInfo() spec.Implementation {
	return c.serverInfo
}

// GetServerCapabilities returns the capabilities of the server.
func (c *syncClientImpl) GetServerCapabilities() spec.ServerCapabilities {
	return c.serverCapabilities
}

// GetClientCapabilities returns the capabilities of the client.
func (c *syncClientImpl) GetClientCapabilities() spec.ClientCapabilities {
	return c.clientCapabilities
}

// GetPrompt gets a prompt from the server with the given arguments.
func (c *syncClientImpl) GetPrompt(name string, args map[string]interface{}) (*spec.GetPromptResult, error) {
	if !c.initialized {
		return nil, errors.New("client not initialized")
	}

	util.AssertNotEmpty(name, "Prompt name must not be empty")

	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	requestParams := &spec.GetPromptRequest{
		Name:      name,
		Arguments: args,
	}

	var result spec.GetPromptResult
	err := c.session.SendRequest(ctx, spec.MethodPromptGet, requestParams, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// Close terminates the client connection and releases resources
func (c *syncClientImpl) Close() error {
	if c.session != nil {
		return c.session.Close()
	}
	return nil
}

// CloseGracefully terminates the client connection gracefully,
// waiting for pending operations to complete
func (c *syncClientImpl) CloseGracefully(ctx context.Context) error {
	if c.session != nil {
		// Check if the session implements the CloseGracefully method
		if closer, ok := c.session.(interface{ CloseGracefully(context.Context) error }); ok {
			return closer.CloseGracefully(ctx)
		}
		// Fall back to regular close if graceful close is not available
		return c.session.Close()
	}
	return nil
}

// GetTools retrieves the list of available tools from the server
func (c *syncClientImpl) GetTools() ([]spec.Tool, error) {
	if !c.initialized {
		return nil, errors.New("client not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	var result spec.ListToolsResult
	err := c.session.SendRequest(ctx, spec.MethodToolsList, nil, &result)
	if err != nil {
		return nil, err
	}

	// Notify handlers about the tools
	for _, handler := range c.features.ToolsChangeHandlers {
		handler(result.Tools)
	}

	return result.Tools, nil
}

// ExecuteTool executes a tool on the server with the given parameters
func (c *syncClientImpl) ExecuteTool(name string, params interface{}, resultPtr interface{}) error {
	if !c.initialized {
		return errors.New("client not initialized")
	}

	util.AssertNotEmpty(name, "Tool name must not be empty")

	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	// Create the request payload
	requestParams := &spec.CallToolRequest{
		Name:      name,
		Arguments: util.ToMap(params),
	}

	// Create a variable to hold the result
	var callResult spec.CallToolResult
	err := c.session.SendRequest(ctx, spec.MethodToolsCall, requestParams, &callResult)
	if err != nil {
		return err
	}

	// Unmarshal the result into the provided pointer
	return util.ToInterface(callResult, resultPtr)
}

// ExecuteToolStream executes a streaming tool on the server.
func (c *syncClientImpl) ExecuteToolStream(ctx context.Context, name string, params interface{}) (chan []spec.Content, chan error) {
	contentCh := make(chan []spec.Content, 10) // Buffer a few content updates
	errCh := make(chan error, 1)

	if !c.initialized {
		errCh <- errors.New("client not initialized")
		close(contentCh)
		return contentCh, errCh
	}

	util.AssertNotEmpty(name, "Tool name must not be empty")

	// If no context is provided, create one with default timeout
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), c.requestTimeout)
		defer cancel()
	}

	// Copy params and add streaming flag
	paramsCopy := make(map[string]interface{})
	if paramsMap := util.ToMap(params); paramsMap != nil {
		for k, v := range paramsMap {
			paramsCopy[k] = v
		}
	}
	paramsCopy["_streaming"] = true

	// Create the request payload
	requestParams := &spec.CallToolRequest{
		Name:      name,
		Arguments: paramsCopy,
	}

	// Register a handler for streaming tool results
	// This needs to be done before sending the request to handle all notifications
	streamHandler := func(streamResult *spec.StreamingToolResult) {
		if streamResult.Error != nil {
			select {
			case errCh <- streamResult.Error:
			default:
				// Error channel might be closed already
			}
			return
		}

		if streamResult.IsFinal {
			close(contentCh)
			close(errCh)
			return
		}

		// Send content if available
		if len(streamResult.Content) > 0 {
			select {
			case contentCh <- streamResult.Content:
			case <-ctx.Done():
				// Context cancelled
			}
		}
	}

	// Create a variable to hold the initial result
	var callResult spec.CallToolResult
	err := c.session.SendRequest(ctx, spec.MethodToolsCall, requestParams, &callResult)
	if err != nil {
		errCh <- err
		close(contentCh)
		return contentCh, errCh
	}

	// Check if streaming was initialized successfully
	if !callResult.IsStreaming || callResult.StreamID == "" {
		errCh <- fmt.Errorf("server does not support streaming for tool %s", name)
		close(contentCh)
		return contentCh, errCh
	}

	// Register handler for streaming notifications with this streamID
	c.session.RegisterStreamHandler(callResult.StreamID, streamHandler)

	// Start a goroutine to handle context cancellation
	go func() {
		<-ctx.Done()
		// If context is done, make sure channels are closed properly
		select {
		case errCh <- ctx.Err():
			// Error sent successfully
		default:
			// Channel might be closed already
		}

		c.session.UnregisterStreamHandler(callResult.StreamID)
	}()

	return contentCh, errCh
}

// GetResources retrieves the list of available resources from the server
func (c *syncClientImpl) GetResources() ([]spec.Resource, error) {
	if !c.initialized {
		return nil, errors.New("client not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	var result spec.ListResourcesResult
	err := c.session.SendRequest(ctx, spec.MethodResourcesList, nil, &result)
	if err != nil {
		return nil, err
	}

	// Notify handlers about the resources
	for _, handler := range c.features.ResourcesChangeHandlers {
		handler(result.Resources)
	}

	return result.Resources, nil
}

// ReadResource reads a resource from the server
func (c *syncClientImpl) ReadResource(uri string) ([]spec.ResourceContents, error) {
	if !c.initialized {
		return nil, errors.New("client not initialized")
	}

	util.AssertNotEmpty(uri, "Resource URI must not be empty")

	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	requestParams := &spec.ReadResourceRequest{
		URI: uri,
	}

	var result spec.ReadResourceResult
	err := c.session.SendRequest(ctx, spec.MethodResourcesRead, requestParams, &result)
	if err != nil {
		return nil, err
	}

	return result.Contents, nil
}

// GetResourceTemplates returns the list of resource templates provided by the server.
func (c *syncClientImpl) GetResourceTemplates() ([]spec.ResourceTemplate, error) {
	if !c.initialized {
		return nil, errors.New("client not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	var result spec.ListResourceTemplatesResult
	err := c.session.SendRequest(ctx, spec.MethodResourcesTemplatesList, nil, &result)
	if err != nil {
		return nil, err
	}

	return result.ResourceTemplates, nil
}

// SubscribeToResource subscribes to changes in a resource.
func (c *syncClientImpl) SubscribeToResource(uri string) error {
	if !c.initialized {
		return errors.New("client not initialized")
	}

	util.AssertNotEmpty(uri, "Resource URI must not be empty")

	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	requestParams := &spec.SubscribeRequest{
		URI: uri,
	}

	return c.session.SendRequest(ctx, spec.MethodResourcesSubscribe, requestParams, nil)
}

// UnsubscribeFromResource unsubscribes from changes in a resource.
func (c *syncClientImpl) UnsubscribeFromResource(uri string) error {
	if !c.initialized {
		return errors.New("client not initialized")

	}

	util.AssertNotEmpty(uri, "Resource URI must not be empty")

	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	requestParams := &spec.UnsubscribeRequest{
		URI: uri,
	}

	return c.session.SendRequest(ctx, spec.MethodResourcesUnsubscribe, requestParams, nil)
}

// GetPrompts retrieves the list of available prompt templates from the server
func (c *syncClientImpl) GetPrompts() ([]spec.Prompt, error) {
	if !c.initialized {
		return nil, errors.New("client not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	var result spec.ListPromptsResult
	err := c.session.SendRequest(ctx, spec.MethodPromptList, nil, &result)
	if err != nil {
		return nil, err
	}

	// Notify handlers about the prompts
	for _, handler := range c.features.PromptsChangeHandlers {
		handler(result.Prompts)
	}

	return result.Prompts, nil
}

// CreateMessage creates a new message for the conversation
func (c *syncClientImpl) CreateMessage(request *spec.CreateMessageRequest) (*spec.CreateMessageResult, error) {
	if !c.initialized {
		return nil, errors.New("client not initialized")
	}

	// Apply sampling if a handler is configured
	if c.features.SamplingHandler != nil {
		return c.features.SamplingHandler.CreateMessage(request)
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	var result spec.CreateMessageResult
	err := c.session.SendRequest(ctx, spec.MethodSamplingCreateMessage, request, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// CreateMessageStream sends a create message request to the server and streams the responses.
// The returned channels will receive partial results and errors as they become available.
func (c *syncClientImpl) CreateMessageStream(ctx context.Context, request *spec.CreateMessageRequest) (<-chan *spec.CreateMessageResult, <-chan error) {
	resultCh := make(chan *spec.CreateMessageResult)
	errCh := make(chan error, 1)

	if !c.initialized {
		errCh <- errors.New("client not initialized")
		close(resultCh)
		return resultCh, errCh
	}

	// Apply sampling if a handler is configured
	if c.features.SamplingHandler != nil {
		// For now, we don't support streaming with the sampling handler
		// In the future, we could extend SamplingHandler to support streaming
		go func() {
			defer close(resultCh)
			defer close(errCh)

			result, err := c.features.SamplingHandler.CreateMessage(request)
			if err != nil {
				errCh <- err
				return
			}
			resultCh <- result
		}()
		return resultCh, errCh
	}

	// Check if server supports streaming
	supportsStreaming := false
	if c.serverCapabilities.Tools != nil {
		supportsStreaming = c.serverCapabilities.Tools.Streaming
	}

	if !supportsStreaming {
		// Fall back to non-streaming implementation
		go func() {
			defer close(resultCh)
			defer close(errCh)

			result, err := c.CreateMessage(request)
			if err != nil {
				errCh <- err
				return
			}
			resultCh <- result
		}()
		return resultCh, errCh
	}

	// If streaming is supported, set up a streaming request
	go func() {
		defer close(resultCh)
		defer close(errCh)

		// Copy the request and add streaming flag
		streamingRequest := *request
		if streamingRequest.Metadata == nil {
			streamingRequest.Metadata = make(map[string]interface{})
		}
		streamingRequest.Metadata["streaming"] = true

		// Register a streaming handler for this request
		requestID := util.GenerateUUID()
		streamingRequest.Metadata["requestId"] = requestID

		// Create a message handler to process streaming results
		handler := func(message *spec.CreateMessageResult) {
			select {
			case resultCh <- message:
				// Successfully sent message
			case <-ctx.Done():
				// Context canceled, stop processing
				return
			}
		}

		// Register the streaming handler
		c.session.SetStreamingHandler(requestID, handler)
		defer c.session.RemoveStreamingHandler(requestID)

		// Send the request
		var result spec.CreateMessageResult
		err := c.session.SendRequest(ctx, spec.MethodSamplingCreateMessageStream, &streamingRequest, &result)
		if err != nil {
			errCh <- err
			return
		}

		// The initial result may also be sent here
		resultCh <- &result
	}()

	return resultCh, errCh
}

// SetLoggingLevel sets the minimum level for logs from the server.
func (c *syncClientImpl) SetLoggingLevel(level spec.LogLevel) error {
	if !c.initialized {
		return errors.New("client not initialized")
	}

	util.AssertNotEmpty(string(level), "Log level must not be empty")

	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	requestParams := &spec.SetLevelRequest{
		Level: level,
	}

	return c.session.SendRequest(ctx, spec.MethodLoggingSetLevel, requestParams, nil)
}

// Ping sends a ping to the server to check connectivity.
func (c *syncClientImpl) Ping() error {
	if !c.initialized {
		return errors.New("client not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	return c.session.SendRequest(ctx, spec.MethodPing, nil, nil)
}
