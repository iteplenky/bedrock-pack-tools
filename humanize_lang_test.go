package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/iteplenky/bedrock-pack-tools/v3/internal/lang"
)

// TestHumanizeClassifiesUnderRussian guards the i18n contract that
// humanize.go pattern-matches upstream/needle error text in English
// regardless of the active language. The keys command wraps connection
// failures via the keys.connect.failed catalog entry, which MUST stay
// the literal `connection to <server> failed:` form so reGameServer
// keeps matching. A Russian translation of that wrap would silently
// drop every game-server diagnostic for RU users.
//
// The test flips the process language to Russian, runs representative
// error chains through humanize, and asserts they still classify. The
// rendered diagnostic text is Russian (not asserted here - the English
// snapshots in humanize_test.go cover wording); what matters is that
// classification still fires (ok == true) and lands on the right branch.
func TestHumanizeClassifiesUnderRussian(t *testing.T) {
	lang.Init("ru", "")
	t.Cleanup(func() { lang.Init("en", "") })

	if lang.Current() != lang.Russian {
		t.Fatalf("expected active language Russian, got %v", lang.Current())
	}

	// The keys command builds this exact wrap from the catalog. Render it
	// through the catalog the same way keys.go does, so a future
	// re-translation of keys.connect.failed breaks this test.
	keysWrap := fmt.Errorf(lang.T("keys.connect.failed"), "play.example.net:19132", "incompatible protocol")
	if !strings.Contains(keysWrap.Error(), "connection to play.example.net:19132 failed:") {
		t.Fatalf("keys.connect.failed must stay the English classification needle, got %q", keysWrap.Error())
	}

	cases := []struct {
		name    string
		err     error
		wantKey string // a substring of the RU headline proving the right branch fired
	}{
		{
			name:    "keys connect wrap -> protocol mismatch",
			err:     keysWrap,
			wantKey: lang.Tf("humanize.protocol.headline", "play.example.net:19132"),
		},
		{
			name:    "game-server app-layer kick",
			err:     fmt.Errorf(lang.T("keys.connect.failed"), "play.example.net:19132", "dial minecraft 1.2.3.4:5->6.7.8.9:19132: nope"),
			wantKey: lang.Tf("humanize.applayerkick.headline", "play.example.net:19132"),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, ok := humanize(c.err)
			if !ok {
				t.Fatalf("humanize did not classify under RU: %v", c.err)
			}
			if !strings.Contains(d.headline, c.wantKey) {
				t.Errorf("RU headline=%q, expected to contain %q", d.headline, c.wantKey)
			}
		})
	}
}
