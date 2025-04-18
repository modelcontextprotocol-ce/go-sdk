package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/pkg/spec"
	"github.com/modelcontextprotocol/go-sdk/pkg/util"
)

var (
	// ErrSessionClosed is returned when attempting to use a closed session
	ErrSessionClosed = errors.New("session is closed")

	// ErrResponseWithoutID is returned when receiving a response without an ID
	ErrResponseWithoutID = errors.New("response without ID")

	// ErrResponseWithoutResultOrError is returned when receiving a response without a result or error
	ErrResponseWithoutResultOrError = errors.New("response without result or error")

	// ErrTimeout is returned when a request times out
	ErrTimeout = errors.New("request timed out")
)

// clientSession implements the spec.McpClientSession interface for client-side communication
type clientSession struct {
	transport            spec.McpClientTransport
	nextID               int64
	handlers             map[string]pendingRequest
	notificationHandlers map[string]spec.NotificationHandler
	streamingHandlers    map[string]func(*spec.CreateMessageResult)
	streamHandlers       map[string]func(*spec.StreamingToolResult)
	clientInfo           spec.ClientInfo
	mu                   sync.RWMutex
	closed               bool
	defaultTimeout       time.Duration
	subscriptions        map[string]bool
}

// pendingRequest represents a request waiting for a response
type pendingRequest struct {
	resultType reflect.Type
	resultCh   chan interface{}
	errCh      chan error
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewClientSession creates a new client session with the given transport
func NewClientSession(transport spec.McpClientTransport, defaultTimeout time.Duration) (spec.McpClientSession, error) {
	util.AssertNotNil(transport, "transport cannot be nil")

	if defaultTimeout <= 0 {
		defaultTimeout = spec.DefaultTimeout // Use spec default timeout
	}

	// Create client info with a unique ID
	clientInfo := spec.NewDefaultClientInfo(
		spec.Implementation{
			Name:    "go-sdk-client",
			Version: "1.0.0",
		},
		util.GenerateUUID(),
	)

	session := &clientSession{
		transport:            transport,
		nextID:               1,
		handlers:             make(map[string]pendingRequest),
		notificationHandlers: make(map[string]spec.NotificationHandler),
		streamingHandlers:    make(map[string]func(*spec.CreateMessageResult)),
		streamHandlers:       make(map[string]func(*spec.StreamingToolResult)),
		clientInfo:           clientInfo,
		defaultTimeout:       defaultTimeout,
		subscriptions:        make(map[string]bool),
	}

	// Connect the transport with the message handler
	err := transport.Connect(context.Background(), session.handleMessage)
	if err != nil {
		return nil, fmt.Errorf("failed to connect transport: %w", err)
	}

	return session, nil
}

// GetID returns the unique identifier for this client session
func (s *clientSession) GetID() string {
	return s.clientInfo.GetClientID()
}

// GetClientInfo returns information about the client
func (s *clientSession) GetClientInfo() spec.ClientInfo {
	return s.clientInfo
}

// IsClosed returns whether the session is closed
func (s *clientSession) IsClosed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.closed
}

// SendRequest sends a request to the counterparty and expects a response
func (s *clientSession) SendRequest(ctx context.Context, method string, params interface{}, resultPtr interface{}) error {
	resultCh, errCh := s.SendRequestAsync(ctx, method, params, reflect.TypeOf(resultPtr).Elem())

	// Wait for response
	select {
	case result := <-resultCh:
		// Copy the result to the pointer
		reflect.ValueOf(resultPtr).Elem().Set(reflect.ValueOf(result).Elem())
		return nil
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SendRequestAsync sends a request to the counterparty and returns channels for the response
func (s *clientSession) SendRequestAsync(ctx context.Context, method string, params interface{}, resultType reflect.Type) (chan interface{}, chan error) {
	resultCh := make(chan interface{}, 1)
	errCh := make(chan error, 1)

	// Check if the session is closed
	if s.IsClosed() {
		errCh <- ErrSessionClosed
		close(resultCh)
		return resultCh, errCh
	}

	// Generate unique ID for this request
	id := atomic.AddInt64(&s.nextID, 1)
	idStr := fmt.Sprintf("%d", id)
	idJSON := json.RawMessage(`"` + idStr + `"`)

	// Create a new request context with cancellation
	reqCtx, cancel := context.WithCancel(ctx)

	// Register the handler for the response
	s.mu.Lock()
	s.handlers[idStr] = pendingRequest{
		resultType: resultType,
		resultCh:   resultCh,
		errCh:      errCh,
		ctx:        reqCtx,
		cancel:     cancel,
	}
	s.mu.Unlock()

	// Create the JSON-RPC message
	var paramsJSON json.RawMessage
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			s.mu.Lock()
			delete(s.handlers, idStr)
			s.mu.Unlock()
			cancel()
			errCh <- fmt.Errorf("failed to marshal parameters: %w", err)
			close(resultCh)
			return resultCh, errCh
		}
		paramsJSON = data
	}

	message := &spec.JSONRPCMessage{
		JSONRPC: spec.JSONRPCVersion,
		ID:      &idJSON,
		Method:  method,
		Params:  paramsJSON,
	}

	// Handle context cancellation
	go func() {
		<-reqCtx.Done()
		// If this happens because of a response being received,
		// the handler will already have been removed
		s.mu.Lock()
		if _, exists := s.handlers[idStr]; exists {
			delete(s.handlers, idStr)
			errCh <- ctx.Err()
			close(resultCh)
		}
		s.mu.Unlock()
	}()

	// Send the message through the transport
	response, err := s.transport.SendMessage(ctx, message)
	if err != nil {
		s.mu.Lock()
		if handler, exists := s.handlers[idStr]; exists {
			delete(s.handlers, idStr)
			handler.cancel()
		}
		s.mu.Unlock()
		errCh <- fmt.Errorf("failed to send message: %w", err)
		close(resultCh)
		return resultCh, errCh
	}

	// Process the response if available immediately
	if response != nil {
		s.handleResponse(response, idStr)
	}

	return resultCh, errCh
}

