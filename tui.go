package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/iteplenky/bedrock-pack-tools/v3/internal/franchise"
	"golang.org/x/oauth2"
)

// Key glyphs used across the menu chrome. Arrows and ▸/› render in any
// terminal font the spinner already relies on.
const (
	gUp        = "↑"
	gDown      = "↓"
	gLeft      = "←"
	gRight     = "→"
	gEnter     = "↵"
	rowCursor  = "▸ "
	crumbArrow = " › "
	caret      = "▌"
)

// isInteractive reports whether stdin is a terminal (not a pipe/file), so
// the menu only launches when a human can drive it.
func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// runTUI opens the sectioned interactive menu. Xbox auth happens up front
// (so any device-code prompt is visible before the alt-screen takes over);
// everything else - browsing, downloading, decrypting - runs inside the
// menu, which stays open until the user quits. quietAuthEnv mutes the
// cached-token line so it doesn't linger on the main screen after exit, and
// is inherited by the child processes the menu spawns.
func runTUI() error {
	_ = os.Setenv(quietAuthEnv, "1")
	ts, err := getTokenSource()
	if err != nil {
		return err
	}
	if _, err := tea.NewProgram(newAppModel(ts), tea.WithAltScreen()).Run(); err != nil {
		return fmt.Errorf("interactive menu: %w", err)
	}
	return nil
}

type screen int

const (
	screenMenu    screen = iota
	screenLoading        // transient: featured catalog fetch + ping in flight
	screenFeatured
	screenSaved
	screenRecent
	screenDecrypt // decrypt packs from remembered download locations
	screenAddress
	screenAction  // pick download / download+decrypt / keys for the chosen targets
	screenRunning // a job queue executing, with live output
	screenDone    // per-target summary, any key returns to the menu
)

// section is one row of the main menu, with a help line shown when it's
// highlighted. group lays out related rows together with a blank-line break.
type section struct {
	label  string
	desc   string
	target screen
	group  int
}

// Ordered by workflow and grouped with blank-line breaks: pick a server to
// pull packs from, then your own saved/recent servers, then work with what
// you've already downloaded.
var sections = []section{
	{"Featured servers", "Browse Mojang's live catalog and pick one or more.", screenFeatured, 0},
	{"Enter a server address", "Type any host:port, e.g. play.example.net:19132.", screenAddress, 0},
	{"Saved servers", "Addresses you saved for quick re-use.", screenSaved, 1},
	{"Recent addresses", "Addresses you entered recently, with their last result.", screenRecent, 1},
	{"Decrypt packs", "Decrypt packs you've downloaded - or fetch what's missing.", screenDecrypt, 2},
}

func sectionTitle(target screen) string {
	for _, s := range sections {
		if s.target == target {
			return s.label
		}
	}
	return ""
}

// runLogTail is how many committed output lines the running screen shows.
const runLogTail = 12

// catalog messages drive the one async section (Featured needs the network).
type catalogLoadedMsg struct {
	servers []franchise.Server
	client  *franchise.Client
}
type catalogErrMsg struct{ err error }

// appModel is the root state machine: a main menu that delegates to the
// featured list, the saved/recent lists, or the address field, picks an
// action, then runs it against each chosen target without leaving the menu.
type appModel struct {
	screen     screen
	menuCursor int
	ts         oauth2.TokenSource
	width      int
	store      store

	featured          tuiModel      // featured-list state (screenFeatured)
	list              addrListModel // saved/recent/decrypt cursor + selection
	addr              addrModel
	loadErr           error
	resolvingFeatured bool   // a ^r bulk experience/event resolve is in flight
	note              string // transient confirmation (e.g. "saved"), cleared on next key

	// decrypt section: remembered downloads + their live on-disk state,
	// index-aligned. m.list (cursor/selection) indexes both.
	dls      []download
	dlStates []decryptState

	// catalog, loaded once and reused across menu visits.
	fServers []franchise.Server
	fClient  *franchise.Client

	// action picker.
	actionFrom   screen // where esc/← returns to
	actionCursor int

	// run queue + live state.
	act           action
	jobs          []job
	jobIdx        int
	runProc       *os.Process
	runCh         chan tea.Msg
	resolveCancel context.CancelFunc // cancels an in-flight featured resolve
	paused        bool
	canceled      bool
	statusLn      string
	logLines      []string
	results       []jobResult
	runSkipped    int // decrypt entries skipped this run (not decryptable)
}

func newAppModel(ts oauth2.TokenSource) appModel {
	return appModel{screen: screenMenu, ts: ts, store: loadStore()}
}

// loadCatalogCmd fetches + pings the featured catalog using the already-minted
// token (so it never triggers device-code auth inside the alt-screen).
func loadCatalogCmd(ts oauth2.TokenSource) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), featuredAPITimeout)
		defer cancel()
		servers, client, err := fetchFeaturedListWithClient(ctx, ts)
		if err != nil {
			return catalogErrMsg{err}
		}
		pingAll(ctx, servers)
		return catalogLoadedMsg{servers, client}
	}
}

// featuredResolvedMsg carries the catalog with experience/event rows resolved
// to real host:port (and pinged) after a ^r press.
type featuredResolvedMsg struct{ servers []franchise.Server }

// featuredHasUnresolved reports whether any experience/event row still lacks a
// concrete address (so ^r has something to do).
func featuredHasUnresolved(servers []franchise.Server) bool {
	for _, s := range servers {
		if !s.HasAddress() && (s.Kind == franchise.KindPartnerExperience || s.Kind == franchise.KindGathering) {
			return true
		}
	}
	return false
}

