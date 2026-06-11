package lang

import (
	"reflect"
	"testing"
)

// setActive swaps the active language for a test and restores it after.
func setActive(t *testing.T, l Lang) {
	t.Helper()
	prev := Lang(active.Load())
	active.Store(int32(l))
	t.Cleanup(func() { active.Store(int32(prev)) })
}

// clearEnv unsets every locale source so an inherited LANG on the test
// host can't leak into precedence tests. Values are restored after.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"BPT_LANG", "LC_ALL", "LC_MESSAGES", "LANG"} {
		t.Setenv(name, "") // t.Setenv records the original and restores it
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		in   string
		want Lang
		ok   bool
	}{
		{"ru", Russian, true},
		{"RU", Russian, true},
		{"ru_RU.UTF-8", Russian, true},
		{"ru-RU", Russian, true},
		{"russian", Russian, true},
		{" ru ", Russian, true},
		{"en", English, true},
		{"en_US.UTF-8", English, true},
		{"en-GB", English, true},
		{"english", English, true},
		{"C", English, true},
		{"POSIX", English, true},
		{"", English, false},
		{"   ", English, false},
		{"fr", English, false},
		{"de_DE.UTF-8", English, false},
		{"xx", English, false},
	}
	for _, tt := range tests {
		got, ok := parse(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("parse(%q) = (%v, %v), want (%v, %v)", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestInitPrecedence(t *testing.T) {
	tests := []struct {
		name      string
		flagValue string
		persisted string
		env       map[string]string
		want      Lang
	}{
		{
			name:      "flag beats every env",
			flagValue: "ru",
			env:       map[string]string{"BPT_LANG": "en", "LC_ALL": "en", "LANG": "en"},
			want:      Russian,
		},
		{
			name:      "persisted beats locale",
			persisted: "ru",
			env:       map[string]string{"LANG": "en"},
			want:      Russian,
		},
		{
			name:      "flag beats persisted",
			flagValue: "en",
			persisted: "ru",
			want:      English,
		},
		{
			name:      "BPT_LANG beats persisted",
			persisted: "ru",
			env:       map[string]string{"BPT_LANG": "en"},
			want:      English,
		},
		{
			name:      "explicit en flag stops precedence walk",
			flagValue: "en",
			env:       map[string]string{"LANG": "ru_RU.UTF-8"},
			want:      English,
		},
		{
			name: "BPT_LANG beats LC_ALL/LANG",
			env:  map[string]string{"BPT_LANG": "ru", "LC_ALL": "en", "LANG": "en"},
			want: Russian,
		},
		{
			name: "LC_ALL beats LC_MESSAGES and LANG",
			env:  map[string]string{"LC_ALL": "ru_RU.UTF-8", "LC_MESSAGES": "en", "LANG": "en"},
			want: Russian,
		},
		{
			name: "LC_MESSAGES beats LANG",
			env:  map[string]string{"LC_MESSAGES": "ru", "LANG": "en"},
			want: Russian,
		},
		{
			name: "LANG used as last resort",
			env:  map[string]string{"LANG": "ru-RU"},
			want: Russian,
		},
		{
			name: "unparseable values fall through to English default",
			env:  map[string]string{"LANG": "fr_FR.UTF-8"},
			want: English,
		},
		{
			name: "unknown flag falls through to env",
			// flag "fr" is unparseable -> falls through; LANG ru wins.
			flagValue: "fr",
			env:       map[string]string{"LANG": "ru"},
			want:      Russian,
		},
		{
			name: "nothing set defaults to English",
			env:  map[string]string{},
			want: English,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setActive(t, English)
			clearEnv(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			Init(tt.flagValue, tt.persisted)
			if got := Current(); got != tt.want {
				t.Errorf("Current() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInitEmptyEnvVarIsSkipped(t *testing.T) {
	setActive(t, English)
	clearEnv(t)
	// An exported-but-empty LC_ALL must not shadow a populated LANG.
	t.Setenv("LC_ALL", "")
	t.Setenv("LANG", "ru")
	Init("", "")
	if got := Current(); got != Russian {
		t.Errorf("Current() = %v, want Russian (empty LC_ALL should not win)", got)
	}
}

func TestTFallback(t *testing.T) {
	Register(English, map[string]string{
		"test.t.both":   "both-en",
		"test.t.enonly": "only-en",
	})
	Register(Russian, map[string]string{
		"test.t.both": "both-ru",
	})

	t.Run("russian active prefers russian", func(t *testing.T) {
		setActive(t, Russian)
		if got := T("test.t.both"); got != "both-ru" {
			t.Errorf("T = %q, want both-ru", got)
		}
	})
	t.Run("russian active falls back to english", func(t *testing.T) {
		setActive(t, Russian)
		if got := T("test.t.enonly"); got != "only-en" {
			t.Errorf("T = %q, want only-en", got)
		}
	})
	t.Run("missing key returns the key itself", func(t *testing.T) {
		setActive(t, Russian)
		if got := T("test.t.missing"); got != "test.t.missing" {
			t.Errorf("T = %q, want the key itself", got)
		}
	})
	t.Run("english active uses english", func(t *testing.T) {
		setActive(t, English)
		if got := T("test.t.both"); got != "both-en" {
			t.Errorf("T = %q, want both-en", got)
		}
	})
}

func TestTf(t *testing.T) {
	Register(English, map[string]string{"test.tf.greet": "hello %s, %d packs"})
	setActive(t, English)
	if got := Tf("test.tf.greet", "world", 3); got != "hello world, 3 packs" {
		t.Errorf("Tf = %q", got)
	}
	// Missing key: T returns the key, Sprintf passes it through with no
	// verbs to fill (the args are dropped, no %!(EXTRA ...) since there
	// are no verbs in the key string).
	if got := Tf("test.tf.missing"); got != "test.tf.missing" {
		t.Errorf("Tf(missing) = %q, want the key", got)
	}
}

func TestTlist(t *testing.T) {
	Register(English, map[string]string{
		"test.list.multi": "one\ntwo\nthree",
		"test.list.empty": "",
	})
	setActive(t, English)
	if got, want := Tlist("test.list.multi"), []string{"one", "two", "three"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Tlist = %v, want %v", got, want)
	}
	if got := Tlist("test.list.empty"); got != nil {
		t.Errorf("Tlist(empty) = %v, want nil", got)
	}
	if got, want := Tlist("test.list.missingkey"), []string{"test.list.missingkey"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Tlist(missing) = %v, want the key as one line", got)
	}
}

func TestLangString(t *testing.T) {
	if English.String() != "en" {
		t.Errorf("English.String() = %q, want en", English.String())
	}
	if Russian.String() != "ru" {
		t.Errorf("Russian.String() = %q, want ru", Russian.String())
	}
}
