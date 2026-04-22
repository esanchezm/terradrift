package diff

import (
	"sort"
	"testing"

	"github.com/esanchezm/terradrift/internal/core"
)

func collectIDs(rs []core.Resource) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.ID
	}
	sort.Strings(out)
	return out
}

func collectPairIDs(pairs []resourcePair) [][2]string {
	out := make([][2]string, len(pairs))
	for i, p := range pairs {
		out[i] = [2]string{p.Desired.ID, p.Actual.ID}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i][0] != out[j][0] {
			return out[i][0] < out[j][0]
		}
		return out[i][1] < out[j][1]
	})
	return out
}

func TestMatchResources_EmptyInputs(t *testing.T) {
	cases := []struct {
		name                                                 string
		desired, actual                                      []core.Resource
		wantPairs, wantUnmatchedDesired, wantUnmatchedActual int
	}{
		{"nil/nil", nil, nil, 0, 0, 0},
		{"nil/empty", nil, []core.Resource{}, 0, 0, 0},
		{"empty/nil", []core.Resource{}, nil, 0, 0, 0},
		{"empty/empty", []core.Resource{}, []core.Resource{}, 0, 0, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pairs, ud, ua := matchResources(tc.desired, tc.actual)
			if len(pairs) != tc.wantPairs {
				t.Errorf("pairs len = %d, want %d", len(pairs), tc.wantPairs)
			}
			if len(ud) != tc.wantUnmatchedDesired {
				t.Errorf("unmatchedDesired len = %d, want %d", len(ud), tc.wantUnmatchedDesired)
			}
			if len(ua) != tc.wantUnmatchedActual {
				t.Errorf("unmatchedActual len = %d, want %d", len(ua), tc.wantUnmatchedActual)
			}
		})
	}
}

func TestMatchResources_Pass1_ExactID(t *testing.T) {
	desired := []core.Resource{
		{ID: "i-1", Type: "aws_instance", Name: "web", Provider: "aws"},
		{ID: "i-2", Type: "aws_instance", Name: "db", Provider: "aws"},
	}
	actual := []core.Resource{
		{ID: "i-1", Type: "aws_instance", Name: "web", Provider: "aws"},
		{ID: "i-2", Type: "aws_instance", Name: "db", Provider: "aws"},
	}

	pairs, ud, ua := matchResources(desired, actual)
	if len(pairs) != 2 {
		t.Fatalf("expected 2 pairs, got %d", len(pairs))
	}
	if len(ud) != 0 || len(ua) != 0 {
		t.Errorf("expected no unmatched, got ud=%v ua=%v", collectIDs(ud), collectIDs(ua))
	}

	got := collectPairIDs(pairs)
	want := [][2]string{{"i-1", "i-1"}, {"i-2", "i-2"}}
	if !equalPairs(got, want) {
		t.Errorf("pair IDs = %v, want %v", got, want)
	}
}

func TestMatchResources_Pass1_IDMatchRequiresProviderAndType(t *testing.T) {
	desired := []core.Resource{
		{ID: "x", Type: "aws_instance", Provider: "aws"},
	}
	actual := []core.Resource{
		{ID: "x", Type: "google_compute_instance", Provider: "gcp"},
	}

	pairs, ud, ua := matchResources(desired, actual)
	if len(pairs) != 0 {
		t.Errorf("same ID across providers must NOT match, got %d pairs", len(pairs))
	}
	if len(ud) != 1 || len(ua) != 1 {
		t.Errorf("expected 1 unmatched each side, got ud=%d ua=%d", len(ud), len(ua))
	}
}

func TestMatchResources_Pass2_FallbackByName(t *testing.T) {
	desired := []core.Resource{
		{ID: "old-id", Type: "aws_instance", Name: "web", Provider: "aws"},
	}
	actual := []core.Resource{
		{ID: "new-id", Type: "aws_instance", Name: "web", Provider: "aws"},
	}

	pairs, ud, ua := matchResources(desired, actual)
	if len(pairs) != 1 {
		t.Fatalf("expected 1 pair via name fallback, got %d", len(pairs))
	}
	if pairs[0].Desired.ID != "old-id" || pairs[0].Actual.ID != "new-id" {
		t.Errorf("pair IDs = (%q,%q), want (old-id,new-id)", pairs[0].Desired.ID, pairs[0].Actual.ID)
	}
	if len(ud) != 0 || len(ua) != 0 {
		t.Errorf("expected no unmatched, got ud=%v ua=%v", ud, ua)
	}
}

