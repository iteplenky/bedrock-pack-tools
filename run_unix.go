//go:build !windows

package main

import (
	"os"
	"syscall"
)

// pauseSupported gates the "p pause" hint and key. SIGSTOP/SIGCONT suspend
// the whole child - including its in-flight network reads - cleanly.
const pauseSupported = true

func suspendProcess(p *os.Process) error { return p.Signal(syscall.SIGSTOP) }
func resumeProcess(p *os.Process) error  { return p.Signal(syscall.SIGCONT) }

// killProcess sends SIGKILL, which also terminates a stopped (paused) child.
func killProcess(p *os.Process) error { return p.Kill() }
