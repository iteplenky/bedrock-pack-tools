package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/iteplenky/bedrock-pack-tools/v3/internal/franchise"
	"golang.org/x/oauth2"
)

// isInteractive reports whether stdin is a terminal (not a pipe/file), so
// the menu only launches when a human can drive it.
func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// runTUI opens the sectioned interactive menu. Xbox auth happens up front
// (so any device-code prompt is visible before the alt-screen takes over);
// the featured catalog is then loaded lazily only if that section is chosen.
// On exit it hands the chosen address(es) to the existing download path.
func runTUI() error {
	ts, err := getTokenSource()
	if err != nil {
		return err
	}

	out, err := tea.NewProgram(newAppModel(ts), tea.WithAltScreen()).Run()
	if err != nil {
		return fmt.Errorf("interactive menu: %w", err)
	}
	app := out.(appModel)

	if app.doAddress != "" {
		fmt.Printf("\n  [->] %s\n", app.doAddress)
		return runDownload([]string{"--decrypt", app.doAddress})
	}
	if len(app.doFeatured) == 0 {
		return nil // user quit without choosing
	}

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	for k, idx := range app.doFeatured {
		s := app.fServers[idx]
		fmt.Printf("\n  [%d/%d] %s\n", k+1, len(app.doFeatured), s.Name)
		rctx, cancel := context.WithTimeout(sigCtx, featuredAPITimeout)
		address, rerr := resolveAddress(rctx, app.fClient, s)
		cancel()
		if rerr != nil {
			fmt.Fprintf(os.Stderr, "  %v\n", rerr)
			continue
		}
		if derr := runDownload([]string{"--decrypt", address}); derr != nil {
			fmt.Fprintf(os.Stderr, "  %v\n", derr)
		}
	}
	return nil
}

type screen int

const (
	screenMenu    screen = iota
	screenLoading        // transient: featured catalog fetch + ping in flight
	screenFeatured
	screenAddress
)

// section is one row of the main menu.
type section struct {
	label  string
	target screen
}

var sections = []section{
	{"Featured servers", screenFeatured},
	{"Enter a server address", screenAddress},
	// Future: decrypt a local pack folder, dump keys only.
}

// catalog messages drive the one async section (Featured needs the network).
type catalogLoadedMsg struct {
	servers []franchise.Server
	client  *franchise.Client
}
type catalogErrMsg struct{ err error }

// appModel is the root state machine: a main menu that delegates to the
// featured list or the address field, then exposes the chosen target.
type appModel struct {
	screen     screen
	menuCursor int
	ts         oauth2.TokenSource
	featured   tuiModel // featured-list state (embedded, used on screenFeatured)
	addr       addrModel
	loadErr    error

	// handoff fields read by runTUI after Run() returns:
	doFeatured []int
	fServers   []franchise.Server
	fClient    *franchise.Client
	doAddress  string
}

func newAppModel(ts oauth2.TokenSource) appModel {
	return appModel{screen: screenMenu, ts: ts}
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

func (m appModel) Init() tea.Cmd { return nil }

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case catalogLoadedMsg:
		if m.screen == screenLoading {
			m.featured = newTUIModel(msg.servers)
			m.fServers = msg.servers
			m.fClient = msg.client
			m.screen = screenFeatured
		}
		return m, nil
	case catalogErrMsg:
		if m.screen == screenLoading {
			m.loadErr = msg.err
			m.screen = screenMenu
		}
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m appModel) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.String() == "ctrl+c" {
		return m, tea.Quit
	}
	switch m.screen {
	case screenMenu:
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
		case "enter":
			switch sections[m.menuCursor].target {
			case screenFeatured:
				m.screen = screenLoading
				m.loadErr = nil
				return m, loadCatalogCmd(m.ts)
			case screenAddress:
				m.screen = screenAddress
				m.addr = addrModel{}
			}
		}
	case screenLoading:
		if key.String() == "esc" {
			m.screen = screenMenu
		}
	case screenFeatured:
		// esc/enter are handled here so the embedded model's tea.Quit
		// doesn't leak out and quit the whole program on a back-press.
		switch key.String() {
		case "esc":
			if m.featured.filter != "" {
				m.featured.filter = ""
				m.featured.applyFilter()
			} else {
				m.screen = screenMenu
			}
		case "enter":
			m.doFeatured = m.featured.confirmedIndices()
			return m, tea.Quit
		default:
			updated, _ := m.featured.Update(key)
			m.featured = updated.(tuiModel)
		}
	case screenAddress:
		switch key.String() {
		case "esc":
			m.screen = screenMenu
		case "enter":
			addr, err := validateAddress(m.addr.value)
			if err != nil {
				m.addr.err = err.Error()
			} else {
				m.doAddress = addr
				return m, tea.Quit
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
	}
	return m, nil
}

