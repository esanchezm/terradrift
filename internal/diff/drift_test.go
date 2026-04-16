package diff

import (
	"testing"

	"github.com/esanchezm/terradrift/internal/core"
)

func TestCalculateDrift(t *testing.T) {
	desired := []core.Resource{
		{
			ID:       "res-1",
			Type:     "instance",
			Name:     "web-server",
			Provider: "aws",
			Data:     map[string]interface{}{"size": "t2.micro"},
		},
		{
			ID:       "res-2",
			Type:     "bucket",
			Name:     "assets",
			Provider: "aws",
			Data:     map[string]interface{}{"acl": "private"},
		},
		{
			ID:       "res-3",
			Type:     "vpc",
			Name:     "main-vpc",
			Provider: "aws",
			Data:     map[string]interface{}{"cidr": "10.0.0.0/16"},
		},
	}

	actual := []core.Resource{
		{
			ID:       "res-1",
			Type:     "instance",
			Name:     "web-server",
			Provider: "aws",
			Data:     map[string]interface{}{"size": "t2.small"}, // Drifted
		},
		{
			ID:       "res-2",
			Type:     "bucket",
			Name:     "assets",
			Provider: "aws",
			Data:     map[string]interface{}{"acl": "private"}, // Managed
		},
		{
			ID:       "res-4",
			Type:     "security-group",
			Name:     "extra-sg",
			Provider: "aws",
			Data:     map[string]interface{}{"rules": []string{"allow-80"}}, // Unmanaged
		},
	}

	report := CalculateDrift(desired, actual)

	// Check Managed
	if len(report.Managed) != 1 || report.Managed[0].ID != "res-2" {
		t.Errorf("Expected 1 managed resource (res-2), got %v", report.Managed)
	}

	// Check Drifted
	if len(report.Drifted) != 1 || report.Drifted[0].Resource.ID != "res-1" {
		t.Errorf("Expected 1 drifted resource (res-1), got %v", report.Drifted)
	}

	// Check Missing
	if len(report.Missing) != 1 || report.Missing[0].ID != "res-3" {
		t.Errorf("Expected 1 missing resource (res-3), got %v", report.Missing)
	}

	// Check Unmanaged
	if len(report.Unmanaged) != 1 || report.Unmanaged[0].ID != "res-4" {
		t.Errorf("Expected 1 unmanaged resource (res-4), got %v", report.Unmanaged)
	}
}
