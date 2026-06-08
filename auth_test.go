package main

import (
	"runtime"
	"testing"
)

// TestTokenPath_NoCwdFallback: when the OS config dir is unavailable,
// tokenPath must error rather than fall back to writing the Xbox refresh
// token into the current working directory.
func TestTokenPath_NoCwdFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("UserConfigDir uses %AppData% on Windows")
	}
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	if _, err := tokenPath(); err == nil {
		t.Fatal("tokenPath should error when the config dir is unavailable, not fall back to cwd")
	}
}
