package franchise

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// ---------- versionInRange / compareDotted ----------

func TestVersionInRange(t *testing.T) {
	cases := []struct {
		name              string
		current, min, max string
		want              bool
	}{
		{"unbounded", "1.21.90", "", "", true},
		{"min only, equal", "1.21.90", "1.21.90", "", true},
		{"min only, above", "1.21.91", "1.21.90", "", true},
		{"min only, below", "1.21.89", "1.21.90", "", false},
		{"max only, equal", "1.21.90", "", "1.21.90", true},
		{"max only, below", "1.21.89", "", "1.21.90", true},
		{"max only, above", "1.21.91", "", "1.21.90", false},
		{"both, inside", "1.21.90", "1.21.0", "1.22.0", true},
		{"both, at min", "1.21.0", "1.21.0", "1.22.0", true},
		{"both, at max", "1.22.0", "1.21.0", "1.22.0", true},
		{"both, below min", "1.20.99", "1.21.0", "1.22.0", false},
		{"both, above max", "1.22.1", "1.21.0", "1.22.0", false},
		// The "1.21.90 vs 1.21.9" bug class - segment-wise numeric MUST
		// rank 90 > 9, while lexicographic compare would rank them wrong.
		{"segment-numeric 1.21.90 > 1.21.9 lower bound", "1.21.90", "1.21.9", "", true},
		{"segment-numeric 1.21.9 < 1.21.90 lower bound", "1.21.9", "1.21.90", "", false},
		{"4-segment dotted", "1.21.90.1", "1.21.90", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := versionInRange(tc.current, tc.min, tc.max); got != tc.want {
				t.Errorf("versionInRange(%q, %q, %q) = %v, want %v", tc.current, tc.min, tc.max, got, tc.want)
			}
		})
	}
}

