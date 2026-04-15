// Package provider defines the interface for cloud provider interactions.
package provider

// Resource represents a cloud resource.
type Resource struct {
	ID         string
	Type       string
	Name       string
	Attributes map[string]interface{}
}

// Provider defines the interface for interacting with cloud providers.
type Provider interface {
	// ListResources returns all resources for the provider.
	ListResources() ([]Resource, error)

	// GetResource returns a specific resource by ID.
	GetResource(id string) (*Resource, error)

	// Name returns the provider name (e.g., "aws", "gcp", "azure").
	Name() string
}