// SetNotificationHandler sets a handler for a specific notification method
func (s *clientSession) SetNotificationHandler(method string, handler spec.NotificationHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notificationHandlers[method] = handler
}

// RemoveNotificationHandler removes a notification handler for a specific method
func (s *clientSession) RemoveNotificationHandler(method string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.notificationHandlers, method)
}

// SetStreamingHandler registers a handler for streaming message responses
func (s *clientSession) SetStreamingHandler(requestID string, handler func(*spec.CreateMessageResult)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create the map if it doesn't exist
	if s.streamingHandlers == nil {
		s.streamingHandlers = make(map[string]func(*spec.CreateMessageResult))
	}

	// Register the handler for this request ID
	s.streamingHandlers[requestID] = handler

	// Register the notification handler for message stream results if not already done
	_, exists := s.notificationHandlers[spec.MethodMessageStreamResult]
	if !exists {
		s.notificationHandlers[spec.MethodMessageStreamResult] = func(ctx context.Context, params json.RawMessage) error {
			var streamResult spec.CreateMessageResult
			if err := json.Unmarshal(params, &streamResult); err != nil {
				return fmt.Errorf("failed to unmarshal streaming message result: %w", err)
			}

			// Get the request ID from metadata
			var requestID string
			if streamResult.Metadata != nil {
				if reqID, ok := streamResult.Metadata["requestId"].(string); ok {
					requestID = reqID
				}
			}

			if requestID != "" {
				s.mu.RLock()
				handler, ok := s.streamingHandlers[requestID]
				s.mu.RUnlock()

				if ok {
					handler(&streamResult)
				}
			}

			return nil
		}
	}
}

// RemoveStreamingHandler removes a streaming handler
func (s *clientSession) RemoveStreamingHandler(requestID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.streamingHandlers, requestID)
}

// SendMessage sends a JSON-RPC message directly
func (s *clientSession) SendMessage(message interface{}) error {
	// Check if the session is closed
	if s.IsClosed() {
		return ErrSessionClosed
	}

	// Marshal the message to JSON
	_, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Check if the session is closed
	if s.IsClosed() {
		return ErrSessionClosed
	}

	// Send the message through the transport
	_, err = s.transport.SendMessage(context.Background(), message.(*spec.JSONRPCMessage))
	return err
}

// SendNotificationAsync sends a notification to the counterparty asynchronously
func (s *clientSession) SendNotificationAsync(ctx context.Context, method string, params interface{}) chan error {
	errCh := make(chan error, 1)

	// Check if the session is closed
	if s.IsClosed() {
		errCh <- ErrSessionClosed
		close(errCh)
		return errCh
	}

	go func() {
		defer close(errCh)

		// Create the JSON-RPC message
		var paramsJSON json.RawMessage
		if params != nil {
			data, err := json.Marshal(params)
			if err != nil {
				errCh <- fmt.Errorf("failed to marshal parameters: %w", err)
				return
			}
			paramsJSON = data
		}

		message := &spec.JSONRPCMessage{
			JSONRPC: spec.JSONRPCVersion,
			Method:  method,
			Params:  paramsJSON,
		}

		// Send the message through the transport
		_, err := s.transport.SendMessage(ctx, message)
		if err != nil {
			errCh <- fmt.Errorf("failed to send notification: %w", err)
			return
		}

		errCh <- nil
	}()

	return errCh
}

