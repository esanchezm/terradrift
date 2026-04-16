// Package diff handles comparison between desired and actual state.
package diff

import (
	"reflect"

	"github.com/esanchezm/terradrift/internal/core"
)

// DriftedResource represents a resource that has drifted from its desired state.
type DriftedResource struct {
	Resource core.Resource
	Changes  []Change
}

// DriftReport summarizes the differences between the desired and actual state.
type DriftReport struct {
	// Managed are resources that are present in both desired and actual state, but might have drifted.
	Managed []core.Resource
	// Unmanaged are resources that are present in the actual state but not in the desired state.
	Unmanaged []core.Resource
	// Missing are resources that are present in the desired state but not in the actual state.
	Missing []core.Resource
	// Drifted are resources that are present in both but have differences in their attributes.
	Drifted []DriftedResource
}

// CalculateDrift compares the desired state and the actual state to produce a DriftReport.
func CalculateDrift(desired, actual []core.Resource) DriftReport {
	report := DriftReport{}

	desiredMap := make(map[string]core.Resource)
	for _, r := range desired {
		desiredMap[r.ID] = r
	}

	actualMap := make(map[string]core.Resource)
	for _, r := range actual {
		actualMap[r.ID] = r
	}

	// Check for missing and drifted resources
	for id, dRes := range desiredMap {
		if aRes, ok := actualMap[id]; ok {
			// Resource exists in both. Check for drift.
			changes := compareResources(dRes, aRes)
			if len(changes) > 0 {
				report.Drifted = append(report.Drifted, DriftedResource{
					Resource: dRes,
					Changes:  changes,
				})
			} else {
				report.Managed = append(report.Managed, dRes)
			}
		} else {
			// Missing in actual
			report.Missing = append(report.Missing, dRes)
		}
	}

	// Check for unmanaged resources
	for id, aRes := range actualMap {
		if _, ok := desiredMap[id]; !ok {
			report.Unmanaged = append(report.Unmanaged, aRes)
		}
	}

	return report
}

func compareResources(desired, actual core.Resource) []Change {
	var changes []Change

	// We compare the Data map
	if !reflect.DeepEqual(desired.Data, actual.Data) {
		// For simplicity, we'll just say the whole Data map changed if it's not equal.
		// A more advanced implementation would find specific key differences.
		changes = append(changes, Change{
			ResourceID:   desired.ID,
			ChangeType:   ChangeTypeUpdate,
			Attribute:    "data",
			OldValue:     desired.Data,
			NewValue:     actual.Data,
			ResourceType: desired.Type,
			ResourceName: desired.Name,
		})
	}

	return changes
}
