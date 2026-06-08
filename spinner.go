package main

import (
	"fmt"
	"sync"
	"time"
)

var spinnerFrames = []string{
	"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏",
}

const spinnerInterval = 80 * time.Millisecond

// spinner shows a one-line animated indicator. stop is safe to call
// twice. In NO_COLOR mode it degrades to printing the label once -
// in-place rewrite would corrupt non-TTY output.
type spinner struct {
	done chan struct{}
	once sync.Once
}

func startSpinner(label string) *spinner {
	s := &spinner{done: make(chan struct{})}
	if clearLine == "" {
		fmt.Printf("  %s...\n", label)
		return s
	}
	fmt.Printf("%s  %s%s%s  %s", clearLine, colorCyan, spinnerFrames[0], colorReset, label)
	go s.run(label)
	return s
}

func (s *spinner) run(label string) {
	ticker := time.NewTicker(spinnerInterval)
	defer ticker.Stop()
	i := 1 // frame 0 was already printed by startSpinner
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			fmt.Printf("%s  %s%s%s  %s", clearLine, colorCyan, spinnerFrames[i%len(spinnerFrames)], colorReset, label)
			i++
		}
	}
}

func (s *spinner) stop(finalMsg string) {
	s.once.Do(func() { close(s.done) })
	if clearLine == "" {
		if finalMsg != "" {
			fmt.Printf("  %s\n", finalMsg)
		}
		return
	}
	if finalMsg != "" {
		fmt.Printf("%s  %s\n", clearLine, finalMsg)
	} else {
		fmt.Print(clearLine)
	}
}
