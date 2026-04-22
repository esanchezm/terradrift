package diff

import (
	"reflect"
	"sort"
	"testing"

	"github.com/esanchezm/terradrift/internal/core"
)

func TestDiffResources_NoChangesWhenIdentical(t *testing.T) {
	res := core.Resource{
		ID:       "i-1",
		Type:     "aws_instance",
		Name:     "web",
		Provider: "aws",
		Data: map[string]interface{}{
			"instance_type": "t2.micro",
			"tags":          map[string]interface{}{"Name": "web"},
		},
	}

	got := diffResources(res, res, DefaultOptions())
	if len(got) != 0 {
		t.Fatalf("expected no changes for identical resources, got %v", got)
	}
}

func TestDiffResources_SimpleScalarChange(t *testing.T) {
	desired := core.Resource{
		ID: "i-1", Type: "aws_instance", Name: "web", Provider: "aws",
		Data: map[string]interface{}{"instance_type": "t2.micro"},
	}
	actual := core.Resource{
		ID: "i-1", Type: "aws_instance", Name: "web", Provider: "aws",
		Data: map[string]interface{}{"instance_type": "t2.small"},
	}

	got := diffResources(desired, actual, DefaultOptions())
	if len(got) != 1 {
		t.Fatalf("expected 1 change, got %d: %v", len(got), got)
	}
	c := got[0]
	if c.Attribute != "instance_type" {
		t.Errorf("attribute = %q, want %q", c.Attribute, "instance_type")
	}
	if c.ChangeType != ChangeTypeUpdate {
		t.Errorf("changeType = %q, want %q", c.ChangeType, ChangeTypeUpdate)
	}
	if c.OldValue != "t2.micro" {
		t.Errorf("oldValue = %v, want %v", c.OldValue, "t2.micro")
	}
	if c.NewValue != "t2.small" {
		t.Errorf("newValue = %v, want %v", c.NewValue, "t2.small")
	}
	if c.ResourceID != "i-1" || c.ResourceType != "aws_instance" || c.ResourceName != "web" {
		t.Errorf("change missing resource metadata: %+v", c)
	}
}

func TestDiffResources_NumericNormalization(t *testing.T) {
	cases := []struct {
		name string
		a, b interface{}
	}{
		{"int vs float64", 42, float64(42)},
		{"int8 vs float64", int8(42), float64(42)},
		{"int16 vs float64", int16(42), float64(42)},
		{"int32 vs float64", int32(42), float64(42)},
		{"int64 vs float64", int64(42), float64(42)},
		{"uint vs float64", uint(42), float64(42)},
		{"uint8 vs float64", uint8(42), float64(42)},
		{"uint32 vs float64", uint32(42), float64(42)},
		{"uint64 vs float64", uint64(42), float64(42)},
		{"float32 vs float64", float32(42), float64(42)},
		{"int vs int32", 42, int32(42)},
		{"int64 vs int32", int64(42), int32(42)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			desired := core.Resource{ID: "r", Data: map[string]interface{}{"port": tc.a}}
			actual := core.Resource{ID: "r", Data: map[string]interface{}{"port": tc.b}}
			got := diffResources(desired, actual, DefaultOptions())
			if len(got) != 0 {
				t.Errorf("expected 0 changes for %v vs %v, got %v", tc.a, tc.b, got)
			}
		})
	}
}

func TestDiffResources_NilVsEmptyCollections(t *testing.T) {
	cases := []struct {
		name string
		a, b interface{}
	}{
		{"nil slice vs empty []string", nil, []string{}},
		{"nil slice vs empty []interface{}", nil, []interface{}{}},
		{"empty []string vs nil slice", []string{}, nil},
		{"nil map vs empty map[string]interface{}", nil, map[string]interface{}{}},
		{"empty map vs nil", map[string]interface{}{}, nil},
		{"empty map[string]string vs empty map[string]interface{}", map[string]string{}, map[string]interface{}{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			desired := core.Resource{ID: "r", Data: map[string]interface{}{"tags": tc.a}}
			actual := core.Resource{ID: "r", Data: map[string]interface{}{"tags": tc.b}}
			got := diffResources(desired, actual, DefaultOptions())
			if len(got) != 0 {
				t.Errorf("expected 0 changes, got %v", got)
			}
		})
	}
}

