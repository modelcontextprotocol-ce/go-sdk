package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/pkg/spec"
	"github.com/modelcontextprotocol/go-sdk/pkg/util"
)

// syncServerImpl implements the McpSyncServer interface
type syncServerImpl struct {
	transportProvider    spec.McpServerTransportProvider
	transport            spec.McpServerTransport
	requestTimeout       time.Duration
	features             *ServerFeatures
	createMessageHandler CreateMessageHandler
	toolHandlers         map[string]ToolHandler
	resourceHandler      ResourceHandler
	promptHandler        PromptHandler
	mu                   sync.RWMutex
	running              bool
	clients              map[string]spec.McpClientSession
	clientsMu            sync.RWMutex
}

// Start implements McpServer
func (s *syncServerImpl) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return errors.New("server is already running")
	}

	transport, err := s.transportProvider.GetTransport()
	if err != nil {
		return fmt.Errorf("failed to get transport: %w", err)
	}

	s.transport = transport
	s.running = true
	s.clients = make(map[string]spec.McpClientSession)

	// Set up handlers for the transport
	transport.SetInitializeHandler(s.handleInitialize)
	transport.SetPingHandler(s.handlePing)
	transport.SetToolsListHandler(s.handleToolsList)
	transport.SetToolsCallHandler(s.handleToolsCall)
	transport.SetResourcesListHandler(s.handleResourcesList)
	transport.SetResourcesReadHandler(s.handleResourcesRead)
	transport.SetResourcesSubscribeHandler(s.handleResourcesSubscribe)
	transport.SetResourcesUnsubscribeHandler(s.handleResourcesUnsubscribe)
	transport.SetPromptsListHandler(s.handlePromptsList)
	transport.SetPromptGetHandler(s.handlePromptGet)
	transport.SetLoggingSetLevelHandler(s.handleLoggingSetLevel)
	transport.SetCreateMessageHandler(s.handleCreateMessage)

	if err := transport.Start(); err != nil {
		s.running = false
		return fmt.Errorf("failed to start transport: %w", err)
	}

	return nil
}

// Stop implements McpServer
func (s *syncServerImpl) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	if err := s.transport.Stop(); err != nil {
		return fmt.Errorf("failed to stop transport: %w", err)
	}

	s.running = false
	return nil
}

// StopGracefully implements McpServer
func (s *syncServerImpl) StopGracefully(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	if err := s.transport.StopGracefully(ctx); err != nil {
		return fmt.Errorf("failed to gracefully stop transport: %w", err)
	}

	s.running = false
	return nil
}

// GetProtocolVersion implements McpServer
func (s *syncServerImpl) GetProtocolVersion() string {
	return s.features.ProtocolVersion
}

// GetServerInfo implements McpServer
func (s *syncServerImpl) GetServerInfo() spec.Implementation {
	return s.features.ServerInfo
}

// GetServerCapabilities implements McpServer
func (s *syncServerImpl) GetServerCapabilities() spec.ServerCapabilities {
	return s.features.ServerCapabilities
}

// BroadcastToolsChanged implements McpServer
func (s *syncServerImpl) BroadcastToolsChanged() error {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	for _, client := range s.clients {
		if err := client.NotifyToolsListChanged(); err != nil {
			return fmt.Errorf("failed to notify client: %w", err)
		}
	}

	return nil
}

// BroadcastResourcesChanged implements McpServer
func (s *syncServerImpl) BroadcastResourcesChanged() error {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	for _, client := range s.clients {
		if err := client.NotifyResourcesListChanged(); err != nil {
			return fmt.Errorf("failed to notify client: %w", err)
		}
	}

	return nil
}

// BroadcastPromptsChanged implements McpServer
func (s *syncServerImpl) BroadcastPromptsChanged() error {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	for _, client := range s.clients {
		if err := client.NotifyPromptsListChanged(); err != nil {
			return fmt.Errorf("failed to notify client: %w", err)
		}
	}

	return nil
}

