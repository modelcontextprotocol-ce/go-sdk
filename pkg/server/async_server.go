package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol-ce/go-sdk/pkg/spec"
	"github.com/modelcontextprotocol-ce/go-sdk/pkg/util"
)

// asyncServerImpl implements the McpAsyncServer interface
type asyncServerImpl struct {
	transportProvider    spec.McpServerTransportProvider
	transport            spec.McpServerTransport
	features             *ServerFeatures
	createMessageHandler interface{}
	toolHandlers         map[string]interface{}
	resourceHandler      AsyncResourceHandler
	promptHandler        AsyncPromptHandler
	mu                   sync.RWMutex
	running              bool
	clients              map[string]spec.McpClientSession
	clientsMu            sync.RWMutex
	requestTimeout       time.Duration
	activeOperations     map[string]context.CancelFunc
}

// AsyncServerExchange represents an asynchronous server exchange
type AsyncServerExchange[T any] interface {
	GetID() string
	GetRequest() T
	SendResponse(response interface{}) error
	SendError(err error) error
	PublishContent(content spec.Content) error
	CompleteContent() error
	ErrorContent(err error) error
}

// ResponseSender sends responses back to the client
type ResponseSender interface {
	SendResponse(response interface{}) error
	SendError(err error) error
}

// ContentPublisher publishes streaming content to the client
type ContentPublisher interface {
	PublishContent(content spec.Content) error
	CompleteContent() error
	ErrorContent(err error) error
}

// DefaultAsyncServerExchange provides a default implementation of AsyncServerExchange
type DefaultAsyncServerExchange[T any] struct {
	ID               string
	Request          T
	ResponseSender   ResponseSender
	ContentPublisher ContentPublisher
}

// NewDefaultAsyncServerExchange creates a new default async server exchange
func NewDefaultAsyncServerExchange[T any](request T, sender ResponseSender, publisher ContentPublisher) *DefaultAsyncServerExchange[T] {
	return &DefaultAsyncServerExchange[T]{
		ID:               uuid.New().String(),
		Request:          request,
		ResponseSender:   sender,
		ContentPublisher: publisher,
	}
}

// GetID returns the exchange ID
func (e *DefaultAsyncServerExchange[T]) GetID() string {
	return e.ID
}

// GetRequest returns the request
func (e *DefaultAsyncServerExchange[T]) GetRequest() T {
	return e.Request
}

// SendResponse sends a response to the client
func (e *DefaultAsyncServerExchange[T]) SendResponse(response interface{}) error {
	return e.ResponseSender.SendResponse(response)
}

// SendError sends an error to the client
func (e *DefaultAsyncServerExchange[T]) SendError(err error) error {
	return e.ResponseSender.SendError(err)
}

// PublishContent publishes content to the client
func (e *DefaultAsyncServerExchange[T]) PublishContent(content spec.Content) error {
	return e.ContentPublisher.PublishContent(content)
}

// CompleteContent signals completion of content streaming
func (e *DefaultAsyncServerExchange[T]) CompleteContent() error {
	return e.ContentPublisher.CompleteContent()
}

// ErrorContent signals an error in content streaming
func (e *DefaultAsyncServerExchange[T]) ErrorContent(err error) error {
	return e.ContentPublisher.ErrorContent(err)
}

// AsyncServer represents an asynchronous MCP server
type AsyncServer struct {
	config ServerConfig
}

// NewAsyncServer creates a new asynchronous server with the provided config
func NewAsyncServer(config ServerConfig) *AsyncServer {
	return &AsyncServer{
		config: config,
	}
}

// HandleContentRequest handles a content request asynchronously
func (s *AsyncServer) HandleContentRequest(ctx context.Context, request *spec.ContentRequest, exchange *DefaultAsyncServerExchange[*spec.ContentRequest]) {
	// Implementation depends on the specific use case
	// This is a placeholder for the actual implementation
}

