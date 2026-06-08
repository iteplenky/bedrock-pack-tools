//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// Enable virtual-terminal processing on the real console so the direct CLI's
// colors and the carriage-return progress redraw render on modern Windows
// (the interactive menu's bubbletea program enables VT itself). Runs after
// term.go's init (filename order), so it respects an already-disabled palette.
// The TUI child (quietAuthEnv set) writes to a pipe, not a console, so it is
// left alone. If VT can't be enabled - legacy conhost, or redirected output -
// fall back to plain text rather than emit escapes nothing will render.
func init() {
	if colorReset == "" || os.Getenv(quietAuthEnv) != "" {
		return
	}
	ok := true
	for _, fd := range []windows.Handle{windows.Handle(os.Stdout.Fd()), windows.Handle(os.Stderr.Fd())} {
		var mode uint32
		if err := windows.GetConsoleMode(fd, &mode); err != nil {
			ok = false
			continue
		}
		if err := windows.SetConsoleMode(fd, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err != nil {
			ok = false
		}
	}
	if !ok {
		disableANSI()
	}
}
