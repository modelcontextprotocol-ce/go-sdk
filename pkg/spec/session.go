// Package spec provides the core interfaces and types for the Model Context Protocol (MCP).
package spec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"
)

// Common errors
var (
	ErrSessionClosed = errors.New("session closed")
	ErrTimeout       = errors.New("request timed out")
	ErrInvalidMethod = errors.New("invalid method")
	ErrInvalidParams = errors.New("invalid parameters")

	// Additional errors for robust client session implementation
	ErrResponseWithoutID            = errors.New("response without ID")
	ErrResponseWithoutResultOrError = errors.New("response without result or error")
)

// pendingRequest represents a request waiting for a response
type pendingRequest struct {
	resultType reflect.Type
	resultCh   chan interface{}
	errCh      chan error
	ctx        context.Context
	cancel     context.CancelFunc
}

// ClientInfo defines information about a client in the MCP protocol
type ClientInfo interface {
	// GetClientName returns the name of the client
	GetClientName() string

	// GetClientVersion returns the version of the client
	GetClientVersion() string

	// GetClientID returns the unique identifier of the client
	GetClientID() string

	// GetInfo returns the implementation information
	GetInfo() Implementation
}

// DefaultClientInfo provides a default implementation of ClientInfo
type DefaultClientInfo struct {
	Implementation
	ID string
}

// GetClientName returns the name of the client
func (c *DefaultClientInfo) GetClientName() string {
	return c.Name
}

// GetClientVersion returns the version of the client
func (c *DefaultClientInfo) GetClientVersion() string {
	return c.Version
}

// GetClientID returns the unique identifier of the client
func (c *DefaultClientInfo) GetClientID() string {
	return c.ID
}

// GetInfo returns the implementation information
func (c *DefaultClientInfo) GetInfo() Implementation {
	return c.Implementation
}

// NewDefaultClientInfo creates a new DefaultClientInfo
func NewDefaultClientInfo(impl Implementation, id string) ClientInfo {
	return &DefaultClientInfo{
		Implementation: impl,
		ID:             id,
	}
}

// DefaultTimeout is the default timeout for requests
const DefaultTimeout = 30 * time.Second

// McpSession defines the common functionality for server and client sessions
type McpSession interface {
	// Close closes the session
	Close() error
	// IsClosed returns true if the session is closed
	IsClosed() bool
}

// McpServerSession defines the interface for server sessions
type McpServerSession interface {
	McpSession
	// HandleRequest handles an incoming request
	HandleRequest(ctx context.Context, method string, params json.RawMessage) (interface{}, error)
	// SendNotification sends a notification to the client
	SendNotification(ctx context.Context, method string, params interface{}) error
}

// McpClientSession defines the interface for client sessions
type McpClientSession interface {
	McpSession

	// GetID returns the unique identifier for this client session
	GetID() string

	// GetClientInfo returns information about the client
	GetClientInfo() ClientInfo

	// SendRequest sends a request to the server and returns the response
	SendRequest(ctx context.Context, method string, params interface{}, result interface{}) error

	// SendMessage sends a JSON-RPC message and returns the response
	SendMessage(message interface{}) error

	// SendNotification sends a notification to the server
	SendNotification(ctx context.Context, method string, params interface{}) error

	// SetNotificationHandler sets the handler for incoming notifications
	SetNotificationHandler(method string, handler NotificationHandler)

	// RemoveNotificationHandler removes a notification handler
	RemoveNotificationHandler(method string)

	// NotifyToolsListChanged sends a notification that the tools list has changed
	NotifyToolsListChanged() error

	// NotifyResourcesListChanged sends a notification that the resources list has changed
	NotifyResourcesListChanged() error

	// NotifyResourceChanged sends a notification that a specific resource has changed
	NotifyResourceChanged(uri string, contents []byte) error

	// NotifyPromptsListChanged sends a notification that the prompts list has changed
	NotifyPromptsListChanged() error

	// SendLogMessage sends a log message to the server
	SendLogMessage(level LogLevel, message string, logger string) error

	// CloseGracefully closes the session gracefully, waiting for pending operations to complete
	CloseGracefully(ctx context.Context) error

	// AddSubscription adds a resource subscription to the session
	AddSubscription(uri string) error

	// RemoveSubscription removes a resource subscription from the session
	RemoveSubscription(uri string) error

	// GetSubscriptions returns the list of resource subscriptions for the session
	GetSubscriptions() []string

	// SetStreamingHandler registers a handler for streaming message results
	SetStreamingHandler(requestID string, handler func(*CreateMessageResult))

	// RemoveStreamingHandler removes a streaming handler
	RemoveStreamingHandler(requestID string)

	// RegisterStreamHandler registers a handler for streaming tool results
	RegisterStreamHandler(streamID string, handler func(*StreamingToolResult))

	// UnregisterStreamHandler removes a streaming tool handler
	UnregisterStreamHandler(streamID string)
}

