// Package diff handles comparison between desired and actual state.
package diff

import "time"

// Options controls how CalculateDriftWithOptions matches and compares resources.
//
// The zero value is valid but produces degenerate behavior (no ephemeral
// filtering, no list normalization, and a nil clock). Most callers should use
// DefaultOptions and mutate the returned value as needed.
type Options struct {
	// IgnoredAttributes holds attribute keys that must never produce drift,
	// in addition to EphemeralAttributes. Match is by exact key name at any
	// depth of the Data tree. Use this to suppress attributes that are
	// project-specific noise (e.g. a managed-by tag).
	IgnoredAttributes []string

	// EphemeralAttributes holds attribute keys that commonly change due to
	// external factors (provider refresh timestamps, write ETags, internal
	// metadata) and should never be reported as drift. Match is by exact key
	// name at any depth of the Data tree. DefaultOptions populates this with
	// a conservative list; callers may replace it entirely.
	EphemeralAttributes []string

	// CaseInsensitive, when true, compares string values using
	// strings.EqualFold. Map keys, resource IDs, and resource names are
	// always compared case-sensitively regardless of this setting.
	CaseInsensitive bool

	// SortLists, when true, sorts slices of homogeneous comparable primitives
	// (string, all-numeric, all-bool) before comparing them. Heterogeneous or
	// complex-element slices are left in order and compared element-wise.
	SortLists bool

	// Now returns the time used to stamp DriftReport.Timestamp. Injecting a
	// fixed clock here keeps tests deterministic. When nil, time.Now is used.
	Now func() time.Time
}

// DefaultOptions returns a conservative set of options suitable for most
// drift-detection use cases.
//
// The defaults are:
//   - EphemeralAttributes: last_updated, last_modified, etag, meta,
//     terraform_version, creation_date, created_at, modified_at.
//   - IgnoredAttributes:   none.
//   - CaseInsensitive:     false.
//   - SortLists:           true.
//   - Now:                 time.Now.
func DefaultOptions() Options {
	return Options{
		IgnoredAttributes:   nil,
		EphemeralAttributes: defaultEphemeralAttributes(),
		CaseInsensitive:     false,
		SortLists:           true,
		Now:                 time.Now,
	}
}

// defaultEphemeralAttributes returns the built-in list of attribute keys
// filtered from comparison by default.
func defaultEphemeralAttributes() []string {
	return []string{
		"last_updated",
		"last_modified",
		"etag",
		"meta",
		"terraform_version",
		"creation_date",
		"created_at",
		"modified_at",
	}
}

// isIgnored reports whether the given attribute key should be skipped during
// comparison. Match is exact and case-sensitive across both IgnoredAttributes
// and EphemeralAttributes.
func (o Options) isIgnored(key string) bool {
	for _, k := range o.EphemeralAttributes {
		if k == key {
			return true
		}
	}
	for _, k := range o.IgnoredAttributes {
		if k == key {
			return true
		}
	}
	return false
}

// now returns the current time using the injected clock, falling back to
// time.Now when Options.Now is nil. The monotonic clock reading is stripped
// (via Round(0)) so the timestamp survives round-trips through JSON and other
// serializations.
func (o Options) now() time.Time {
	if o.Now != nil {
		return o.Now().Round(0)
	}
	return time.Now().Round(0)
}