func TestDiffResources_MixedSliceTypes(t *testing.T) {
	desired := core.Resource{ID: "r", Data: map[string]interface{}{
		"sgs": []interface{}{"sg-1", "sg-2"},
	}}
	actual := core.Resource{ID: "r", Data: map[string]interface{}{
		"sgs": []string{"sg-1", "sg-2"},
	}}

	got := diffResources(desired, actual, DefaultOptions())
	if len(got) != 0 {
		t.Errorf("[]interface{} vs []string with same elements should match, got %v", got)
	}
}

func TestDiffResources_NestedMapPath(t *testing.T) {
	desired := core.Resource{ID: "r", Type: "aws_instance", Name: "web", Data: map[string]interface{}{
		"config": map[string]interface{}{
			"network": map[string]interface{}{
				"cidr": "10.0.0.0/16",
			},
		},
	}}
	actual := core.Resource{ID: "r", Type: "aws_instance", Name: "web", Data: map[string]interface{}{
		"config": map[string]interface{}{
			"network": map[string]interface{}{
				"cidr": "10.1.0.0/16",
			},
		},
	}}

	got := diffResources(desired, actual, DefaultOptions())
	if len(got) != 1 {
		t.Fatalf("expected 1 change, got %d: %v", len(got), got)
	}
	if got[0].Attribute != "config.network.cidr" {
		t.Errorf("attribute = %q, want %q", got[0].Attribute, "config.network.cidr")
	}
}

func TestDiffResources_IntersectionOnlyKeys(t *testing.T) {
	desired := core.Resource{ID: "r", Data: map[string]interface{}{
		"id":            "i-abc",
		"instance_type": "t2.micro",
		"state_only":    "only-in-state",
	}}
	actual := core.Resource{ID: "r", Data: map[string]interface{}{
		"instance_id":   "i-abc",
		"instance_type": "t2.micro",
		"cloud_only":    "only-in-cloud",
	}}

	got := diffResources(desired, actual, DefaultOptions())
	if len(got) != 0 {
		t.Errorf("intersection-only diff should ignore one-sided keys, got %v", got)
	}
}

func TestDiffResources_IntersectionWithDrift(t *testing.T) {
	desired := core.Resource{ID: "r", Data: map[string]interface{}{
		"instance_type": "t2.micro",
		"state_only":    "x",
	}}
	actual := core.Resource{ID: "r", Data: map[string]interface{}{
		"instance_type": "t2.large",
		"cloud_only":    "y",
	}}

	got := diffResources(desired, actual, DefaultOptions())
	if len(got) != 1 {
		t.Fatalf("expected 1 change (instance_type only), got %d: %v", len(got), got)
	}
	if got[0].Attribute != "instance_type" {
		t.Errorf("attribute = %q, want %q", got[0].Attribute, "instance_type")
	}
}

func TestDiffResources_SliceSortEnabled(t *testing.T) {
	desired := core.Resource{ID: "r", Data: map[string]interface{}{
		"sgs": []interface{}{"sg-a", "sg-b", "sg-c"},
	}}
	actual := core.Resource{ID: "r", Data: map[string]interface{}{
		"sgs": []interface{}{"sg-c", "sg-a", "sg-b"},
	}}

	opts := DefaultOptions()
	opts.SortLists = true
	got := diffResources(desired, actual, opts)
	if len(got) != 0 {
		t.Errorf("sorted slice compare should match, got %v", got)
	}
}

func TestDiffResources_SliceSortDisabled(t *testing.T) {
	desired := core.Resource{ID: "r", Data: map[string]interface{}{
		"sgs": []interface{}{"sg-a", "sg-b", "sg-c"},
	}}
	actual := core.Resource{ID: "r", Data: map[string]interface{}{
		"sgs": []interface{}{"sg-c", "sg-a", "sg-b"},
	}}

	opts := DefaultOptions()
	opts.SortLists = false
	got := diffResources(desired, actual, opts)
	if len(got) == 0 {
		t.Errorf("unsorted slice compare with different order should emit change")
	}
}

