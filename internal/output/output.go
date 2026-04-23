// Package output renders drift reports for the CLI.
//
// The rendering style intentionally mirrors `terraform plan` output so the
// format is instantly familiar to anyone who uses Terraform:
//
//   - Green `+` lines flag resources found in the cloud but not in state
//     (unmanaged).
//   - Red `-` lines flag resources in state but absent from the cloud
//     (missing).
//   - Yellow `~` lines flag resources present in both sides whose
//     attributes drifted, with each diverging attribute printed on an
//     indented follow-up line in `attribute: old → new` form.
//   - A final summary line prints counts for the four categories
//     (managed, unmanaged, missing, drifted).
//
// Color emission is delegated to lipgloss, which auto-degrades to plain
// ASCII when the destination writer is not a terminal (piped or
// redirected output). Callers can force ASCII regardless of TTY detection
// by passing Options.NoColor=true, and can suppress everything except
// the summary line with Options.Quiet=true.
package output

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/esanchezm/terradrift/internal/core"
	"github.com/esanchezm/terradrift/internal/diff"
)

// ScanInfo holds the scan metadata rendered in the report header. It
// complements DriftReport, which intentionally records only per-resource
// data, by capturing the environmental context a user needs to interpret
// the report (where state was loaded from, which provider was queried,
// which region).
type ScanInfo struct {
	// Provider is the cloud provider name as supplied on the command
	// line, for example "aws". The scanning banner upper-cases this
	// value; the Provider line prints it verbatim.
	Provider string
	// Region is the cloud region scanned, for example "eu-west-1".
	Region string
	// StateSource is a human-readable identifier for the state that was
	// loaded, typically a file path, s3:// URI, http(s):// URL, or "-"
	// for stdin. It is used verbatim in the "State source:" header line.
	StateSource string
	// StateResourceCount is the number of resources loaded from state.
	// Rendered in parentheses next to StateSource.
	StateResourceCount int
}

// Options configures a Renderer. The zero value is valid and produces a
// color-aware, verbose renderer bound to os.Stdout.
type Options struct {
	// Writer is the destination for rendered output. When nil, os.Stdout
	// is used. Tests should pass a *bytes.Buffer for deterministic
	// capture; lipgloss automatically disables color for non-terminal
	// writers.
	Writer io.Writer
	// NoColor, when true, forces the renderer's color profile to
	// termenv.Ascii regardless of writer TTY detection. This is the
	// escape hatch for users running in a TTY who still want uncolored
	// output (for example for CI log readability), and corresponds to
	// the --no-color CLI flag.
	NoColor bool
	// Quiet, when true, suppresses the header and per-resource detail
	// and prints only the summary line. Corresponds to the --quiet CLI
	// flag. Quiet output is a superset of --no-color in the sense that
	// it emits a single line with no styling applied to it.
	Quiet bool
}

// Renderer turns a diff.DriftReport into terraform-plan-style CLI output.
//
// Styles are resolved once at construction time via a *lipgloss.Renderer
// bound to the destination writer so that subsequent Render calls are
// cheap and consistent. Instances are not safe for concurrent use; callers
// needing parallel rendering should construct one Renderer per goroutine.
type Renderer struct {
	writer io.Writer
	quiet  bool

	// lr is retained so internal tests can override the color profile
	// to exercise ANSI-emitting paths deterministically.
	lr *lipgloss.Renderer

	added   lipgloss.Style // `+` prefix for unmanaged resources
	removed lipgloss.Style // `-` prefix for missing resources
	drifted lipgloss.Style // `~` prefix for drifted resources
	bold    lipgloss.Style // section headers
	arrow   lipgloss.Style // `→` between old and new values
}

// New returns a Renderer configured by opts. When opts.Writer is nil the
// renderer writes to os.Stdout. When opts.NoColor is true the color
// profile is forced to termenv.Ascii so the output contains no ANSI
// escape sequences even if the destination is a terminal.
func New(opts Options) *Renderer {
	w := opts.Writer
	if w == nil {
		w = os.Stdout
	}

	lr := lipgloss.NewRenderer(w)
	if opts.NoColor {
		lr.SetColorProfile(termenv.Ascii)
	}

	return &Renderer{
		writer:  w,
		quiet:   opts.Quiet,
		lr:      lr,
		added:   lr.NewStyle().Foreground(lipgloss.Color("2")).Bold(true),
		removed: lr.NewStyle().Foreground(lipgloss.Color("1")).Bold(true),
		drifted: lr.NewStyle().Foreground(lipgloss.Color("3")).Bold(true),
		bold:    lr.NewStyle().Bold(true),
		arrow:   lr.NewStyle().Faint(true),
	}
}

// Render prints the drift report in terraform-plan style. When the
// renderer is in Quiet mode only the summary line is emitted; otherwise
// the output consists of a header, a drift detail section, and the
// summary.
//
// Render returns the first error encountered while writing to the
// destination; partial output may have been emitted in that case.
func (r *Renderer) Render(info ScanInfo, report *diff.DriftReport) error {
	if report == nil {
		return fmt.Errorf("output: nil drift report")
	}

	if r.quiet {
		return r.writeSummary(report)
	}

	if err := r.writeHeader(info); err != nil {
		return err
	}
	if err := r.writeDriftSection(report); err != nil {
		return err
	}
	return r.writeSummary(report)
}

