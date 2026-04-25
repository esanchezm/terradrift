package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/esanchezm/terradrift/internal/core"
	"github.com/esanchezm/terradrift/internal/ignore"
	"github.com/esanchezm/terradrift/internal/provider"
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

func TestRunScan_IgnorePatterns_FilterUnmanagedAndDrifted(t *testing.T) {
	reader := &fakeStateReader{
		resources: []core.Resource{
			{ID: "i-managed", Type: "aws_instance", Name: "web-managed"},
			{ID: "i-0xyz789", Type: "aws_instance", Name: "web-1", Data: map[string]interface{}{"instance_type": "t3.medium"}},
			{ID: "r-admin", Type: "aws_iam_role", Name: "admin", Data: map[string]interface{}{"path": "/"}},
		},
		source: "state.tfstate",
	}
	prov := &fakeProvider{
		name:  "aws",
		types: []string{"aws_instance", "aws_s3_bucket", "aws_security_group", "aws_iam_role"},
		resources: []core.Resource{
			{ID: "i-managed", Type: "aws_instance", Name: "web-managed"},
			{ID: "i-0xyz789", Type: "aws_instance", Name: "web-1", Data: map[string]interface{}{"instance_type": "t3.large"}},
			{ID: "i-0abc123", Type: "aws_instance", Name: "web-2"},
			{ID: "r-admin", Type: "aws_iam_role", Name: "admin", Data: map[string]interface{}{"path": "/team/"}},
		},
	}

	patterns := writeAndLoadIgnore(t, `# .driftignore
aws_instance.web-2
aws_iam_role.*
`)

	var buf bytes.Buffer
	err := runScan(context.Background(), scanConfig{
		Reader:       reader,
		Provider:     prov,
		ProviderName: "aws",
		Region:       "eu-west-1",
		NoColor:      true,
		Out:          &buf,
		Ignore:       patterns,
	})
	if err != nil {
		t.Fatalf("runScan: %v", err)
	}

	out := buf.String()

	mustContain := []string{
		"Summary: 1 managed, 0 unmanaged, 0 missing, 1 drifted, 2 ignored",
		`instance_type: "t3.medium" → "t3.large"`,
	}
	mustNotContain := []string{
		"aws_instance.web-2 (i-0abc123) — unmanaged",
		"aws_iam_role.admin",
	}
	for _, s := range mustContain {
		if !strings.Contains(out, s) {
			t.Errorf("expected output to contain %q, full output:\n%s", s, out)
		}
	}
	for _, s := range mustNotContain {
		if strings.Contains(out, s) {
			t.Errorf("expected output to NOT contain %q (driftignore suppressed), full output:\n%s", s, out)
		}
	}
}

func TestRunScan_NilIgnore_LeavesReportUnchanged(t *testing.T) {
	reader := &fakeStateReader{
		resources: []core.Resource{{ID: "i-1", Type: "aws_instance", Name: "web"}},
		source:    "s.tfstate",
	}
	prov := &fakeProvider{
		name:      "aws",
		types:     []string{"aws_instance"},
		resources: []core.Resource{{ID: "i-1", Type: "aws_instance", Name: "web"}, {ID: "i-2", Type: "aws_instance", Name: "rogue"}},
	}

	var buf bytes.Buffer
	if err := runScan(context.Background(), scanConfig{
		Reader:       reader,
		Provider:     prov,
		ProviderName: "aws",
		NoColor:      true,
		Quiet:        true,
		Out:          &buf,
		Ignore:       nil,
	}); err != nil {
		t.Fatalf("runScan: %v", err)
	}

	want := "Summary: 1 managed, 1 unmanaged, 0 missing, 0 drifted\n"
	if buf.String() != want {
		t.Errorf("nil Ignore must not change output\n got:  %q\n want: %q", buf.String(), want)
	}
}

func TestLoadIgnorePatterns_ExplicitPath_Loads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.driftignore")
	if err := os.WriteFile(path, []byte("aws_instance.web-1\n"), 0o644); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	p, err := loadIgnorePatterns(path)
	if err != nil {
		t.Fatalf("loadIgnorePatterns: %v", err)
	}
	if !p.Match("aws_instance.web-1") {
		t.Errorf("expected pattern loaded from explicit path")
	}
}

func TestLoadIgnorePatterns_ExplicitPath_MissingFileErrors(t *testing.T) {
	_, err := loadIgnorePatterns(filepath.Join(t.TempDir(), "nope"))
	if err == nil {
		t.Fatal("expected error for missing --ignore-file path")
	}
}

func TestLoadIgnorePatterns_EmptyPath_UsesDiscover(t *testing.T) {
	p, err := loadIgnorePatterns("")
	if err != nil {
		t.Fatalf("loadIgnorePatterns with empty path should not error absent a malformed file: %v", err)
	}
	if p == nil {
		t.Fatal("loadIgnorePatterns with empty path must return non-nil Patterns")
	}
}

