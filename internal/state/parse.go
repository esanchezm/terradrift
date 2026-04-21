// Package state handles loading and parsing infrastructure state.
package state

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/esanchezm/terradrift/internal/core"
)

// MaxStateSize is the maximum allowed size of a Terraform state file in bytes
// (256 MB). Readers should wrap their input with io.LimitReader(r, MaxStateSize)
// to prevent memory exhaustion from hostile or pathological inputs.
const MaxStateSize = 256 * 1024 * 1024

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

// ParseState decodes raw Terraform v4 state JSON bytes and returns the managed
// resources it contains. Data sources (mode "data") are excluded. An empty
// resources array returns an empty (non-nil) slice with no error.
func ParseState(data []byte) ([]core.Resource, error) {
	var s tfState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("decoding state JSON: %w", err)
	}

	resources := make([]core.Resource, 0)
	for _, res := range s.Resources {
		if res.Mode != "managed" {
			continue
		}

		provider := ExtractProvider(res.Provider)

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

// ExtractProvider extracts the short provider name from a Terraform provider
// string. For example:
//
//	`provider["registry.terraform.io/hashicorp/aws"]` -> "aws"
func ExtractProvider(raw string) string {
	raw = strings.TrimPrefix(raw, `provider["`)
	raw = strings.TrimSuffix(raw, `"]`)

	if idx := strings.LastIndex(raw, "/"); idx != -1 {
		return raw[idx+1:]
	}

	return raw
}
