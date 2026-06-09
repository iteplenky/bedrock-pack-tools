package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
)

// TestWriteDeviceIDAtomic_HappyPath confirms the file lands at the
// requested path, has the correct 0600 mode, and contains exactly the
// UUID plus the trailing newline that loadOrCreateDeviceID trims off.
func TestWriteDeviceIDAtomic_HappyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".device_id")
	id := uuid.NewString()

	if err := writeDeviceIDAtomic(path, id); err != nil {
		t.Fatalf("writeDeviceIDAtomic: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readback: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != id {
		t.Errorf("file contains %q, want %q", got, id)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm() & 0o077; perm != 0 {
		t.Errorf("file mode = %o, want only owner-readable (no group/other bits)", info.Mode().Perm())
	}
}

// TestWriteDeviceIDAtomic_NoLeftoverTmps verifies that even with many
// rapid back-to-back writes, the directory ends with one file (the
// final .device_id) and zero leftover .tmp files. This is the
// rename-vs-truncate guarantee we built the atomic path for.
func TestWriteDeviceIDAtomic_NoLeftoverTmps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".device_id")
	for range 50 {
		if err := writeDeviceIDAtomic(path, uuid.NewString()); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("leftover tmp file: %s", e.Name())
		}
	}
}

// TestWriteDeviceIDAtomic_Concurrent runs N writers against the same
// path. The atomic-rename guarantee is that whatever lands as the
// final .device_id contains exactly one valid UUID (not a partial
// write, not interleaved bytes from two writers).
func TestWriteDeviceIDAtomic_Concurrent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".device_id")

	const writers = 16
	var wg sync.WaitGroup
	wg.Add(writers)
	for range writers {
		go func() {
			defer wg.Done()
			_ = writeDeviceIDAtomic(path, uuid.NewString())
		}()
	}
	wg.Wait()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readback: %v", err)
	}
	got := strings.TrimSpace(string(data))
	if _, err := uuid.Parse(got); err != nil {
		t.Errorf("final file does not parse as UUID: %q (%v)", got, err)
	}
}

// TestMigrateMCTokenCacheIn_FirstRunDropsToken verifies the v3.2.0
// cohort migration: the pre-existing .mctoken.json gets removed and
// the .v32-migrated marker is written.
func TestMigrateMCTokenCacheIn_FirstRunDropsToken(t *testing.T) {
	dir := t.TempDir()
	tok := filepath.Join(dir, ".mctoken.json")
	if err := os.WriteFile(tok, []byte("pre-migration"), 0600); err != nil {
		t.Fatal(err)
	}

	migrateMCTokenCacheIn(dir)

	if _, err := os.Stat(tok); !os.IsNotExist(err) {
		t.Errorf("expected pre-migration token removed, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".v32-migrated")); err != nil {
		t.Errorf("expected marker created, got err=%v", err)
	}
}

// TestMigrateMCTokenCacheIn_IsIdempotent verifies the second run does
// NOT touch a freshly-minted .mctoken.json. This is the critical
// regression to protect: if the marker logic broke, every CLI
// invocation would stomp the token and force re-auth.
func TestMigrateMCTokenCacheIn_IsIdempotent(t *testing.T) {
	dir := t.TempDir()
	// First run with no token: just writes the marker.
	migrateMCTokenCacheIn(dir)

	// Now drop a fresh token that the second run must NOT touch.
	tok := filepath.Join(dir, ".mctoken.json")
	body := []byte("post-migration fresh token")
	if err := os.WriteFile(tok, body, 0600); err != nil {
		t.Fatal(err)
	}

	migrateMCTokenCacheIn(dir)

	got, err := os.ReadFile(tok)
	if err != nil {
		t.Fatalf("fresh token disappeared on second migration: %v", err)
	}
	if string(got) != string(body) {
		t.Errorf("fresh token was modified: got %q want %q", got, body)
	}
}

// TestMigrateMCTokenCacheIn_NoTokenToDrop checks the no-op happy path:
// first run with no pre-existing token just writes the marker, no
// errors propagate (the inner function returns no error anyway, but
// we still want to confirm the side effects).
func TestMigrateMCTokenCacheIn_NoTokenToDrop(t *testing.T) {
	dir := t.TempDir()
	migrateMCTokenCacheIn(dir)
	if _, err := os.Stat(filepath.Join(dir, ".v32-migrated")); err != nil {
		t.Errorf("marker not written on first run with no token: %v", err)
	}
}

// TestLoadOrCreateDeviceID_RoundTrip verifies that on a fresh
// UserConfigDir the function generates and persists a UUID, and a
// second call returns the same value. We can't fully mock
// os.UserConfigDir without touching the global env, so we just check
// the contract that two back-to-back calls agree.
func TestLoadOrCreateDeviceID_RoundTrip(t *testing.T) {
	// Skip on CI runners without a writable UserConfigDir.
	if _, err := os.UserConfigDir(); err != nil {
		t.Skipf("no UserConfigDir: %v", err)
	}
	first := loadOrCreateDeviceID()
	second := loadOrCreateDeviceID()
	if first != second {
		t.Errorf("device.ID changed between calls: %q vs %q", first, second)
	}
	if _, err := uuid.Parse(first); err != nil {
		t.Errorf("not a valid UUID: %q (%v)", first, err)
	}
}

// TestResetDeviceID_DropsMCToken: the featured cohort treatments are baked into
// the minted MCToken, so resetting the device.ID must also drop the cached
// token or the featured list wouldn't change until the token expired (~4h).
func TestResetDeviceID_DropsMCToken(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("config dir is not HOME-derived on windows")
	}
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "cfg"))

	devPath, err := deviceIDPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(devPath, []byte("old-id\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mcPath, err := mctokenPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mcPath, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := resetDeviceID(); err != nil {
		t.Fatalf("resetDeviceID: %v", err)
	}
	if _, err := os.Stat(devPath); !os.IsNotExist(err) {
		t.Errorf("device.ID should be removed (stat err=%v)", err)
	}
	if _, err := os.Stat(mcPath); !os.IsNotExist(err) {
		t.Errorf("mctoken cache should be dropped so the new cohort takes effect (stat err=%v)", err)
	}
}
