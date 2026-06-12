package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/iteplenky/bedrock-pack-tools/v3/internal/franchise"
	"github.com/iteplenky/bedrock-pack-tools/v3/internal/lang"
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
	screenEncrypt // encrypt a local pack folder into a .mcpack
	screenAddress
	screenSettings // sign in / out + maintenance actions
	screenAction   // pick download / download+decrypt / keys for the chosen targets
	screenRunning  // a job queue executing, with live output
	screenDone     // per-target summary, any key returns to the menu
)

// section is one row of the main menu, with a help line shown when it's
// highlighted. group lays out related rows together with a blank-line break.
// labelKey/descKey are catalog keys resolved at render time (not at package
// init, where the catalog isn't registered yet).
type section struct {
	labelKey string
	descKey  string
	target   screen
	group    int
}

func (s section) label() string { return lang.T(s.labelKey) }
func (s section) desc() string  { return lang.T(s.descKey) }

// Ordered by workflow and grouped with blank-line breaks: pick a server to
// pull packs from, then your own saved/recent servers, then work with what
// you've already downloaded.
var sections = []section{
	{"tui.section.featured.label", "tui.section.featured.desc", screenFeatured, 0},
	{"tui.section.address.label", "tui.section.address.desc", screenAddress, 0},
	{"tui.section.saved.label", "tui.section.saved.desc", screenSaved, 1},
	{"tui.section.recent.label", "tui.section.recent.desc", screenRecent, 1},
	{"tui.section.decrypt.label", "tui.section.decrypt.desc", screenDecrypt, 2},
	{"tui.section.encrypt.label", "tui.section.encrypt.desc", screenEncrypt, 2},
	{"tui.section.settings.label", "tui.section.settings.desc", screenSettings, 3},
}

func sectionTitle(target screen) string {
	for _, s := range sections {
		if s.target == target {
			return s.label()
		}
	}
	return ""
}

// confirmKind identifies a pending y/n confirmation (a destructive account or
// settings action). confirmNone means nothing is pending.
type confirmKind int

const (
	confirmNone confirmKind = iota
	confirmLogout
	confirmClearAddrs
	confirmClearDownloads
	confirmResetCohort
)

func confirmPrompt(k confirmKind) string {
	switch k {
	case confirmLogout:
		return lang.T("tui.confirm.logout")
	case confirmClearAddrs:
		return lang.T("tui.confirm.clearAddrs")
	case confirmClearDownloads:
		return lang.T("tui.confirm.clearDownloads")
	case confirmResetCohort:
		return lang.T("tui.confirm.resetCohort")
	}
	return ""
}

// settingsItem is a row on the Settings screen. Maintenance rows arm a y/n
// confirm; the sign-in row (signIn=true) hands off to the device flow with no
// confirm; the language row (langToggle=true) flips EN/RU in place. group lays
// related rows out together with a blank-line break. labelKey/descKey resolve
// through the catalog at render time.
type settingsItem struct {
	labelKey   string
	descKey    string
	confirm    confirmKind
	signIn     bool
	langToggle bool
	group      int
}

func (it settingsItem) label() string {
	if it.langToggle {
		return lang.Tf("tui.settings.language.label", langDisplayName())
	}
	return lang.T(it.labelKey)
}
func (it settingsItem) desc() string { return lang.T(it.descKey) }

// langDisplayName is the active language's own name, always shown in its own
// script (never translated), so the toggle reads "Language: English" /
// "Язык: Русский".
func langDisplayName() string {
	if lang.Current() == lang.Russian {
		return "Русский"
	}
	return "English"
}

// maintenanceItems are the destructive housekeeping rows, shown between the
// language toggle and the sign-in/out row regardless of auth state.
var maintenanceItems = []settingsItem{
	{"tui.settings.clearAddrs.label", "tui.settings.clearAddrs.desc", confirmClearAddrs, false, false, 0},
	{"tui.settings.clearDownloads.label", "tui.settings.clearDownloads.desc", confirmClearDownloads, false, false, 0},
	{"tui.settings.resetCohort.label", "tui.settings.resetCohort.desc", confirmResetCohort, false, false, 0},
}

