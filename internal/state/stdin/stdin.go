// Package stdin implements a StateReader that reads Terraform state from an
// io.Reader (typically os.Stdin, wired by the factory).
package stdin

import (
	"context"
	"fmt"
	"io"

	"github.com/esanchezm/terradrift/internal/core"
	"github.com/esanchezm/terradrift/internal/state"
)

// Reader implements state.StateReader by reading raw Terraform state JSON
// from an injected io.Reader.
type Reader struct {
	r io.Reader
}

// New constructs a Reader that reads from the given io.Reader. In production
// the caller passes os.Stdin; tests pass bytes.NewReader(data).
func New(r io.Reader) *Reader {
	return &Reader{r: r}
}

// Resources reads all bytes from the underlying reader and parses them as
// Terraform state. Reads are capped at state.MaxStateSize to avoid memory
// exhaustion from unbounded input.
func (r *Reader) Resources(ctx context.Context) ([]core.Resource, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	data, err := io.ReadAll(io.LimitReader(r.r, state.MaxStateSize))
	if err != nil {
		return nil, fmt.Errorf("reading state from stdin: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("reading state from stdin: no data received")
	}

	resources, err := state.ParseState(data)
	if err != nil {
		return nil, fmt.Errorf("parsing state from stdin: %w", err)
	}
	return resources, nil
}

// Source returns the constant "stdin" — the state was read from standard
// input.
func (r *Reader) Source() string {
	return "stdin"
}

// Compile-time check that Reader satisfies the StateReader interface.
var _ state.StateReader = (*Reader)(nil)