// HandleToolExecution handles tool execution asynchronously
func (s *AsyncServer) HandleToolExecution(ctx context.Context, request *spec.ToolExecutionRequest, exchange *DefaultAsyncServerExchange[*spec.ToolExecutionRequest]) {
	if s.config.ToolExecutionHandler == nil {
		exchange.SendError(fmt.Errorf("no tool execution handler configured"))
		return
	}

	executionContext := ToolExecutionContext{
		Request:       request,
		ToolID:        request.ToolID,
		ToolName:      request.ToolName,
		ToolArguments: request.ToolArguments,
		Callback:      &asyncToolExecutionCallback{exchange: exchange},
	}

	err := s.config.ToolExecutionHandler.ExecuteTool(ctx, executionContext)
	if err != nil {
		exchange.SendError(err)
	}
}

// asyncToolExecutionCallback implements ToolExecutionCallback for async servers
type asyncToolExecutionCallback struct {
	exchange *DefaultAsyncServerExchange[*spec.ToolExecutionRequest]
}

// OnToolResult is called when a tool execution completes successfully
func (c *asyncToolExecutionCallback) OnToolResult(toolResult *spec.ToolResult) error {
	response := &spec.ToolExecutionResponse{
		ToolResult: toolResult,
	}
	return c.exchange.SendResponse(response)
}

// OnToolError is called when a tool execution fails
func (c *asyncToolExecutionCallback) OnToolError(err error) error {
	return c.exchange.SendError(err)
}

// Start implements McpServer
func (s *asyncServerImpl) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return errors.New("server already running")
	}

	transport, err := s.transportProvider.GetTransport()
	if err != nil {
		return fmt.Errorf("failed to get transport: %w", err)
	}

	s.transport = transport
	s.clients = make(map[string]spec.McpClientSession)
	s.running = true

	go s.handleMessages()

	return nil
}

// Stop implements McpServer
func (s *asyncServerImpl) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return errors.New("server not running")
	}

	s.running = false

	// Close client sessions
	s.clientsMu.Lock()
	for _, client := range s.clients {
		_ = client.Close()
	}
	s.clients = nil
	s.clientsMu.Unlock()

	// Close transport
	if s.transport != nil {
		err := s.transport.Close()
		s.transport = nil
		return err
	}

	return nil
}

// StopGracefully implements McpServer
func (s *asyncServerImpl) StopGracefully(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return errors.New("server not running")
	}

	s.running = false

	// Close client sessions gracefully
	s.clientsMu.Lock()
	for _, client := range s.clients {
		// Attempt graceful close with context
		if closer, ok := client.(interface{ CloseGracefully(context.Context) error }); ok {
			_ = closer.CloseGracefully(ctx)
		} else {
			_ = client.Close()
		}
	}
	s.clients = nil
	s.clientsMu.Unlock()

	// Close transport gracefully
	if s.transport != nil {
		err := s.transport.StopGracefully(ctx)
		s.transport = nil
		return err
	}

	return nil
}

// IsRunning implements McpServer
func (s *asyncServerImpl) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// GetFeatures implements McpServer
func (s *asyncServerImpl) GetFeatures() *ServerFeatures {
	return s.features
}

// GetProtocolVersion implements McpServer
func (s *asyncServerImpl) GetProtocolVersion() string {
	return s.features.ProtocolVersion
}

// GetServerInfo implements McpServer
func (s *asyncServerImpl) GetServerInfo() spec.Implementation {
	return s.features.ServerInfo
}

// GetServerCapabilities implements McpServer
func (s *asyncServerImpl) GetServerCapabilities() spec.ServerCapabilities {
	return s.features.ServerCapabilities
}

