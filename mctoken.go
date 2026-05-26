package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/service"
	"golang.org/x/oauth2"
)

const mctokenFileName = ".mctoken.json"

// gatheringsEnv decodes the "gatherings" entry from the discovery JSON.
// Only ServiceURI is needed for the featured-server flow.
type gatheringsEnv struct {
	ServiceURI *url.URL
}

func (g *gatheringsEnv) ServiceName() string { return "gatherings" }

func (g *gatheringsEnv) UnmarshalJSON(b []byte) error {
	var raw struct {
		ServiceURI string `json:"serviceUri"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if raw.ServiceURI == "" {
		return errors.New("empty serviceUri")
	}
	u, err := url.Parse(raw.ServiceURI)
	if err != nil {
		return fmt.Errorf("parse gatherings serviceUri: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("malformed gatherings serviceUri: %q", raw.ServiceURI)
	}
	g.ServiceURI = u
	return nil
}

// mctokenPath returns the on-disk cache path. Returns an error rather
// than falling back to the current working directory — dropping a JWT
// into a shared cwd would be a quiet credential leak.
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

// saveMCToken writes the token atomically: marshal → write *.tmp with
// 0600 → rename. Two CLI invocations running `featured` in parallel
// otherwise risk interleaved writes leaving a truncated cache.
func saveMCToken(t *service.Token) {
	path, err := mctokenPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not resolve mctoken cache path: %v\n", err)
		return
	}
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not marshal mctoken: %v\n", err)
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".mctoken-*.tmp")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not create mctoken temp: %v\n", err)
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		fmt.Fprintf(os.Stderr, "Warning: could not write mctoken temp: %v\n", err)
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		fmt.Fprintf(os.Stderr, "Warning: could not close mctoken temp: %v\n", err)
		return
	}
	if err := os.Chmod(tmpName, 0600); err != nil {
		os.Remove(tmpName)
		fmt.Fprintf(os.Stderr, "Warning: could not chmod mctoken temp: %v\n", err)
		return
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		fmt.Fprintf(os.Stderr, "Warning: could not rename mctoken cache: %v\n", err)
	}
}

// dropMCToken deletes the on-disk cache. Called after the server
// rejects our cached token (401/403) so the next mint goes through
// PlayFab fresh instead of hitting the same wall in a loop.
func dropMCToken() {
	path, err := mctokenPath()
	if err != nil {
		return
	}
	_ = os.Remove(path)
}

// gatheringsClient bundles the resolved gatherings serviceURI with a
// lazily-minted MCToken. Used by the featured subcommand to issue
// authenticated calls without re-running discovery on every request.
type gatheringsClient struct {
	gatheringsURI *url.URL

	mu          sync.Mutex
	cachedToken *service.Token
	tokenSource service.TokenSource
}

// newGatheringsClient runs the discovery handshake and prepares an
// authenticated client. The XBL TokenSource is captured for lazy
// MCToken minting on the first request. A persisted device.ID is fed
// into TokenConfig so the catalog cohort stays stable across runs.
func newGatheringsClient(ctx context.Context, xbl oauth2.TokenSource) (*gatheringsClient, error) {
	disc, err := service.Discover(ctx, service.ApplicationTypeMinecraftPE, protocol.CurrentVersion)
	if err != nil {
		return nil, fmt.Errorf("discover services: %w", err)
	}
	authEnv := &service.AuthorizationEnvironment{}
	if err := disc.Environment(authEnv); err != nil {
		return nil, fmt.Errorf("decode auth environment: %w", err)
	}
	var gEnv gatheringsEnv
	if err := disc.Environment(&gEnv); err != nil {
		return nil, fmt.Errorf("decode gatherings environment: %w", err)
	}
	migrateMCTokenCacheOnce()
	// Pass context.WithoutCancel so the inner TokenSource keeps the oauth2
	// HTTPClient and refresh path alive after the calling Dial context
	// gets cancelled — recommended by gophertunnel/service docs.
	src := authEnv.TokenSource(context.WithoutCancel(ctx), xbl, service.TokenConfig{
		Device: service.DeviceConfig{ID: loadOrCreateDeviceID()},
	})
	return &gatheringsClient{
		gatheringsURI: gEnv.ServiceURI,
		tokenSource:   src,
		cachedToken:   loadMCToken(),
	}, nil
}

// Token returns a valid MCToken, minting a fresh one via PlayFab if the
// disk-cached one is missing or expired. Successful mints persist to
// disk so a follow-up CLI invocation skips the PlayFab roundtrip.
//
// The context arg is currently unused: gophertunnel's TokenSource was
// constructed with its own (uncancellable) context, so the caller can't
// cancel a slow PlayFab mint mid-flight. Kept on the signature so any
// future upstream API that takes ctx is a non-breaking swap-in.
func (c *gatheringsClient) Token(_ context.Context) (*service.Token, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cachedToken != nil && c.cachedToken.Valid() {
		return c.cachedToken, nil
	}
	tok, err := c.tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("mint mctoken: %w", err)
	}
	c.cachedToken = tok
	saveMCToken(tok)
	return tok, nil
}

// invalidate clears the in-memory and on-disk cached token so the next
// Token() call re-mints from scratch. Used after the server rejects
// a cached token (typically because it was server-side revoked while
// still within its time-based validity window).
func (c *gatheringsClient) invalidate() {
	c.mu.Lock()
	c.cachedToken = nil
	c.mu.Unlock()
	dropMCToken()
}
