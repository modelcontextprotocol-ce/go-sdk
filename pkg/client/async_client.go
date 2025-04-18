package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/pkg/spec"
	"github.com/modelcontextprotocol/go-sdk/pkg/util"
)

// asyncClientImpl implements the McpAsyncClient interface
type asyncClientImpl struct {
	transport             spec.McpClientTransport
	requestTimeout        time.Duration
	initializationTimeout time.Duration
	clientInfo            spec.Implementation
	clientCapabilities    spec.ClientCapabilities
	serverInfo            spec.Implementation
	serverCapabilities    spec.ServerCapabilities
	protocolVersion       string
	features              *ClientFeatures
	asyncSamplingHandler  AsyncSamplingHandler
	session               spec.McpClientSession
	initialized           bool
}

// newAsyncClient creates a new instance of asyncClientImpl with properly initialized fields
func newAsyncClient(transport spec.McpClientTransport, config McpClientConfig, samplingHandler AsyncSamplingHandler) *asyncClientImpl {
	return &asyncClientImpl{
		transport:             transport,
		requestTimeout:        config.RequestTimeout,
		initializationTimeout: config.InitializationTimeout,
		clientInfo:            config.ClientInfo,
		clientCapabilities:    config.ClientCapabilities,
		asyncSamplingHandler:  samplingHandler,
		features: &ClientFeatures{
			ClientInfo:   config.ClientInfo,
			Capabilities: config.ClientCapabilities,
			Roots:        makeRootsMap(config.Roots),
			ToolsChangeHandlers: []ToolsChangeHandler{
				config.ToolsChangeHandler,
			},
			ResourcesChangeHandlers: []ResourcesChangeHandler{
				config.ResourcesChangeHandler,
			},
			ResourceChangeHandlers: []ResourceChangeHandler{},
			PromptsChangeHandlers: []PromptsChangeHandler{
				config.PromptsChangeHandler,
			},
			LoggingHandlers: []LoggingHandler{
				config.LoggingHandler,
			},
		},
		initialized: false,
	}
}