// SendNotification sends a notification to the server
func (s *clientSession) SendNotification(ctx context.Context, method string, params interface{}) error {
	// Check if the session is closed
	if s.IsClosed() {
		return ErrSessionClosed
	}

	// Create the JSON-RPC message
	var paramsJSON json.RawMessage
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("failed to marshal parameters: %w", err)
		}
		paramsJSON = data
	}

	message := &spec.JSONRPCMessage{
		JSONRPC: spec.JSONRPCVersion,
		Method:  method,
		Params:  paramsJSON,
	}

	// Send the message through the transport
	_, err := s.transport.SendMessage(ctx, message)
	if err != nil {
		return fmt.Errorf("failed to send notification: %w", err)
	}

	return nil
}

// Close closes the session and releases any associated resources
func (s *clientSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true

	// Reject all pending requests
	for _, handler := range s.handlers {
		handler.cancel()
		handler.errCh <- ErrSessionClosed
		close(handler.resultCh)
	}
	s.handlers = make(map[string]pendingRequest)

	return s.transport.Close()
}

// CloseGracefully closes the session gracefully
func (s *clientSession) CloseGracefully(ctx context.Context) error {
	// Check if there are active handlers
	s.mu.RLock()
	pendingCount := len(s.handlers)
	s.mu.RUnlock()

	if pendingCount == 0 {
		// No active handlers, close immediately
		return s.Close()
	}

	// Create a channel to signal when all handlers are done
	done := make(chan struct{})
	go func() {
		for {
			time.Sleep(100 * time.Millisecond)
			s.mu.RLock()
			pendingCount = len(s.handlers)
			s.mu.RUnlock()
			if pendingCount == 0 {
				close(done)
				return
			}
		}
	}()

	// Wait for all handlers to complete or context to be done
	select {
	case <-done:
		return s.Close()
	case <-ctx.Done():
		// Context timed out or was canceled
		return s.Close()
	}
}

// handleMessage processes incoming JSON-RPC messages from the transport
func (s *clientSession) handleMessage(message *spec.JSONRPCMessage) (*spec.JSONRPCMessage, error) {
	// Check for nil message
	if message == nil {
		return nil, errors.New("received nil message")
	}

	// If this is a notification, handle it
	if message.IsNotification() {
		s.mu.RLock()
		handler, exists := s.notificationHandlers[message.Method]
		s.mu.RUnlock()

		if exists && message.Params != nil {
			// Call the registered notification handler
			err := handler(context.Background(), message.Params)
			if err != nil {
				return nil, fmt.Errorf("notification handler error: %w", err)
			}
		}
		return nil, nil
	}

	// If this is a request, we'd need to handle it
	// For a client session, this is unusual but possible
	if message.IsRequest() {
		// For now, return method not found
		idCopy := *message.ID
		return &spec.JSONRPCMessage{
			JSONRPC: spec.JSONRPCVersion,
			ID:      &idCopy,
			Error: &spec.JSONRPCError{
				Code:    spec.ErrCodeMethodNotFound, // Ensure this constant is defined in the spec package
				Message: "method not found",
			},
		}, nil
	}

	// Handle response
	if message.IsResponse() && message.ID != nil {
		idStr := string(*message.ID)
		s.handleResponse(message, idStr)
		return nil, nil
	}

	return nil, fmt.Errorf("unhandled message type")
}

// handleResponse processes a JSON-RPC response
func (s *clientSession) handleResponse(message *spec.JSONRPCMessage, idStr string) {
	s.mu.Lock()
	handler, exists := s.handlers[idStr]
	if exists {
		delete(s.handlers, idStr)
	}
	s.mu.Unlock()

	if !exists {
		// Check if this is a streaming response with a requestId in the result
		if message.Result != nil {
			// Try to unmarshal as a CreateMessageResult to check for streaming
			var result map[string]interface{}
			if err := json.Unmarshal(message.Result, &result); err == nil {
				// Check if it has a metadata field with a requestId
				if metadata, ok := result["metadata"].(map[string]interface{}); ok {
					if requestID, ok := metadata["requestId"].(string); ok {
						// This is a streaming response, call the streaming handler
						s.mu.RLock()
						handler, exists := s.streamingHandlers[requestID]
						s.mu.RUnlock()

						if exists {
							var msgResult spec.CreateMessageResult
							if err := json.Unmarshal(message.Result, &msgResult); err == nil {
								handler(&msgResult)
							}
						}
						return
					}
				}
			}
		}
		return // No handler found
	}

	// Cancel the context for this request since we've received a response
	handler.cancel()

	// Check for error
	if message.Error != nil {
		mcpErr := &spec.McpError{
			Code:    message.Error.Code,
			Message: message.Error.Message,
			Data:    message.Error.Data,
		}
		handler.errCh <- mcpErr
		close(handler.resultCh)
		return
	}

	// Unmarshal the result
	if message.Result == nil {
		handler.errCh <- ErrResponseWithoutResultOrError
		close(handler.resultCh)
		return
	}

	// Create a new instance of the expected result type
	result := reflect.New(handler.resultType).Interface()

	// Unmarshal the result
	if err := json.Unmarshal(message.Result, result); err != nil {
		handler.errCh <- fmt.Errorf("failed to unmarshal result: %w", err)
		close(handler.resultCh)
		return
	}

	// Send the result to the channel
	handler.resultCh <- reflect.ValueOf(result).Elem().Interface()
	close(handler.errCh)
}

