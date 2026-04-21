// Package local implements a StateReader for local terraform.tfstate files.
package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/esanchezm/terradrift/internal/core"
	"github.com/esanchezm/terradrift/internal/state"
)

// Reader implements state.StateReader for local .tfstate files discovered
// via a filesystem glob pattern.
type Reader struct {
	pattern string
}

// New creates a Reader that reads state from files matching the given glob
// pattern (e.g. "./infra/**/*.tfstate").
func New(pattern string) *Reader {
	return &Reader{pattern: pattern}
}

// Resources returns all managed resources found across every state file that
// matches the configured glob pattern. Data sources (mode "data") are
// excluded. If no files match the pattern, nil is returned.
func (r *Reader) Resources(ctx context.Context) ([]core.Resource, error) {
	matches, err := filepath.Glob(r.pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid glob pattern %q: %w", r.pattern, err)
	}

	if len(matches) == 0 {
		return nil, nil
	}

	var resources []core.Resource
	for _, path := range matches {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		rs, err := parseFile(path)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
		resources = append(resources, rs...)
	}

	return resources, nil
}

// Source returns the glob pattern used to locate state files.
func (r *Reader) Source() string {
	return r.pattern
}

// parseFile reads a single .tfstate file and returns the managed resources it
// contains. Data sources are skipped.
func parseFile(path string) ([]core.Resource, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}
	return state.ParseState(data)
}
