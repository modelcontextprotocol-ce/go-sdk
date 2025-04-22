// Package stream provides implementations for streaming message creation
package stream

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol-ce/go-sdk/spec"
	"github.com/modelcontextprotocol-ce/go-sdk/util"
)

// HTTPClientTransport implements an HTTP client transport with support for 
// standard POST requests and Server-Sent Events (SSE) streaming.
type HTTPClientTransport struct {
	baseURL           string
	httpClient        *http.Client
	handler           spec.MessageHandler
	mu                sync.Mutex
	closed            bool
	activeStreams     map[string]context.CancelFunc
	streamsMu         sync.Mutex
	debug             bool
	requestTimeout    time.Duration
}

// NewHTTPClientTransport creates a new HTTP client transport.
func NewHTTPClientTransport(baseURL string, options ...HTTPClientOption) *HTTPClientTransport {
	t := &HTTPClientTransport{
		baseURL:        baseURL,
		httpClient:     &http.Client{Timeout: 30 * time.Second},
		activeStreams:  make(map[string]context.CancelFunc),
		requestTimeout: 30 * time.Second,
		debug:          false,
	}

	// Apply options
	for _, opt := range options {
		opt(t)
	}

	return t
}

// HTTPClientOption allows for customizing the HTTP client transport
type HTTPClientOption func(*HTTPClientTransport)

// WithDebug enables debug logging
func WithDebug(debug bool) HTTPClientOption {
	return func(t *HTTPClientTransport) {
		t.debug = debug
	}
}

// WithHTTPClient sets a custom HTTP client
func WithHTTPClient(client *http.Client) HTTPClientOption {
	return func(t *HTTPClientTransport) {
		t.httpClient = client
	}
}

// WithRequestTimeout sets the request timeout
func WithRequestTimeout(timeout time.Duration) HTTPClientOption {
	return func(t *HTTPClientTransport) {
		t.requestTimeout = timeout
		
		// If the client already exists, update its timeout
		if t.httpClient != nil {
			t.httpClient.Timeout = timeout
		}
	}
}

// Connect establishes a connection and registers the message handler.
func (t *HTTPClientTransport) Connect(ctx context.Context, handler spec.MessageHandler) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return errors.New("transport is closed")
	}

	// Store the handler
	t.handler = handler
	
	if t.debug {
		fmt.Println("HTTP transport connected to", t.baseURL)
	}
	
	return nil
}

// Send sends raw message data over the transport.
func (t *HTTPClientTransport) Send(message []byte) error {
	if t.closed {
		return errors.New("transport is closed")
	}

	if t.debug {
		fmt.Printf("DEBUG: Sent raw message: %s\n", string(message))
	}
	
	// This method is required by the interface but not directly used in HTTP transport
	return nil
}

// SendMessage sends a JSON-RPC message via HTTP POST and returns the response.
// For streaming requests, sets up an SSE connection.
func (t *HTTPClientTransport) SendMessage(ctx context.Context, message *spec.JSONRPCMessage) (*spec.JSONRPCMessage, error) {
	t.mu.Lock()
	
	if t.closed {
		t.mu.Unlock()
		return nil, errors.New("transport is closed")
	}
	
	// Debug logging
	if t.debug {
		jsonBytes, _ := json.MarshalIndent(message, "", "  ")
		fmt.Printf("DEBUG: Sending message: %s\n", string(jsonBytes))
	}
	
	t.mu.Unlock()

	// For streaming message creation, handle it specially
	if message.Method == spec.MethodContentCreateMessageStream {
		return t.handleStreamingRequest(ctx, message)
	}
	
	// For all other methods, use standard POST request
	return t.sendPostRequest(ctx, message)
}

// sendPostRequest sends a standard JSON-RPC request via HTTP POST
func (t *HTTPClientTransport) sendPostRequest(ctx context.Context, message *spec.JSONRPCMessage) (*spec.JSONRPCMessage, error) {
	// Marshal the JSON-RPC message
	jsonData, err := json.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}
	
	// Create the HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", t.baseURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	
	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	
	// Send the request
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()
	
	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned non-200 status: %d - %s", resp.StatusCode, string(body))
	}
	
	// Decode the response
	var response spec.JSONRPCMessage
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	
	// Check for error in response
	if response.Error != nil {
		return &response, fmt.Errorf("server returned error: %s", response.Error.Message)
	}
	
	return &response, nil
}

// handleStreamingRequest handles streaming message creation
func (t *HTTPClientTransport) handleStreamingRequest(ctx context.Context, message *spec.JSONRPCMessage) (*spec.JSONRPCMessage, error) {
	// Parse request to get metadata
	var request struct {
		Content  string                 `json:"content"`
		Metadata map[string]interface{} `json:"metadata"`
	}
	if err := json.Unmarshal(message.Params, &request); err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}
	
	// Extract or generate requestId for streaming
	requestID, _ := request.Metadata["requestId"].(string)
	if requestID == "" {
		requestID = util.GenerateUUID()
		
		// Update the metadata with the generated ID
		if request.Metadata == nil {
			request.Metadata = make(map[string]interface{})
		}
		request.Metadata["requestId"] = requestID
		
		// Re-marshal the params with the updated metadata
		updatedParams, err := json.Marshal(request)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal updated params: %w", err)
		}
		message.Params = updatedParams
	}
	
	// Send initial request via POST
	initialResponse, err := t.sendPostRequest(ctx, message)
	if err != nil {
		return nil, err
	}
	
	// Parse the initial response to get the request ID if it wasn't provided
	var initialResult spec.CreateMessageResult
	if err := json.Unmarshal(initialResponse.Result, &initialResult); err != nil {
		return nil, fmt.Errorf("failed to parse initial response: %w", err)
	}
	
	// Use the request ID from the response if we didn't have one
	if initialResult.Metadata != nil {
		if responseID, ok := initialResult.Metadata["requestId"].(string); ok && responseID != "" {
			requestID = responseID
		}
	}
	
	// Start SSE connection for streaming updates
	streamCtx, cancel := context.WithCancel(context.Background())
	
	// Store the cancel function so we can stop the stream later
	t.streamsMu.Lock()
	t.activeStreams[requestID] = cancel
	t.streamsMu.Unlock()
	
	// Start the SSE connection in a goroutine
	go t.connectToEventStream(streamCtx, requestID)
	
	return initialResponse, nil
}

