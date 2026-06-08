package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/iteplenky/bedrock-pack-tools/v3/internal/franchise"
)

// isInteractive reports whether stdin is a terminal (not a pipe/file), so
// the bubbletea menu only launches when a human can drive it.
func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// runTUI fetches the featured catalog, lets the user pick a server from an
// arrow-key menu, then downloads and decrypts the chosen one in one go.
// Entered when the binary is run with no command on an interactive terminal.
func runTUI() error {
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	tokenSource, err := getTokenSource()
	if err != nil {
		return err
	}

	fmt.Println()
	sp := startSpinner("Fetching catalog")
	servers, client, err := fetchFeaturedListWithClient(sigCtx, tokenSource)
	sp.stop("")
	if err != nil {
		return err
	}
	if len(servers) == 0 {
		fmt.Println("  No featured servers returned by the API.")
		return nil
	}

	sp = startSpinner(fmt.Sprintf("Pinging %d servers", len(servers)))
	pingAll(sigCtx, servers)
	sp.stop("")

	final, err := tea.NewProgram(tuiModel{servers: servers, selected: -1}, tea.WithAltScreen()).Run()
	if err != nil {
		return fmt.Errorf("interactive menu: %w", err)
	}
	chosen := final.(tuiModel).selected
	if chosen < 0 {
		return nil // user quit without choosing
	}

	s := servers[chosen]
	resolveCtx, cancel := context.WithTimeout(sigCtx, featuredAPITimeout)
	defer cancel()
	address, err := resolveAddress(resolveCtx, client, s)
	if err != nil {
		return err
	}
	fmt.Printf("\n  [->] %s  ->  %s\n", s.Name, address)
	return runDownload([]string{"--decrypt", address})
}

// tuiModel is a single-screen list picker over the featured catalog.
type tuiModel struct {
	servers  []franchise.Server
	cursor   int
	selected int // index of the chosen server, or -1 if the user quit
}

func (m tuiModel) Init() tea.Cmd { return nil }

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "ctrl+c", "q", "esc":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.servers)-1 {
			m.cursor++
		}
	case "enter":
		m.selected = m.cursor
		return m, tea.Quit
	}
	return m, nil
}

func (m tuiModel) View() string {
	var b strings.Builder
	b.WriteString("\n  Featured Servers   ")
	b.WriteString("↑/↓ move · ↵ download+decrypt · q quit\n\n")
	for i, s := range m.servers {
		row := fmt.Sprintf("%-18s  %-30s  %s", s.Name, addressColumn(s), statusFor(s))
		if i == m.cursor {
			b.WriteString(" " + colorCyan + "▸ " + row + colorReset + "\n")
		} else {
			b.WriteString("   " + row + "\n")
		}
	}
	b.WriteString("\n")
	return b.String()
}