// NotificationHandler defines the function signature for notification handlers
type NotificationHandler func(ctx context.Context, params json.RawMessage) error

// BaseSession provides the common functionality for server and client sessions
type BaseSession struct {
	transport McpTransport
	closed    bool
	mu        sync.Mutex
}

// NewBaseSession creates a new base session
func NewBaseSession(transport McpTransport) *BaseSession {
	return &BaseSession{
		transport: transport,
		closed:    false,
	}
}

// Close closes the session
func (s *BaseSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	return s.transport.Close()
}

// IsClosed returns true if the session is closed
func (s *BaseSession) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// Send sends a message over the transport
func (s *BaseSession) Send(message []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrSessionClosed
	}

	return s.transport.Send(message)
}

// ClientSession implements the McpClientSession interface
type ClientSession struct {
	*BaseSession
	nextID               int64
	pendingRequests      map[string]*pendingRequest
	notificationHandlers map[string]NotificationHandler
	streamingHandlers    map[string]func(*CreateMessageResult)
	subscriptions        map[string]bool // Track subscribed resource URIs
	mu                   sync.Mutex
}

// StreamingToolHandlers holds the registered streaming tool handlers
type streamHandlers struct {
	handlers map[string]func(*StreamingToolResult)
	mu       sync.RWMutex
}

// Add a private field to ClientSession to store stream handlers
var (
	streamHandlersSingleton = &streamHandlers{
		handlers: make(map[string]func(*StreamingToolResult)),
	}
)

// NewClientSession creates a new client session
func NewClientSession(transport McpTransport) (*ClientSession, error) {
	if transport == nil {
		return nil, fmt.Errorf("transport cannot be nil")
	}

	session := &ClientSession{
		BaseSession:          NewBaseSession(transport),
		nextID:               1,
		pendingRequests:      make(map[string]*pendingRequest),
		notificationHandlers: make(map[string]NotificationHandler),
		streamingHandlers:    make(map[string]func(*CreateMessageResult)),
		subscriptions:        make(map[string]bool),
	}

	// If this is a client transport, establish the connection and set the message handler
	if clientTransport, ok := transport.(McpClientTransport); ok {
		err := clientTransport.Connect(context.Background(), func(msg *JSONRPCMessage) (*JSONRPCMessage, error) {
			return nil, session.HandleMessage([]byte{})
		})
		if err != nil {
			return nil, fmt.Errorf("failed to connect transport: %w", err)
		}
	}

	return session, nil
}

