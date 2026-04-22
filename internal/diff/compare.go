package diff

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/esanchezm/terradrift/internal/core"
)

// diffResources compares the Data maps of two resources that identify the
// same logical object and returns the attribute-level differences.
//
// Only keys present in BOTH maps are compared. This intersection policy
// prevents schema differences between the Terraform state format (e.g. key
// "id") and the cloud provider format (e.g. key "instance_id") from
// producing phantom add/remove changes.
//
// The returned slice is sorted by Change.Attribute so output is deterministic
// across runs regardless of Go's random map iteration order.
func diffResources(desired, actual core.Resource, opts Options) []Change {
	var changes []Change
	diffMap("", desired.Data, actual.Data, desired, opts, &changes)
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].Attribute < changes[j].Attribute
	})
	return changes
}

// diffMap walks two maps intersecting by key and appends changes to out.
// A nil map is treated as empty (no keys to intersect with the other side).
// The prefix, when non-empty, is prepended (with a dot separator) to each
// emitted attribute path.
func diffMap(prefix string, oldM, newM map[string]interface{}, res core.Resource, opts Options, out *[]Change) {
	if oldM == nil || newM == nil {
		return
	}
	for k, oldV := range oldM {
		if opts.isIgnored(k) {
			continue
		}
		newV, ok := newM[k]
		if !ok {
			continue
		}
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}
		diffValue(path, oldV, newV, res, opts, out)
	}
}

// diffValue compares two arbitrary values at a given path. It normalizes both
// sides, handles the nil/empty-collection equivalence, and then recurses into
// maps or slices or falls back to scalar equality. For length-mismatched
// slices it emits a single whole-slice change rather than per-index changes.
func diffValue(path string, oldV, newV interface{}, res core.Resource, opts Options, out *[]Change) {
	oldN := normalizeValue(oldV, opts)
	newN := normalizeValue(newV, opts)

	if isEmptyCollection(oldN) && isEmptyCollection(newN) {
		return
	}

	if oldM, ok := oldN.(map[string]interface{}); ok {
		if newM, ok := newN.(map[string]interface{}); ok {
			diffMap(path, oldM, newM, res, opts, out)
			return
		}
	}

	if oldS, ok := oldN.([]interface{}); ok {
		if newS, ok := newN.([]interface{}); ok {
			if len(oldS) == len(newS) {
				for i := range oldS {
					diffValue(fmt.Sprintf("%s.%d", path, i), oldS[i], newS[i], res, opts, out)
				}
				return
			}
			*out = append(*out, Change{
				ResourceID:   res.ID,
				ResourceType: res.Type,
				ResourceName: res.Name,
				ChangeType:   ChangeTypeUpdate,
				Attribute:    path,
				OldValue:     oldV,
				NewValue:     newV,
			})
			return
		}
	}

	if valuesEqual(oldN, newN, opts) {
		return
	}

	*out = append(*out, Change{
		ResourceID:   res.ID,
		ResourceType: res.Type,
		ResourceName: res.Name,
		ChangeType:   ChangeTypeUpdate,
		Attribute:    path,
		OldValue:     oldV,
		NewValue:     newV,
	})
}

// valuesEqual compares two normalized non-collection values. Numeric types
// are compared via numericsEqual so int/uint families retain exact precision
// (avoiding the 2^53 rounding trap of float64). Strings are compared with
// strings.EqualFold when opts.CaseInsensitive is true. Everything else
// falls through to reflect.DeepEqual.
func valuesEqual(a, b interface{}, opts Options) bool {
	if eq, ok := numericsEqual(a, b); ok {
		return eq
	}
	if opts.CaseInsensitive {
		sa, aIsStr := a.(string)
		sb, bIsStr := b.(string)
		if aIsStr && bIsStr {
			return strings.EqualFold(sa, sb)
		}
	}
	return reflect.DeepEqual(a, b)
}

// isEmptyCollection reports whether v is nil, an empty map[string]interface{},
// or an empty []interface{}. The empty string is intentionally NOT considered
// empty: "" is a distinct value from a missing/nil string.
func isEmptyCollection(v interface{}) bool {
	if v == nil {
		return true
	}
	switch x := v.(type) {
	case map[string]interface{}:
		return len(x) == 0
	case []interface{}:
		return len(x) == 0
	}
	return false
}

