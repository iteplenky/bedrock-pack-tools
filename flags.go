package main

import (
	"fmt"
	"strings"
)

// flagSet is a tiny command-flag parser shared by subcommands.
// Supports bool flags (`-v`, `--verbose`) and string flags taking the
// next arg (`-k PATH`, `--key-out PATH`) or `--key-out=PATH`. Unknown
// args pass through as positionals. Errors only when a string flag is
// present without a value.
type flagSet struct {
	bools map[string]*bool
	strs  map[string]*string
}

func newFlagSet() *flagSet {
	return &flagSet{
		bools: map[string]*bool{},
		strs:  map[string]*string{},
	}
}

// Bool registers all aliases of a bool flag. Presence sets *target=true.
func (f *flagSet) Bool(target *bool, names ...string) {
	for _, n := range names {
		f.bools[n] = target
	}
}

// String registers all aliases of a string flag. The =VALUE form is
// only honoured for long-form aliases (`--name=value`).
func (f *flagSet) String(target *string, names ...string) {
	for _, n := range names {
		f.strs[n] = target
	}
}

// langFlagNames are the aliases of the global language selector. It is
// a true global: handled once in main before dispatch, then stripped
// from os.Args so no subcommand parser ever sees it.
var langFlagNames = []string{"--lang", "-lang"}

// extractGlobalLang pulls the --lang / -lang value out of args and
// returns the value plus a copy of args with the flag (and its value)
// removed. Both the space form (`--lang ru`) and the equals form
// (`--lang=ru`) are recognized; the last occurrence wins. A trailing
// `--lang` with no value is dropped and yields the empty value, so
// lang.Init falls through to the env precedence rather than erroring -
// language selection must never block a command from running.
func extractGlobalLang(args []string) (value string, rest []string) {
	rest = make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if isLangFlag(a) {
			if i+1 < len(args) {
				value = args[i+1]
				i++
			}
			continue
		}
		if name, v, hasEq := strings.Cut(a, "="); hasEq && isLangFlag(name) {
			value = v
			continue
		}
		rest = append(rest, a)
	}
	return value, rest
}

func isLangFlag(a string) bool {
	for _, n := range langFlagNames {
		if a == n {
			return true
		}
	}
	return false
}

// parse walks args, applies any registered flags, and returns the
// remaining positional args. The original args slice is not modified.
func (f *flagSet) parse(args []string) ([]string, error) {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if p, ok := f.bools[a]; ok {
			*p = true
			continue
		}
		if p, ok := f.strs[a]; ok {
			if i+1 >= len(args) {
				return nil, fmt.Errorf("%s requires a value", a)
			}
			*p = args[i+1]
			i++
			continue
		}
		if name, value, hasEq := strings.Cut(a, "="); hasEq {
			if p, ok := f.strs[name]; ok {
				if value == "" {
					return nil, fmt.Errorf("%s= requires a value after the equals sign", name)
				}
				*p = value
				continue
			}
		}
		out = append(out, a)
	}
	return out, nil
}
