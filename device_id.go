package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

const deviceIDFileName = ".device_id"

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
// fresh UUID on first run. The ID is fed into gophertunnel's MCToken
// mint via TokenConfig.Device.ID. PlayFab Experiments cohort assignment
// is primarily keyed on the signed-in Xbox account (XUID); device.ID is
// a secondary axis used for device-tier rollouts and telemetry segments,
// so persisting it stabilises the *subset* of treatments that vary by
// device while leaving the XUID-bound catalog slice unchanged. Practical
// effect: deterministic CLI output across runs without flipping which
// partners or events the account is eligible to see.
//
// On any persistence error (no UserConfigDir, write fails, etc.) the
// function still returns a fresh UUID so the auth flow can proceed —
// the user just loses cohort stability for this one run, which is the
// least-surprising failure mode.
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
		// File exists but contents are corrupt — overwrite with a fresh ID.
	}

	id := uuid.NewString()
	if err := writeDeviceIDAtomic(path, id); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not persist device.ID: %v\n", err)
	}
	return id
}

// writeDeviceIDAtomic writes the ID via tmp + rename so two concurrent
// first-run invocations can't leave a truncated file. Permissions are
// 0600 to mirror the token caches — device.ID is a stable user
// identifier, not as sensitive as a JWT but still worth scoping.
func writeDeviceIDAtomic(path, id string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".device_id-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write([]byte(id + "\n")); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, 0600); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// migrateMCTokenCacheOnce drops any pre-v3.2.0 .mctoken.json that was
// minted before device.ID persistence existed. Those tokens carry the
// treatments assigned to a now-discarded ephemeral device.ID, so reusing
// them would lock the user into the stale cohort until natural expiry.
// Triggered by the presence of .device_id without a sibling marker; the
// marker file is written on first successful migration.
func migrateMCTokenCacheOnce() {
	dir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	base := filepath.Join(dir, "bedrock-pack-tools")
	marker := filepath.Join(base, ".v32-migrated")
	if _, err := os.Stat(marker); err == nil {
		return
	}
	_ = os.Remove(filepath.Join(base, ".mctoken.json"))
	_ = os.WriteFile(marker, []byte("v3.2.0 cohort migration\n"), 0600)
}