// BroadcastResourceChanged implements McpServer
func (s *syncServerImpl) BroadcastResourceChanged(uri string, contents []byte) error {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	for _, client := range s.clients {
		if err := client.NotifyResourceChanged(uri, contents); err != nil {
			return fmt.Errorf("failed to notify client: %w", err)
		}
	}

	return nil
}

// SendLogMessage implements McpServer
func (s *syncServerImpl) SendLogMessage(level spec.LogLevel, message string, logger string) error {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	for _, client := range s.clients {
		if err := client.SendLogMessage(level, message, logger); err != nil {
			return fmt.Errorf("failed to send log message: %w", err)
		}
	}

	return nil
}

// RegisterToolHandler implements McpSyncServer
func (s *syncServerImpl) RegisterToolHandler(name string, handler ToolHandler) error {
	util.AssertNotNil(name, "Tool name must not be nil")
	util.AssertNotNil(handler, "Tool handler must not be nil")

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.toolHandlers == nil {
		s.toolHandlers = make(map[string]ToolHandler)
	}

	s.toolHandlers[name] = handler
	return nil
}

// RegisterResourceHandler implements McpSyncServer
func (s *syncServerImpl) RegisterResourceHandler(handler ResourceHandler) error {
	util.AssertNotNil(handler, "Resource handler must not be nil")

	s.mu.Lock()
	defer s.mu.Unlock()

	s.resourceHandler = handler
	return nil
}

// RegisterPromptHandler implements McpSyncServer
func (s *syncServerImpl) RegisterPromptHandler(handler PromptHandler) error {
	util.AssertNotNil(handler, "Prompt handler must not be nil")

	s.mu.Lock()
	defer s.mu.Unlock()

	s.promptHandler = handler
	return nil
}

// SetCreateMessageHandler implements McpSyncServer
func (s *syncServerImpl) SetCreateMessageHandler(handler CreateMessageHandler) error {
	util.AssertNotNil(handler, "Create message handler must not be nil")

	s.mu.Lock()
	defer s.mu.Unlock()

	s.createMessageHandler = handler
	return nil
}

// AddTool implements McpSyncServer
func (s *syncServerImpl) AddTool(tool spec.Tool) error {
	util.AssertNotNil(tool, "Tool must not be nil")
	util.AssertNotNil(tool.Name, "Tool name must not be nil")

	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove existing tool with the same name if it exists
	for i, t := range s.features.AvailableTools {
		if t.Name == tool.Name {
			s.features.AvailableTools = append(s.features.AvailableTools[:i], s.features.AvailableTools[i+1:]...)
			break
		}
	}

	s.features.AvailableTools = append(s.features.AvailableTools, tool)
	return nil
}

// AddResource implements McpSyncServer
func (s *syncServerImpl) AddResource(resource spec.Resource) error {
	util.AssertNotNil(resource, "Resource must not be nil")
	util.AssertNotNil(resource.URI, "Resource URI must not be nil")

	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove existing resource with the same URI if it exists
	for i, r := range s.features.AvailableResources {
		if r.URI == resource.URI {
			s.features.AvailableResources = append(s.features.AvailableResources[:i], s.features.AvailableResources[i+1:]...)
			break
		}
	}

	s.features.AvailableResources = append(s.features.AvailableResources, resource)
	return nil
}

// AddPrompt implements McpSyncServer
func (s *syncServerImpl) AddPrompt(prompt spec.Prompt) error {
	util.AssertNotNil(prompt, "Prompt must not be nil")
	util.AssertNotNil(prompt.Name, "Prompt name must not be nil")

	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove existing prompt with the same name if it exists
	for i, p := range s.features.AvailablePrompts {
		if p.Name == prompt.Name {
			s.features.AvailablePrompts = append(s.features.AvailablePrompts[:i], s.features.AvailablePrompts[i+1:]...)
			break
		}
	}

	s.features.AvailablePrompts = append(s.features.AvailablePrompts, prompt)
	return nil
}

