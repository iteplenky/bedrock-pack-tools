package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/iteplenky/bedrock-pack-tools/v3/internal/franchise"
)

func TestTuiModel_NavigateAndSelect(t *testing.T) {
	servers := []franchise.Server{{Name: "alpha"}, {Name: "bravo"}, {Name: "charlie"}}
	var m tea.Model = tuiModel{servers: servers, selected: -1}

	down := tea.KeyMsg{Type: tea.KeyDown}
	m, _ = m.Update(down)
	m, _ = m.Update(down)
	if got := m.(tuiModel).cursor; got != 2 {
		t.Fatalf("cursor after two downs = %d, want 2", got)
	}
	// Past the end clamps.
	m, _ = m.Update(down)
	if got := m.(tuiModel).cursor; got != 2 {
		t.Fatalf("cursor should clamp at last index, got %d", got)
	}
	// Enter selects the cursor and signals quit.
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := m.(tuiModel).selected; got != 2 {
		t.Fatalf("enter selected = %d, want 2", got)
	}
	if cmd == nil {
		t.Fatal("enter should return a quit command")
	}
}

func TestTuiModel_QuitWithoutSelecting(t *testing.T) {
	var m tea.Model = tuiModel{servers: []franchise.Server{{Name: "alpha"}}, selected: -1}
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if got := m.(tuiModel).selected; got != -1 {
		t.Fatalf("esc should leave selected = -1, got %d", got)
	}
	if cmd == nil {
		t.Fatal("esc should return a quit command")
	}
}

func TestTuiModel_ViewListsServers(t *testing.T) {
	m := tuiModel{servers: []franchise.Server{{Name: "alpha"}, {Name: "bravo"}}, selected: -1}
	view := m.View()
	for _, name := range []string{"alpha", "bravo"} {
		if !strings.Contains(view, name) {
			t.Errorf("view missing server %q", name)
		}
	}
}
