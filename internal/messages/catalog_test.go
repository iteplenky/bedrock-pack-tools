package messages

import (
	"regexp"
	"strings"
	"testing"

	"github.com/iteplenky/bedrock-pack-tools/v3/internal/lang"
)

// verbRE matches a single printf verb (flags, width/precision, then the
// verb letter), e.g. %s %d %q %v %w %.1f %02d %-50s. The escaped %% is
// stripped before matching so it is never counted.
var verbRE = regexp.MustCompile(`%[#+\- 0]*[0-9.*]*[a-zA-Z]`)

// verbTypes returns the ordered sequence of verb letters in s (the last
// rune of each verb), e.g. "%s failed: %w" -> "sw". fmt fills verbs
// positionally, so EN and RU must share the same ordered sequence or the
// translated string would format the wrong arguments.
func verbTypes(s string) string {
	s = strings.ReplaceAll(s, "%%", "")
	var b strings.Builder
	for _, v := range verbRE.FindAllString(s, -1) {
		b.WriteByte(v[len(v)-1])
	}
	return b.String()
}

// TestCatalogSymmetry guards the whole EN/RU catalog (registered by this
// package's init functions): every key exists in both languages and the
// two strings carry the same ordered printf-verb sequence, so lang.Tf
// can never panic or print %!(MISSING)/%!s(int=...) for a translated key.
func TestCatalogSymmetry(t *testing.T) {
	en := lang.Snapshot(lang.English)
	ru := lang.Snapshot(lang.Russian)
	if len(en) == 0 || len(ru) == 0 {
		t.Fatalf("catalogs not registered (en=%d ru=%d) - is the package init running?", len(en), len(ru))
	}

	for k := range en {
		if _, ok := ru[k]; !ok {
			t.Errorf("key %q is in EN but missing in RU", k)
		}
	}
	for k := range ru {
		if _, ok := en[k]; !ok {
			t.Errorf("key %q is in RU but missing in EN", k)
		}
	}

	for k, ev := range en {
		rv, ok := ru[k]
		if !ok {
			continue
		}
		if everbs, rverbs := verbTypes(ev), verbTypes(rv); everbs != rverbs {
			t.Errorf("key %q: EN verbs %q != RU verbs %q\n  EN: %s\n  RU: %s", k, everbs, rverbs, ev, rv)
		}
	}
}
