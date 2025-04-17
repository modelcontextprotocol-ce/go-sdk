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
	if ctx == nil {
		ctx = context.Background()
	}

	// Set timeout if not already set
	_, hasTimeout := ctx.Deadline()
	if !hasTimeout {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.defaultTimeout)
		defer cancel()
	}

	// Create channels for async handling
	resultCh, errCh := s.SendRequestAsync(ctx, method, params, reflect.TypeOf(resultPtr).Elem())

	// Wait for result or error
	select {
	case result := <-resultCh:
		// Copy the result to the provided pointer
		resultValue := reflect.ValueOf(resultPtr).Elem()
		resultValue.Set(reflect.ValueOf(result))
		return nil
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return fmt.Errorf("%w: %s", ErrTimeout, ctx.Err())
	}
}

// SendMessage sends a JSON-RPC message and returns the response
func (s *clientSession) SendMessage(message interface{}) error {
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

// SendRequestAsync sends a request to the counterparty and returns channels for the response
func (s *clientSession) SendRequestAsync(ctx context.Context, method string, params interface{}, resultType reflect.Type) (chan interface{}, chan error) {
	resultCh := make(chan interface{}, 1)
	errCh := make(chan error, 1)

	if ctx == nil {
		ctx = context.Background()
	}

	// Check if the session is closed
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		errCh <- ErrSessionClosed
		close(resultCh)
		return resultCh, errCh
	}
	s.mu.RUnlock()

	// Generate a new ID for the request
	id := atomic.AddInt64(&s.nextID, 1)
	idStr := fmt.Sprintf("%d", id)
	idJSON := json.RawMessage([]byte(`"` + idStr + `"`))

	// Create a context for this request that can be cancelled
	reqCtx, cancel := context.WithCancel(ctx)

	// Register the pending request
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

	// If the transport is synchronous, we may already have a response
	if response != nil {
		s.handleMessage(response)
	}

	return resultCh, errCh
}

// SetNotificationHandler sets the handler for incoming notifications
func (s *clientSession) SetNotificationHandler(method string, handler spec.NotificationHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notificationHandlers[method] = handler
}

// RemoveNotificationHandler removes a notification handler
func (s *clientSession) RemoveNotificationHandler(method string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.notificationHandlers, method)
}

// NotifyToolsListChanged notifies the client that the tools list has changed
func (s *clientSession) NotifyToolsListChanged() error {
	return s.SendNotification(spec.MethodNotificationToolsListChanged, nil)
}

// NotifyResourcesListChanged notifies the client that the resources list has changed
func (s *clientSession) NotifyResourcesListChanged() error {
	return s.SendNotification(spec.MethodNotificationResourcesListChanged, nil)
}

// NotifyPromptsListChanged notifies the client that the prompts list has changed
func (s *clientSession) NotifyPromptsListChanged() error {
	return s.SendNotification(spec.MethodNotificationPromptsListChanged, nil)
}

// NotifyResourceChanged notifies the client that a resource has changed
func (s *clientSession) NotifyResourceChanged(uri string, contents []byte) error {
	s.mu.RLock()
	subscribed := s.subscriptions[uri]
	s.mu.RUnlock()

	if subscribed {
		params := map[string]interface{}{
			"uri":      uri,
			"contents": contents,
		}
		return s.SendNotification(spec.MethodNotificationResourceChanged, params)
	}
	return nil
}

// SendLogMessage sends a log message to the client
func (s *clientSession) SendLogMessage(level spec.LogLevel, message string, logger string) error {
	params := spec.LoggingMessage{
		Level:   level,
		Message: message,
		Logger:  logger,
	}
	return s.SendNotification(spec.MethodNotificationMessage, params)
}

// AddSubscription adds a subscription to the specified resource URI
func (s *clientSession) AddSubscription(uri string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subscriptions[uri] = true
	return nil
}

// RemoveSubscription removes a subscription from the specified resource URI
func (s *clientSession) RemoveSubscription(uri string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.subscriptions, uri)
	return nil
}

// SendNotification sends a notification to the counterparty
func (s *clientSession) SendNotification(method string, params interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.defaultTimeout)
	defer cancel()

	errCh := s.SendNotificationAsync(ctx, method, params)
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return fmt.Errorf("%w: %s", ErrTimeout, ctx.Err())
	}
}

// SendNotificationAsync sends a notification to the counterparty asynchronously
func (s *clientSession) SendNotificationAsync(ctx context.Context, method string, params interface{}) chan error {
	errCh := make(chan error, 1)

	if ctx == nil {
		ctx = context.Background()
	}

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		errCh <- ErrSessionClosed
		close(errCh)
		return errCh
	}
	s.mu.RUnlock()

	// Create the JSON-RPC message
	var paramsJSON json.RawMessage
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			errCh <- fmt.Errorf("failed to marshal parameters: %w", err)
			close(errCh)
			return errCh
		}
		paramsJSON = data
	}

	message := &spec.JSONRPCMessage{
		JSONRPC: spec.JSONRPCVersion,
		Method:  method,
		Params:  paramsJSON,
	}

	// Send the message through the transport
	go func() {
		defer close(errCh)
		_, err := s.transport.SendMessage(ctx, message)
		if err != nil {
			errCh <- fmt.Errorf("failed to send notification: %w", err)
		} else {
			errCh <- nil
		}
	}()

	return errCh
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
				s.mu.RLock()
				pendingCount := len(s.handlers)
				s.mu.RUnlock()

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
				Code:    spec.ErrCodeMethodNotFound,
				Message: "Method not found",
			},
		}, nil
	}

	// This must be a response, find the handler
	if message.ID == nil {
		return nil, ErrResponseWithoutID
	}

	// Extract the ID
	var idStr string
	if err := json.Unmarshal(*message.ID, &idStr); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response ID: %w", err)
	}

	// Look up the handler
	s.mu.Lock()
	handler, exists := s.handlers[idStr]
	if exists {
		delete(s.handlers, idStr)
	}
	s.mu.Unlock()

	if !exists {
		return nil, fmt.Errorf("no handler for response ID: %s", idStr)
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
		return nil, nil
	}

	// Unmarshal the result
	if message.Result == nil {
		handler.errCh <- ErrResponseWithoutResultOrError
		close(handler.resultCh)
		return nil, nil
	}

	// Create a new instance of the expected result type
	result := reflect.New(handler.resultType).Interface()

	// Unmarshal the result
	if err := json.Unmarshal(message.Result, result); err != nil {
		handler.errCh <- fmt.Errorf("failed to unmarshal result: %w", err)
		close(handler.resultCh)
		return nil, nil
	}

	// Send the result to the channel
	handler.resultCh <- reflect.ValueOf(result).Elem().Interface()
	close(handler.errCh)

	return nil, nil
}