func writeAndLoadIgnore(t *testing.T, content string) *ignore.Patterns {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".driftignore")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing .driftignore: %v", err)
	}
	p, err := ignore.LoadFile(path)
	if err != nil {
		t.Fatalf("loading .driftignore: %v", err)
	}
	return p
}

func TestRunScan_ExitOnDrift_UnmanagedPresent_ReturnsDriftError(t *testing.T) {
	reader := &fakeStateReader{resources: nil, source: "s.tfstate"}
	prov := &fakeProvider{
		name:      "aws",
		types:     []string{"aws_instance"},
		resources: []core.Resource{{ID: "i-1", Type: "aws_instance", Name: "rogue"}},
	}

	var buf bytes.Buffer
	err := runScan(context.Background(), scanConfig{
		Reader:       reader,
		Provider:     prov,
		ProviderName: "aws",
		NoColor:      true,
		Out:          &buf,
		ExitOnDrift:  true,
	})
	if err == nil {
		t.Fatal("expected driftError when unmanaged resources exist and ExitOnDrift is true")
	}

	var drift *driftError
	if !errors.As(err, &drift) {
		t.Fatalf("expected *driftError, got %T: %v", err, err)
	}
	if drift.Unmanaged != 1 || drift.Missing != 0 || drift.Drifted != 0 {
		t.Errorf("counts = (u=%d m=%d d=%d), want (1, 0, 0)", drift.Unmanaged, drift.Missing, drift.Drifted)
	}
}

func TestRunScan_ExitOnDrift_MissingPresent_ReturnsDriftError(t *testing.T) {
	reader := &fakeStateReader{
		resources: []core.Resource{{ID: "sg-1", Type: "aws_security_group", Name: "legacy"}},
		source:    "s.tfstate",
	}
	prov := &fakeProvider{name: "aws", types: []string{"aws_security_group"}}

	err := runScan(context.Background(), scanConfig{
		Reader:       reader,
		Provider:     prov,
		ProviderName: "aws",
		NoColor:      true,
		Out:          &bytes.Buffer{},
		ExitOnDrift:  true,
	})

	var drift *driftError
	if !errors.As(err, &drift) {
		t.Fatalf("expected *driftError for missing resource, got %T: %v", err, err)
	}
	if drift.Missing != 1 {
		t.Errorf("Missing = %d, want 1", drift.Missing)
	}
}

func TestRunScan_ExitOnDrift_DriftedPresent_ReturnsDriftError(t *testing.T) {
	reader := &fakeStateReader{
		resources: []core.Resource{{ID: "i-1", Type: "aws_instance", Name: "web", Data: map[string]interface{}{"instance_type": "t3.micro"}}},
		source:    "s.tfstate",
	}
	prov := &fakeProvider{
		name:      "aws",
		types:     []string{"aws_instance"},
		resources: []core.Resource{{ID: "i-1", Type: "aws_instance", Name: "web", Data: map[string]interface{}{"instance_type": "t3.large"}}},
	}

	err := runScan(context.Background(), scanConfig{
		Reader:       reader,
		Provider:     prov,
		ProviderName: "aws",
		NoColor:      true,
		Out:          &bytes.Buffer{},
		ExitOnDrift:  true,
	})

	var drift *driftError
	if !errors.As(err, &drift) {
		t.Fatalf("expected *driftError for drifted resource, got %T: %v", err, err)
	}
	if drift.Drifted != 1 {
		t.Errorf("Drifted = %d, want 1", drift.Drifted)
	}
}

func TestRunScan_ExitOnDriftFalse_DriftPresent_ReturnsNil(t *testing.T) {
	reader := &fakeStateReader{resources: nil, source: "s.tfstate"}
	prov := &fakeProvider{
		name:      "aws",
		types:     []string{"aws_instance"},
		resources: []core.Resource{{ID: "i-1", Type: "aws_instance", Name: "rogue"}},
	}

	var buf bytes.Buffer
	err := runScan(context.Background(), scanConfig{
		Reader:       reader,
		Provider:     prov,
		ProviderName: "aws",
		NoColor:      true,
		Out:          &buf,
		ExitOnDrift:  false,
	})
	if err != nil {
		t.Fatalf("ExitOnDrift=false must not produce error on drift, got: %v", err)
	}
	if !strings.Contains(buf.String(), "unmanaged") {
		t.Errorf("expected drift rendered to stdout even with ExitOnDrift=false, got:\n%s", buf.String())
	}
}

