// Package core holds the canonical types used throughout the terradrift project.
// By centralizing these types, we prevent circular dependencies between
// provider and state packages.
package core

// Resource represents a canonical cloud resource.
// It serves as the common language between providers (which discover resources)
// and state (which records the observed state of resources).
type Resource struct {
	ID       string
	Type     string
	Name     string
	Provider string
	Region   string
	Data     map[string]interface{}
}