// BroadcastToolsChanged implements McpServer
func (s *asyncServerImpl) BroadcastToolsChanged() error {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	var lastErr error
	for _, client := range s.clients {
		if err := client.NotifyToolsListChanged(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// BroadcastResourcesChanged implements McpServer
func (s *asyncServerImpl) BroadcastResourcesChanged() error {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	var lastErr error
	for _, client := range s.clients {
		if err := client.NotifyResourcesListChanged(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// BroadcastPromptsChanged implements McpServer
func (s *asyncServerImpl) BroadcastPromptsChanged() error {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	var lastErr error
	for _, client := range s.clients {
		if err := client.NotifyPromptsListChanged(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// BroadcastResourceChanged implements McpServer
func (s *asyncServerImpl) BroadcastResourceChanged(uri string, contents []byte) error {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	var lastErr error
	for _, client := range s.clients {
		if err := client.NotifyResourceChanged(uri, contents); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// SendLogMessage implements McpServer
func (s *asyncServerImpl) SendLogMessage(level spec.LogLevel, message string, logger string) error {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	var lastErr error
	for _, client := range s.clients {
		if err := client.SendLogMessage(level, message, logger); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// RegisterToolHandler implements McpAsyncServer
func (s *asyncServerImpl) RegisterToolHandler(name string, handler AsyncToolHandler) error {
	util.AssertNotNil(name, "Tool name must not be nil")
	util.AssertNotNil(handler, "Tool handler must not be nil")

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.toolHandlers == nil {
		s.toolHandlers = make(map[string]interface{})
	}
	s.toolHandlers[name] = handler
	return nil
}

// RegisterResourceHandler implements McpAsyncServer
func (s *asyncServerImpl) RegisterResourceHandler(handler AsyncResourceHandler) error {
	util.AssertNotNil(handler, "Resource handler must not be nil")

	s.mu.Lock()
	defer s.mu.Unlock()

	s.resourceHandler = handler
	return nil
}

// RegisterPromptHandler implements McpAsyncServer
func (s *asyncServerImpl) RegisterPromptHandler(handler AsyncPromptHandler) error {
	util.AssertNotNil(handler, "Prompt handler must not be nil")

	s.mu.Lock()
	defer s.mu.Unlock()

	s.promptHandler = handler
	return nil
}

// SetCreateMessageHandler implements McpAsyncServer
func (s *asyncServerImpl) SetCreateMessageHandler(handler AsyncCreateMessageHandler) error {
	util.AssertNotNil(handler, "Create message handler must not be nil")

	s.mu.Lock()
	defer s.mu.Unlock()

	s.createMessageHandler = handler
	return nil
}

// AddTool implements McpAsyncServer
func (s *asyncServerImpl) AddTool(tool spec.Tool) error {
	util.AssertNotNil(tool, "Tool must not be nil")

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if tool already exists
	for i, t := range s.features.AvailableTools {
		if t.Name == tool.Name {
			// Replace the existing tool
			s.features.AvailableTools[i] = tool
			return s.BroadcastToolsChanged()
		}
	}

	// Add new tool
	s.features.AvailableTools = append(s.features.AvailableTools, tool)
	return s.BroadcastToolsChanged()
}

// AddResource implements McpAsyncServer
func (s *asyncServerImpl) AddResource(resource spec.Resource) error {
	util.AssertNotNil(resource, "Resource must not be nil")

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if resource already exists
	for i, r := range s.features.AvailableResources {
		if r.URI == resource.URI {
			// Replace the existing resource
			s.features.AvailableResources[i] = resource
			return s.BroadcastResourcesChanged()
		}
	}

	// Add new resource
	s.features.AvailableResources = append(s.features.AvailableResources, resource)
	return s.BroadcastResourcesChanged()
}

// AddPrompt implements McpAsyncServer
func (s *asyncServerImpl) AddPrompt(prompt spec.Prompt) error {
	util.AssertNotNil(prompt, "Prompt must not be nil")

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if prompt already exists
	for i, p := range s.features.AvailablePrompts {
		if p.Name == prompt.Name {
			// Replace the existing prompt
			s.features.AvailablePrompts[i] = prompt
			return s.BroadcastPromptsChanged()
		}
	}

	// Add new prompt
	s.features.AvailablePrompts = append(s.features.AvailablePrompts, prompt)
	return s.BroadcastPromptsChanged()
}

// RemoveTool implements McpAsyncServer
func (s *asyncServerImpl) RemoveTool(name string) error {
	util.AssertNotNil(name, "Tool name must not be nil")

	s.mu.Lock()
	defer s.mu.Unlock()

	// Find and remove the tool
	for i, tool := range s.features.AvailableTools {
		if tool.Name == name {
			// Remove the tool at index i
			s.features.AvailableTools = append(s.features.AvailableTools[:i], s.features.AvailableTools[i+1:]...)
			return s.BroadcastToolsChanged()
		}
	}

	return fmt.Errorf("tool not found: %s", name)
}

// RemoveResource implements McpAsyncServer
func (s *asyncServerImpl) RemoveResource(uri string) error {
	util.AssertNotNil(uri, "Resource URI must not be nil")

	s.mu.Lock()
	defer s.mu.Unlock()

	// Find and remove the resource
	for i, resource := range s.features.AvailableResources {
		if resource.URI == uri {
			// Remove the resource at index i
			s.features.AvailableResources = append(s.features.AvailableResources[:i], s.features.AvailableResources[i+1:]...)
			return s.BroadcastResourcesChanged()
		}
	}

	return fmt.Errorf("resource not found: %s", uri)
}

// RemovePrompt implements McpAsyncServer
func (s *asyncServerImpl) RemovePrompt(name string) error {
	util.AssertNotNil(name, "Prompt name must not be nil")

	s.mu.Lock()
	defer s.mu.Unlock()

	// Find and remove the prompt
	for i, prompt := range s.features.AvailablePrompts {
		if prompt.Name == name {
			// Remove the prompt at index i
			s.features.AvailablePrompts = append(s.features.AvailablePrompts[:i], s.features.AvailablePrompts[i+1:]...)
			return s.BroadcastPromptsChanged()
		}
	}

	return fmt.Errorf("prompt not found: %s", name)
}

// handleMessages is the main message processing goroutine
func (s *asyncServerImpl) handleMessages() {
	for s.IsRunning() {
		msg, session, err := s.transport.ReceiveMessage()
		if err != nil {
			// Log error but continue processing
			fmt.Printf("Error receiving message: %v\n", err)
			continue
		}

		// Register client session if new
		s.registerClientSession(session)

		// Process message in a new goroutine
		go s.processMessage(msg, session)
	}
}

// registerClientSession registers a client session if it's new
func (s *asyncServerImpl) registerClientSession(session spec.McpClientSession) {
	clientID := session.GetClientInfo().GetClientID()
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()

	if _, exists := s.clients[clientID]; !exists {
		s.clients[clientID] = session
	}
}

// unregisterClientSession removes a client session
func (s *asyncServerImpl) unregisterClientSession(session spec.McpClientSession) {
	clientID := session.GetClientInfo().GetClientID()
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()

	delete(s.clients, clientID)
}

// processMessage processes an incoming message
func (s *asyncServerImpl) processMessage(msg interface{}, session spec.McpClientSession) {
	switch request := msg.(type) {
	case *spec.ContentRequest:
		s.handleContentRequest(request, session)
	case *spec.ToolExecutionRequest:
		s.handleToolExecutionRequest(request, session)
	case *spec.ResourceRequest:
		s.handleResourceRequest(request, session)
	case *spec.PromptRequest:
		s.handlePromptRequest(request, session)
	case *spec.CloseSessionRequest:
		s.handleCloseSessionRequest(request, session)
	case *spec.CancelRequest:
		s.handleCancelRequest(request, session)
	default:
		// Unknown message type
		errMsg := fmt.Sprintf("unsupported message type: %T", msg)
		errorResponse := &spec.ErrorResponse{
			Error: &spec.McpError{
				Code:    spec.ErrCodeInvalidRequest,
				Message: errMsg,
			},
		}
		_ = session.SendMessage(errorResponse)
	}
}

// handleContentRequest handles a content request asynchronously
func (s *asyncServerImpl) handleContentRequest(request *spec.ContentRequest, session spec.McpClientSession) {
	if s.createMessageHandler == nil {
		errorResponse := &spec.ErrorResponse{
			Error: &spec.McpError{
				Code:    spec.ErrCodeServerError,
				Message: "no message handler configured",
			},
		}
		_ = session.SendMessage(errorResponse)
		return
	}

	responseSender := &defaultResponseSender{
		session: session,
		request: request,
	}
	contentPublisher := &defaultContentPublisher{
		session:   session,
		requestID: request.RequestID,
	}

	exchange := NewDefaultAsyncServerExchange(request, responseSender, contentPublisher)

	// Create a context with timeout and store it for potential cancellation
	ctx, cancel := context.WithTimeout(context.Background(), s.requestTimeout)

	// Store the cancel function in server to allow explicit cancellation
	// This maps request ID to a cancel function to enable explicit cancellation
	operationID := request.RequestID
	if operationID == "" {
		operationID = uuid.New().String()
	}

	// Register this operation in a registry of active operations
	s.mu.Lock()
	if s.activeOperations == nil {
		s.activeOperations = make(map[string]context.CancelFunc)
	}
	s.activeOperations[operationID] = cancel
	s.mu.Unlock()

	// Ensure cleanup when done
	defer func() {
		s.mu.Lock()
		delete(s.activeOperations, operationID)
		s.mu.Unlock()
		cancel() // Ensure context is cancelled when done
	}()

	// Handle the request asynchronously
	go func() {
		defer func() {
			if r := recover(); r != nil {
				errorMsg := fmt.Sprintf("panic in content handler: %v", r)
				_ = exchange.SendError(errors.New(errorMsg))
			}
		}()

		// Create a proper CreateMessageRequest from the ContentRequest
		createMsgReq := spec.NewCreateMessageRequestBuilder().Metadata(map[string]interface{}{
			"requestId":   request.RequestID,
			"operationId": operationID,
		}).Content(request.SystemPrompt).Build()

		// Pass along any provided metadata from the request
		if request.Metadata != nil {
			for k, v := range request.Metadata {
				createMsgReq.Metadata[k] = v
			}
		}

		resultCh, errCh := s.createMessageHandler.(AsyncCreateMessageHandler)(ctx, createMsgReq)

		// Handle the result or error with cancellation support
		select {
		case response := <-resultCh:
			if response != nil {
				_ = exchange.SendResponse(response)
			} else {
				_ = exchange.SendError(errors.New("received nil response from create message handler"))
			}
		case err := <-errCh:
			_ = exchange.SendError(err)
		case <-ctx.Done():
			if ctx.Err() == context.Canceled {
				// This was an explicit cancellation
				_ = exchange.SendError(&spec.McpError{
					Code:    spec.ErrCodeOperationCancelled,
					Message: "Operation was cancelled by client request",
				})
			} else {
				// This was a timeout
				_ = exchange.SendError(&spec.McpError{
					Code:    spec.ErrCodeTimeoutError,
					Message: "Operation timed out",
				})
			}
		}
	}()
}

// handleToolExecutionRequest handles a tool execution request asynchronously
func (s *asyncServerImpl) handleToolExecutionRequest(request *spec.ToolExecutionRequest, session spec.McpClientSession) {
	handler, ok := s.toolHandlers[request.ToolName]
	if !ok {
		errorResponse := &spec.ErrorResponse{
			Error: &spec.McpError{
				Code:    spec.ErrCodeInvalidRequest,
				Message: fmt.Sprintf("no handler for tool: %s", request.ToolName),
			},
		}
		_ = session.SendMessage(errorResponse)
		return
	}

	responseSender := &defaultResponseSender{
		session: session,
		request: request,
	}
	contentPublisher := &defaultContentPublisher{
		session:   session,
		requestID: request.RequestID,
	}

	exchange := NewDefaultAsyncServerExchange(request, responseSender, contentPublisher)

	// Create a context with timeout and store it for potential cancellation
	ctx, cancel := context.WithTimeout(context.Background(), s.requestTimeout)

	// Generate an operation ID for this tool execution if not provided
	operationID := request.ToolID
	if operationID == "" {
		operationID = fmt.Sprintf("tool-%s", uuid.New().String())
	}

	// Register this operation in a registry of active operations
	s.mu.Lock()
	if s.activeOperations == nil {
		s.activeOperations = make(map[string]context.CancelFunc)
	}
	s.activeOperations[operationID] = cancel
	s.mu.Unlock()

	// Ensure cleanup when done
	defer func() {
		s.mu.Lock()
		delete(s.activeOperations, operationID)
		s.mu.Unlock()
		cancel() // Ensure context is cancelled when done
	}()

	// Handle the request asynchronously
	go func() {
		defer func() {
			if r := recover(); r != nil {
				errorMsg := fmt.Sprintf("panic in tool handler: %v", r)
				_ = exchange.SendError(errors.New(errorMsg))
			}
		}()

		// Marshal the tool arguments to JSON for the handler
		paramsJSON, err := json.Marshal(request.ToolArguments)
		if err != nil {
			_ = exchange.SendError(fmt.Errorf("failed to marshal tool arguments: %w", err))
			return
		}

		// Call the async tool handler and handle the results
		resultCh, errCh := handler.(AsyncToolHandler)(ctx, paramsJSON)

		// Process the result or error
		select {
		case result := <-resultCh:
			if result != nil {
				// Create a tool result from the execution
				toolResult := &spec.ToolResult{
					Content: result,
				}
				// Create and send the response
				response := &spec.ToolExecutionResponse{
					ToolResult: toolResult,
				}
				_ = exchange.SendResponse(response)
			} else {
				_ = exchange.SendError(errors.New("received nil result from tool handler"))
			}
		case err := <-errCh:
			_ = exchange.SendError(err)
		case <-ctx.Done():
			if ctx.Err() == context.Canceled {
				// This was an explicit cancellation
				_ = exchange.SendError(&spec.McpError{
					Code:    spec.ErrCodeOperationCancelled,
					Message: "Tool execution was cancelled by client request",
				})
			} else {
				// This was a timeout
				_ = exchange.SendError(&spec.McpError{
					Code:    spec.ErrCodeTimeoutError,
					Message: "Tool execution timed out",
				})
			}
		}
	}()
}

// handleResourceRequest handles a resource request asynchronously
func (s *asyncServerImpl) handleResourceRequest(request *spec.ResourceRequest, session spec.McpClientSession) {
	if s.resourceHandler == nil {
		errorResponse := &spec.ErrorResponse{
			Error: &spec.McpError{
				Code:    spec.ErrCodeServerError,
				Message: "no resource handler configured",
			},
		}
		_ = session.SendMessage(errorResponse)
		return
	}

	responseSender := &defaultResponseSender{
		session: session,
		request: request,
	}
	contentPublisher := &defaultContentPublisher{
		session:   session,
		requestID: request.RequestID,
	}

	exchange := NewDefaultAsyncServerExchange(request, responseSender, contentPublisher)

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), s.requestTimeout)
	defer cancel()

	// Handle the request asynchronously
	go func() {
		defer func() {
			if r := recover(); r != nil {
				errorMsg := fmt.Sprintf("panic in resource handler: %v", r)
				_ = exchange.SendError(errors.New(errorMsg))
			}
		}()

		// Call the async resource handler
		contentCh, errCh := s.resourceHandler(ctx, request.URI)

		// Process the result or error
		select {
		case content := <-contentCh:
			if content != nil {
				// Create a ReadResourceResult object with proper resource contents
				// Use TextResourceContents as a default format
				resourceContents := &spec.TextResourceContents{
					URI:      request.URI,
					MimeType: detectMimeType(content),
					Text:     string(content),
				}

				result := &spec.ReadResourceResult{
					Contents: []spec.ResourceContents{resourceContents},
				}

				_ = exchange.SendResponse(result)
			} else {
				_ = exchange.SendError(errors.New("received nil content from resource handler"))
			}
		case err := <-errCh:
			_ = exchange.SendError(err)
		case <-ctx.Done():
			_ = exchange.SendError(ctx.Err())
		}
	}()
}

// detectMimeType returns a MIME type based on the content
func detectMimeType(content []byte) string {
	// Simple detection - could be improved
	if looksLikeBinary(content) {
		return "application/octet-stream"
	}
	return "text/plain"
}

// looksLikeBinary checks if content appears to be binary
func looksLikeBinary(content []byte) bool {
	// Check for common binary file signatures or null bytes
	for _, b := range content {
		if b == 0 {
			return true
		}
	}
	return false
}

// handlePromptRequest handles a prompt request asynchronously
func (s *asyncServerImpl) handlePromptRequest(request *spec.PromptRequest, session spec.McpClientSession) {
	if s.promptHandler == nil {
		errorResponse := &spec.ErrorResponse{
			Error: &spec.McpError{
				Code:    spec.ErrCodeServerError,
				Message: "no prompt handler configured",
			},
		}
		_ = session.SendMessage(errorResponse)
		return
	}

	responseSender := &defaultResponseSender{
		session: session,
		request: request,
	}
	contentPublisher := &defaultContentPublisher{
		session:   session,
		requestID: request.RequestID,
	}

	exchange := NewDefaultAsyncServerExchange(request, responseSender, contentPublisher)

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), s.requestTimeout)
	defer cancel()

	// Handle the request asynchronously
	go func() {
		defer func() {
			if r := recover(); r != nil {
				errorMsg := fmt.Sprintf("panic in prompt handler: %v", r)
				_ = exchange.SendError(errors.New(errorMsg))
			}
		}()

		// Call the async prompt handler and handle the results
		// Use Parameters field instead of Arguments
		resultCh, errCh := s.promptHandler(ctx, request.Name, request.Parameters)

		// Process the result or error
		select {
		case promptText := <-resultCh:
			if promptText != "" {
				// Create a prompt result from the execution
				result := &spec.GetPromptResult{
					Messages: []spec.PromptMessage{
						{
							Role: spec.RoleUser,
							Content: &spec.TextContent{
								Text: promptText,
							},
						},
					},
				}
				// Send the result directly since there's no PromptResponse type
				_ = exchange.SendResponse(result)
			} else {
				_ = exchange.SendError(errors.New("received empty prompt text from handler"))
			}
		case err := <-errCh:
			_ = exchange.SendError(err)
		case <-ctx.Done():
			_ = exchange.SendError(ctx.Err())
		}
	}()
}

// handleCloseSessionRequest handles a request to close the session
func (s *asyncServerImpl) handleCloseSessionRequest(_ *spec.CloseSessionRequest, session spec.McpClientSession) {
	// Send acknowledgment
	response := &spec.CloseSessionResponse{}
	_ = session.SendMessage(response)

	// Unregister and close the session
	s.unregisterClientSession(session)
	_ = session.Close()
}

// handleCancelRequest handles a request to cancel an ongoing operation
func (s *asyncServerImpl) handleCancelRequest(request *spec.CancelRequest, session spec.McpClientSession) {
	// Create the response object
	response := &spec.CancelResult{
		Success: false,
		Message: "Operation not found or already completed",
	}

	// First try direct cancellation using our operation registry
	s.mu.Lock()
	if cancelFunc, exists := s.activeOperations[request.OperationID]; exists {
		// Found the operation, cancel it directly
		cancelFunc()                                    // This will cause the context to be cancelled
		delete(s.activeOperations, request.OperationID) // Clean up the map
		response.Success = true
		response.Message = "Operation cancelled successfully"
		s.mu.Unlock()
		_ = session.SendMessage(response)
		return
	}
	s.mu.Unlock()

	// If no direct match, try the specialized cancellation interfaces
	switch request.Type {
	case "message", "message-stream":
		// Cancel any ongoing message creation operations with the given ID
		if s.createMessageHandler != nil {
			// If we have a handler that supports cancellation, try to cancel the operation
			if canceller, ok := s.createMessageHandler.(interface {
				CancelMessageCreation(operationID string) bool
			}); ok {
				if canceller.CancelMessageCreation(request.OperationID) {
					response.Success = true
					response.Message = "Message creation operation cancelled"
				}
			}
		}
	case "tool":
		// Cancel any ongoing tool execution operations with the given ID
		if toolID := extractToolID(request.OperationID); toolID != "" {
			if handler, exists := s.toolHandlers[toolID]; exists {
				// If the tool handler supports cancellation, use it
				if canceller, ok := handler.(interface {
					CancelExecution(operationID string) bool
				}); ok {
					if canceller.CancelExecution(request.OperationID) {
						response.Success = true
						response.Message = "Tool execution operation cancelled"
					}
				}
			}
		}
	default:
		// For other operation types or if no type is specified,
		// check with any handlers that implement the CancelOperation interface
		if s.createMessageHandler != nil {
			if canceller, ok := s.createMessageHandler.(interface {
				CancelOperation(operationID string) bool
			}); ok {
				if canceller.CancelOperation(request.OperationID) {
					response.Success = true
					response.Message = "Operation cancelled"
				}
			}
		}

		// Check tool handlers that might implement generic cancellation
		for _, handler := range s.toolHandlers {
			if canceller, ok := handler.(interface {
				CancelOperation(operationID string) bool
			}); ok {
				if canceller.CancelOperation(request.OperationID) {
					response.Success = true
					response.Message = "Operation cancelled"
					break
				}
			}
		}
	}

	// Send the response
	_ = session.SendMessage(response)
}

// extractToolID extracts the tool ID from an operation ID formatted as "tool-{toolID}"
func extractToolID(operationID string) string {
	if strings.HasPrefix(operationID, "tool-") {
		return strings.TrimPrefix(operationID, "tool-")
	}
	return ""
}

// defaultResponseSender implements ResponseSender
type defaultResponseSender struct {
	session spec.McpClientSession
	request interface{}
}

// SendResponse sends a response to the client
func (s *defaultResponseSender) SendResponse(response interface{}) error {
	return s.session.SendMessage(response)
}

// SendError sends an error response to the client
func (s *defaultResponseSender) SendError(err error) error {
	mcpErr, ok := err.(*spec.McpError)
	if !ok {
		mcpErr = &spec.McpError{
			Code:    spec.ErrCodeServerError,
			Message: err.Error(),
		}
	}

	errorResponse := &spec.ErrorResponse{
		Error: mcpErr,
	}

	return s.session.SendMessage(errorResponse)
}

// defaultContentPublisher implements ContentPublisher
type defaultContentPublisher struct {
	session   spec.McpClientSession
	requestID string
}

// PublishContent publishes content to the client
func (p *defaultContentPublisher) PublishContent(content spec.Content) error {
	contentEvent := &spec.ContentEvent{
		RequestID: p.requestID,
		Content:   content,
	}
	return p.session.SendMessage(contentEvent)
}

// CompleteContent signals completion of content streaming
func (p *defaultContentPublisher) CompleteContent() error {
	completeEvent := &spec.ContentCompleteEvent{
		RequestID: p.requestID,
	}
	return p.session.SendMessage(completeEvent)
}

// ErrorContent signals an error in content streaming
func (p *defaultContentPublisher) ErrorContent(err error) error {
	mcpErr, ok := err.(*spec.McpError)
	if !ok {
		mcpErr = &spec.McpError{
			Code:    spec.ErrCodeServerError,
			Message: err.Error(),
		}
	}

	errorEvent := &spec.ContentErrorEvent{
		RequestID: p.requestID,
		Error:     mcpErr,
	}

	return p.session.SendMessage(errorEvent)
}
