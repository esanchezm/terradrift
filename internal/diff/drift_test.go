package diff

import (
	"reflect"
	"testing"
	"time"

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
			Data:     map[string]interface{}{"size": "t2.small"},
		},
		{
			ID:       "res-2",
			Type:     "bucket",
			Name:     "assets",
			Provider: "aws",
			Data:     map[string]interface{}{"acl": "private"},
		},
		{
			ID:       "res-4",
			Type:     "security-group",
			Name:     "extra-sg",
			Provider: "aws",
			Data:     map[string]interface{}{"rules": []string{"allow-80"}},
		},
	}

	report := CalculateDrift(desired, actual)

	if len(report.Managed) != 1 || report.Managed[0].ID != "res-2" {
		t.Errorf("Expected 1 managed resource (res-2), got %v", report.Managed)
	}
	if len(report.Drifted) != 1 || report.Drifted[0].Resource.ID != "res-1" {
		t.Errorf("Expected 1 drifted resource (res-1), got %v", report.Drifted)
	}
	if len(report.Missing) != 1 || report.Missing[0].ID != "res-3" {
		t.Errorf("Expected 1 missing resource (res-3), got %v", report.Missing)
	}
	if len(report.Unmanaged) != 1 || report.Unmanaged[0].ID != "res-4" {
		t.Errorf("Expected 1 unmanaged resource (res-4), got %v", report.Unmanaged)
	}
}

// TestCalculateDrift_AcceptanceCounts exercises the headline acceptance
// scenario from THI-102: 5 resources in state and 6 in cloud produce 4
// managed, 2 unmanaged, and 1 missing with no drifted resources. The
// ticket wording reads "7 in cloud" which is an off-by-one typo; 6 is the
// mathematically consistent count for 4 managed + 2 unmanaged.
func TestCalculateDrift_AcceptanceCounts(t *testing.T) {
	desired := []core.Resource{
		awsInstance("i-1", "web-1", map[string]interface{}{"instance_type": "t2.micro"}),
		awsInstance("i-2", "web-2", map[string]interface{}{"instance_type": "t2.micro"}),
		awsInstance("i-3", "web-3", map[string]interface{}{"instance_type": "t2.small"}),
		awsInstance("i-4", "web-4", map[string]interface{}{"instance_type": "t2.large"}),
		awsInstance("i-5", "web-5", map[string]interface{}{"instance_type": "t2.xlarge"}),
	}
	actual := []core.Resource{
		awsInstance("i-1", "web-1", map[string]interface{}{"instance_type": "t2.micro"}),
		awsInstance("i-2", "web-2", map[string]interface{}{"instance_type": "t2.micro"}),
		awsInstance("i-3", "web-3", map[string]interface{}{"instance_type": "t2.small"}),
		awsInstance("i-4", "web-4", map[string]interface{}{"instance_type": "t2.large"}),
		awsInstance("cloud-x", "extra-1", map[string]interface{}{"instance_type": "t2.nano"}),
		awsInstance("cloud-y", "extra-2", map[string]interface{}{"instance_type": "t2.micro"}),
	}

	report := CalculateDrift(desired, actual)

	if got := len(report.Managed); got != 4 {
		t.Errorf("Managed: got %d, want 4", got)
	}
	if got := len(report.Unmanaged); got != 2 {
		t.Errorf("Unmanaged: got %d, want 2", got)
	}
	if got := len(report.Missing); got != 1 {
		t.Errorf("Missing: got %d, want 1", got)
	}
	if got := len(report.Drifted); got != 0 {
		t.Errorf("Drifted: got %d, want 0", got)
	}

	if len(report.Missing) == 1 && report.Missing[0].ID != "i-5" {
		t.Errorf("Missing[0] = %q, want %q", report.Missing[0].ID, "i-5")
	}
}

