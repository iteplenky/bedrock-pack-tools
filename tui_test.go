package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestAppModel_RightArrowDrillsInLeftArrowBacks(t *testing.T) {
	// Right arrow opens the highlighted section; left arrow returns.
	var m tea.Model = appModel{screen: screenMenu, menuCursor: 2} // "Saved servers"
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.(appModel).screen != screenSaved {
		t.Fatalf("right arrow should open screenSaved, got %d", m.(appModel).screen)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.(appModel).screen != screenMenu {
		t.Fatalf("left arrow should return to the menu, got %d", m.(appModel).screen)
	}
}

func TestAppModel_RecentSaveAndDelete(t *testing.T) {
	// "s" on a recent row saves it; "d" forgets it. Store path is empty, so
	// nothing touches disk.
	m := appModel{screen: screenRecent, store: store{Recent: []string{"a:1", "b:2"}}}
	m.list = newAddrList(m.store.Recent)
	m.list.cursor = 1 // "b:2"

	var tm tea.Model = m
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	am := tm.(appModel)
	if len(am.store.Saved) != 1 || am.store.Saved[0] != "b:2" {
		t.Fatalf("save: store.Saved = %v, want [b:2]", am.store.Saved)
	}
	tm, _ = am.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	am = tm.(appModel)
	if len(am.store.Recent) != 1 || am.store.Recent[0] != "a:1" {
		t.Fatalf("delete: store.Recent = %v, want [a:1]", am.store.Recent)
	}
}

func TestAppModel_SavedEnterQueuesJobs(t *testing.T) {
	m := appModel{screen: screenSaved, store: store{Saved: []string{"a:1", "b:2"}}}
	m.list = newAddrList(m.store.Saved)
	m.list.picked = map[int]bool{0: true, 1: true}
	var tm tea.Model = m
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	am := tm.(appModel)
	if am.screen != screenAction || len(am.jobs) != 2 || am.jobs[0].address != "a:1" {
		t.Fatalf("saved enter: screen=%d jobs=%+v", am.screen, am.jobs)
	}
	// esc from the action picker returns to the saved list.
	tm, _ = am.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if tm.(appModel).screen != screenSaved {
		t.Fatalf("esc on action should return to screenSaved, got %d", tm.(appModel).screen)
	}
}

func TestAppModel_AddressToActionPicker(t *testing.T) {
	// Invalid address -> stays on the field with an error, no job queued.
	var m tea.Model = appModel{screen: screenAddress, addr: addrModel{value: "nope"}}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if am := m.(appModel); am.screen != screenAddress || am.addr.err == "" || len(am.jobs) != 0 {
		t.Fatalf("invalid address: screen=%d err=%q jobs=%d", am.screen, am.addr.err, len(am.jobs))
	}
	// Valid address -> one job queued, action picker opens.
	m = appModel{screen: screenAddress, addr: addrModel{value: "play.example.net:19132"}}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	am := m.(appModel)
	if am.screen != screenAction || len(am.jobs) != 1 || am.jobs[0].address != "play.example.net:19132" {
		t.Fatalf("valid address: screen=%d jobs=%+v", am.screen, am.jobs)
	}
	// esc from the action picker returns to the address field.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.(appModel).screen != screenAddress {
		t.Fatalf("esc on action picker should return to %d, got %d", screenAddress, m.(appModel).screen)
	}
}

func TestAppModel_ActionPickerNavigates(t *testing.T) {
	// down twice lands on the third choice ("Keys only"); we stop short of
	// enter, which would spawn the child process.
	var m tea.Model = appModel{screen: screenAction, jobs: []job{{label: "x", address: "x:1"}}}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // clamps at the last row
	am := m.(appModel)
	if am.actionCursor != len(actionChoices)-1 || actionChoices[am.actionCursor].value != actionKeys {
		t.Fatalf("cursor = %d, want last row mapping to actionKeys", am.actionCursor)
	}
}

func TestAppModel_CatalogLoadedOpensFeatured(t *testing.T) {
	var m tea.Model = appModel{screen: screenLoading}
	m, _ = m.Update(catalogLoadedMsg{servers: []franchise.Server{{Name: "alpha"}}})
	am := m.(appModel)
	if am.screen != screenFeatured {
		t.Fatalf("catalogLoadedMsg should open screenFeatured, got %d", am.screen)
	}
	if len(am.featured.servers) != 1 || am.fServers == nil {
		t.Fatalf("featured/catalog not populated: %d servers", len(am.featured.servers))
	}
}

