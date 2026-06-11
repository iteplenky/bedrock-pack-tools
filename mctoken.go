package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/iteplenky/bedrock-pack-tools/v3/internal/franchise"
	"github.com/iteplenky/bedrock-pack-tools/v3/internal/lang"
	"github.com/iteplenky/gophertunnel/minecraft/service"
	"golang.org/x/oauth2"
)

const mctokenFileName = ".mctoken.json"

// mctokenPath returns the on-disk cache path. Errors instead of
// falling back to cwd - a JWT dropped into a shared cwd would be a
// silent credential leak.
func mctokenPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	p := filepath.Join(dir, "bedrock-pack-tools")
	if err := os.MkdirAll(p, 0700); err != nil {
		return "", fmt.Errorf("create cache dir %s: %w", p, err)
	}
	return filepath.Join(p, mctokenFileName), nil
}

func loadMCToken() *service.Token {
	path, err := mctokenPath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var t service.Token
	if err := json.Unmarshal(data, &t); err != nil {
		return nil
	}
	if t.AuthorizationHeader == "" || !t.Valid() {
		return nil
	}
	return &t
}

// saveMCToken writes the token atomically (write *.tmp 0600 → rename)
// so concurrent CLI invocations can't interleave into a truncated cache.
func saveMCToken(t *service.Token) {
	path, err := mctokenPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, lang.T("auth.warn.mctoken.resolve"), err)
		return
	}
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, lang.T("auth.warn.mctoken.marshal"), err)
		return
	}
	if err := atomicWriteFile(path, ".mctoken-*.tmp", data, 0600); err != nil {
		fmt.Fprintf(os.Stderr, lang.T("auth.warn.mctoken.save"), err)
	}
}

// dropMCToken deletes the on-disk cache after a 401/403 so the next
// mint goes through PlayFab instead of looping on the same bad token.
func dropMCToken() {
	path, err := mctokenPath()
	if err != nil {
		return
	}
	_ = os.Remove(path)
}

// newFranchiseClient runs franchise discovery and wires the disk-backed
// MCToken + device.ID caches into the resulting client.
func newFranchiseClient(ctx context.Context, xbl oauth2.TokenSource) (*franchise.Client, error) {
	migrateMCTokenCacheOnce()
	c, err := franchise.New(ctx, xbl, loadOrCreateDeviceID())
	if err != nil {
		return nil, err
	}
	c.SeedToken(loadMCToken())
	return c, nil
}

// persistFranchiseToken writes the franchise client's in-memory
// MCToken to disk so a follow-up CLI invocation skips the PlayFab
// roundtrip. Safe to call after any successful franchise.Client method.
func persistFranchiseToken(c *franchise.Client) {
	if tok := c.CachedToken(); tok != nil {
		saveMCToken(tok)
	}
}

// invalidateFranchise clears both the franchise client's in-memory
// cache and our on-disk copy.
func invalidateFranchise(c *franchise.Client) {
	c.Invalidate()
	dropMCToken()
}
