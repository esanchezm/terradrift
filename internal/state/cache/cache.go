// Package cache provides a decorator for state.StateReader that persists
// successful read results to disk and falls back to the cached copy when a
// subsequent read fails (for example when offline or during a dry-run).
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/esanchezm/terradrift/internal/core"
	"github.com/esanchezm/terradrift/internal/state"
)

// DefaultDir is the cache directory used when the caller passes an empty
// string to New.
const DefaultDir = ".terradrift-cache"

// Reader wraps another state.StateReader with "try remote, fall back to local
// cache" semantics.
type Reader struct {
	inner state.StateReader
	dir   string
}

// New creates a caching Reader. If dir is empty, DefaultDir is used.
func New(inner state.StateReader, dir string) *Reader {
	if dir == "" {
		dir = DefaultDir
	}
	return &Reader{inner: inner, dir: dir}
}

// Resources attempts to read from the inner reader. On success the result is
// written to the cache (errors are ignored) and returned. On failure the
// cached copy is consulted; if present it is returned, otherwise the original
// error is returned joined with the cache-read error.
func (r *Reader) Resources(ctx context.Context) ([]core.Resource, error) {
	resources, err := r.inner.Resources(ctx)
	if err == nil {
		_ = r.writeCache(resources)
		return resources, nil
	}

	cached, cacheErr := r.readCache()
	if cacheErr == nil {
		return cached, nil
	}

	return nil, errors.Join(
		fmt.Errorf("remote: %w", err),
		fmt.Errorf("cache fallback: %w", cacheErr),
	)
}

// Source delegates to the inner reader's Source.
func (r *Reader) Source() string {
	return r.inner.Source()
}

// cachePath returns the on-disk path for this reader's cached state, keyed by
// sha256(inner.Source()).
func (r *Reader) cachePath() string {
	h := sha256.Sum256([]byte(r.inner.Source()))
	name := "state-" + hex.EncodeToString(h[:]) + ".json"
	return filepath.Join(r.dir, name)
}

// writeCache serializes resources to disk under cachePath. Errors are
// returned so callers can observe them in tests; production callers ignore
// the error (best-effort cache).
func (r *Reader) writeCache(resources []core.Resource) error {
	if err := os.MkdirAll(r.dir, 0o755); err != nil {
		return fmt.Errorf("creating cache dir: %w", err)
	}
	data, err := json.Marshal(resources)
	if err != nil {
		return fmt.Errorf("marshaling cache: %w", err)
	}
	if err := os.WriteFile(r.cachePath(), data, 0o644); err != nil {
		return fmt.Errorf("writing cache: %w", err)
	}
	return nil
}

// readCache reads and decodes the cached resources for this source.
func (r *Reader) readCache() ([]core.Resource, error) {
	data, err := os.ReadFile(r.cachePath())
	if err != nil {
		return nil, fmt.Errorf("reading cache file: %w", err)
	}
	var resources []core.Resource
	if err := json.Unmarshal(data, &resources); err != nil {
		return nil, fmt.Errorf("decoding cache JSON: %w", err)
	}
	return resources, nil
}

// Compile-time check that Reader satisfies the StateReader interface.
var _ state.StateReader = (*Reader)(nil)
