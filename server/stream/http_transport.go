// Package stream provides implementations for streaming server transports
package stream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol-ce/go-sdk/spec"
	"github.com/modelcontextprotocol-ce/go-sdk/util"
)

// HTTPServerTransport implements a server-side transport over HTTP with SSE streaming support
type HTTPServerTransport struct {
	// Core HTTP server
	server *http.Server
	router *http.ServeMux
	addr   string

	// Streaming state management
	activeStreams map[string]chan *spec.CreateMessageResult
	streamsMu     sync.RWMutex

	// Handler functions
	initializeHandler           func(ctx context.Context, session spec.McpClientSession, request *spec.InitializeRequest) (*spec.InitializeResult, error)
	pingHandler                 func(ctx context.Context, session spec.McpClientSession) error
	toolsListHandler            func(ctx context.Context, session spec.McpClientSession) (*spec.ListToolsResult, error)
	toolsCallHandler            func(ctx context.Context, session spec.McpClientSession, request *spec.CallToolRequest) (*spec.CallToolResult, error)
	resourcesListHandler        func(ctx context.Context, session spec.McpClientSession) (*spec.ListResourcesResult, error)
	resourcesReadHandler        func(ctx context.Context, session spec.McpClientSession, request *spec.ReadResourceRequest) (*spec.ReadResourceResult, error)
	resourcesSubscribeHandler   func(ctx context.Context, session spec.McpClientSession, request *spec.SubscribeRequest) error
	resourcesUnsubscribeHandler func(ctx context.Context, session spec.McpClientSession, request *spec.UnsubscribeRequest) error
	promptsListHandler          func(ctx context.Context, session spec.McpClientSession) (*spec.ListPromptsResult, error)
	promptGetHandler            func(ctx context.Context, session spec.McpClientSession, request *spec.GetPromptRequest) (*spec.GetPromptResult, error)
	loggingSetLevelHandler      func(ctx context.Context, session spec.McpClientSession, request *spec.SetLevelRequest) error
	createMessageHandler        func(ctx context.Context, session spec.McpClientSession, request *spec.CreateMessageRequest) (*spec.CreateMessageResponse, error)

	// Security
	apiToken    string
	requireAuth bool

	// Logger
	logger util.Logger

	// State
	debug     bool
	isRunning bool
	mu        sync.RWMutex
	sessions  map[string]*httpClientSession
}

// NewHTTPServerTransport creates a new HTTP server transport
func NewHTTPServerTransport(addr string, options ...HTTPServerOption) *HTTPServerTransport {
	router := http.NewServeMux()

	// Create a default logger if none is provided later through options
	defaultLogger := util.DefaultRootLogger().WithComponent("HTTPServerTransport")

	t := &HTTPServerTransport{
		addr:          addr,
		router:        router,
		server:        &http.Server{Addr: addr, Handler: router},
		activeStreams: make(map[string]chan *spec.CreateMessageResult),
		sessions:      make(map[string]*httpClientSession),
		debug:         false,
		logger:        defaultLogger,
	}

	// Apply options
	for _, opt := range options {
		opt(t)
	}

	// Set up routes
	t.setupRoutes()

	t.logger.Info("Created HTTP server transport", "addr", addr)

	return t
}

// HTTPServerOption allows for customizing the HTTP server transport
type HTTPServerOption func(*HTTPServerTransport)

// WithServerDebug enables debug logging for the server
func WithServerDebug(debug bool) HTTPServerOption {
	return func(t *HTTPServerTransport) {
		t.debug = debug
		if t.logger != nil {
			t.logger.Debug("Debug mode enabled for HTTP server transport")
		}
	}
}

// WithCustomServer sets a custom HTTP server
func WithCustomServer(server *http.Server) HTTPServerOption {
	return func(t *HTTPServerTransport) {
		t.server = server
		t.server.Handler = t.router
	}
}

// WithServerLogger sets a custom logger for the server
func WithServerLogger(logger util.Logger) HTTPServerOption {
	return func(t *HTTPServerTransport) {
		if logger != nil {
			t.logger = logger.WithComponent("HTTPServerTransport")
		}
	}
}

// WithAPIToken sets the API token for authentication
func WithAPIToken(token string) HTTPServerOption {
	return func(t *HTTPServerTransport) {
		t.apiToken = token
		// If a non-empty token is provided, enable authentication
		if token != "" {
			t.requireAuth = true
			t.logger.Info("API token authentication enabled")
		}
	}
}

// WithAuthRequired sets whether authentication is required
func WithAuthRequired(required bool) HTTPServerOption {
	return func(t *HTTPServerTransport) {
		t.requireAuth = required
		if required && t.apiToken == "" {
			t.logger.Warn("Authentication required but no API token set")
		}
	}
}

// validateAPIToken checks if the request contains a valid API token
func (t *HTTPServerTransport) validateAPIToken(r *http.Request) bool {
	// If auth is not required, always pass
	if !t.requireAuth {
		return true
	}

	if t.apiToken == "" {
		t.logger.Warn("Authentication required but no API token configured")
		return false
	}

	// Check Authorization header (Bearer token)
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		return t.apiToken == token
	}

	// Check X-API-Token header
	apiTokenHeader := r.Header.Get("X-API-Token")
	if apiTokenHeader != "" {
		return t.apiToken == apiTokenHeader
	}

	// Check token query parameter
	if token := r.URL.Query().Get("token"); token != "" {
		return t.apiToken == token
	}

	// No token found in request
	return false
}

// Send sends a message over the transport (implementation of McpTransport interface)
func (t *HTTPServerTransport) Send(message []byte) error {
	if t.debug {
		log.Printf("DEBUG: Raw send not applicable for HTTP server transport")
	}
	return nil
}

// Close closes the transport immediately
func (t *HTTPServerTransport) Close() error {
	return t.Stop()
}

