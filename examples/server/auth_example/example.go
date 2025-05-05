package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol-ce/go-sdk/auth"
	"github.com/modelcontextprotocol-ce/go-sdk/server"
	"github.com/modelcontextprotocol-ce/go-sdk/server/stream"
	"github.com/modelcontextprotocol-ce/go-sdk/spec"
)

func main() {
	// Example showing different authentication methods for MCP servers
	fmt.Println("Model Context Protocol Authentication Examples")
	fmt.Println("=============================================")

	// Create a transport provider for our server
	transportProvider := stream.NewHTTPServerTransportProvider(":8080")

	// Example 1: Simple API Token Authentication (original method)
	fmt.Println("\n1. Simple API Token Authentication:")
	_ = server.NewSync(transportProvider).
		WithAPIToken("secret-api-token").
		WithServerInfo(spec.Implementation{
			Name:    "MCP Authentication Example Server",
			Version: "1.0.0",
		}).(server.SyncBuilder).
		WithCreateMessageHandler(handleCreateMessage).
		Build()

	fmt.Println("  - Server configured with API token authentication")
	fmt.Println("  - Clients need to provide token in Authorization header, X-API-Token header, or token query parameter")

	// Example 2: JWT Authentication
	fmt.Println("\n2. JWT Authentication:")
	// Create a JWT authenticator with a secret key and issuer
	jwtAuthenticator := auth.NewJWTAuthenticator(
		[]byte("your-jwt-signing-key"),
		auth.WithIssuer("mcp-example-issuer"),
		auth.WithAudience("mcp-example-audience"),
	)

	_ = server.NewSync(transportProvider).
		WithAuthenticator(jwtAuthenticator).
		WithServerInfo(spec.Implementation{
			Name:    "MCP JWT Example Server",
			Version: "1.0.0",
		}).(server.SyncBuilder).WithCreateMessageHandler(handleCreateMessage).Build()

	fmt.Println("  - Server configured with JWT authentication")
	fmt.Println("  - Clients need to provide a valid JWT token in the Authorization header")
	fmt.Println("  - Token must be signed with the correct key and have valid issuer and audience claims")

	// Example 3: OAuth2/OIDC Authentication
	fmt.Println("\n3. OAuth2/OIDC Authentication:")

	// Create a sample OIDC verifier (in a real app, you'd use a proper OIDC library)
	oidcVerifier, _ := auth.NewDefaultOIDCVerifier("https://example.com/oauth2", "client-id")

	oauth2Authenticator := auth.NewOAuth2Authenticator(
		oidcVerifier,
		auth.WithClientCredentials("client-id", "client-secret"),
	)

	_ = server.NewSync(transportProvider).WithAuthenticator(oauth2Authenticator).WithServerInfo(spec.Implementation{
		Name:    "MCP OAuth2 Example Server",
		Version: "1.0.0",
	}).(server.SyncBuilder).WithCreateMessageHandler(handleCreateMessage).Build()

	fmt.Println("  - Server configured with OAuth2/OIDC authentication")
	fmt.Println("  - Clients need to provide a valid OAuth2 access token or OIDC ID token in the Authorization header")
	fmt.Println("  - Token is verified with the OIDC provider")

	// Example 4: Custom Authentication
	fmt.Println("\n4. Custom Authentication:")

	// Create a custom authenticator with specific business logic
	customAuthenticator := &customAuthenticator{
		validAPIKeys: map[string]bool{
			"user1-api-key": true,
			"user2-api-key": true,
		},
		userRoles: map[string]string{
			"user1-api-key": "admin",
			"user2-api-key": "user",
		},
	}

	_ = server.NewSync(transportProvider).WithAuthenticator(customAuthenticator).WithServerInfo(spec.Implementation{
		Name:    "MCP Custom Auth Example Server",
		Version: "1.0.0",
	}).(server.SyncBuilder).WithCreateMessageHandler(handleCreateMessage).Build()

	fmt.Println("  - Server configured with custom authentication")
	fmt.Println("  - Authentication uses a custom logic to validate API keys")
	fmt.Println("  - The authenticator also provides role information for authorization")

	// Just to demonstrate - in a real app you'd start one of these servers
	fmt.Println("\nIn a real application, you would start one of these servers.")
	fmt.Println("For example:")
	fmt.Println("  err := jwtServer.Start()")
	fmt.Println("  if err != nil {")
	fmt.Println("      log.Fatal(\"Failed to start server:\", err)")
	fmt.Println("  }")
	fmt.Println("  defer jwtServer.Stop()")
}

// Example handler for create message requests
func handleCreateMessage(ctx context.Context, request spec.CreateMessageRequest) (*spec.CreateMessageResponse, error) {
	return &spec.CreateMessageResponse{
		Result: &spec.CreateMessageResult{
			Role:       "assistant",
			Content:    "This is a response from the authentication example server.",
			StopReason: spec.StopReasonEndTurn,
		},
	}, nil
}

// Custom authenticator implementation
type customAuthenticator struct {
	validAPIKeys map[string]bool
	userRoles    map[string]string
}

// Authenticate checks if the request contains a valid API key
func (a *customAuthenticator) Authenticate(r *http.Request) bool {
	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" {
		// Try query parameter
		apiKey = r.URL.Query().Get("api_key")
	}

	if apiKey == "" {
		return false
	}

	// Check if API key is valid
	return a.validAPIKeys[apiKey]
}

// GetAuthInfo returns authentication information including user role
func (a *customAuthenticator) GetAuthInfo(r *http.Request) map[string]interface{} {
	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" {
		// Try query parameter
		apiKey = r.URL.Query().Get("api_key")
	}

	info := map[string]interface{}{
		"auth_type": "custom",
		"valid":     a.validAPIKeys[apiKey],
	}

	if role, ok := a.userRoles[apiKey]; ok {
		info["role"] = role
	}

	return info
}