func TestAppModel_FeaturedEnterQueuesJobs(t *testing.T) {
	servers := []franchise.Server{{Name: "alpha"}, {Name: "bravo"}}
	m := appModel{screen: screenFeatured, fServers: servers, featured: newTUIModel(servers)}
	m.featured.picked = map[int]bool{0: true, 1: true}
	var tm tea.Model = m
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	am := tm.(appModel)
	if am.screen != screenAction || len(am.jobs) != 2 {
		t.Fatalf("featured enter: screen=%d jobs=%d, want action + 2 jobs", am.screen, len(am.jobs))
	}
	if am.jobs[0].server == nil || am.jobs[0].label != "alpha" {
		t.Fatalf("job not wired to a server: %+v", am.jobs[0])
	}
}

func TestAppModel_FeaturedEscBacksToMenu(t *testing.T) {
	var m tea.Model = appModel{screen: screenFeatured, featured: newTUIModel([]franchise.Server{{Name: "alpha"}})}
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.(appModel).screen != screenMenu || cmd != nil {
		t.Fatal("esc on featured (no filter) should return to menu, not quit")
	}
}

func TestAppModel_DoneKeyReturnsToMenu(t *testing.T) {
	m := appModel{
		screen:  screenDone,
		jobs:    []job{{label: "x"}},
		results: []jobResult{{label: "x"}},
	}
	var tm tea.Model = m
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	am := tm.(appModel)
	if am.screen != screenMenu || am.jobs != nil || am.results != nil {
		t.Fatalf("done key should reset to menu, got screen=%d jobs=%d results=%d", am.screen, len(am.jobs), len(am.results))
	}
}

func TestAppModel_JobFinishedAdvancesAndSummarizes(t *testing.T) {
	// Two ready jobs; finishing both lands on the summary with two results.
	m := appModel{
		screen: screenRunning,
		jobs:   []job{{label: "a", address: "a:1"}, {label: "b", address: "b:1"}},
		jobIdx: 0,
	}
	var tm tea.Model = m
	tm, cmd := tm.Update(jobFinishedMsg{err: nil})
	am := tm.(appModel)
	if am.jobIdx != 1 || len(am.results) != 1 || cmd == nil {
		t.Fatalf("first finish: jobIdx=%d results=%d cmd=%v (want a start cmd for job 2)", am.jobIdx, len(am.results), cmd)
	}
	tm, _ = am.Update(jobFinishedMsg{err: nil})
	am = tm.(appModel)
	if am.screen != screenDone || len(am.results) != 2 {
		t.Fatalf("second finish: screen=%d results=%d, want done + 2", am.screen, len(am.results))
	}
}

func TestInspectDownload(t *testing.T) {
	dir := t.TempDir()
	// One encrypted pack folder + a keys file.
	packDir := filepath.Join(dir, "Pack_v1")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packDir, contentsJSON), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	keys := filepath.Join(dir, "srv_keys.json")
	if err := os.WriteFile(keys, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	st := inspectDownload(download{Dir: dir, KeysFile: keys})
	if st.packs != 1 || !st.hasKeys || !st.decryptable() {
		t.Fatalf("inspect = %+v, want 1 pack + keys + decryptable", st)
	}
	// No keys -> not decryptable.
	st = inspectDownload(download{Dir: dir, KeysFile: filepath.Join(dir, "missing.json")})
	if st.packs != 1 || st.hasKeys || st.decryptable() {
		t.Fatalf("inspect (no keys) = %+v, want 1 pack, no keys, not decryptable", st)
	}
}

func TestAppModel_DecryptForwardBuildsDecryptJob(t *testing.T) {
	d := download{Label: "srv", Address: "srv:19132", Dir: "/tmp/x", KeysFile: "/tmp/x/srv_keys.json"}
	m := appModel{
		screen:   screenDecrypt,
		dls:      []download{d},
		dlStates: []decryptState{{packs: 2, hasKeys: true}},
		list:     newAddrList([]string{d.Label}),
	}
	var tm tea.Model = m
	tm, cmd := tm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	am := tm.(appModel)
	if am.screen != screenRunning || len(am.jobs) != 1 || cmd == nil {
		t.Fatalf("decrypt enter: screen=%d jobs=%d cmd=%v", am.screen, len(am.jobs), cmd)
	}
	// The decrypt run is grouped by server and the destination is recorded.
	wantOut := decryptOutBase(d.Dir, d.Address)
	want := []string{"decrypt", "--all", d.KeysFile, d.Dir, wantOut}
	if strings.Join(am.jobs[0].argv, " ") != strings.Join(want, " ") {
		t.Fatalf("decrypt job argv = %v, want %v", am.jobs[0].argv, want)
	}
	if am.jobs[0].outDir != wantOut {
		t.Fatalf("job outDir = %q, want %q", am.jobs[0].outDir, wantOut)
	}
}

