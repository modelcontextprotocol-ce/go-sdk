// Package util provides utility functions for the MCP SDK.
package util

import "fmt"

// AssertNotNil checks that the provided value is not nil.
// If the value is nil, it panics with the provided message.
func AssertNotNil(val interface{}, message string) {
	if val == nil {
		panic(message)
	}
}

// AssertNotEmpty checks that the provided string is not empty.
// If the string is empty, it panics with the provided message.
func AssertNotEmpty(val string, message string) {
	if val == "" {
		panic(message)
	}
}

// AssertTrue checks that the provided condition is true.
// If the condition is false, it panics with the provided message.
func AssertTrue(condition bool, message string) {
	if !condition {
		panic(message)
	}
}

// AssertInRange checks that the provided value is within the specified range.
// If the value is outside the range, it panics with a message.
func AssertInRange(val int, min int, max int, message string) {
	if val < min || val > max {
		panic(fmt.Sprintf("%s: %d must be between %d and %d", message, val, min, max))
	}
}

// PanicIfError checks if the provided error is not nil.
// If the error is not nil, it panics with the error message.
func PanicIfError(err error) {
	if err != nil {
		panic(err.Error())
	}
}
