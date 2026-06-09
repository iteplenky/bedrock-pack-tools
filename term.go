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

	// iconColor selects how the menu icon is rendered: "256" (the xterm-256
	// color cube, the default everywhere) or "none" (NO_COLOR / non-terminal ->
	// plain block silhouette). init() flips it to "none".
	iconColor = "256"
)

// iconPx maps each icon character to an RGB: the palette for the menu mascot, a
// little knight raising a key. The icon is drawn with half-block glyphs (▀ / ▄)
// - foreground is the top pixel, background the bottom, and '.' is transparent
// - so the 16x16 sprite floats on the terminal background. RGB is quantized to
// the 256-color cube at render time (see rgbTo256), so it looks the same with
// or without truecolor.
var iconPx = map[byte][3]uint8{
	'o': {38, 40, 55},    // outline
	'L': {208, 214, 226}, // light steel
	'S': {150, 162, 182}, // steel
	'D': {92, 104, 132},  // dark steel / armor
	'c': {78, 206, 224},  // cyan plume
	'C': {150, 236, 246}, // cyan plume highlight
	'g': {255, 214, 84},  // gold highlight
	'G': {214, 160, 40},  // gold
}

// disableANSI blanks every color and the clear-line escape so output is plain
// text - for NO_COLOR and for redirected, non-terminal stdout.
func disableANSI() {
	colorRed, colorGreen, colorYellow, colorCyan, colorDim, colorReset = "", "", "", "", "", ""
	iconColor = "none"
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