func TestRunScan_ExitOnDrift_CleanScan_ReturnsNil(t *testing.T) {
	shared := []core.Resource{{ID: "i-1", Type: "aws_instance", Name: "web"}}
	reader := &fakeStateReader{resources: shared, source: "s.tfstate"}
	prov := &fakeProvider{name: "aws", types: []string{"aws_instance"}, resources: shared}

	err := runScan(context.Background(), scanConfig{
		Reader:       reader,
		Provider:     prov,
		ProviderName: "aws",
		NoColor:      true,
		Out:          &bytes.Buffer{},
		ExitOnDrift:  true,
	})
	if err != nil {
		t.Errorf("clean scan with ExitOnDrift=true must return nil, got: %v", err)
	}
}

func TestRunScan_ExitOnDrift_OnlyIgnoredResources_ReturnsNil(t *testing.T) {
	reader := &fakeStateReader{resources: nil, source: "s.tfstate"}
	prov := &fakeProvider{
		name:      "aws",
		types:     []string{"aws_instance"},
		resources: []core.Resource{{ID: "i-1", Type: "aws_instance", Name: "rogue"}},
	}

	patterns := writeAndLoadIgnore(t, "aws_instance.rogue\n")

	err := runScan(context.Background(), scanConfig{
		Reader:       reader,
		Provider:     prov,
		ProviderName: "aws",
		NoColor:      true,
		Out:          &bytes.Buffer{},
		Ignore:       patterns,
		ExitOnDrift:  true,
	})
	if err != nil {
		t.Errorf("ExitOnDrift must ignore driftignore-suppressed resources, got: %v", err)
	}
}

func TestRunScan_ExitOnDrift_AcceptanceCounts(t *testing.T) {
	reader := &fakeStateReader{
		resources: []core.Resource{
			{ID: "sg-legacy", Type: "aws_security_group", Name: "legacy"},
			{ID: "i-web-1", Type: "aws_instance", Name: "web-1", Data: map[string]interface{}{"instance_type": "t3.medium"}},
		},
		source: "s.tfstate",
	}
	prov := &fakeProvider{
		name:  "aws",
		types: []string{"aws_instance", "aws_security_group"},
		resources: []core.Resource{
			{ID: "i-web-1", Type: "aws_instance", Name: "web-1", Data: map[string]interface{}{"instance_type": "t3.large"}},
			{ID: "i-rogue-1", Type: "aws_instance", Name: "rogue-1"},
			{ID: "i-rogue-2", Type: "aws_instance", Name: "rogue-2"},
		},
	}

	err := runScan(context.Background(), scanConfig{
		Reader:       reader,
		Provider:     prov,
		ProviderName: "aws",
		NoColor:      true,
		Out:          &bytes.Buffer{},
		ExitOnDrift:  true,
	})

	var drift *driftError
	if !errors.As(err, &drift) {
		t.Fatalf("expected driftError, got %T: %v", err, err)
	}
	if drift.Unmanaged != 2 || drift.Missing != 1 || drift.Drifted != 1 {
		t.Errorf("counts = (u=%d m=%d d=%d), want (2, 1, 1)", drift.Unmanaged, drift.Missing, drift.Drifted)
	}

	stderrCode := handleExitError(err, &bytes.Buffer{})
	if stderrCode != ExitCodeDrift {
		t.Errorf("handleExitError code = %d, want %d", stderrCode, ExitCodeDrift)
	}
}

func TestExecute_Integration_CleanScan_ExitsZero(t *testing.T) {
	resetScanFlags(t)
	tfstate := writeTfstate(t, `{"version":4,"resources":[]}`)
	original := newProviderFn
	newProviderFn = func(_ context.Context, _, _ string) (provider.Provider, error) {
		return &fakeProvider{name: "aws", types: []string{"aws_instance"}, resources: nil}, nil
	}
	defer func() { newProviderFn = original }()

	rootCmd.SetArgs([]string{"scan", "--provider", "aws", "--region", "us-east-1", "--state", tfstate, "--no-color"})
	defer rootCmd.SetArgs(nil)

	code := handleExitError(rootCmd.Execute(), &bytes.Buffer{})
	if code != ExitCodeClean {
		t.Errorf("clean scan exit code = %d, want %d", code, ExitCodeClean)
	}
}

