// Package state handles loading and parsing infrastructure state.
package state

import (
	"context"

	"github.com/esanchezm/terradrift/internal/core"
)

// StateReader defines the interface for reading infrastructure state.
// Implementations are responsible for fetching the current state of resources
// from a storage backend or file.
type StateReader interface {
	// Resources returns the resources currently present in the state.
	Resources(ctx context.Context) ([]core.Resource, error)

	// Source returns the source of the state (e.g., a file path or URL).
	Source() string
}
