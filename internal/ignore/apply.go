package ignore

import (
	"fmt"

	"github.com/esanchezm/terradrift/internal/core"
	"github.com/esanchezm/terradrift/internal/diff"
)

// Apply returns a copy of report with every Unmanaged or Drifted entry
// whose "type.name" identity matches p moved into the Ignored slice.
//
// Managed and Missing are deliberately passed through untouched:
// driftignore's scope per THI-104 is "resources you knowingly manage
// outside Terraform," which cleanly maps to unmanaged (extra in cloud)
// and drifted (diverged from state). Missing (present in state, absent
// from cloud) is a different failure mode and falls outside this scope.
//
// Apply is a no-op when p is nil or empty: the input report is returned
// unchanged and no allocations are made beyond what the caller already
// owns. This lets callers unconditionally invoke Apply regardless of
// whether a driftignore file was discovered.
func Apply(report diff.DriftReport, p *Patterns) diff.DriftReport {
	if p.IsEmpty() {
		return report
	}

	out := diff.DriftReport{
		Managed:   report.Managed,
		Missing:   report.Missing,
		Timestamp: report.Timestamp,
	}

	for _, r := range report.Unmanaged {
		if p.Match(identity(r)) {
			out.Ignored = append(out.Ignored, r)
			continue
		}
		out.Unmanaged = append(out.Unmanaged, r)
	}

	for _, d := range report.Drifted {
		if p.Match(identity(d.Resource)) {
			out.Ignored = append(out.Ignored, d.Resource)
			continue
		}
		out.Drifted = append(out.Drifted, d)
	}

	return out
}

// identity builds the string a pattern is matched against. The format
// mirrors what the renderer prints as the primary resource label
// (internal/output.resourceLabel): "type.name" when Name is present,
// "type.id" when Name is empty but ID is set. Keeping these two in sync
// is critical — otherwise users would write patterns matching the label
// they see on screen and find them silently ignored for nameless
// resources (for example EC2 instances without a Name tag).
func identity(r core.Resource) string {
	if r.Name != "" {
		return fmt.Sprintf("%s.%s", r.Type, r.Name)
	}
	return fmt.Sprintf("%s.%s", r.Type, r.ID)
}
