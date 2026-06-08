package franchise

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/iteplenky/gophertunnel/minecraft/protocol"
)

const (
	pathBlob       = "/api/v2.0/discovery/blob/client"
	pathConfig     = "/api/v1.0/config/public"
	pathAccess     = "/api/v1.0/access"
	pathVenue      = "/api/v1.0/venue/"
	pathJoinExpURL = "/api/v2.0/join/experience"
)

// Kind classifies how the in-game client reaches a Server row's target.
type Kind int

const (
	// KindPartnerDirect is an inline url:port entry over plain RakNet.
	KindPartnerDirect Kind = iota
	// KindPartnerExperience carries only an experienceId, resolved via JoinExperience.
	KindPartnerExperience
	// KindGathering is a live event from /config/public, resolved via Venue.
	KindGathering
)

// Server is the flattened Featured-tab row. Online/MOTD/Players are
// not populated by this package; callers fill them after PartnerCatalog
// / LiveEvents returns (typically via raknet ping).
type Server struct {
	Kind         Kind
	Name         string
	Host         string
	Port         int
	ExperienceID string
	GatheringID  string

	Online  bool
	MOTD    string
	Players int
}

// HasAddress is true for inline host:port. Experience and gathering
// rows return false; their address is resolved via the API.
func (s Server) HasAddress() bool {
	return s.Host != "" && s.Port != 0
}

// Address returns "host:port" when HasAddress is true, "" otherwise.
func (s Server) Address() string {
	if !s.HasAddress() {
		return ""
	}
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

// clientQuery is the lang/clientVersion/clientPlatform tuple the
// in-game Bedrock client puts on /config/public and /access.
func clientQuery() url.Values {
	q := url.Values{}
	q.Set("lang", "en-GB")
	q.Set("clientVersion", protocol.CurrentVersion)
	q.Set("clientPlatform", "Windows10")
	q.Set("clientSubPlatform", "Windows10")
	return q
}

// translatedField is Mojang's locale-keyed string map. Pick prefers
// NEUTRAL, then en-US, then anything non-empty.
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

type blobItem struct {
	Title             translatedField  `json:"Title"`
	DisplayProperties blobDisplayProps `json:"DisplayProperties"`
}

type blobDisplayProps struct {
	URL              string `json:"url"`
	Port             int    `json:"port"`
	ExperienceID     string `json:"experienceId"`
	MinClientVersion string `json:"minClientVersion"`
	MaxClientVersion string `json:"maxClientVersion"`
}

type blobData struct {
	Items []blobItem `json:"Items"`
}

// PartnerCatalog posts to /blob/client and flattens each item into a
// Server. Drops entries outside the min/maxClientVersion window so we
// don't return rows the user can't reach.
func (c *Client) PartnerCatalog(ctx context.Context) ([]Server, error) {
	tok, err := c.Token(ctx)
	if err != nil {
		return nil, err
	}
	return partnerCatalog(ctx, c.gatheringsURI, tok.AuthorizationHeader)
}

func partnerCatalog(ctx context.Context, gatheringsURI *url.URL, authHeader string) ([]Server, error) {
	u := gatheringsURI.JoinPath(pathBlob).String()
	body, err := request(ctx, http.MethodPost, u, nil, authHeader)
	if err != nil {
		return nil, err
	}
	env, err := decodeAPI[apiData[blobData]](body, "blob")
	if err != nil {
		return nil, err
	}

	out := make([]Server, 0, len(env.Data.Items))
	for _, item := range env.Data.Items {
		if !versionInRange(protocol.CurrentVersion, item.DisplayProperties.MinClientVersion, item.DisplayProperties.MaxClientVersion) {
			continue
		}
		kind := KindPartnerDirect
		if item.DisplayProperties.URL == "" || item.DisplayProperties.Port == 0 {
			kind = KindPartnerExperience
		}
		out = append(out, Server{
			Kind:         kind,
			Name:         item.Title.Pick(),
			Host:         item.DisplayProperties.URL,
			Port:         item.DisplayProperties.Port,
			ExperienceID: item.DisplayProperties.ExperienceID,
		})
	}
	return out, nil
}

type gathering struct {
	GatheringID  string    `json:"gatheringId"`
	Title        string    `json:"title"`
	StartTimeUtc time.Time `json:"startTimeUtc"`
	EndTimeUtc   time.Time `json:"endTimeUtc"`
	IsEnabled    bool      `json:"isEnabled"`
	IsPrivate    bool      `json:"isPrivate"`
}

// LiveEvents enumerates currently-active gatherings from /config/public.
// Empty slice is normal (most cohorts have no active events).
func (c *Client) LiveEvents(ctx context.Context) ([]Server, error) {
	tok, err := c.Token(ctx)
	if err != nil {
		return nil, err
	}
	return liveEvents(ctx, c.gatheringsURI, tok.AuthorizationHeader)
}

func liveEvents(ctx context.Context, gatheringsURI *url.URL, authHeader string) ([]Server, error) {
	u := gatheringsURI.JoinPath(pathConfig)
	u.RawQuery = clientQuery().Encode()
	body, err := request(ctx, http.MethodGet, u.String(), nil, authHeader)
	if err != nil {
		return nil, err
	}
	env, err := decodeAPI[apiResult[[]gathering]](body, "gatherings")
	if err != nil {
		return nil, err
	}
	now := time.Now()
	out := make([]Server, 0, len(env.Result))
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
		out = append(out, Server{
			Kind:        KindGathering,
			Name:        g.Title,
			GatheringID: g.GatheringID,
		})
	}
	return out, nil
}

// versionInRange compares dotted-numeric segments (so "1.21.90" > "1.21.9",
// which lexicographic compare gets wrong). Empty bounds are unbounded.
func versionInRange(current, lo, hi string) bool {
	if lo != "" && compareDotted(current, lo) < 0 {
		return false
	}
	if hi != "" && compareDotted(current, hi) > 0 {
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
