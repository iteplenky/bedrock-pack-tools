package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// ---------- versionInRange / compareDotted ----------

func TestVersionInRange(t *testing.T) {
	cases := []struct {
		name             string
		current, min, max string
		want             bool
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
		{"1.0", "1.0.0", 0},  // implicit-zero padding
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

// ---------- featuredServer ----------

func TestFeaturedServer_HasAddress(t *testing.T) {
	cases := []struct {
		name string
		s    featuredServer
		want bool
	}{
		{"both set", featuredServer{Host: "h", Port: 1}, true},
		{"host empty", featuredServer{Host: "", Port: 1}, false},
		{"port zero", featuredServer{Host: "h", Port: 0}, false},
		{"both empty", featuredServer{}, false},
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
	if got := (featuredServer{Host: "example.com", Port: 19132}).Address(); got != "example.com:19132" {
		t.Errorf("Address() = %q, want example.com:19132", got)
	}
	if got := (featuredServer{}).Address(); got != "" {
		t.Errorf("Address() with no host/port = %q, want empty", got)
	}
}

// ---------- fetchPartnerCatalog (httptest) ----------

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
		if !strings.HasSuffix(r.URL.Path, gatheringsBlobPath) {
			t.Errorf("path = %s, want suffix %s", r.URL.Path, gatheringsBlobPath)
		}
		_, _ = io.WriteString(w, resp)
	}))
	defer ts.Close()

	u, _ := url.Parse(ts.URL)
	got, err := fetchPartnerCatalog(context.Background(), u, "MCToken x")
	if err != nil {
		t.Fatalf("fetchPartnerCatalog: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d items, want 2", len(got))
	}
	if got[0].Kind != kindPartnerDirect || got[0].Host != "a.example" || got[0].Port != 19132 {
		t.Errorf("item 0 = %+v, want direct a.example:19132", got[0])
	}
	if got[1].Kind != kindPartnerExperience || got[1].ExperienceID != "abc-123" {
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
	got, err := fetchPartnerCatalog(context.Background(), u, "MCToken x")
	if err != nil {
		t.Fatalf("fetchPartnerCatalog: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d items, want 0 (future minClientVersion should filter)", len(got))
	}
}

// ---------- fetchGatherings (httptest) ----------

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
	got, err := fetchGatherings(context.Background(), u, "MCToken x")
	if err != nil {
		t.Fatalf("fetchGatherings: %v", err)
	}
	if len(got) != 1 || got[0].GatheringID != "g1" {
		t.Errorf("got %+v, want only g1 (Disabled/Private filtered)", got)
	}
	if got[0].Kind != kindGathering {
		t.Errorf("got[0].Kind = %v, want kindGathering", got[0].Kind)
	}
}

func TestFetchGatherings_EmptyCohort(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"result":[]}`)
	}))
	defer ts.Close()
	u, _ := url.Parse(ts.URL)
	got, err := fetchGatherings(context.Background(), u, "MCToken x")
	if err != nil {
		t.Fatalf("fetchGatherings on empty cohort: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("empty cohort should yield zero items, got %d", len(got))
	}
}