// settingsRows is the live Settings list: the language toggle and maintenance
// rows in the first group, then a state-aware sign-in/out row last in its own
// group (you're always signed in when the menu opens, so it reads "Sign out"
// until you sign out in-session).
func settingsRows(signedIn bool) []settingsItem {
	language := settingsItem{labelKey: "tui.settings.language.label", descKey: "tui.settings.language.desc", confirm: confirmNone, langToggle: true, group: 0}
	auth := settingsItem{labelKey: "tui.settings.signIn.label", descKey: "tui.settings.signIn.desc", confirm: confirmNone, signIn: true, group: 1}
	if signedIn {
		auth = settingsItem{labelKey: "tui.settings.signOut.label", descKey: "tui.settings.signOut.desc", confirm: confirmLogout, group: 1}
	}
	rows := []settingsItem{language}
	rows = append(rows, maintenanceItems...)
	return append(rows, auth)
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
	height     int
	store      store

	featured          tuiModel      // featured-list state (screenFeatured)
	list              addrListModel // saved/recent/decrypt cursor + selection
	addr              textField     // screenAddress input
	enc               textField     // screenEncrypt pack-path input
	loadErr           error
	resolvingFeatured bool   // a ^r bulk experience/event resolve is in flight
	note              string // transient confirmation (e.g. "saved"), cleared on next key
	noteErr           bool   // the note is a failure (render it red, not green)

	// settings cursor + a shared y/n confirm for destructive actions.
	settingsCursor int
	confirm        confirmKind

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
		m.height = msg.Height
		// Repaint from a clean slate: some terminals leave stale cells behind
		// when the grid is resized (e.g. font-zoom that reflows rows/cols).
		return m, tea.ClearScreen
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
			// Count rows that were unresolved before and now carry an address,
			// so ^r gives feedback instead of silently swapping the list.
			gained := 0
			for i := range msg.servers {
				if i < len(m.fServers) {
					old := m.fServers[i]
					wasUnresolved := !old.HasAddress() &&
						(old.Kind == franchise.KindPartnerExperience || old.Kind == franchise.KindGathering)
					if wasUnresolved && msg.servers[i].HasAddress() {
						gained++
					}
				}
			}
			if gained > 0 {
				m.note = lang.Tf("tui.note.resolved", pluralServers(gained))
			} else {
				m.note, m.noteErr = lang.T("tui.note.noResolved"), true
			}
			m.fServers = msg.servers
			m.featured.servers = msg.servers // same indices, so cursor/picks hold
			m.featured.applyFilter()
		}
		return m, nil
	case loginDoneMsg:
		if msg.err == nil && loadToken() != nil {
			if ts, err := getTokenSource(); err == nil {
				m.ts = ts // refresh the in-process source after a fresh sign-in
			}
			m.note = lang.T("tui.note.signedIn")
		} else {
			m.note = lang.T("tui.note.signInIncomplete")
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
		} else if line, ok := cleanLogLine(msg.text); ok {
			m.logLines = append(m.logLines, line)
			if m.statusLn == lang.T("tui.status.starting") {
				m.statusLn = "" // the first committed line is the live signal now
			}
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
	m.note, m.noteErr = "", false // any key clears a transient note; setters re-set it below
	if m.confirm != confirmNone {
		if key.String() == "y" {
			return m.runConfirmed()
		}
		m.confirm = confirmNone // n / esc / any other key cancels
		return m, nil
	}
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
	case screenEncrypt:
		return m.handleEncryptKey(key)
	case screenAddress:
		return m.handleAddressKey(key)
	case screenSettings:
		return m.handleSettingsKey(key)
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
	case "esc", "q", "left": // left = back; at the top level that means quit
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
		m.loadErr = nil // any drill-in dismisses a stale "could not load featured servers" banner
		switch sections[m.menuCursor].target {
		case screenFeatured:
			if m.ts == nil { // signed out: nothing to authenticate the catalog fetch
				m.settingsCursor = len(settingsRows(false)) - 1 // lands on the Sign in row (now last)
				m.confirm = confirmNone
				m.note = lang.T("tui.note.signInFirstFeatured")
				m.screen = screenSettings
				return m, nil
			}
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
		case screenEncrypt:
			m.enc = textField{}
			m.screen = screenEncrypt
		case screenAddress:
			m.addr = textField{}
			m.screen = screenAddress
		case screenSettings:
			m.settingsCursor = len(settingsRows(false)) - 1 // lands on the Sign in row (now last)
			m.confirm = confirmNone
			m.screen = screenSettings
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
		// Delete acts on the picked set (or the cursor row when nothing is
		// picked), matching what enter/go operate on.
		addrs := m.list.confirmed()
		for _, addr := range addrs {
			if isRecent {
				m.store.removeRecent(addr)
			} else {
				m.store.removeSaved(addr)
			}
		}
		if len(addrs) > 0 {
			if isRecent {
				m.list = newAddrList(m.store.Recent)
			} else {
				m.list = newAddrList(m.store.Saved)
			}
			m.note = lang.Tf("tui.note.forgot", pluralAddrs(len(addrs)))
		}
		return m, nil
	case "s":
		if isRecent {
			addrs := m.list.confirmed()
			for _, addr := range addrs {
				m.store.addSaved(addr)
			}
			if len(addrs) > 0 {
				m.note = lang.Tf("tui.note.saved", pluralAddrs(len(addrs)))
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
	case "left":
		if m.addr.value == "" {
			m.screen = screenMenu // empty field: left exits, like every other screen
		} else if m.addr.cursor > 0 {
			m.addr.cursor-- // otherwise move the caret through the text
		}
	case "right":
		if m.addr.cursor < len([]rune(m.addr.value)) {
			m.addr.cursor++
		}
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
			m.note = lang.Tf("tui.note.savedAddr", addr)
		}
	case "backspace":
		m.addr.deleteBack()
	default:
		if key.Type == tea.KeyRunes {
			m.addr.insert(string(key.Runes))
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

func (m appModel) handleSettingsKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	rows := settingsRows(loadToken() != nil)
	switch key.String() {
	case "up":
		if m.settingsCursor > 0 {
			m.settingsCursor--
		}
	case "down":
		if m.settingsCursor < len(rows)-1 {
			m.settingsCursor++
		}
	}
	switch {
	case back(key):
		m.screen = screenMenu
	case forward(key):
		it := rows[min(m.settingsCursor, len(rows)-1)]
		switch {
		case it.langToggle:
			next := lang.English
			if lang.Current() == lang.English {
				next = lang.Russian
			}
			lang.SetActive(next)
			m.store.setLanguage(next.String())
			m.note = lang.Tf("tui.note.languageChanged", langDisplayName())
		case it.signIn:
			return m, loginCmd() // hands the terminal to the device-code flow
		default:
			m.confirm = it.confirm
		}
	}
	return m, nil
}

// runConfirmed performs the action the pending y/n confirm was guarding.
func (m appModel) runConfirmed() (tea.Model, tea.Cmd) {
	switch m.confirm {
	case confirmLogout:
		wasSignedIn := loadToken() != nil
		if err := clearAuthCaches(); err != nil {
			m.note, m.noteErr = lang.Tf("tui.note.logoutFailed", err.Error()), true
		} else {
			// Drop the in-memory session too, not just the disk caches: the
			// refresh-token source and franchise client would otherwise keep
			// minting tokens (silently re-writing the cache we just deleted).
			m.ts = nil
			m.fClient = nil
			m.fServers = nil
			m.featured = tuiModel{}
			if wasSignedIn {
				m.note = lang.T("tui.note.signedOut")
			} else {
				m.note = lang.T("tui.note.alreadySignedOut")
			}
		}
	case confirmClearAddrs:
		m.store.clearAddresses()
		m.note = lang.T("tui.note.clearedAddrs")
	case confirmClearDownloads:
		m.store.clearDownloads()
		m.note = lang.T("tui.note.clearedDownloads")
	case confirmResetCohort:
		if err := resetDeviceID(); err != nil {
			m.note, m.noteErr = lang.Tf("tui.note.resetFailed", err.Error()), true
		} else {
			// Drop the cached catalog + client so the next Featured open builds a
			// fresh client (new device id, no seeded token) and shows the new
			// cohort - otherwise the reset wouldn't take effect until next launch.
			m.fServers = nil
			m.fClient = nil
			m.note = lang.T("tui.note.cohortReset")
		}
	}
	m.confirm = confirmNone
	return m, nil
}

func (m appModel) handleEncryptKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.screen = screenMenu
	case "left":
		if m.enc.value == "" {
			m.screen = screenMenu
		} else if m.enc.cursor > 0 {
			m.enc.cursor--
		}
	case "right":
		if m.enc.cursor < len([]rune(m.enc.value)) {
			m.enc.cursor++
		}
	case "enter":
		path, err := validatePackDir(m.enc.value)
		if err != nil {
			m.enc.err = err.Error()
		} else {
			return m.startRun([]job{{label: filepath.Base(path), argv: []string{"encrypt", path}}}, actionDownload)
		}
	case "backspace":
		m.enc.deleteBack()
	default:
		if key.Type == tea.KeyRunes {
			m.enc.insert(string(key.Runes))
		}
	}
	return m, nil
}

// validatePackDir mirrors the encrypt command's entry guard (the path must be a
// folder holding a manifest.json), so an obviously bad path is caught in-menu.
// Deeper checks - a valid header.uuid, at least one encryptable file - run in
// the child and surface on the Done screen.
func validatePackDir(s string) (string, error) {
	s = strings.TrimRight(strings.TrimSpace(s), "/\\")
	if s == "" {
		return "", fmt.Errorf("%s", lang.T("tui.validate.enterPath"))
	}
	if fi, err := os.Stat(s); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("%s", lang.T("tui.validate.notFolder"))
	}
	if _, err := os.Stat(filepath.Join(s, manifestJSON)); err != nil {
		return "", fmt.Errorf("%s", lang.T("tui.validate.noManifest"))
	}
	return s, nil
}