// writeHeader emits the three-line banner followed by a blank line:
//
//	Scanning AWS resources...
//	State source: terraform.tfstate (23 resources)
//	Provider: aws (region: eu-west-1)
//
// The first line upper-cases info.Provider because it reads as a
// sentence; the "Provider:" line preserves the original casing. When
// info.Region is empty the "(region: ...)" suffix is omitted so the
// line reads "Provider: aws" rather than the awkward
// "Provider: aws (region: )".
func (r *Renderer) writeHeader(info ScanInfo) error {
	if _, err := fmt.Fprintf(r.writer, "Scanning %s resources...\n", strings.ToUpper(info.Provider)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(r.writer, "State source: %s (%d resources)\n", info.StateSource, info.StateResourceCount); err != nil {
		return err
	}
	providerLine := fmt.Sprintf("Provider: %s", info.Provider)
	if info.Region != "" {
		providerLine += fmt.Sprintf(" (region: %s)", info.Region)
	}
	if _, err := fmt.Fprintln(r.writer, providerLine); err != nil {
		return err
	}
	_, err := fmt.Fprintln(r.writer)
	return err
}

// writeDriftSection emits either a "~~ Drift detected ~~" banner
// followed by per-category resource lines, or a "No drift detected."
// message when the report is clean. Output is terminated with a blank
// line so the summary stays visually separate.
//
// Within the drift section resources are grouped by category in a fixed
// order — unmanaged, missing, drifted — to match the ticket's worked
// example. Within each category, the order comes straight from the
// report (which is already ID-sorted by diff.CalculateDrift).
func (r *Renderer) writeDriftSection(report *diff.DriftReport) error {
	hasAny := len(report.Unmanaged) > 0 || len(report.Missing) > 0 || len(report.Drifted) > 0

	if !hasAny {
		if _, err := fmt.Fprintln(r.writer, "No drift detected."); err != nil {
			return err
		}
		_, err := fmt.Fprintln(r.writer)
		return err
	}

	if _, err := fmt.Fprintln(r.writer, r.bold.Render("~~ Drift detected ~~")); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(r.writer); err != nil {
		return err
	}

	for _, res := range report.Unmanaged {
		if _, err := fmt.Fprintf(r.writer, "  %s %s — unmanaged\n", r.added.Render("+"), resourceLabel(res)); err != nil {
			return err
		}
	}
	for _, res := range report.Missing {
		if _, err := fmt.Fprintf(r.writer, "  %s %s — missing from cloud\n", r.removed.Render("-"), resourceLabel(res)); err != nil {
			return err
		}
	}
	for _, d := range report.Drifted {
		if _, err := fmt.Fprintf(r.writer, "  %s %s\n", r.drifted.Render("~"), resourceLabel(d.Resource)); err != nil {
			return err
		}
		for _, c := range d.Changes {
			if _, err := fmt.Fprintf(r.writer, "      %s: %s %s %s\n",
				c.Attribute,
				formatValue(c.OldValue),
				r.arrow.Render("→"),
				formatValue(c.NewValue),
			); err != nil {
				return err
			}
		}
	}

	_, err := fmt.Fprintln(r.writer)
	return err
}

// writeSummary emits the single "Summary: ..." line. The counts are
// taken directly from report slice lengths and always rendered, even
// when every count is zero, so downstream tooling can rely on a
// well-formed final line.
func (r *Renderer) writeSummary(report *diff.DriftReport) error {
	_, err := fmt.Fprintf(r.writer, "Summary: %d managed, %d unmanaged, %d missing, %d drifted\n",
		len(report.Managed),
		len(report.Unmanaged),
		len(report.Missing),
		len(report.Drifted),
	)
	return err
}

// resourceLabel formats a resource as "type.name (id)", dropping the
// parenthesised ID when it duplicates the name (as is typical for S3
// buckets, whose ID is the bucket name) or when the ID is empty. When
// Name itself is empty the label falls back to "type.id" so providers
// that expose IDs but not user-supplied names (for example synthesised
// resources) still render intelligibly.
func resourceLabel(res core.Resource) string {
	switch {
	case res.Name != "" && res.ID != "" && res.Name != res.ID:
		return fmt.Sprintf("%s.%s (%s)", res.Type, res.Name, res.ID)
	case res.Name != "":
		return fmt.Sprintf("%s.%s", res.Type, res.Name)
	default:
		return fmt.Sprintf("%s.%s", res.Type, res.ID)
	}
}

// formatValue renders a drift value for display. Strings are Go-quoted
// via %q so whitespace, empty strings, and embedded escape characters
// are visible and unambiguous, matching the way `terraform plan`
// displays string attributes. Non-string values use %v, which produces
// the canonical Go representation for numbers, booleans, and nested
// structures. A nil value is rendered as the literal text "<nil>" rather
// than the bare `%v` result to avoid confusion with the empty string.
func formatValue(v interface{}) string {
	if v == nil {
		return "<nil>"
	}
	if s, ok := v.(string); ok {
		return fmt.Sprintf("%q", s)
	}
	return fmt.Sprintf("%v", v)
}