// Listen starts listening for connections (implementation of McpServerTransport interface)
func (t *HTTPServerTransport) Listen(ctx context.Context, handler spec.MessageHandler) error {
	if t.debug {
		log.Printf("DEBUG: HTTP server transport Listen method called, but not applicable for this implementation")
	}
	return nil
}

// SendMessage sends a message to a client (implementation of McpServerTransport interface)
func (t *HTTPServerTransport) SendMessage(ctx context.Context, message *spec.JSONRPCMessage) (*spec.JSONRPCMessage, error) {
	if t.debug {
		log.Printf("DEBUG: HTTP server transport SendMessage called, but not directly applicable")
	}
	return message, nil
}

// ReceiveMessage receives a message from a client (implementation of McpServerTransport interface)
func (t *HTTPServerTransport) ReceiveMessage() (interface{}, spec.McpClientSession, error) {
	if t.debug {
		log.Printf("DEBUG: HTTP server transport ReceiveMessage called, but not applicable")
	}
	return nil, nil, errors.New("not applicable for HTTP server transport")
}

// Start starts the HTTP server
func (t *HTTPServerTransport) Start() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.isRunning {
		t.logger.Warn("Server is already running", "addr", t.addr)
		return errors.New("server is already running")
	}

	t.isRunning = true

	t.logger.Info("Starting HTTP server", "addr", t.addr)

	// Start the server in a goroutine
	go func() {
		if err := t.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			t.logger.Error("HTTP server error", "error", err)
		}
	}()

	return nil
}

// Stop stops the server immediately
func (t *HTTPServerTransport) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.isRunning {
		t.logger.Debug("Server is not running, nothing to stop", "addr", t.addr)
		return nil
	}

	t.logger.Info("Stopping HTTP server", "addr", t.addr)

	// Close all active streams
	t.streamsMu.Lock()
	streamCount := len(t.activeStreams)
	for id, stream := range t.activeStreams {
		close(stream)
		delete(t.activeStreams, id)
	}
	t.streamsMu.Unlock()
	t.logger.Debug("Closed active streams", "count", streamCount)

	// Shutdown the server
	if err := t.server.Close(); err != nil {
		t.logger.Error("Server close error", "error", err)
		return fmt.Errorf("server close error: %w", err)
	}

	t.isRunning = false
	t.logger.Info("HTTP server stopped", "addr", t.addr)
	return nil
}

// StopGracefully stops the server gracefully
func (t *HTTPServerTransport) StopGracefully(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.isRunning {
		t.logger.Debug("Server is not running, nothing to stop gracefully", "addr", t.addr)
		return nil
	}

	t.logger.Info("Gracefully stopping HTTP server", "addr", t.addr)

	// Close all active streams
	t.streamsMu.Lock()
	streamCount := len(t.activeStreams)
	for id, stream := range t.activeStreams {
		close(stream)
		delete(t.activeStreams, id)
	}
	t.streamsMu.Unlock()
	t.logger.Debug("Closed active streams", "count", streamCount)

	// Shutdown the server gracefully
	if err := t.server.Shutdown(ctx); err != nil {
		t.logger.Error("Server shutdown error", "error", err)
		return fmt.Errorf("server shutdown error: %w", err)
	}

	t.isRunning = false
	t.logger.Info("HTTP server gracefully stopped", "addr", t.addr)
	return nil
}

// setupRoutes configures HTTP routes
func (t *HTTPServerTransport) setupRoutes() {
	// Main JSON-RPC endpoint for POST requests
	t.router.HandleFunc("/jsonrpc", t.handleJSONRPC)

	// SSE streaming endpoint for specific request IDs
	t.router.HandleFunc("/stream/", t.handleStream)

	// Default SSE endpoint with ping for clients to connect to
	t.router.HandleFunc("/sse", t.handleDefaultSSE)
}

// handleJSONRPC processes incoming JSON-RPC requests
func (t *HTTPServerTransport) handleJSONRPC(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		t.logger.Warn("Method not allowed for JSON-RPC endpoint", "method", r.Method, "path", r.URL.Path)
		return
	}

	// Validate API token if authentication is required
	if t.requireAuth && !t.validateAPIToken(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		t.logger.Warn("Unauthorized access attempt to JSON-RPC endpoint",
			"path", r.URL.Path,
			"remoteAddr", r.RemoteAddr)
		return
	}

	// Set headers
	w.Header().Set("Content-Type", "application/json")

	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.logger.Error("Failed to read request body", "error", err)
		writeJSONError(w, &spec.JSONRPCError{
			Code:    -32700,
			Message: "Parse error",
			Data:    json.RawMessage([]byte(fmt.Sprintf("Could not read request body: %v", err))),
		})
		return
	}

	// Parse the JSON-RPC message
	var message spec.JSONRPCMessage
	if err := json.Unmarshal(body, &message); err != nil {
		t.logger.Error("Failed to unmarshal JSON-RPC message", "error", err)
		writeJSONError(w, &spec.JSONRPCError{
			Code:    -32700,
			Message: "Parse error",
			Data:    json.RawMessage([]byte(fmt.Sprintf("Invalid JSON: %v", err))),
		})
		return
	}

	t.logger.Debug("Received JSON-RPC request", "method", message.Method, "id", message.ID)

	// Extract or create session ID from headers or cookies
	sessionID := t.getOrCreateSessionID(r)

	// Get or create session
	session := t.getOrCreateSession(sessionID, w)

	// Process the request based on method
	response, err := t.processJSONRPCRequest(r.Context(), &message, session)
	if err != nil {
		t.logger.Error("Error processing JSON-RPC request", "method", message.Method, "error", err, "sessionID", sessionID)

		// Create error response
		errorResponse := &spec.JSONRPCMessage{
			JSONRPC: spec.JSONRPCVersion,
			ID:      message.ID,
			Error: &spec.JSONRPCError{
				Code:    -32000,
				Message: "Server error",
				Data:    json.RawMessage(err.Error()),
			},
		}

		// Marshal the error response
		responseBytes, err := session.Respond(r.Context(), "message", errorResponse)
		if err != nil {
			t.logger.Error("Failed to send error response through session", "error", err)
		}

		// Also write to the response for HTTP clients that don't support SSE
		w.Write(responseBytes)
		return
	}

	// Marshal the response
	responseBytes, err := json.Marshal(response)
	if err != nil {
		t.logger.Error("Failed to marshal JSON-RPC response", "error", err)
		// Handle marshaling error
		errorResponse := &spec.JSONRPCMessage{
			JSONRPC: spec.JSONRPCVersion,
			ID:      message.ID,
			Error: &spec.JSONRPCError{
				Code:    -32603,
				Message: "Internal error",
				Data:    json.RawMessage([]byte(fmt.Sprintf("Failed to marshal response: %v", err))),
			},
		}

		// Marshal the error response
		errorBytes, _ := json.Marshal(errorResponse)

		// Send through the session's Respond method
		if _, respErr := session.Respond(r.Context(), "message", errorResponse); respErr != nil {
			t.logger.Error("Failed to send error response through session", "error", respErr)
		}

		// Also write to the response for HTTP clients that don't support SSE
		w.Write(errorBytes)
		return
	}

	t.logger.Debug("Sending JSON-RPC response", "method", message.Method, "id", message.ID, "sessionID", sessionID)

	// Send through the session's Respond method
	if _, respErr := session.Respond(r.Context(), "message", response); respErr != nil {
		t.logger.Error("Failed to send response through session", "error", respErr)
	}

	// Also write to the response for HTTP clients that don't support SSE
	w.Write(responseBytes)
}