func TestCompareDotted(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0", "1.0", 0},
		{"1.0", "1.0.0", 0}, // implicit-zero padding
		{"1.0.1", "1.0", 1},
		{"1.0", "1.0.1", -1},
		{"1.21.90", "1.21.9", 1},
		{"1.21.9", "1.21.90", -1},
		{"2.0.0", "1.99.99", 1},
		{"", "", 0},
	}
	for _, tc := range cases {
		got := compareDotted(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("compareDotted(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

// ---------- translatedField.Pick ----------

func TestTranslatedFieldPick(t *testing.T) {
	cases := []struct {
		name string
		in   translatedField
		want string
	}{
		{"NEUTRAL wins", translatedField{"NEUTRAL": "n", "en-US": "e"}, "n"},
		{"en-US fallback", translatedField{"en-US": "e", "ja-JP": "j"}, "e"},
		{"any non-empty if no priority key", translatedField{"de-DE": "d"}, "d"},
		{"empty NEUTRAL skipped", translatedField{"NEUTRAL": "   ", "en-US": "e"}, "e"},
		{"whitespace-only all", translatedField{"NEUTRAL": " ", "en-US": "  "}, ""},
		{"nil map", translatedField(nil), ""},
		{"empty map", translatedField{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.Pick(); got != tc.want {
				t.Errorf("Pick() = %q, want %q", got, tc.want)
			}
		})
	}
}

// ---------- Server ----------

func TestFeaturedServer_HasAddress(t *testing.T) {
	cases := []struct {
		name string
		s    Server
		want bool
	}{
		{"both set", Server{Host: "h", Port: 1}, true},
		{"host empty", Server{Host: "", Port: 1}, false},
		{"port zero", Server{Host: "h", Port: 0}, false},
		{"both empty", Server{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.s.HasAddress(); got != tc.want {
				t.Errorf("HasAddress() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFeaturedServer_Address(t *testing.T) {
	if got := (Server{Host: "example.com", Port: 19132}).Address(); got != "example.com:19132" {
		t.Errorf("Address() = %q, want example.com:19132", got)
	}
	if got := (Server{}).Address(); got != "" {
		t.Errorf("Address() with no host/port = %q, want empty", got)
	}
}

// ---------- partnerCatalog (httptest) ----------

func TestFetchPartnerCatalog_Decode(t *testing.T) {
	// Two items: one direct (url+port), one experience-only.
	resp := `{
		"status":"OK","code":200,
		"data":{
			"Count":2,
			"Items":[
				{"Title":{"NEUTRAL":"Direct"},"DisplayProperties":{"url":"a.example","port":19132}},
				{"Title":{"NEUTRAL":"Exp"},"DisplayProperties":{"experienceId":"abc-123"}}
			]
		}
	}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, pathBlob) {
			t.Errorf("path = %s, want suffix %s", r.URL.Path, pathBlob)
		}
		_, _ = io.WriteString(w, resp)
	}))
	defer ts.Close()

	u, _ := url.Parse(ts.URL)
	got, err := partnerCatalog(context.Background(), u, "MCToken x")
	if err != nil {
		t.Fatalf("partnerCatalog: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d items, want 2", len(got))
	}
	if got[0].Kind != KindPartnerDirect || got[0].Host != "a.example" || got[0].Port != 19132 {
		t.Errorf("item 0 = %+v, want direct a.example:19132", got[0])
	}
	if got[1].Kind != KindPartnerExperience || got[1].ExperienceID != "abc-123" {
		t.Errorf("item 1 = %+v, want experience abc-123", got[1])
	}
}

func TestFetchPartnerCatalog_FiltersVersion(t *testing.T) {
	// Item asks for an absurdly high minClientVersion; should be filtered out.
	resp := `{
		"status":"OK","code":200,
		"data":{
			"Count":1,
			"Items":[
				{"Title":{"NEUTRAL":"FromFuture"},"DisplayProperties":{"url":"a","port":1,"minClientVersion":"99.0.0"}}
			]
		}
	}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, resp)
	}))
	defer ts.Close()
	u, _ := url.Parse(ts.URL)
	got, err := partnerCatalog(context.Background(), u, "MCToken x")
	if err != nil {
		t.Fatalf("partnerCatalog: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d items, want 0 (future minClientVersion should filter)", len(got))
	}
}

// ---------- liveEvents (httptest) ----------

func TestFetchGatherings_Decode(t *testing.T) {
	resp := `{
		"status":"OK","code":200,
		"result":[
			{"gatheringId":"g1","title":"Live Now","isEnabled":true,"isPrivate":false},
			{"gatheringId":"g2","title":"Disabled","isEnabled":false},
			{"gatheringId":"g3","title":"Private","isEnabled":true,"isPrivate":true}
		]
	}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if got := r.URL.Query().Get("clientPlatform"); got != "Windows10" {
			t.Errorf("clientPlatform = %q, want Windows10", got)
		}
		_, _ = io.WriteString(w, resp)
	}))
	defer ts.Close()
	u, _ := url.Parse(ts.URL)
	got, err := liveEvents(context.Background(), u, "MCToken x")
	if err != nil {
		t.Fatalf("liveEvents: %v", err)
	}
	if len(got) != 1 || got[0].GatheringID != "g1" {
		t.Errorf("got %+v, want only g1 (Disabled/Private filtered)", got)
	}
	if got[0].Kind != KindGathering {
		t.Errorf("got[0].Kind = %v, want KindGathering", got[0].Kind)
	}
}

// TestFetchGatherings_TimeWindow exercises the StartTimeUtc/EndTimeUtc guards:
// a future start or a past end drops the event; an open window or an unset
// (zero) time keeps it.
func TestFetchGatherings_TimeWindow(t *testing.T) {
	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	resp := fmt.Sprintf(`{"status":"OK","code":200,"result":[
		{"gatheringId":"future","title":"Not yet","isEnabled":true,"startTimeUtc":%q},
		{"gatheringId":"ended","title":"Over","isEnabled":true,"endTimeUtc":%q},
		{"gatheringId":"open","title":"Open","isEnabled":true,"startTimeUtc":%q,"endTimeUtc":%q},
		{"gatheringId":"unbounded","title":"Always","isEnabled":true}
	]}`, future, past, past, future)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, resp)
	}))
	defer ts.Close()
	u, _ := url.Parse(ts.URL)

	got, err := liveEvents(context.Background(), u, "MCToken x")
	if err != nil {
		t.Fatalf("liveEvents: %v", err)
	}
	kept := map[string]bool{}
	for _, s := range got {
		kept[s.GatheringID] = true
	}
	if !kept["open"] || !kept["unbounded"] {
		t.Errorf("open + unbounded should be kept, got %+v", got)
	}
	if kept["future"] || kept["ended"] {
		t.Errorf("future/ended should be dropped, got %+v", got)
	}
}

// ---------- joinExperience ----------

func TestJoinExperience_OK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, pathJoinExpURL) {
			t.Errorf("path = %s, want suffix %s", r.URL.Path, pathJoinExpURL)
		}
		_, _ = io.WriteString(w, `{"status":"OK","code":200,"result":{"ipV4Address":"1.2.3.4","port":19132}}`)
	}))
	defer ts.Close()
	u, _ := url.Parse(ts.URL)
	host, port, err := joinExperience(context.Background(), u, "MCToken x", "exp-1")
	if err != nil {
		t.Fatalf("joinExperience: %v", err)
	}
	if host != "1.2.3.4" || port != 19132 {
		t.Errorf("got %s:%d, want 1.2.3.4:19132", host, port)
	}
}

func TestJoinExperience_404IsOffline(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	u, _ := url.Parse(ts.URL)
	_, _, err := joinExperience(context.Background(), u, "MCToken x", "exp-1")
	if !errors.Is(err, ErrExperienceOffline) {
		t.Errorf("got %v, want ErrExperienceOffline", err)
	}
}

func TestJoinExperience_ShapelessIsOffline(t *testing.T) {
	// A 200 OK with no address fields should map to ErrExperienceOffline
	// so callers don't have to second-guess. The unexpected-shape note
	// is wrapped via %w so errors.Is still hits the sentinel.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"result":{}}`)
	}))
	defer ts.Close()
	u, _ := url.Parse(ts.URL)
	_, _, err := joinExperience(context.Background(), u, "MCToken x", "exp-1")
	if !errors.Is(err, ErrExperienceOffline) {
		t.Errorf("got %v, want ErrExperienceOffline", err)
	}
}