// RemoveTool implements McpSyncServer
func (s *syncServerImpl) RemoveTool(name string) error {
	util.AssertNotNil(name, "Tool name must not be nil")

	s.mu.Lock()
	defer s.mu.Unlock()

	for i, tool := range s.features.AvailableTools {
		if tool.Name == name {
			s.features.AvailableTools = append(s.features.AvailableTools[:i], s.features.AvailableTools[i+1:]...)
			return nil
		}
	}

	return errors.New("tool not found: " + name)
}

// RemoveResource implements McpSyncServer
func (s *syncServerImpl) RemoveResource(uri string) error {
	util.AssertNotNil(uri, "Resource URI must not be nil")

	s.mu.Lock()
	defer s.mu.Unlock()

	for i, resource := range s.features.AvailableResources {
		if resource.URI == uri {
			s.features.AvailableResources = append(s.features.AvailableResources[:i], s.features.AvailableResources[i+1:]...)
			return nil
		}
	}

	return errors.New("resource not found: " + uri)
}

// RemovePrompt implements McpSyncServer
func (s *syncServerImpl) RemovePrompt(name string) error {
	util.AssertNotNil(name, "Prompt name must not be nil")

	s.mu.Lock()
	defer s.mu.Unlock()

	for i, prompt := range s.features.AvailablePrompts {
		if prompt.Name == name {
			s.features.AvailablePrompts = append(s.features.AvailablePrompts[:i], s.features.AvailablePrompts[i+1:]...)
			return nil
		}
	}

	return errors.New("prompt not found: " + name)
}

// Internal request handlers

func (s *syncServerImpl) handleInitialize(ctx context.Context, session spec.McpClientSession, request *spec.InitializeRequest) (*spec.InitializeResult, error) {
	s.clientsMu.Lock()
	s.clients[session.GetID()] = session
	s.clientsMu.Unlock()

	// Return the initialization result
	return &spec.InitializeResult{
		ProtocolVersion: s.features.ProtocolVersion,
		Capabilities:    s.features.ServerCapabilities,
		ServerInfo:      s.features.ServerInfo,
		Instructions:    s.features.Instructions,
	}, nil
}

func (s *syncServerImpl) handlePing(ctx context.Context, session spec.McpClientSession) error {
	// Nothing to do, just return nil
	return nil
}

func (s *syncServerImpl) handleToolsList(ctx context.Context, session spec.McpClientSession) (*spec.ListToolsResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return &spec.ListToolsResult{
		Tools: s.features.AvailableTools,
	}, nil
}

func (s *syncServerImpl) handleToolsCall(ctx context.Context, session spec.McpClientSession, request *spec.CallToolRequest) (*spec.CallToolResult, error) {
	s.mu.RLock()
	handler, ok := s.toolHandlers[request.Name]
	s.mu.RUnlock()

	if !ok || handler == nil {
		return nil, &spec.McpError{
			Code:    spec.ErrCodeMethodNotFound,
			Message: "tool not found: " + request.Name,
		}
	}

	// Convert arguments to JSON bytes
	params, err := json.Marshal(request.Arguments)
	if err != nil {
		return nil, &spec.McpError{
			Code:    spec.ErrCodeInvalidParams,
			Message: "invalid arguments: " + err.Error(),
		}
	}

	// Call the handler
	result, err := handler(ctx, params)
	if err != nil {
		return &spec.CallToolResult{
			Content: []spec.Content{&spec.TextContent{
				Text: "Error: " + err.Error(),
			}},
			IsError: true,
		}, nil
	}

	// Convert result to content
	if result == nil {
		return &spec.CallToolResult{
			Content: []spec.Content{},
		}, nil
	}

	// Try to convert the result to content
	switch v := result.(type) {
	case spec.Content:
		return &spec.CallToolResult{
			Content: []spec.Content{v},
		}, nil
	case []spec.Content:
		return &spec.CallToolResult{
			Content: v,
		}, nil
	default:
		// For other types, convert to string and return as text content
		text, err := json.Marshal(v)
		if err != nil {
			return nil, &spec.McpError{
				Code:    spec.ErrCodeInternalError,
				Message: "failed to marshal result: " + err.Error(),
			}
		}
		return &spec.CallToolResult{
			Content: []spec.Content{&spec.TextContent{
				Text: string(text),
			}},
		}, nil
	}
}

