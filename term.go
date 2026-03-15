package main

import "os"

var (
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorReset  = "\033[0m"
	clearLine   = "\r\033[K"
)

func init() {
	if os.Getenv("NO_COLOR") != "" {
		colorRed = ""
		colorGreen = ""
		colorYellow = ""
		colorCyan = ""
		colorReset = ""
		clearLine = ""
	}
}
