package ignore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse_Empty_ReturnsEmptyPatterns(t *testing.T) {
	p, err := Parse(strings.NewReader(""), "test")
	if err != nil {
		t.Fatalf("Parse empty: %v", err)
	}
	if !p.IsEmpty() {
		t.Errorf("expected empty Patterns, got Len=%d", p.Len())
	}
	if p.Source() != "test" {
		t.Errorf("Source() = %q, want %q", p.Source(), "test")
	}
}

func TestParse_OnlyCommentsAndBlanks_ReturnsEmpty(t *testing.T) {
	input := "# comment one\n\n   \n   # indented comment\n\t\n"
	p, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !p.IsEmpty() {
		t.Errorf("expected empty Patterns, got %d patterns", p.Len())
	}
}

func TestParse_MixedContent_ExtractsPatternsOnly(t *testing.T) {
	input := `# Manually created for testing
aws_instance.web-2
# Legacy buckets managed by scripts
aws_s3_bucket.temp-*

# Entire resource type ignored
aws_iam_role.*
`
	p, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got, want := p.Len(), 3; got != want {
		t.Fatalf("pattern count = %d, want %d", got, want)
	}
	for _, identity := range []string{"aws_instance.web-2", "aws_s3_bucket.temp-logs", "aws_iam_role.admin"} {
		if !p.Match(identity) {
			t.Errorf("pattern set should match %q", identity)
		}
	}
}

func TestParse_TrimsLeadingAndTrailingWhitespace(t *testing.T) {
	input := "  aws_instance.web-1  \n\taws_s3_bucket.logs\t\n"
	p, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !p.Match("aws_instance.web-1") {
		t.Errorf("whitespace should be trimmed from leading/trailing")
	}
	if !p.Match("aws_s3_bucket.logs") {
		t.Errorf("whitespace should be trimmed from leading/trailing")
	}
}

func TestParse_InvalidPattern_ReturnsLineQualifiedError(t *testing.T) {
	input := "aws_instance.web-1\n[broken\naws_s3_bucket.logs\n"
	_, err := Parse(strings.NewReader(input), ".driftignore")
	if err == nil {
		t.Fatal("expected error for malformed glob, got nil")
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("error should mention line 2, got: %v", err)
	}
	if !strings.Contains(err.Error(), "[broken") {
		t.Errorf("error should quote offending pattern, got: %v", err)
	}
	if !strings.Contains(err.Error(), ".driftignore") {
		t.Errorf("error should mention source, got: %v", err)
	}
}

func TestLoadFile_Nonexistent_ReturnsError(t *testing.T) {
	_, err := LoadFile(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadFile_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".driftignore")
	content := "aws_instance.web-1\naws_s3_bucket.temp-*\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("seeding file: %v", err)
	}

	p, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if p.Len() != 2 {
		t.Errorf("Len = %d, want 2", p.Len())
	}
	if p.Source() != path {
		t.Errorf("Source = %q, want %q", p.Source(), path)
	}
}

func TestDiscover_CWD_LoadsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, DefaultFileName)
	if err := os.WriteFile(path, []byte("aws_instance.web-1\n"), 0o644); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	p, err := Discover(dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if !p.Match("aws_instance.web-1") {
		t.Errorf("expected pattern loaded from CWD file")
	}
	if p.Source() != path {
		t.Errorf("Source = %q, want %q", p.Source(), path)
	}
}

func TestDiscover_GitRoot_FallbackWhenCWDHasNoFile(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("seeding .git: %v", err)
	}
	rootFile := filepath.Join(root, DefaultFileName)
	if err := os.WriteFile(rootFile, []byte("aws_iam_role.*\n"), 0o644); err != nil {
		t.Fatalf("seeding root file: %v", err)
	}

	sub := filepath.Join(root, "subdir")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("seeding subdir: %v", err)
	}

	p, err := Discover(sub)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if !p.Match("aws_iam_role.admin") {
		t.Errorf("expected pattern loaded from git root when CWD has none")
	}
	if p.Source() != rootFile {
		t.Errorf("Source = %q, want %q", p.Source(), rootFile)
	}
}

func TestDiscover_CWDTakesPrecedenceOverGitRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("seeding .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, DefaultFileName), []byte("aws_iam_role.*\n"), 0o644); err != nil {
		t.Fatalf("seeding root file: %v", err)
	}

	sub := filepath.Join(root, "subdir")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("seeding subdir: %v", err)
	}
	subFile := filepath.Join(sub, DefaultFileName)
	if err := os.WriteFile(subFile, []byte("aws_instance.web-*\n"), 0o644); err != nil {
		t.Fatalf("seeding sub file: %v", err)
	}

	p, err := Discover(sub)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if p.Source() != subFile {
		t.Errorf("Source = %q, want CWD file %q (not git root)", p.Source(), subFile)
	}
	if p.Match("aws_iam_role.admin") {
		t.Errorf("CWD file should have been used, not git root file")
	}
	if !p.Match("aws_instance.web-1") {
		t.Errorf("CWD pattern should be active")
	}
}

func TestDiscover_NoFileFound_ReturnsEmptyNonNil(t *testing.T) {
	dir := t.TempDir()

	p, err := Discover(dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if p == nil {
		t.Fatal("Discover must return non-nil Patterns even when no file found")
	}
	if !p.IsEmpty() {
		t.Errorf("expected empty patterns when no file found")
	}
}

func TestDiscover_DriftignoreDirectory_IsIgnored(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, DefaultFileName), 0o755); err != nil {
		t.Fatalf("seeding directory: %v", err)
	}

	p, err := Discover(dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if !p.IsEmpty() {
		t.Errorf("a directory named .driftignore must not be treated as a file")
	}
}

func TestMatch_Cases(t *testing.T) {
	p, err := Parse(strings.NewReader(`aws_instance.web-2
aws_s3_bucket.temp-*
aws_iam_role.*
aws_instance.dbs-[12]
`), "t")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	cases := []struct {
		identity string
		want     bool
	}{
		{"aws_instance.web-2", true},
		{"aws_instance.web-3", false},
		{"aws_s3_bucket.temp-logs", true},
		{"aws_s3_bucket.temp-", true},
		{"aws_s3_bucket.backups", false},
		{"aws_iam_role.admin", true},
		{"aws_iam_role.", true},
		{"aws_instance.dbs-1", true},
		{"aws_instance.dbs-2", true},
		{"aws_instance.dbs-3", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := p.Match(tc.identity); got != tc.want {
			t.Errorf("Match(%q) = %v, want %v", tc.identity, got, tc.want)
		}
	}
}

func TestMatch_NilReceiver_ReturnsFalse(t *testing.T) {
	var p *Patterns
	if p.Match("anything") {
		t.Error("nil receiver Match must return false")
	}
	if !p.IsEmpty() {
		t.Error("nil receiver IsEmpty must return true")
	}
	if p.Len() != 0 {
		t.Error("nil receiver Len must return 0")
	}
	if p.Source() != "" {
		t.Error("nil receiver Source must return empty string")
	}
}

func TestParse_DoesNotMatchAcrossDot(t *testing.T) {
	p, err := Parse(strings.NewReader("aws_*.web-*\n"), "t")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !p.Match("aws_instance.web-1") {
		t.Error("should match aws_instance.web-1")
	}
	if !p.Match("aws_s3_bucket.web-x") {
		t.Error("* before dot should match any non-dot run in first segment")
	}
}