func TestLiveEvents_404IsEmpty(t *testing.T) {
	// A 404 from config/public means "no active events" - liveEvents should
	// return an empty list with no error, so the menu shows no warning.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	u, _ := url.Parse(ts.URL)
	got, err := liveEvents(context.Background(), u, "MCToken x")
	if err != nil {
		t.Errorf("got err %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d events, want 0", len(got))
	}
}

// ---------- venue ----------

func TestVenueAddress_OK(t *testing.T) {
	hit := struct {
		access bool
		venue  bool
	}{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, pathAccess):
			hit.access = true
			_, _ = io.WriteString(w, `{}`)
		case strings.Contains(r.URL.Path, pathVenue):
			hit.venue = true
			_, _ = io.WriteString(w, `{"status":"OK","code":200,"result":{"venue":{"serverIpAddress":"5.6.7.8","serverPort":19133}}}`)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
	}))
	defer ts.Close()
	u, _ := url.Parse(ts.URL)
	host, port, err := venue(context.Background(), u, "MCToken x", "gath-1")
	if err != nil {
		t.Fatalf("venue: %v", err)
	}
	if !hit.access || !hit.venue {
		t.Errorf("expected both /access and /venue to be hit, got %+v", hit)
	}
	if host != "5.6.7.8" || port != 19133 {
		t.Errorf("got %s:%d, want 5.6.7.8:19133", host, port)
	}
}

func TestVenueAddress_AccessFails(t *testing.T) {
	// /access returns 500. We surface a wrapped "access warm-up" error
	// instead of pretending the venue lookup happened.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, pathAccess) {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		t.Errorf("venue should not be called when access fails")
	}))
	defer ts.Close()
	u, _ := url.Parse(ts.URL)
	_, _, err := venue(context.Background(), u, "MCToken x", "gath-1")
	if err == nil {
		t.Fatal("expected error from /access failure, got nil")
	}
	if !strings.Contains(err.Error(), "access warm-up") {
		t.Errorf("err = %v, want it to mention 'access warm-up'", err)
	}
}

func TestVenueAddress_NoServerIP(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, pathAccess) {
			_, _ = io.WriteString(w, `{}`)
			return
		}
		_, _ = io.WriteString(w, `{"result":{"venue":{}}}`)
	}))
	defer ts.Close()
	u, _ := url.Parse(ts.URL)
	_, _, err := venue(context.Background(), u, "MCToken x", "gath-1")
	if err == nil {
		t.Fatal("expected error when venue has no serverIpAddress, got nil")
	}
	if !strings.Contains(err.Error(), "serverIpAddress") {
		t.Errorf("err = %v, want it to mention serverIpAddress", err)
	}
}

func TestFetchGatherings_EmptyCohort(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"result":[]}`)
	}))
	defer ts.Close()
	u, _ := url.Parse(ts.URL)
	got, err := liveEvents(context.Background(), u, "MCToken x")
	if err != nil {
		t.Fatalf("liveEvents on empty cohort: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("empty cohort should yield zero items, got %d", len(got))
	}
}