func TestMatchResources_Pass2_EmptyNameSkipsMatch(t *testing.T) {
	desired := []core.Resource{
		{ID: "old-id", Type: "aws_instance", Name: "", Provider: "aws"},
	}
	actual := []core.Resource{
		{ID: "new-id", Type: "aws_instance", Name: "", Provider: "aws"},
	}

	pairs, ud, ua := matchResources(desired, actual)
	if len(pairs) != 0 {
		t.Errorf("empty Name must not match empty Name, got %d pairs", len(pairs))
	}
	if len(ud) != 1 || len(ua) != 1 {
		t.Errorf("both should be unmatched, got ud=%d ua=%d", len(ud), len(ua))
	}
}

func TestMatchResources_Pass3_FallbackByTagName(t *testing.T) {
	desired := []core.Resource{
		{ID: "old-id", Type: "aws_instance", Name: "old-name", Provider: "aws", Data: map[string]interface{}{
			"tags": map[string]interface{}{"Name": "production-db"},
		}},
	}
	actual := []core.Resource{
		{ID: "new-id", Type: "aws_instance", Name: "new-name", Provider: "aws", Data: map[string]interface{}{
			"tags": map[string]interface{}{"Name": "production-db"},
		}},
	}

	pairs, ud, ua := matchResources(desired, actual)
	if len(pairs) != 1 {
		t.Fatalf("expected 1 pair via tag Name fallback, got %d (ud=%v ua=%v)", len(pairs), ud, ua)
	}
	if pairs[0].Desired.ID != "old-id" || pairs[0].Actual.ID != "new-id" {
		t.Errorf("pair IDs = (%q,%q), want (old-id,new-id)", pairs[0].Desired.ID, pairs[0].Actual.ID)
	}
}

func TestMatchResources_Pass3_TagShapeVariance(t *testing.T) {
	desired := []core.Resource{
		{ID: "d-1", Type: "aws_instance", Name: "d-name", Provider: "aws", Data: map[string]interface{}{
			"tags": map[string]string{"Name": "web"},
		}},
	}
	actual := []core.Resource{
		{ID: "a-1", Type: "aws_instance", Name: "a-name", Provider: "aws", Data: map[string]interface{}{
			"tags": map[string]interface{}{"Name": "web"},
		}},
	}

	pairs, _, _ := matchResources(desired, actual)
	if len(pairs) != 1 {
		t.Errorf("pass 3 must accept both tag shapes, got %d pairs", len(pairs))
	}
}

func TestMatchResources_Pass3_MissingTagsSkipsMatch(t *testing.T) {
	desired := []core.Resource{
		{ID: "d-1", Type: "aws_instance", Name: "d-name", Provider: "aws", Data: map[string]interface{}{}},
	}
	actual := []core.Resource{
		{ID: "a-1", Type: "aws_instance", Name: "a-name", Provider: "aws", Data: map[string]interface{}{}},
	}

	pairs, ud, ua := matchResources(desired, actual)
	if len(pairs) != 0 {
		t.Errorf("no tags.Name on either side must not match, got %d pairs", len(pairs))
	}
	if len(ud) != 1 || len(ua) != 1 {
		t.Errorf("expected 1 unmatched each, got ud=%d ua=%d", len(ud), len(ua))
	}
}

func TestMatchResources_Pass3_EmptyTagNameSkipsMatch(t *testing.T) {
	desired := []core.Resource{
		{ID: "d-1", Type: "aws_instance", Name: "d-name", Provider: "aws", Data: map[string]interface{}{
			"tags": map[string]interface{}{"Name": ""},
		}},
	}
	actual := []core.Resource{
		{ID: "a-1", Type: "aws_instance", Name: "a-name", Provider: "aws", Data: map[string]interface{}{
			"tags": map[string]interface{}{"Name": ""},
		}},
	}

	pairs, _, _ := matchResources(desired, actual)
	if len(pairs) != 0 {
		t.Errorf("empty tag Name must not match empty tag Name, got %d pairs", len(pairs))
	}
}

