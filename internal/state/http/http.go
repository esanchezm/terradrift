// Package httpstate implements a StateReader for Terraform state served over
// HTTP or HTTPS (for example a Terraform Cloud signed download URL or a
// custom HTTP backend).
package httpstate

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/esanchezm/terradrift/internal/core"
	"github.com/esanchezm/terradrift/internal/state"
)

// DefaultTimeout is the total request timeout (dial + TLS + headers + body).
const DefaultTimeout = 30 * time.Second

// userAgent is sent on every request so servers can identify terradrift
// traffic.
const userAgent = "terradrift/0.1 (+https://github.com/esanchezm/terradrift)"

// Reader implements state.StateReader by fetching state from an HTTP(S) URL.
type Reader struct {
	url    string
	client *http.Client
}

// Option configures a Reader at construction time.
type Option func(*Reader)

// WithClient overrides the default HTTP client. Use this in tests to inject
// an httptest-backed client with a short timeout.
func WithClient(client *http.Client) Option {
	return func(r *Reader) { r.client = client }
}

// New constructs a Reader that will GET the given URL. The default client is
// production-tuned with layered timeouts and connection pooling.
func New(url string, opts ...Option) *Reader {
	r := &Reader{
		url:    url,
		client: defaultClient(),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Resources fetches the URL and parses the body as Terraform state.
func (r *Reader) Resources(ctx context.Context) ([]core.Resource, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request for %s: %w", r.url, err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching state from %s: %w", r.url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("fetching state from %s: HTTP %d %s: %s",
			r.url, resp.StatusCode, http.StatusText(resp.StatusCode), truncate(string(snippet)))
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, state.MaxStateSize))
	if err != nil {
		return nil, fmt.Errorf("reading response body from %s: %w", r.url, err)
	}

	resources, err := state.ParseState(data)
	if err != nil {
		return nil, fmt.Errorf("parsing state from %s: %w", r.url, err)
	}
	return resources, nil
}

// Source returns the configured URL.
func (r *Reader) Source() string {
	return r.url
}

// defaultClient returns a production-tuned HTTP client with layered timeouts.
func defaultClient() *http.Client {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
	}
	return &http.Client{Transport: transport, Timeout: DefaultTimeout}
}

// truncate returns s unchanged or trims it to 200 bytes with an ellipsis for
// inclusion in error messages.
func truncate(s string) string {
	const max = 200
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// Compile-time check that Reader satisfies the StateReader interface.
var _ state.StateReader = (*Reader)(nil)