// resolveExperiencesCmd resolves every experience-join / live-event row to a
// real host:port and pings it, so the featured list can show actual IPs and
// online status instead of placeholders. Rows that can't resolve (offline / no
// active venue) are left as-is.
func resolveExperiencesCmd(client *franchise.Client, servers []franchise.Server) tea.Cmd {
	return func() tea.Msg {
		out := append([]franchise.Server(nil), servers...)
		ctx, cancel := context.WithTimeout(context.Background(), featuredAPITimeout)
		defer cancel()
		for i := range out {
			s := &out[i]
			if s.HasAddress() || (s.Kind != franchise.KindPartnerExperience && s.Kind != franchise.KindGathering) {
				continue
			}
			addr, err := resolveAddress(ctx, client, *s)
			if err != nil {
				continue
			}
			host, portStr, err := net.SplitHostPort(addr)
			if err != nil {
				continue
			}
			port, err := strconv.Atoi(portStr)
			if err != nil {
				continue
			}
			s.Host = host
			s.Port = port
		}
		pingAll(ctx, out)
		return featuredResolvedMsg{servers: out}
	}
}

func (m appModel) Init() tea.Cmd { return nil }

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case catalogLoadedMsg:
		if m.screen == screenLoading {
			m.fServers = msg.servers
			m.fClient = msg.client
			m.featured = newTUIModel(msg.servers)
			m.screen = screenFeatured
		}
		return m, nil
	case catalogErrMsg:
		if m.screen == screenLoading {
			m.loadErr = msg.err
			m.screen = screenMenu
		}
		return m, nil
	case featuredResolvedMsg:
		if m.resolvingFeatured {
			m.resolvingFeatured = false
			m.fServers = msg.servers
			m.featured.servers = msg.servers // same indices, so cursor/picks hold
			m.featured.applyFilter()
		}
		return m, nil
	case resolvedMsg:
		return m.onResolved(msg)
	case jobStartedMsg:
		return m.onJobStarted(msg)
	case runEvent:
		if m.screen != screenRunning {
			return m, nil
		}
		if msg.transient {
			m.statusLn = msg.text
		} else {
			m.logLines = append(m.logLines, msg.text)
		}
		if m.runCh != nil {
			return m, waitRun(m.runCh)
		}
		return m, nil
	case jobFinishedMsg:
		return m.onJobFinished(msg)
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m appModel) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.String() == "ctrl+c" {
		if m.runProc != nil {
			_ = killProcess(m.runProc)
		}
		return m, tea.Quit
	}
	m.note = "" // any key clears a transient confirmation; setters re-set it below
	switch m.screen {
	case screenMenu:
		return m.handleMenuKey(key)
	case screenLoading:
		if back(key) {
			m.screen = screenMenu
		}
	case screenFeatured:
		return m.handleFeaturedKey(key)
	case screenSaved:
		return m.handleListKey(key, false)
	case screenRecent:
		return m.handleListKey(key, true)
	case screenDecrypt:
		return m.handleDecryptKey(key)
	case screenAddress:
		return m.handleAddressKey(key)
	case screenAction:
		return m.handleActionKey(key)
	case screenRunning:
		return m.handleRunningKey(key)
	case screenDone:
		return m.toMenu(), nil // any key returns to the menu
	}
	return m, nil
}

// back reports whether key means "go back a level" (esc or left arrow);
// forward means "drill in / confirm" (enter or right arrow). Left/right are
// deliberately not treated this way inside the text field, where they'd risk
// discarding typed input.
func back(key tea.KeyMsg) bool    { return key.String() == "esc" || key.String() == "left" }
func forward(key tea.KeyMsg) bool { return key.String() == "enter" || key.String() == "right" }

func (m appModel) handleMenuKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "q":
		return m, tea.Quit
	case "up":
		if m.menuCursor > 0 {
			m.menuCursor--
		}
	case "down":
		if m.menuCursor < len(sections)-1 {
			m.menuCursor++
		}
	}
	if forward(key) {
		switch sections[m.menuCursor].target {
		case screenFeatured:
			m.loadErr = nil
			if m.fServers != nil { // reuse the catalog from an earlier visit
				m.featured = newTUIModel(m.fServers)
				m.screen = screenFeatured
				return m, nil
			}
			m.screen = screenLoading
			return m, loadCatalogCmd(m.ts)
		case screenSaved:
			m.list = newAddrList(m.store.Saved)
			m.screen = screenSaved
		case screenRecent:
			m.list = newAddrList(m.store.Recent)
			m.screen = screenRecent
		case screenDecrypt:
			m.openDecrypt()
		case screenAddress:
			m.addr = addrModel{}
			m.screen = screenAddress
		}
	}
	return m, nil
}

func (m appModel) handleFeaturedKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	// esc/←/enter/→ are handled here so the embedded model's tea.Quit doesn't
	// leak out and quit the whole program.
	switch {
	case key.String() == "ctrl+r":
		// Resolve every experience/event row to a real host:port + status.
		if !m.resolvingFeatured && m.fClient != nil && featuredHasUnresolved(m.fServers) {
			m.resolvingFeatured = true
			return m, resolveExperiencesCmd(m.fClient, m.fServers)
		}
		return m, nil
	case back(key):
		if m.featured.filter != "" {
			m.featured.filter = ""
			m.featured.applyFilter()
		} else {
			m.screen = screenMenu
		}
	case forward(key):
		idxs := m.featured.confirmedIndices()
		if len(idxs) == 0 {
			return m, nil
		}
		m.jobs = make([]job, 0, len(idxs))
		for _, idx := range idxs {
			m.jobs = append(m.jobs, job{label: m.fServers[idx].Name, server: &m.fServers[idx]})
		}
		m.enterAction(screenFeatured)
	default:
		updated, _ := m.featured.Update(key)
		m.featured = updated.(tuiModel)
	}
	return m, nil
}