func TestMatchResources_Pass2_AmbiguityNoMatch(t *testing.T) {
	desired := []core.Resource{
		{ID: "d-1", Type: "aws_instance", Name: "web", Provider: "aws"},
		{ID: "d-2", Type: "aws_instance", Name: "web", Provider: "aws"},
	}
	actual := []core.Resource{
		{ID: "a-1", Type: "aws_instance", Name: "web", Provider: "aws"},
	}

	pairs, ud, ua := matchResources(desired, actual)
	if len(pairs) != 0 {
		t.Errorf("ambiguous Name must not match, got %d pairs", len(pairs))
	}
	if len(ud) != 2 || len(ua) != 1 {
		t.Errorf("expected ud=2 ua=1, got ud=%d ua=%d", len(ud), len(ua))
	}
}

func TestMatchResources_Pass3_AmbiguityNoMatch(t *testing.T) {
	desired := []core.Resource{
		{ID: "d-1", Type: "aws_instance", Name: "d1", Provider: "aws", Data: map[string]interface{}{
			"tags": map[string]interface{}{"Name": "shared"},
		}},
		{ID: "d-2", Type: "aws_instance", Name: "d2", Provider: "aws", Data: map[string]interface{}{
			"tags": map[string]interface{}{"Name": "shared"},
		}},
	}
	actual := []core.Resource{
		{ID: "a-1", Type: "aws_instance", Name: "a1", Provider: "aws", Data: map[string]interface{}{
			"tags": map[string]interface{}{"Name": "shared"},
		}},
		{ID: "a-2", Type: "aws_instance", Name: "a2", Provider: "aws", Data: map[string]interface{}{
			"tags": map[string]interface{}{"Name": "shared"},
		}},
	}

	pairs, ud, ua := matchResources(desired, actual)
	if len(pairs) != 0 {
		t.Errorf("ambiguous tag Name must not match, got %d pairs", len(pairs))
	}
	if len(ud) != 2 || len(ua) != 2 {
		t.Errorf("expected ud=2 ua=2, got ud=%d ua=%d", len(ud), len(ua))
	}
}

func TestMatchResources_Pass1WinsOverPass2(t *testing.T) {
	desired := []core.Resource{
		{ID: "same-id", Type: "aws_instance", Name: "web", Provider: "aws"},
		{ID: "d-2", Type: "aws_instance", Name: "web", Provider: "aws"},
	}
	actual := []core.Resource{
		{ID: "same-id", Type: "aws_instance", Name: "web", Provider: "aws"},
	}

	pairs, ud, ua := matchResources(desired, actual)
	if len(pairs) != 1 {
		t.Fatalf("expected 1 pair (pass 1 wins), got %d", len(pairs))
	}
	if pairs[0].Desired.ID != "same-id" || pairs[0].Actual.ID != "same-id" {
		t.Errorf("pass 1 must win: got pair (%q,%q)", pairs[0].Desired.ID, pairs[0].Actual.ID)
	}
	if len(ud) != 1 || ud[0].ID != "d-2" {
		t.Errorf("d-2 should be unmatched after pass 1 consumed same-id pairing, ud=%v", collectIDs(ud))
	}
	if len(ua) != 0 {
		t.Errorf("no unmatched actual expected, got %v", collectIDs(ua))
	}
}

