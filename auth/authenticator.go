// Package auth provides authentication interfaces and implementations for MCP servers
package auth

import (
	"net/http"
)

// Authenticator defines an interface for authenticating HTTP requests
type Authenticator interface {
	// Authenticate verifies if the request is authenticated
	// Returns true if authenticated, false otherwise
	Authenticate(r *http.Request) bool

	// GetAuthInfo returns additional authentication information if needed
	// For example, user identity for auditing or authorization
	GetAuthInfo(r *http.Request) map[string]interface{}
}

// NoAuthenticator is an authenticator that always returns true
// Use this for public endpoints or for development
type NoAuthenticator struct{}

// NewNoAuthenticator creates a new authenticator that always authenticates
func NewNoAuthenticator() *NoAuthenticator {
	return &NoAuthenticator{}
}

// Authenticate always returns true
func (a *NoAuthenticator) Authenticate(r *http.Request) bool {
	return true
}

// GetAuthInfo returns empty auth info
func (a *NoAuthenticator) GetAuthInfo(r *http.Request) map[string]interface{} {
	return map[string]interface{}{
		"auth_type": "none",
	}
}
