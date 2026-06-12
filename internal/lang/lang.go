// Package lang is the i18n foundation for bedrock-pack-tools.
//
// It holds a single process-wide active language, resolved once at
// startup via Init, plus a package-global message catalog that any
// file in any package may contribute to from its own init().
//
// # Active language
//
// English is the default and the fallback. Init resolves the active
// language from (in precedence order) the --lang flag value, then
// BPT_LANG, the persisted store language, LC_ALL, LC_MESSAGES, and
// LANG. Anything that doesn't parse as Russian leaves the language
// English. The active language is stored atomically so the Settings
// toggle can switch it at runtime while command/spinner goroutines
// read it concurrently.
//
// # Catalog and keys
//
// A message is a dotted key (e.g. "tui.menu.quit",
// "err.noprofile.headline") mapped to a per-language string. Keys are
// namespaced by domain so that catalogs contributed from different
// files never collide. The leading segment is the domain prefix; pick
// one prefix per file/domain and keep every key under it:
//
//	tui.*        interactive menu labels, hints, prompts
//	err.*        humanize diagnostic output (headline/body/causes/fix)
//	keys.*       the `keys` subcommand
//	download.*   the `download` subcommand
//	decrypt.*    the `decrypt` subcommand
//	encrypt.*    the `encrypt` subcommand
//	featured.*   the `featured` subcommand
//	auth.*       login / logout
//	usage.*      top-level usage / help text
//	common.*     shared one-liners (spinner verbs, generic words)
//
// Pick a unique domain prefix before registering and parallel agents
// will never clash, because Register only ever inserts new keys.
//
// # Registration
//
// Each domain registers its own EN and RU catalogs from its own file's
// init(), so there is no shared catalog file to serialize edits
// through:
//
//	func init() {
//		lang.Register(lang.English, map[string]string{
//			"tui.menu.quit": "Quit",
//		})
//		lang.Register(lang.Russian, map[string]string{
//			"tui.menu.quit": "Выход",
//		})
//	}
//
// # Lookup
//
// T(key) returns the active-language string, falling back to English,
// then to the key itself when nothing is registered. Tf wraps T with
// fmt.Sprintf. Tlist splits a registered string on "\n" for the
// multi-line groups humanize uses (causes lists, multi-step fixes).
package lang

import (
	"fmt"
	"maps"
	"strings"
	"sync/atomic"
)

// Lang identifies an active language. The zero value is English so an
// un-Init'd process still renders the default catalog.
type Lang int

const (
	// English is the default and the fallback language.
	English Lang = iota
	// Russian is the only non-default language currently shipped.
	Russian
)

// String returns the lowercase ISO-639-1 tag for l ("en" / "ru").
// Unknown values render as "en" so callers never see an empty tag.
func (l Lang) String() string {
	switch l {
	case Russian:
		return "ru"
	default:
		return "en"
	}
}

// active is the process-wide language. The zero value is 0 == English,
// so an un-Init'd process renders the default catalog. It is held in an
// atomic so the Settings toggle (SetActive) can switch languages at
// runtime while command/spinner goroutines read it via T/Current.
var active atomic.Int32

// catalog holds every registered message, keyed by language then by
// dotted message key. Populated entirely from init() functions, which
// the Go runtime runs single-threaded before main, so Register needs
// no locking.
var catalog = map[Lang]map[string]string{
	English: {},
	Russian: {},
}

// Register merges entries into l's catalog. It is meant to be called
// from init() in the file that owns a domain prefix. Later entries for
// the same key overwrite earlier ones, but since each domain owns a
// unique prefix that should never happen across files.
func Register(l Lang, entries map[string]string) {
	m, ok := catalog[l]
	if !ok {
		m = map[string]string{}
		catalog[l] = m
	}
	maps.Copy(m, entries)
}

// Current returns the active language resolved by Init (English until
// Init runs), or the last value set by SetActive.
func Current() Lang { return Lang(active.Load()) }

// SetActive switches the process-wide language at runtime. It is safe
// to call from any goroutine; readers via T/Current observe the new
// value on their next call.
func SetActive(l Lang) { active.Store(int32(l)) }

// Snapshot returns a copy of every message registered for l. It exists
// for tests and tooling that inspect the catalog (e.g. checking EN/RU
// key and format-verb symmetry); mutating the result does not affect
// the live catalog.
func Snapshot(l Lang) map[string]string {
	out := make(map[string]string, len(catalog[l]))
	maps.Copy(out, catalog[l])
	return out
}

// T returns the message registered for key in the active language. It
// falls back to the English entry, then to key itself when no entry
// exists in either - so a missing translation degrades to a visible,
// greppable key rather than an empty string.
func T(key string) string {
	a := Lang(active.Load())
	if s, ok := catalog[a][key]; ok {
		return s
	}
	if s, ok := catalog[English][key]; ok {
		return s
	}
	return key
}

// Tf is T followed by fmt.Sprintf, for messages with format verbs. The
// registered string is the format; a (the args) fills its verbs. When
// the key is missing, T returns the key and Sprintf passes it through
// unchanged (no verbs to fill).
//
// The format string is resolved at runtime, so go vet's printf analyzer
// cannot check verb/argument agreement at Tf/T call sites. The catalog
// symmetry test (internal/messages) guards that EN and RU carry the same
// verb sequence per key, but nothing verifies that sequence against the
// args passed at a call site. So when you edit a format message, keep its
// verbs - especially %w - in step across both languages and the call
// site, or the mismatch only surfaces at runtime as %!v(MISSING).
func Tf(key string, a ...any) string {
	return fmt.Sprintf(T(key), a...)
}

// Tlist returns the message for key split into lines on "\n". It is the
// list form used by the humanize causes/fix groups: register one
// string with embedded newlines and read it back as a slice. An empty
// registered string yields a nil slice rather than [""].
func Tlist(key string) []string {
	s := T(key)
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