func TestMatchResources_FullMultiPassScenario(t *testing.T) {
	desired := []core.Resource{
		{ID: "i-1", Type: "aws_instance", Name: "web-1", Provider: "aws"},
		{ID: "i-2", Type: "aws_instance", Name: "web-2", Provider: "aws"},
		{ID: "old-id", Type: "aws_s3_bucket", Name: "bucket-a", Provider: "aws"},
		{ID: "d-tag", Type: "aws_security_group", Name: "sg-d", Provider: "aws", Data: map[string]interface{}{
			"tags": map[string]interface{}{"Name": "shared-sg"},
		}},
		{ID: "i-5", Type: "aws_iam_role", Name: "role-x", Provider: "aws"},
	}
	actual := []core.Resource{
		{ID: "i-1", Type: "aws_instance", Name: "web-1", Provider: "aws"},
		{ID: "i-2", Type: "aws_instance", Name: "web-2", Provider: "aws"},
		{ID: "new-id", Type: "aws_s3_bucket", Name: "bucket-a", Provider: "aws"},
		{ID: "a-tag", Type: "aws_security_group", Name: "sg-a", Provider: "aws", Data: map[string]interface{}{
			"tags": map[string]interface{}{"Name": "shared-sg"},
		}},
		{ID: "cloud-only", Type: "aws_instance", Name: "cloud-one", Provider: "aws"},
	}

	pairs, ud, ua := matchResources(desired, actual)

	if len(pairs) != 4 {
		t.Fatalf("expected 4 pairs, got %d (ud=%v ua=%v)", len(pairs), collectIDs(ud), collectIDs(ua))
	}
	if len(ud) != 1 || ud[0].ID != "i-5" {
		t.Errorf("expected unmatched desired=[i-5], got %v", collectIDs(ud))
	}
	if len(ua) != 1 || ua[0].ID != "cloud-only" {
		t.Errorf("expected unmatched actual=[cloud-only], got %v", collectIDs(ua))
	}

	wantPairs := [][2]string{
		{"d-tag", "a-tag"},
		{"i-1", "i-1"},
		{"i-2", "i-2"},
		{"old-id", "new-id"},
	}
	got := collectPairIDs(pairs)
	if !equalPairs(got, wantPairs) {
		t.Errorf("pairs = %v, want %v", got, wantPairs)
	}
}

func TestMatchResources_ResourceMatchedOnlyOnce(t *testing.T) {
	desired := []core.Resource{
		{ID: "d-1", Type: "aws_instance", Name: "web", Provider: "aws", Data: map[string]interface{}{
			"tags": map[string]interface{}{"Name": "web-tag"},
		}},
	}
	actual := []core.Resource{
		{ID: "a-1", Type: "aws_instance", Name: "web", Provider: "aws", Data: map[string]interface{}{
			"tags": map[string]interface{}{"Name": "web-tag"},
		}},
	}

	pairs, ud, ua := matchResources(desired, actual)
	if len(pairs) != 1 {
		t.Fatalf("expected 1 pair (single match), got %d", len(pairs))
	}
	if len(ud) != 0 || len(ua) != 0 {
		t.Errorf("once matched, must not appear in unmatched lists: ud=%d ua=%d", len(ud), len(ua))
	}
}

func TestMatchResources_TypeMismatch(t *testing.T) {
	desired := []core.Resource{
		{ID: "x", Type: "aws_instance", Name: "web", Provider: "aws"},
	}
	actual := []core.Resource{
		{ID: "x", Type: "aws_s3_bucket", Name: "web", Provider: "aws"},
	}

	pairs, ud, ua := matchResources(desired, actual)
	if len(pairs) != 0 {
		t.Errorf("different Type must not match, got %d pairs", len(pairs))
	}
	if len(ud) != 1 || len(ua) != 1 {
		t.Errorf("both should be unmatched, ud=%d ua=%d", len(ud), len(ua))
	}
}

func TestMatchResources_Determinism(t *testing.T) {
	desired := []core.Resource{
		{ID: "i-3", Type: "aws_instance", Name: "c", Provider: "aws"},
		{ID: "i-1", Type: "aws_instance", Name: "a", Provider: "aws"},
		{ID: "i-2", Type: "aws_instance", Name: "b", Provider: "aws"},
	}
	actual := []core.Resource{
		{ID: "i-2", Type: "aws_instance", Name: "b", Provider: "aws"},
		{ID: "i-1", Type: "aws_instance", Name: "a", Provider: "aws"},
		{ID: "i-3", Type: "aws_instance", Name: "c", Provider: "aws"},
	}

	var prev [][2]string
	for i := 0; i < 20; i++ {
		pairs, _, _ := matchResources(desired, actual)
		got := collectPairIDs(pairs)
		if i > 0 && !equalPairs(got, prev) {
			t.Fatalf("iter %d: non-deterministic output: %v vs %v", i, prev, got)
		}
		prev = got
	}
}

func equalPairs(a, b [][2]string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