// connectToEventStream establishes and maintains an SSE connection
func (t *HTTPClientTransport) connectToEventStream(ctx context.Context, requestID string) {
	// Construct SSE endpoint URL
	sseURL := fmt.Sprintf("%s/stream/%s", t.baseURL, requestID)
	
	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", sseURL, nil)
	if err != nil {
		if t.debug {
			fmt.Printf("DEBUG: Failed to create SSE request: %v\n", err)
		}
		return
	}
	
	// Set headers for SSE
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "keep-alive")
	
	// Use a client without timeout for streaming
	client := &http.Client{
		Timeout: 0, // No timeout for streaming
	}
	
	// Make the request
	resp, err := client.Do(req)
	if err != nil {
		if t.debug {
			fmt.Printf("DEBUG: SSE connection failed: %v\n", err)
		}
		return
	}
	defer resp.Body.Close()
	
	// Check status code
	if resp.StatusCode != http.StatusOK {
		if t.debug {
			fmt.Printf("DEBUG: SSE connection returned non-200 status: %d\n", resp.StatusCode)
		}
		return
	}
	
	// Process the event stream
	reader := bufio.NewReader(resp.Body)
	
	// Variables for parsing the SSE stream
	var (
		line     string
		eventData strings.Builder
	)
	
	for {
		select {
		case <-ctx.Done():
			// Context was canceled, exit the loop
			if t.debug {
				fmt.Println("DEBUG: SSE stream canceled")
			}
			return
		default:
			// Continue reading the stream
			line, err = reader.ReadString('\n')
			if err != nil {
				// Check if it's EOF or real error
				if err != io.EOF {
					if t.debug {
						fmt.Printf("DEBUG: Error reading SSE stream: %v\n", err)
					}
				}
				return
			}
			
			line = strings.TrimSpace(line)
			
			// Empty line marks the end of an event
			if line == "" {
				data := eventData.String()
				if data != "" {
					// Process the event data
					t.processEventData(requestID, data)
					eventData.Reset()
				}
				continue
			}
			
			// Process the line
			if strings.HasPrefix(line, "data: ") {
				// Append data content
				eventData.WriteString(strings.TrimPrefix(line, "data: "))
			}
		}
	}
}

// processEventData handles incoming SSE events
func (t *HTTPClientTransport) processEventData(requestID string, data string) {
	// Parse the event data as a CreateMessageResult
	var result spec.CreateMessageResult
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		if t.debug {
			fmt.Printf("DEBUG: Failed to parse SSE event: %v\n", err)
		}
		return
	}
	
	// Update the result metadata with the request ID if not present
	if result.Metadata == nil {
		result.Metadata = make(map[string]interface{})
	}
	result.Metadata["requestId"] = requestID
	
	// Create notification message
	notification := &spec.JSONRPCMessage{
		JSONRPC: spec.JSONRPCVersion,
		Method:  spec.MethodMessageStreamResult,
	}
	
	// Marshal the parameters
	params, err := json.Marshal(result)
	if err != nil {
		if t.debug {
			fmt.Printf("DEBUG: Failed to marshal SSE result: %v\n", err)
		}
		return
	}
	notification.Params = params
	
	// Call the handler with the notification
	if t.handler != nil {
		t.mu.Lock()
		handler := t.handler
		t.mu.Unlock()
		
		if handler != nil {
			handler(notification)
		}
	}
	
	// If this is the final message, clean up the stream
	if result.StopReason != "" {
		t.cleanupStream(requestID)
	}
}

// cleanupStream removes a stream from active streams and cancels its context
func (t *HTTPClientTransport) cleanupStream(requestID string) {
	t.streamsMu.Lock()
	defer t.streamsMu.Unlock()
	
	if cancel, ok := t.activeStreams[requestID]; ok {
		cancel() // Cancel the context
		delete(t.activeStreams, requestID)
	}
}

// Close closes the transport and cleans up all active streams
func (t *HTTPClientTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	if t.closed {
		return nil
	}
	
	t.closed = true
	
	// Cancel all active streams
	t.streamsMu.Lock()
	defer t.streamsMu.Unlock()
	
	for _, cancel := range t.activeStreams {
		if cancel != nil {
			cancel()
		}
	}
	
	// Clear the streams map
	t.activeStreams = make(map[string]context.CancelFunc)
	
	if t.debug {
		fmt.Println("HTTP transport closed")
	}
	
	return nil
}