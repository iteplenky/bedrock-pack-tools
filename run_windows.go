//go:build windows

package main

import "os"

// pauseSupported is false on Windows: there's no portable SIGSTOP, so the
// menu hides the pause hint and only offers cancel.
const pauseSupported = false

func suspendProcess(*os.Process) error { return nil }
func resumeProcess(*os.Process) error  { return nil }

func killProcess(p *os.Process) error { return p.Kill() }
