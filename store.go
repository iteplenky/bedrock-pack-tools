package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	storeFileName = "servers.json"
	maxRecent     = 12 // recent addresses are a convenience, not an archive
	maxDownloads  = 20
)

// store persists the user's recent/saved addresses, the last run outcome per
// address, and where past downloads landed - next to the token caches. A zero
// path (e.g. in tests, or when the config dir can't be resolved) makes every
// write a no-op, so the menu still works in memory.
type store struct {
	path      string
	Recent    []string                `json:"recent"`
	Saved     []string                `json:"saved"`
	Status    map[string]recentStatus `json:"status,omitempty"`
	Downloads []download              `json:"downloads,omitempty"`
}

// recentStatus is the outcome of the last run against an address.
type recentStatus struct {
	OK       bool   `json:"ok"`
	LastUsed string `json:"last_used,omitempty"` // RFC3339
}

// download remembers where a server's packs + keys were saved, so the decrypt
// section can find them again (and offer to re-fetch what's missing).
type download struct {
	Label    string `json:"label"`
	Address  string `json:"address"`
	Dir      string `json:"dir"`
	KeysFile string `json:"keys_file,omitempty"`
	When     string `json:"when,omitempty"`
}

// loadStore reads servers.json from the user config dir. A missing or
// corrupt file yields an empty store - this is best-effort convenience data.
func loadStore() store {
	s := store{}
	dir, err := os.UserConfigDir()
	if err != nil {
		return s
	}
	s.path = filepath.Join(dir, "bedrock-pack-tools", storeFileName)
	if data, err := os.ReadFile(s.path); err == nil {
		_ = json.Unmarshal(data, &s) // path is unexported, so it survives
	}
	return s
}

func (s *store) persist() {
	s.pruneStatus()
	if s.path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	_ = atomicWriteFile(s.path, "servers-*.tmp", data, 0600)
}

func (s *store) addRecent(addr string) {
	s.Recent = dedupPrepend(s.Recent, addr, maxRecent)
	s.persist()
}

func (s *store) addSaved(addr string) {
	s.Saved = dedupPrepend(s.Saved, addr, 0)
	s.persist()
}

func (s *store) removeRecent(addr string) {
	s.Recent = removeString(s.Recent, addr)
	s.persist()
}

func (s *store) removeSaved(addr string) {
	s.Saved = removeString(s.Saved, addr)
	s.persist()
}

// recordStatus stores the outcome of the last run against addr. Pruning in
// persist drops it again if addr isn't a recent/saved address.
func (s *store) recordStatus(addr string, ok bool) {
	if s.Status == nil {
		s.Status = map[string]recentStatus{}
	}
	s.Status[addr] = recentStatus{OK: ok, LastUsed: nowStamp()}
	s.persist()
}

// pruneStatus keeps the status map bounded to addresses that still exist in
// the recent or saved lists.
func (s *store) pruneStatus() {
	if s.Status == nil {
		return
	}
	keep := make(map[string]bool, len(s.Recent)+len(s.Saved))
	for _, a := range s.Recent {
		keep[a] = true
	}
	for _, a := range s.Saved {
		keep[a] = true
	}
	for a := range s.Status {
		if !keep[a] {
			delete(s.Status, a)
		}
	}
	if len(s.Status) == 0 {
		s.Status = nil
	}
}

// addDownload records (or refreshes) where a server's packs + keys landed,
// most-recent first, deduped by dir+address.
func (s *store) addDownload(d download) {
	out := make([]download, 0, len(s.Downloads)+1)
	out = append(out, d)
	for _, x := range s.Downloads {
		if x.Dir != d.Dir || x.Address != d.Address {
			out = append(out, x)
		}
	}
	if len(out) > maxDownloads {
		out = out[:maxDownloads]
	}
	s.Downloads = out
	s.persist()
}

func (s *store) removeDownload(d download) {
	out := make([]download, 0, len(s.Downloads))
	for _, x := range s.Downloads {
		if x.Dir != d.Dir || x.Address != d.Address {
			out = append(out, x)
		}
	}
	s.Downloads = out
	s.persist()
}

func nowStamp() string { return time.Now().UTC().Format(time.RFC3339) }

// ageLabel renders a compact "5m ago" for an RFC3339 stamp, or "" if unset.
func ageLabel(stamp string) string {
	t, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours())/24)
	}
}

// dedupPrepend moves addr to the front (most recent first), dropping any
// earlier copy. A positive limit caps the list length.
func dedupPrepend(list []string, addr string, limit int) []string {
	out := make([]string, 0, len(list)+1)
	out = append(out, addr)
	for _, a := range list {
		if a != addr {
			out = append(out, a)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func removeString(list []string, addr string) []string {
	out := make([]string, 0, len(list))
	for _, a := range list {
		if a != addr {
			out = append(out, a)
		}
	}
	return out
}