func (m appModel) handleListKey(key tea.KeyMsg, isRecent bool) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "up":
		m.list.moveUp()
	case "down":
		m.list.moveDown()
	case " ":
		m.list.toggle()
	case "d":
		if addr, ok := m.list.current(); ok {
			if isRecent {
				m.store.removeRecent(addr)
				m.list = newAddrList(m.store.Recent)
			} else {
				m.store.removeSaved(addr)
				m.list = newAddrList(m.store.Saved)
			}
		}
		return m, nil
	case "s":
		if isRecent {
			if addr, ok := m.list.current(); ok {
				m.store.addSaved(addr)
				m.note = "saved " + addr
			}
		}
		return m, nil
	}
	switch {
	case back(key):
		m.screen = screenMenu
	case forward(key):
		addrs := m.list.confirmed()
		if len(addrs) == 0 {
			return m, nil
		}
		m.jobs = make([]job, 0, len(addrs))
		for _, a := range addrs {
			m.jobs = append(m.jobs, job{label: a, address: a})
		}
		m.enterAction(m.screen)
	}
	return m, nil
}

func (m appModel) handleAddressKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.screen = screenMenu
	case "enter":
		addr, err := validateAddress(m.addr.value)
		if err != nil {
			m.addr.err = err.Error()
		} else {
			m.store.addRecent(addr)
			m.jobs = []job{{label: addr, address: addr}}
			m.enterAction(screenAddress)
		}
	case "ctrl+s":
		addr, err := validateAddress(m.addr.value)
		if err != nil {
			m.addr.err = err.Error()
		} else {
			m.store.addSaved(addr)
			m.note = "saved " + addr
		}
	case "backspace":
		if m.addr.value != "" {
			m.addr.value = m.addr.value[:len(m.addr.value)-1]
			m.addr.err = ""
		}
	default:
		if key.Type == tea.KeyRunes {
			m.addr.value += string(key.Runes)
			m.addr.err = ""
		}
	}
	return m, nil
}

func (m appModel) handleActionKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "up":
		if m.actionCursor > 0 {
			m.actionCursor--
		}
	case "down":
		if m.actionCursor < len(actionChoices)-1 {
			m.actionCursor++
		}
	}
	switch {
	case back(key):
		m.screen = m.actionFrom
	case forward(key):
		return m.startRun(m.jobs, actionChoices[m.actionCursor].value)
	}
	return m, nil
}

// startRun resets the live state and kicks off the queue of jobs.
func (m appModel) startRun(jobs []job, act action) (appModel, tea.Cmd) {
	// Runs launched straight from the decrypt section set their own
	// breadcrumb origin (the action picker already set actionFrom otherwise).
	if m.screen == screenDecrypt {
		m.actionFrom = screenDecrypt
	}
	m.jobs = jobs
	m.act = act
	m.screen = screenRunning
	m.jobIdx = 0
	m.logLines = nil
	m.results = nil
	m.statusLn = ""
	m.paused = false
	m.canceled = false
	m.runSkipped = 0
	m.resolveCancel = nil
	return m.beginJob()
}

func (m appModel) handleRunningKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case back(key):
		// Cancel the queue: kill the current child (or abort an in-flight
		// resolve) and let the resulting message route us to the summary.
		m.canceled = true
		if m.runProc != nil {
			_ = killProcess(m.runProc)
		}
		if m.resolveCancel != nil {
			m.resolveCancel()
			m.resolveCancel = nil
		}
	case key.String() == "p":
		if pauseSupported && m.runProc != nil {
			if m.paused {
				_ = resumeProcess(m.runProc)
			} else {
				_ = suspendProcess(m.runProc)
			}
			m.paused = !m.paused
		}
	}
	return m, nil
}

// enterAction moves to the action picker, remembering where to go on back.
func (m *appModel) enterAction(from screen) {
	m.actionFrom = from
	m.actionCursor = 0
	m.screen = screenAction
}

// beginJob advances to jobs[jobIdx], kicking off a resolve (featured rows)
// or the child process (ready addresses). When the queue is exhausted - or a
// cancel is pending - it shows the summary.
func (m appModel) beginJob() (appModel, tea.Cmd) {
	if m.canceled || m.jobIdx >= len(m.jobs) {
		m.screen = screenDone
		m.statusLn = ""
		return m, nil
	}
	j := m.jobs[m.jobIdx]
	m.logLines = append(m.logLines, "-- "+j.label+" --")
	if j.argv != nil {
		m.statusLn = "Starting..."
		return m, runArgvCmd(j.argv)
	}
	if j.address == "" && j.server != nil {
		m.statusLn = "Resolving address..."
		ctx, cancel := context.WithTimeout(context.Background(), featuredAPITimeout)
		m.resolveCancel = cancel
		return m, resolveJobCmd(ctx, m.fClient, *j.server)
	}
	m.markDecryptOut(j.address)
	m.statusLn = "Starting..."
	return m, runArgvCmd(m.act.args(j.address))
}

// markDecryptOut records where the current job's decrypted packs will land,
// mirroring what the download child computes, so the Done summary can show
// the exact path. Only meaningful for the download+decrypt action.
func (m *appModel) markDecryptOut(addr string) {
	if m.act != actionDownloadDecrypt {
		return
	}
	if cwd, err := os.Getwd(); err == nil {
		m.jobs[m.jobIdx].outDir = decryptOutBase(cwd, addr)
	}
}

