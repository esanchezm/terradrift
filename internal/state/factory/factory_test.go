package factory

import (
	"context"
	"strings"
	"testing"
)

func TestNewStateReader_Stdin(t *testing.T) {
	r, err := NewStateReader(context.Background(), "-")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := r.Source(); got != "stdin" {
		t.Errorf("Source() = %q, want %q", got, "stdin")
	}
}

func TestNewStateReader_HTTP(t *testing.T) {
	const url = "https://example.com/terraform.tfstate"
	r, err := NewStateReader(context.Background(), url)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := r.Source(); got != url {
		t.Errorf("Source() = %q, want %q", got, url)
	}
}

func TestNewStateReader_HTTPInsecure(t *testing.T) {
	const url = "http://example.com/terraform.tfstate"
	r, err := NewStateReader(context.Background(), url)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := r.Source(); got != url {
		t.Errorf("Source() = %q, want %q", got, url)
	}
}

func TestNewStateReader_Local(t *testing.T) {
	const pattern = "./infra/**/*.tfstate"
	r, err := NewStateReader(context.Background(), pattern)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := r.Source(); got != pattern {
		t.Errorf("Source() = %q, want %q", got, pattern)
	}
}

func TestNewStateReader_S3Dispatch(t *testing.T) {
	// S3 dispatch creates a client via LoadDefaultConfig. If AWS config is
	// available the call succeeds; if not it returns a "initializing S3
	// client" error. Both outcomes are acceptable here — we only verify the
	// s3:// prefix is dispatched to the S3 branch (not falling through to
	// local).
	const uri = "s3://my-bucket/path/to/state.tfstate"
	r, err := NewStateReader(context.Background(), uri)
	if err != nil {
		// Expected path when no AWS credentials are configured in the
		// environment. Confirm it's from the S3 branch.
		if !strings.Contains(err.Error(), "S3") && !strings.Contains(err.Error(), "AWS") {
			t.Fatalf("expected S3 or AWS error, got: %v", err)
		}
		return
	}
	// Happy path: verify Source() delegates through cache to S3 reader.
	if got := r.Source(); got != uri {
		t.Errorf("Source() = %q, want %q", got, uri)
	}
}

func TestNewStateReader_S3InvalidURI(t *testing.T) {
	// Malformed S3 URI — the error may come from the S3 client construction
	// (if AWS creds unavailable) OR from parseURI. Either way we expect a
	// non-nil error, and not a local reader fallback.
	_, err := NewStateReader(context.Background(), "s3://")
	if err == nil {
		t.Fatal("expected error for malformed S3 URI, got nil")
	}
}

func TestNewStateReader_DispatchTable(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		wantSource string
	}{
		{"stdin sentinel", "-", "stdin"},
		{"https URL", "https://example.com/state", "https://example.com/state"},
		{"http URL", "http://localhost:8080/state", "http://localhost:8080/state"},
		{"local glob", "./terraform.tfstate", "./terraform.tfstate"},
		{"local absolute", "/var/lib/state.tfstate", "/var/lib/state.tfstate"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := NewStateReader(context.Background(), tt.source)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := r.Source(); got != tt.wantSource {
				t.Errorf("Source() = %q, want %q", got, tt.wantSource)
			}
		})
	}
}
