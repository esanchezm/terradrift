package cmd

import (
	"errors"
	"fmt"
	"io"
)

// Exit codes are the CI-friendly 0 / 1 / 2 contract:
//
//   - ExitCodeClean (0): scan completed and found no drift.
//   - ExitCodeDrift (1): scan completed and reported unmanaged, missing,
//     or drifted resources (ignored resources do not count).
//   - ExitCodeError (2): scan could not complete — bad arguments,
//     unreadable state, cloud-provider failure, etc.
//
// Note: this differs from `terraform plan -detailed-exitcode`, which
// swaps 1 and 2 (1 == error, 2 == diff). The ordering chosen here puts
// "drift detected" between "clean" and "error" so the magnitude of the
// exit code monotonically tracks the severity of the outcome, which is
// what CI authors generally expect.
//
// These numeric values are part of the public CLI contract; changing
// them breaks CI scripts that pattern-match on exit status.
const (
	ExitCodeClean = 0
	ExitCodeDrift = 1
	ExitCodeError = 2
)

// ExitCoder is implemented by errors that know their own desired
// process exit code. exitCodeFor consults it to classify errors; any
// error that does not implement ExitCoder is treated as a generic
// failure (ExitCodeError).
type ExitCoder interface {
	ExitCode() int
}

// driftError signals that the scan succeeded but drift was detected
// and the user has requested CI-friendly exit codes (--exit-code, the
// default). The counts are recorded so callers can print a concise
// human-readable summary to stderr without re-traversing the report.
//
// The Error() string intentionally reads as a CI log line
// ("drift detected: 2 unmanaged, 1 missing, 1 drifted") rather than a
// Go-style wrapped error, because Execute prints this message verbatim
// to stderr alongside the full stdout report.
type driftError struct {
	Unmanaged int
	Missing   int
	Drifted   int
}

func (e *driftError) Error() string {
	return fmt.Sprintf("drift detected: %d unmanaged, %d missing, %d drifted",
		e.Unmanaged, e.Missing, e.Drifted)
}

// ExitCode satisfies ExitCoder.
func (e *driftError) ExitCode() int { return ExitCodeDrift }

// exitCodeFor maps a cobra RunE error to a process exit code. The rules
// are deliberately minimal so the classification is auditable:
//
//  1. A nil error is a clean scan (ExitCodeClean).
//  2. Any error whose chain (via errors.As) implements ExitCoder yields
//     that code. This is the extension point for future non-drift
//     signals such as "partial scan succeeded" without changing Execute.
//  3. Everything else is a generic failure (ExitCodeError).
func exitCodeFor(err error) int {
	if err == nil {
		return ExitCodeClean
	}
	var coder ExitCoder
	if errors.As(err, &coder) {
		return coder.ExitCode()
	}
	return ExitCodeError
}

// handleExitError is the testable core of Execute: it takes the error
// cobra returned from rootCmd.Execute, writes a one-line diagnostic to
// stderr when the error is non-nil, and returns the exit code.
//
// Two stderr formats are produced:
//
//   - Drift signals (*driftError) print their message verbatim — for
//     example "drift detected: 2 unmanaged, 1 missing, 1 drifted". No
//     "Error:" prefix: CI logs should see this as a status, not a
//     Go-style failure.
//   - Everything else is printed as "Error: <message>", matching the
//     convention cobra uses when SilenceErrors is false.
func handleExitError(err error, stderr io.Writer) int {
	if err == nil {
		return ExitCodeClean
	}
	var drift *driftError
	if errors.As(err, &drift) {
		fmt.Fprintln(stderr, drift.Error())
		return drift.ExitCode()
	}
	fmt.Fprintln(stderr, "Error:", err)
	return exitCodeFor(err)
}
