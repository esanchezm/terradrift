// Package provider defines the interface for cloud provider interactions.
package provider

import (
	"context"

	"github.com/esanchezm/terradrift/internal/core"
)

// Provider defines the interface for interacting with cloud providers.
// It is responsible for discovering and describing cloud resources.
type Provider interface {
	// Name returns the provider name (e.g., "aws", "gcp", "azure").
	Name() string

	// Resources returns the resources of the specified types.
	// If types is empty, all supported resource types are returned.
	Resources(ctx context.Context, types []string) ([]core.Resource, error)

	// SupportedTypes returns the list of resource types this provider can manage.
	SupportedTypes() []string
}
