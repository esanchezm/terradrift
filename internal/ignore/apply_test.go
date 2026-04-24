package ignore

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/esanchezm/terradrift/internal/core"
	"github.com/esanchezm/terradrift/internal/diff"
)

func baseReport() diff.DriftReport {
	return diff.DriftReport{
		Managed: []core.Resource{
			{ID: "i-m", Type: "aws_instance", Name: "web-managed"},
		},
		Unmanaged: []core.Resource{
			{ID: "i-1", Type: "aws_instance", Name: "web-2"},
			{ID: "b-1", Type: "aws_s3_bucket", Name: "temp-logs"},
			{ID: "b-2", Type: "aws_s3_bucket", Name: "keepers"},
		},
		Missing: []core.Resource{
			{ID: "sg-1", Type: "aws_security_group", Name: "legacy"},
		},
		Drifted: []diff.DriftedResource{
			{
				Resource: core.Resource{ID: "r-1", Type: "aws_iam_role", Name: "admin"},
				Changes:  []diff.Change{{Attribute: "path", OldValue: "/", NewValue: "/team/"}},
			},
			{
				Resource: core.Resource{ID: "i-2", Type: "aws_instance", Name: "web-1"},
				Changes:  []diff.Change{{Attribute: "instance_type", OldValue: "t3.micro", NewValue: "t3.small"}},
			},
		},
		Timestamp: time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC),
	}
}

func TestApply_NilPatterns_ReturnsInputUnchanged(t *testing.T) {
	in := baseReport()
	out := Apply(in, nil)
	if !reflect.DeepEqual(out, in) {
		t.Errorf("Apply(report, nil) must return input unchanged")
	}
}

func TestApply_EmptyPatterns_ReturnsInputUnchanged(t *testing.T) {
	in := baseReport()
	out := Apply(in, &Patterns{})
	if !reflect.DeepEqual(out, in) {
		t.Errorf("Apply(report, empty) must return input unchanged")
	}
}

func TestApply_FiltersUnmanaged_ByExactMatch(t *testing.T) {
	p, err := Parse(strings.NewReader("aws_instance.web-2\n"), "t")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	out := Apply(baseReport(), p)

	if len(out.Unmanaged) != 2 {
		t.Errorf("Unmanaged len = %d, want 2 (web-2 filtered)", len(out.Unmanaged))
	}
	for _, r := range out.Unmanaged {
		if r.Name == "web-2" {
			t.Error("web-2 should have been filtered out of Unmanaged")
		}
	}
	if len(out.Ignored) != 1 {
		t.Fatalf("Ignored len = %d, want 1", len(out.Ignored))
	}
	if out.Ignored[0].Name != "web-2" {
		t.Errorf("Ignored[0].Name = %q, want %q", out.Ignored[0].Name, "web-2")
	}
}

func TestApply_FiltersUnmanaged_ByWildcard(t *testing.T) {
	p, err := Parse(strings.NewReader("aws_s3_bucket.temp-*\n"), "t")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	out := Apply(baseReport(), p)

	if len(out.Unmanaged) != 2 {
		t.Errorf("Unmanaged len = %d, want 2", len(out.Unmanaged))
	}
	for _, r := range out.Unmanaged {
		if strings.HasPrefix(r.Name, "temp-") && r.Type == "aws_s3_bucket" {
			t.Errorf("temp-* bucket %q must be filtered", r.Name)
		}
	}
}

func TestApply_FiltersDrifted_ByMatch(t *testing.T) {
	p, err := Parse(strings.NewReader("aws_iam_role.*\n"), "t")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	out := Apply(baseReport(), p)

	if len(out.Drifted) != 1 {
		t.Fatalf("Drifted len = %d, want 1 (iam_role filtered)", len(out.Drifted))
	}
	if out.Drifted[0].Resource.Type == "aws_iam_role" {
		t.Error("aws_iam_role.* should have filtered the IAM role")
	}
	if len(out.Ignored) != 1 || out.Ignored[0].Type != "aws_iam_role" {
		t.Errorf("expected IAM role moved to Ignored, got %+v", out.Ignored)
	}
}

func TestApply_LeavesManagedAndMissingUntouched(t *testing.T) {
	p, err := Parse(strings.NewReader("aws_security_group.legacy\naws_instance.web-managed\n"), "t")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	out := Apply(baseReport(), p)

	if !reflect.DeepEqual(out.Missing, baseReport().Missing) {
		t.Errorf("Missing must not be touched by Apply; got %+v", out.Missing)
	}
	if !reflect.DeepEqual(out.Managed, baseReport().Managed) {
		t.Errorf("Managed must not be touched by Apply; got %+v", out.Managed)
	}
	if len(out.Ignored) != 0 {
		t.Errorf("patterns matching only Managed/Missing must not populate Ignored, got %+v", out.Ignored)
	}
}

