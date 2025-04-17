package util

import (
	"encoding/json"
	"fmt"
)

// ToInterface converts any object to an interface{} through serialization and deserialization.
// This is useful when you need to convert between complex types or ensure deep copying.
// Returns nil if the value is nil or if there's an error during conversion.
func ToInterface(value interface{}, target interface{}) error {
	if value == nil {
		return nil
	}

	// Try to marshal and unmarshal to convert to interface{}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	err = json.Unmarshal(data, &target)
	if err != nil {
		return err
	}

	return nil
}

// ToMap converts any object to a map[string]interface{} through serialization and deserialization.
// Returns nil if the value is nil, not a struct/map, or if there's an error during conversion.
func ToMap(value interface{}) map[string]interface{} {
	if value == nil {
		return nil
	}

	// Convert directly if it's already a map[string]interface{}
	if m, ok := value.(map[string]interface{}); ok {
		return m
	}

	// Try to marshal and unmarshal to convert to map
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	if err != nil {
		return nil
	}

	return result
}

// ToSlice converts any object to a []interface{} through serialization and deserialization.
// Returns nil if the value is nil, not a slice/array, or if there's an error during conversion.
func ToSlice(value interface{}) []interface{} {
	if value == nil {
		return nil
	}

	// Convert directly if it's already a []interface{}
	if s, ok := value.([]interface{}); ok {
		return s
	}

	// Try to marshal and unmarshal to convert to slice
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}

	var result []interface{}
	err = json.Unmarshal(data, &result)
	if err != nil {
		return nil
	}

	return result
}

// MustToInterface is like ToInterface but panics if conversion fails.
// Use this when you're confident the conversion will succeed.
func MustToInterface(value interface{}) interface{} {
	if value == nil {
		return nil
	}

	data, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal value: %v", err))
	}

	var result interface{}
	err = json.Unmarshal(data, &result)
	if err != nil {
		panic(fmt.Sprintf("failed to unmarshal value: %v", err))
	}

	return result
}