// Initialize initializes the client and connects to the server.
func (c *asyncClientImpl) Initialize() error {
	ctx, cancel := context.WithTimeout(context.Background(), c.initializationTimeout)
	defer cancel()

	errCh := c.InitializeAsync()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// InitializeAsync initializes the client and connects to the server asynchronously.
func (c *asyncClientImpl) InitializeAsync() chan error {
	errCh := make(chan error, 1)

	go func() {
		defer close(errCh)

		// Create a new session if not already done
		if c.session == nil {
			session, err := spec.NewClientSession(c.transport)
			if err != nil {
				errCh <- err
				return
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
		ctx, cancel := context.WithTimeout(context.Background(), c.initializationTimeout)
		defer cancel()

		var result spec.InitializeResult
		err := c.session.SendRequest(ctx, spec.MethodInitialize, initRequest, &result)
		if err != nil {
			errCh <- err
			return
		}

		// Store server information
		c.serverInfo = result.ServerInfo
		c.serverCapabilities = result.Capabilities
		c.protocolVersion = result.ProtocolVersion

		// Register notification handlers for server-initiated notifications
		c.registerNotificationHandlers()

		c.initialized = true
		errCh <- nil
	}()

	return errCh
}

// registerNotificationHandlers sets up handlers for various notifications from the server
func (c *asyncClientImpl) registerNotificationHandlers() {
	if c.session == nil {
		return
	}

	// Register handler for tools list changes
	c.session.SetNotificationHandler(spec.MethodNotificationToolsListChanged, func(ctx context.Context, params json.RawMessage) error {
		// Refresh tools list
		go func() {
			tools, _ := c.GetTools()
			for _, handler := range c.features.ToolsChangeHandlers {
				handler(tools)
			}
		}()
		return nil
	})

	// Register handler for resources list changes
	c.session.SetNotificationHandler(spec.MethodNotificationResourcesListChanged, func(ctx context.Context, params json.RawMessage) error {
		// Refresh resources list
		go func() {
			resources, _ := c.GetResources()
			for _, handler := range c.features.ResourcesChangeHandlers {
				handler(resources)
			}
		}()
		return nil
	})

	// Register handler for prompts list changes
	c.session.SetNotificationHandler(spec.MethodNotificationPromptsListChanged, func(ctx context.Context, params json.RawMessage) error {
		// Refresh prompts list
		go func() {
			prompts, _ := c.GetPrompts()
			for _, handler := range c.features.PromptsChangeHandlers {
				handler(prompts)
			}
		}()
		return nil
	})

	// Register handler for resource changes
	c.session.SetNotificationHandler(spec.MethodNotificationResourceChanged, func(ctx context.Context, params json.RawMessage) error {
		var notification struct {
			URI      string          `json:"uri"`
			Contents json.RawMessage `json:"contents"`
		}

		if err := json.Unmarshal(params, &notification); err != nil {
			return err
		}

		// Notify resource change handlers
		for _, handler := range c.features.ResourceChangeHandlers {
			if handler.URI == notification.URI {
				var contents []spec.ResourceContents
				if err := json.Unmarshal(notification.Contents, &contents); err != nil {
					return err
				}
				handler.Callback(contents)
			}
		}

		return nil
	})

	// Register handler for log messages
	c.session.SetNotificationHandler(spec.MethodNotificationMessage, func(ctx context.Context, params json.RawMessage) error {
		var logMsg spec.LoggingMessage
		if err := json.Unmarshal(params, &logMsg); err != nil {
			return err
		}

		// Notify logging handlers
		for _, handler := range c.features.LoggingHandlers {
			handler(logMsg)
		}

		return nil
	})

	// Add root change notification handler
	c.registerRootsChangeNotificationHandler()
}

// Close closes the client and releases any resources.
func (c *asyncClientImpl) Close() error {
	if c.session != nil {
		return c.session.Close()
	}
	return nil
}

// CloseGracefully closes the client gracefully, waiting for pending operations to complete.
func (c *asyncClientImpl) CloseGracefully(ctx context.Context) error {
	if c.session != nil {
		return c.session.CloseGracefully(ctx)
	}
	return nil
}

// GetProtocolVersion returns the protocol version used by the server.
func (c *asyncClientImpl) GetProtocolVersion() string {
	return c.protocolVersion
}

// GetServerInfo returns information about the server.
func (c *asyncClientImpl) GetServerInfo() spec.Implementation {
	return c.serverInfo
}

// GetServerCapabilities returns the capabilities of the server.
func (c *asyncClientImpl) GetServerCapabilities() spec.ServerCapabilities {
	return c.serverCapabilities
}

// GetClientCapabilities returns the capabilities of the client.
func (c *asyncClientImpl) GetClientCapabilities() spec.ClientCapabilities {
	return c.clientCapabilities
}

// GetTools returns the list of tools provided by the server.
func (c *asyncClientImpl) GetTools() ([]spec.Tool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	resultCh, errCh := c.GetToolsAsync(ctx)

	select {
	case result := <-resultCh:
		return result, nil
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// GetToolsAsync returns the list of tools provided by the server asynchronously.
func (c *asyncClientImpl) GetToolsAsync(ctx context.Context) (chan []spec.Tool, chan error) {
	resultCh := make(chan []spec.Tool, 1)
	errCh := make(chan error, 1)

	if !c.initialized {
		errCh <- errors.New("client not initialized")
		close(resultCh)
		return resultCh, errCh
	}

	go func() {
		defer close(resultCh)
		defer close(errCh)

		var result spec.ListToolsResult
		err := c.session.SendRequest(ctx, spec.MethodToolsList, nil, &result)
		if err != nil {
			errCh <- err
			return
		}

		// Notify handlers about the tools
		for _, handler := range c.features.ToolsChangeHandlers {
			handler(result.Tools)
		}

		resultCh <- result.Tools
	}()

	return resultCh, errCh
}

// ExecuteTool executes a tool on the server.
func (c *asyncClientImpl) ExecuteTool(name string, params interface{}, resultPtr interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	resultCh, errCh := c.ExecuteToolAsync(ctx, name, params, reflect.TypeOf(resultPtr).Elem())

	select {
	case result := <-resultCh:
		// Use reflection to set the pointer value
		resultValue := reflect.ValueOf(resultPtr).Elem()
		resultValue.Set(reflect.ValueOf(result).Elem())
		return nil
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ExecuteToolAsync executes a tool on the server asynchronously.
func (c *asyncClientImpl) ExecuteToolAsync(ctx context.Context, name string, params interface{}, resultType interface{}) (chan interface{}, chan error) {
	resultCh := make(chan interface{}, 1)
	errCh := make(chan error, 1)

	if !c.initialized {
		errCh <- errors.New("client not initialized")
		close(resultCh)
		return resultCh, errCh
	}

	util.AssertNotEmpty(name, "Tool name must not be empty")

	go func() {
		defer close(resultCh)
		defer close(errCh)

		// Create the request payload
		requestParams := &spec.CallToolRequest{
			Name:      name,
			Arguments: util.ToMap(params),
		}

		// Create a variable to hold the result
		var callResult spec.CallToolResult
		err := c.session.SendRequest(ctx, spec.MethodToolsCall, requestParams, &callResult)
		if err != nil {
			errCh <- err
			return
		}

		// Create a new instance of the expected result type
		newResult := reflect.New(reflect.TypeOf(resultType).Elem())

		// Unmarshal the result into the created instance
		err = util.ToInterface(callResult, &newResult)
		if err != nil {
			errCh <- err
			return
		}

		resultCh <- newResult.Interface()
	}()

	return resultCh, errCh
}

// ExecuteToolStream executes a streaming tool on the server.
// It returns a channel that receives content as it becomes available and a channel for errors.
// The contents channel will be closed when streaming is complete.
func (c *asyncClientImpl) ExecuteToolStream(ctx context.Context, name string, params interface{}) (chan []spec.Content, chan error) {
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

	go func() {
		// Handle any panics
		defer func() {
			if r := recover(); r != nil {
				errCh <- fmt.Errorf("panic in ExecuteToolStream: %v", r)
				close(contentCh)
				close(errCh)
			}
		}()

		// Create a variable to hold the initial result
		var callResult spec.CallToolResult
		err := c.session.SendRequest(ctx, spec.MethodToolsCall, requestParams, &callResult)
		if err != nil {
			errCh <- err
			close(contentCh)
			return
		}

		// Check if streaming was initialized successfully
		if !callResult.IsStreaming || callResult.StreamID == "" {
			errCh <- fmt.Errorf("server does not support streaming for tool %s", name)
			close(contentCh)
			return
		}

		// Register handler for streaming notifications with this streamID
		c.session.RegisterStreamHandler(callResult.StreamID, streamHandler)

		// Return any initial content provided in the response
		if len(callResult.Content) > 0 {
			contentCh <- callResult.Content
		}

		// Handle context cancellation
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

// GetResources returns the list of resources provided by the server.
func (c *asyncClientImpl) GetResources() ([]spec.Resource, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	resultCh, errCh := c.GetResourcesAsync(ctx)

	select {
	case result := <-resultCh:
		return result, nil
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// GetResourcesAsync returns the list of resources provided by the server asynchronously.
func (c *asyncClientImpl) GetResourcesAsync(ctx context.Context) (chan []spec.Resource, chan error) {
	resultCh := make(chan []spec.Resource, 1)
	errCh := make(chan error, 1)

	if !c.initialized {
		errCh <- errors.New("client not initialized")
		close(resultCh)
		return resultCh, errCh
	}

	go func() {
		defer close(resultCh)
		defer close(errCh)

		var result spec.ListResourcesResult
		err := c.session.SendRequest(ctx, spec.MethodResourcesList, nil, &result)
		if err != nil {
			errCh <- err
			return
		}

		// Notify handlers about the resources
		for _, handler := range c.features.ResourcesChangeHandlers {
			handler(result.Resources)
		}

		resultCh <- result.Resources
	}()

	return resultCh, errCh
}

// ReadResource reads a resource from the server.
func (c *asyncClientImpl) ReadResource(uri string) ([]spec.ResourceContents, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	resultCh, errCh := c.ReadResourceAsync(ctx, uri)

	select {
	case result := <-resultCh:
		return result, nil
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ReadResourceAsync reads a resource from the server asynchronously.
func (c *asyncClientImpl) ReadResourceAsync(ctx context.Context, uri string) (chan []spec.ResourceContents, chan error) {
	resultCh := make(chan []spec.ResourceContents, 1)
	errCh := make(chan error, 1)

	if !c.initialized {
		errCh <- errors.New("client not initialized")
		close(resultCh)
		return resultCh, errCh
	}

	util.AssertNotEmpty(uri, "Resource URI must not be empty")

	go func() {
		defer close(resultCh)
		defer close(errCh)

		// Create request payload
		requestParams := &spec.ReadResourceRequest{
			URI: uri,
		}

		// Send the request
		var result spec.ReadResourceResult
		err := c.session.SendRequest(ctx, spec.MethodResourcesRead, requestParams, &result)
		if err != nil {
			errCh <- err
			return
		}

		resultCh <- result.Contents
	}()

	return resultCh, errCh
}

// CreateResource creates a new resource on the server.
func (c *asyncClientImpl) CreateResource(resource spec.Resource, contents []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	errCh := c.CreateResourceAsync(ctx, resource, contents)

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// CreateResourceAsync creates a new resource on the server asynchronously.
func (c *asyncClientImpl) CreateResourceAsync(ctx context.Context, resource spec.Resource, contents []byte) chan error {
	errCh := make(chan error, 1)

	if !c.initialized {
		errCh <- errors.New("client not initialized")
		close(errCh)
		return errCh
	}

	util.AssertNotNil(resource, "Resource must not be nil")
	util.AssertNotEmpty(resource.URI, "Resource URI must not be empty")

	go func() {
		defer close(errCh)

		// Create request payload
		requestParams := &spec.CreateResourceRequest{
			Resource: resource,
			Contents: contents,
		}

		var result spec.CreateResourceResult
		errCh <- c.session.SendRequest(ctx, spec.MethodResourcesCreate, requestParams, &result)
	}()

	return errCh
}

// UpdateResource updates an existing resource on the server.
func (c *asyncClientImpl) UpdateResource(resource spec.Resource, contents []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	errCh := c.UpdateResourceAsync(ctx, resource, contents)

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// UpdateResourceAsync updates an existing resource on the server asynchronously.
func (c *asyncClientImpl) UpdateResourceAsync(ctx context.Context, resource spec.Resource, contents []byte) chan error {
	errCh := make(chan error, 1)

	if !c.initialized {
		errCh <- errors.New("client not initialized")
		close(errCh)
		return errCh
	}

	util.AssertNotNil(resource, "Resource must not be nil")
	util.AssertNotEmpty(resource.URI, "Resource URI must not be empty")

	go func() {
		defer close(errCh)

		// Create request payload
		requestParams := &spec.UpdateResourceRequest{
			Resource: resource,
			Contents: contents,
		}

		var result spec.UpdateResourceResult
		errCh <- c.session.SendRequest(ctx, spec.MethodResourcesUpdate, requestParams, &result)
	}()

	return errCh
}

// DeleteResource deletes a resource from the server.
func (c *asyncClientImpl) DeleteResource(uri string) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	errCh := c.DeleteResourceAsync(ctx, uri)

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// DeleteResourceAsync deletes a resource from the server asynchronously.
func (c *asyncClientImpl) DeleteResourceAsync(ctx context.Context, uri string) chan error {
	errCh := make(chan error, 1)

	if !c.initialized {
		errCh <- errors.New("client not initialized")
		close(errCh)
		return errCh
	}

	util.AssertNotEmpty(uri, "Resource URI must not be empty")

	go func() {
		defer close(errCh)

		// Create request payload
		requestParams := &spec.DeleteResourceRequest{
			URI: uri,
		}

		var result spec.DeleteResourceResult
		errCh <- c.session.SendRequest(ctx, spec.MethodResourcesDelete, requestParams, &result)
	}()

	return errCh
}

// GetResourceTemplates returns the list of resource templates provided by the server.
func (c *asyncClientImpl) GetResourceTemplates() ([]spec.ResourceTemplate, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	resultCh, errCh := c.GetResourceTemplatesAsync(ctx)

	select {
	case result := <-resultCh:
		return result, nil
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// GetResourceTemplatesAsync returns the list of resource templates provided by the server asynchronously.
func (c *asyncClientImpl) GetResourceTemplatesAsync(ctx context.Context) (chan []spec.ResourceTemplate, chan error) {
	resultCh := make(chan []spec.ResourceTemplate, 1)
	errCh := make(chan error, 1)

	if !c.initialized {
		errCh <- errors.New("client not initialized")
		close(resultCh)
		return resultCh, errCh
	}

	go func() {
		defer close(resultCh)
		defer close(errCh)

		var result spec.ListResourceTemplatesResult
		err := c.session.SendRequest(ctx, spec.MethodResourcesTemplatesList, nil, &result)
		if err != nil {
			errCh <- err
			return
		}

		resultCh <- result.ResourceTemplates
	}()

	return resultCh, errCh
}

// SubscribeToResource subscribes to changes in a resource.
func (c *asyncClientImpl) SubscribeToResource(uri string) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	errCh := c.SubscribeToResourceAsync(ctx, uri)

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SubscribeToResourceAsync subscribes to changes in a resource asynchronously.
func (c *asyncClientImpl) SubscribeToResourceAsync(ctx context.Context, uri string) chan error {
	errCh := make(chan error, 1)

	if !c.initialized {
		errCh <- errors.New("client not initialized")
		close(errCh)
		return errCh
	}

	util.AssertNotEmpty(uri, "Resource URI must not be empty")

	go func() {
		defer close(errCh)

		// Create request payload
		requestParams := &spec.SubscribeRequest{
			URI: uri,
		}

		// Send the request
		errCh <- c.session.SendRequest(ctx, spec.MethodResourcesSubscribe, requestParams, nil)
	}()

	return errCh
}

// UnsubscribeFromResource unsubscribes from changes in a resource.
func (c *asyncClientImpl) UnsubscribeFromResource(uri string) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	errCh := c.UnsubscribeFromResourceAsync(ctx, uri)

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// UnsubscribeFromResourceAsync unsubscribes from changes in a resource asynchronously.
func (c *asyncClientImpl) UnsubscribeFromResourceAsync(ctx context.Context, uri string) chan error {
	errCh := make(chan error, 1)

	if !c.initialized {
		errCh <- errors.New("client not initialized")
		close(errCh)
		return errCh
	}

	util.AssertNotEmpty(uri, "Resource URI must not be empty")

	go func() {
		defer close(errCh)

		// Create request payload
		requestParams := &spec.UnsubscribeRequest{
			URI: uri,
		}

		// Send the request
		errCh <- c.session.SendRequest(ctx, spec.MethodResourcesUnsubscribe, requestParams, nil)
	}()

	return errCh
}

// GetPrompts returns the list of prompts provided by the server.
func (c *asyncClientImpl) GetPrompts() ([]spec.Prompt, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	resultCh, errCh := c.GetPromptsAsync(ctx)

	select {
	case result := <-resultCh:
		return result, nil
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// GetPromptsAsync returns the list of prompts provided by the server asynchronously.
func (c *asyncClientImpl) GetPromptsAsync(ctx context.Context) (chan []spec.Prompt, chan error) {
	resultCh := make(chan []spec.Prompt, 1)
	errCh := make(chan error, 1)

	if !c.initialized {
		errCh <- errors.New("client not initialized")
		close(resultCh)
		return resultCh, errCh
	}

	go func() {
		defer close(resultCh)
		defer close(errCh)

		var result spec.ListPromptsResult
		err := c.session.SendRequest(ctx, spec.MethodPromptList, nil, &result)
		if err != nil {
			errCh <- err
			return
		}

		// Notify handlers about the prompts
		for _, handler := range c.features.PromptsChangeHandlers {
			handler(result.Prompts)
		}

		resultCh <- result.Prompts
	}()

	return resultCh, errCh
}

// GetPrompt gets a prompt from the server with the given arguments.
func (c *asyncClientImpl) GetPrompt(name string, args map[string]interface{}) (*spec.GetPromptResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	resultCh, errCh := c.GetPromptAsync(ctx, name, args)

	select {
	case result := <-resultCh:
		return result, nil
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// GetPromptAsync gets a prompt from the server with the given arguments asynchronously.
func (c *asyncClientImpl) GetPromptAsync(ctx context.Context, name string, args map[string]interface{}) (chan *spec.GetPromptResult, chan error) {
	resultCh := make(chan *spec.GetPromptResult, 1)
	errCh := make(chan error, 1)

	if !c.initialized {
		errCh <- errors.New("client not initialized")
		close(resultCh)
		return resultCh, errCh
	}

	util.AssertNotEmpty(name, "Prompt name must not be empty")

	go func() {
		defer close(resultCh)
		defer close(errCh)

		// Create request payload
		requestParams := &spec.GetPromptRequest{
			Name:      name,
			Arguments: args,
		}

		// Send the request
		var result spec.GetPromptResult
		err := c.session.SendRequest(ctx, spec.MethodPromptGet, requestParams, &result)
		if err != nil {
			errCh <- err
			return
		}

		resultCh <- &result
	}()

	return resultCh, errCh
}

// CreatePrompt creates a new prompt on the server.
func (c *asyncClientImpl) CreatePrompt(prompt spec.Prompt, messages []spec.PromptMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	errCh := c.CreatePromptAsync(ctx, prompt, messages)

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// CreatePromptAsync creates a new prompt on the server asynchronously.
func (c *asyncClientImpl) CreatePromptAsync(ctx context.Context, prompt spec.Prompt, messages []spec.PromptMessage) chan error {
	errCh := make(chan error, 1)

	if !c.initialized {
		errCh <- errors.New("client not initialized")
		close(errCh)
		return errCh
	}

	util.AssertNotNil(prompt, "Prompt must not be nil")
	util.AssertNotEmpty(prompt.Name, "Prompt name must not be empty")
	util.AssertNotNil(messages, "Prompt messages must not be nil")

	go func() {
		defer close(errCh)

		// Create request payload
		requestParams := &spec.CreatePromptRequest{
			Prompt:   prompt,
			Messages: messages,
		}

		var result spec.CreatePromptResult
		errCh <- c.session.SendRequest(ctx, spec.MethodPromptCreate, requestParams, &result)
	}()

	return errCh
}

// UpdatePrompt updates an existing prompt on the server.
func (c *asyncClientImpl) UpdatePrompt(prompt spec.Prompt, messages []spec.PromptMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	errCh := c.UpdatePromptAsync(ctx, prompt, messages)

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// UpdatePromptAsync updates an existing prompt on the server asynchronously.
func (c *asyncClientImpl) UpdatePromptAsync(ctx context.Context, prompt spec.Prompt, messages []spec.PromptMessage) chan error {
	errCh := make(chan error, 1)

	if !c.initialized {
		errCh <- errors.New("client not initialized")
		close(errCh)
		return errCh
	}

	util.AssertNotNil(prompt, "Prompt must not be nil")
	util.AssertNotEmpty(prompt.Name, "Prompt name must not be empty")
	util.AssertNotNil(messages, "Prompt messages must not be nil")

	go func() {
		defer close(errCh)

		// Create request payload
		requestParams := &spec.UpdatePromptRequest{
			Prompt:   prompt,
			Messages: messages,
		}

		var result spec.UpdatePromptResult
		errCh <- c.session.SendRequest(ctx, spec.MethodPromptUpdate, requestParams, &result)
	}()

	return errCh
}

// DeletePrompt deletes a prompt from the server.
func (c *asyncClientImpl) DeletePrompt(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	errCh := c.DeletePromptAsync(ctx, name)

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// DeletePromptAsync deletes a prompt from the server asynchronously.
func (c *asyncClientImpl) DeletePromptAsync(ctx context.Context, name string) chan error {
	errCh := make(chan error, 1)

	if !c.initialized {
		errCh <- errors.New("client not initialized")
		close(errCh)
		return errCh
	}

	util.AssertNotEmpty(name, "Prompt name must not be empty")

	go func() {
		defer close(errCh)

		// Create request payload
		requestParams := &spec.DeletePromptRequest{
			Name: name,
		}

		var result spec.DeletePromptResult
		errCh <- c.session.SendRequest(ctx, spec.MethodPromptDelete, requestParams, &result)
	}()

	return errCh
}

// CreateMessage sends a create message request to the server.
func (c *asyncClientImpl) CreateMessage(request *spec.CreateMessageRequest) (*spec.CreateMessageResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	resultCh, errCh := c.CreateMessageAsync(ctx, request)

	select {
	case result := <-resultCh:
		return result, nil
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// CreateMessageAsync sends a create message request to the server asynchronously.
func (c *asyncClientImpl) CreateMessageAsync(ctx context.Context, request *spec.CreateMessageRequest) (chan *spec.CreateMessageResult, chan error) {
	resultCh := make(chan *spec.CreateMessageResult, 1)
	errCh := make(chan error, 1)

	if !c.initialized {
		errCh <- errors.New("client not initialized")
		close(resultCh)
		return resultCh, errCh
	}

	go func() {
		defer close(resultCh)
		defer close(errCh)

		// Apply sampling if a handler is configured
		if c.asyncSamplingHandler != nil {
			resCh, sErrCh := c.asyncSamplingHandler.CreateMessageAsync(request)

			// Wait for result or error from sampling
			select {
			case result := <-resCh:
				resultCh <- result
				return
			case err := <-sErrCh:
				errCh <- err
				return
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			}
		}

		// Send the request
		var result spec.CreateMessageResult
		err := c.session.SendRequest(ctx, spec.MethodSamplingCreateMessage, request, &result)
		if err != nil {
			errCh <- err
			return
		}

		resultCh <- &result
	}()

	return resultCh, errCh
}

// CreateMessageStreamAsync sends a create message request to the server and streams the responses asynchronously.
// The returned channels will receive partial results and errors as they become available.
func (c *asyncClientImpl) CreateMessageStreamAsync(ctx context.Context, request *spec.CreateMessageRequest) (chan *spec.CreateMessageResult, chan error) {
	resultCh := make(chan *spec.CreateMessageResult)
	errCh := make(chan error, 1)

	if !c.initialized {
		errCh <- errors.New("client not initialized")
		close(resultCh)
		return resultCh, errCh
	}

	go func() {
		defer close(resultCh)
		defer close(errCh)

		// Apply sampling if a handler is configured
		if c.asyncSamplingHandler != nil {
			// For now, we don't support streaming with the sampling handler
			// In the future, we could extend AsyncSamplingHandler to support streaming
			resCh, sErrCh := c.asyncSamplingHandler.CreateMessageAsync(request)

			select {
			case result := <-resCh:
				resultCh <- result
				return
			case err := <-sErrCh:
				errCh <- err
				return
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			}
		}

		// Check if server supports streaming
		supportsStreaming := false
		if c.serverCapabilities.Tools != nil {
			supportsStreaming = c.serverCapabilities.Tools.Streaming
		}

		if !supportsStreaming {
			// Fall back to non-streaming implementation
			resCh, sErrCh := c.CreateMessageAsync(ctx, request)

			select {
			case result := <-resCh:
				resultCh <- result
				return
			case err := <-sErrCh:
				errCh <- err
				return
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			}
		}

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

		// Send the streaming request
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

// CreateMessageStream sends a create message request to the server and streams the responses.
// This is a synchronous wrapper around the async version.
func (c *asyncClientImpl) CreateMessageStream(ctx context.Context, request *spec.CreateMessageRequest) (<-chan *spec.CreateMessageResult, <-chan error) {
	return c.CreateMessageStreamAsync(ctx, request)
}

// CreateMessageWithModelPreferences creates a new message with specified model preferences
func (c *asyncClientImpl) CreateMessageWithModelPreferences(content string, modelPreferences *spec.ModelPreferences) (*spec.CreateMessageResult, error) {
	if !c.initialized {
		return nil, errors.New("client not initialized")
	}

	// Create a request with the specified model preferences
	request := spec.NewCreateMessageRequestBuilder().
		Content(content).
		ModelPreferences(modelPreferences).
		Build()

	return c.CreateMessage(&request)
}

// CreateMessageWithModel creates a new message with a specific model hint
func (c *asyncClientImpl) CreateMessageWithModel(content string, modelName string) (*spec.CreateMessageResult, error) {
	if !c.initialized {
		return nil, errors.New("client not initialized")
	}

	// Create model preferences with a single hint
	prefs := spec.NewModelPreferencesBuilder().
		AddHint(modelName).
		Build()

	return c.CreateMessageWithModelPreferences(content, &prefs)
}

// CreateMessageWithModelPreferencesAsync creates a new message with specified model preferences asynchronously
func (c *asyncClientImpl) CreateMessageWithModelPreferencesAsync(ctx context.Context, content string, modelPreferences *spec.ModelPreferences) (chan *spec.CreateMessageResult, chan error) {
	if !c.initialized {
		resultCh := make(chan *spec.CreateMessageResult, 1)
		errCh := make(chan error, 1)
		errCh <- errors.New("client not initialized")
		close(resultCh)
		return resultCh, errCh
	}

	// Create a request with the specified model preferences
	request := spec.NewCreateMessageRequestBuilder().
		Content(content).
		ModelPreferences(modelPreferences).
		Build()

	return c.CreateMessageAsync(ctx, &request)
}

// CreateMessageWithModelAsync creates a new message with a specific model hint asynchronously
func (c *asyncClientImpl) CreateMessageWithModelAsync(ctx context.Context, content string, modelName string) (chan *spec.CreateMessageResult, chan error) {
	if !c.initialized {
		resultCh := make(chan *spec.CreateMessageResult, 1)
		errCh := make(chan error, 1)
		errCh <- errors.New("client not initialized")
		close(resultCh)
		return resultCh, errCh
	}

	// Create model preferences with a single hint
	prefs := spec.NewModelPreferencesBuilder().
		AddHint(modelName).
		Build()

	return c.CreateMessageWithModelPreferencesAsync(ctx, content, &prefs)
}

// SetLoggingLevel sets the minimum level for logs from the server.
func (c *asyncClientImpl) SetLoggingLevel(level spec.LogLevel) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	errCh := c.SetLoggingLevelAsync(ctx, level)

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SetLoggingLevelAsync sets the minimum level for logs from the server asynchronously.
func (c *asyncClientImpl) SetLoggingLevelAsync(ctx context.Context, level spec.LogLevel) chan error {
	errCh := make(chan error, 1)

	if !c.initialized {
		errCh <- errors.New("client not initialized")
		close(errCh)
		return errCh
	}

	util.AssertNotEmpty(string(level), "Log level must not be empty")

	go func() {
		defer close(errCh)

		// Create request payload
		requestParams := &spec.SetLevelRequest{
			Level: level,
		}

		// Send the request
		errCh <- c.session.SendRequest(ctx, spec.MethodLoggingSetLevel, requestParams, nil)
	}()

	return errCh
}

// Ping sends a ping to the server to check connectivity.
func (c *asyncClientImpl) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	errCh := c.PingAsync(ctx)

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// PingAsync sends a ping to the server to check connectivity asynchronously.
func (c *asyncClientImpl) PingAsync(ctx context.Context) chan error {
	errCh := make(chan error, 1)

	if !c.initialized {
		errCh <- errors.New("client not initialized")
		close(errCh)
		return errCh
	}

	go func() {
		defer close(errCh)

		// Send the ping request
		errCh <- c.session.SendRequest(ctx, spec.MethodPing, nil, nil)
	}()

	return errCh
}

// ExecuteToolsParallel executes multiple tools on the server in parallel.
// The toolCalls parameter is a map where keys are tool names and values are the parameters to pass to each tool.
// The results parameter is a map where keys are tool names and values are pointers to store the results.
// Returns a map of tool names to errors for any failed tool executions.
func (c *asyncClientImpl) ExecuteToolsParallel(ctx context.Context, toolCalls map[string]interface{}, results map[string]interface{}) map[string]error {
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), c.requestTimeout)
		defer cancel()
	}

	// Execute tools in parallel and wait for all of them to complete
	resultCh, errCh := c.ExecuteToolsParallelAsync(ctx, toolCalls, results)

	var errorMap map[string]error

	// Wait for either a result, error, or context cancellation
	select {
	case result := <-resultCh:
		// Loop through the result map and use reflection to store the values in the provided pointers
		for toolName, resultValue := range result {
			resultPtr, exists := results[toolName]
			if exists {
				// Use reflection to set the pointer value
				ptrValue := reflect.ValueOf(resultPtr)
				if ptrValue.Kind() == reflect.Ptr && !ptrValue.IsNil() {
					ptrValue.Elem().Set(reflect.ValueOf(resultValue).Elem())
				}
			}
		}
		return make(map[string]error) // Return empty error map if successful
	case err := <-errCh:
		// If we got a consolidated error, create an error map where all tools have the same error
		errorMap = make(map[string]error)
		for toolName := range toolCalls {
			errorMap[toolName] = err
		}
		return errorMap
	case <-ctx.Done():
		// If context was cancelled, create an error map with the context error
		errorMap = make(map[string]error)
		for toolName := range toolCalls {
			errorMap[toolName] = ctx.Err()
		}
		return errorMap
	}
}

// ExecuteToolsParallelAsync executes multiple tools on the server in parallel asynchronously.
// The toolCalls parameter is a map where keys are tool names and values are the parameters to pass to each tool.
// The resultTypes parameter is a map where keys are tool names and values are the expected result types.
// Returns two channels: one for results (map of tool names to results) and one for errors.
func (c *asyncClientImpl) ExecuteToolsParallelAsync(ctx context.Context, toolCalls map[string]interface{}, resultTypes map[string]interface{}) (chan map[string]interface{}, chan error) {
	resultCh := make(chan map[string]interface{}, 1)
	errCh := make(chan error, 1)

	if !c.initialized {
		errCh <- errors.New("client not initialized")
		close(resultCh)
		return resultCh, errCh
	}

	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), c.requestTimeout)
		defer cancel()
	}

	go func() {
		defer close(resultCh)
		defer close(errCh)

		// Get the maximum number of tools that can be executed in parallel
		maxParallelism := 1 // Default to sequential execution
		if c.clientCapabilities.MaxToolParallelism > 0 {
			maxParallelism = c.clientCapabilities.MaxToolParallelism
		}

		// Check if server supports parallel execution
		supportsParallel := false
		if c.serverCapabilities.Tools != nil {
			supportsParallel = c.serverCapabilities.Tools.Parallel
		}

		// If server or client doesn't support parallel execution, fall back to sequential
		if !supportsParallel || maxParallelism == 1 {
			toolResults, err := c.executeToolsSequentialAsync(ctx, toolCalls, resultTypes)
			if err != nil {
				errCh <- err
				return
			}
			resultCh <- toolResults
			return
		}

		// Execute tools in parallel
		toolResults, err := c.executeToolsParallelAsync(ctx, toolCalls, resultTypes, maxParallelism)
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- toolResults
	}()

	return resultCh, errCh
}

// executeToolsSequentialAsync executes tools one at a time asynchronously
func (c *asyncClientImpl) executeToolsSequentialAsync(ctx context.Context, toolCalls map[string]interface{}, resultTypes map[string]interface{}) (map[string]interface{}, error) {
	results := make(map[string]interface{})

	for toolName, params := range toolCalls {
		resultType, exists := resultTypes[toolName]
		if !exists {
			return nil, fmt.Errorf("no result type provided for tool %s", toolName)
		}

		// Create a context that will be cancelled if the parent context is cancelled
		toolCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		// Add a goroutine to cancel this context if the parent is cancelled
		go func() {
			select {
			case <-ctx.Done():
				cancel()
			case <-toolCtx.Done():
				// Tool completed or was cancelled
			}
		}()

		// Execute the tool and wait for the result
		resultCh, errCh := c.ExecuteToolAsync(toolCtx, toolName, params, resultType)

		select {
		case result := <-resultCh:
			results[toolName] = result
		case err := <-errCh:
			return nil, fmt.Errorf("error executing tool %s: %w", toolName, err)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return results, nil
}

// executeToolsParallelAsync executes tools in parallel with a limit on concurrency
func (c *asyncClientImpl) executeToolsParallelAsync(ctx context.Context, toolCalls map[string]interface{}, resultTypes map[string]interface{}, maxConcurrent int) (map[string]interface{}, error) {
	results := make(map[string]interface{})
	resultsMutex := sync.Mutex{}

	// Create a cancellable context for all tool executions
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Create a wait group to synchronize all goroutines
	var wg sync.WaitGroup

	// Create a semaphore to limit concurrency
	sem := make(chan struct{}, maxConcurrent)

	// Channel for errors
	errorCh := make(chan error, len(toolCalls))

	// Start a goroutine for each tool
	for toolName, params := range toolCalls {
		resultType, exists := resultTypes[toolName]
		if !exists {
			errorCh <- fmt.Errorf("no result type provided for tool %s", toolName)
			continue
		}

		wg.Add(1)
		go func(name string, p interface{}, rt interface{}) {
			defer wg.Done()

			// Acquire semaphore
			select {
			case sem <- struct{}{}:
				// Acquired semaphore
				defer func() { <-sem }() // Release semaphore
			case <-execCtx.Done():
				// Context cancelled while waiting for semaphore
				errorCh <- execCtx.Err()
				return
			}

			// Execute the tool
			resultCh, errToolCh := c.ExecuteToolAsync(execCtx, name, p, rt)

			// Wait for result or error
			select {
			case result := <-resultCh:
				// Store the result
				resultsMutex.Lock()
				results[name] = result
				resultsMutex.Unlock()
			case err := <-errToolCh:
				// Send error to error channel
				errorCh <- fmt.Errorf("error executing tool %s: %w", name, err)
				// Cancel all other executions
				cancel()
			case <-execCtx.Done():
				// Context cancelled
				errorCh <- execCtx.Err()
			}
		}(toolName, params, resultType)
	}

	// Wait for all tools to complete or context to be cancelled
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
		close(errorCh)
	}()

	// Wait for either completion or an error
	select {
	case <-done:
		// Check if we have any errors
		select {
		case err := <-errorCh:
			return nil, err
		default:
			// No errors
			return results, nil
		}
	case err := <-errorCh:
		// Got an error, cancel and return
		cancel()
		return nil, err
	case <-ctx.Done():
		// Context cancelled
		return nil, ctx.Err()
	}
}

// GetRoots returns the list of roots provided by the server.
func (c *asyncClientImpl) GetRoots() ([]spec.Root, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	resultCh, errCh := c.GetRootsAsync(ctx)

	select {
	case result := <-resultCh:
		return result, nil
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// GetRootsAsync returns the list of roots provided by the server asynchronously.
func (c *asyncClientImpl) GetRootsAsync(ctx context.Context) (chan []spec.Root, chan error) {
	resultCh := make(chan []spec.Root, 1)
	errCh := make(chan error, 1)

	if !c.initialized {
		errCh <- errors.New("client not initialized")
		close(resultCh)
		return resultCh, errCh
	}

	go func() {
		defer close(resultCh)
		defer close(errCh)

		var result spec.ListRootsResult
		err := c.session.SendRequest(ctx, spec.MethodRootsList, nil, &result)
		if err != nil {
			errCh <- err
			return
		}

		// Notify handlers about the roots
		for _, handler := range c.features.RootsChangeHandlers {
			handler(result.Roots)
		}

		resultCh <- result.Roots
	}()

	return resultCh, errCh
}

// CreateRoot creates a new root on the server.
func (c *asyncClientImpl) CreateRoot(root spec.Root) (*spec.Root, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	resultCh, errCh := c.CreateRootAsync(ctx, root)

	select {
	case result := <-resultCh:
		return result, nil
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// CreateRootAsync creates a new root on the server asynchronously.
func (c *asyncClientImpl) CreateRootAsync(ctx context.Context, root spec.Root) (chan *spec.Root, chan error) {
	resultCh := make(chan *spec.Root, 1)
	errCh := make(chan error, 1)

	if !c.initialized {
		errCh <- errors.New("client not initialized")
		close(resultCh)
		return resultCh, errCh
	}

	util.AssertNotNil(root, "Root must not be nil")
	util.AssertNotEmpty(root.URI, "Root URI must not be empty")

	go func() {
		defer close(resultCh)
		defer close(errCh)

		// Create request payload
		requestParams := &spec.CreateRootRequest{
			Root: root,
		}

		var result spec.CreateRootResult
		err := c.session.SendRequest(ctx, spec.MethodRootsCreate, requestParams, &result)
		if err != nil {
			errCh <- err
			return
		}

		// Update local cache
		c.features.AddRoot(result.Root)

		// Notify handlers about the root creation
		if len(c.features.RootsChangeHandlers) > 0 {
			// Get the updated list of roots
			roots, _ := c.GetRoots()
			if roots != nil {
				for _, handler := range c.features.RootsChangeHandlers {
					handler(roots)
				}
			}
		}

		resultCh <- &result.Root
	}()

	return resultCh, errCh
}

// UpdateRoot updates an existing root on the server.
func (c *asyncClientImpl) UpdateRoot(root spec.Root) (*spec.Root, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	resultCh, errCh := c.UpdateRootAsync(ctx, root)

	select {
	case result := <-resultCh:
		return result, nil
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// UpdateRootAsync updates an existing root on the server asynchronously.
func (c *asyncClientImpl) UpdateRootAsync(ctx context.Context, root spec.Root) (chan *spec.Root, chan error) {
	resultCh := make(chan *spec.Root, 1)
	errCh := make(chan error, 1)

	if !c.initialized {
		errCh <- errors.New("client not initialized")
		close(resultCh)
		return resultCh, errCh
	}

	util.AssertNotNil(root, "Root must not be nil")
	util.AssertNotEmpty(root.URI, "Root URI must not be empty")

	go func() {
		defer close(resultCh)
		defer close(errCh)

		// Create request payload
		requestParams := &spec.UpdateRootRequest{
			Root: root,
		}

		var result spec.UpdateRootResult
		err := c.session.SendRequest(ctx, spec.MethodRootsUpdate, requestParams, &result)
		if err != nil {
			errCh <- err
			return
		}

		// Update local cache
		c.features.AddRoot(result.Root)

		// Notify handlers about the root update
		if len(c.features.RootsChangeHandlers) > 0 {
			// Get the updated list of roots
			roots, _ := c.GetRoots()
			if roots != nil {
				for _, handler := range c.features.RootsChangeHandlers {
					handler(roots)
				}
			}
		}

		resultCh <- &result.Root
	}()

	return resultCh, errCh
}

// DeleteRoot deletes a root from the server.
func (c *asyncClientImpl) DeleteRoot(uri string) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	errCh := c.DeleteRootAsync(ctx, uri)

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// DeleteRootAsync deletes a root from the server asynchronously.
func (c *asyncClientImpl) DeleteRootAsync(ctx context.Context, uri string) chan error {
	errCh := make(chan error, 1)

	if !c.initialized {
		errCh <- errors.New("client not initialized")
		close(errCh)
		return errCh
	}

	util.AssertNotEmpty(uri, "Root URI must not be empty")

	go func() {
		defer close(errCh)

		// Create request payload
		requestParams := &spec.DeleteRootRequest{
			URI: uri,
		}

		var result spec.DeleteRootResult
		err := c.session.SendRequest(ctx, spec.MethodRootsDelete, requestParams, &result)
		if err != nil {
			errCh <- err
			return
		}

		// Remove from local cache
		delete(c.features.Roots, uri)

		// Notify handlers about the root deletion
		if len(c.features.RootsChangeHandlers) > 0 {
			// Get the updated list of roots
			roots, _ := c.GetRoots()
			if roots != nil {
				for _, handler := range c.features.RootsChangeHandlers {
					handler(roots)
				}
			}
		}

		errCh <- nil
	}()

	return errCh
}

// OnRootsChanged registers a callback to be notified when roots change.
func (c *asyncClientImpl) OnRootsChanged(callback func([]spec.Root)) {
	c.features.AddRootsChangeHandler(callback)
}

// Add root change notification handler
func (c *asyncClientImpl) registerRootsChangeNotificationHandler() {
	if c.session == nil {
		return
	}

	// Register handler for roots list changes
	c.session.SetNotificationHandler(spec.MethodNotificationRootsListChanged, func(ctx context.Context, params json.RawMessage) error {
		// Refresh roots list
		go func() {
			roots, _ := c.GetRoots()
			for _, handler := range c.features.RootsChangeHandlers {
				handler(roots)
			}
		}()
		return nil
	})
}
