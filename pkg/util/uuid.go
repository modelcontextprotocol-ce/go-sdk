// Package util provides utility functions for the MCP SDK.
package util

import (
	"encoding/base64"
	"strings"

	"github.com/google/uuid"
)

// GenerateUUID creates a new UUID string.
// This function is used to generate unique identifiers for JSON-RPC requests.
func GenerateUUID() string {
	return uuid.New().String()
}

// GenerateShortUUID creates a shorter UUID string by encoding a new UUID in base64 and trimming special characters.
// This function is useful when a compact identifier is required.
func GenerateShortUUID() string {
	// Generate a new UUID and get its raw bytes
	id := uuid.New()
	bytes, _ := id.MarshalBinary()

	// Encode the bytes in base64
	encoded := base64.StdEncoding.EncodeToString(bytes)

	// Remove padding characters and other non-alphanumeric characters
	encoded = strings.ReplaceAll(encoded, "=", "")
	encoded = strings.ReplaceAll(encoded, "/", "")
	encoded = strings.ReplaceAll(encoded, "+", "")

	// Return the first 12 characters for a reasonably short ID that's still unique
	if len(encoded) > 12 {
		return encoded[:12]
	}
	return encoded
}