// startRun resets the live state and kicks off the queue of jobs.
func (m appModel) startRun(jobs []job, act action) (appModel, tea.Cmd) {
	// download/keys jobs re-exec an auth-requiring child; encrypt/decrypt run
	// offline. When signed out the auth child would stall on a device-code
	// prompt buried in the capture pipe, so route the user to Settings instead.
	if m.ts == nil {
		for _, j := range jobs {
			if j.needsAuth() {
				m.settingsCursor = len(settingsRows(false)) - 1 // lands on the Sign in row (now last)
				m.confirm = confirmNone
				m.note = lang.T("tui.note.signInFirstRun")
				m.screen = screenSettings
				return m, nil
			}
		}
	}
	// Runs launched straight from the decrypt/encrypt sections set their own
	// breadcrumb origin (the action picker already set actionFrom otherwise).
	if m.screen == screenDecrypt || m.screen == screenEncrypt {
		m.actionFrom = m.screen
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
		if pauseSupported && !m.canceled && m.runProc != nil {
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
		m.statusLn = lang.T("tui.status.starting")
		return m, runArgvCmd(j.argv)
	}
	if j.address == "" && j.server != nil {
		m.statusLn = lang.T("tui.status.resolving")
		ctx, cancel := context.WithTimeout(context.Background(), featuredAPITimeout)
		m.resolveCancel = cancel
		return m, resolveJobCmd(ctx, m.fClient, *j.server)
	}
	m.markDecryptOut(j.address)
	m.statusLn = lang.T("tui.status.starting")
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
		m.logLines = append(m.logLines, lang.Tf("tui.log.err", msg.err.Error()))
		m.results = append(m.results, jobResult{label: m.jobs[m.jobIdx].label, err: msg.err})
		m.jobIdx++
		return m.beginJob()
	}
	m.jobs[m.jobIdx].address = msg.address
	m.markDecryptOut(msg.address)
	m.statusLn = lang.T("tui.status.starting")
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
	// Only claim "decrypted -> X" if that output actually exists; a plain
	// (unencrypted) server produces no decrypted folder.
	if j.outDir != "" {
		if _, err := os.Stat(j.outDir); err != nil {
			j.outDir = ""
		}
	}
	switch {
	case msg.err == nil:
		m.results = append(m.results, jobResult{label: j.label, outDir: j.outDir})
		m.recordOutcome(j, true)
	case errors.Is(msg.err, errPartialResult):
		// Exit 2: packs or keys genuinely landed, just not the whole run.
		// Keep the decrypt-section entry, but don't claim a clean success.
		m.results = append(m.results, jobResult{label: j.label, partial: true, detail: msg.detail, outDir: j.outDir})
		if msg.detail != "" {
			m.logLines = append(m.logLines, lang.Tf("tui.log.partial", msg.detail))
		}
		m.recordOutcome(j, true)
	default:
		resErr := msg.err
		if msg.detail != "" {
			resErr = fmt.Errorf("%s", msg.detail) // the child's real last line, not just "exited with code N"
		}
		m.results = append(m.results, jobResult{label: j.label, err: resErr, outDir: j.outDir})
		m.logLines = append(m.logLines, lang.Tf("tui.log.err", resErr.Error()))
		m.recordOutcome(j, false)
	}
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
	// download groups each server's output under <cwd>/<server>/, so the
	// decrypt section is anchored on that folder, not the bare cwd.
	serverDir := filepath.Join(cwd, sanitizeServerAddr(j.address))
	keysFile := filepath.Join(serverDir, keysFileName)
	// Only list it in the decrypt section if a keys file landed - i.e. the
	// server shipped encrypted packs. Plain packs need no decryption.
	if _, err := os.Stat(keysFile); err != nil {
		return
	}
	m.store.addDownload(download{
		Label:    j.label,
		Address:  j.address,
		Dir:      serverDir,
		KeysFile: keysFile,
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
	m.noteErr = false
	m.paused = false
	m.canceled = false
	return m
}

// --- views ---------------------------------------------------------------

func (m appModel) View() string {
	switch m.screen {
	case screenLoading:
		return m.crumb() + "\n  " + lang.T("tui.loading.featured") + "\n\n" + hintBar(hint{gLeft + "/esc", lang.T("tui.hint.cancel")})
	case screenFeatured:
		return m.featuredView()
	case screenSaved:
		return m.listView(lang.T("tui.empty.saved"), false)
	case screenRecent:
		return m.listView(lang.T("tui.empty.recent"), true)
	case screenDecrypt:
		return m.decryptView()
	case screenEncrypt:
		return m.encryptView()
	case screenAddress:
		return m.addressView()
	case screenSettings:
		return m.settingsView()
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
	trail := []string{lang.T("tui.crumb.home")}
	switch m.screen {
	case screenLoading:
		trail = append(trail, sectionTitle(screenFeatured), lang.T("tui.crumb.loading"))
	case screenFeatured, screenSaved, screenRecent, screenDecrypt, screenEncrypt, screenAddress, screenSettings:
		trail = append(trail, sectionTitle(m.screen))
	case screenAction:
		trail = append(trail, sectionTitle(m.actionFrom), lang.T("tui.crumb.chooseAction"))
	case screenRunning:
		trail = append(trail, sectionTitle(m.actionFrom), lang.T("tui.crumb.working"))
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

// rgbTo256 maps a 24-bit color to the nearest xterm-256 color-cube index, for
// terminals that don't advertise truecolor.
func rgbTo256(c [3]uint8) int {
	q := func(v uint8) int { return (int(v)*5 + 127) / 255 }
	return 16 + 36*q(c[0]) + 6*q(c[1]) + q(c[2])
}

// pxCell renders one icon cell as a half-block: the top pixel above the bottom
// pixel inside a single (roughly square) terminal cell. '.' is transparent and
// shows the terminal background, so the sprite floats. Color is quantized to the
// xterm-256 cube so it renders identically with or without truecolor; callers
// reset color at row end. Under NO_COLOR it degrades to a plain silhouette.
func pxCell(top, bottom byte) string {
	topT, botT := top == '.', bottom == '.'
	if iconColor == "none" {
		switch {
		case topT && botT:
			return " "
		case botT:
			return "▀"
		case topT:
			return "▄"
		default:
			return "█"
		}
	}
	switch {
	case topT && botT:
		return "\033[0m "
	case botT: // top pixel only -> upper half, default background below
		return fmt.Sprintf("\033[49;38;5;%dm▀", rgbTo256(iconPx[top]))
	case topT: // bottom pixel only -> lower half, default background above
		return fmt.Sprintf("\033[49;38;5;%dm▄", rgbTo256(iconPx[bottom]))
	default:
		return fmt.Sprintf("\033[38;5;%d;48;5;%dm▀",
			rgbTo256(iconPx[top]), rgbTo256(iconPx[bottom]))
	}
}

// menuHeader is the Home banner: the menu mascot - a little knight raising a key
// (it unlocks the packs) - beside the wordmark and tagline. Half-block glyphs
// pack two pixel rows into each terminal cell, so the 16x16 sprite reads square
// in an 8-cell-tall band and floats on the terminal background.
func menuHeader() string {
	px := []string{
		".....cC.........",
		"....oCco........",
		"...oLLLLo.......",
		"..oLLLLLLo.ggg..",
		"..oLoLLoLo.g.g..",
		"..oLLLLLLo.ggg..",
		"..oLooooLo..G...",
		"...oLLLLo...GG..",
		"..oDDDDDDo..G...",
		".oSSLDDLSSo.....",
		"oSSSLDDLSSSo....",
		"oSSSSSSSSSSo....",
		".oSDSSSSDSo.....",
		".oSSo..oSSo.....",
		".oDDo..oDDo.....",
		".oLL....LLo.....",
	}
	labels := []string{
		"",
		"",
		"",
		colorCyan + "bedrock-pack-tools" + colorReset,
		colorDim + lang.T("tui.header.tagline1") + colorReset,
		colorDim + lang.T("tui.header.tagline2") + colorReset,
		"",
		"",
	}
	var b strings.Builder
	b.WriteString("\n")
	for i := 0; i < len(px)/2; i++ {
		top, bottom := px[2*i], px[2*i+1]
		b.WriteString("   ")
		for x := 0; x < len(top); x++ {
			b.WriteString(pxCell(top[x], bottom[x]))
		}
		b.WriteString(colorReset)
		if labels[i] != "" {
			b.WriteString("   " + labels[i])
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	return b.String()
}

func (m appModel) menuView() string {
	var b strings.Builder
	b.WriteString(m.crumb())
	b.WriteString(menuHeader())
	prevGroup := sections[0].group
	for i, s := range sections {
		if s.group != prevGroup {
			b.WriteString("\n") // air between logical groups
			prevGroup = s.group
		}
		writeRow(&b, i == m.menuCursor, s.label())
	}
	b.WriteString("\n  " + colorDim + sections[m.menuCursor].desc() + colorReset + "\n")
	if m.loadErr != nil {
		if d, ok := humanize(m.loadErr); ok {
			var buf strings.Builder
			writeDiagnostic(&buf, d, m.loadErr)
			b.WriteString(buf.String())
		} else {
			b.WriteString("\n  " + colorRed + lang.Tf("tui.error.loadFeatured", m.loadErr.Error()) + colorReset + "\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(hintBar(
		hint{gUp + gDown, lang.T("tui.hint.move")},
		hint{gRight + "/" + gEnter, lang.T("tui.hint.open")},
		hint{gLeft + "/esc/q", lang.T("tui.hint.quit")},
	))
	return b.String()
}

func (m appModel) featuredView() string {
	var b strings.Builder
	b.WriteString(m.crumb())
	m.featured.width = m.width // for row truncation (m is a copy; render-only)
	b.WriteString(m.featured.View())
	// The per-row help describes the cursor row, which is only the action target
	// when nothing is picked (enter acts on the picked set otherwise).
	if s, ok := m.featured.selectedServer(); ok && len(m.featured.picked) == 0 {
		b.WriteString("\n  " + colorDim + featuredHelp(s) + colorReset + "\n")
	}
	if m.resolvingFeatured {
		b.WriteString("  " + colorCyan + lang.T("tui.featured.resolving") + colorReset + "\n")
	}
	if m.note != "" {
		noteColor := colorGreen
		if m.noteErr {
			noteColor = colorRed
		}
		b.WriteString("  " + noteColor + m.note + colorReset + "\n")
	}
	b.WriteString("\n")
	if len(m.featured.servers) == 0 {
		// Empty catalog - only back does anything; move/space/continue/filter are no-ops.
		b.WriteString(hintBar(hint{gLeft + "/esc", lang.T("tui.hint.back")}))
		return b.String()
	}
	hints := []hint{
		{gUp + gDown, lang.T("tui.hint.move")},
		{"space", lang.T("tui.hint.select")},
		{gRight + "/" + gEnter, lang.T("tui.hint.continue")},
	}
	if featuredHasUnresolved(m.fServers) {
		hints = append(hints, hint{"^r", lang.T("tui.hint.resolveIPs")})
	}
	hints = append(hints, hint{"type", lang.T("tui.hint.filter")})
	if m.featured.filter != "" {
		hints = append(hints, hint{gLeft + "/esc", lang.T("tui.hint.clearFilter")})
	} else {
		hints = append(hints, hint{gLeft + "/esc", lang.T("tui.hint.back")})
	}
	b.WriteString(hintBar(hints...))
	return b.String()
}

// featuredHelp explains the highlighted row, especially the ones whose
// address isn't known until it's resolved.
func featuredHelp(s franchise.Server) string {
	if s.HasAddress() {
		return lang.Tf("tui.featuredHelp.direct", s.Address())
	}
	switch s.Kind {
	case franchise.KindGathering:
		return lang.T("tui.featuredHelp.liveEvent")
	case franchise.KindPartnerExperience:
		return lang.T("tui.featuredHelp.experience")
	}
	return lang.T("tui.featuredHelp.none")
}

func (m appModel) listView(emptyMsg string, isRecent bool) string {
	var b strings.Builder
	b.WriteString(m.crumb())
	b.WriteString("\n")
	if len(m.list.items) == 0 {
		b.WriteString("  " + colorDim + emptyMsg + colorReset + "\n\n")
		b.WriteString(hintBar(hint{gLeft + "/esc", lang.T("tui.hint.back")}))
		return b.String()
	}
	for i, addr := range m.list.items {
		box := "[ ]"
		if m.list.picked[i] {
			box = "[x]"
		}
		// Wrap only the box+address in the selection color so the status
		// keeps its own green/red; append it after. The fixed-width address
		// lines the status badges into a column, like the decrypt list.
		cell := clip(box+" "+fmt.Sprintf("%-24s", addr), m.width-3)
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
		{gUp + gDown, lang.T("tui.hint.move")},
		{"space", lang.T("tui.hint.select")},
		{gRight + "/" + gEnter, lang.T("tui.hint.continue")},
	}
	if isRecent {
		hints = append(hints, hint{"s", lang.T("tui.hint.save")})
	}
	hints = append(hints, hint{"d", lang.T("tui.hint.forget")}, hint{gLeft + "/esc", lang.T("tui.hint.back")})
	b.WriteString(hintBar(hints...))
	return b.String()
}

func (m appModel) addressView() string {
	var b strings.Builder
	b.WriteString(m.crumb())
	r := []rune(m.addr.value)
	c := min(m.addr.cursor, len(r))
	b.WriteString("\n  " + lang.T("tui.address.label") + colorCyan + string(r[:c]) + caret + string(r[c:]) + colorReset + "\n")
	if m.addr.err != "" {
		b.WriteString("  " + colorRed + m.addr.err + colorReset + "\n")
	}
	if m.note != "" {
		b.WriteString("  " + colorGreen + m.note + colorReset + "\n")
	}
	b.WriteString("  " + colorDim + lang.T("tui.address.example") + colorReset + "\n\n")
	b.WriteString(hintBar(
		hint{gEnter, lang.T("tui.hint.continue")},
		hint{gLeft + gRight, lang.T("tui.hint.moveCaret")},
		hint{"^s", lang.T("tui.hint.save")},
		hint{"esc", lang.T("tui.hint.back")},
	))
	return b.String()
}

func (m appModel) encryptView() string {
	var b strings.Builder
	b.WriteString(m.crumb())
	r := []rune(m.enc.value)
	c := min(m.enc.cursor, len(r))
	b.WriteString("\n  " + lang.T("tui.encrypt.label") + colorCyan + string(r[:c]) + caret + string(r[c:]) + colorReset + "\n")
	if m.enc.err != "" {
		b.WriteString("  " + colorRed + m.enc.err + colorReset + "\n")
	}
	b.WriteString("  " + colorDim + lang.T("tui.encrypt.example") + colorReset + "\n\n")
	b.WriteString(hintBar(
		hint{gEnter, lang.T("tui.hint.encrypt")},
		hint{gLeft + gRight, lang.T("tui.hint.moveCaret")},
		hint{"esc", lang.T("tui.hint.back")},
	))
	return b.String()
}

func (m appModel) settingsView() string {
	var b strings.Builder
	b.WriteString(m.crumb())
	b.WriteString("\n")
	signedIn := loadToken() != nil
	if signedIn {
		b.WriteString("  " + colorGreen + lang.T("tui.settings.signedIn") + colorReset + "\n")
	} else {
		b.WriteString("  " + colorYellow + lang.T("tui.settings.notSignedIn") + colorReset + "\n")
	}
	if dir, ok := configDir(); ok {
		b.WriteString("  " + colorDim + lang.Tf("tui.settings.config", dir) + colorReset + "\n")
	}
	b.WriteString("\n")
	rows := settingsRows(signedIn)
	cursor := min(m.settingsCursor, len(rows)-1)
	prevGroup := rows[0].group
	for i, it := range rows {
		if it.group != prevGroup {
			b.WriteString("\n") // air between logical groups
			prevGroup = it.group
		}
		writeRow(&b, i == cursor, it.label())
	}
	b.WriteString("\n  " + colorDim + rows[cursor].desc() + colorReset + "\n")
	if m.confirm != confirmNone {
		b.WriteString("\n  " + colorYellow + confirmPrompt(m.confirm) + colorReset + "\n")
		b.WriteString(hintBar(hint{"y", lang.T("tui.hint.yes")}, hint{"n/esc", lang.T("tui.hint.cancel")}))
		return b.String()
	}
	if m.note != "" {
		noteColor := colorGreen
		if m.noteErr {
			noteColor = colorRed
		}
		b.WriteString("  " + noteColor + m.note + colorReset + "\n")
	}
	b.WriteString("\n")
	b.WriteString(hintBar(
		hint{gUp + gDown, lang.T("tui.hint.move")},
		hint{gRight + "/" + gEnter, lang.T("tui.hint.select")},
		hint{gLeft + "/esc", lang.T("tui.hint.back")},
	))
	return b.String()
}

func (m appModel) actionView() string {
	var b strings.Builder
	b.WriteString(m.crumb())
	b.WriteString("\n  " + colorDim + lang.Tf("tui.action.for", pluralServers(len(m.jobs))) + colorReset + "\n\n")
	for i, c := range actionChoices {
		writeRow(&b, i == m.actionCursor, c.label())
	}
	b.WriteString("\n  " + colorDim + actionChoices[m.actionCursor].desc() + colorReset + "\n\n")
	b.WriteString(hintBar(
		hint{gUp + gDown, lang.T("tui.hint.move")},
		hint{gRight + "/" + gEnter, lang.T("tui.hint.start")},
		hint{gLeft + "/esc", lang.T("tui.hint.back")},
	))
	return b.String()
}

func (m appModel) runningView() string {
	var b strings.Builder
	b.WriteString(m.crumb())
	b.WriteString("  " + colorDim + lang.Tf("tui.running.job", min(m.jobIdx+1, len(m.jobs)), len(m.jobs)) + colorReset + "\n\n")

	// Chrome around the log is about seven rows: the crumb (a blank + the
	// trail), the job line plus a blank, an optional status line, a trailing
	// blank, and the hint bar. Reserve eight for a little slack.
	tail := runLogTail
	if m.height > runLogTail+8 {
		tail = m.height - 8 // fill a tall screen, keeping the chrome on-screen
	}
	if tail < 3 {
		tail = 3 // never collapse the log to nothing on a very short terminal
	}
	start := 0
	if len(m.logLines) > tail {
		start = len(m.logLines) - tail
	}
	for _, ln := range m.logLines[start:] {
		b.WriteString("  " + m.truncate(ln) + "\n")
	}
	switch {
	case m.canceled:
		b.WriteString("  " + colorYellow + lang.T("tui.status.canceling") + colorReset + "\n")
	case m.statusLn != "" && m.paused:
		b.WriteString("  " + colorYellow + lang.Tf("tui.status.pausedWith", m.truncate(m.statusLn)) + colorReset + "\n")
	case m.statusLn != "":
		b.WriteString("  " + colorCyan + m.truncate(m.statusLn) + colorReset + "\n")
	case m.paused:
		b.WriteString("  " + colorYellow + lang.T("tui.status.paused") + colorReset + "\n")
	}
	b.WriteString("\n")
	if m.canceled {
		return b.String() // canceling - no actionable keys left
	}
	hints := []hint{}
	if pauseSupported && m.runProc != nil { // pause is a no-op before the child is running
		label := lang.T("tui.hint.pause")
		if m.paused {
			label = lang.T("tui.hint.resume")
		}
		hints = append(hints, hint{"p", label})
	}
	hints = append(hints, hint{gLeft + "/esc", lang.T("tui.hint.cancel")})
	b.WriteString(hintBar(hints...))
	return b.String()
}

func (m appModel) doneTitle() string {
	if m.canceled {
		return lang.T("tui.done.canceled")
	}
	return lang.T("tui.done.done")
}

func (m appModel) doneView() string {
	var b strings.Builder
	b.WriteString(m.crumb())
	b.WriteString("\n")
	ok, partial := 0, 0
	for _, r := range m.results {
		switch {
		case r.err != nil:
			b.WriteString("  " + colorRed + lang.T("tui.done.err") + colorReset + "     " + r.label + " - " + r.err.Error() + "\n")
		case r.partial:
			partial++
			line := "  " + colorYellow + lang.T("tui.done.partial") + colorReset + " " + r.label
			if r.detail != "" {
				line += " - " + r.detail
			}
			b.WriteString(line + "\n")
		default:
			ok++
			b.WriteString("  " + colorGreen + lang.T("tui.done.ok") + colorReset + "      " + r.label + "\n")
			if r.outDir != "" {
				b.WriteString("        " + colorDim + lang.T("tui.done.decryptedTo") + colorReset + m.truncate(r.outDir) + "\n")
			}
		}
	}
	b.WriteString("\n  " + lang.Tf("tui.done.succeeded", ok, len(m.results)) + "\n")
	if partial > 0 {
		b.WriteString("  " + lang.Tf("tui.done.partialSummary", partial) + "\n")
	}
	if m.runSkipped > 0 {
		b.WriteString("  " + lang.Tf("tui.done.skippedSummary", m.runSkipped) + "\n")
	}
	if ok > 0 {
		switch {
		case m.actionFrom == screenDecrypt:
			// the per-row "decrypted -> <path>" lines above already show where
		case m.actionFrom == screenEncrypt:
			b.WriteString("  " + lang.T("tui.done.encryptWritten") + "\n")
		case m.act == actionKeys:
			b.WriteString("  " + lang.T("tui.done.keysSaved") + "\n")
		default:
			b.WriteString("  " + lang.T("tui.done.downloadsCurrent") + "\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(hintBar(hint{"any key", lang.T("tui.hint.backToMenu")}))
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

// pluralRu picks the Russian plural form for n (one for ...1 not 11, few for
// ...2-4 not 12-14, else many) and formats it with n.
func pluralRu(n int, one, few, many string) string {
	switch {
	case n%10 == 1 && n%100 != 11:
		return fmt.Sprintf(one, n)
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
		return fmt.Sprintf(few, n)
	default:
		return fmt.Sprintf(many, n)
	}
}

func pluralServers(n int) string {
	return pluralRu(n, lang.T("tui.plural.server.one"), lang.T("tui.plural.server.few"), lang.T("tui.plural.server.many"))
}

func pluralPacks(n int) string {
	return pluralRu(n, lang.T("tui.plural.pack.one"), lang.T("tui.plural.pack.few"), lang.T("tui.plural.pack.many"))
}

func pluralAddrs(n int) string {
	return pluralRu(n, lang.T("tui.plural.addr.one"), lang.T("tui.plural.addr.few"), lang.T("tui.plural.addr.many"))
}

// --- address field -------------------------------------------------------

// textField is a one-line editable input (host:port, a pack path, ...).
// cursor is a rune index into value, so left/right move within the text.
type textField struct {
	value  string
	cursor int
	err    string
}

func (a *textField) insert(s string) {
	r, ins := []rune(a.value), []rune(s)
	out := make([]rune, 0, len(r)+len(ins))
	out = append(out, r[:a.cursor]...)
	out = append(out, ins...)
	out = append(out, r[a.cursor:]...)
	a.value, a.cursor, a.err = string(out), a.cursor+len(ins), ""
}

func (a *textField) deleteBack() {
	if a.cursor == 0 {
		return
	}
	r := []rune(a.value)
	a.value = string(append(r[:a.cursor-1], r[a.cursor:]...))
	a.cursor, a.err = a.cursor-1, ""
}

// validateAddress accepts a host:port with a numeric 0-65535 port and a
// non-empty host. IPv6 literals must be bracketed, like the Bedrock client.
func validateAddress(s string) (string, error) {
	s = strings.TrimSpace(s)
	host, port, err := net.SplitHostPort(s)
	if err != nil || host == "" {
		return "", fmt.Errorf("%s", lang.T("tui.validate.expectAddr"))
	}
	if _, perr := strconv.ParseUint(port, 10, 16); perr != nil {
		return "", fmt.Errorf("%s", lang.T("tui.validate.expectAddr"))
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
	label := colorGreen + lang.T("tui.recentStatus.ok") + colorReset
	if !st.OK {
		label = colorRed + lang.T("tui.recentStatus.failed") + colorReset
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
	keys, _ := readKeyMap(d.KeysFile) // nil if the file is absent/corrupt
	st.hasKeys = len(keys) > 0
	if entries, err := os.ReadDir(d.Dir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			packDir := filepath.Join(d.Dir, e.Name())
			if _, err := os.Stat(filepath.Join(packDir, contentsJSON)); err != nil {
				continue // not an encrypted pack folder
			}
			// Count only packs THIS download's keys can actually decrypt, so a
			// shared working directory doesn't mix in other servers' packs.
			if uid, err := readPackUUID(packDir); err == nil {
				if _, ok := keys[uid]; ok {
					st.packs++
				}
			}
		}
	}
	// Already-decrypted output (re-decrypting just overwrites it, so this is
	// only a hint, not a lock). d.Dir is this download's own server folder,
	// so its decrypted/ subdir is unambiguous.
	if out, err := os.ReadDir(filepath.Join(d.Dir, decryptedDir)); err == nil && len(out) > 0 {
		st.decrypted = true
	}
	return st
}

// openDecrypt loads the remembered downloads and their current on-disk state,
// keeping only entries that have a keys file - a plain (keyless) download has
// nothing to decrypt and would just be confusing clutter here.
func (m *appModel) openDecrypt() {
	m.dls = nil
	m.dlStates = nil
	var labels []string
	for _, d := range m.store.Downloads {
		st := inspectDownload(d)
		if !st.hasKeys {
			continue
		}
		m.dls = append(m.dls, d)
		m.dlStates = append(m.dlStates, st)
		labels = append(labels, d.Label)
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
				m.note = lang.T("tui.note.noAddrEntry")
				return m, nil
			}
			d := m.dls[i]
			// Anchor the re-download to this entry's own dir (not the menu's cwd)
			// with an explicit argv, so it refreshes THIS entry in place.
			out := filepath.Join(d.Dir, decryptedDir)
			return m.startRun([]job{{
				label:  d.Label,
				argv:   []string{"download", "--decrypt", d.Address, filepath.Dir(d.Dir)},
				outDir: out,
			}}, actionDownloadDecrypt)
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
			out := filepath.Join(d.Dir, decryptedDir)
			if d.Address == "" {
				// No saved server name to file the output under - use the
				// deterministic <dir>_decrypted sibling instead of the shared
				// decrypted/ parent, which would mix servers together.
				out = defaultDecryptOutBase(d.Dir)
			}
			jobs = append(jobs, job{
				label:  d.Label,
				argv:   []string{"decrypt", "--all", d.KeysFile, d.Dir, out},
				outDir: out,
			})
		}
		if len(jobs) == 0 {
			m.note = lang.T("tui.note.nothingToDecrypt")
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
		b.WriteString("  " + colorDim + lang.T("tui.empty.decrypt") + colorReset + "\n\n")
		b.WriteString(hintBar(hint{gLeft + "/esc", lang.T("tui.hint.back")}))
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
		hint{gUp + gDown, lang.T("tui.hint.move")},
		hint{"space", lang.T("tui.hint.select")},
		hint{gRight + "/" + gEnter, lang.T("tui.hint.decrypt")},
		hint{"g", lang.T("tui.hint.downloadDecrypt")},
		hint{"d", lang.T("tui.hint.forget")},
		hint{gLeft + "/esc", lang.T("tui.hint.back")},
	))
	return b.String()
}

func decryptBadge(st decryptState) string {
	// openDecrypt only lists entries that have a keys file, so keys is always
	// present on this screen.
	badge := colorDim + pluralPacks(st.packs) + " · " + colorReset + colorGreen + lang.T("tui.decrypt.badge.keys") + colorReset
	if st.decrypted {
		badge += colorDim + " · " + colorReset + colorGreen + lang.T("tui.decrypt.badge.decrypted") + colorReset
	}
	return badge
}

// decryptHelp explains the highlighted entry. Keys are guaranteed present here
// (openDecrypt filters to hasKeys), so the only states are: decryptable (with
// or without an existing decrypted output) and packs-gone.
func decryptHelp(st decryptState, hasAddr bool) string {
	switch {
	case st.decryptable() && st.decrypted:
		return lang.Tf("tui.decryptHelp.reDecrypt", pluralPacks(st.packs))
	case st.decryptable():
		return lang.Tf("tui.decryptHelp.decrypt", pluralPacks(st.packs))
	default: // packs gone, keys still on disk
		if hasAddr {
			return lang.T("tui.decryptHelp.packsGone")
		}
		return lang.T("tui.decryptHelp.noAddr")
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
		b.WriteString("  " + colorDim + lang.Tf("tui.featuredList.filter", m.filter) + colorReset + "\n")
		if hidden := m.hiddenPicks(); hidden > 0 {
			b.WriteString("  " + colorDim + lang.Tf("tui.featuredList.selHidden", len(m.picked), hidden) + colorReset + "\n")
		}
	}
	b.WriteString("\n")
	if len(m.filtered) == 0 {
		if m.filter == "" {
			b.WriteString("   " + colorDim + lang.T("tui.featuredList.empty") + colorReset + "\n")
		} else {
			b.WriteString("   " + colorDim + lang.T("tui.featuredList.noMatch") + colorReset + "\n")
		}
		return b.String()
	}
	// Size the name + address columns to the widest entries (over the full set,
	// not the filtered view, so columns don't jump while typing), mirroring the
	// CLI table instead of hard-coding widths that long names overflow.
	nameW, addrW := 4, 7
	for _, s := range m.servers {
		if w := len(s.Name); w > nameW {
			nameW = w
		}
		if w := len(addressColumn(s)); w > addrW {
			addrW = w
		}
	}
	rowFmt := fmt.Sprintf("%%s %%-%ds  %%-%ds", nameW, addrW)
	for vi, idx := range m.filtered {
		s := m.servers[idx]
		box := "[ ]"
		if m.picked[idx] {
			box = "[x]"
		}
		// Clip the box+name+address cell; the status keeps its own state color,
		// appended after (like listView).
		cell := clip(fmt.Sprintf(rowFmt, box, s.Name, addressColumn(s)), m.width-3)
		if vi == m.cursor {
			b.WriteString(" " + colorCyan + rowCursor + cell + colorReset)
		} else {
			b.WriteString("   " + cell)
		}
		b.WriteString("  " + coloredStatusFor(s) + "\n")
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