// NotifyToolsListChanged sends a notification that the tools list has changed
func (s *clientSession) NotifyToolsListChanged() error {
	ctx, cancel := context.WithTimeout(context.Background(), s.defaultTimeout)
	defer cancel()

	errCh := s.SendNotificationAsync(ctx, spec.MethodNotificationToolsListChanged, nil)

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// NotifyResourcesListChanged sends a notification that the resources list has changed
func (s *clientSession) NotifyResourcesListChanged() error {
	ctx, cancel := context.WithTimeout(context.Background(), s.defaultTimeout)
	defer cancel()

	errCh := s.SendNotificationAsync(ctx, spec.MethodNotificationResourcesListChanged, nil)

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// NotifyPromptsListChanged sends a notification that the prompts list has changed
func (s *clientSession) NotifyPromptsListChanged() error {
	ctx, cancel := context.WithTimeout(context.Background(), s.defaultTimeout)
	defer cancel()

	errCh := s.SendNotificationAsync(ctx, spec.MethodNotificationPromptsListChanged, nil)

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// NotifyResourceChanged sends a notification that a specific resource has changed
func (s *clientSession) NotifyResourceChanged(uri string, contents []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.defaultTimeout)
	defer cancel()

	params := map[string]interface{}{
		"uri":      uri,
		"contents": contents,
	}

	errCh := s.SendNotificationAsync(ctx, spec.MethodNotificationResourceChanged, params)

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// AddSubscription adds a subscription for a resource URI
func (s *clientSession) AddSubscription(uri string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subscriptions[uri] = true
	return nil
}

// RemoveSubscription removes a subscription for a resource URI
func (s *clientSession) RemoveSubscription(uri string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.subscriptions, uri)
	return nil
}

// GetSubscriptions returns the list of resource subscriptions for the session
func (s *clientSession) GetSubscriptions() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	subscriptions := make([]string, 0, len(s.subscriptions))
	for uri := range s.subscriptions {
		subscriptions = append(subscriptions, uri)
	}
	return subscriptions
}

// SendLogMessage sends a log message to the client
func (s *clientSession) SendLogMessage(level spec.LogLevel, message string, logger string) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.defaultTimeout)
	defer cancel()

	params := spec.LoggingMessage{
		Level:   level,
		Message: message,
		Logger:  logger,
	}

	errCh := s.SendNotificationAsync(ctx, spec.MethodNotificationMessage, params)

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// RegisterStreamHandler registers a handler for streaming tool results
func (s *clientSession) RegisterStreamHandler(streamID string, handler func(*spec.StreamingToolResult)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create a singleton for stream handlers if needed
	if s.streamHandlers == nil {
		s.streamHandlers = make(map[string]func(*spec.StreamingToolResult))
	}

	// Register the handler for this stream ID
	s.streamHandlers[streamID] = handler

	// Register the notification handler for tool stream results if not already done
	_, exists := s.notificationHandlers[spec.MethodToolsStreamResult]
	if !exists {
		s.notificationHandlers[spec.MethodToolsStreamResult] = func(ctx context.Context, params json.RawMessage) error {
			var streamResult spec.StreamingToolResult
			if err := json.Unmarshal(params, &streamResult); err != nil {
				return fmt.Errorf("failed to unmarshal streaming tool result: %w", err)
			}

			s.mu.RLock()
			handler, ok := s.streamHandlers[streamResult.StreamID]
			s.mu.RUnlock()

			if ok {
				handler(&streamResult)
			}

			return nil
		}
	}
}

// UnregisterStreamHandler removes a streaming tool handler
func (s *clientSession) UnregisterStreamHandler(streamID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.streamHandlers != nil {
		delete(s.streamHandlers, streamID)
	}
}
