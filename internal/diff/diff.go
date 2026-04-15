// Package diff handles comparison between desired and actual state.
package diff

// ChangeType represents the type of drift detected.
type ChangeType string

const (
	// ChangeTypeAdd indicates a resource was added.
	ChangeTypeAdd ChangeType = "add"
	// ChangeTypeRemove indicates a resource was removed.
	ChangeTypeRemove ChangeType = "remove"
	// ChangeTypeUpdate indicates a resource was modified.
	ChangeTypeUpdate ChangeType = "update"
)

// Change represents a detected change between states.
type Change struct {
	ResourceID   string
	ChangeType   ChangeType
	Attribute    string
	OldValue     interface{}
	NewValue     interface{}
	ResourceType string
	ResourceName string
}

// Differ defines the interface for computing differences between states.
type Differ interface {
	// Compare computes the differences between desired and actual state.
	Compare(desired, actual interface{}) ([]Change, error)
}
