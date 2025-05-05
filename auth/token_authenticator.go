package auth

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol-ce/go-sdk/util"
)

// TokenAuthenticator implements token-based authentication
type TokenAuthenticator struct {
	apiToken string
}

// NewTokenAuthenticator creates a new token authenticator
func NewTokenAuthenticator(token string) *TokenAuthenticator {
	return &TokenAuthenticator{apiToken: token}
}

// FromConfig creates a new TokenAuthenticator by loading the API token from config
// If configPath is empty, it will use the default config location
func FromConfig(configPath string) (*TokenAuthenticator, error) {
	config, err := util.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load API token from config: %w", err)
	}

	return NewTokenAuthenticator(config.APIToken), nil
}

// GetToken returns the API token
func (a *TokenAuthenticator) GetToken() string {
	return a.apiToken
}

// Authenticate checks if the request contains a valid token
func (a *TokenAuthenticator) Authenticate(r *http.Request) bool {
	if a.apiToken == "" {
		return false
	}

	// Check Authorization header (Bearer token)
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		return a.apiToken == token
	}

	// Check X-API-Token header
	apiTokenHeader := r.Header.Get("X-API-Token")
	if apiTokenHeader != "" {
		return a.apiToken == apiTokenHeader
	}

	// Check token query parameter
	if token := r.URL.Query().Get("token"); token != "" {
		return a.apiToken == token
	}

	return false
}

// GetAuthInfo returns token info
func (a *TokenAuthenticator) GetAuthInfo(r *http.Request) map[string]interface{} {
	return map[string]interface{}{
		"auth_type": "token",
	}
}
