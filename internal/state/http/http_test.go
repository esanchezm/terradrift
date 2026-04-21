package httpstate

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func loadValidState(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/valid.tfstate")
	if err != nil {
		t.Fatalf("reading testdata: %v", err)
	}
	return data
}

func TestResources_ValidState(t *testing.T) {
	validStateBytes := loadValidState(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(validStateBytes)
	}))
	defer server.Close()

	reader := New(server.URL, WithClient(server.Client()))
	resources, err := reader.Resources(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resources) != 5 {
		t.Fatalf("expected 5 resources, got %d", len(resources))
	}

	wantIDs := []string{
		"i-1234567890abcdef0",
		"my-assets-bucket",
		"sg-0123456789abcdef0",
		"vpc-abcdef01",
		"projects/my-project/zones/us-central1-a/instances/vm",
	}
	for i, r := range resources {
		if r.ID != wantIDs[i] {
			t.Errorf("resource[%d].ID = %q, want %q", i, r.ID, wantIDs[i])
		}
	}
}

func TestResources_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "state file not found", http.StatusNotFound)
	}))
	defer server.Close()

	reader := New(server.URL, WithClient(server.Client()))
	_, err := reader.Resources(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !contains(err.Error(), "HTTP 404") {
		t.Errorf("error %q does not contain %q", err.Error(), "HTTP 404")
	}
}

func TestResources_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer server.Close()

	reader := New(server.URL, WithClient(server.Client()))
	_, err := reader.Resources(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !contains(err.Error(), "HTTP 500") {
		t.Errorf("error %q does not contain %q", err.Error(), "HTTP 500")
	}
	if !contains(err.Error(), "boom") {
		t.Errorf("error %q does not contain %q", err.Error(), "boom")
	}
}

func TestResources_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{not json"))
	}))
	defer server.Close()

	reader := New(server.URL, WithClient(server.Client()))
	_, err := reader.Resources(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !contains(err.Error(), "parsing state") {
		t.Errorf("error %q does not contain %q", err.Error(), "parsing state")
	}
}

func TestResources_ContextCanceled(t *testing.T) {
	unblock := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-unblock
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	defer close(unblock)

	reader := New(server.URL, WithClient(server.Client()))

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		_, err := reader.Resources(ctx)
		errCh <- err
	}()

	cancel()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled in error chain, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for canceled request to return")
	}
}

func TestResources_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	shortClient := &http.Client{Timeout: 20 * time.Millisecond}
	reader := New(server.URL, WithClient(shortClient))

	_, err := reader.Resources(context.Background())
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestResources_UserAgentSent(t *testing.T) {
	validStateBytes := loadValidState(t)

	var gotUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(validStateBytes)
	}))
	defer server.Close()

	reader := New(server.URL, WithClient(server.Client()))
	_, err := reader.Resources(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantUA := "terradrift/0.1 (+https://github.com/esanchezm/terradrift)"
	if gotUA != wantUA {
		t.Errorf("User-Agent = %q, want %q", gotUA, wantUA)
	}
}

func TestSource(t *testing.T) {
	url := "https://example.com/state"
	reader := New(url)
	if got := reader.Source(); got != url {
		t.Errorf("Source() = %q, want %q", got, url)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}())
}
