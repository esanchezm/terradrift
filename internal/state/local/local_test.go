package local

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/esanchezm/terradrift/internal/core"
	"github.com/esanchezm/terradrift/internal/state"
)

// Compile-time check that Reader satisfies the StateReader interface.
var _ state.StateReader = (*Reader)(nil)

func TestResources_ValidState(t *testing.T) {
	r := New(filepath.Join("testdata", "valid.tfstate"))

	resources, err := r.Resources(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := len(resources); got != 5 {
		t.Fatalf("expected 5 resources, got %d", got)
	}

	want := []struct {
		id       string
		typ      string
		name     string
		provider string
	}{
		{"i-1234567890abcdef0", "aws_instance", "web", "aws"},
		{"my-assets-bucket", "aws_s3_bucket", "assets", "aws"},
		{"sg-0123456789abcdef0", "aws_security_group", "allow_tls", "aws"},
		{"vpc-abcdef01", "aws_vpc", "main", "aws"},
		{"projects/my-project/zones/us-central1-a/instances/vm", "google_compute_instance", "vm", "google"},
	}

	for i, w := range want {
		got := resources[i]
		if got.ID != w.id {
			t.Errorf("resource[%d].ID = %q, want %q", i, got.ID, w.id)
		}
		if got.Type != w.typ {
			t.Errorf("resource[%d].Type = %q, want %q", i, got.Type, w.typ)
		}
		if got.Name != w.name {
			t.Errorf("resource[%d].Name = %q, want %q", i, got.Name, w.name)
		}
		if got.Provider != w.provider {
			t.Errorf("resource[%d].Provider = %q, want %q", i, got.Provider, w.provider)
		}
		if got.Data == nil {
			t.Errorf("resource[%d].Data is nil", i)
		}
	}
}

func TestResources_DataSourcesExcluded(t *testing.T) {
	r := New(filepath.Join("testdata", "valid.tfstate"))

	resources, err := r.Resources(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, res := range resources {
		if res.Type == "aws_ami" || res.Type == "aws_availability_zones" {
			t.Errorf("data source %q should have been excluded", res.Type)
		}
	}
}

func TestResources_Attributes(t *testing.T) {
	r := New(filepath.Join("testdata", "valid.tfstate"))

	resources, err := r.Resources(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	first := findResource(resources, "aws_instance", "web")
	if first == nil {
		t.Fatal("aws_instance.web not found")
	}

	if v, ok := first.Data["instance_type"].(string); !ok || v != "t3.micro" {
		t.Errorf("instance_type = %v, want %q", first.Data["instance_type"], "t3.micro")
	}
}

func TestResources_MissingFile(t *testing.T) {
	r := New(filepath.Join("testdata", "nonexistent.tfstate"))

	resources, err := r.Resources(context.Background())
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}

	if len(resources) != 0 {
		t.Errorf("expected empty slice, got %d resources", len(resources))
	}
}

func TestResources_MalformedJSON(t *testing.T) {
	r := New(filepath.Join("testdata", "malformed.tfstate"))

	_, err := r.Resources(context.Background())
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestSource(t *testing.T) {
	pattern := "./infra/**/*.tfstate"
	r := New(pattern)

	if got := r.Source(); got != pattern {
		t.Errorf("Source() = %q, want %q", got, pattern)
	}
}

func TestExtractProvider(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`provider["registry.terraform.io/hashicorp/aws"]`, "aws"},
		{`provider["registry.terraform.io/hashicorp/google"]`, "google"},
		{`provider["registry.terraform.io/hashicorp/azurerm"]`, "azurerm"},
		{"aws", "aws"},
		{"", ""},
	}

	for _, tt := range tests {
		if got := extractProvider(tt.input); got != tt.want {
			t.Errorf("extractProvider(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func findResource(resources []core.Resource, typ, name string) *core.Resource {
	for i := range resources {
		if resources[i].Type == typ && resources[i].Name == name {
			return &resources[i]
		}
	}
	return nil
}
