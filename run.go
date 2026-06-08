package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/iteplenky/bedrock-pack-tools/v3/internal/franchise"
)

// action is the operation the menu runs against each chosen target. The
// three are mutually exclusive modes, picked one at a time on screenAction.
type action int

const (
	actionDownload action = iota
	actionDownloadDecrypt
	actionKeys
)

type actionChoice struct {
	label string
	desc  string
	value action
}

var actionChoices = []actionChoice{
	{"Download packs", "Save the keys file and the encrypted packs to this folder.", actionDownload},
	{"Download + decrypt", "Download, then turn every pack into a ready-to-edit folder.", actionDownloadDecrypt},
	{"Keys only", "Just dump the AES content keys - no packs downloaded.", actionKeys},
}

// args turns an action plus a resolved address into the subcommand we
// re-exec. These mirror the documented CLI exactly.
func (a action) args(address string) []string {
	switch a {
	case actionDownloadDecrypt:
		return []string{"download", "--decrypt", address}
	case actionKeys:
		return []string{"keys", address}
	default:
		return []string{"download", address}
	}
}

// job is one target to run the chosen action against. Featured rows may
// need a network resolve (experience/gathering) before they have an
// address; direct partners and typed addresses are ready immediately. A job
// may instead carry explicit argv (e.g. a decrypt run), in which case the
// action/address are ignored.
type job struct {
	label   string
	server  *franchise.Server // nil for a typed address
	address string            // set when ready (typed, or after a resolve)
	argv    []string          // when set, run verbatim instead of action.args
	outDir  string            // where decrypted packs land, for the Done summary
}

type jobResult struct {
	label  string
	err    error
	outDir string
}

// runEvent is one line of child output. Transient lines (carriage-return
// rewrites: the speed ticker, the spinner) replace the status line;
// committed lines (newline-terminated) append to the scrollback.
type runEvent struct {
	text      string
	transient bool
}

// resolvedMsg carries the host:port for a featured row that needed a
// network resolve, or the error that stopped it.
type resolvedMsg struct {
	address string
	err     error
}

// jobStartedMsg hands the running child back to the model so key presses
// can pause (SIGSTOP) or cancel (kill) it.
type jobStartedMsg struct {
	proc *os.Process
	ch   chan tea.Msg
}

// jobFinishedMsg fires once the child exits and all its output is drained.
type jobFinishedMsg struct{ err error }

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

// stripANSI removes color/cursor escapes so a line lays out predictably in
// the alt-screen (bubbletea repaints the whole frame each tick).
func stripANSI(s string) string {
	return strings.TrimRight(ansiRE.ReplaceAllString(s, ""), " \t")
}

// envWithout returns env with every key=... entry dropped. We strip
// NO_COLOR from the child so it always uses the carriage-return progress
// protocol we parse - we discard the color codes ourselves either way.
func envWithout(env []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			out = append(out, e)
		}
	}
	return out
}

// interpretExit maps the child's exit status to an error. Exit code 2 is
// errPartialResult - the run produced useful output (some packs, or the
// keys), so it isn't a hard failure for the summary.
func interpretExit(err error) error {
	if err == nil {
		return nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if ee.ExitCode() == 2 {
			return nil
		}
		return fmt.Errorf("exited with code %d", ee.ExitCode())
	}
	return err
}

// splitStream reads r and emits a runEvent per terminated chunk, marking
// carriage-return chunks transient and newline chunks committed. Empty
// chunks are dropped so blank repaints don't spam the scrollback.
func splitStream(r io.Reader, emit func(runEvent)) {
	br := bufio.NewReader(r)
	var buf []byte
	flush := func(transient bool) {
		text := stripANSI(string(buf))
		buf = buf[:0]
		if strings.TrimSpace(text) == "" {
			return
		}
		emit(runEvent{text: text, transient: transient})
	}
	for {
		b, err := br.ReadByte()
		if err != nil {
			if len(buf) > 0 {
				flush(false)
			}
			return
		}
		switch b {
		case '\n':
			flush(false)
		case '\r':
			flush(true)
		default:
			buf = append(buf, b)
		}
	}
}

// runArgvCmd re-execs this binary with the given subcommand argv, streaming
// its output back as messages.
//
// We run the real CLI in a child process rather than calling runDownload
// in-process so the menu keeps full control: OS-level pause (SIGSTOP) and
// cancel (kill) work without weaving stop-gates through the networking and
// decrypt code, and the tested CLI path runs byte-for-byte. The Xbox token
// is already cached by runTUI's up-front auth, so the child never prompts.
func runArgvCmd(argv []string) tea.Cmd {
	return func() tea.Msg {
		self, err := os.Executable()
		if err != nil {
			return jobFinishedMsg{err: fmt.Errorf("locate binary: %w", err)}
		}
		cmd := exec.Command(self, argv...)
		cmd.Env = envWithout(os.Environ(), "NO_COLOR")

		pr, pw := io.Pipe()
		cmd.Stdout = pw
		cmd.Stderr = pw
		if err := cmd.Start(); err != nil {
			_ = pw.Close()
			return jobFinishedMsg{err: err}
		}

		ch := make(chan tea.Msg, 64)
		go streamChild(cmd, pr, pw, ch)
		return jobStartedMsg{proc: cmd.Process, ch: ch}
	}
}

// streamChild forwards the child's output as runEvents, then a single
// jobFinishedMsg once it exits and every line has been delivered.
func streamChild(cmd *exec.Cmd, pr *io.PipeReader, pw *io.PipeWriter, ch chan tea.Msg) {
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		splitStream(pr, func(ev runEvent) { ch <- ev })
	}()
	// cmd.Wait blocks until the output-copy goroutines finish, so closing
	// the pipe afterwards cleanly ends the reader with everything flushed.
	err := cmd.Wait()
	_ = pw.Close()
	<-drained
	ch <- jobFinishedMsg{err: interpretExit(err)}
	// Close so a consumer that stopped reading (e.g. after a cancel) can drain
	// to completion instead of leaking this goroutine on a blocked send.
	close(ch)
}

// waitRun pulls the next message off a running job's channel.
func waitRun(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

// resolveJobCmd turns a featured row into a host:port off the main loop. The
// context is owned by the model so a cancel keypress can abort the resolve
// instead of waiting out the timeout.
func resolveJobCmd(ctx context.Context, client *franchise.Client, s franchise.Server) tea.Cmd {
	return func() tea.Msg {
		addr, err := resolveAddress(ctx, client, s)
		return resolvedMsg{address: addr, err: err}
	}
}