func TestApply_PreservesTimestamp(t *testing.T) {
	in := baseReport()
	p, _ := Parse(strings.NewReader("aws_instance.web-2\n"), "t")

	out := Apply(in, p)

	if !out.Timestamp.Equal(in.Timestamp) {
		t.Errorf("Timestamp = %v, want %v", out.Timestamp, in.Timestamp)
	}
}

func TestApply_NoMatches_LeavesReportEquivalent(t *testing.T) {
	p, _ := Parse(strings.NewReader("aws_vpc.*\n"), "t")

	in := baseReport()
	out := Apply(in, p)

	if len(out.Unmanaged) != len(in.Unmanaged) {
		t.Errorf("Unmanaged must be unchanged when no pattern matches")
	}
	if len(out.Drifted) != len(in.Drifted) {
		t.Errorf("Drifted must be unchanged when no pattern matches")
	}
	if len(out.Ignored) != 0 {
		t.Errorf("Ignored must be empty when no pattern matches")
	}
}

func TestApply_NamelessResource_MatchesByTypeID(t *testing.T) {
	report := diff.DriftReport{
		Unmanaged: []core.Resource{
			{ID: "i-0abc123", Type: "aws_instance", Name: ""},
		},
		Drifted: []diff.DriftedResource{
			{
				Resource: core.Resource{ID: "sg-999", Type: "aws_security_group", Name: ""},
				Changes:  []diff.Change{{Attribute: "description", OldValue: "a", NewValue: "b"}},
			},
		},
	}

	p, err := Parse(strings.NewReader(`aws_instance.i-0abc123
aws_security_group.sg-*
`), "t")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	out := Apply(report, p)

	if len(out.Unmanaged) != 0 {
		t.Errorf("nameless unmanaged resource must be ignored by type.id pattern, got Unmanaged=%+v", out.Unmanaged)
	}
	if len(out.Drifted) != 0 {
		t.Errorf("nameless drifted resource must be ignored by wildcard on id, got Drifted=%+v", out.Drifted)
	}
	if len(out.Ignored) != 2 {
		t.Errorf("Ignored len = %d, want 2", len(out.Ignored))
	}
}

func TestApply_LabelConsistency_UserCopiesFromScreenWorks(t *testing.T) {
	cases := []struct {
		name           string
		resource       core.Resource
		patternLiteral string
	}{
		{
			name:           "resource with Name",
			resource:       core.Resource{ID: "i-1", Type: "aws_instance", Name: "web-2"},
			patternLiteral: "aws_instance.web-2",
		},
		{
			name:           "resource without Name falls back to ID",
			resource:       core.Resource{ID: "i-0abc123", Type: "aws_instance", Name: ""},
			patternLiteral: "aws_instance.i-0abc123",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := diff.DriftReport{Unmanaged: []core.Resource{tc.resource}}
			p, err := Parse(strings.NewReader(tc.patternLiteral+"\n"), "t")
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			out := Apply(report, p)
			if len(out.Unmanaged) != 0 || len(out.Ignored) != 1 {
				t.Errorf("pattern %q must match resource %+v exactly; got Unmanaged=%d, Ignored=%d",
					tc.patternLiteral, tc.resource, len(out.Unmanaged), len(out.Ignored))
			}
		})
	}
}

func TestApply_CombinedPatterns_FilterMultipleCategories(t *testing.T) {
	p, err := Parse(strings.NewReader(`aws_instance.web-2
aws_iam_role.*
`), "t")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	out := Apply(baseReport(), p)

	if len(out.Unmanaged) != 2 {
		t.Errorf("Unmanaged = %d, want 2", len(out.Unmanaged))
	}
	if len(out.Drifted) != 1 {
		t.Errorf("Drifted = %d, want 1", len(out.Drifted))
	}
	if len(out.Ignored) != 2 {
		t.Fatalf("Ignored = %d, want 2", len(out.Ignored))
	}

	gotTypes := map[string]bool{}
	for _, r := range out.Ignored {
		gotTypes[r.Type] = true
	}
	if !gotTypes["aws_instance"] || !gotTypes["aws_iam_role"] {
		t.Errorf("Ignored should contain both aws_instance and aws_iam_role, got %+v", out.Ignored)
	}
}
