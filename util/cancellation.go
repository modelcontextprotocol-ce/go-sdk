package util

import (
	"context"
	"sync"
)

// CancellationManager manages cancellation for operations
type CancellationManager struct {
	operations      map[string]context.CancelFunc
	operationsMutex sync.RWMutex
}

// NewCancellationManager creates a new cancellation manager
func NewCancellationManager() *CancellationManager {
	return &CancellationManager{
		operations: make(map[string]context.CancelFunc),
	}
}

// RegisterOperation registers a new operation with a cancel function
func (m *CancellationManager) RegisterOperation(operationID string, cancel context.CancelFunc) {
	m.operationsMutex.Lock()
	defer m.operationsMutex.Unlock()

	// If there's already an operation with this ID, cancel it first
	if existing, exists := m.operations[operationID]; exists {
		existing()
	}

	m.operations[operationID] = cancel
}

// CancelOperation cancels a specific operation
func (m *CancellationManager) CancelOperation(operationID string) bool {
	m.operationsMutex.Lock()
	defer m.operationsMutex.Unlock()

	if cancel, exists := m.operations[operationID]; exists {
		cancel()
		delete(m.operations, operationID)
		return true
	}

	return false
}

// Complete marks an operation as completed and removes it from tracking
func (m *CancellationManager) Complete(operationID string) {
	m.operationsMutex.Lock()
	defer m.operationsMutex.Unlock()

	delete(m.operations, operationID)
}

// CancelAll cancels all tracked operations
func (m *CancellationManager) CancelAll() {
	m.operationsMutex.Lock()
	defer m.operationsMutex.Unlock()

	for _, cancel := range m.operations {
		cancel()
	}

	m.operations = make(map[string]context.CancelFunc)
}

// GetOperationIDs returns a slice of all tracked operation IDs
func (m *CancellationManager) GetOperationIDs() []string {
	m.operationsMutex.RLock()
	defer m.operationsMutex.RUnlock()

	ids := make([]string, 0, len(m.operations))
	for id := range m.operations {
		ids = append(ids, id)
	}

	return ids
}
