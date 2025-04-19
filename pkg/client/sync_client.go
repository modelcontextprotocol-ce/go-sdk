package client

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/modelcontextprotocol-ce/go-sdk/pkg/spec"
	"github.com/modelcontextprotocol-ce/go-sdk/pkg/util"
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

// CreatePrompt creates a new prompt on the server.
func (c *syncClientImpl) CreatePrompt(prompt spec.Prompt, messages []spec.PromptMessage) error {
	if !c.initialized {
		return errors.New("client not initialized")
	}

	util.AssertNotNil(prompt, "Prompt must not be nil")
	util.AssertNotEmpty(prompt.Name, "Prompt name must not be empty")
	util.AssertNotNil(messages, "Prompt messages must not be nil")

	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	// Create request payload
	requestParams := &spec.CreatePromptRequest{
		Prompt:   prompt,
		Messages: messages,
	}

	var result spec.CreatePromptResult
	return c.session.SendRequest(ctx, spec.MethodPromptCreate, requestParams, &result)
}

// UpdatePrompt updates an existing prompt on the server.
func (c *syncClientImpl) UpdatePrompt(prompt spec.Prompt, messages []spec.PromptMessage) error {
	if !c.initialized {
		return errors.New("client not initialized")
	}

	util.AssertNotNil(prompt, "Prompt must not be nil")
	util.AssertNotEmpty(prompt.Name, "Prompt name must not be empty")
	util.AssertNotNil(messages, "Prompt messages must not be nil")

	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	// Create request payload
	requestParams := &spec.UpdatePromptRequest{
		Prompt:   prompt,
		Messages: messages,
	}

	var result spec.UpdatePromptResult
	return c.session.SendRequest(ctx, spec.MethodPromptUpdate, requestParams, &result)
}

// DeletePrompt deletes a prompt from the server.
func (c *syncClientImpl) DeletePrompt(name string) error {
	if !c.initialized {
		return errors.New("client not initialized")
	}

	util.AssertNotEmpty(name, "Prompt name must not be empty")

	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	// Create request payload
	requestParams := &spec.DeletePromptRequest{
		Name: name,
	}

	var result spec.DeletePromptResult
	return c.session.SendRequest(ctx, spec.MethodPromptDelete, requestParams, &result)
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

// ExecuteToolsParallel executes multiple tools on the server in parallel.
// The toolCalls parameter is a map where keys are tool names and values are the parameters to pass to each tool.
// The results parameter is a map where keys are tool names and values are pointers to store the results.
// Returns a map of tool names to errors for any failed tool executions.
func (c *syncClientImpl) ExecuteToolsParallel(ctx context.Context, toolCalls map[string]interface{}, results map[string]interface{}) map[string]error {
	if !c.initialized {
		errorMap := make(map[string]error)
		for toolName := range toolCalls {
			errorMap[toolName] = errors.New("client not initialized")
		}
		return errorMap
	}

	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), c.requestTimeout)
		defer cancel()
	}

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
		return c.executeToolsSequential(ctx, toolCalls, results)
	}

	// Execute tools in parallel
	return c.executeToolsParallel(ctx, toolCalls, results, maxParallelism)
}

// executeToolsSequential executes tools one at a time
func (c *syncClientImpl) executeToolsSequential(ctx context.Context, toolCalls map[string]interface{}, results map[string]interface{}) map[string]error {
	errorMap := make(map[string]error)

	for toolName, params := range toolCalls {
		resultPtr, exists := results[toolName]
		if !exists {
			errorMap[toolName] = fmt.Errorf("no result pointer provided for tool %s", toolName)
			continue
		}

		err := c.ExecuteTool(toolName, params, resultPtr)
		if err != nil {
			errorMap[toolName] = err
		}

		// Check if context is cancelled
		select {
		case <-ctx.Done():
			// Add context error for remaining tools
			for name := range toolCalls {
				if _, processed := errorMap[name]; !processed {
					errorMap[name] = ctx.Err()
				}
			}
			return errorMap
		default:
			// Continue processing
		}
	}

	return errorMap
}

// executeToolsParallel executes tools in parallel with a limit on concurrency
func (c *syncClientImpl) executeToolsParallel(ctx context.Context, toolCalls map[string]interface{}, results map[string]interface{}, maxConcurrent int) map[string]error {
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrent)
	errorMap := make(map[string]error)
	errorMapMutex := sync.Mutex{}

	// Create a context that can be cancelled if we need to abort
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Execute tools in parallel, controlling concurrency with semaphore
	for toolName, params := range toolCalls {
		wg.Add(1)
		resultPtr := results[toolName]

		go func(name string, p interface{}, r interface{}) {
			defer wg.Done()

			// Acquire semaphore
			select {
			case sem <- struct{}{}:
				// Successfully acquired semaphore
			case <-execCtx.Done():
				// Context cancelled while waiting for semaphore
				errorMapMutex.Lock()
				errorMap[name] = execCtx.Err()
				errorMapMutex.Unlock()
				return
			}
			defer func() { <-sem }() // Release semaphore

			// Execute the tool
			err := c.ExecuteTool(name, p, r)
			if err != nil {
				errorMapMutex.Lock()
				errorMap[name] = err
				errorMapMutex.Unlock()
			}
		}(toolName, params, resultPtr)
	}

	// Wait for all tools to complete or context to be cancelled
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	// Wait for either completion or context cancellation
	select {
	case <-done:
		// All tools completed
		return errorMap
	case <-execCtx.Done():
		// Context cancelled
		errorMapMutex.Lock()
		defer errorMapMutex.Unlock()

		// Add context error for all tools that didn't complete
		for toolName := range toolCalls {
			if _, exists := errorMap[toolName]; !exists {
				errorMap[toolName] = execCtx.Err()
			}
		}

		// Ensure we cancel any ongoing operations
		cancel()

		return errorMap
	}
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

