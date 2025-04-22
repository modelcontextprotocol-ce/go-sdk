// Package stream provides implementations for streaming server transports
package stream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/modelcontextprotocol-ce/go-sdk/spec"
	"github.com/modelcontextprotocol-ce/go-sdk/util"
)

// httpClientSession represents a client connection in the HTTP server transport
type httpClientSession struct {
	id                   string
	lastActive           time.Time
	initialized          bool
	subscriptions        map[string]bool // Track subscribed resource URIs
	notificationHandlers map[string]spec.NotificationHandler
	streamingHandlers    map[string]func(*spec.CreateMessageResult)
	metadata             map[string]interface{}
	w                    http.ResponseWriter
	logger               util.Logger
	mu                   sync.RWMutex
}

// GetID returns the unique identifier for this client session
func (s *httpClientSession) GetID() string {
	return s.id
}

// IsInitialized returns whether the session has been initialized
func (s *httpClientSession) IsInitialized() bool {
	return s.initialized
}

// IsClosed returns true if the session is closed
func (s *httpClientSession) IsClosed() bool {
	return !s.initialized // A simple implementation - if not initialized, consider it closed
}

// Close closes the session and releases any resources
func (s *httpClientSession) Close() error {
	return nil
}

// CloseGracefully closes the session gracefully
func (s *httpClientSession) CloseGracefully(ctx context.Context) error {
	// Implement graceful closure logic if needed
	return nil
}

// GetClientInfo returns information about the client
func (s *httpClientSession) GetClientInfo() spec.ClientInfo {
	// Return a default client info using the session ID
	return spec.NewDefaultClientInfo(
		spec.Implementation{
			Name:    "http-client",
			Version: "1.0.0",
		},
		s.id,
	)
}

// SendRequest sends a request to the server and returns the response
func (s *httpClientSession) SendRequest(ctx context.Context, method string, params interface{}, result interface{}) error {
	// For the HTTP transport, this is not directly used as requests are handled through HTTP
	// This is a placeholder implementation to satisfy the interface
	return errors.New("SendRequest not supported in HTTP server transport")
}

// Send sends a response to the client for a specific event
func (s *httpClientSession) Send(ctx context.Context, event string, message interface{}) (body []byte, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Log the response attempt
	s.logger.Debug("Preparing to send response", "sessionID", s.id, "event", event)

	switch message := message.(type) {
	case *spec.JSONRPCMessage:
		body, err = json.Marshal(message)
		if err != nil {
			s.logger.Error("Failed to marshal JSON-RPC message", "error", err)
			return nil, fmt.Errorf("failed to marshal JSON-RPC message: %w", err)
		}
	case []byte:
		body = message
	case *[]byte:
		body = *message
	case string:
		body = []byte(message)
	case *string:
		body = []byte(*message)
	default:
		if b, err := json.Marshal(message); err != nil {
			s.logger.Error("Failed to marshal message", "error", err)
			return nil, fmt.Errorf("failed to marshal message: %w", err)
		} else {
			body = b
		}
	}

	// Write the event and data to the response
	_, writeErr := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, body)
	if writeErr != nil {
		s.logger.Error("Failed to write response", "error", writeErr, "event", event)
		return body, fmt.Errorf("failed to write response: %w", writeErr)
	}

	// Create a flusher for streaming
	flusher, ok := s.w.(http.Flusher)
	if ok {
		flusher.Flush()
		s.logger.Debug("Response sent and flushed", "sessionID", s.id, "event", event, "bodySize", len(body))
	} else {
		s.logger.Warn("Could not flush response - client might not support streaming", "sessionID", s.id)
	}

	return body, nil
}

// SendMessage sends a JSON-RPC message directly
func (s *httpClientSession) SendMessage(message interface{}) error {
	// For the HTTP transport, this is not directly used as messages are sent through HTTP responses
	// This is a placeholder implementation to satisfy the interface
	return errors.New("SendMessage not supported in HTTP server transport")
}

// SendLogMessage sends a log message to the server
func (s *httpClientSession) SendLogMessage(level spec.LogLevel, source string, message string) error {
	// This is a placeholder implementation for HTTP transport
	// In a real implementation, you might want to log this somewhere or send it to the client
	return nil
}

// SendNotification sends a notification to the server
func (s *httpClientSession) SendNotification(ctx context.Context, method string, params interface{}) error {
	// This is a placeholder implementation for HTTP transport
	// In a real implementation, you would send notifications to the client through some mechanism
	return nil
}

// SetNotificationHandler sets the handler for incoming notifications
func (s *httpClientSession) SetNotificationHandler(method string, handler spec.NotificationHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notificationHandlers[method] = handler
}

// RemoveNotificationHandler removes a notification handler
func (s *httpClientSession) RemoveNotificationHandler(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.notificationHandlers, id)
}

// SetStreamingHandler registers a handler for streaming message responses
func (s *httpClientSession) SetStreamingHandler(requestID string, handler func(*spec.CreateMessageResult)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.streamingHandlers[requestID] = handler
}