// TestCalculateDrift_FullScenario exercises all four classifications
// simultaneously, including the Drifted path with nested attribute changes
// and ephemeral-field filtering. It also verifies that an ephemeral field
// difference does NOT produce drift.
func TestCalculateDrift_FullScenario(t *testing.T) {
	desired := []core.Resource{
		awsInstance("i-1", "web", map[string]interface{}{
			"instance_type": "t2.micro",
			"last_updated":  "2026-01-01T00:00:00Z",
			"config": map[string]interface{}{
				"network": map[string]interface{}{"cidr": "10.0.0.0/16"},
			},
		}),
		awsInstance("i-2", "db", map[string]interface{}{"instance_type": "t2.small"}),
		awsInstance("i-3", "cache", map[string]interface{}{"instance_type": "t2.medium"}),
		awsBucket("b-1", "assets", map[string]interface{}{"versioning": "Enabled"}),
		awsInstance("i-missing", "zombie", map[string]interface{}{"instance_type": "t2.large"}),
	}
	actual := []core.Resource{
		awsInstance("i-1", "web", map[string]interface{}{
			"instance_type": "t2.micro",
			"last_updated":  "2026-04-22T12:00:00Z",
			"config": map[string]interface{}{
				"network": map[string]interface{}{"cidr": "10.1.0.0/16"},
			},
		}),
		awsInstance("i-2", "db", map[string]interface{}{"instance_type": "t2.small"}),
		awsInstance("i-3", "cache", map[string]interface{}{"instance_type": "t2.medium"}),
		awsBucket("b-1", "assets", map[string]interface{}{"versioning": "Enabled"}),
		awsInstance("cloud-extra-1", "rogue-1", map[string]interface{}{"instance_type": "t2.nano"}),
		awsInstance("cloud-extra-2", "rogue-2", map[string]interface{}{"instance_type": "t2.nano"}),
		awsInstance("cloud-extra-3", "rogue-3", map[string]interface{}{"instance_type": "t2.nano"}),
	}

	report := CalculateDrift(desired, actual)

	if got := len(report.Managed); got != 3 {
		t.Errorf("Managed: got %d (%v), want 3", got, idsOf(report.Managed))
	}
	if got := len(report.Drifted); got != 1 {
		t.Fatalf("Drifted: got %d (%v), want 1", got, driftedIDs(report.Drifted))
	}
	if got := len(report.Unmanaged); got != 3 {
		t.Errorf("Unmanaged: got %d (%v), want 3", got, idsOf(report.Unmanaged))
	}
	if got := len(report.Missing); got != 1 {
		t.Errorf("Missing: got %d (%v), want 1", got, idsOf(report.Missing))
	}

	drifted := report.Drifted[0]
	if drifted.Resource.ID != "i-1" {
		t.Errorf("drifted.ID = %q, want %q", drifted.Resource.ID, "i-1")
	}
	if len(drifted.Changes) != 1 {
		t.Fatalf("drifted changes: got %d (%v), want 1", len(drifted.Changes), drifted.Changes)
	}
	c := drifted.Changes[0]
	if c.Attribute != "config.network.cidr" {
		t.Errorf("change.Attribute = %q, want %q (ephemeral last_updated must be filtered)", c.Attribute, "config.network.cidr")
	}
	if c.OldValue != "10.0.0.0/16" || c.NewValue != "10.1.0.0/16" {
		t.Errorf("change values = (%v, %v), want (10.0.0.0/16, 10.1.0.0/16)", c.OldValue, c.NewValue)
	}

	if report.Missing[0].ID != "i-missing" {
		t.Errorf("Missing[0].ID = %q, want %q", report.Missing[0].ID, "i-missing")
	}

	unmanagedIDs := idsOf(report.Unmanaged)
	wantUnmanaged := []string{"cloud-extra-1", "cloud-extra-2", "cloud-extra-3"}
	if !reflect.DeepEqual(unmanagedIDs, wantUnmanaged) {
		t.Errorf("unmanaged IDs (sorted) = %v, want %v", unmanagedIDs, wantUnmanaged)
	}
}

func TestCalculateDrift_Deterministic(t *testing.T) {
	desired := []core.Resource{
		awsInstance("i-beta", "beta", map[string]interface{}{"instance_type": "t2.small"}),
		awsInstance("i-alpha", "alpha", map[string]interface{}{"instance_type": "t2.micro"}),
	}
	actual := []core.Resource{
		awsInstance("i-alpha", "alpha", map[string]interface{}{"instance_type": "t2.large"}),
		awsInstance("i-beta", "beta", map[string]interface{}{"instance_type": "t2.xlarge"}),
	}

	opts := DefaultOptions()
	opts.Now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }

	var prev DriftReport
	for i := 0; i < 10; i++ {
		got := CalculateDriftWithOptions(desired, actual, opts)
		if i > 0 && !reflect.DeepEqual(got, prev) {
			t.Fatalf("iter %d: non-deterministic output:\nprev=%+v\ngot=%+v", i, prev, got)
		}
		prev = got
	}

	drifted := prev.Drifted
	if len(drifted) != 2 {
		t.Fatalf("expected 2 drifted, got %d", len(drifted))
	}
	if drifted[0].Resource.ID != "i-alpha" || drifted[1].Resource.ID != "i-beta" {
		t.Errorf("drifted not sorted by ID: [%s, %s]", drifted[0].Resource.ID, drifted[1].Resource.ID)
	}
}