// CreateResource creates a new resource on the server.
func (c *syncClientImpl) CreateResource(resource spec.Resource, contents []byte) error {
	if !c.initialized {
		return errors.New("client not initialized")
	}

	util.AssertNotNil(resource, "Resource must not be nil")
	util.AssertNotEmpty(resource.URI, "Resource URI must not be empty")

	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	// Create request payload
	requestParams := &spec.CreateResourceRequest{
		Resource: resource,
		Contents: contents,
	}

	var result spec.CreateResourceResult
	return c.session.SendRequest(ctx, spec.MethodResourcesCreate, requestParams, &result)
}

// UpdateResource updates an existing resource on the server.
func (c *syncClientImpl) UpdateResource(resource spec.Resource, contents []byte) error {
	if !c.initialized {
		return errors.New("client not initialized")
	}

	util.AssertNotNil(resource, "Resource must not be nil")
	util.AssertNotEmpty(resource.URI, "Resource URI must not be empty")

	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	// Create request payload
	requestParams := &spec.UpdateResourceRequest{
		Resource: resource,
		Contents: contents,
	}

	var result spec.UpdateResourceResult
	return c.session.SendRequest(ctx, spec.MethodResourcesUpdate, requestParams, &result)
}

// DeleteResource deletes a resource from the server.
func (c *syncClientImpl) DeleteResource(uri string) error {
	if !c.initialized {
		return errors.New("client not initialized")
	}

	util.AssertNotEmpty(uri, "Resource URI must not be empty")

	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	// Create request payload
	requestParams := &spec.DeleteResourceRequest{
		URI: uri,
	}

	var result spec.DeleteResourceResult
	return c.session.SendRequest(ctx, spec.MethodResourcesDelete, requestParams, &result)
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

// CreateMessageWithModelPreferences creates a new message with specified model preferences
func (c *syncClientImpl) CreateMessageWithModelPreferences(content string, modelPreferences *spec.ModelPreferences) (*spec.CreateMessageResult, error) {
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
func (c *syncClientImpl) CreateMessageWithModel(content string, modelName string) (*spec.CreateMessageResult, error) {
	if !c.initialized {
		return nil, errors.New("client not initialized")
	}

	// Create model preferences with a single hint
	prefs := spec.NewModelPreferencesBuilder().
		AddHint(modelName).
		Build()

	return c.CreateMessageWithModelPreferences(content, &prefs)
}

// GetRoots retrieves the list of available roots from the server
func (c *syncClientImpl) GetRoots() ([]spec.Root, error) {
	if !c.initialized {
		return nil, errors.New("client not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	var result spec.ListRootsResult
	err := c.session.SendRequest(ctx, spec.MethodRootsList, nil, &result)
	if err != nil {
		return nil, err
	}

	// Notify handlers about the roots
	for _, handler := range c.features.RootsChangeHandlers {
		handler(result.Roots)
	}

	return result.Roots, nil
}

// CreateRoot creates a new root on the server
func (c *syncClientImpl) CreateRoot(root spec.Root) (*spec.Root, error) {
	if !c.initialized {
		return nil, errors.New("client not initialized")
	}

	util.AssertNotNil(root, "Root must not be nil")
	util.AssertNotEmpty(root.URI, "Root URI must not be empty")

	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	// Create request payload
	requestParams := &spec.CreateRootRequest{
		Root: root,
	}

	var result spec.CreateRootResult
	err := c.session.SendRequest(ctx, spec.MethodRootsCreate, requestParams, &result)
	if err != nil {
		return nil, err
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

	return &result.Root, nil
}

// UpdateRoot updates an existing root on the server
func (c *syncClientImpl) UpdateRoot(root spec.Root) (*spec.Root, error) {
	if !c.initialized {
		return nil, errors.New("client not initialized")
	}

	util.AssertNotNil(root, "Root must not be nil")
	util.AssertNotEmpty(root.URI, "Root URI must not be empty")

	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	// Create request payload
	requestParams := &spec.UpdateRootRequest{
		Root: root,
	}

	var result spec.UpdateRootResult
	err := c.session.SendRequest(ctx, spec.MethodRootsUpdate, requestParams, &result)
	if err != nil {
		return nil, err
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

	return &result.Root, nil
}

// DeleteRoot deletes a root from the server
func (c *syncClientImpl) DeleteRoot(uri string) error {
	if !c.initialized {
		return errors.New("client not initialized")
	}

	util.AssertNotEmpty(uri, "Root URI must not be empty")

	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()

	// Create request payload
	requestParams := &spec.DeleteRootRequest{
		URI: uri,
	}

	var result spec.DeleteRootResult
	err := c.session.SendRequest(ctx, spec.MethodRootsDelete, requestParams, &result)
	if err != nil {
		return err
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

	return nil
}

// OnRootsChanged registers a callback to be notified when roots change.
func (c *syncClientImpl) OnRootsChanged(callback func([]spec.Root)) {
	c.features.AddRootsChangeHandler(callback)
}
