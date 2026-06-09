package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

const deviceIDFileName = ".device_id"

// configDir is the per-OS directory holding the token caches and servers.json,
// or ok=false when it can't be resolved.
func configDir() (string, bool) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", false
	}
	return filepath.Join(dir, "bedrock-pack-tools"), true
}

// resetDeviceID deletes the persisted PlayFab device.ID so the next mint rolls
// a fresh one (re-rolling the featured cohort). A missing file is not an error.
func resetDeviceID() error {
	p, err := deviceIDPath()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// Drop the cached MCToken too: the cohort treatments are baked into the
	// minted token, so a fresh device.ID has no visible effect until the next
	// mint goes through PlayFab.
	dropMCToken()
	return nil
}

// deviceIDPath returns the on-disk persistence path for the stable
// PlayFab device identifier. Falls back to an empty string + error
// rather than dumping a fingerprint into the current working dir.
func deviceIDPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	p := filepath.Join(dir, "bedrock-pack-tools")
	if err := os.MkdirAll(p, 0700); err != nil {
		return "", fmt.Errorf("create cache dir %s: %w", p, err)
	}
	return filepath.Join(p, deviceIDFileName), nil
}

// loadOrCreateDeviceID returns the persisted device.ID, generating a
// fresh UUID on first run. Fed to gophertunnel's MCToken mint via
// TokenConfig.Device.ID. PlayFab Experiments cohort assignment is
// primarily keyed on XUID; device.ID is a secondary axis for
// device-tier rollouts. Persisting it keeps that subset stable across
// runs so partner/event eligibility doesn't flip between invocations.
// On any persistence error a fresh UUID is still returned - the user
// loses cohort stability for the run but auth proceeds.
func loadOrCreateDeviceID() string {
	path, err := deviceIDPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: device.ID not persisted (%v); cohort assignment will be unstable\n", err)
		return uuid.NewString()
	}
	if data, err := os.ReadFile(path); err == nil {
		if id := strings.TrimSpace(string(data)); id != "" {
			if _, err := uuid.Parse(id); err == nil {
				return id
			}
		}
		// File exists but contents are corrupt - overwrite with a fresh ID.
	}

	id := uuid.NewString()
	if err := writeDeviceIDAtomic(path, id); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not persist device.ID: %v\n", err)
	}
	return id
}

// writeDeviceIDAtomic writes the ID via tmp + rename so two concurrent
// first-run invocations can't leave a truncated file. 0600 perms
// mirror the token caches.
func writeDeviceIDAtomic(path, id string) error {
	return atomicWriteFile(path, ".device_id-*.tmp", []byte(id+"\n"), 0600)
}

// atomicWriteFile writes data to a temp file beside dst (named via
// pattern), gives it perm, then renames it onto dst - so a concurrent
// reader never sees a half-written file. The temp is removed on any failure.
func atomicWriteFile(dst, pattern string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(dst), pattern)
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// migrateMCTokenCacheOnce drops any pre-v3.2.0 .mctoken.json minted
// before device.ID persistence existed. Those tokens carry treatments
// from a now-discarded ephemeral device.ID, locking the user into a
// stale cohort until natural expiry. Marker prevents repeat migration.
func migrateMCTokenCacheOnce() {
	dir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	migrateMCTokenCacheIn(filepath.Join(dir, "bedrock-pack-tools"))
}

// migrateMCTokenCacheIn is the testable inner form, parameterised on
// the cache base directory so unit tests can use t.TempDir().
func migrateMCTokenCacheIn(base string) {
	marker := filepath.Join(base, ".v32-migrated")
	if _, err := os.Stat(marker); err == nil {
		return
	}
	_ = os.Remove(filepath.Join(base, ".mctoken.json"))
	_ = os.WriteFile(marker, []byte("v3.2.0 cohort migration\n"), 0600)
}
