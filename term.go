package main

import (
	"os"

	"github.com/charmbracelet/x/term"
)

var (
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorDim    = "\033[2m"
	colorReset  = "\033[0m"
	clearLine   = "\r\033[K"
)

// disableANSI blanks every color and the clear-line escape so output is plain
// text - for NO_COLOR and for redirected, non-terminal stdout.
func disableANSI() {
	colorRed, colorGreen, colorYellow, colorCyan, colorDim, colorReset = "", "", "", "", "", ""
	clearLine = ""
}

func init() {
	switch {
	case os.Getenv("NO_COLOR") != "":
		disableANSI()
	case os.Getenv(quietAuthEnv) == "" && !term.IsTerminal(os.Stdout.Fd()):
		// Redirected to a file/pipe: don't pollute it with escape codes. The
		// interactive menu's child process (quietAuthEnv set) is exempt - its
		// piped output uses the carriage-return progress protocol the parent
		// parses, and the parent strips the color codes itself.
		disableANSI()
	}
}
