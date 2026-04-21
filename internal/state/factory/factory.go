// Package factory constructs a state.StateReader appropriate for a given
// source string. The dispatch rules are:
//
//   - source starting with "s3://"           → S3 reader wrapped in cache
//   - source starting with "http://" or "https://" → HTTP reader wrapped in cache
//   - source equal to "-"                    → stdin reader (no cache)
//   - everything else                        → local reader (no cache)
package factory

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/esanchezm/terradrift/internal/state"
	"github.com/esanchezm/terradrift/internal/state/cache"
	httpstate "github.com/esanchezm/terradrift/internal/state/http"
	"github.com/esanchezm/terradrift/internal/state/local"
	s3backend "github.com/esanchezm/terradrift/internal/state/s3"
	"github.com/esanchezm/terradrift/internal/state/stdin"
)

// NewStateReader returns a StateReader chosen by inspecting source. The
// context is used to initialize any cloud clients (currently just the S3
// default client) and is not retained for later reads — callers pass their
// own context to Resources.
func NewStateReader(ctx context.Context, source string) (state.StateReader, error) {
	switch {
	case strings.HasPrefix(source, "s3://"):
		client, err := s3backend.DefaultClient(ctx)
		if err != nil {
			return nil, fmt.Errorf("initializing S3 client: %w", err)
		}
		reader, err := s3backend.NewFromURI(client, source)
		if err != nil {
			return nil, err
		}
		return cache.New(reader, ""), nil

	case strings.HasPrefix(source, "http://"), strings.HasPrefix(source, "https://"):
		return cache.New(httpstate.New(source), ""), nil

	case source == "-":
		return stdin.New(os.Stdin), nil

	default:
		return local.New(source), nil
	}
}