func (m appModel) onResolved(msg resolvedMsg) (tea.Model, tea.Cmd) {
	if m.screen != screenRunning {
		return m, nil
	}
	if m.resolveCancel != nil { // resolve finished; release its context
		m.resolveCancel()
		m.resolveCancel = nil
	}
	if m.canceled {
		return m.beginJob() // routes to the summary
	}
	if msg.err != nil {
		m.logLines = append(m.logLines, "[err] "+msg.err.Error())
		m.results = append(m.results, jobResult{label: m.jobs[m.jobIdx].label, err: msg.err})
		m.jobIdx++
		return m.beginJob()
	}
	m.jobs[m.jobIdx].address = msg.address
	m.markDecryptOut(msg.address)
	m.statusLn = "Starting..."
	return m, runArgvCmd(m.act.args(msg.address))
}

func (m appModel) onJobStarted(msg jobStartedMsg) (tea.Model, tea.Cmd) {
	if m.screen != screenRunning || m.canceled {
		_ = killProcess(msg.proc)
		// Nobody will read this child's channel, so drain it to completion
		// (streamChild closes it after the final message) to free its pipe FDs.
		go func(c chan tea.Msg) {
			for range c { //nolint:revive // intentional drain-to-close
			}
		}(msg.ch)
		if m.screen == screenRunning {
			m.screen = screenDone
			m.statusLn = ""
		}
		return m, nil
	}
	m.runProc = msg.proc
	m.runCh = msg.ch
	return m, waitRun(msg.ch)
}

func (m appModel) onJobFinished(msg jobFinishedMsg) (tea.Model, tea.Cmd) {
	if m.screen != screenRunning {
		return m, nil
	}
	if m.canceled {
		// The child was killed on purpose - don't record it as a failure or
		// persist a "failed" status; just show the summary of what completed.
		m.runProc = nil
		m.runCh = nil
		m.paused = false
		m.statusLn = ""
		m.screen = screenDone
		return m, nil
	}
	j := m.jobs[m.jobIdx]
	m.results = append(m.results, jobResult{label: j.label, err: msg.err, outDir: j.outDir})
	if msg.err != nil {
		m.logLines = append(m.logLines, "[err] "+msg.err.Error())
	}
	m.recordOutcome(j, msg.err == nil)
	m.runProc = nil
	m.runCh = nil
	m.paused = false
	m.statusLn = ""
	m.jobIdx++
	return m.beginJob()
}

// recordOutcome persists a finished server job's result: the last status for
// its address, plus where it saved (so the decrypt section can find it).
// Decrypt jobs (argv set) and empty addresses are skipped.
func (m *appModel) recordOutcome(j job, ok bool) {
	if j.argv != nil || j.address == "" {
		return
	}
	m.store.recordStatus(j.address, ok)
	if !ok {
		return
	}
	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	m.store.addDownload(download{
		Label:    j.label,
		Address:  j.address,
		Dir:      cwd,
		KeysFile: filepath.Join(cwd, sanitizeServerAddr(j.address)+keysSuffix),
		When:     nowStamp(),
	})
}

// toMenu resets the per-run state and returns to the main menu.
func (m appModel) toMenu() appModel {
	m.screen = screenMenu
	m.jobs = nil
	m.results = nil
	m.logLines = nil
	m.statusLn = ""
	m.note = ""
	m.paused = false
	m.canceled = false
	return m
}

// --- views ---------------------------------------------------------------

func (m appModel) View() string {
	switch m.screen {
	case screenLoading:
		return m.crumb() + "\n  Loading featured servers...\n"
	case screenFeatured:
		return m.featuredView()
	case screenSaved:
		return m.listView("Nothing saved yet - save an address from Recent or the address screen.", false)
	case screenRecent:
		return m.listView("No recent addresses yet - enter one from the address screen.", true)
	case screenDecrypt:
		return m.decryptView()
	case screenAddress:
		return m.addressView()
	case screenAction:
		return m.actionView()
	case screenRunning:
		return m.runningView()
	case screenDone:
		return m.doneView()
	default:
		return m.menuView()
	}
}

// crumb renders the breadcrumb trail - dim parents, a highlighted current -
// so the user can always see where they are.
func (m appModel) crumb() string {
	trail := []string{"Home"}
	switch m.screen {
	case screenLoading:
		trail = append(trail, sectionTitle(screenFeatured), "Loading")
	case screenFeatured, screenSaved, screenRecent, screenDecrypt, screenAddress:
		trail = append(trail, sectionTitle(m.screen))
	case screenAction:
		trail = append(trail, sectionTitle(m.actionFrom), "Choose an action")
	case screenRunning:
		trail = append(trail, sectionTitle(m.actionFrom), "Working")
	case screenDone:
		trail = append(trail, sectionTitle(m.actionFrom), m.doneTitle())
	}
	// Drop any empty segment (e.g. an unset origin) so the trail never
	// renders a dangling "Home ›  › Working".
	compact := trail[:0]
	for _, t := range trail {
		if t != "" {
			compact = append(compact, t)
		}
	}
	trail = compact

	var b strings.Builder
	b.WriteString("\n  ")
	for i, t := range trail {
		if i > 0 {
			b.WriteString(colorDim + crumbArrow + colorReset)
		}
		if i == len(trail)-1 {
			b.WriteString(colorCyan + t + colorReset)
		} else {
			b.WriteString(colorDim + t + colorReset)
		}
	}
	b.WriteString("\n")
	return b.String()
}

// hint is one key + label pair in the footer.
type hint struct{ key, label string }

