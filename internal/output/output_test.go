package output

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/esanchezm/terradrift/internal/core"
	"github.com/esanchezm/terradrift/internal/diff"
)

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func sampleReport() *diff.DriftReport {
	return &diff.DriftReport{
		Managed: []core.Resource{
			{ID: "i-managed-0", Type: "aws_instance", Name: "web-0"},
			{ID: "i-managed-1", Type: "aws_instance", Name: "web-3"},
			{ID: "i-managed-2", Type: "aws_instance", Name: "web-4"},
			{ID: "i-managed-3", Type: "aws_instance", Name: "web-5"},
			{ID: "i-managed-4", Type: "aws_instance", Name: "web-6"},
			{ID: "i-managed-5", Type: "aws_instance", Name: "web-7"},
			{ID: "i-managed-6", Type: "aws_instance", Name: "web-8"},
			{ID: "i-managed-7", Type: "aws_instance", Name: "web-9"},
			{ID: "i-managed-8", Type: "aws_instance", Name: "web-10"},
			{ID: "i-managed-9", Type: "aws_instance", Name: "web-11"},
			{ID: "i-managed-10", Type: "aws_instance", Name: "web-12"},
			{ID: "i-managed-11", Type: "aws_instance", Name: "web-13"},
			{ID: "i-managed-12", Type: "aws_instance", Name: "web-14"},
			{ID: "i-managed-13", Type: "aws_instance", Name: "web-15"},
			{ID: "i-managed-14", Type: "aws_instance", Name: "web-16"},
			{ID: "i-managed-15", Type: "aws_instance", Name: "web-17"},
			{ID: "i-managed-16", Type: "aws_instance", Name: "web-18"},
			{ID: "i-managed-17", Type: "aws_instance", Name: "web-19"},
		},
		Unmanaged: []core.Resource{
			{ID: "i-0abc123", Type: "aws_instance", Name: "web-2"},
			{ID: "temp-logs", Type: "aws_s3_bucket", Name: "temp-logs"},
		},
		Missing: []core.Resource{
			{ID: "sg-0def456", Type: "aws_security_group", Name: "legacy-rules"},
		},
		Drifted: []diff.DriftedResource{
			{
				Resource: core.Resource{ID: "i-0xyz789", Type: "aws_instance", Name: "web-1"},
				Changes: []diff.Change{
					{Attribute: "instance_type", OldValue: "t3.medium", NewValue: "t3.large"},
					{Attribute: "tags.Environment", OldValue: "production", NewValue: "staging"},
				},
			},
		},
		Timestamp: time.Date(2026, 4, 23, 0, 0, 0, 0, time.UTC),
	}
}

func sampleInfo() ScanInfo {
	return ScanInfo{
		Provider:           "aws",
		Region:             "eu-west-1",
		StateSource:        "terraform.tfstate",
		StateResourceCount: 23,
	}
}

