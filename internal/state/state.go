// Package state handles loading and parsing infrastructure state.
package state

// ResourceState represents the state of an infrastructure resource.
type ResourceState struct {
	ID         string
	Type       string
	Name       string
	Attributes map[string]interface{}
}

// StateLoader defines the interface for loading infrastructure state.
type StateLoader interface {
	// Load loads the state from a given path or configuration.
	Load(path string) ([]ResourceState, error)

	// Type returns the type of state loader (e.g., "terraform", "pulumi").
	Type() string
}
