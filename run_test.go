package main

import (
	"errors"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestActionArgs(t *testing.T) {
	cases := []struct {
		act  action
		want string
	}{
		{actionDownload, "download host:1"},
		{actionDownloadDecrypt, "download --decrypt host:1"},
		{actionKeys, "keys host:1"},
	}
	for _, c := range cases {
		if got := strings.Join(c.act.args("host:1"), " "); got != c.want {
			t.Errorf("action %d args = %q, want %q", c.act, got, c.want)
		}
	}
}

func TestStripANSI(t *testing.T) {
	in := "\x1b[36m[OK]\x1b[0m  Pack_v1.0.0   \t"
	if got := stripANSI(in); got != "[OK]  Pack_v1.0.0" {
		t.Errorf("stripANSI = %q", got)
	}
}

func TestEnvWithout(t *testing.T) {
	env := []string{"PATH=/bin", "NO_COLOR=1", "HOME=/home/x", "NO_COLORISH=keep"}
	got := envWithout(env, "NO_COLOR")
	for _, e := range got {
		if e == "NO_COLOR=1" {
			t.Fatal("NO_COLOR=1 should have been dropped")
		}
	}
	if len(got) != 3 {
		t.Fatalf("envWithout kept %d entries, want 3 (NO_COLORISH must survive): %v", len(got), got)
	}
}

func TestSplitStream(t *testing.T) {
	// \r marks a transient (progress) line; \n a committed one. A final
	// chunk without a terminator commits at EOF; blanks are dropped.
	in := "first line\nDownloading 1.0 MB\rDownloading 2.0 MB\r\n\n[OK] done\ntail-no-newline"
	var got []runEvent
	splitStream(strings.NewReader(in), func(ev runEvent) { got = append(got, ev) })

	want := []runEvent{
		{"first line", false},
		{"Downloading 1.0 MB", true},
		{"Downloading 2.0 MB", true},
		{"[OK] done", false},
		{"tail-no-newline", false},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestStreamChildIntegration runs a real subprocess through streamChild and
// asserts the event stream: committed (\n) and transient (\r) lines, then a
// single jobFinishedMsg. This is the pipeline every menu run uses.
func TestStreamChildIntegration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell")
	}
	cmd := exec.Command("sh", "-c", `printf 'first\nprog1\rprog2\r\ndone\n'`)
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	ch := make(chan tea.Msg, 64)
	go streamChild(cmd, pr, pw, ch)

	var events []runEvent
	var finished *jobFinishedMsg
	deadline := time.After(10 * time.Second)
	for finished == nil {
		select {
		case msg := <-ch:
			switch m := msg.(type) {
			case runEvent:
				events = append(events, m)
			case jobFinishedMsg:
				finished = &m
			}
		case <-deadline:
			t.Fatal("timed out waiting for child output")
		}
	}
	if finished.err != nil {
		t.Fatalf("clean exit expected, got %v", finished.err)
	}
	want := []runEvent{
		{"first", false},
		{"prog1", true},
		{"prog2", true},
		{"done", false},
	}
	if len(events) != len(want) {
		t.Fatalf("got %d events, want %d: %+v", len(events), len(want), events)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Errorf("event %d = %+v, want %+v", i, events[i], want[i])
		}
	}
}

// TestProcessControl validates the OS-level pause/cancel primitives the
// running screen relies on: suspend/resume don't error and kill terminates
// the child (so cmd.Wait returns).
func TestProcessControl(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SIGSTOP/SIGCONT are POSIX-only")
	}
	cmd := exec.Command("sh", "-c", "sleep 30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := suspendProcess(cmd.Process); err != nil {
		t.Errorf("suspend: %v", err)
	}
	if err := resumeProcess(cmd.Process); err != nil {
		t.Errorf("resume: %v", err)
	}
	if err := killProcess(cmd.Process); err != nil {
		t.Errorf("kill: %v", err)
	}

	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("killed process did not exit")
	}
}

// TestInterpretExit_SignalKill pins that a signal-terminated child (exit -1,
// the cancel path) maps to an error - distinct from the exit-2 soft success.
func TestInterpretExit_SignalKill(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX signal kill")
	}
	cmd := exec.Command("sh", "-c", "sleep 30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = killProcess(cmd.Process)
	if interpretExit(cmd.Wait()) == nil {
		t.Error("signal-killed child (-1) should map to an error")
	}
}

func TestInterpretExit(t *testing.T) {
	if interpretExit(nil) != nil {
		t.Error("nil should stay nil")
	}
	sentinel := errors.New("boom")
	if !errors.Is(interpretExit(sentinel), sentinel) {
		t.Error("non-exit errors should pass through")
	}

	if runtime.GOOS == "windows" {
		t.Skip("shell-based exit-code check is POSIX-only")
	}
	// Exit code 2 is errPartialResult - treated as a soft success.
	err2 := exec.Command("sh", "-c", "exit 2").Run()
	if interpretExit(err2) != nil {
		t.Errorf("exit code 2 should map to nil, got %v", interpretExit(err2))
	}
	// Any other non-zero is a real failure.
	err1 := exec.Command("sh", "-c", "exit 1").Run()
	if interpretExit(err1) == nil {
		t.Error("exit code 1 should map to an error")
	}
}