func TestRender_GoldenTicketExample(t *testing.T) {
	var buf bytes.Buffer
	r := New(Options{Writer: &buf, NoColor: true})

	if err := r.Render(sampleInfo(), sampleReport()); err != nil {
		t.Fatalf("Render returned unexpected error: %v", err)
	}

	want := `Scanning AWS resources...
State source: terraform.tfstate (23 resources)
Provider: aws (region: eu-west-1)

~~ Drift detected ~~

  + aws_instance.web-2 (i-0abc123) — unmanaged
  + aws_s3_bucket.temp-logs — unmanaged
  - aws_security_group.legacy-rules (sg-0def456) — missing from cloud
  ~ aws_instance.web-1 (i-0xyz789)
      instance_type: "t3.medium" → "t3.large"
      tags.Environment: "production" → "staging"

Summary: 18 managed, 2 unmanaged, 1 missing, 1 drifted
`

	if got := buf.String(); got != want {
		t.Errorf("output mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRender_NoDrift_PrintsCleanMessage(t *testing.T) {
	var buf bytes.Buffer
	r := New(Options{Writer: &buf, NoColor: true})

	report := &diff.DriftReport{
		Managed: []core.Resource{
			{ID: "i-1", Type: "aws_instance", Name: "web"},
		},
	}

	if err := r.Render(sampleInfo(), report); err != nil {
		t.Fatalf("Render: %v", err)
	}

	got := buf.String()

	if !strings.Contains(got, "No drift detected.") {
		t.Errorf("expected 'No drift detected.' message, got:\n%s", got)
	}
	if strings.Contains(got, "~~ Drift detected ~~") {
		t.Errorf("unexpected drift banner in clean report:\n%s", got)
	}
	if !strings.Contains(got, "Summary: 1 managed, 0 unmanaged, 0 missing, 0 drifted") {
		t.Errorf("expected summary line, got:\n%s", got)
	}
}

func TestRender_Quiet_OnlySummaryLine(t *testing.T) {
	var buf bytes.Buffer
	r := New(Options{Writer: &buf, NoColor: true, Quiet: true})

	if err := r.Render(sampleInfo(), sampleReport()); err != nil {
		t.Fatalf("Render: %v", err)
	}

	got := buf.String()
	want := "Summary: 18 managed, 2 unmanaged, 1 missing, 1 drifted\n"

	if got != want {
		t.Errorf("quiet output mismatch\ngot:  %q\nwant: %q", got, want)
	}
}

func TestRender_Quiet_StripsHeader(t *testing.T) {
	var buf bytes.Buffer
	r := New(Options{Writer: &buf, NoColor: true, Quiet: true})

	if err := r.Render(sampleInfo(), sampleReport()); err != nil {
		t.Fatalf("Render: %v", err)
	}

	got := buf.String()

	forbidden := []string{
		"Scanning",
		"State source:",
		"Provider:",
		"~~ Drift detected ~~",
		"— unmanaged",
		"missing from cloud",
		"instance_type",
	}
	for _, needle := range forbidden {
		if strings.Contains(got, needle) {
			t.Errorf("quiet mode leaked %q in output:\n%s", needle, got)
		}
	}
}

func TestRender_NoColor_StripsAllANSI(t *testing.T) {
	var buf bytes.Buffer
	r := New(Options{Writer: &buf, NoColor: true})

	if err := r.Render(sampleInfo(), sampleReport()); err != nil {
		t.Fatalf("Render: %v", err)
	}

	got := buf.String()
	if ansiEscape.MatchString(got) {
		t.Errorf("NoColor=true output contained ANSI escape sequences:\n%q", got)
	}
}

func TestRender_EmitsANSIWhenColorProfileSupports(t *testing.T) {
	var buf bytes.Buffer
	r := New(Options{Writer: &buf, NoColor: false})

	r.lr.SetColorProfile(termenv.ANSI256)
	r.added = r.lr.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	r.removed = r.lr.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	r.drifted = r.lr.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
	r.bold = r.lr.NewStyle().Bold(true)
	r.arrow = r.lr.NewStyle().Faint(true)

	if err := r.Render(sampleInfo(), sampleReport()); err != nil {
		t.Fatalf("Render: %v", err)
	}

	got := buf.String()
	if !ansiEscape.MatchString(got) {
		t.Errorf("expected ANSI escape sequences with color profile ANSI256, got no ANSI in:\n%s", got)
	}

	stripped := ansiEscape.ReplaceAllString(got, "")
	if !strings.Contains(stripped, "+ aws_instance.web-2 (i-0abc123) — unmanaged") {
		t.Errorf("ANSI-stripped output missing expected line:\n%s", stripped)
	}
}

func TestRender_NilReport_ReturnsError(t *testing.T) {
	var buf bytes.Buffer
	r := New(Options{Writer: &buf, NoColor: true})

	err := r.Render(sampleInfo(), nil)
	if err == nil {
		t.Fatal("expected error for nil report, got nil")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Errorf("error should mention nil report, got: %v", err)
	}
}

func TestRender_EmptyReport_AllZeroSummary(t *testing.T) {
	var buf bytes.Buffer
	r := New(Options{Writer: &buf, NoColor: true})

	if err := r.Render(sampleInfo(), &diff.DriftReport{}); err != nil {
		t.Fatalf("Render: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "Summary: 0 managed, 0 unmanaged, 0 missing, 0 drifted") {
		t.Errorf("expected zero-count summary, got:\n%s", got)
	}
	if !strings.Contains(got, "No drift detected.") {
		t.Errorf("empty report should report no drift, got:\n%s", got)
	}
}

func TestRender_Deterministic(t *testing.T) {
	var first, second bytes.Buffer

	r1 := New(Options{Writer: &first, NoColor: true})
	if err := r1.Render(sampleInfo(), sampleReport()); err != nil {
		t.Fatalf("render 1: %v", err)
	}

	r2 := New(Options{Writer: &second, NoColor: true})
	if err := r2.Render(sampleInfo(), sampleReport()); err != nil {
		t.Fatalf("render 2: %v", err)
	}

	if first.String() != second.String() {
		t.Errorf("Render is not deterministic:\nfirst:\n%s\nsecond:\n%s", first.String(), second.String())
	}
}

func TestRender_DriftedNonStringValues_RenderWithPercentV(t *testing.T) {
	var buf bytes.Buffer
	r := New(Options{Writer: &buf, NoColor: true})

	report := &diff.DriftReport{
		Drifted: []diff.DriftedResource{
			{
				Resource: core.Resource{ID: "i-1", Type: "aws_instance", Name: "web"},
				Changes: []diff.Change{
					{Attribute: "cpu_count", OldValue: 2, NewValue: 4},
					{Attribute: "monitoring", OldValue: false, NewValue: true},
					{Attribute: "ratio", OldValue: 1.5, NewValue: 2.75},
				},
			},
		},
	}

	if err := r.Render(sampleInfo(), report); err != nil {
		t.Fatalf("Render: %v", err)
	}

	got := buf.String()

	cases := []string{
		"cpu_count: 2 → 4",
		"monitoring: false → true",
		"ratio: 1.5 → 2.75",
	}
	for _, want := range cases {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in output:\n%s", want, got)
		}
	}
}

func TestRender_NilAttributeValue_RendersAsPlaceholder(t *testing.T) {
	var buf bytes.Buffer
	r := New(Options{Writer: &buf, NoColor: true})

	report := &diff.DriftReport{
		Drifted: []diff.DriftedResource{
			{
				Resource: core.Resource{ID: "i-1", Type: "aws_instance", Name: "web"},
				Changes: []diff.Change{
					{Attribute: "optional", OldValue: nil, NewValue: "now-set"},
				},
			},
		},
	}

	if err := r.Render(sampleInfo(), report); err != nil {
		t.Fatalf("Render: %v", err)
	}

	if !strings.Contains(buf.String(), `optional: <nil> → "now-set"`) {
		t.Errorf("expected nil placeholder in output:\n%s", buf.String())
	}
}

func TestResourceLabel(t *testing.T) {
	cases := []struct {
		name string
		res  core.Resource
		want string
	}{
		{
			name: "distinct ID and Name yields type.name (id)",
			res:  core.Resource{Type: "aws_instance", Name: "web-1", ID: "i-0xyz789"},
			want: "aws_instance.web-1 (i-0xyz789)",
		},
		{
			name: "ID equals Name collapses to type.name",
			res:  core.Resource{Type: "aws_s3_bucket", Name: "temp-logs", ID: "temp-logs"},
			want: "aws_s3_bucket.temp-logs",
		},
		{
			name: "empty ID collapses to type.name",
			res:  core.Resource{Type: "aws_iam_role", Name: "admin", ID: ""},
			want: "aws_iam_role.admin",
		},
		{
			name: "empty Name falls back to type.id",
			res:  core.Resource{Type: "aws_instance", Name: "", ID: "i-auto"},
			want: "aws_instance.i-auto",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resourceLabel(tc.res); got != tc.want {
				t.Errorf("resourceLabel(%+v) = %q, want %q", tc.res, got, tc.want)
			}
		})
	}
}

func TestFormatValue(t *testing.T) {
	cases := []struct {
		name string
		in   interface{}
		want string
	}{
		{"nil", nil, "<nil>"},
		{"empty string", "", `""`},
		{"plain string", "hello", `"hello"`},
		{"string with special chars", "a\tb", `"a\tb"`},
		{"int", 42, "42"},
		{"float", 3.14, "3.14"},
		{"bool", true, "true"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatValue(tc.in); got != tc.want {
				t.Errorf("formatValue(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNew_NilWriter_DefaultsToStdout(t *testing.T) {
	r := New(Options{Writer: nil, NoColor: true})
	if r.writer == nil {
		t.Error("Renderer.writer must not be nil when Options.Writer is nil")
	}
}

func TestRender_EmptyRegion_OmitsRegionSuffix(t *testing.T) {
	var buf bytes.Buffer
	r := New(Options{Writer: &buf, NoColor: true})

	info := sampleInfo()
	info.Region = ""

	if err := r.Render(info, &diff.DriftReport{}); err != nil {
		t.Fatalf("Render: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "Provider: aws\n") {
		t.Errorf("expected 'Provider: aws' without region suffix, got:\n%s", got)
	}
	if strings.Contains(got, "(region: )") {
		t.Errorf("expected no empty region suffix, got:\n%s", got)
	}
}

func TestRender_NonEmptyRegion_IncludesRegionSuffix(t *testing.T) {
	var buf bytes.Buffer
	r := New(Options{Writer: &buf, NoColor: true})

	if err := r.Render(sampleInfo(), &diff.DriftReport{}); err != nil {
		t.Fatalf("Render: %v", err)
	}

	if !strings.Contains(buf.String(), "Provider: aws (region: eu-west-1)") {
		t.Errorf("expected region suffix present, got:\n%s", buf.String())
	}
}


