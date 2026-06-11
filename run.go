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
	"github.com/iteplenky/bedrock-pack-tools/v3/internal/lang"
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
	labelKey string
	descKey  string
	value    action
}

func (c actionChoice) label() string { return lang.T(c.labelKey) }
func (c actionChoice) desc() string  { return lang.T(c.descKey) }

var actionChoices = []actionChoice{
	{"tui.action.download.label", "tui.action.download.desc", actionDownload},
	{"tui.action.downloadDecrypt.label", "tui.action.downloadDecrypt.desc", actionDownloadDecrypt},
	{"tui.action.keys.label", "tui.action.keys.desc", actionKeys},
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

// needsAuth reports whether running this job re-execs an Xbox-authed subcommand
// (download or keys). Address-based jobs (no explicit argv) always run one of
// those via action.args; explicit-argv jobs only need auth when they carry a
// download/keys command (encrypt and decrypt run fully offline).
func (j job) needsAuth() bool {
	if j.argv == nil {
		return true
	}
	return len(j.argv) > 0 && (j.argv[0] == "download" || j.argv[0] == "keys")
}

type jobResult struct {
	label   string
	err     error
	partial bool   // exit 2: useful output landed but the run did not fully finish
	detail  string // the child's last line, shown for a partial result
	outDir  string
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
// detail is the child's last committed output line - usually the real error
// when it failed, surfaced instead of a bare "exited with code N".
type jobFinishedMsg struct {
	err    error
	detail string
}

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

// stripANSI removes color/cursor escapes so a line lays out predictably in
// the alt-screen (bubbletea repaints the whole frame each tick).
func stripANSI(s string) string {
	return strings.TrimRight(ansiRE.ReplaceAllString(s, ""), " \t")
}

var progressLineRE = regexp.MustCompile(`Downloading: [\d.]+ MB \(\d+ KB/s\)\s*`)

// cleanLogLine prepares a child stdout line for the in-menu log: it drops pure
// noise (the long CDN source URL, the box-drawing banner) and strips the live
// "Downloading: X MB (Y KB/s)" rate, which the child rewrites in place and
// which can glue onto a real line when captured. The live rate is shown
// separately as the status line. Returns ("", false) to drop the line.
func cleanLogLine(s string) (string, bool) {
	switch t := strings.TrimSpace(s); {
	case t == "",
		strings.HasPrefix(t, "CDN download:"),
		strings.HasPrefix(t, "┌"),
		strings.HasPrefix(t, "│"),
		strings.HasPrefix(t, "└"):
		return "", false
	}
	s = strings.TrimRight(progressLineRE.ReplaceAllString(s, ""), " ")
	if strings.TrimSpace(s) == "" {
		return "", false
	}
	return s, true
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
			return errPartialResult // useful output landed, but the run wasn't fully done
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
// decrypt code, and the tested CLI path runs byte-for-byte. The menu mints the
// Xbox token up front and refuses to launch download/keys jobs while signed out
// (see startRun), so the captured-pipe child never hits the device-code prompt.
// selfExe is the path to this binary, re-execed for run jobs and the
// interactive login handover.
func selfExe() (string, error) { return os.Executable() }

func runArgvCmd(argv []string) tea.Cmd {
	return func() tea.Msg {
		self, err := selfExe()
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
	var lastLine string
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		splitStream(pr, func(ev runEvent) {
			if !ev.transient {
				if t := strings.TrimSpace(ev.text); t != "" {
					lastLine = t // the child's last real line - usually the error on failure
				}
			}
			ch <- ev
		})
	}()
	// cmd.Wait blocks until the output-copy goroutines finish, so closing
	// the pipe afterwards cleanly ends the reader with everything flushed.
	err := cmd.Wait()
	_ = pw.Close()
	<-drained
	ch <- jobFinishedMsg{err: interpretExit(err), detail: lastLine}
	// Close so a consumer that stopped reading (e.g. after a cancel) can drain
	// to completion instead of leaking this goroutine on a blocked send.
	close(ch)
}

// waitRun pulls the next message off a running job's channel.
func waitRun(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

// loginDoneMsg fires after the re-execed `login` child finishes.
type loginDoneMsg struct{ err error }

// loginCmd re-execs `self login` with the terminal handed to it via
// tea.ExecProcess, so the device-code URL + code print and the poll run on the
// real TTY; bubbletea restores the menu's alt-screen when it returns. The
// child gets a normal interactive env (quietAuthEnv stripped) since it writes
// straight to the terminal, not through our capture pipe.
func loginCmd() tea.Cmd {
	self, err := selfExe()
	if err != nil {
		return func() tea.Msg { return loginDoneMsg{err: err} }
	}
	c := exec.Command(self, "login")
	c.Env = envWithout(os.Environ(), quietAuthEnv)
	return tea.ExecProcess(c, func(err error) tea.Msg { return loginDoneMsg{err: err} })
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
