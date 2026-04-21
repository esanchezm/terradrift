package cache

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/esanchezm/terradrift/internal/core"
)

type fakeReader struct {
	source    string
	resources []core.Resource
	err       error
}

func (f *fakeReader) Resources(ctx context.Context) ([]core.Resource, error) {
	return f.resources, f.err
}

func (f *fakeReader) Source() string {
	return f.source
}

func TestResources_SuccessWritesCache(t *testing.T) {
	dir := t.TempDir()
	inner := &fakeReader{
		source: "s3://bucket/key",
		resources: []core.Resource{
			{ID: "res-1", Type: "aws_instance"},
			{ID: "res-2", Type: "aws_s3_bucket"},
		},
	}
	r := New(inner, dir)

	got, err := r.Resources(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 resources, got %d", len(got))
	}

	path := r.cachePath()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cache file not found at %s: %v", path, err)
	}

	var cached []core.Resource
	if err := json.Unmarshal(data, &cached); err != nil {
		t.Fatalf("failed to decode cache JSON: %v", err)
	}
	if len(cached) != 2 || cached[0].ID != "res-1" || cached[1].ID != "res-2" {
		t.Errorf("cached resources mismatch: %+v", cached)
	}
}

func TestResources_FailureFallsBackToCache(t *testing.T) {
	dir := t.TempDir()
	inner := &fakeReader{source: "s3://bucket/key"}

	r := New(inner, dir)

	preloaded := []core.Resource{{ID: "cached-1", Type: "aws_instance"}}
	data, _ := json.Marshal(preloaded)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.cachePath(), data, 0o644); err != nil {
		t.Fatal(err)
	}

	inner.err = errors.New("remote unavailable")

	got, err := r.Resources(context.Background())
	if err != nil {
		t.Fatalf("expected no error on cache fallback, got: %v", err)
	}
	if len(got) != 1 || got[0].ID != "cached-1" {
		t.Errorf("expected cached resources, got: %+v", got)
	}
}

func TestResources_BothFailReturnsJoinedError(t *testing.T) {
	dir := t.TempDir()
	inner := &fakeReader{
		source: "s3://bucket/missing",
		err:    errors.New("remote unavailable"),
	}
	r := New(inner, dir)

	_, err := r.Resources(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "remote:") {
		t.Errorf("error missing 'remote:' prefix: %s", msg)
	}
	if !strings.Contains(msg, "cache fallback:") {
		t.Errorf("error missing 'cache fallback:' prefix: %s", msg)
	}
}

func TestResources_CacheWriteFailureIgnored(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("skipping: running as root, read-only dir test is not reliable")
	}

	dir := t.TempDir()
	roDir := dir + "/readonly"
	if err := os.MkdirAll(roDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(roDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(roDir, 0o755) })

	inner := &fakeReader{
		source: "s3://bucket/key",
		resources: []core.Resource{
			{ID: "res-1", Type: "aws_instance"},
		},
	}
	r := New(inner, roDir)

	got, err := r.Resources(context.Background())
	if err != nil {
		t.Fatalf("expected no error even with write failure, got: %v", err)
	}
	if len(got) != 1 || got[0].ID != "res-1" {
		t.Errorf("expected original resources, got: %+v", got)
	}
}

func TestCachePath_DeterministicHash(t *testing.T) {
	dir := t.TempDir()
	inner := &fakeReader{source: "s3://bucket/deterministic"}

	r1 := New(inner, dir)
	r2 := New(inner, dir)

	if r1.cachePath() != r2.cachePath() {
		t.Errorf("cache paths differ for identical source: %s vs %s", r1.cachePath(), r2.cachePath())
	}
}

func TestCachePath_DifferentSources(t *testing.T) {
	dir := t.TempDir()

	r1 := New(&fakeReader{source: "s3://bucket/a"}, dir)
	r2 := New(&fakeReader{source: "s3://bucket/b"}, dir)

	if r1.cachePath() == r2.cachePath() {
		t.Errorf("expected different cache paths for different sources, both: %s", r1.cachePath())
	}
}

func TestSource_DelegatesToInner(t *testing.T) {
	inner := &fakeReader{source: "s3://my-bucket/tfstate"}
	r := New(inner, t.TempDir())

	if r.Source() != inner.source {
		t.Errorf("Source() = %q, want %q", r.Source(), inner.source)
	}
}

func TestNew_EmptyDirUsesDefault(t *testing.T) {
	inner := &fakeReader{source: "s3://bucket/key"}
	r := New(inner, "")

	if r.dir != DefaultDir {
		t.Errorf("dir = %q, want %q", r.dir, DefaultDir)
	}
}
