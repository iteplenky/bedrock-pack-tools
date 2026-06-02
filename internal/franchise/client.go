package franchise

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sync"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/service"
	"golang.org/x/oauth2"
)

// Client pairs the resolved gatherings serviceURI with a lazily-minted
// MCToken. Construct one per CLI invocation via New.
type Client struct {
	gatheringsURI *url.URL

	mu          sync.Mutex
	cachedToken *service.Token
	tokenSource service.TokenSource
}

// New runs discovery and prepares an authenticated client. deviceID
// is fed into the PlayFab TokenConfig so the cohort stays stable
// across runs - pass a persisted UUID, or accept fresh-each-run by
// passing a generated one.
func New(ctx context.Context, xbl oauth2.TokenSource, deviceID string) (*Client, error) {
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
	// WithoutCancel: keeps the TokenSource HTTPClient + refresh alive
	// after the calling Dial ctx is cancelled (per gophertunnel docs).
	src := authEnv.TokenSource(context.WithoutCancel(ctx), xbl, service.TokenConfig{
		Device: service.DeviceConfig{ID: deviceID},
	})
	return &Client{
		gatheringsURI: gEnv.ServiceURI,
		tokenSource:   src,
	}, nil
}

// SeedToken installs a previously-minted MCToken (typically loaded
// from a disk cache) as the initial in-memory cache. A nil token is
// a no-op.
func (c *Client) SeedToken(t *service.Token) {
	if t == nil {
		return
	}
	c.mu.Lock()
	c.cachedToken = t
	c.mu.Unlock()
}

// Token returns a valid MCToken, minting via PlayFab when the cache
// is empty or expired. ctx is currently unused - the inner TokenSource
// has its own context - but kept for future-proofing.
func (c *Client) Token(_ context.Context) (*service.Token, error) {
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
	return tok, nil
}

// Invalidate drops the in-memory cache so the next Token re-mints.
// Use after a server rejects a still time-valid token.
func (c *Client) Invalidate() {
	c.mu.Lock()
	c.cachedToken = nil
	c.mu.Unlock()
}

// CachedToken returns the currently-cached MCToken, or nil. Use to
// persist the in-memory cache to disk after a successful call.
func (c *Client) CachedToken() *service.Token {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cachedToken
}

// gatheringsEnv decodes the discovery JSON's "gatherings" entry.
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