func TestDiffResources_MixedTypeSliceDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected panic on mixed-type slice: %v", r)
		}
	}()

	desired := core.Resource{ID: "r", Data: map[string]interface{}{
		"mixed": []interface{}{1, "two", true, 3.14},
	}}
	actual := core.Resource{ID: "r", Data: map[string]interface{}{
		"mixed": []interface{}{1, "two", true, 3.14},
	}}

	opts := DefaultOptions()
	opts.SortLists = true
	got := diffResources(desired, actual, opts)
	if len(got) != 0 {
		t.Errorf("identical mixed slice should produce no changes, got %v", got)
	}
}

func TestDiffResources_SliceLengthMismatch(t *testing.T) {
	desired := core.Resource{ID: "r", Data: map[string]interface{}{
		"sgs": []interface{}{"sg-a"},
	}}
	actual := core.Resource{ID: "r", Data: map[string]interface{}{
		"sgs": []interface{}{"sg-a", "sg-b"},
	}}

	got := diffResources(desired, actual, DefaultOptions())
	if len(got) != 1 {
		t.Fatalf("expected 1 change for length mismatch, got %d: %v", len(got), got)
	}
	if got[0].Attribute != "sgs" {
		t.Errorf("attribute = %q, want %q", got[0].Attribute, "sgs")
	}
}

func TestDiffResources_CaseInsensitiveEnabled(t *testing.T) {
	desired := core.Resource{ID: "r", Data: map[string]interface{}{
		"state": "Enabled",
	}}
	actual := core.Resource{ID: "r", Data: map[string]interface{}{
		"state": "enabled",
	}}

	opts := DefaultOptions()
	opts.CaseInsensitive = true
	got := diffResources(desired, actual, opts)
	if len(got) != 0 {
		t.Errorf("case-insensitive compare should match %q and %q, got %v", "Enabled", "enabled", got)
	}
}

func TestDiffResources_CaseSensitiveByDefault(t *testing.T) {
	desired := core.Resource{ID: "r", Data: map[string]interface{}{
		"state": "Enabled",
	}}
	actual := core.Resource{ID: "r", Data: map[string]interface{}{
		"state": "enabled",
	}}

	got := diffResources(desired, actual, DefaultOptions())
	if len(got) != 1 {
		t.Errorf("case-sensitive compare should differ for %q and %q, got %v", "Enabled", "enabled", got)
	}
}

func TestDiffResources_CaseInsensitiveDoesNotAffectKeys(t *testing.T) {
	desired := core.Resource{ID: "r", Data: map[string]interface{}{
		"Name": "web",
	}}
	actual := core.Resource{ID: "r", Data: map[string]interface{}{
		"name": "web",
	}}

	opts := DefaultOptions()
	opts.CaseInsensitive = true
	got := diffResources(desired, actual, opts)
	if len(got) != 0 {
		t.Errorf("keys differ only by case; intersection is empty, expected 0 changes, got %v", got)
	}
}

func TestDiffResources_EphemeralFilterTopLevel(t *testing.T) {
	desired := core.Resource{ID: "r", Data: map[string]interface{}{
		"last_updated":  "2026-01-01T00:00:00Z",
		"instance_type": "t2.micro",
	}}
	actual := core.Resource{ID: "r", Data: map[string]interface{}{
		"last_updated":  "2026-04-22T00:00:00Z",
		"instance_type": "t2.micro",
	}}

	got := diffResources(desired, actual, DefaultOptions())
	if len(got) != 0 {
		t.Errorf("ephemeral last_updated should not produce drift, got %v", got)
	}
}

func TestDiffResources_EphemeralFilterNested(t *testing.T) {
	desired := core.Resource{ID: "r", Data: map[string]interface{}{
		"meta": map[string]interface{}{
			"last_updated": "2026-01-01T00:00:00Z",
			"region":       "us-east-1",
		},
	}}
	actual := core.Resource{ID: "r", Data: map[string]interface{}{
		"meta": map[string]interface{}{
			"last_updated": "2026-04-22T00:00:00Z",
			"region":       "us-east-1",
		},
	}}

	got := diffResources(desired, actual, DefaultOptions())
	if len(got) != 0 {
		t.Errorf("ephemeral last_updated at nested path should not produce drift, got %v", got)
	}
}