func (s *syncServerImpl) handleResourcesList(ctx context.Context, session spec.McpClientSession) (*spec.ListResourcesResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return &spec.ListResourcesResult{
		Resources: s.features.AvailableResources,
	}, nil
}

func (s *syncServerImpl) handleResourcesRead(ctx context.Context, session spec.McpClientSession, request *spec.ReadResourceRequest) (*spec.ReadResourceResult, error) {
	s.mu.RLock()
	handler := s.resourceHandler
	s.mu.RUnlock()

	if handler == nil {
		return nil, &spec.McpError{
			Code:    spec.ErrCodeMethodNotFound,
			Message: "resource handler not registered",
		}
	}

	content, err := handler(ctx, request.URI)
	if err != nil {
		return nil, &spec.McpError{
			Code:    spec.ErrCodeInternalError,
			Message: "failed to read resource: " + err.Error(),
		}
	}

	// Simple MIME type detection
	mimeType := "text/plain"
	if isBinary(content) {
		mimeType = "application/octet-stream"
	}

	return &spec.ReadResourceResult{
		Contents: []spec.ResourceContents{
			&spec.TextResourceContents{
				URI:      request.URI,
				MimeType: mimeType,
				Text:     string(content),
			},
		},
	}, nil
}

func (s *syncServerImpl) handleResourcesSubscribe(ctx context.Context, session spec.McpClientSession, request *spec.SubscribeRequest) error {
	// Add subscription in the session
	return session.AddSubscription(request.URI)
}

func (s *syncServerImpl) handleResourcesUnsubscribe(ctx context.Context, session spec.McpClientSession, request *spec.UnsubscribeRequest) error {
	// Remove subscription from the session
	return session.RemoveSubscription(request.URI)
}

func (s *syncServerImpl) handlePromptsList(ctx context.Context, session spec.McpClientSession) (*spec.ListPromptsResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return &spec.ListPromptsResult{
		Prompts: s.features.AvailablePrompts,
	}, nil
}

func (s *syncServerImpl) handlePromptGet(ctx context.Context, session spec.McpClientSession, request *spec.GetPromptRequest) (*spec.GetPromptResult, error) {
	s.mu.RLock()
	handler := s.promptHandler
	s.mu.RUnlock()

	if handler == nil {
		return nil, &spec.McpError{
			Code:    spec.ErrCodeMethodNotFound,
			Message: "prompt handler not registered",
		}
	}

	// Call the prompt handler
	text, err := handler(ctx, request.Name, request.Arguments)
	if err != nil {
		return nil, &spec.McpError{
			Code:    spec.ErrCodeInternalError,
			Message: "failed to get prompt: " + err.Error(),
		}
	}

	return &spec.GetPromptResult{
		Messages: []spec.PromptMessage{
			{
				Role: spec.RoleUser,
				Content: &spec.TextContent{
					Text: text,
				},
			},
		},
	}, nil
}

func (s *syncServerImpl) handleLoggingSetLevel(ctx context.Context, session spec.McpClientSession, request *spec.SetLevelRequest) error {
	// For now, we just accept the level change but don't do anything with it
	return nil
}

func (s *syncServerImpl) handleCreateMessage(ctx context.Context, session spec.McpClientSession, request *spec.CreateMessageRequest) (*spec.CreateMessageResponse, error) {
	s.mu.RLock()
	handler := s.createMessageHandler
	s.mu.RUnlock()

	if handler == nil {
		return nil, &spec.McpError{
			Code:    spec.ErrCodeMethodNotFound,
			Message: "create message handler not registered",
		}
	}

	// Call the create message handler
	return handler(ctx, *request)
}

// Helper method to detect binary content
func isBinary(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}
