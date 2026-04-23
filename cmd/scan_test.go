package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/esanchezm/terradrift/internal/core"
)

type fakeStateReader struct {
	resources []core.Resource
	err       error
	source    string
}

func (f *fakeStateReader) Resources(_ context.Context) ([]core.Resource, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.resources, nil
}

func (f *fakeStateReader) Source() string { return f.source }

type fakeProvider struct {
	name      string
	types     []string
	resources []core.Resource
	err       error
	lastTypes []string
}

func (f *fakeProvider) Name() string             { return f.name }
func (f *fakeProvider) SupportedTypes() []string { return f.types }

func (f *fakeProvider) Resources(_ context.Context, types []string) ([]core.Resource, error) {
	f.lastTypes = append([]string(nil), types...)
	if f.err != nil {
		return nil, f.err
	}
	if len(types) == 0 {
		return f.resources, nil
	}
	filtered := make([]core.Resource, 0, len(f.resources))
	wanted := make(map[string]struct{}, len(types))
	for _, t := range types {
		wanted[t] = struct{}{}
	}
	for _, r := range f.resources {
		if _, ok := wanted[r.Type]; ok {
			filtered = append(filtered, r)
		}
	}
	return filtered, nil
}

func TestRunScan_FullReport_MatchesTicketStyle(t *testing.T) {
	reader := &fakeStateReader{
		resources: []core.Resource{
			{ID: "i-managed", Type: "aws_instance", Name: "web-managed"},
			{ID: "sg-0def456", Type: "aws_security_group", Name: "legacy-rules"},
			{ID: "i-0xyz789", Type: "aws_instance", Name: "web-1", Data: map[string]interface{}{"instance_type": "t3.medium"}},
		},
		source: "terraform.tfstate",
	}
	prov := &fakeProvider{
		name:  "aws",
		types: []string{"aws_instance", "aws_s3_bucket", "aws_security_group"},
		resources: []core.Resource{
			{ID: "i-managed", Type: "aws_instance", Name: "web-managed"},
			{ID: "i-0abc123", Type: "aws_instance", Name: "web-2"},
			{ID: "i-0xyz789", Type: "aws_instance", Name: "web-1", Data: map[string]interface{}{"instance_type": "t3.large"}},
		},
	}

	var buf bytes.Buffer
	err := runScan(context.Background(), scanConfig{
		Reader:       reader,
		Provider:     prov,
		ProviderName: "aws",
		Region:       "eu-west-1",
		NoColor:      true,
		Out:          &buf,
	})
	if err != nil {
		t.Fatalf("runScan: %v", err)
	}

	out := buf.String()

	checks := []string{
		"Scanning AWS resources...",
		"State source: terraform.tfstate (3 resources)",
		"Provider: aws (region: eu-west-1)",
		"~~ Drift detected ~~",
		"+ aws_instance.web-2 (i-0abc123) — unmanaged",
		"- aws_security_group.legacy-rules (sg-0def456) — missing from cloud",
		"~ aws_instance.web-1 (i-0xyz789)",
		`instance_type: "t3.medium" → "t3.large"`,
		"Summary: 1 managed, 1 unmanaged, 1 missing, 1 drifted",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("missing expected fragment %q in output:\n%s", want, out)
		}
	}
}

func TestRunScan_TypeFilter_AppliesToBothSides(t *testing.T) {
	reader := &fakeStateReader{
		resources: []core.Resource{
			{ID: "i-1", Type: "aws_instance", Name: "web"},
			{ID: "sg-1", Type: "aws_security_group", Name: "legacy"},
			{ID: "bkt-1", Type: "aws_s3_bucket", Name: "assets"},
		},
		source: "state.tfstate",
	}
	prov := &fakeProvider{
		name:  "aws",
		types: []string{"aws_instance", "aws_s3_bucket", "aws_security_group"},
		resources: []core.Resource{
			{ID: "i-1", Type: "aws_instance", Name: "web"},
			{ID: "sg-1", Type: "aws_security_group", Name: "legacy"},
			{ID: "bkt-1", Type: "aws_s3_bucket", Name: "assets"},
		},
	}

	var buf bytes.Buffer
	err := runScan(context.Background(), scanConfig{
		Reader:       reader,
		Provider:     prov,
		TypeFilter:   "aws_instance",
		ProviderName: "aws",
		Region:       "eu-west-1",
		NoColor:      true,
		Quiet:        true,
		Out:          &buf,
	})
	if err != nil {
		t.Fatalf("runScan: %v", err)
	}

	got := buf.String()
	want := "Summary: 1 managed, 0 unmanaged, 0 missing, 0 drifted\n"
	if got != want {
		t.Errorf("type-filtered summary mismatch:\n got:  %q\n want: %q", got, want)
	}

	if len(prov.lastTypes) != 1 || prov.lastTypes[0] != "aws_instance" {
		t.Errorf("provider query types = %v, want [aws_instance]", prov.lastTypes)
	}
}

