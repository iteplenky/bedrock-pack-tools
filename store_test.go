package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
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

func TestStoreNoPathIsNoop(t *testing.T) {
	// A zero-path store must not panic or attempt any disk write.
	s := store{}
	s.addRecent("a:1")
	s.addSaved("b:2")
	if len(s.Recent) != 1 || len(s.Saved) != 1 {
		t.Fatal("in-memory mutation should still work with an empty path")
	}
}
