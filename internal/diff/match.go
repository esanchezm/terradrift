package diff

import (
	"sort"

	"github.com/esanchezm/terradrift/internal/core"
)

// resourcePair pairs a state-side (desired) resource with its cloud-side
// (actual) counterpart.
type resourcePair struct {
	Desired core.Resource
	Actual  core.Resource
}

// matchResources pairs each desired resource with its corresponding actual
// resource using a three-pass strategy and returns the pairs alongside the
// unmatched remainders.
//
// The passes are tried in order; each pass only considers resources not yet
// matched by an earlier pass:
//
//  1. Exact match on (Provider, Type, ID). This is the cheapest and most
//     reliable signal; when state and cloud agree on the provider-native
//     identifier, they refer to the same object.
//  2. Fallback match on (Provider, Type, Name) when Name is non-empty on
//     both sides. Useful when the provider changed IDs (rare but real, e.g.
//     a recreated resource).
//  3. Fallback match on (Provider, Type, tags["Name"]) when present and
//     non-empty. Both tag map shapes (map[string]string and
//     map[string]interface{}) are accepted.
//
// Within any pass, a (key -> resources) group is considered ambiguous when
// either side of the group contains more than one resource; ambiguous
// groups are skipped and their members fall through to subsequent passes
// or to the unmatched output.
//
// The output is deterministic: pairs are sorted by Desired.ID, and both
// unmatched slices are sorted by ID, so identical inputs always produce
// byte-identical results.
func matchResources(desired, actual []core.Resource) ([]resourcePair, []core.Resource, []core.Resource) {
	desiredMatched := make([]bool, len(desired))
	actualMatched := make([]bool, len(actual))
	var pairs []resourcePair

	for _, keyOf := range matchKeyFuncs() {
		desiredByKey := groupByKey(desired, desiredMatched, keyOf)
		actualByKey := groupByKey(actual, actualMatched, keyOf)

		for key, dIdxs := range desiredByKey {
			aIdxs, ok := actualByKey[key]
			if !ok {
				continue
			}
			if len(dIdxs) != 1 || len(aIdxs) != 1 {
				continue
			}
			d, a := dIdxs[0], aIdxs[0]
			pairs = append(pairs, resourcePair{Desired: desired[d], Actual: actual[a]})
			desiredMatched[d] = true
			actualMatched[a] = true
		}
	}

	var unmatchedDesired []core.Resource
	for i, m := range desiredMatched {
		if !m {
			unmatchedDesired = append(unmatchedDesired, desired[i])
		}
	}
	var unmatchedActual []core.Resource
	for j, m := range actualMatched {
		if !m {
			unmatchedActual = append(unmatchedActual, actual[j])
		}
	}

	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].Desired.ID < pairs[j].Desired.ID
	})
	sort.Slice(unmatchedDesired, func(i, j int) bool {
		return unmatchedDesired[i].ID < unmatchedDesired[j].ID
	})
	sort.Slice(unmatchedActual, func(i, j int) bool {
		return unmatchedActual[i].ID < unmatchedActual[j].ID
	})

	return pairs, unmatchedDesired, unmatchedActual
}

// matchKey derives a (Provider, Type, Kind, Value) lookup key for a resource,
// or returns ok=false when the resource does not participate in this pass
// (for example, empty Name in the name-fallback pass).
type matchKey func(core.Resource) (key string, ok bool)

// matchKeyFuncs returns the ordered list of key functions that implement the
// three-pass match strategy. Kept as a function so callers receive a fresh
// slice (no shared mutable state) and to make the pass order explicit.
func matchKeyFuncs() []matchKey {
	return []matchKey{
		keyByID,
		keyByName,
		keyByTagName,
	}
}

func keyByID(r core.Resource) (string, bool) {
	if r.ID == "" {
		return "", false
	}
	return r.Provider + "|" + r.Type + "|id|" + r.ID, true
}

func keyByName(r core.Resource) (string, bool) {
	if r.Name == "" {
		return "", false
	}
	return r.Provider + "|" + r.Type + "|name|" + r.Name, true
}

func keyByTagName(r core.Resource) (string, bool) {
	name, ok := extractTagName(r)
	if !ok || name == "" {
		return "", false
	}
	return r.Provider + "|" + r.Type + "|tag|" + name, true
}

// extractTagName reads tags["Name"] from r.Data, tolerating both
// map[string]string and map[string]interface{} tag shapes. Returns
// (value, true) on a string value and ("", false) otherwise.
func extractTagName(r core.Resource) (string, bool) {
	if r.Data == nil {
		return "", false
	}
	tagsRaw, ok := r.Data["tags"]
	if !ok {
		return "", false
	}
	switch tags := tagsRaw.(type) {
	case map[string]interface{}:
		name, nameOk := tags["Name"]
		if !nameOk {
			return "", false
		}
		s, sOk := name.(string)
		return s, sOk
	case map[string]string:
		s, ok := tags["Name"]
		return s, ok
	}
	return "", false
}

// groupByKey returns a map of key -> unmatched-resource-indices for a single
// pass. Only resources whose matched[i] is false are considered.
func groupByKey(rs []core.Resource, matched []bool, keyOf matchKey) map[string][]int {
	out := make(map[string][]int)
	for i, r := range rs {
		if matched[i] {
			continue
		}
		key, ok := keyOf(r)
		if !ok {
			continue
		}
		out[key] = append(out[key], i)
	}
	return out
}
