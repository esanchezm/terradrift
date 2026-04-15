// Package output handles formatting and outputting drift results.
package output

// DriftResult represents the result of a drift scan.
type DriftResult struct {
	HasDrift bool
	Changes  []ChangeInfo
	Summary  Summary
}

// ChangeInfo contains information about a detected change.
type ChangeInfo struct {
	ResourceID   string
	ResourceType string
	ResourceName string
	ChangeType   string
	Attribute    string
	OldValue     interface{}
	NewValue     interface{}
}

// Summary provides a summary of drift detection.
type Summary struct {
	TotalChanges int
	Added        int
	Removed      int
	Updated      int
}

// Formatter defines the interface for formatting output.
type Formatter interface {
	// Format formats the drift result for output.
	Format(result DriftResult) (string, error)

	// Type returns the formatter type (e.g., "table", "json", "yaml").
	Type() string
}
