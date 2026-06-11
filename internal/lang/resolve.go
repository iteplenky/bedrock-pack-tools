package lang

import (
	"os"
	"strings"
)

// localePrecedence is the ordered list of POSIX locale environment
// variables consulted after the explicit/persisted sources, highest
// precedence first: LC_ALL overrides LC_MESSAGES, which overrides LANG.
var localePrecedence = []string{"LC_ALL", "LC_MESSAGES", "LANG"}

// Init resolves and fixes the active language for the rest of the
// process. Precedence, highest first:
//
//  1. flagValue   - the --lang / -lang value (empty when unset)
//  2. BPT_LANG    - the tool's own override env var
//  3. persisted   - the language saved in the store (Settings toggle)
//  4. LC_ALL      - POSIX: overrides all other locale categories
//  5. LC_MESSAGES - POSIX: message-catalog locale
//  6. LANG        - POSIX: default locale
//
// The first non-empty source that parses to a known language wins. Any
// value that doesn't parse as Russian (including the POSIX "C" / "POSIX"
// locales, an empty string, or an unrecognized tag) leaves the language
// English. Init is safe to call exactly once at startup; it must run
// before any user-facing output.
func Init(flagValue, persisted string) {
	for _, v := range []string{flagValue, os.Getenv("BPT_LANG"), persisted} {
		if l, ok := parse(v); ok {
			active.Store(int32(l))
			return
		}
	}
	for _, name := range localePrecedence {
		if l, ok := parse(os.Getenv(name)); ok {
			active.Store(int32(l))
			return
		}
	}
	active.Store(int32(English))
}

// parse maps a raw flag/env value to a language. ok is false when the
// value is empty or unparseable, letting Init fall through to the next
// source. A value that parses but isn't Russian resolves to English
// with ok=true, so an explicit "en" or "fr" stops the precedence walk
// rather than leaking a lower-priority locale through.
//
// Accepted forms (case-insensitive): "ru", "russian", "ru_RU.UTF-8",
// "ru-RU", "en", "english", "en_US.UTF-8", "en-GB". The language
// subtag is taken as the run of letters before the first separator
// ('_', '-', '.', or '@').
func parse(value string) (Lang, bool) {
	v := strings.TrimSpace(value)
	if v == "" {
		return English, false
	}
	tag := strings.ToLower(primarySubtag(v))
	switch tag {
	case "ru", "rus", "russian":
		return Russian, true
	case "en", "eng", "english":
		return English, true
	}
	// The POSIX "C" / "POSIX" locales mean "no localization": treat them
	// as a deliberate request for the default (English), not as unknown.
	if tag == "c" || tag == "posix" {
		return English, true
	}
	return English, false
}

// primarySubtag returns the leading language subtag of a locale string,
// i.e. the run of characters up to the first '_', '-', '.', or '@'
// (territory, codeset, and modifier separators). "ru_RU.UTF-8" -> "ru",
// "en-GB" -> "en", "ru" -> "ru".
func primarySubtag(v string) string {
	cut := len(v)
	for i, r := range v {
		if r == '_' || r == '-' || r == '.' || r == '@' {
			cut = i
			break
		}
	}
	return v[:cut]
}