func TestRunScan_TypeFilter_UnsupportedType_ReturnsError(t *testing.T) {
	reader := &fakeStateReader{
		resources: []core.Resource{{ID: "i-1", Type: "aws_instance", Name: "web"}},
	}
	prov := &fakeProvider{
		name:  "aws",
		types: []string{"aws_instance", "aws_s3_bucket"},
	}

	var buf bytes.Buffer
	err := runScan(context.Background(), scanConfig{
		Reader:       reader,
		Provider:     prov,
		TypeFilter:   "aws_vpc",
		ProviderName: "aws",
		NoColor:      true,
		Out:          &buf,
	})
	if err == nil {
		t.Fatal("expected error for unsupported type, got nil")
	}
	if !strings.Contains(err.Error(), "aws_vpc") {
		t.Errorf("error should mention unsupported type, got: %v", err)
	}
	if !strings.Contains(err.Error(), "aws_instance") {
		t.Errorf("error should list supported types, got: %v", err)
	}
	if prov.lastTypes != nil {
		t.Errorf("provider must not be queried for unsupported type, got types=%v", prov.lastTypes)
	}
	if buf.Len() != 0 {
		t.Errorf("no output must be written on validation failure, got:\n%s", buf.String())
	}
}

func TestRunScan_StateReaderError_Propagates(t *testing.T) {
	reader := &fakeStateReader{err: errors.New("permission denied")}
	prov := &fakeProvider{name: "aws", types: []string{"aws_instance"}}

	err := runScan(context.Background(), scanConfig{
		Reader:       reader,
		Provider:     prov,
		ProviderName: "aws",
		Out:          &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected error from reader to propagate")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("wrapped error should preserve original message, got: %v", err)
	}
}

func TestRunScan_ProviderError_Propagates(t *testing.T) {
	reader := &fakeStateReader{resources: nil}
	prov := &fakeProvider{name: "aws", types: []string{"aws_instance"}, err: errors.New("auth failed")}

	err := runScan(context.Background(), scanConfig{
		Reader:       reader,
		Provider:     prov,
		ProviderName: "aws",
		Out:          &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected error from provider to propagate")
	}
	if !strings.Contains(err.Error(), "auth failed") {
		t.Errorf("wrapped error should preserve original message, got: %v", err)
	}
}

func TestRunScan_NoColor_StripsANSI(t *testing.T) {
	reader := &fakeStateReader{resources: []core.Resource{{ID: "i-1", Type: "aws_instance", Name: "web"}}, source: "s.tfstate"}
	prov := &fakeProvider{
		name:      "aws",
		types:     []string{"aws_instance"},
		resources: []core.Resource{{ID: "i-2", Type: "aws_instance", Name: "other"}},
	}

	var buf bytes.Buffer
	if err := runScan(context.Background(), scanConfig{
		Reader:       reader,
		Provider:     prov,
		ProviderName: "aws",
		NoColor:      true,
		Out:          &buf,
	}); err != nil {
		t.Fatalf("runScan: %v", err)
	}

	if strings.Contains(buf.String(), "\x1b[") {
		t.Errorf("NoColor=true output must not contain ANSI escapes, got:\n%q", buf.String())
	}
}

func TestProviderSupportsType(t *testing.T) {
	p := &fakeProvider{name: "aws", types: []string{"aws_instance", "aws_s3_bucket"}}

	if !providerSupportsType(p, "aws_instance") {
		t.Error("providerSupportsType(aws_instance) = false, want true")
	}
	if providerSupportsType(p, "aws_vpc") {
		t.Error("providerSupportsType(aws_vpc) = true, want false")
	}
	if providerSupportsType(p, "") {
		t.Error("providerSupportsType(empty) = true, want false")
	}
}

func TestFilterResourcesByType(t *testing.T) {
	rs := []core.Resource{
		{ID: "i-1", Type: "aws_instance"},
		{ID: "sg-1", Type: "aws_security_group"},
		{ID: "i-2", Type: "aws_instance"},
	}

	got := filterResourcesByType(rs, "aws_instance")
	if len(got) != 2 {
		t.Fatalf("got %d resources, want 2", len(got))
	}
	for _, r := range got {
		if r.Type != "aws_instance" {
			t.Errorf("filter leaked resource of type %q", r.Type)
		}
	}

	empty := filterResourcesByType(rs, "aws_vpc")
	if len(empty) != 0 {
		t.Errorf("expected empty slice for missing type, got %d", len(empty))
	}

	allOut := filterResourcesByType(nil, "aws_instance")
	if allOut == nil {
		t.Error("filterResourcesByType(nil, ...) must return non-nil slice")
	}
}
