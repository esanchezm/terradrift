// Package config handles the application configuration.
package config

// Config holds the application settings.
type Config struct {
	// ProviderCredentials maps provider names to their credentials (e.g., API keys).
	ProviderCredentials map[string]string
	// StatePaths maps state types to their file paths or URIs.
	StatePaths map[string]string
}

// NewDefaultConfig returns a default configuration.
func NewDefaultConfig() *Config {
	return &Config{
		ProviderCredentials: make(map[string]string),
		StatePaths:          make(map[string]string),
	}
}
