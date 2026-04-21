package state

import (
	"strings"
	"testing"
)

const validStateJSON = `{
  "version": 4,
  "terraform_version": "1.9.0",
  "serial": 1,
  "lineage": "00000000-0000-0000-0000-000000000000",
  "outputs": {},
  "resources": [
    {
      "mode": "managed",
      "type": "aws_instance",
      "name": "web",
      "provider": "provider[\"registry.terraform.io/hashicorp/aws\"]",
      "instances": [
        {
          "schema_version": 1,
          "attributes": {
            "id": "i-0abc",
            "instance_type": "t3.micro"
          }
        }
      ]
    },
    {
      "mode": "managed",
      "type": "google_compute_instance",
      "name": "vm",
      "provider": "provider[\"registry.terraform.io/hashicorp/google\"]",
      "instances": [
        {
          "schema_version": 0,
          "attributes": {
            "id": "projects/p/zones/z/instances/vm",
            "machine_type": "e2-medium"
          }
        }
      ]
    },
    {
      "mode": "data",
      "type": "aws_ami",
      "name": "ubuntu",
      "provider": "provider[\"registry.terraform.io/hashicorp/aws\"]",
      "instances": [
        {
          "schema_version": 0,
          "attributes": {
            "id": "ami-0abc"
          }
        }
      ]
    }
  ]
}`

func TestParseState_ValidJSON(t *testing.T) {
	resources, err := ParseState([]byte(validStateJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := len(resources); got != 2 {
		t.Fatalf("expected 2 managed resources, got %d", got)
	}

	want := []struct {
		id       string
		typ      string
		name     string
		provider string
	}{
		{"i-0abc", "aws_instance", "web", "aws"},
		{"projects/p/zones/z/instances/vm", "google_compute_instance", "vm", "google"},
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

func TestParseState_DataSourcesExcluded(t *testing.T) {
	const onlyDataSources = `{
  "version": 4,
  "resources": [
    {
      "mode": "data",
      "type": "aws_ami",
      "name": "ubuntu",
      "provider": "provider[\"registry.terraform.io/hashicorp/aws\"]",
      "instances": [{"attributes": {"id": "ami-1"}}]
    }
  ]
}`

	resources, err := ParseState([]byte(onlyDataSources))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resources) != 0 {
		t.Errorf("expected 0 resources, got %d", len(resources))
	}
}

func TestParseState_MalformedJSON(t *testing.T) {
	_, err := ParseState([]byte("{invalid"))
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "decoding state JSON") {
		t.Errorf("expected error to mention 'decoding state JSON', got: %v", err)
	}
}

func TestParseState_EmptyResources(t *testing.T) {
	const empty = `{"version": 4, "resources": []}`

	resources, err := ParseState([]byte(empty))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resources) != 0 {
		t.Errorf("expected 0 resources, got %d", len(resources))
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
		if got := ExtractProvider(tt.input); got != tt.want {
			t.Errorf("ExtractProvider(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
