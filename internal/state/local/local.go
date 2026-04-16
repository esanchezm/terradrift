// Package local implements a StateReader for local terraform.tfstate files.
package local

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/esanchezm/terradrift/internal/core"
)

// tfState represents the top-level structure of a Terraform v4 state file.
type tfState struct {
	Version   int          `json:"version"`
	Resources []tfResource `json:"resources"`
}

// tfResource represents a single resource block in the state.
type tfResource struct {
	Mode      string       `json:"mode"`
	Type      string       `json:"type"`
	Name      string       `json:"name"`
	Provider  string       `json:"provider"`
	Instances []tfInstance `json:"instances"`
}

// tfInstance represents one instance of a resource (there can be multiple
// when count or for_each is used).
type tfInstance struct {
	Attributes map[string]interface{} `json:"attributes"`
}

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
// excluded. If no files match the pattern, an empty slice is returned.
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

	var s tfState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("decoding JSON: %w", err)
	}

	var resources []core.Resource
	for _, res := range s.Resources {
		if res.Mode != "managed" {
			continue
		}

		provider := extractProvider(res.Provider)

		for _, inst := range res.Instances {
			id, _ := inst.Attributes["id"].(string)

			resources = append(resources, core.Resource{
				ID:       id,
				Type:     res.Type,
				Name:     res.Name,
				Provider: provider,
				Data:     inst.Attributes,
			})
		}
	}

	return resources, nil
}

// extractProvider extracts the short provider name from a Terraform provider
// string. For example:
//
//	"provider[\"registry.terraform.io/hashicorp/aws\"]" -> "aws"
func extractProvider(raw string) string {
	raw = strings.TrimPrefix(raw, `provider["`)
	raw = strings.TrimSuffix(raw, `"]`)

	if idx := strings.LastIndex(raw, "/"); idx != -1 {
		return raw[idx+1:]
	}

	return raw
}