// SendRequest sends a request to the server and returns the response
func (s *ClientSession) SendRequest(ctx context.Context, method string, params interface{}, result interface{}) error {
	if ctx == nil {
		ctx = context.Background()
	}

	// Set timeout if not already set
	_, hasTimeout := ctx.Deadline()
	if !hasTimeout {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultTimeout)
		defer cancel()
	}

	// Generate request ID
	s.mu.Lock()
	id := fmt.Sprintf("%d", s.nextID)
	s.nextID++
	s.mu.Unlock()

	// Marshal parameters if provided
	var paramsJSON json.RawMessage
	if params != nil {
		var err error
		paramsJSON, err = json.Marshal(params)
		if err != nil {
			return fmt.Errorf("failed to marshal params: %w", err)
		}
	}

	// Create request message
	rawID := json.RawMessage(fmt.Sprintf("%q", id))
	request := &JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		ID:      &rawID,
		Method:  method,
		Params:  paramsJSON,
	}

	// Marshal the request
	requestBytes, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create channels for communication
	resultCh := make(chan interface{}, 1)
	errCh := make(chan error, 1)

	// Determine the result type for unmarshaling
	var resultType reflect.Type
	if result != nil {
		resultType = reflect.TypeOf(result).Elem()
	} else {
		// Use interface{} when result is nil
		resultType = reflect.TypeOf((*interface{})(nil)).Elem()
	}

	// Create the pending request
	reqCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	pendingReq := &pendingRequest{
		resultType: resultType,
		resultCh:   resultCh,
		errCh:      errCh,
		ctx:        reqCtx,
		cancel:     cancel,
	}

	// Register the pending request
	s.mu.Lock()
	if s.IsClosed() {
		s.mu.Unlock()
		return ErrSessionClosed
	}
	s.pendingRequests[id] = pendingReq
	s.mu.Unlock()

	// Clean up when done
	defer func() {
		s.mu.Lock()
		delete(s.pendingRequests, id)
		s.mu.Unlock()
	}()

	// Send the request
	if err := s.Send(requestBytes); err != nil {
		return err
	}

	// Wait for the response or timeout
	select {
	case res := <-resultCh:
		// If result is provided, set it
		if result != nil && res != nil {
			resultValue := reflect.ValueOf(result).Elem()
			resultValue.Set(reflect.ValueOf(res))
		}
		return nil
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return fmt.Errorf("%w: %s", ErrTimeout, ctx.Err())
	}
}

// HandleMessage handles incoming messages for the client session
func (s *ClientSession) HandleMessage(message []byte) error {
	var msg JSONRPCMessage
	if err := json.Unmarshal(message, &msg); err != nil {
		return fmt.Errorf("failed to unmarshal message: %w", err)
	}

	// Handle response
	if msg.IsResponse() {
		if msg.ID == nil {
			return ErrResponseWithoutID
		}

		s.mu.Lock()
		pendingReq, ok := s.pendingRequests[string(*msg.ID)]
		s.mu.Unlock()

		if !ok {
			return nil
		}

		if msg.Error != nil {
			pendingReq.errCh <- msg.Error
		} else if len(msg.Result) > 0 {
			result := reflect.New(pendingReq.resultType).Interface()
			if err := json.Unmarshal(msg.Result, result); err != nil {
				pendingReq.errCh <- fmt.Errorf("failed to unmarshal result: %w", err)
			} else {
				pendingReq.resultCh <- result
			}
		} else {
			pendingReq.errCh <- ErrResponseWithoutResultOrError
		}
		return nil
	}

	// Handle notification
	if msg.IsNotification() {
		s.mu.Lock()
		handler, ok := s.notificationHandlers[msg.Method]
		s.mu.Unlock()

		if ok {
			return handler(context.Background(), msg.Params)
		}
		return nil
	}

	// Ignore other message types
	return nil
}

// SetNotificationHandler sets the handler for incoming notifications
func (s *ClientSession) SetNotificationHandler(method string, handler NotificationHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notificationHandlers[method] = handler
}

// RemoveNotificationHandler removes a notification handler
func (s *ClientSession) RemoveNotificationHandler(method string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.notificationHandlers, method)
}

// SendMessage sends a JSON-RPC message and returns the response
func (s *ClientSession) SendMessage(message interface{}) error {
	// Check if the session is closed
	if s.IsClosed() {
		return ErrSessionClosed
	}

	// Cast to JSONRPCMessage if possible
	jsonRpcMsg, ok := message.(*JSONRPCMessage)
	if !ok {
		// Try to marshal and then unmarshal to convert to JSONRPCMessage
		data, err := json.Marshal(message)
		if err != nil {
			return fmt.Errorf("failed to marshal message: %w", err)
		}
		jsonRpcMsg = &JSONRPCMessage{}
		if err := json.Unmarshal(data, jsonRpcMsg); err != nil {
			return fmt.Errorf("failed to unmarshal to JSONRPCMessage: %w", err)
		}
	}

	// Marshal the message to bytes
	msgBytes, err := json.Marshal(jsonRpcMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal JSONRPCMessage: %w", err)
	}

	// Send the message
	return s.Send(msgBytes)
}

// NotifyToolsListChanged notifies the client that the tools list has changed
func (s *ClientSession) NotifyToolsListChanged() error {
	return s.SendNotification(context.Background(), MethodNotificationToolsListChanged, nil)
}

