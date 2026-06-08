package franchise

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// JoinExperience resolves an experienceId to ip:port via
// POST /join/experience. Returns ErrExperienceOffline when no venue
// is currently active (the in-game client gets the same 404).
func (c *Client) JoinExperience(ctx context.Context, experienceID string) (host string, port int, err error) {
	tok, err := c.Token(ctx)
	if err != nil {
		return "", 0, err
	}
	return joinExperience(ctx, c.gatheringsURI, tok.AuthorizationHeader, experienceID)
}

func joinExperience(ctx context.Context, gatheringsURI *url.URL, authHeader, experienceID string) (host string, port int, err error) {
	u := gatheringsURI.JoinPath(pathJoinExpURL).String()
	payload, _ := json.Marshal(map[string]string{"experienceId": experienceID})
	body, err := request(ctx, http.MethodPost, u, payload, authHeader)
	if err != nil {
		if httpStatusOf(err) == http.StatusNotFound {
			return "", 0, ErrExperienceOffline
		}
		return "", 0, err
	}
	// POST but uses {result} envelope - same as /venue/{id} and
	// /config/public. Only /discovery/blob/client uses {data}.
	type joinResult struct {
		IPV4Address string `json:"ipV4Address"`
		Port        int    `json:"port"`
	}
	env, err := decodeAPI[apiResult[joinResult]](body, "join/experience")
	if err != nil {
		return "", 0, err
	}
	if env.Result.IPV4Address == "" || env.Result.Port == 0 {
		// Shapeless 200 is treated as offline. Body is wrapped in so a
		// future Mojang transport change (NetherNet signaling, etc.)
		// surfaces visibly rather than masquerading as "no active venue".
		return "", 0, fmt.Errorf("%w (unexpected response shape: %s)", ErrExperienceOffline, previewBody(body))
	}
	return env.Result.IPV4Address, env.Result.Port, nil
}

// Venue resolves a gathering's current venue via /access + /venue/{id}.
// Skipping /access has been observed to make /venue 404 on some Mojang
// edges (likely load-balancer affinity), so keep the call.
func (c *Client) Venue(ctx context.Context, gatheringID string) (host string, port int, err error) {
	tok, err := c.Token(ctx)
	if err != nil {
		return "", 0, err
	}
	return venue(ctx, c.gatheringsURI, tok.AuthorizationHeader, gatheringID)
}

func venue(ctx context.Context, gatheringsURI *url.URL, authHeader, gatheringID string) (host string, port int, err error) {
	accessURL := gatheringsURI.JoinPath(pathAccess)
	accessURL.RawQuery = clientQuery().Encode()
	if _, err := request(ctx, http.MethodGet, accessURL.String(), nil, authHeader); err != nil {
		return "", 0, fmt.Errorf("access warm-up: %w", err)
	}

	venueURL := gatheringsURI.JoinPath(pathVenue + gatheringID).String()
	body, err := request(ctx, http.MethodGet, venueURL, nil, authHeader)
	if err != nil {
		return "", 0, err
	}
	type venueResult struct {
		Venue struct {
			ServerIPAddress string `json:"serverIpAddress"`
			ServerPort      int    `json:"serverPort"`
		} `json:"venue"`
	}
	env, err := decodeAPI[apiResult[venueResult]](body, "venue")
	if err != nil {
		return "", 0, err
	}
	if env.Result.Venue.ServerIPAddress == "" {
		// Include raw body so a future Mojang transport change surfaces
		// readably rather than as a silent "no address".
		return "", 0, fmt.Errorf("venue response has no serverIpAddress (raw: %s)", previewBody(body))
	}
	return env.Result.Venue.ServerIPAddress, env.Result.Venue.ServerPort, nil
}