func hintBar(hints ...hint) string {
	parts := make([]string, len(hints))
	for i, h := range hints {
		parts[i] = h.key + " " + h.label
	}
	return colorDim + "  " + strings.Join(parts, " · ") + colorReset + "\n"
}

func writeRow(b *strings.Builder, selected bool, text string) {
	if selected {
		b.WriteString(" " + colorCyan + rowCursor + text + colorReset + "\n")
	} else {
		b.WriteString("   " + text + "\n")
	}
}

func (m appModel) menuView() string {
	var b strings.Builder
	b.WriteString(m.crumb())
	b.WriteString("  " + colorDim + "Dump, download, and decrypt Minecraft Bedrock resource packs" + colorReset + "\n\n")
	prevGroup := sections[0].group
	for i, s := range sections {
		if s.group != prevGroup {
			b.WriteString("\n") // air between logical groups
			prevGroup = s.group
		}
		writeRow(&b, i == m.menuCursor, s.label)
	}
	b.WriteString("\n  " + colorDim + sections[m.menuCursor].desc + colorReset + "\n")
	if m.loadErr != nil {
		b.WriteString("\n  " + colorRed + "Could not load featured servers: " + m.loadErr.Error() + colorReset + "\n")
	}
	b.WriteString("\n")
	b.WriteString(hintBar(
		hint{gUp + gDown, "move"},
		hint{gRight + "/" + gEnter, "open"},
		hint{gLeft + "/esc", "quit"},
	))
	return b.String()
}

func (m appModel) featuredView() string {
	var b strings.Builder
	b.WriteString(m.crumb())
	m.featured.width = m.width // for row truncation (m is a copy; render-only)
	b.WriteString(m.featured.View())
	if s, ok := m.featured.selectedServer(); ok {
		b.WriteString("\n  " + colorDim + featuredHelp(s) + colorReset + "\n")
	}
	if m.resolvingFeatured {
		b.WriteString("  " + colorCyan + "Resolving experiences..." + colorReset + "\n")
	}
	b.WriteString("\n")
	hints := []hint{
		{gUp + gDown, "move"},
		{"space", "select"},
		{gRight + "/" + gEnter, "go"},
	}
	if featuredHasUnresolved(m.fServers) {
		hints = append(hints, hint{"^r", "resolve IPs"})
	}
	hints = append(hints, hint{"a-z", "filter"}, hint{gLeft + "/esc", "back"})
	b.WriteString(hintBar(hints...))
	return b.String()
}

// featuredHelp explains the highlighted row, especially the ones whose
// address isn't known until it's resolved.
func featuredHelp(s franchise.Server) string {
	if s.HasAddress() {
		return "Direct address: " + s.Address()
	}
	switch s.Kind {
	case franchise.KindGathering:
		return "Live event - press ^r to resolve its address (or it resolves on download)."
	case franchise.KindPartnerExperience:
		return "Experience server - press ^r to resolve its address (or it resolves on download)."
	}
	return "No public address for this entry."
}

func (m appModel) listView(emptyMsg string, isRecent bool) string {
	var b strings.Builder
	b.WriteString(m.crumb())
	b.WriteString("\n")
	if len(m.list.items) == 0 {
		b.WriteString("  " + colorDim + emptyMsg + colorReset + "\n\n")
		b.WriteString(hintBar(hint{gLeft + "/esc", "back"}))
		return b.String()
	}
	for i, addr := range m.list.items {
		box := "[ ]"
		if m.list.picked[i] {
			box = "[x]"
		}
		// Wrap only the box+address in the selection color so the status
		// keeps its own green/red; append it after.
		cell := clip(box+" "+addr, m.width-3)
		if i == m.list.cursor {
			b.WriteString(" " + colorCyan + rowCursor + cell + colorReset)
		} else {
			b.WriteString("   " + cell)
		}
		if st := m.recentStatusLabel(addr); st != "" {
			b.WriteString("   " + st)
		}
		b.WriteString("\n")
	}
	if m.note != "" {
		b.WriteString("\n  " + colorGreen + m.note + colorReset + "\n")
	}
	b.WriteString("\n")
	hints := []hint{
		{gUp + gDown, "move"},
		{"space", "select"},
		{gRight + "/" + gEnter, "go"},
	}
	if isRecent {
		hints = append(hints, hint{"s", "save"})
	}
	hints = append(hints, hint{"d", "delete"}, hint{gLeft + "/esc", "back"})
	b.WriteString(hintBar(hints...))
	return b.String()
}

func (m appModel) addressView() string {
	var b strings.Builder
	b.WriteString(m.crumb())
	b.WriteString("\n  Server address: " + colorCyan + m.addr.value + colorReset + caret + "\n")
	if m.addr.err != "" {
		b.WriteString("  " + colorRed + m.addr.err + colorReset + "\n")
	}
	if m.note != "" {
		b.WriteString("  " + colorGreen + m.note + colorReset + "\n")
	}
	b.WriteString("  " + colorDim + "Example: play.example.net:19132 or 1.2.3.4:19132" + colorReset + "\n\n")
	b.WriteString(hintBar(
		hint{gEnter, "continue"},
		hint{"^s", "save"},
		hint{"esc", "back"},
	))
	return b.String()
}

func (m appModel) actionView() string {
	var b strings.Builder
	b.WriteString(m.crumb())
	b.WriteString("\n  " + colorDim + "Action for " + pluralServers(len(m.jobs)) + colorReset + "\n\n")
	for i, c := range actionChoices {
		writeRow(&b, i == m.actionCursor, c.label)
	}
	b.WriteString("\n  " + colorDim + actionChoices[m.actionCursor].desc + colorReset + "\n\n")
	b.WriteString(hintBar(
		hint{gUp + gDown, "move"},
		hint{gRight + "/" + gEnter, "start"},
		hint{gLeft + "/esc", "back"},
	))
	return b.String()
}