// normalizeValue canonicalizes the container shape of v (maps and slices)
// without canonicalizing numeric types. Numeric comparison happens later in
// numericsEqual with exact precision for int/uint families.
//
// Transformations:
//   - map[string]string is widened to map[string]interface{} so both shapes
//     of "tags" compare equal.
//   - Typed slices ([]string, []int, []int32, []int64, []float64, []bool)
//     are widened to []interface{} for uniform element-wise iteration.
//   - Nested maps and slices are normalized element-wise.
//   - When opts.SortLists is true, slices whose elements are all of the same
//     primitive kind (string, numeric, or bool) are sorted. String sort
//     honors opts.CaseInsensitive; numeric sort works across mixed int/uint/
//     float combinations. Heterogeneous or complex-element slices are left
//     in order to avoid spurious panics and preserve semantic ordering.
//
// Numeric values (int, int32, uint64, float64, etc.) are returned unchanged
// so their exact bit pattern survives into numericsEqual.
func normalizeValue(v interface{}, opts Options) interface{} {
	switch x := v.(type) {
	case nil:
		return nil
	case map[string]interface{}:
		out := make(map[string]interface{}, len(x))
		for k, vv := range x {
			out[k] = normalizeValue(vv, opts)
		}
		return out
	case map[string]string:
		out := make(map[string]interface{}, len(x))
		for k, vv := range x {
			out[k] = vv
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(x))
		for i, vv := range x {
			out[i] = normalizeValue(vv, opts)
		}
		if opts.SortLists {
			sortIfHomogeneous(out, opts)
		}
		return out
	case []string:
		out := make([]interface{}, len(x))
		for i, vv := range x {
			out[i] = vv
		}
		if opts.SortLists {
			sortIfHomogeneous(out, opts)
		}
		return out
	case []int:
		out := make([]interface{}, len(x))
		for i, vv := range x {
			out[i] = vv
		}
		if opts.SortLists {
			sortIfHomogeneous(out, opts)
		}
		return out
	case []int32:
		out := make([]interface{}, len(x))
		for i, vv := range x {
			out[i] = vv
		}
		if opts.SortLists {
			sortIfHomogeneous(out, opts)
		}
		return out
	case []int64:
		out := make([]interface{}, len(x))
		for i, vv := range x {
			out[i] = vv
		}
		if opts.SortLists {
			sortIfHomogeneous(out, opts)
		}
		return out
	case []float64:
		out := make([]interface{}, len(x))
		for i, vv := range x {
			out[i] = vv
		}
		if opts.SortLists {
			sortIfHomogeneous(out, opts)
		}
		return out
	case []bool:
		out := make([]interface{}, len(x))
		for i, vv := range x {
			out[i] = vv
		}
		if opts.SortLists {
			sortIfHomogeneous(out, opts)
		}
		return out
	}
	return v
}

// numericCategory classifies a value's numeric kind for cross-type equality.
type numericCategory int

const (
	catNonNumeric numericCategory = iota
	catInt
	catUint
	catFloat
)

// numericCat classifies v and returns its int64/uint64/float64 projections.
// For non-numeric values it returns catNonNumeric and zero projections.
//
// The returned float64 is always populated for numeric inputs and is used
// for numeric sorting; the catInt and catUint projections preserve exact
// integer precision for equality comparisons.
func numericCat(v interface{}) (cat numericCategory, i int64, u uint64, f float64) {
	switch x := v.(type) {
	case int:
		return catInt, int64(x), 0, float64(x)
	case int8:
		return catInt, int64(x), 0, float64(x)
	case int16:
		return catInt, int64(x), 0, float64(x)
	case int32:
		return catInt, int64(x), 0, float64(x)
	case int64:
		return catInt, x, 0, float64(x)
	case uint:
		return catUint, 0, uint64(x), float64(x)
	case uint8:
		return catUint, 0, uint64(x), float64(x)
	case uint16:
		return catUint, 0, uint64(x), float64(x)
	case uint32:
		return catUint, 0, uint64(x), float64(x)
	case uint64:
		return catUint, 0, x, float64(x)
	case float32:
		return catFloat, 0, 0, float64(x)
	case float64:
		return catFloat, 0, 0, x
	}
	return catNonNumeric, 0, 0, 0
}

