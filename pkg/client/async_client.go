package client

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
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