// handleStream processes SSE streaming connections
func (t *HTTPServerTransport) handleStream(w http.ResponseWriter, r *http.Request) {
	// Extract the stream ID from the URL
	// URL format: /stream/{requestId}
	requestID := r.URL.Path[len("/stream/"):]

	// Validate request ID
	if requestID == "" {
		http.Error(w, "Invalid stream ID", http.StatusBadRequest)
		t.logger.Warn("Invalid stream ID in request", "path", r.URL.Path)
		return
	}

	// Validate API token if authentication is required
	if t.requireAuth && !t.validateAPIToken(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		t.logger.Warn("Unauthorized access attempt to stream endpoint",
			"requestID", requestID,
			"remoteAddr", r.RemoteAddr)
		return
	}

	t.logger.Debug("Stream connection requested", "requestID", requestID)

	// Check if the stream exists
	t.streamsMu.RLock()
	stream, exists := t.activeStreams[requestID]
	t.streamsMu.RUnlock()

	if !exists {
		http.Error(w, "Stream not found", http.StatusNotFound)
		t.logger.Warn("Stream not found", "requestID", requestID)
		return
	}

	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create a notification channel for connection closure
	notify := r.Context().Done()

	// Create a flusher for streaming
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		t.logger.Error("Streaming not supported by client", "requestID", requestID)
		return
	}

	t.logger.Info("Started streaming connection", "requestID", requestID)

	// Stream results
	for {
		select {
		case <-notify:
			// Client disconnected
			t.logger.Info("Client disconnected from stream", "requestID", requestID)
			return

		case result, ok := <-stream:
			if !ok {
				// Channel closed, end streaming
				t.logger.Info("Stream channel closed", "requestID", requestID)
				return
			}

			// Marshal the result to JSON
			data, err := json.Marshal(result)
			if err != nil {
				t.logger.Error("Failed to marshal streaming result", "requestID", requestID, "error", err)
				continue
			}

			// Send the SSE message
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()

			// If this is the final message, end streaming
			if result.StopReason != "" {
				t.logger.Info("Ending stream with stop reason", "requestID", requestID, "stopReason", result.StopReason)
				return
			}
		}
	}
}

// handleDefaultSSE provides a default SSE endpoint with regular ping messages
func (t *HTTPServerTransport) handleDefaultSSE(w http.ResponseWriter, r *http.Request) {
	// Only accept GET requests
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		t.logger.Warn("Method not allowed for default SSE endpoint", "method", r.Method, "path", r.URL.Path)
		return
	}

	// Validate API token if authentication is required
	if t.requireAuth && !t.validateAPIToken(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		t.logger.Warn("Unauthorized access attempt to SSE endpoint",
			"path", r.URL.Path,
			"remoteAddr", r.RemoteAddr)
		return
	}

	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create a notification channel for connection closure
	notify := r.Context().Done()

	// Extract or create a session ID
	sessionID := t.getOrCreateSessionID(r)

	// Get or create session
	session := t.getOrCreateSession(sessionID, w)

	// Update session metadata
	session.mu.Lock()
	session.metadata["connected_to_sse"] = true
	session.lastActive = time.Now()
	session.mu.Unlock()

	t.logger.Info("Client connected to default SSE endpoint", "sessionID", sessionID)

	// Create ping ticker
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	// Construct endpoint URL with session ID and token (if using authentication)
	endpoint := fmt.Sprintf("/jsonrpc?sid=%s", sessionID)
	if t.requireAuth && t.apiToken != "" {
		endpoint = fmt.Sprintf("%s&token=%s", endpoint, t.apiToken)
	}

	// Send initial endpoint message - this is what VS Code MCP clients expect
	if _, err := session.Respond(r.Context(), "endpoint", endpoint); err != nil {
		t.logger.Error("Failed to send initial endpoint message", "sessionID", sessionID, "error", err)
	}

	// Stream ping messages until the client disconnects
	for {
		select {
		case <-notify:
			// Client disconnected
			t.logger.Info("Client disconnected from default SSE endpoint", "sessionID", sessionID)
			return

		case <-ticker.C:
			// Send an endpoint message with server info
			if _, err := session.Respond(context.Background(), "endpoint", endpoint); err != nil {
				t.logger.Warn("Failed to send ping message", "sessionID", sessionID, "error", err)
			} else {
				t.logger.Debug("Sent ping message", "sessionID", sessionID)
			}

			// Update session last active time
			session.lastActive = time.Now()
		}
	}
}