// NotifyResourcesListChanged notifies the client that the resources list has changed
func (s *ClientSession) NotifyResourcesListChanged() error {
	return s.SendNotification(context.Background(), MethodNotificationResourcesListChanged, nil)
}

// NotifyPromptsListChanged notifies the client that the prompts list has changed
func (s *ClientSession) NotifyPromptsListChanged() error {
	return s.SendNotification(context.Background(), MethodNotificationPromptsListChanged, nil)
}

// NotifyResourceChanged notifies the client that a resource has changed
func (s *ClientSession) NotifyResourceChanged(uri string, contents []byte) error {
	params := map[string]interface{}{
		"uri":      uri,
		"contents": contents,
	}
	return s.SendNotification(context.Background(), MethodNotificationResourceChanged, params)
}

// SendLogMessage sends a log message to the client
func (s *ClientSession) SendLogMessage(level LogLevel, message string, logger string) error {
	params := LoggingMessage{
		Level:   level,
		Message: message,
		Logger:  logger,
	}
	return s.SendNotification(context.Background(), MethodNotificationMessage, params)
}

// SendNotification sends a notification to the client
func (s *ClientSession) SendNotification(ctx context.Context, method string, params interface{}) error {
	// Marshal parameters if provided
	var paramsJSON json.RawMessage
	if params != nil {
		var err error
		paramsJSON, err = json.Marshal(params)
		if err != nil {
			return fmt.Errorf("failed to marshal params: %w", err)
		}
	}

	// Create notification message
	notification := &JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  method,
		Params:  paramsJSON,
	}

	// Marshal the notification
	notificationBytes, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	// Send the notification
	return s.Send(notificationBytes)
}