func TestDiffResources_CustomIgnoredAttribute(t *testing.T) {
	desired := core.Resource{ID: "r", Data: map[string]interface{}{
		"arn":           "arn:aws:ec2:us-east-1:123:instance/i-abc",
		"instance_type": "t2.micro",
	}}
	actual := core.Resource{ID: "r", Data: map[string]interface{}{
		"arn":           "arn:aws:ec2:us-east-1:456:instance/i-xyz",
		"instance_type": "t2.micro",
	}}

	opts := DefaultOptions()
	opts.IgnoredAttributes = []string{"arn"}
	got := diffResources(desired, actual, opts)
	if len(got) != 0 {
		t.Errorf("custom ignored arn should suppress drift, got %v", got)
	}
}

func TestDiffResources_CustomIgnoredAttributeNested(t *testing.T) {
	desired := core.Resource{ID: "r", Data: map[string]interface{}{
		"meta": map[string]interface{}{"arn": "arn:x"},
	}}
	actual := core.Resource{ID: "r", Data: map[string]interface{}{
		"meta": map[string]interface{}{"arn": "arn:y"},
	}}

	opts := DefaultOptions()
	opts.IgnoredAttributes = []string{"arn"}
	got := diffResources(desired, actual, opts)
	if len(got) != 0 {
		t.Errorf("custom ignored arn at nested path should suppress drift, got %v", got)
	}
}

func TestDiffResources_TagShapeVariance(t *testing.T) {
	desired := core.Resource{ID: "r", Data: map[string]interface{}{
		"tags": map[string]interface{}{"Name": "web", "Env": "prod"},
	}}
	actual := core.Resource{ID: "r", Data: map[string]interface{}{
		"tags": map[string]string{"Name": "web", "Env": "prod"},
	}}

	got := diffResources(desired, actual, DefaultOptions())
	if len(got) != 0 {
		t.Errorf("map[string]interface{} and map[string]string with same content should match, got %v", got)
	}
}

func TestDiffResources_DeterministicOrder(t *testing.T) {
	desired := core.Resource{ID: "r", Type: "aws_instance", Name: "web", Data: map[string]interface{}{
		"zebra":  "z1",
		"alpha":  "a1",
		"middle": "m1",
		"beta":   "b1",
	}}
	actual := core.Resource{ID: "r", Type: "aws_instance", Name: "web", Data: map[string]interface{}{
		"zebra":  "z2",
		"alpha":  "a2",
		"middle": "m2",
		"beta":   "b2",
	}}

	opts := DefaultOptions()
	var prev []Change
	for i := 0; i < 20; i++ {
		got := diffResources(desired, actual, opts)
		if len(got) != 4 {
			t.Fatalf("iter %d: expected 4 changes, got %d", i, len(got))
		}
		if i > 0 {
			if !reflect.DeepEqual(prev, got) {
				t.Fatalf("non-deterministic output across runs: prev=%v got=%v", prev, got)
			}
		}
		prev = got
	}

	wantOrder := []string{"alpha", "beta", "middle", "zebra"}
	gotOrder := make([]string, len(prev))
	for i, c := range prev {
		gotOrder[i] = c.Attribute
	}
	if !sort.StringsAreSorted(gotOrder) {
		t.Errorf("changes not sorted by attribute: %v", gotOrder)
	}
	if !reflect.DeepEqual(gotOrder, wantOrder) {
		t.Errorf("sort order = %v, want %v", gotOrder, wantOrder)
	}
}

func TestDiffResources_NilDataOnOneSide(t *testing.T) {
	desired := core.Resource{ID: "r", Data: nil}
	actual := core.Resource{ID: "r", Data: map[string]interface{}{"instance_type": "t2.micro"}}

	got := diffResources(desired, actual, DefaultOptions())
	if len(got) != 0 {
		t.Errorf("intersection of nil and non-nil map is empty, expected 0 changes, got %v", got)
	}
}