// processJSONRPCRequest processes a JSON-RPC request and returns a response
func (t *HTTPServerTransport) processJSONRPCRequest(ctx context.Context, message *spec.JSONRPCMessage, session *httpClientSession) (*spec.JSONRPCMessage, error) {
	// Update session last active time
	session.lastActive = time.Now()

	var response *spec.JSONRPCMessage
	var err error

	// Process by method
	switch message.Method {
	case spec.MethodInitialize:
		response, err = t.handleInitialize(ctx, message, session)

	case spec.MethodNotificationInitialized:
		// Handle the initialized notification (no response needed for notifications)
		t.logger.Info("Received initialized notification from client", "sessionID", session.id)
		session.mu.Lock()
		session.metadata["client_initialized"] = true
		session.mu.Unlock()
		response = &spec.JSONRPCMessage{
			JSONRPC: message.JSONRPC,
			ID:      message.ID,
			Method:  message.Method,
			Result:  json.RawMessage(`{"status": "ok"}`),
		}

	case spec.MethodPing:
		response, err = t.handlePing(ctx, message, session)

	case spec.MethodToolsList:
		response, err = t.handleToolsList(ctx, message, session)

	case spec.MethodToolsCall:
		response, err = t.handleToolsCall(ctx, message, session)

	case spec.MethodResourcesList:
		response, err = t.handleResourcesList(ctx, message, session)

	case spec.MethodResourcesRead:
		response, err = t.handleResourcesRead(ctx, message, session)

	case spec.MethodResourcesSubscribe:
		response, err = t.handleResourcesSubscribe(ctx, message, session)

	case spec.MethodResourcesUnsubscribe:
		response, err = t.handleResourcesUnsubscribe(ctx, message, session)

	case spec.MethodPromptList:
		response, err = t.handlePromptsList(ctx, message, session)

	case spec.MethodPromptGet:
		response, err = t.handlePromptGet(ctx, message, session)

	case spec.MethodLoggingSetLevel:
		response, err = t.handleLoggingSetLevel(ctx, message, session)

	case spec.MethodSamplingCreateMessage:
		response, err = t.handleCreateMessage(ctx, message, session)

	case spec.MethodContentCreateMessageStream:
		response, err = t.handleCreateMessageStream(ctx, message, session)

	default:
		return nil, fmt.Errorf("unsupported method: %s", message.Method)
	}

	if err != nil {
		return nil, err
	}

	// Each response should also be sent through the session's Respond method
	// to ensure it follows the MCP specification for client communication
	if response != nil {
		_, err := session.Respond(ctx, "message", response)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal JSON-RPC response: %w", err)
		}
	}

	return response, nil
}

// Implementation of handler methods

// handleInitialize processes initialize requests
func (t *HTTPServerTransport) handleInitialize(ctx context.Context, message *spec.JSONRPCMessage, session *httpClientSession) (*spec.JSONRPCMessage, error) {
	if t.initializeHandler == nil {
		return createEmptyResponse(message), nil
	}

	var request spec.InitializeRequest
	if err := json.Unmarshal(message.Params, &request); err != nil {
		return nil, fmt.Errorf("failed to unmarshal initialize params: %w", err)
	}

	result, err := t.initializeHandler(ctx, session, &request)
	if err != nil {
		return nil, fmt.Errorf("initialize handler error: %w", err)
	}

	// Mark session as initialized
	session.initialized = true

	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal initialize result: %w", err)
	}

	return &spec.JSONRPCMessage{
		JSONRPC: spec.JSONRPCVersion,
		ID:      message.ID,
		Result:  resultBytes,
	}, nil
}

// handlePing processes ping requests
func (t *HTTPServerTransport) handlePing(ctx context.Context, message *spec.JSONRPCMessage, session *httpClientSession) (*spec.JSONRPCMessage, error) {
	if t.pingHandler == nil {
		return createEmptyResponse(message), nil
	}

	if err := t.pingHandler(ctx, session); err != nil {
		return nil, fmt.Errorf("ping handler error: %w", err)
	}

	return createEmptyResponse(message), nil
}

// handleToolsList processes tools list requests
func (t *HTTPServerTransport) handleToolsList(ctx context.Context, message *spec.JSONRPCMessage, session *httpClientSession) (*spec.JSONRPCMessage, error) {
	if t.toolsListHandler == nil {
		return createEmptyResponse(message), nil
	}

	result, err := t.toolsListHandler(ctx, session)
	if err != nil {
		return nil, fmt.Errorf("tools list handler error: %w", err)
	}

	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tools list result: %w", err)
	}

	return &spec.JSONRPCMessage{
		JSONRPC: spec.JSONRPCVersion,
		ID:      message.ID,
		Result:  resultBytes,
	}, nil
}

// handleToolsCall processes tool call requests
func (t *HTTPServerTransport) handleToolsCall(ctx context.Context, message *spec.JSONRPCMessage, session *httpClientSession) (*spec.JSONRPCMessage, error) {
	if t.toolsCallHandler == nil {
		return createEmptyResponse(message), nil
	}

	var request spec.CallToolRequest
	if err := json.Unmarshal(message.Params, &request); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tool call params: %w", err)
	}

	result, err := t.toolsCallHandler(ctx, session, &request)
	if err != nil {
		return nil, fmt.Errorf("tool call handler error: %w", err)
	}

	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tool call result: %w", err)
	}

	return &spec.JSONRPCMessage{
		JSONRPC: spec.JSONRPCVersion,
		ID:      message.ID,
		Result:  resultBytes,
	}, nil
}