func (m appModel) runningView() string {
	var b strings.Builder
	b.WriteString(m.crumb())
	b.WriteString("  " + colorDim + fmt.Sprintf("job %d/%d", min(m.jobIdx+1, len(m.jobs)), len(m.jobs)) + colorReset + "\n\n")

	start := 0
	if len(m.logLines) > runLogTail {
		start = len(m.logLines) - runLogTail
	}
	for _, ln := range m.logLines[start:] {
		b.WriteString("  " + m.truncate(ln) + "\n")
	}
	switch {
	case m.canceled:
		b.WriteString("  " + colorYellow + "[canceling]" + colorReset + "\n")
	case m.statusLn != "" && m.paused:
		b.WriteString("  " + colorYellow + "[paused] " + m.truncate(m.statusLn) + colorReset + "\n")
	case m.statusLn != "":
		b.WriteString("  " + colorCyan + m.truncate(m.statusLn) + colorReset + "\n")
	case m.paused:
		b.WriteString("  " + colorYellow + "[paused]" + colorReset + "\n")
	}
	b.WriteString("\n")
	hints := []hint{}
	if pauseSupported {
		label := "pause"
		if m.paused {
			label = "resume"
		}
		hints = append(hints, hint{"p", label})
	}
	hints = append(hints, hint{gLeft + "/esc", "cancel"})
	b.WriteString(hintBar(hints...))
	return b.String()
}

func (m appModel) doneTitle() string {
	if m.canceled {
		return "Canceled"
	}
	return "Done"
}