func TestDecryptOutBase(t *testing.T) {
	got := decryptOutBase("/packs", "play.example.net:19132")
	want := filepath.Join("/packs", "decrypted", "play_example_net_19132")
	if got != want {
		t.Fatalf("decryptOutBase = %q, want %q", got, want)
	}
}

func TestAppModel_DecryptNotDecryptableShowsNote(t *testing.T) {
	m := appModel{
		screen:   screenDecrypt,
		dls:      []download{{Label: "srv"}},
		dlStates: []decryptState{{packs: 0, hasKeys: false}},
		list:     newAddrList([]string{"srv"}),
	}
	var tm tea.Model = m
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	am := tm.(appModel)
	if am.screen != screenDecrypt || am.note == "" || len(am.jobs) != 0 {
		t.Fatalf("non-decryptable enter: screen=%d note=%q jobs=%d", am.screen, am.note, len(am.jobs))
	}
}

func TestAppModel_CancelFinishDoesNotRecordFailure(t *testing.T) {
	// A user-cancelled run's killed child reports exit -1; that must not be
	// recorded as a failed result or persist a "failed" status.
	m := appModel{
		screen:   screenRunning,
		canceled: true,
		jobs:     []job{{label: "a", address: "a:1"}},
		jobIdx:   0,
		store:    store{},
	}
	var tm tea.Model = m
	tm, _ = tm.Update(jobFinishedMsg{err: errors.New("exited with code -1")})
	am := tm.(appModel)
	if am.screen != screenDone {
		t.Fatalf("cancel-finish should go to the summary, got screen %d", am.screen)
	}
	if len(am.results) != 0 {
		t.Errorf("cancelled job should not be recorded: %+v", am.results)
	}
	if _, ok := am.store.Status["a:1"]; ok {
		t.Error("cancelled job should not persist a status")
	}
}

func TestRecentStatusLabel(t *testing.T) {
	m := appModel{store: store{Status: map[string]recentStatus{
		"a:1": {OK: true, LastUsed: time.Now().Add(-5 * time.Minute).UTC().Format(time.RFC3339)},
		"b:2": {OK: false, LastUsed: time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)},
	}}}
	if got := m.recentStatusLabel("a:1"); !strings.Contains(got, "ok") || !strings.Contains(got, "·") {
		t.Errorf("ok label = %q, want it to contain ok + the separator", got)
	}
	if got := m.recentStatusLabel("b:2"); !strings.Contains(got, "failed") {
		t.Errorf("failed label = %q", got)
	}
	if got := m.recentStatusLabel("missing"); got != "" {
		t.Errorf("unknown address label = %q, want empty", got)
	}
}

func TestFeaturedDisplay_ResolvedExperience(t *testing.T) {
	// A resolved experience row shows its IP + online status, not the placeholder.
	r := franchise.Server{Kind: franchise.KindPartnerExperience, Name: "Exp", Host: "1.2.3.4", Port: 19132, Online: true, Players: 340}
	if got := addressColumn(r); got != "1.2.3.4:19132" {
		t.Errorf("resolved addressColumn = %q, want the IP", got)
	}
	if tag, _ := tagFor(r); tag != "[ON]" {
		t.Errorf("resolved tag = %q, want [ON]", tag)
	}
	// Unresolved experience keeps its placeholder + [EXP] tag.
	u := franchise.Server{Kind: franchise.KindPartnerExperience, Name: "Exp2"}
	if got := addressColumn(u); got != "(experience-join)" {
		t.Errorf("unresolved addressColumn = %q", got)
	}
	if tag, _ := tagFor(u); tag != "[EXP]" {
		t.Errorf("unresolved tag = %q, want [EXP]", tag)
	}
}

func TestFeaturedHasUnresolved(t *testing.T) {
	if !featuredHasUnresolved([]franchise.Server{
		{Kind: franchise.KindPartnerDirect, Host: "h", Port: 1},
		{Kind: franchise.KindPartnerExperience}, // unresolved
	}) {
		t.Error("should detect an unresolved experience")
	}
	if featuredHasUnresolved([]franchise.Server{{Kind: franchise.KindPartnerExperience, Host: "h", Port: 2}}) {
		t.Error("a resolved experience is not unresolved")
	}
}

func TestResolveAddress_NoNetworkBranches(t *testing.T) {
	ctx := context.Background()
	// Direct partner: no client needed, returns host:port inline.
	addr, err := resolveAddress(ctx, nil, franchise.Server{Kind: franchise.KindPartnerDirect, Host: "h", Port: 1})
	if err != nil || addr != "h:1" {
		t.Fatalf("direct: addr=%q err=%v", addr, err)
	}
	// Unknown kind errors instead of panicking.
	if _, err := resolveAddress(ctx, nil, franchise.Server{Kind: franchise.Kind(99), Name: "x"}); err == nil {
		t.Error("unknown kind should error")
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