// handleResourcesList processes resources list requests
func (t *HTTPServerTransport) handleResourcesList(ctx context.Context, message *spec.JSONRPCMessage, session *httpClientSession) (*spec.JSONRPCMessage, error) {
	if t.resourcesListHandler == nil {
		return createEmptyResponse(message), nil
	}

	result, err := t.resourcesListHandler(ctx, session)
	if err != nil {
		return nil, fmt.Errorf("resources list handler error: %w", err)
	}

	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal resources list result: %w", err)
	}

	return &spec.JSONRPCMessage{
		JSONRPC: spec.JSONRPCVersion,
		ID:      message.ID,
		Result:  resultBytes,
	}, nil
}

// handleResourcesRead processes resource read requests
func (t *HTTPServerTransport) handleResourcesRead(ctx context.Context, message *spec.JSONRPCMessage, session *httpClientSession) (*spec.JSONRPCMessage, error) {
	if t.resourcesReadHandler == nil {
		return createEmptyResponse(message), nil
	}

	var request spec.ReadResourceRequest
	if err := json.Unmarshal(message.Params, &request); err != nil {
		return nil, fmt.Errorf("failed to unmarshal resource read params: %w", err)
	}

	result, err := t.resourcesReadHandler(ctx, session, &request)
	if err != nil {
		return nil, fmt.Errorf("resource read handler error: %w", err)
	}

	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal resource read result: %w", err)
	}

	return &spec.JSONRPCMessage{
		JSONRPC: spec.JSONRPCVersion,
		ID:      message.ID,
		Result:  resultBytes,
	}, nil
}

// handleResourcesSubscribe processes resource subscribe requests
func (t *HTTPServerTransport) handleResourcesSubscribe(ctx context.Context, message *spec.JSONRPCMessage, session *httpClientSession) (*spec.JSONRPCMessage, error) {
	if t.resourcesSubscribeHandler == nil {
		return createEmptyResponse(message), nil
	}

	var request spec.SubscribeRequest
	if err := json.Unmarshal(message.Params, &request); err != nil {
		return nil, fmt.Errorf("failed to unmarshal resource subscribe params: %w", err)
	}

	if err := t.resourcesSubscribeHandler(ctx, session, &request); err != nil {
		return nil, fmt.Errorf("resource subscribe handler error: %w", err)
	}

	return createEmptyResponse(message), nil
}

// handleResourcesUnsubscribe processes resource unsubscribe requests
func (t *HTTPServerTransport) handleResourcesUnsubscribe(ctx context.Context, message *spec.JSONRPCMessage, session *httpClientSession) (*spec.JSONRPCMessage, error) {
	if t.resourcesUnsubscribeHandler == nil {
		return createEmptyResponse(message), nil
	}

	var request spec.UnsubscribeRequest
	if err := json.Unmarshal(message.Params, &request); err != nil {
		return nil, fmt.Errorf("failed to unmarshal resource unsubscribe params: %w", err)
	}

	if err := t.resourcesUnsubscribeHandler(ctx, session, &request); err != nil {
		return nil, fmt.Errorf("resource unsubscribe handler error: %w", err)
	}

	return createEmptyResponse(message), nil
}

// handlePromptsList processes prompts list requests
func (t *HTTPServerTransport) handlePromptsList(ctx context.Context, message *spec.JSONRPCMessage, session *httpClientSession) (*spec.JSONRPCMessage, error) {
	if t.promptsListHandler == nil {
		return createEmptyResponse(message), nil
	}

	result, err := t.promptsListHandler(ctx, session)
	if err != nil {
		return nil, fmt.Errorf("prompts list handler error: %w", err)
	}

	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal prompts list result: %w", err)
	}

	return &spec.JSONRPCMessage{
		JSONRPC: spec.JSONRPCVersion,
		ID:      message.ID,
		Result:  resultBytes,
	}, nil
}

// handlePromptGet processes prompt get requests
func (t *HTTPServerTransport) handlePromptGet(ctx context.Context, message *spec.JSONRPCMessage, session *httpClientSession) (*spec.JSONRPCMessage, error) {
	if t.promptGetHandler == nil {
		return createEmptyResponse(message), nil
	}

	var request spec.GetPromptRequest
	if err := json.Unmarshal(message.Params, &request); err != nil {
		return nil, fmt.Errorf("failed to unmarshal prompt get params: %w", err)
	}

	result, err := t.promptGetHandler(ctx, session, &request)
	if err != nil {
		return nil, fmt.Errorf("prompt get handler error: %w", err)
	}

	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal prompt get result: %w", err)
	}

	return &spec.JSONRPCMessage{
		JSONRPC: spec.JSONRPCVersion,
		ID:      message.ID,
		Result:  resultBytes,
	}, nil
}

// handleLoggingSetLevel processes logging set level requests
func (t *HTTPServerTransport) handleLoggingSetLevel(ctx context.Context, message *spec.JSONRPCMessage, session *httpClientSession) (*spec.JSONRPCMessage, error) {
	if t.loggingSetLevelHandler == nil {
		return createEmptyResponse(message), nil
	}

	var request spec.SetLevelRequest
	if err := json.Unmarshal(message.Params, &request); err != nil {
		return nil, fmt.Errorf("failed to unmarshal logging set level params: %w", err)
	}

	if err := t.loggingSetLevelHandler(ctx, session, &request); err != nil {
		return nil, fmt.Errorf("logging set level handler error: %w", err)
	}

	return createEmptyResponse(message), nil
}

