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
	"sync"
	"time"

	"github.com/modelcontextprotocol-ce/go-sdk/spec"
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

	// State
	debug     bool
	isRunning bool
	mu        sync.RWMutex
	sessions  map[string]*httpClientSession
}

// NewHTTPServerTransport creates a new HTTP server transport
func NewHTTPServerTransport(addr string, options ...HTTPServerOption) *HTTPServerTransport {
	router := http.NewServeMux()

	t := &HTTPServerTransport{
		addr:          addr,
		router:        router,
		server:        &http.Server{Addr: addr, Handler: router},
		activeStreams: make(map[string]chan *spec.CreateMessageResult),
		sessions:      make(map[string]*httpClientSession),
		debug:         false,
	}

	// Apply options
	for _, opt := range options {
		opt(t)
	}

	// Set up routes
	t.setupRoutes()

	return t
}

// HTTPServerOption allows for customizing the HTTP server transport
type HTTPServerOption func(*HTTPServerTransport)

// WithServerDebug enables debug logging for the server
func WithServerDebug(debug bool) HTTPServerOption {
	return func(t *HTTPServerTransport) {
		t.debug = debug
	}
}

// WithCustomServer sets a custom HTTP server
func WithCustomServer(server *http.Server) HTTPServerOption {
	return func(t *HTTPServerTransport) {
		t.server = server
		t.server.Handler = t.router
	}
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
		return errors.New("server is already running")
	}

	t.isRunning = true

	if t.debug {
		log.Printf("Starting HTTP server on %s", t.addr)
	}

	// Start the server in a goroutine
	go func() {
		if err := t.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	return nil
}

// Stop stops the server immediately
func (t *HTTPServerTransport) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.isRunning {
		return nil
	}

	if t.debug {
		log.Printf("Stopping HTTP server")
	}

	// Close all active streams
	t.streamsMu.Lock()
	for id, stream := range t.activeStreams {
		close(stream)
		delete(t.activeStreams, id)
	}
	t.streamsMu.Unlock()

	// Shutdown the server
	if err := t.server.Close(); err != nil {
		return fmt.Errorf("server close error: %w", err)
	}

	t.isRunning = false
	return nil
}