func (m appModel) doneView() string {
	var b strings.Builder
	b.WriteString(m.crumb())
	b.WriteString("\n")
	ok := 0
	for _, r := range m.results {
		if r.err != nil {
			b.WriteString("  " + colorRed + "[err]" + colorReset + " " + r.label + " - " + r.err.Error() + "\n")
			continue
		}
		ok++
		b.WriteString("  " + colorGreen + "[ok]" + colorReset + "  " + r.label + "\n")
		if r.outDir != "" {
			b.WriteString("        " + colorDim + "decrypted -> " + colorReset + m.truncate(r.outDir) + "\n")
		}
	}
	fmt.Fprintf(&b, "\n  %d/%d succeeded\n", ok, len(m.results))
	if m.runSkipped > 0 {
		fmt.Fprintf(&b, "  %d skipped - needed a download first\n", m.runSkipped)
	}
	if ok > 0 {
		switch {
		case m.actionFrom == screenDecrypt:
			// the per-row "decrypted -> <path>" lines above already show where
		case m.act == actionKeys:
			b.WriteString("  keys saved to the current directory\n")
		default:
			b.WriteString("  downloads in the current directory\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(hintBar(hint{"any key", gRight + " menu"}))
	return b.String()
}

// truncate clips a line to the current terminal width so a long pack name or
// path can't wrap and break the repaint. Lines are ANSI-stripped already.
func (m appModel) truncate(s string) string {
	return clip(s, m.width-2) // the two-space indent View adds
}

// clip shortens plain (ANSI-free) text to at most width bytes, appending "..."
// when it must cut. A width <= 0 disables clipping. Measured in bytes, which
// slightly over-counts multibyte glyphs - acceptable for a wrap guard.
func clip(s string, width int) string {
	if width <= 0 || len(s) <= width {
		return s
	}
	if width < 4 {
		return s[:width]
	}
	return s[:width-3] + "..."
}

func pluralServers(n int) string {
	if n == 1 {
		return "1 server"
	}
	return fmt.Sprintf("%d servers", n)
}

// --- address field -------------------------------------------------------

// addrModel is the host:port text field's data (rendered by addressView).
type addrModel struct {
	value string
	err   string
}

// validateAddress accepts a host:port with a numeric 0-65535 port and a
// non-empty host. IPv6 literals must be bracketed, like the Bedrock client.
func validateAddress(s string) (string, error) {
	s = strings.TrimSpace(s)
	host, port, err := net.SplitHostPort(s)
	if err != nil || host == "" {
		return "", fmt.Errorf("expected host:port, e.g. play.example.net:19132")
	}
	if _, perr := strconv.ParseUint(port, 10, 16); perr != nil {
		return "", fmt.Errorf("expected host:port, e.g. play.example.net:19132")
	}
	return s, nil
}

// --- saved / recent list -------------------------------------------------

// addrListModel is a filter-free, multi-select picker over plain addresses,
// shared by the Saved and Recent screens.
type addrListModel struct {
	items  []string
	cursor int
	picked map[int]bool
}

func newAddrList(items []string) addrListModel {
	return addrListModel{items: items, picked: map[int]bool{}}
}

func (m *addrListModel) moveUp() {
	if m.cursor > 0 {
		m.cursor--
	}
}

func (m *addrListModel) moveDown() {
	if m.cursor < len(m.items)-1 {
		m.cursor++
	}
}

func (m *addrListModel) toggle() {
	if len(m.items) == 0 {
		return
	}
	if m.picked[m.cursor] {
		delete(m.picked, m.cursor)
	} else {
		m.picked[m.cursor] = true
	}
}

func (m addrListModel) current() (string, bool) {
	if m.cursor >= 0 && m.cursor < len(m.items) {
		return m.items[m.cursor], true
	}
	return "", false
}

// confirmedIndices returns the picked row indices, or the cursor row when nothing
// is explicitly selected.
func (m addrListModel) confirmedIndices() []int {
	if len(m.picked) > 0 {
		idx := make([]int, 0, len(m.picked))
		for i := range m.picked {
			idx = append(idx, i)
		}
		sort.Ints(idx)
		return idx
	}
	if len(m.items) > 0 {
		return []int{m.cursor}
	}
	return nil
}

// confirmed maps confirmedIndices through the (string) items.
func (m addrListModel) confirmed() []string {
	idx := m.confirmedIndices()
	out := make([]string, len(idx))
	for k, i := range idx {
		out[k] = m.items[i]
	}
	return out
}

// recentStatusLabel renders the last run outcome (ok/failed + age) for an
// address, or "" if it was never run.
func (m appModel) recentStatusLabel(addr string) string {
	st, ok := m.store.Status[addr]
	if !ok {
		return ""
	}
	label := colorGreen + "ok" + colorReset
	if !st.OK {
		label = colorRed + "failed" + colorReset
	}
	if age := ageLabel(st.LastUsed); age != "" {
		label += colorDim + " · " + age + colorReset
	}
	return label
}

// --- decrypt section -----------------------------------------------------

// decryptState is the live on-disk picture of a remembered download.
type decryptState struct {
	packs     int  // pack folders that still contain a contents.json
	hasKeys   bool // the keys file is present
	decrypted bool // a decrypted/<server>/ output already exists
}

func (d decryptState) decryptable() bool { return d.packs > 0 && d.hasKeys }

// inspectDownload reads the filesystem to see what's actually present now.
func inspectDownload(d download) decryptState {
	st := decryptState{}
	if d.KeysFile != "" {
		if _, err := os.Stat(d.KeysFile); err == nil {
			st.hasKeys = true
		}
	}
	entries, err := os.ReadDir(d.Dir)
	if err != nil {
		return st
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(d.Dir, e.Name(), contentsJSON)); err == nil {
			st.packs++
		}
	}
	// Already-decrypted output (re-decrypting just overwrites it, so this is
	// only a hint, not a lock).
	if out, err := os.ReadDir(decryptOutBase(d.Dir, d.Address)); err == nil && len(out) > 0 {
		st.decrypted = true
	}
	return st
}

// openDecrypt loads the remembered downloads and their current on-disk state.
func (m *appModel) openDecrypt() {
	m.dls = append([]download(nil), m.store.Downloads...)
	m.dlStates = make([]decryptState, len(m.dls))
	labels := make([]string, len(m.dls))
	for i, d := range m.dls {
		m.dlStates[i] = inspectDownload(d)
		labels[i] = d.Label
	}
	m.list = newAddrList(labels)
	m.screen = screenDecrypt
}

func (m appModel) currentDownload() (int, bool) {
	if m.list.cursor >= 0 && m.list.cursor < len(m.dls) {
		return m.list.cursor, true
	}
	return 0, false
}

func (m appModel) handleDecryptKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "up":
		m.list.moveUp()
		return m, nil
	case "down":
		m.list.moveDown()
		return m, nil
	case " ":
		m.list.toggle()
		return m, nil
	case "d":
		if i, ok := m.currentDownload(); ok {
			m.store.removeDownload(m.dls[i])
			m.openDecrypt()
		}
		return m, nil
	case "g":
		if i, ok := m.currentDownload(); ok {
			if m.dls[i].Address == "" {
				m.note = "no saved address for this entry - press d to forget it"
				return m, nil
			}
			d := m.dls[i]
			return m.startRun([]job{{label: d.Label, address: d.Address}}, actionDownloadDecrypt)
		}
		return m, nil
	}
	switch {
	case back(key):
		m.screen = screenMenu
	case forward(key):
		picked := m.list.confirmedIndices()
		var jobs []job
		for _, i := range picked {
			d, st := m.dls[i], m.dlStates[i]
			if !st.decryptable() {
				continue
			}
			out := decryptOutBase(d.Dir, d.Address)
			jobs = append(jobs, job{
				label:  d.Label,
				argv:   []string{"decrypt", "--all", d.KeysFile, d.Dir, out},
				outDir: out,
			})
		}
		if len(jobs) == 0 {
			m.note = "nothing to decrypt - need both packs and a keys file (press g to fetch)"
			return m, nil
		}
		nm, cmd := m.startRun(jobs, actionDownloadDecrypt)
		nm.runSkipped = len(picked) - len(jobs) // surfaced on the done screen
		return nm, cmd
	}
	return m, nil
}

func (m appModel) decryptView() string {
	var b strings.Builder
	b.WriteString(m.crumb())
	b.WriteString("\n")
	if len(m.dls) == 0 {
		b.WriteString("  " + colorDim + "Nothing downloaded yet - download a server first, then come back to decrypt." + colorReset + "\n\n")
		b.WriteString(hintBar(hint{gLeft + "/esc", "back"}))
		return b.String()
	}
	for i, d := range m.dls {
		box := "[ ]"
		if m.list.picked[i] {
			box = "[x]"
		}
		cell := clip(box+" "+fmt.Sprintf("%-18s", d.Label), m.width-3)
		if i == m.list.cursor {
			b.WriteString(" " + colorCyan + rowCursor + cell + colorReset)
		} else {
			b.WriteString("   " + cell)
		}
		b.WriteString("  " + decryptBadge(m.dlStates[i]) + "\n")
	}
	if i, ok := m.currentDownload(); ok {
		b.WriteString("\n  " + colorDim + decryptHelp(m.dlStates[i], m.dls[i].Address != "") + colorReset + "\n")
	}
	if m.note != "" {
		b.WriteString("  " + colorGreen + m.note + colorReset + "\n")
	}
	b.WriteString("\n")
	b.WriteString(hintBar(
		hint{gUp + gDown, "move"},
		hint{"space", "select"},
		hint{gRight + "/" + gEnter, "decrypt"},
		hint{"g", "download"},
		hint{"d", "forget"},
		hint{gLeft + "/esc", "back"},
	))
	return b.String()
}