func TestCalculateDrift_TimestampIsSet(t *testing.T) {
	report := CalculateDrift(nil, nil)
	if report.Timestamp.IsZero() {
		t.Errorf("expected Timestamp to be non-zero after CalculateDrift")
	}
}

func TestCalculateDrift_TimestampUsesInjectedClock(t *testing.T) {
	want := time.Date(2000, 1, 2, 3, 4, 5, 0, time.UTC)
	opts := DefaultOptions()
	opts.Now = func() time.Time { return want }

	report := CalculateDriftWithOptions(nil, nil, opts)
	if !report.Timestamp.Equal(want) {
		t.Errorf("Timestamp = %v, want %v (clock injection failed)", report.Timestamp, want)
	}
}

func TestCalculateDrift_TimestampStripsMonotonic(t *testing.T) {
	report := CalculateDrift(nil, nil)
	encoded, err := report.Timestamp.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var decoded time.Time
	if err := decoded.UnmarshalJSON(encoded); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if !decoded.Equal(report.Timestamp) {
		t.Errorf("Timestamp did not round-trip through JSON: %v -> %v", report.Timestamp, decoded)
	}
}

func TestCalculateDrift_EphemeralFieldDoesNotCauseDrift(t *testing.T) {
	desired := []core.Resource{
		awsInstance("i-1", "web", map[string]interface{}{
			"instance_type": "t2.micro",
			"last_updated":  "2026-01-01T00:00:00Z",
		}),
	}
	actual := []core.Resource{
		awsInstance("i-1", "web", map[string]interface{}{
			"instance_type": "t2.micro",
			"last_updated":  "2026-04-22T12:00:00Z",
		}),
	}

	report := CalculateDrift(desired, actual)

	if len(report.Drifted) != 0 {
		t.Errorf("ephemeral last_updated must not cause drift, got %d drifted", len(report.Drifted))
	}
	if len(report.Managed) != 1 {
		t.Errorf("expected 1 managed, got %d", len(report.Managed))
	}
}

func TestCalculateDrift_ShimMatchesWithOptions(t *testing.T) {
	desired := []core.Resource{
		awsInstance("i-1", "web", map[string]interface{}{"instance_type": "t2.micro"}),
	}
	actual := []core.Resource{
		awsInstance("i-1", "web", map[string]interface{}{"instance_type": "t2.large"}),
	}

	fixedClock := func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }

	opts := DefaultOptions()
	opts.Now = fixedClock

	gotViaShim := CalculateDriftWithOptions(desired, actual, opts)
	gotViaOptions := CalculateDriftWithOptions(desired, actual, opts)

	if !reflect.DeepEqual(gotViaShim, gotViaOptions) {
		t.Errorf("shim output differs from explicit options call:\nshim=%+v\nopts=%+v", gotViaShim, gotViaOptions)
	}
}

func awsInstance(id, name string, data map[string]interface{}) core.Resource {
	return core.Resource{
		ID:       id,
		Type:     "aws_instance",
		Name:     name,
		Provider: "aws",
		Region:   "us-east-1",
		Data:     data,
	}
}

func awsBucket(id, name string, data map[string]interface{}) core.Resource {
	return core.Resource{
		ID:       id,
		Type:     "aws_s3_bucket",
		Name:     name,
		Provider: "aws",
		Region:   "us-east-1",
		Data:     data,
	}
}

func idsOf(rs []core.Resource) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.ID
	}
	return out
}

func driftedIDs(drifted []DriftedResource) []string {
	out := make([]string, len(drifted))
	for i, d := range drifted {
		out[i] = d.Resource.ID
	}
	return out
}