func TestExecute_Integration_DriftDetected_ExitsOne(t *testing.T) {
	resetScanFlags(t)
	tfstate := writeTfstate(t, `{"version":4,"resources":[]}`)
	original := newProviderFn
	newProviderFn = func(_ context.Context, _, _ string) (provider.Provider, error) {
		return &fakeProvider{
			name:      "aws",
			types:     []string{"aws_instance"},
			resources: []core.Resource{{ID: "i-rogue", Type: "aws_instance", Name: "rogue"}},
		}, nil
	}
	defer func() { newProviderFn = original }()

	rootCmd.SetArgs([]string{"scan", "--provider", "aws", "--region", "us-east-1", "--state", tfstate, "--no-color"})
	defer rootCmd.SetArgs(nil)

	var stderr bytes.Buffer
	code := handleExitError(rootCmd.Execute(), &stderr)

	if code != ExitCodeDrift {
		t.Errorf("drift exit code = %d, want %d", code, ExitCodeDrift)
	}
	if !strings.Contains(stderr.String(), "drift detected:") {
		t.Errorf("stderr must contain drift summary, got: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "1 unmanaged") {
		t.Errorf("stderr must reflect counts, got: %q", stderr.String())
	}
}

func TestExecute_Integration_DriftDetected_ExitCodeDisabled_ExitsZero(t *testing.T) {
	resetScanFlags(t)
	tfstate := writeTfstate(t, `{"version":4,"resources":[]}`)
	original := newProviderFn
	newProviderFn = func(_ context.Context, _, _ string) (provider.Provider, error) {
		return &fakeProvider{
			name:      "aws",
			types:     []string{"aws_instance"},
			resources: []core.Resource{{ID: "i-rogue", Type: "aws_instance", Name: "rogue"}},
		}, nil
	}
	defer func() { newProviderFn = original }()

	rootCmd.SetArgs([]string{"scan", "--provider", "aws", "--region", "us-east-1", "--state", tfstate, "--no-color", "--exit-code=false"})
	defer rootCmd.SetArgs(nil)

	var stderr bytes.Buffer
	code := handleExitError(rootCmd.Execute(), &stderr)

	if code != ExitCodeClean {
		t.Errorf("--exit-code=false on drift must return %d (reported, not failed), got %d", ExitCodeClean, code)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr must be empty when --exit-code=false, got: %q", stderr.String())
	}
}

func TestExecute_Integration_MissingProvider_ExitsTwo(t *testing.T) {
	resetScanFlags(t)
	rootCmd.SetArgs([]string{"scan"})
	defer rootCmd.SetArgs(nil)

	var stderr bytes.Buffer
	code := handleExitError(rootCmd.Execute(), &stderr)

	if code != ExitCodeError {
		t.Errorf("missing --provider exit code = %d, want %d", code, ExitCodeError)
	}
	if !strings.Contains(stderr.String(), "provider is required") {
		t.Errorf("stderr must mention missing provider, got: %q", stderr.String())
	}
	if strings.Contains(stderr.String(), "Usage:") {
		t.Errorf("stderr must not spam usage (SilenceUsage=true), got: %q", stderr.String())
	}
}

func TestExecute_Integration_UnknownSubcommand_ExitsTwo_NoUsageSpam(t *testing.T) {
	resetScanFlags(t)
	rootCmd.SetArgs([]string{"definitely-not-a-command"})
	defer rootCmd.SetArgs(nil)

	var stderr bytes.Buffer
	code := handleExitError(rootCmd.Execute(), &stderr)

	if code != ExitCodeError {
		t.Errorf("unknown command exit code = %d, want %d", code, ExitCodeError)
	}
	if strings.Contains(stderr.String(), "Usage:") {
		t.Errorf("rootCmd must silence usage on unknown command, got: %q", stderr.String())
	}
	errPrefixCount := strings.Count(stderr.String(), "Error:")
	if errPrefixCount != 1 {
		t.Errorf("expected exactly one 'Error:' prefix (handleExitError owns stderr), got %d in: %q",
			errPrefixCount, stderr.String())
	}
}

func TestExecute_Integration_UnknownRootFlag_ExitsTwo_NoUsageSpam(t *testing.T) {
	resetScanFlags(t)
	rootCmd.SetArgs([]string{"--definitely-not-a-flag"})
	defer rootCmd.SetArgs(nil)

	var stderr bytes.Buffer
	code := handleExitError(rootCmd.Execute(), &stderr)

	if code != ExitCodeError {
		t.Errorf("unknown flag exit code = %d, want %d", code, ExitCodeError)
	}
	if strings.Contains(stderr.String(), "Usage:") {
		t.Errorf("rootCmd must silence usage on unknown flag, got: %q", stderr.String())
	}
	errPrefixCount := strings.Count(stderr.String(), "Error:")
	if errPrefixCount != 1 {
		t.Errorf("expected exactly one 'Error:' prefix, got %d in: %q",
			errPrefixCount, stderr.String())
	}
}

func writeTfstate(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "terraform.tfstate")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("writing tfstate: %v", err)
	}
	return path
}

// resetScanFlags clears every package-level scan flag variable so an
// integration test starts with a known baseline. Cobra binds flags to
// these vars at init time and never zeroes them between Execute calls,
// which would otherwise leak state across tests in this file (a missing
// --provider would silently inherit the value set by the previous test).
func resetScanFlags(t *testing.T) {
	t.Helper()
	scanProvider = ""
	scanType = ""
	scanRegion = ""
	scanState = ""
	scanNoColor = false
	scanQuiet = false
	scanIgnoreFile = ""
	scanExitCode = true
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
