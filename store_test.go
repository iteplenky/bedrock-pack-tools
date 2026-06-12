package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestDedupPrepend(t *testing.T) {
	// Most-recent first, existing copy removed, capped at the limit.
	got := dedupPrepend([]string{"a", "b", "c"}, "b", 3)
	if want := []string{"b", "a", "c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("dedupPrepend = %v, want %v", got, want)
	}
	got = dedupPrepend([]string{"a", "b", "c"}, "d", 3)
	if want := []string{"d", "a", "b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("dedupPrepend cap = %v, want %v", got, want)
	}
	got = dedupPrepend([]string{"a", "b"}, "c", 0) // 0 = unbounded
	if want := []string{"c", "a", "b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("dedupPrepend unbounded = %v, want %v", got, want)
	}
}

func TestStoreRoundTrip(t *testing.T) {
	s := store{path: filepath.Join(t.TempDir(), "servers.json")}
	s.addRecent("a:1")
	s.addRecent("b:2")
	s.addSaved("fav:19132")
	s.removeRecent("a:1")

	// Re-load from the same path and confirm what persisted.
	got := store{path: s.path}
	data, err := os.ReadFile(got.path)
	if err != nil {
		t.Fatalf("store file not written: %v", err)
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal store: %v", err)
	}
	if want := []string{"b:2"}; !reflect.DeepEqual(got.Recent, want) {
		t.Errorf("Recent = %v, want %v", got.Recent, want)
	}
	if want := []string{"fav:19132"}; !reflect.DeepEqual(got.Saved, want) {
		t.Errorf("Saved = %v, want %v", got.Saved, want)
	}
}

// TestStoreLanguageRoundTrip covers the Settings language toggle: the
// chosen language is persisted to servers.json and survives a reload, so
// the choice sticks across runs (and feeds lang.Init's precedence).
func TestStoreLanguageRoundTrip(t *testing.T) {
	s := store{path: filepath.Join(t.TempDir(), "servers.json")}
	s.setLanguage("ru")

	got := store{path: s.path}
	data, err := os.ReadFile(got.path)
	if err != nil {
		t.Fatalf("store file not written: %v", err)
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal store: %v", err)
	}
	if got.Language != "ru" {
		t.Errorf("persisted Language = %q, want %q", got.Language, "ru")
	}
}

func TestStoreDownloadsAndStatus(t *testing.T) {
	s := store{path: filepath.Join(t.TempDir(), "servers.json")}

	// dedup by dir+address, refreshed copy moves to the front
	s.addDownload(download{Dir: "d1", Address: "a:1", Label: "old"})
	s.addDownload(download{Dir: "d2", Address: "b:2"})
	s.addDownload(download{Dir: "d1", Address: "a:1", Label: "new"})
	if len(s.Downloads) != 2 || s.Downloads[0].Dir != "d1" || s.Downloads[0].Label != "new" {
		t.Fatalf("dedup/refresh failed: %+v", s.Downloads)
	}

	s.removeDownload(download{Dir: "d1", Address: "a:1"})
	if len(s.Downloads) != 1 || s.Downloads[0].Dir != "d2" {
		t.Fatalf("removeDownload failed: %+v", s.Downloads)
	}

	// status survives a disk round-trip, but only while its address is in
	// Recent/Saved (pruneStatus runs on persist).
	s.addRecent("a:1")
	s.recordStatus("a:1", true)
	s.recordStatus("orphan:9", false) // not in recent/saved -> pruned

	got := store{path: s.path}
	data, err := os.ReadFile(got.path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if !got.Status["a:1"].OK {
		t.Errorf("status for a:1 not persisted: %+v", got.Status)
	}
	if _, ok := got.Status["orphan:9"]; ok {
		t.Errorf("orphan status should have been pruned: %+v", got.Status)
	}
	if len(got.Downloads) != 1 || got.Downloads[0].Dir != "d2" {
		t.Errorf("downloads round-trip mismatch: %+v", got.Downloads)
	}
}

func TestStoreDownloadsCap(t *testing.T) {
	s := store{path: filepath.Join(t.TempDir(), "servers.json")}
	for i := range maxDownloads + 5 {
		s.addDownload(download{Dir: fmt.Sprintf("d%d", i), Address: "a:1"})
	}
	if len(s.Downloads) != maxDownloads {
		t.Fatalf("Downloads len = %d, want %d", len(s.Downloads), maxDownloads)
	}
	if s.Downloads[0].Dir != fmt.Sprintf("d%d", maxDownloads+4) {
		t.Errorf("newest should be first, got %q", s.Downloads[0].Dir)
	}
}

func TestAgeLabel(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "just now"},
		{5 * time.Minute, "5m ago"},
		{3 * time.Hour, "3h ago"},
		{50 * time.Hour, "2d ago"},
	}
	for _, c := range cases {
		stamp := time.Now().Add(-c.d).UTC().Format(time.RFC3339)
		if got := ageLabel(stamp); got != c.want {
			t.Errorf("ageLabel(-%s) = %q, want %q", c.d, got, c.want)
		}
	}
	if got := ageLabel("not-a-stamp"); got != "" {
		t.Errorf("ageLabel(bad) = %q, want empty", got)
	}
}

func TestStoreNoPathIsNoop(t *testing.T) {
	// A zero-path store must not panic or attempt any disk write.
	s := store{}
	s.addRecent("a:1")
	s.addSaved("b:2")
	if len(s.Recent) != 1 || len(s.Saved) != 1 {
		t.Fatal("in-memory mutation should still work with an empty path")
	}
}
