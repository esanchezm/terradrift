package stdin

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

const validStateJSON = `{
  "version": 4,
  "resources": [
    {
      "mode": "managed",
      "type": "aws_instance",
      "name": "web",
      "provider": "provider[\"registry.terraform.io/hashicorp/aws\"]",
      "instances": [
        {"attributes": {"id": "i-1", "instance_type": "t3.micro"}}
      ]
    }
  ]
}`

func TestResources_ValidState(t *testing.T) {
	r := New(bytes.NewReader([]byte(validStateJSON)))

	resources, err := r.Resources(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := len(resources); got != 1 {
		t.Fatalf("expected 1 resource, got %d", got)
	}

	got := resources[0]
	if got.ID != "i-1" {
		t.Errorf("ID = %q, want %q", got.ID, "i-1")
	}
	if got.Type != "aws_instance" {
		t.Errorf("Type = %q, want %q", got.Type, "aws_instance")
	}
	if got.Provider != "aws" {
		t.Errorf("Provider = %q, want %q", got.Provider, "aws")
	}
}

func TestResources_EmptyInput(t *testing.T) {
	r := New(bytes.NewReader(nil))

	_, err := r.Resources(context.Background())
	if err == nil {
		t.Fatal("expected error for empty input, got nil")
	}
	if !strings.Contains(err.Error(), "no data received") {
		t.Errorf("expected error to mention 'no data received', got: %v", err)
	}
}

func TestResources_MalformedJSON(t *testing.T) {
	r := New(bytes.NewReader([]byte("{not json")))

	_, err := r.Resources(context.Background())
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "parsing state") {
		t.Errorf("expected error to mention 'parsing state', got: %v", err)
	}
}

func TestResources_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r := New(bytes.NewReader([]byte(validStateJSON)))

	_, err := r.Resources(ctx)
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

func TestResources_ReaderError(t *testing.T) {
	r := New(&errReader{err: errors.New("read failed")})

	_, err := r.Resources(context.Background())
	if err == nil {
		t.Fatal("expected error from reader, got nil")
	}
	if !strings.Contains(err.Error(), "reading state from stdin") {
		t.Errorf("expected error to mention 'reading state from stdin', got: %v", err)
	}
}

func TestSource(t *testing.T) {
	r := New(bytes.NewReader(nil))
	if got := r.Source(); got != "stdin" {
		t.Errorf("Source() = %q, want %q", got, "stdin")
	}
}

type errReader struct {
	err error
}

func (e *errReader) Read([]byte) (int, error) {
	return 0, e.err
}
