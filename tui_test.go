package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/iteplenky/bedrock-pack-tools/v3/internal/franchise"
)

func TestTuiModel_FilterMultiSelect(t *testing.T) {
	servers := []franchise.Server{{Name: "alpha"}, {Name: "bravo"}, {Name: "alto"}}
	var m tea.Model = newTUIModel(servers)

	// Type "al" -> only "alpha" and "alto" remain.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("al")})
	if got := len(m.(tuiModel).filtered); got != 2 {
		t.Fatalf("filter \"al\" matched %d, want 2", got)
	}
	// Select both with space + down.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if got := len(m.(tuiModel).confirmedIndices()); got != 2 {
		t.Fatalf("picked %d servers, want 2", got)
	}
}

func TestTuiModel_EnterPicksCursorWhenNoneSelected(t *testing.T) {
	m := newTUIModel([]franchise.Server{{Name: "alpha"}, {Name: "bravo"}})
	m.cursor = 1
	got := m.confirmedIndices()
	if len(got) != 1 || got[0] != 1 {
		t.Fatalf("confirmedIndices = %v, want [1]", got)
	}
}

func TestAppModel_MenuToAddressAndBack(t *testing.T) {
	var m tea.Model = appModel{screen: screenMenu, menuCursor: 1} // "Enter a server address"
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.(appModel).screen != screenAddress {
		t.Fatalf("enter on address row should open screenAddress, got %d", m.(appModel).screen)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.(appModel).screen != screenMenu {
		t.Fatal("esc on a sub-screen should return to the menu")
	}
}

func TestAppModel_AddressValidation(t *testing.T) {
	// Invalid -> stays, sets error, no quit.
	var m tea.Model = appModel{screen: screenAddress, addr: addrModel{value: "nope"}}
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	am := m.(appModel)
	if am.doAddress != "" || am.addr.err == "" || am.screen != screenAddress || cmd != nil {
		t.Fatalf("invalid address: doAddress=%q err=%q screen=%d cmd=%v", am.doAddress, am.addr.err, am.screen, cmd)
	}
	// Valid -> sets doAddress and quits.
	m = appModel{screen: screenAddress, addr: addrModel{value: "play.example.net:19132"}}
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	am = m.(appModel)
	if am.doAddress != "play.example.net:19132" || cmd == nil {
		t.Fatalf("valid address: doAddress=%q cmd=%v", am.doAddress, cmd)
	}
}

func TestAppModel_CatalogLoadedOpensFeatured(t *testing.T) {
	var m tea.Model = appModel{screen: screenLoading}
	m, _ = m.Update(catalogLoadedMsg{servers: []franchise.Server{{Name: "alpha"}}})
	am := m.(appModel)
	if am.screen != screenFeatured {
		t.Fatalf("catalogLoadedMsg should open screenFeatured, got %d", am.screen)
	}
	if len(am.featured.servers) != 1 {
		t.Fatalf("featured not populated: %d servers", len(am.featured.servers))
	}
}

func TestAppModel_FeaturedEscBacksToMenu(t *testing.T) {
	var m tea.Model = appModel{screen: screenFeatured, featured: newTUIModel([]franchise.Server{{Name: "alpha"}})}
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.(appModel).screen != screenMenu || cmd != nil {
		t.Fatal("esc on featured (no filter) should return to menu, not quit")
	}
}

func TestValidateAddress(t *testing.T) {
	for _, ok := range []string{"play.example.net:19132", "1.2.3.4:25565", "[::1]:19132"} {
		if _, err := validateAddress(ok); err != nil {
			t.Errorf("validateAddress(%q) errored: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "nope", "host:", ":19132", "host:70000", "host:abc"} {
		if _, err := validateAddress(bad); err == nil {
			t.Errorf("validateAddress(%q) should have failed", bad)
		}
	}
}

func TestAppModel_MenuViewLists(t *testing.T) {
	v := appModel{screen: screenMenu}.View()
	if !strings.Contains(v, "Featured servers") || !strings.Contains(v, "Enter a server address") {
		t.Error("menu view should list the sections")
	}
}
