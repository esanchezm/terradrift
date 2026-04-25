package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestExitCodeConstants_ArePartOfCLIContract(t *testing.T) {
	cases := []struct {
		name string
		got  int
		want int
	}{
		{"clean", ExitCodeClean, 0},
		{"drift", ExitCodeDrift, 1},
		{"error", ExitCodeError, 2},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s exit code = %d, want %d (CLI contract; changing breaks CI scripts)", tc.name, tc.got, tc.want)
		}
	}
}

func TestDriftError_ErrorFormat(t *testing.T) {
	e := &driftError{Unmanaged: 2, Missing: 1, Drifted: 1}
	want := "drift detected: 2 unmanaged, 1 missing, 1 drifted"
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestDriftError_ExitCodeIsDrift(t *testing.T) {
	e := &driftError{}
	if got := e.ExitCode(); got != ExitCodeDrift {
		t.Errorf("ExitCode() = %d, want %d", got, ExitCodeDrift)
	}
}

func TestDriftError_ZeroCounts_StillFormatsCorrectly(t *testing.T) {
	e := &driftError{}
	want := "drift detected: 0 unmanaged, 0 missing, 0 drifted"
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestExitCodeFor_NilErr_IsClean(t *testing.T) {
	if got := exitCodeFor(nil); got != ExitCodeClean {
		t.Errorf("exitCodeFor(nil) = %d, want %d", got, ExitCodeClean)
	}
}

func TestExitCodeFor_DriftError_IsDrift(t *testing.T) {
	err := &driftError{Unmanaged: 1}
	if got := exitCodeFor(err); got != ExitCodeDrift {
		t.Errorf("exitCodeFor(driftError) = %d, want %d", got, ExitCodeDrift)
	}
}

func TestExitCodeFor_WrappedDriftError_IsDrift(t *testing.T) {
	err := fmt.Errorf("wrapping drift: %w", &driftError{Drifted: 1})
	if got := exitCodeFor(err); got != ExitCodeDrift {
		t.Errorf("exitCodeFor(wrapped) = %d, want %d (errors.As must unwrap)", got, ExitCodeDrift)
	}
}

func TestExitCodeFor_PlainError_IsError(t *testing.T) {
	err := errors.New("state read failure")
	if got := exitCodeFor(err); got != ExitCodeError {
		t.Errorf("exitCodeFor(plain err) = %d, want %d", got, ExitCodeError)
	}
}

func TestHandleExitError_NilErr_NoOutputClean(t *testing.T) {
	var buf bytes.Buffer
	code := handleExitError(nil, &buf)
	if code != ExitCodeClean {
		t.Errorf("code = %d, want %d", code, ExitCodeClean)
	}
	if buf.Len() != 0 {
		t.Errorf("stderr must be empty on clean exit, got %q", buf.String())
	}
}

func TestHandleExitError_DriftError_WritesSummaryAndReturnsOne(t *testing.T) {
	var buf bytes.Buffer
	err := &driftError{Unmanaged: 2, Missing: 1, Drifted: 1}

	code := handleExitError(err, &buf)

	if code != ExitCodeDrift {
		t.Errorf("code = %d, want %d", code, ExitCodeDrift)
	}
	want := "drift detected: 2 unmanaged, 1 missing, 1 drifted\n"
	if buf.String() != want {
		t.Errorf("stderr = %q, want %q", buf.String(), want)
	}
	if strings.Contains(buf.String(), "Error:") {
		t.Errorf("drift stderr must not have 'Error:' prefix, got: %s", buf.String())
	}
}

func TestHandleExitError_WrappedDriftError_WritesUnderlyingMessage(t *testing.T) {
	var buf bytes.Buffer
	err := fmt.Errorf("run scan: %w", &driftError{Unmanaged: 1})

	code := handleExitError(err, &buf)

	if code != ExitCodeDrift {
		t.Errorf("code = %d, want %d", code, ExitCodeDrift)
	}
	if !strings.Contains(buf.String(), "drift detected:") {
		t.Errorf("stderr should contain drift message, got %q", buf.String())
	}
}

func TestHandleExitError_PlainError_WritesErrorPrefix(t *testing.T) {
	var buf bytes.Buffer
	err := errors.New("failed to read state")

	code := handleExitError(err, &buf)

	if code != ExitCodeError {
		t.Errorf("code = %d, want %d", code, ExitCodeError)
	}
	want := "Error: failed to read state\n"
	if buf.String() != want {
		t.Errorf("stderr = %q, want %q", buf.String(), want)
	}
}

func TestHandleExitError_IdempotentAcrossCalls(t *testing.T) {
	err := &driftError{Drifted: 1}

	var first, second bytes.Buffer
	c1 := handleExitError(err, &first)
	c2 := handleExitError(err, &second)

	if c1 != c2 {
		t.Errorf("handleExitError is not deterministic: c1=%d, c2=%d", c1, c2)
	}
	if first.String() != second.String() {
		t.Errorf("stderr differs across calls:\n1st: %q\n2nd: %q", first.String(), second.String())
	}
}