// handleCreateMessage processes standard (non-streaming) message creation requests
func (t *HTTPServerTransport) handleCreateMessage(ctx context.Context, message *spec.JSONRPCMessage, session *httpClientSession) (*spec.JSONRPCMessage, error) {
	if t.createMessageHandler == nil {
		return createEmptyResponse(message), nil
	}

	var request spec.CreateMessageRequest
	if err := json.Unmarshal(message.Params, &request); err != nil {
		return nil, fmt.Errorf("failed to unmarshal create message params: %w", err)
	}

	response, err := t.createMessageHandler(ctx, session, &request)
	if err != nil {
		return nil, fmt.Errorf("create message handler error: %w", err)
	}

	resultBytes, err := json.Marshal(response.Result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal create message result: %w", err)
	}

	return &spec.JSONRPCMessage{
		JSONRPC: spec.JSONRPCVersion,
		ID:      message.ID,
		Result:  resultBytes,
	}, nil
}

// handleCreateMessageStream processes streaming message creation requests
func (t *HTTPServerTransport) handleCreateMessageStream(ctx context.Context, message *spec.JSONRPCMessage, session *httpClientSession) (*spec.JSONRPCMessage, error) {
	if t.createMessageHandler == nil {
		return createEmptyResponse(message), nil
	}

	var request spec.CreateMessageRequest
	if err := json.Unmarshal(message.Params, &request); err != nil {
		return nil, fmt.Errorf("failed to unmarshal create message stream params: %w", err)
	}

	// Extract or generate a request ID for streaming
	requestID := ""
	if request.Metadata != nil {
		if id, ok := request.Metadata["requestId"].(string); ok {
			requestID = id
		}
	}

	if requestID == "" {
		// Generate a UUID for the request
		requestID = generateRequestID()

		// Add it to the request's metadata
		if request.Metadata == nil {
			request.Metadata = make(map[string]interface{})
		}
		request.Metadata["requestId"] = requestID
	}

	// Associate the requestID with this session for later use
	session.mu.Lock()
	session.metadata["latestStreamRequestID"] = requestID
	session.mu.Unlock()

	// Create a channel for streaming results
	streamCh := make(chan *spec.CreateMessageResult, 10)

	// Store the stream channel with both requestID and sessionID
	t.streamsMu.Lock()
	t.activeStreams[requestID] = streamCh
	// Also store a mapping from session ID to request ID for easier lookups
	sessionKey := fmt.Sprintf("session:%s", session.id)
	t.activeStreams[sessionKey] = streamCh
	t.streamsMu.Unlock()

	// Set up goroutine for message generation
	go t.generateMessageStream(ctx, &request, streamCh, session)

	// Create initial response
	initialContent := ""
	result := &spec.CreateMessageResult{
		Role:    "assistant",
		Content: initialContent,
		Metadata: map[string]interface{}{
			"requestId": requestID,
		},
	}

	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal create message stream result: %w", err)
	}

	return &spec.JSONRPCMessage{
		JSONRPC: spec.JSONRPCVersion,
		ID:      message.ID,
		Result:  resultBytes,
	}, nil
}

// generateMessageStream handles the actual message generation and streams results
func (t *HTTPServerTransport) generateMessageStream(ctx context.Context, request *spec.CreateMessageRequest, streamCh chan<- *spec.CreateMessageResult, session *httpClientSession) {
	defer close(streamCh)

	// Get the request ID
	requestID := ""
	if request.Metadata != nil {
		if id, ok := request.Metadata["requestId"].(string); ok {
			requestID = id
		}
	}

	// Call the message creation handler
	response, err := t.createMessageHandler(ctx, session, request)
	if err != nil {
		// Send error
		errorResult := &spec.CreateMessageResult{
			Role:    "assistant",
			Content: fmt.Sprintf("Error: %v", err),
			Metadata: map[string]interface{}{
				"requestId": requestID,
			},
			StopReason: spec.StopReasonError,
		}

		select {
		case streamCh <- errorResult:
			// Successfully sent error
		case <-ctx.Done():
			// Context canceled
		}
		return
	}

	// For demonstration, split the response into chunks and stream them
	content := response.Result.Content
	chunkSize := 20 // Adjust as needed

	for i := 0; i < len(content); i += chunkSize {
		select {
		case <-ctx.Done():
			// Context canceled
			return
		default:
			// Continue processing
		}

		// Get the next chunk
		end := i + chunkSize
		if end > len(content) {
			end = len(content)
		}

		chunk := content[i:end]
		isLast := end == len(content)

		// Create a result for this chunk
		result := &spec.CreateMessageResult{
			Role:    "assistant",
			Content: chunk,
			Metadata: map[string]interface{}{
				"requestId": requestID,
			},
		}

		// Set the stop reason if this is the last chunk
		if isLast {
			result.StopReason = response.Result.StopReason
			if result.StopReason == "" {
				result.StopReason = spec.StopReasonEndTurn
			}
		}

		// Send the chunk
		select {
		case streamCh <- result:
			// Successfully sent chunk
		case <-ctx.Done():
			// Context canceled
			return
		}

		// Small delay to simulate streaming (remove in production)
		time.Sleep(100 * time.Millisecond)
	}
}

// SendToStream sends a message to an active stream by request ID
func (t *HTTPServerTransport) SendToStream(requestID string, result *spec.CreateMessageResult) error {
	t.streamsMu.RLock()
	stream, exists := t.activeStreams[requestID]
	t.streamsMu.RUnlock()

	if !exists {
		return fmt.Errorf("stream with ID %s not found", requestID)
	}

	// Clone the result to avoid modifying the original
	resultCopy := *result

	// Add requestID to metadata if not present
	if resultCopy.Metadata == nil {
		resultCopy.Metadata = make(map[string]interface{})
	}
	if _, exists := resultCopy.Metadata["requestId"]; !exists {
		resultCopy.Metadata["requestId"] = requestID
	}

	// Send to stream
	select {
	case stream <- &resultCopy:
		return nil
	default:
		return fmt.Errorf("stream channel buffer full or closed")
	}
}