// RemoveStreamingHandler removes a streaming handler
func (s *httpClientSession) RemoveStreamingHandler(tool string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.streamingHandlers, tool)
}

// RegisterStreamHandler registers a handler for streaming tool results
func (s *httpClientSession) RegisterStreamHandler(tool string, handler func(*spec.StreamingToolResult)) {
	// Implementation for registering stream handler
	// This is a placeholder since HTTP sessions handle streams differently
}

// UnregisterStreamHandler removes a streaming tool handler
func (s *httpClientSession) UnregisterStreamHandler(streamID string) {
	// This is a placeholder implementation for unregistering stream handlers
	// HTTP sessions handle streaming differently
}

// NotifyPromptsListChanged sends a notification that the prompts list has changed
func (s *httpClientSession) NotifyPromptsListChanged() error {
	notification := &spec.JSONRPCMessage{
		JSONRPC: spec.JSONRPCVersion,
		Method:  spec.MethodNotificationPromptsListChanged,
		Params:  json.RawMessage("{}"),
	}

	_, err := s.Send(context.Background(), "notification", notification)
	if err != nil {
		return fmt.Errorf("failed to marshal prompts list changed notification: %w", err)
	}

	return err
}

// NotifyResourcesListChanged sends a notification that the resources list has changed
func (s *httpClientSession) NotifyResourcesListChanged() error {
	notification := &spec.JSONRPCMessage{
		JSONRPC: spec.JSONRPCVersion,
		Method:  spec.MethodNotificationResourcesListChanged,
		Params:  json.RawMessage("{}"),
	}

	_, err := s.Send(context.Background(), "notification", notification)
	if err != nil {
		return fmt.Errorf("failed to marshal resources list changed notification: %w", err)
	}

	return err
}

// NotifyResourceChanged sends a notification that a specific resource has changed
func (s *httpClientSession) NotifyResourceChanged(uri string, data []byte) error {
	// Check if this client is subscribed to this resource
	s.mu.RLock()
	subscribed := s.subscriptions[uri]
	s.mu.RUnlock()

	if !subscribed {
		// Client is not subscribed to this resource
		return nil
	}

	// Create resource change notification
	params := map[string]interface{}{
		"uri":      uri,
		"contents": data,
	}

	paramsBytes, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal resource change params: %w", err)
	}

	notification := &spec.JSONRPCMessage{
		JSONRPC: spec.JSONRPCVersion,
		Method:  spec.MethodNotificationResourceChanged,
		Params:  paramsBytes,
	}

	_, err = s.Send(context.Background(), "notification", notification)
	if err != nil {
		return fmt.Errorf("failed to marshal resource changed notification: %w", err)
	}

	return err
}

// NotifyToolsListChanged sends a notification that the tools list has changed
func (s *httpClientSession) NotifyToolsListChanged() error {
	notification := &spec.JSONRPCMessage{
		JSONRPC: spec.JSONRPCVersion,
		Method:  spec.MethodNotificationToolsListChanged,
		Params:  json.RawMessage("{}"),
	}

	_, err := s.Send(context.Background(), "notification", notification)
	if err != nil {
		return fmt.Errorf("failed to marshal tools list changed notification: %w", err)
	}

	return err
}

// AddSubscription adds a subscription for a resource URI
func (s *httpClientSession) AddSubscription(uri string) error {
	if uri == "" {
		return fmt.Errorf("resource URI cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.subscriptions[uri] = true
	return nil
}

// RemoveSubscription removes a subscription for a resource URI
func (s *httpClientSession) RemoveSubscription(uri string) error {
	if uri == "" {
		return fmt.Errorf("resource URI cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.subscriptions, uri)
	return nil
}

// GetSubscriptions returns the list of resource subscriptions for the session
func (s *httpClientSession) GetSubscriptions() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	subscriptions := make([]string, 0, len(s.subscriptions))
	for uri := range s.subscriptions {
		subscriptions = append(subscriptions, uri)
	}
	return subscriptions
}

// createClientSession creates a new client session with the given ID
func createClientSession(sessionID string, w http.ResponseWriter, logger util.Logger) *httpClientSession {
	// If no logger is provided, create a default one
	if logger == nil {
		logger = util.DefaultRootLogger().WithComponent("ClientSession:" + sessionID)
	} else {
		logger = logger.WithComponent("ClientSession:" + sessionID)
	}

	session := &httpClientSession{
		id:                   sessionID,
		lastActive:           time.Now(),
		subscriptions:        make(map[string]bool),
		notificationHandlers: make(map[string]spec.NotificationHandler),
		streamingHandlers:    make(map[string]func(*spec.CreateMessageResult)),
		w:                    w,
		metadata:             make(map[string]interface{}),
		logger:               logger,
	}

	// Log session creation
	session.logger.Info("Created new client session", "id", sessionID)

	return session
}
