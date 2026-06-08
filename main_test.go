package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

// TestHandleErr_ExitCodes verifies the classification matrix the main
// loop now uses: Ctrl-C is silent (130), usage errors print nothing
// extra (1), partial results signal CI (2), everything else gets the
// humanize / raw output and exits 1.
func TestHandleErr_ExitCodes(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		wantCode  int
		wantBody  string // substring expected on stderr, "" = no output
		notWanted string // substring that MUST NOT appear (e.g. Error: on Ctrl-C)
	}{
		{
			name:      "context.Canceled is silent exit 130",
			err:       context.Canceled,
			wantCode:  130,
			notWanted: "Error:",
		},
		{
			name:      "wrapped context.Canceled also silent",
			err:       fmt.Errorf("dial: %w", context.Canceled),
			wantCode:  130,
			notWanted: "Error:",
		},
		{
			name:      "errUsage exits 1 without re-printing",
			err:       errUsage,
			wantCode:  1,
			notWanted: "Error:",
		},
		{
			name:      "errPartialResult exits 2 silently",
			err:       errPartialResult,
			wantCode:  2,
			notWanted: "Error:",
		},
		{
			name:     "humanize-matched error: exits 1 with diagnostic",
			err:      fmt.Errorf("%w: 16 chars", errPackBadKeyLen),
			wantCode: 1,
			wantBody: "Key length is wrong",
		},
		{
			name:     "unclassified error: exits 1 with raw",
			err:      errors.New("totally novel failure mode"),
			wantCode: 1,
			wantBody: "totally novel failure mode",
		},
	}

	// Force NO_COLOR so the buffer doesn't have ANSI escapes that
	// would break naive substring matches.
	savedRed, savedYellow, savedReset := colorRed, colorYellow, colorReset
	colorRed, colorYellow, colorReset = "", "", ""
	t.Cleanup(func() { colorRed, colorYellow, colorReset = savedRed, savedYellow, savedReset })

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf strings.Builder
			code := handleErr(&buf, tc.err)
			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d", code, tc.wantCode)
			}
			if tc.wantBody != "" && !strings.Contains(buf.String(), tc.wantBody) {
				t.Errorf("stderr missing %q\nstderr:\n%s", tc.wantBody, buf.String())
			}
			if tc.notWanted != "" && strings.Contains(buf.String(), tc.notWanted) {
				t.Errorf("stderr should NOT contain %q\nstderr:\n%s", tc.notWanted, buf.String())
			}
		})
	}
}

// TestHandleErr_NilWriterDoesNotPanic safety-checks that an
// implementation change passing a nil writer (e.g. a future tracing
// rework) would surface as a panic in tests rather than at runtime.
func TestHandleErr_NilWriterDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("handleErr panicked with nil-ish writer: %v", r)
		}
	}()
	_ = handleErr(io.Discard, errors.New("any err"))
}

