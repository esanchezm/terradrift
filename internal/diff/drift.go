// Package diff handles comparison between desired and actual state.
package diff

import (
	"sort"
	"time"

	"github.com/esanchezm/terradrift/internal/core"
)

// DriftedResource represents a resource present in both desired and actual
// state but whose attributes differ. Changes contains one Change per
// attribute that diverges between the two sides.
type DriftedResource struct {
	Resource core.Resource
	Changes  []Change
}

// DriftReport summarizes the differences between the desired and actual
// state of a set of resources.
type DriftReport struct {
	// Managed are resources present in both desired and actual state with
	// no attribute drift detected.
	Managed []core.Resource
	// Unmanaged are resources present in the actual (cloud) state but not
	// in the desired (Terraform) state.
	Unmanaged []core.Resource
	// Missing are resources present in the desired state but not in the
	// actual state (deleted, never created, or inaccessible to the provider
	// scan).
	Missing []core.Resource
	// Drifted are resources present in both sides but with one or more
	// attribute differences listed in DriftedResource.Changes.
	Drifted []DriftedResource
	// Timestamp records when the report was produced. The monotonic clock
	// reading is stripped (via Round(0)) so the timestamp survives JSON
	// and other serialization round-trips.
	Timestamp time.Time
}

// CalculateDrift compares desired against actual state using DefaultOptions
// and returns a DriftReport.
//
// This is a thin wrapper around CalculateDriftWithOptions; callers who need
// to tune matching, normalization, or ephemeral-field filtering should use
// the options-accepting variant directly.
func CalculateDrift(desired, actual []core.Resource) DriftReport {
	return CalculateDriftWithOptions(desired, actual, DefaultOptions())
}

// CalculateDriftWithOptions compares desired and actual resources using the
// supplied options and returns a DriftReport.
//
// Algorithm:
//
//  1. matchResources pairs resources using a three-pass strategy (by ID,
//     then Name, then tags["Name"]). Unpaired desired resources are
//     classified as Missing; unpaired actual resources as Unmanaged.
//  2. For each matched pair, diffResources produces attribute-level changes
//     under an intersection-only key policy with user-configurable
//     normalization. A pair is Managed when no changes remain and Drifted
//     otherwise.
//
// All report slices (Managed, Unmanaged, Missing, Drifted) are sorted by
// resource ID so the output is byte-identical across runs for identical
// input.
func CalculateDriftWithOptions(desired, actual []core.Resource, opts Options) DriftReport {
	report := DriftReport{Timestamp: opts.now()}

	pairs, unmatchedDesired, unmatchedActual := matchResources(desired, actual)

	for _, p := range pairs {
		changes := diffResources(p.Desired, p.Actual, opts)
		if len(changes) == 0 {
			report.Managed = append(report.Managed, p.Desired)
			continue
		}
		report.Drifted = append(report.Drifted, DriftedResource{
			Resource: p.Desired,
			Changes:  changes,
		})
	}
	report.Missing = unmatchedDesired
	report.Unmanaged = unmatchedActual

	sortResourcesByID(report.Managed)
	sortResourcesByID(report.Unmanaged)
	sortResourcesByID(report.Missing)
	sort.Slice(report.Drifted, func(i, j int) bool {
		return report.Drifted[i].Resource.ID < report.Drifted[j].Resource.ID
	})

	return report
}

// sortResourcesByID sorts rs in place by ascending Resource.ID.
func sortResourcesByID(rs []core.Resource) {
	sort.Slice(rs, func(i, j int) bool {
		return rs[i].ID < rs[j].ID
	})
}