// SendToSessionStream sends a message to the most recent stream opened by a session
func (t *HTTPServerTransport) SendToSessionStream(sessionID string, result *spec.CreateMessageResult) error {
	// Get session
	t.mu.RLock()
	session, exists := t.sessions[sessionID]
	t.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session %s not found", sessionID)
	}

	// Get the latest stream request ID from the session
	session.mu.RLock()
	requestID, ok := session.metadata["latestStreamRequestID"].(string)
	session.mu.RUnlock()

	if !ok || requestID == "" {
		return fmt.Errorf("no active stream for session %s", sessionID)
	}

	// Send to the stream
	return t.SendToStream(requestID, result)
}

// CloseStream closes an active stream by request ID
func (t *HTTPServerTransport) CloseStream(requestID string, finalResult *spec.CreateMessageResult) error {
	t.streamsMu.RLock()
	stream, exists := t.activeStreams[requestID]
	t.streamsMu.RUnlock()

	if !exists {
		return fmt.Errorf("stream with ID %s not found", requestID)
	}

	// If a final result is provided, send it
	if finalResult != nil {
		// Make sure we have a stop reason
		if finalResult.StopReason == "" {
			finalResult.StopReason = spec.StopReasonEndTurn
		}

		// Add requestID to metadata if not present
		if finalResult.Metadata == nil {
			finalResult.Metadata = make(map[string]interface{})
		}
		if _, exists := finalResult.Metadata["requestId"]; !exists {
			finalResult.Metadata["requestId"] = requestID
		}

		// Send the final result
		select {
		case stream <- finalResult:
			// Successfully sent
		default:
			// Channel closed or full, continue to cleanup
		}
	}

	// Remove from active streams map
	t.streamsMu.Lock()
	delete(t.activeStreams, requestID)
	// Also remove any session mapping to this stream
	for key, s := range t.activeStreams {
		if s == stream && key != requestID {
			delete(t.activeStreams, key)
		}
	}
	t.streamsMu.Unlock()

	// Close the channel
	close(stream)

	return nil
}

// CloseSessionStream closes the most recent stream opened by a session
func (t *HTTPServerTransport) CloseSessionStream(sessionID string, finalResult *spec.CreateMessageResult) error {
	// Get session
	t.mu.RLock()
	session, exists := t.sessions[sessionID]
	t.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session %s not found", sessionID)
	}

	// Get the latest stream request ID from the session
	session.mu.RLock()
	requestID, ok := session.metadata["latestStreamRequestID"].(string)
	session.mu.RUnlock()

	if !ok || requestID == "" {
		return fmt.Errorf("no active stream for session %s", sessionID)
	}

	// Close the stream
	return t.CloseStream(requestID, finalResult)
}

// GetSessionStreams returns all active stream request IDs for a session
func (t *HTTPServerTransport) GetSessionStreams(sessionID string) []string {
	var streams []string

	// Check if session exists
	t.mu.RLock()
	_, exists := t.sessions[sessionID]
	t.mu.RUnlock()

	if !exists {
		return streams
	}

	// Look for stream keys that match the session ID pattern
	sessionKey := fmt.Sprintf("session:%s", sessionID)

	t.streamsMu.RLock()
	for id := range t.activeStreams {
		if id == sessionKey {
			continue // Skip the session mapping entry
		}

		// Check if this stream belongs to our session
		if stream, ok := t.activeStreams[id]; ok {
			if sessionStream, ok := t.activeStreams[sessionKey]; ok && stream == sessionStream {
				streams = append(streams, id)
			}
		}
	}
	t.streamsMu.RUnlock()

	return streams
}

// Helper methods

// getOrCreateSessionID extracts a session ID from the request or creates a new one
func (t *HTTPServerTransport) getOrCreateSessionID(r *http.Request) string {
	// Try to get from header
	sessionID := r.Header.Get("X-MCP-Session-ID")
	if sessionID != "" {
		t.logger.Debug("Using session ID from header", "sessionID", sessionID)
		return sessionID
	}

	// Try to get from query parameter "sid"
	if sid := r.URL.Query().Get("sid"); sid == "" {
		// URL-decode the sid parameter
		decodedSid, err := url.QueryUnescape(sid)
		if err != nil {
			t.logger.Warn("Failed to URL-decode sid parameter", "sid", sid, "error", err)
		} else if decodedSid != "" {
			t.logger.Debug("Using session ID from query parameter", "sessionID", decodedSid)
			return decodedSid
		}
	} else {
		return sid
	}

	// Try to get from cookie
	cookie, err := r.Cookie("MCP-Session-ID")
	if err == nil && cookie.Value != "" {
		t.logger.Debug("Using session ID from cookie", "sessionID", cookie.Value)
		return cookie.Value
	}

	// Generate a new session ID
	newID := generateSessionID()
	t.logger.Debug("Generated new session ID", "sessionID", newID)
	return newID
}

// getOrCreateSession gets or creates a client session
func (t *HTTPServerTransport) getOrCreateSession(sessionID string, w http.ResponseWriter) *httpClientSession {
	t.mu.Lock()
	defer t.mu.Unlock()

	session, exists := t.sessions[sessionID]
	if !exists {
		// Create a new session with the transport's logger
		session = createClientSession(sessionID, w, t.logger)
		t.sessions[sessionID] = session
		t.logger.Debug("Created new client session", "sessionID", sessionID)
	}

	return session
}

// generateSessionID generates a unique session ID
func generateSessionID() string {
	return fmt.Sprintf("session-%d", time.Now().UnixNano())
}

// generateRequestID generates a unique request ID
func generateRequestID() string {
	return fmt.Sprintf("req-%d", time.Now().UnixNano())
}

// createEmptyResponse creates an empty JSON-RPC response
func createEmptyResponse(request *spec.JSONRPCMessage) *spec.JSONRPCMessage {
	return &spec.JSONRPCMessage{
		JSONRPC: spec.JSONRPCVersion,
		ID:      request.ID,
		Result:  []byte("{}"),
	}
}

