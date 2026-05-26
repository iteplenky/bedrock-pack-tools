package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
)

// Paths on the gatherings service, observed from live wire traffic.
// All require an MCToken Authorization header except where noted.
const (
	gatheringsBlobPath    = "/api/v2.0/discovery/blob/client"
	gatheringsConfigPath  = "/api/v1.0/config/public"
	gatheringsAccessPath  = "/api/v1.0/access"
	gatheringsVenuePath   = "/api/v1.0/venue/"
	gatheringsJoinExpPath = "/api/v2.0/join/experience"
)

// errExperienceOffline is returned when an experienceId resolves but the
// API reports no active venue. Distinct from [errAuthRejected] because
// no retry helps; the caller surfaces it as "currently offline".
var errExperienceOffline = errors.New("gatherings: experience has no active venue")

// translatedField is the title/description value shape Mojang uses for
// catalog entries: a locale → string map keyed by region tag. The retail
// client picks the user's locale; we read NEUTRAL first then fall back
// to en-US then anything non-empty so a locale-only entry never renders
// as a blank cell.
type translatedField map[string]string

func (t translatedField) Pick() string {
	if v := strings.TrimSpace(t["NEUTRAL"]); v != "" {
		return v
	}
	if v := strings.TrimSpace(t["en-US"]); v != "" {
		return v
	}
	for _, v := range t {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

// featuredKind classifies a row by how the client would reach its target.
// The featured subcommand uses it to pick a status tag and a download
// resolution path.
type featuredKind int

const (
	// kindPartnerDirect: catalog item with inline url:port; reachable
	// over plain RakNet without further API calls.
	kindPartnerDirect featuredKind = iota
	// kindPartnerExperience: catalog item with only experienceId.
	// Address is resolved on demand via joinExperience; the in-game
	// client routes the same way.
	kindPartnerExperience
	// kindGathering: a live event from /config/public. Address is
	// resolved via /access + /venue/{gatheringId}.
	kindGathering
)

// featuredServer is the flattened view of one Featured-tab row used by
// the featured subcommand. Catalog source and resolve path differ by
// Kind; Online/MOTD/Players are populated by the raknet ping pass and
// only meaningful for kindPartnerDirect.
type featuredServer struct {
	Kind         featuredKind
	Name         string
	Host         string
	Port         int
	ExperienceID string
	GatheringID  string

	Online  bool
	MOTD    string
	Players int
}

// HasAddress reports whether the catalog entry carries a public host:port.
// Experience-only and gathering rows return false; their address has to
// be resolved through the appropriate API call.
func (f featuredServer) HasAddress() bool {
	return f.Host != "" && f.Port != 0
}

// Address returns "host:port" when HasAddress is true, "" otherwise.
func (f featuredServer) Address() string {
	if !f.HasAddress() {
		return ""
	}
	return fmt.Sprintf("%s:%d", f.Host, f.Port)
}

// ---- partner catalog ------------------------------------------------

type blobItem struct {
	Title             translatedField  `json:"Title"`
	DisplayProperties blobDisplayProps `json:"DisplayProperties"`
}

type blobDisplayProps struct {
	URL              string   `json:"url"`
	Port             int      `json:"port"`
	ExperienceID     string   `json:"experienceId"`
	MinClientVersion string   `json:"minClientVersion"`
	MaxClientVersion string   `json:"maxClientVersion"`
	Tags             []string `json:"tags"`
}

type blobData struct {
	Count int        `json:"Count"`
	Items []blobItem `json:"Items"`
}

// fetchPartnerCatalog posts to the blob endpoint and flattens each item
// into a [featuredServer], dropping entries whose minClientVersion or
// maxClientVersion window excludes our current build (matching the
// in-game client's own filtering - otherwise we'd display rows the user
// can't actually reach).
func fetchPartnerCatalog(ctx context.Context, gatheringsURI *url.URL, authHeader string) ([]featuredServer, error) {
	u := gatheringsURI.JoinPath(gatheringsBlobPath).String()
	body, err := franchiseRequest(ctx, http.MethodPost, u, nil, authHeader)
	if err != nil {
		return nil, err
	}
	env, err := decodeAPI[apiData[blobData]](body, "blob")
	if err != nil {
		return nil, err
	}

	out := make([]featuredServer, 0, len(env.Data.Items))
	for _, item := range env.Data.Items {
		if !versionInRange(protocol.CurrentVersion, item.DisplayProperties.MinClientVersion, item.DisplayProperties.MaxClientVersion) {
			continue
		}
		kind := kindPartnerDirect
		if item.DisplayProperties.URL == "" || item.DisplayProperties.Port == 0 {
			kind = kindPartnerExperience
		}
		out = append(out, featuredServer{
			Kind:         kind,
			Name:         item.Title.Pick(),
			Host:         item.DisplayProperties.URL,
			Port:         item.DisplayProperties.Port,
			ExperienceID: item.DisplayProperties.ExperienceID,
		})
	}
	return out, nil
}

// ---- live events ----------------------------------------------------

type gathering struct {
	GatheringID   string    `json:"gatheringId"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	GatheringType string    `json:"gatheringType"`
	StartTimeUtc  time.Time `json:"startTimeUtc"`
	EndTimeUtc    time.Time `json:"endTimeUtc"`
	IsEnabled     bool      `json:"isEnabled"`
	IsPrivate     bool      `json:"isPrivate"`
}

// fetchGatherings hits /config/public to enumerate live events the
// client surfaces above the partner list. Returns an empty slice when
// the cohort has no active events — that's a normal state, not an error.
func fetchGatherings(ctx context.Context, gatheringsURI *url.URL, authHeader string) ([]featuredServer, error) {
	q := url.Values{}
	q.Set("lang", "en-GB")
	q.Set("clientVersion", protocol.CurrentVersion)
	q.Set("clientPlatform", "Windows10")
	q.Set("clientSubPlatform", "Windows10")
	u := gatheringsURI.JoinPath(gatheringsConfigPath)
	u.RawQuery = q.Encode()
	body, err := franchiseRequest(ctx, http.MethodGet, u.String(), nil, authHeader)
	if err != nil {
		return nil, err
	}
	env, err := decodeAPI[apiResult[[]gathering]](body, "gatherings")
	if err != nil {
		return nil, err
	}
	now := time.Now()
	out := make([]featuredServer, 0, len(env.Result))
	for _, g := range env.Result {
		if !g.IsEnabled || g.IsPrivate {
			continue
		}
		if !g.StartTimeUtc.IsZero() && now.Before(g.StartTimeUtc) {
			continue
		}
		if !g.EndTimeUtc.IsZero() && now.After(g.EndTimeUtc) {
			continue
		}
		out = append(out, featuredServer{
			Kind:        kindGathering,
			Name:        g.Title,
			GatheringID: g.GatheringID,
		})
	}
	return out, nil
}

// ---- address resolution ---------------------------------------------

// joinExperience resolves an experienceId to a live ip:port via the
// /join/experience POST. Returns [errExperienceOffline] on 404 (the
// in-game client gets the same response when an experience has no
// active venue right now).
func joinExperience(ctx context.Context, gatheringsURI *url.URL, authHeader, experienceID string) (host string, port int, err error) {
	u := gatheringsURI.JoinPath(gatheringsJoinExpPath).String()
	payload, _ := json.Marshal(map[string]string{"experienceId": experienceID})
	body, err := franchiseRequest(ctx, http.MethodPost, u, payload, authHeader)
	if err != nil {
		if status := httpStatusOf(err); status == http.StatusNotFound {
			return "", 0, errExperienceOffline
		}
		return "", 0, err
	}
	// Even though this is a POST, the franchise services use the
	// {status,code,result} envelope here — same as /venue/{id} and
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
		// Treat a shapeless 200 as "offline" but include the raw body in the
		// wrapped error so a future Mojang transport change (e.g. NetherNet
		// signaling instead of plain ip:port) surfaces visibly rather than
		// silently mapping to "no active venue". [errors.Is] at the call
		// site still hits [errExperienceOffline].
		return "", 0, fmt.Errorf("%w (unexpected response shape: %s)", errExperienceOffline, previewBody(body))
	}
	return env.Result.IPV4Address, env.Result.Port, nil
}

// venueAddress resolves a gathering's current venue via /access (a
// warm-up the client always does first, even though the response is
// discarded) + /venue/{id}. Skipping /access has been observed to make
// /venue 404 against some Mojang edges — likely a load-balancer affinity
// thing — so keep the call even though it looks redundant.
func venueAddress(ctx context.Context, gatheringsURI *url.URL, authHeader, gatheringID string) (host string, port int, err error) {
	q := url.Values{}
	q.Set("lang", "en-GB")
	q.Set("clientVersion", protocol.CurrentVersion)
	q.Set("clientPlatform", "Windows10")
	q.Set("clientSubPlatform", "Windows10")
	accessURL := gatheringsURI.JoinPath(gatheringsAccessPath)
	accessURL.RawQuery = q.Encode()
	if _, err := franchiseRequest(ctx, http.MethodGet, accessURL.String(), nil, authHeader); err != nil {
		return "", 0, fmt.Errorf("access warm-up: %w", err)
	}

	venueURL := gatheringsURI.JoinPath(gatheringsVenuePath + gatheringID).String()
	body, err := franchiseRequest(ctx, http.MethodGet, venueURL, nil, authHeader)
	if err != nil {
		return "", 0, err
	}
	type venueResult struct {
		Venue struct {
			ServerIpAddress string `json:"serverIpAddress"`
			ServerPort      int    `json:"serverPort"`
		} `json:"venue"`
	}
	env, err := decodeAPI[apiResult[venueResult]](body, "venue")
	if err != nil {
		return "", 0, err
	}
	if env.Result.Venue.ServerIpAddress == "" {
		// Include the raw body so a future Mojang change (e.g. switching
		// gatherings to NetherNet signaling or a regional-lockout envelope)
		// surfaces as a readable error instead of a silent "no address".
		return "", 0, fmt.Errorf("venue response has no serverIpAddress (raw: %s)", previewBody(body))
	}
	return env.Result.Venue.ServerIpAddress, env.Result.Venue.ServerPort, nil
}

// ---- version compare ------------------------------------------------

// versionInRange reports whether current ∈ [min, max], comparing dotted
// numeric segments rather than lexicographically — "1.21.90" must be
// greater than "1.21.9", which a string compare gets wrong. Empty bounds
// are treated as unbounded.
func versionInRange(current, min, max string) bool {
	if min != "" && compareDotted(current, min) < 0 {
		return false
	}
	if max != "" && compareDotted(current, max) > 0 {
		return false
	}
	return true
}

func compareDotted(a, b string) int {
	pa := strings.Split(a, ".")
	pb := strings.Split(b, ".")
	n := max(len(pa), len(pb))
	for i := range n {
		ai, bi := 0, 0
		if i < len(pa) {
			ai, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			bi, _ = strconv.Atoi(pb[i])
		}
		if ai != bi {
			if ai < bi {
				return -1
			}
			return 1
		}
	}
	return 0
}