// StopGracefully stops the server gracefully
func (t *HTTPServerTransport) StopGracefully(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.isRunning {
		return nil
	}

	if t.debug {
		log.Printf("Gracefully stopping HTTP server")
	}

	// Close all active streams
	t.streamsMu.Lock()
	for id, stream := range t.activeStreams {
		close(stream)
		delete(t.activeStreams, id)
	}
	t.streamsMu.Unlock()

	// Shutdown the server gracefully
	if err := t.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown error: %w", err)
	}

	t.isRunning = false
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
		return
	}

	// Set headers
	w.Header().Set("Content-Type", "application/json")

	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
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
		writeJSONError(w, &spec.JSONRPCError{
			Code:    -32700,
			Message: "Parse error",
			Data:    json.RawMessage([]byte(fmt.Sprintf("Invalid JSON: %v", err))),
		})
		return
	}

	// Extract or create session ID from headers or cookies
	sessionID := t.getOrCreateSessionID(r)

	// Get or create session
	session := t.getOrCreateSession(sessionID, w)

	// Process the request based on method
	response, err := t.processJSONRPCRequest(r.Context(), &message, session)
	if err != nil {
		if t.debug {
			log.Printf("ERROR processing request: %v", err)
		}
		writeJSONError(w, &spec.JSONRPCError{
			Code:    -32000,
			Message: "Server error",
			Data:    json.RawMessage([]byte(err.Error())),
		})
		return
	}
	// Marshal the response
	responseBytes, err := json.Marshal(response)
	if err != nil {
		writeJSONError(w, &spec.JSONRPCError{
			Code:    -32603,
			Message: "Internal error",
			Data:    json.RawMessage([]byte(fmt.Sprintf("Failed to marshal response: %v", err))),
		})
		return
	}

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
		return
	}

	// Check if the stream exists
	t.streamsMu.RLock()
	stream, exists := t.activeStreams[requestID]
	t.streamsMu.RUnlock()

	if !exists {
		http.Error(w, "Stream not found", http.StatusNotFound)
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
		return
	}

	// Stream results
	for {
		select {
		case <-notify:
			// Client disconnected
			return

		case result, ok := <-stream:
			if !ok {
				// Channel closed, end streaming
				return
			}

			// Marshal the result to JSON
			data, err := json.Marshal(result)
			if err != nil {
				if t.debug {
					log.Printf("ERROR: Failed to marshal streaming result: %v", err)
				}
				continue
			}

			// Send the SSE message
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()

			// If this is the final message, end streaming
			if result.StopReason != "" {
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

	// Log connection if debug is enabled
	if t.debug {
		log.Printf("Client connected to default SSE endpoint: %s", sessionID)
	}

	// Create ping ticker
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	// Send initial endpoint message - this is what VS Code MCP clients expect
	session.Respond(r.Context(), "endpoint", "/jsonrpc")

	// Stream ping messages until the client disconnects
	for {
		select {
		case <-notify:
			// Client disconnected
			if t.debug {
				log.Printf("Client disconnected from default SSE endpoint: %s", sessionID)
			}
			return

		case <-ticker.C:
			// Send an endpoint message with server info
			session.Respond(context.Background(), "endpoint", "/jsonrpc")

			// Update session last active time
			session.lastActive = time.Now()
		}
	}
}

// processJSONRPCRequest processes a JSON-RPC request and returns a response
func (t *HTTPServerTransport) processJSONRPCRequest(ctx context.Context, message *spec.JSONRPCMessage, session *httpClientSession) (*spec.JSONRPCMessage, error) {
	// Update session last active time
	session.lastActive = time.Now()

	// Process by method
	switch message.Method {
	case spec.MethodInitialize:
		return t.handleInitialize(ctx, message, session)

	case spec.MethodPing:
		return t.handlePing(ctx, message, session)

	case spec.MethodToolsList:
		return t.handleToolsList(ctx, message, session)

	case spec.MethodToolsCall:
		return t.handleToolsCall(ctx, message, session)

	case spec.MethodResourcesList:
		return t.handleResourcesList(ctx, message, session)

	case spec.MethodResourcesRead:
		return t.handleResourcesRead(ctx, message, session)

	case spec.MethodResourcesSubscribe:
		return t.handleResourcesSubscribe(ctx, message, session)

	case spec.MethodResourcesUnsubscribe:
		return t.handleResourcesUnsubscribe(ctx, message, session)

	case spec.MethodPromptList:
		return t.handlePromptsList(ctx, message, session)

	case spec.MethodPromptGet:
		return t.handlePromptGet(ctx, message, session)

	case spec.MethodLoggingSetLevel:
		return t.handleLoggingSetLevel(ctx, message, session)

	case spec.MethodSamplingCreateMessage:
		return t.handleCreateMessage(ctx, message, session)

	case spec.MethodContentCreateMessageStream:
		return t.handleCreateMessageStream(ctx, message, session)

	default:
		return nil, fmt.Errorf("unsupported method: %s", message.Method)
	}
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
		return sessionID
	}

	// Try to get from cookie
	cookie, err := r.Cookie("MCP-Session-ID")
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}

	// Generate a new session ID
	return generateSessionID()
}

// getOrCreateSession gets or creates a client session
func (t *HTTPServerTransport) getOrCreateSession(sessionID string, w http.ResponseWriter) *httpClientSession {
	t.mu.Lock()
	defer t.mu.Unlock()

	session, exists := t.sessions[sessionID]
	if !exists {
		session = createClientSession(sessionID, w)
		t.sessions[sessionID] = session
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

	transport *HTTPServerTransport
	mu        sync.Mutex
}

// NewHTTPServerTransportProvider creates a new HTTP server transport provider
func NewHTTPServerTransportProvider(addr string, options ...HTTPServerOption) *HTTPServerTransportProvider {
	return &HTTPServerTransportProvider{
		addr:    addr,
		options: options,
	}
}

// CreateTransport creates a new server transport
func (p *HTTPServerTransportProvider) CreateTransport() (spec.McpServerTransport, error) {
	transport := NewHTTPServerTransport(p.addr, p.options...)
	return transport, nil
}

// GetTransport returns an existing transport or creates a new one
func (p *HTTPServerTransportProvider) GetTransport() (spec.McpServerTransport, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.transport == nil {
		transport := NewHTTPServerTransport(p.addr, p.options...)
		p.transport = transport
	}

	return p.transport, nil
}