// writeJSONError writes a JSON-RPC error response
func writeJSONError(w http.ResponseWriter, err *spec.JSONRPCError) {
	response := spec.JSONRPCMessage{
		JSONRPC: spec.JSONRPCVersion,
		Error:   err,
	}

	responseBytes, _ := json.Marshal(response)
	w.WriteHeader(http.StatusInternalServerError)
	w.Write(responseBytes)
}

// Implementation of required handler registration methods from the McpServerTransport interface

func (t *HTTPServerTransport) SetInitializeHandler(handler func(ctx context.Context, session spec.McpClientSession, request *spec.InitializeRequest) (*spec.InitializeResult, error)) {
	t.initializeHandler = handler
}

func (t *HTTPServerTransport) SetPingHandler(handler func(ctx context.Context, session spec.McpClientSession) error) {
	t.pingHandler = handler
}

func (t *HTTPServerTransport) SetToolsListHandler(handler func(ctx context.Context, session spec.McpClientSession) (*spec.ListToolsResult, error)) {
	t.toolsListHandler = handler
}

func (t *HTTPServerTransport) SetToolsCallHandler(handler func(ctx context.Context, session spec.McpClientSession, request *spec.CallToolRequest) (*spec.CallToolResult, error)) {
	t.toolsCallHandler = handler
}

func (t *HTTPServerTransport) SetResourcesListHandler(handler func(ctx context.Context, session spec.McpClientSession) (*spec.ListResourcesResult, error)) {
	t.resourcesListHandler = handler
}

func (t *HTTPServerTransport) SetResourcesReadHandler(handler func(ctx context.Context, session spec.McpClientSession, request *spec.ReadResourceRequest) (*spec.ReadResourceResult, error)) {
	t.resourcesReadHandler = handler
}

func (t *HTTPServerTransport) SetResourcesSubscribeHandler(handler func(ctx context.Context, session spec.McpClientSession, request *spec.SubscribeRequest) error) {
	t.resourcesSubscribeHandler = handler
}

func (t *HTTPServerTransport) SetResourcesUnsubscribeHandler(handler func(ctx context.Context, session spec.McpClientSession, request *spec.UnsubscribeRequest) error) {
	t.resourcesUnsubscribeHandler = handler
}

func (t *HTTPServerTransport) SetPromptsListHandler(handler func(ctx context.Context, session spec.McpClientSession) (*spec.ListPromptsResult, error)) {
	t.promptsListHandler = handler
}

func (t *HTTPServerTransport) SetPromptGetHandler(handler func(ctx context.Context, session spec.McpClientSession, request *spec.GetPromptRequest) (*spec.GetPromptResult, error)) {
	t.promptGetHandler = handler
}

func (t *HTTPServerTransport) SetLoggingSetLevelHandler(handler func(ctx context.Context, session spec.McpClientSession, request *spec.SetLevelRequest) error) {
	t.loggingSetLevelHandler = handler
}

func (t *HTTPServerTransport) SetCreateMessageHandler(handler func(ctx context.Context, session spec.McpClientSession, request *spec.CreateMessageRequest) (*spec.CreateMessageResponse, error)) {
	t.createMessageHandler = handler
}

// HTTPServerTransportProvider implements the McpServerTransportProvider interface
type HTTPServerTransportProvider struct {
	addr    string
	options []HTTPServerOption
	logger  util.Logger

	transport *HTTPServerTransport
	mu        sync.Mutex
}

// NewHTTPServerTransportProvider creates a new HTTP server transport provider
func NewHTTPServerTransportProvider(addr string, options ...HTTPServerOption) *HTTPServerTransportProvider {
	// Create default logger
	logger := util.DefaultRootLogger().WithComponent("HTTPServerTransportProvider")

	return &HTTPServerTransportProvider{
		addr:    addr,
		options: options,
		logger:  logger,
	}
}

// WithProviderLogger sets a custom logger for the server transport provider
func WithProviderLogger(logger util.Logger) func(*HTTPServerTransportProvider) {
	return func(p *HTTPServerTransportProvider) {
		if logger != nil {
			p.logger = logger.WithComponent("HTTPServerTransportProvider")
		}
	}
}

// WithAPIToken sets a custom API token for the server transport provider
func (p *HTTPServerTransportProvider) WithAPIToken(token string) *HTTPServerTransportProvider {
	p.logger.Info("Setting API token for HTTP server transport")
	p.options = append(p.options, WithAPIToken(token))
	return p
}

// CreateTransport creates a new server transport
func (p *HTTPServerTransportProvider) CreateTransport() (spec.McpServerTransport, error) {
	// Add logger option if not already present
	hasLoggerOption := false
	for _, opt := range p.options {
		// This is an approximate way to check if logger option is present
		// We can't directly check function equality
		if fmt.Sprintf("%T", opt) == fmt.Sprintf("%T", WithServerLogger(nil)) {
			hasLoggerOption = true
			break
		}
	}

	if !hasLoggerOption {
		p.options = append(p.options, WithServerLogger(p.logger))
	}

	p.logger.Debug("Creating new HTTP server transport", "addr", p.addr)
	transport := NewHTTPServerTransport(p.addr, p.options...)
	return transport, nil
}

// GetTransport returns an existing transport or creates a new one
func (p *HTTPServerTransportProvider) GetTransport() (spec.McpServerTransport, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.transport == nil {
		p.logger.Info("Initializing HTTP server transport", "addr", p.addr)

		// Add logger option if not already present
		hasLoggerOption := false
		for _, opt := range p.options {
			// This is an approximate way to check if logger option is present
			if fmt.Sprintf("%T", opt) == fmt.Sprintf("%T", WithServerLogger(nil)) {
				hasLoggerOption = true
				break
			}
		}

		if !hasLoggerOption {
			p.options = append(p.options, WithServerLogger(p.logger))
		}

		transport := NewHTTPServerTransport(p.addr, p.options...)
		p.transport = transport
	}

	return p.transport, nil
}