func (m appModel) View() string {
	switch m.screen {
	case screenLoading:
		return "\n  Loading featured servers...\n"
	case screenFeatured:
		return m.featured.View()
	case screenAddress:
		return m.addr.view()
	default:
		return m.menuView()
	}
}

func (m appModel) menuView() string {
	var b strings.Builder
	b.WriteString("\n  bedrock-pack-tools\n")
	b.WriteString("  ↑/↓ move · ↵ select · esc quit\n\n")
	for i, s := range sections {
		if i == m.menuCursor {
			b.WriteString(" " + colorCyan + "▸ " + s.label + colorReset + "\n")
		} else {
			b.WriteString("   " + s.label + "\n")
		}
	}
	if m.loadErr != nil {
		b.WriteString("\n  " + colorRed + "Could not load featured servers: " + m.loadErr.Error() + colorReset + "\n")
	}
	b.WriteString("\n")
	return b.String()
}

// addrModel is a single-line host:port text field.
type addrModel struct {
	value string
	err   string
}

func (a addrModel) view() string {
	var b strings.Builder
	b.WriteString("\n  Enter a server address\n")
	b.WriteString("  type host:port · ↵ download+decrypt · esc back\n\n")
	b.WriteString("  Server address: " + colorCyan + a.value + colorReset + "▌\n")
	if a.err != "" {
		b.WriteString("  " + colorRed + a.err + colorReset + "\n")
	}
	b.WriteString("\n")
	return b.String()
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

// tuiModel is the featured-list screen: a filterable, multi-select picker.
type tuiModel struct {
	servers   []franchise.Server
	filtered  []int        // indices into servers passing the current filter
	cursor    int          // position within filtered
	filter    string       // case-insensitive name substring
	picked    map[int]bool // chosen server indices (into servers)
	confirmed []int        // set on enter: servers to download, in order
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

func (m tuiModel) Init() tea.Cmd { return nil }

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.filter != "" {
			m.filter = ""
			m.applyFilter()
			return m, nil
		}
		return m, tea.Quit
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
	case "enter":
		m.confirmed = m.confirmedIndices()
		return m, tea.Quit
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

func (m tuiModel) View() string {
	var b strings.Builder
	b.WriteString("\n  Featured Servers\n")
	b.WriteString("  ↑/↓ move · space select · ↵ download+decrypt · type to filter · esc back\n")
	if m.filter != "" {
		b.WriteString("  filter: " + m.filter + "\n")
	}
	b.WriteString("\n")
	if len(m.filtered) == 0 {
		b.WriteString("   (no servers match)\n\n")
		return b.String()
	}
	for vi, idx := range m.filtered {
		s := m.servers[idx]
		box := "[ ]"
		if m.picked[idx] {
			box = "[x]"
		}
		row := fmt.Sprintf("%s %-18s  %-30s  %s", box, s.Name, addressColumn(s), statusFor(s))
		if vi == m.cursor {
			b.WriteString(" " + colorCyan + "▸ " + row + colorReset + "\n")
		} else {
			b.WriteString("   " + row + "\n")
		}
	}
	b.WriteString("\n")
	return b.String()
}
