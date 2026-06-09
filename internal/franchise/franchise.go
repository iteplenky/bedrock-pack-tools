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

// ErrAuthRejected fires on 401 Unauthorized: the MCToken itself was not
// accepted (stale or server-side-revoked). Callers should Invalidate and
// re-mint once before propagating - a raw 401 would otherwise strand the user
// on a time-valid but revoked cache.
var ErrAuthRejected = errors.New("franchise: token rejected")

// ErrForbidden fires on 403 Forbidden: the token is valid, but this account
// isn't allowed to reach the resource (e.g. an experience that's region-locked
// or only joinable from the official client). Re-minting the token will NOT
// help, so callers must not retry on it.
var ErrForbidden = errors.New("franchise: access forbidden")

// ErrExperienceOffline is returned when JoinExperience resolves but
// no venue is currently active. Same response the in-game client
// gets outside an event window.
var ErrExperienceOffline = errors.New("franchise: experience has no active venue")