// numericsEqual reports whether a and b are both numeric and equal.
//
// Precision semantics:
//   - Two signed ints or two unsigned ints compare exactly (full int64
//     or full uint64 range respectively).
//   - A signed/unsigned mix is resolved by sign: a negative signed int
//     is never equal to any uint, and a non-negative signed int is
//     compared to its uint64 projection. Full precision up to 2^63-1.
//   - Any comparison involving a float (float32 or float64) falls back
//     to float64 equality. Precision loss is possible only when a float
//     participates, which matches Go's native conversion semantics and
//     the fact that JSON-decoded numbers are already float64 upstream.
//
// The second return value indicates whether both inputs were numeric;
// when false, the first return value is meaningless and the caller
// should continue with non-numeric comparison strategies.
func numericsEqual(a, b interface{}) (equal bool, bothNumeric bool) {
	aCat, aInt, aUint, aFloat := numericCat(a)
	bCat, bInt, bUint, bFloat := numericCat(b)
	if aCat == catNonNumeric || bCat == catNonNumeric {
		return false, false
	}
	switch {
	case aCat == catInt && bCat == catInt:
		return aInt == bInt, true
	case aCat == catUint && bCat == catUint:
		return aUint == bUint, true
	case aCat == catInt && bCat == catUint:
		if aInt < 0 {
			return false, true
		}
		return uint64(aInt) == bUint, true
	case aCat == catUint && bCat == catInt:
		if bInt < 0 {
			return false, true
		}
		return aUint == uint64(bInt), true
	default:
		return aFloat == bFloat, true
	}
}

// sortIfHomogeneous sorts s in place when all elements share a single
// primitive kind. Supported kinds are:
//
//   - string: sorted lexicographically; when opts.CaseInsensitive is true,
//     sort keys are lowercased first so case-different permutations (for
//     example ["B","a"] and ["a","B"]) produce the same ordering and
//     thus compare equal after element-wise case-fold comparison.
//   - numeric (any mix of int, uint, or float types): sorted by numericLess,
//     which uses exact int64/uint64 comparisons within integer categories
//     and falls back to float64 only when a float is involved. This
//     prevents large integers that project to the same float64 from
//     tying in sort order (which would otherwise make equal multisets
//     compare unequal after reorder).
//   - bool: sorted false-before-true.
//
// Heterogeneous slices and slices of maps are left untouched to avoid
// panics and to preserve semantic order where it matters.
func sortIfHomogeneous(s []interface{}, opts Options) {
	if len(s) < 2 {
		return
	}
	allStrings := true
	allNumeric := true
	allBools := true
	for _, v := range s {
		if _, ok := v.(string); !ok {
			allStrings = false
		}
		if cat, _, _, _ := numericCat(v); cat == catNonNumeric {
			allNumeric = false
		}
		if _, ok := v.(bool); !ok {
			allBools = false
		}
	}
	switch {
	case allStrings:
		sort.Slice(s, func(i, j int) bool {
			a, aOK := s[i].(string)
			b, bOK := s[j].(string)
			if !aOK || !bOK {
				return false
			}
			if opts.CaseInsensitive {
				return strings.ToLower(a) < strings.ToLower(b)
			}
			return a < b
		})
	case allNumeric:
		sort.Slice(s, func(i, j int) bool {
			return numericLess(s[i], s[j])
		})
	case allBools:
		sort.Slice(s, func(i, j int) bool {
			a, aOK := s[i].(bool)
			b, bOK := s[j].(bool)
			if !aOK || !bOK {
				return false
			}
			return !a && b
		})
	}
}

// numericLess defines a total order over numeric values consistent with
// numericsEqual: two values that numericsEqual reports as equal are never
// ordered, and two values that it reports as unequal always have a stable
// order.
//
// Within a single integer category (catInt or catUint) the comparison is
// exact (int64 or uint64). Across signed/unsigned integers the comparison
// is sign-safe: negative signed ints are ordered below any unsigned int.
// Only when a float participates does the function fall back to float64,
// matching the float-involved precision semantics of numericsEqual.
func numericLess(a, b interface{}) bool {
	aCat, aInt, aUint, aFloat := numericCat(a)
	bCat, bInt, bUint, bFloat := numericCat(b)
	switch {
	case aCat == catInt && bCat == catInt:
		return aInt < bInt
	case aCat == catUint && bCat == catUint:
		return aUint < bUint
	case aCat == catInt && bCat == catUint:
		if aInt < 0 {
			return true
		}
		return uint64(aInt) < bUint
	case aCat == catUint && bCat == catInt:
		if bInt < 0 {
			return false
		}
		return aUint < uint64(bInt)
	default:
		return aFloat < bFloat
	}
}