// CloseGracefully closes the session gracefully, waiting for pending operations to complete
func (s *ClientSession) CloseGracefully(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	// Create a channel to signal when all pending requests are done
	done := make(chan struct{})

	go func() {
		// Wait for all pending requests to complete or for the context to be done
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.mu.Lock()
				pendingCount := len(s.pendingRequests)
				s.mu.Unlock()

				if pendingCount == 0 {
					close(done)
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Wait for either all requests to complete or the context to be done
	select {
	case <-done:
		return s.Close()
	case <-ctx.Done():
		return fmt.Errorf("graceful close timed out: %w", ctx.Err())
	}
}

// AddSubscription adds a subscription to the specified resource URI
func (s *ClientSession) AddSubscription(uri string) error {
	if uri == "" {
		return fmt.Errorf("resource URI cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.subscriptions[uri] = true
	return nil
}

// RemoveSubscription removes a subscription from the specified resource URI
func (s *ClientSession) RemoveSubscription(uri string) error {
	if uri == "" {
		return fmt.Errorf("resource URI cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.subscriptions, uri)
	return nil
}

// GetSubscriptions returns the list of resource subscriptions for the session
func (s *ClientSession) GetSubscriptions() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	subscriptions := make([]string, 0, len(s.subscriptions))
	for uri := range s.subscriptions {
		subscriptions = append(subscriptions, uri)
	}
	return subscriptions
}

// RegisterStreamHandler registers a handler for streaming tool results
func (s *ClientSession) RegisterStreamHandler(streamID string, handler func(*StreamingToolResult)) {
	streamHandlersSingleton.mu.Lock()
	defer streamHandlersSingleton.mu.Unlock()
	streamHandlersSingleton.handlers[streamID] = handler

	// Register the notification handler for tool stream results if not already done
	s.mu.Lock()
	_, exists := s.notificationHandlers[MethodToolsStreamResult]
	s.mu.Unlock()

	if !exists {
		s.SetNotificationHandler(MethodToolsStreamResult, func(ctx context.Context, params json.RawMessage) error {
			var streamResult StreamingToolResult
			if err := json.Unmarshal(params, &streamResult); err != nil {
				return fmt.Errorf("failed to unmarshal streaming tool result: %w", err)
			}

			streamHandlersSingleton.mu.RLock()
			handler, ok := streamHandlersSingleton.handlers[streamResult.StreamID]
			streamHandlersSingleton.mu.RUnlock()

			if ok {
				handler(&streamResult)
			}

			return nil
		})
	}
}

// UnregisterStreamHandler removes a streaming tool handler
func (s *ClientSession) UnregisterStreamHandler(streamID string) {
	streamHandlersSingleton.mu.Lock()
	defer streamHandlersSingleton.mu.Unlock()
	delete(streamHandlersSingleton.handlers, streamID)
}

// GetID returns the unique identifier for this client session
func (s *ClientSession) GetID() string {
	// For a basic implementation, we can use the nextID as a unique identifier
	return fmt.Sprintf("client-session-%d", s.nextID)
}

// GetClientInfo returns information about the client
func (s *ClientSession) GetClientInfo() ClientInfo {
	// This would typically return client info that was set during session creation
	// For a basic implementation, we return a default client info
	return NewDefaultClientInfo(
		Implementation{
			Name:    "go-sdk-client",
			Version: "1.0.0",
		},
		fmt.Sprintf("client-%d", s.nextID),
	)
}

// SetStreamingHandler registers a handler for streaming message responses
func (s *ClientSession) SetStreamingHandler(requestID string, handler func(*CreateMessageResult)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.streamingHandlers[requestID] = handler
}

// RemoveStreamingHandler removes a streaming handler
func (s *ClientSession) RemoveStreamingHandler(requestID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.streamingHandlers, requestID)
}

// ServerSession implements the McpServerSession interface
type ServerSession struct {
	*BaseSession
	handlers map[string]RequestHandler
	mu       sync.Mutex
}

// RequestHandler defines the function signature for request handlers
type RequestHandler func(ctx context.Context, params json.RawMessage) (interface{}, error)

// NewServerSession creates a new server session
func NewServerSession(transport McpTransport) *ServerSession {
	return &ServerSession{
		BaseSession: NewBaseSession(transport),
		handlers:    make(map[string]RequestHandler),
	}
}

// RegisterHandler registers a handler for a specific method
func (s *ServerSession) RegisterHandler(method string, handler RequestHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[method] = handler
}

// HandleRequest handles an incoming request
func (s *ServerSession) HandleRequest(ctx context.Context, method string, params json.RawMessage) (interface{}, error) {
	s.mu.Lock()
	handler, ok := s.handlers[method]
	s.mu.Unlock()

	if !ok {
		return nil, &McpError{
			Code:    ErrCodeMethodNotFound,
			Message: fmt.Sprintf("method '%s' not found", method),
		}
	}

	return handler(ctx, params)
}

// HandleMessage handles incoming messages for the server session
func (s *ServerSession) HandleMessage(message []byte) error {
	var msg JSONRPCMessage
	if err := json.Unmarshal(message, &msg); err != nil {
		return fmt.Errorf("failed to unmarshal message: %w", err)
	}

	// Handle request
	if msg.IsRequest() {
		ctx := context.Background()
		result, err := s.HandleRequest(ctx, msg.Method, msg.Params)

		// Create response message
		response := &JSONRPCMessage{
			JSONRPC: JSONRPCVersion,
			ID:      msg.ID,
		}

		if err != nil {
			// Set error response
			mcpError, ok := err.(*McpError)
			if !ok {
				mcpError = &McpError{
					Code:    ErrCodeInternalError,
					Message: err.Error(),
				}
			}
			response.Error = &JSONRPCError{
				Code:    mcpError.Code,
				Message: mcpError.Message,
				Data:    mcpError.Data,
			}
		} else {
			// Set success response
			resultBytes, err := json.Marshal(result)
			if err != nil {
				response.Error = &JSONRPCError{
					Code:    ErrCodeInternalError,
					Message: fmt.Sprintf("failed to marshal result: %s", err),
				}
			} else {
				response.Result = resultBytes
			}
		}

		// Send the response
		responseBytes, err := json.Marshal(response)
		if err != nil {
			return fmt.Errorf("failed to marshal response: %w", err)
		}
		return s.Send(responseBytes)
	}

	// Ignore other message types
	return nil
}

// SendNotification sends a notification to the client
func (s *ServerSession) SendNotification(ctx context.Context, method string, params interface{}) error {
	// Marshal parameters if provided
	var paramsJSON json.RawMessage
	if params != nil {
		var err error
		paramsJSON, err = json.Marshal(params)
		if err != nil {
			return fmt.Errorf("failed to marshal params: %w", err)
		}
	}

	// Create notification message
	notification := &JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  method,
		Params:  paramsJSON,
	}

	// Marshal the notification
	notificationBytes, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	// Send the notification
	return s.Send(notificationBytes)
}