func TestDiffResources_NilDataOnBothSides(t *testing.T) {
	desired := core.Resource{ID: "r", Data: nil}
	actual := core.Resource{ID: "r", Data: nil}

	got := diffResources(desired, actual, DefaultOptions())
	if len(got) != 0 {
		t.Errorf("nil vs nil data should produce no changes, got %v", got)
	}
}

func TestDiffResources_NestedSliceOfMaps(t *testing.T) {
	desired := core.Resource{ID: "r", Data: map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{"port": float64(80), "proto": "tcp"},
			map[string]interface{}{"port": float64(443), "proto": "tcp"},
		},
	}}
	actual := core.Resource{ID: "r", Data: map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{"port": float64(80), "proto": "tcp"},
			map[string]interface{}{"port": float64(8443), "proto": "tcp"},
		},
	}}

	got := diffResources(desired, actual, DefaultOptions())
	if len(got) != 1 {
		t.Fatalf("expected 1 change in nested slice-of-maps, got %d: %v", len(got), got)
	}
	if got[0].Attribute != "rules.1.port" {
		t.Errorf("attribute = %q, want %q", got[0].Attribute, "rules.1.port")
	}
}

func TestDiffResources_BooleanComparison(t *testing.T) {
	desired := core.Resource{ID: "r", Data: map[string]interface{}{"versioning": true}}
	actual := core.Resource{ID: "r", Data: map[string]interface{}{"versioning": false}}

	got := diffResources(desired, actual, DefaultOptions())
	if len(got) != 1 {
		t.Fatalf("expected 1 change for bool diff, got %d", len(got))
	}
}

func TestDiffResources_StringVsBoolNoCoercion(t *testing.T) {
	desired := core.Resource{ID: "r", Data: map[string]interface{}{"flag": "true"}}
	actual := core.Resource{ID: "r", Data: map[string]interface{}{"flag": true}}

	got := diffResources(desired, actual, DefaultOptions())
	if len(got) != 1 {
		t.Errorf("string %q and bool true should differ (no type coercion), got %v", "true", got)
	}
}

func TestDiffResources_CaseInsensitiveStringSliceReordered(t *testing.T) {
	desired := core.Resource{ID: "r", Data: map[string]interface{}{
		"sgs": []interface{}{"B", "a"},
	}}
	actual := core.Resource{ID: "r", Data: map[string]interface{}{
		"sgs": []interface{}{"A", "b"},
	}}

	opts := DefaultOptions()
	opts.CaseInsensitive = true
	opts.SortLists = true

	got := diffResources(desired, actual, opts)
	if len(got) != 0 {
		t.Errorf("case-insensitive compare of reordered string slices should match, got %v", got)
	}
}

func TestDiffResources_CaseInsensitiveStringSliceWithSortDisabled(t *testing.T) {
	desired := core.Resource{ID: "r", Data: map[string]interface{}{
		"sgs": []interface{}{"A", "b"},
	}}
	actual := core.Resource{ID: "r", Data: map[string]interface{}{
		"sgs": []interface{}{"a", "B"},
	}}

	opts := DefaultOptions()
	opts.CaseInsensitive = true
	opts.SortLists = false

	got := diffResources(desired, actual, opts)
	if len(got) != 0 {
		t.Errorf("element-wise case-insensitive compare should match for %v and %v, got %v",
			desired.Data["sgs"], actual.Data["sgs"], got)
	}
}

func TestDiffResources_LargeInt64PrecisionDetectsDrift(t *testing.T) {
	desired := core.Resource{ID: "r", Data: map[string]interface{}{
		"size": int64(9007199254740993),
	}}
	actual := core.Resource{ID: "r", Data: map[string]interface{}{
		"size": int64(9007199254740995),
	}}

	got := diffResources(desired, actual, DefaultOptions())
	if len(got) != 1 {
		t.Fatalf("large int64 values differing by 2 must report drift (not lost to float64 rounding), got %v", got)
	}
}

