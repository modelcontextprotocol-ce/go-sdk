// Package util provides utility functions for the MCP SDK.
package util

import (
	"github.com/google/uuid"
)

// GenerateUUID creates a new UUID string.
// This function is used to generate unique identifiers for JSON-RPC requests.
func GenerateUUID() string {
	return uuid.New().String()
}