func decryptBadge(st decryptState) string {
	keys := colorRed + "no keys" + colorReset
	if st.hasKeys {
		keys = colorGreen + "keys" + colorReset
	}
	badge := colorDim + fmt.Sprintf("%d packs · ", st.packs) + colorReset + keys
	if st.decrypted {
		badge += colorDim + " · decrypted" + colorReset
	}
	return badge
}

func decryptHelp(st decryptState, hasAddr bool) string {
	switch {
	case st.decryptable():
		return fmt.Sprintf("Decrypt %d packs - output lands in a sibling _decrypted folder.", st.packs)
	case st.packs > 0 && !st.hasKeys:
		if hasAddr {
			return "Packs are here but the keys file is missing - press g to fetch keys + packs."
		}
		return "Packs are here but the keys file is missing, and no saved address to fetch from."
	case st.packs == 0 && st.hasKeys:
		if hasAddr {
			return "Keys are here but the packs are gone - press g to download them again."
		}
		return "Keys are here but the packs are gone, and no saved address to re-download."
	default:
		if hasAddr {
			return "Nothing on disk anymore - press g to re-download, or d to forget this entry."
		}
		return "Nothing on disk anymore - press d to forget this entry."
	}
}

// --- featured list -------------------------------------------------------

// tuiModel is the featured-list screen: a filterable, multi-select picker.
type tuiModel struct {
	servers  []franchise.Server
	filtered []int        // indices into servers passing the current filter
	cursor   int          // position within filtered
	filter   string       // case-insensitive name substring
	picked   map[int]bool // chosen server indices (into servers)
	width    int          // terminal width, for row truncation (set by featuredView)
}

func newTUIModel(servers []franchise.Server) tuiModel {
	m := tuiModel{servers: servers, picked: map[int]bool{}}
	m.applyFilter()
	return m
}

func (m *tuiModel) applyFilter() {
	q := strings.ToLower(m.filter)
	m.filtered = m.filtered[:0]
	for i, s := range m.servers {
		if q == "" || strings.Contains(strings.ToLower(s.Name), q) {
			m.filtered = append(m.filtered, i)
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

// confirmedIndices returns the picked servers, or the one under the cursor
// when nothing is explicitly selected.
func (m tuiModel) confirmedIndices() []int {
	if len(m.picked) > 0 {
		out := make([]int, 0, len(m.picked))
		for i := range m.picked {
			out = append(out, i)
		}
		sort.Ints(out)
		return out
	}
	if len(m.filtered) > 0 {
		return []int{m.filtered[m.cursor]}
	}
	return nil
}

func (m tuiModel) selectedServer() (franchise.Server, bool) {
	if len(m.filtered) == 0 {
		return franchise.Server{}, false
	}
	return m.servers[m.filtered[m.cursor]], true
}

func (m tuiModel) Init() tea.Cmd { return nil }

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down":
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
	case " ":
		if len(m.filtered) > 0 {
			idx := m.filtered[m.cursor]
			if m.picked[idx] {
				delete(m.picked, idx)
			} else {
				m.picked[idx] = true
			}
		}
	case "backspace":
		if m.filter != "" {
			m.filter = m.filter[:len(m.filter)-1]
			m.applyFilter()
		}
	default:
		if key.Type == tea.KeyRunes {
			m.filter += string(key.Runes)
			m.applyFilter()
		}
	}
	return m, nil
}

// View renders just the filter line and rows; the breadcrumb and footer are
// supplied by appModel.featuredView.
func (m tuiModel) View() string {
	var b strings.Builder
	if m.filter != "" {
		b.WriteString("  " + colorDim + "filter: " + m.filter + colorReset + "\n")
		if hidden := m.hiddenPicks(); hidden > 0 {
			b.WriteString("  " + colorDim + fmt.Sprintf("%d selected (%d hidden by filter)", len(m.picked), hidden) + colorReset + "\n")
		}
	}
	b.WriteString("\n")
	if len(m.filtered) == 0 {
		if m.filter == "" {
			b.WriteString("   " + colorDim + "No featured servers right now - try again later." + colorReset + "\n")
		} else {
			b.WriteString("   (no servers match)\n")
		}
		return b.String()
	}
	for vi, idx := range m.filtered {
		s := m.servers[idx]
		box := "[ ]"
		if m.picked[idx] {
			box = "[x]"
		}
		row := clip(fmt.Sprintf("%s %-18s  %-30s  %s", box, s.Name, addressColumn(s), statusFor(s)), m.width-3)
		if vi == m.cursor {
			b.WriteString(" " + colorCyan + rowCursor + row + colorReset + "\n")
		} else {
			b.WriteString("   " + row + "\n")
		}
	}
	return b.String()
}

// hiddenPicks counts selected servers not visible under the current filter, so
// the user isn't surprised that a filtered-out pick still runs.
func (m tuiModel) hiddenPicks() int {
	if len(m.picked) == 0 {
		return 0
	}
	visible := make(map[int]bool, len(m.filtered))
	for _, idx := range m.filtered {
		visible[idx] = true
	}
	hidden := 0
	for idx := range m.picked {
		if !visible[idx] {
			hidden++
		}
	}
	return hidden
}