func TestDiffResources_LargeUint64PrecisionDetectsDrift(t *testing.T) {
	desired := core.Resource{ID: "r", Data: map[string]interface{}{
		"count": uint64(18446744073709551613),
	}}
	actual := core.Resource{ID: "r", Data: map[string]interface{}{
		"count": uint64(18446744073709551615),
	}}

	got := diffResources(desired, actual, DefaultOptions())
	if len(got) != 1 {
		t.Fatalf("large uint64 values differing by 2 must report drift, got %v", got)
	}
}

func TestDiffResources_LargeInt64ExactEqualityAcrossInt32Path(t *testing.T) {
	desired := core.Resource{ID: "r", Data: map[string]interface{}{
		"size": int64(9007199254740993),
	}}
	actual := core.Resource{ID: "r", Data: map[string]interface{}{
		"size": int64(9007199254740993),
	}}

	got := diffResources(desired, actual, DefaultOptions())
	if len(got) != 0 {
		t.Errorf("identical int64 values must compare equal without loss, got %v", got)
	}
}

func TestDiffResources_NegativeIntVsUintNeverEqual(t *testing.T) {
	desired := core.Resource{ID: "r", Data: map[string]interface{}{
		"offset": int64(-1),
	}}
	actual := core.Resource{ID: "r", Data: map[string]interface{}{
		"offset": uint64(18446744073709551615),
	}}

	got := diffResources(desired, actual, DefaultOptions())
	if len(got) != 1 {
		t.Errorf("negative int64 must never equal uint64 max, got %v", got)
	}
}

func TestDiffResources_NumericSliceSortsAcrossMixedIntTypes(t *testing.T) {
	desired := core.Resource{ID: "r", Data: map[string]interface{}{
		"ports": []interface{}{int32(443), int64(22), int(80)},
	}}
	actual := core.Resource{ID: "r", Data: map[string]interface{}{
		"ports": []interface{}{int(22), int64(80), int32(443)},
	}}

	opts := DefaultOptions()
	opts.SortLists = true

	got := diffResources(desired, actual, opts)
	if len(got) != 0 {
		t.Errorf("mixed-int-type numeric slice should match after sort, got %v", got)
	}
}

func TestDiffResources_LargeIntSliceReorderedUsesExactSort(t *testing.T) {
	desired := core.Resource{ID: "r", Data: map[string]interface{}{
		"values": []interface{}{int64(9007199254740992), int64(9007199254740993)},
	}}
	actual := core.Resource{ID: "r", Data: map[string]interface{}{
		"values": []interface{}{int64(9007199254740993), int64(9007199254740992)},
	}}

	opts := DefaultOptions()
	opts.SortLists = true

	for i := 0; i < 10; i++ {
		got := diffResources(desired, actual, opts)
		if len(got) != 0 {
			t.Fatalf("iter %d: reordered large int64 slice must sort exactly and compare equal, got %v", i, got)
		}
	}
}

func TestDiffResources_LargeUintSliceReorderedUsesExactSort(t *testing.T) {
	desired := core.Resource{ID: "r", Data: map[string]interface{}{
		"values": []interface{}{uint64(18446744073709551613), uint64(18446744073709551615)},
	}}
	actual := core.Resource{ID: "r", Data: map[string]interface{}{
		"values": []interface{}{uint64(18446744073709551615), uint64(18446744073709551613)},
	}}

	opts := DefaultOptions()
	opts.SortLists = true

	for i := 0; i < 10; i++ {
		got := diffResources(desired, actual, opts)
		if len(got) != 0 {
			t.Fatalf("iter %d: reordered large uint64 slice must sort exactly, got %v", i, got)
		}
	}
}

func TestDiffResources_LargeIntSliceDifferentValuesStillDriftsAfterSort(t *testing.T) {
	desired := core.Resource{ID: "r", Data: map[string]interface{}{
		"values": []interface{}{int64(9007199254740992), int64(9007199254740993)},
	}}
	actual := core.Resource{ID: "r", Data: map[string]interface{}{
		"values": []interface{}{int64(9007199254740992), int64(9007199254740995)},
	}}

	opts := DefaultOptions()
	opts.SortLists = true

	got := diffResources(desired, actual, opts)
	if len(got) == 0 {
		t.Errorf("distinct large int64 values must still be detectable after sort, got no changes")
	}
}
