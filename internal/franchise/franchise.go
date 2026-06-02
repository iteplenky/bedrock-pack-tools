// Package franchise wraps Mojang's first-party HTTPS surface for
// Minecraft Bedrock Edition: service discovery, MCToken minting via
// PlayFab, the featured-server catalog (POST /discovery/blob/client),
// live events (GET /config/public), and address resolution
// (/join/experience, /access + /venue/{id}).
//
// The package exposes a single Client. Disk persistence of the
// MCToken cache and the PlayFab device.ID is the caller's job - pass
// them in to New, and use SeedToken/CachedToken to drive your own cache.
package franchise

import "errors"

// ErrAuthRejected fires when the gatherings service rejects our
// MCToken (401/403). Callers should Invalidate and re-mint once
// before propagating - a raw 401 would strand the user on a
// server-side-revoked but time-valid cache.
var ErrAuthRejected = errors.New("franchise: token rejected")

// ErrExperienceOffline is returned when JoinExperience resolves but
// no venue is currently active. Same response the in-game client
// gets outside an event window.
var ErrExperienceOffline = errors.New("franchise: experience has no active venue")
