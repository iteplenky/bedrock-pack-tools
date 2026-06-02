package main

import (
	"reflect"
	"testing"
)

func TestFlagSet_StringFlag(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantArgs   []string
		wantKeyOut string
		wantErr    bool
	}{
		{
			name:       "no flag",
			args:       []string{"./pack"},
			wantArgs:   []string{"./pack"},
			wantKeyOut: "",
		},
		{
			name:       "long form with space",
			args:       []string{"--key-out", "/tmp/pack.key", "./pack"},
			wantArgs:   []string{"./pack"},
			wantKeyOut: "/tmp/pack.key",
		},
		{
			name:       "short form with space",
			args:       []string{"-k", "/tmp/pack.key", "./pack"},
			wantArgs:   []string{"./pack"},
			wantKeyOut: "/tmp/pack.key",
		},
		{
			name:       "long form with equals",
			args:       []string{"--key-out=/tmp/pack.key", "./pack"},
			wantArgs:   []string{"./pack"},
			wantKeyOut: "/tmp/pack.key",
		},
		{
			name:       "flag mixed with positional",
			args:       []string{"./pack", "--key-out", "/tmp/k", "MASTERKEY"},
			wantArgs:   []string{"./pack", "MASTERKEY"},
			wantKeyOut: "/tmp/k",
		},
		{
			name:    "long form missing value",
			args:    []string{"./pack", "--key-out"},
			wantErr: true,
		},
		{
			name:    "short form missing value",
			args:    []string{"./pack", "-k"},
			wantErr: true,
		},
		{
			name:    "equals form with empty value",
			args:    []string{"--key-out=", "./pack"},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var keyOut string
			fs := newFlagSet()
			fs.String(&keyOut, "--key-out", "-k")
			gotArgs, err := fs.parse(tc.args)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if !reflect.DeepEqual(gotArgs, tc.wantArgs) {
				t.Errorf("args = %#v, want %#v", gotArgs, tc.wantArgs)
			}
			if keyOut != tc.wantKeyOut {
				t.Errorf("keyOut = %q, want %q", keyOut, tc.wantKeyOut)
			}
		})
	}
}

func TestFlagSet_BoolFlag(t *testing.T) {
	cases := []struct {
		name        string
		args        []string
		wantArgs    []string
		wantVerbose bool
	}{
		{
			name:        "no flag",
			args:        []string{"server.example.com"},
			wantArgs:    []string{"server.example.com"},
			wantVerbose: false,
		},
		{
			name:        "long form sets",
			args:        []string{"--verbose", "server.example.com"},
			wantArgs:    []string{"server.example.com"},
			wantVerbose: true,
		},
		{
			name:        "short form sets",
			args:        []string{"-v", "server.example.com"},
			wantArgs:    []string{"server.example.com"},
			wantVerbose: true,
		},
		{
			name:        "flag after positional",
			args:        []string{"server.example.com", "-v"},
			wantArgs:    []string{"server.example.com"},
			wantVerbose: true,
		},
		{
			name:        "flag between positionals",
			args:        []string{"server.example.com", "-v", "./out"},
			wantArgs:    []string{"server.example.com", "./out"},
			wantVerbose: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var verbose bool
			fs := newFlagSet()
			fs.Bool(&verbose, "-v", "--verbose")
			gotArgs, err := fs.parse(tc.args)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if !reflect.DeepEqual(gotArgs, tc.wantArgs) {
				t.Errorf("args = %#v, want %#v", gotArgs, tc.wantArgs)
			}
			if verbose != tc.wantVerbose {
				t.Errorf("verbose = %v, want %v", verbose, tc.wantVerbose)
			}
		})
	}
}

// TestFlagSet_UnknownArgsPassThrough confirms strings that look like
// flags but aren't registered come through as positional args. Lets
// the caller's positional parsing surface invalid input on its own.
func TestFlagSet_UnknownArgsPassThrough(t *testing.T) {
	var keyOut string
	fs := newFlagSet()
	fs.String(&keyOut, "--key-out")

	args, err := fs.parse([]string{"--unknown=value", "-x", "./pack"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := []string{"--unknown=value", "-x", "./pack"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("args = %#v, want %#v", args, want)
	}
	if keyOut != "" {
		t.Errorf("keyOut = %q, want \"\" (no --key-out was given)", keyOut)
	}
}

// TestFlagSet_MixedBoolAndString exercises both registrations on the
// same flagSet - the realistic future-extension case.
func TestFlagSet_MixedBoolAndString(t *testing.T) {
	var verbose bool
	var keyOut string
	fs := newFlagSet()
	fs.Bool(&verbose, "-v", "--verbose")
	fs.String(&keyOut, "--key-out", "-k")

	args, err := fs.parse([]string{"-v", "--key-out", "/tmp/k", "./pack"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !verbose {
		t.Error("verbose should be true")
	}
	if keyOut != "/tmp/k" {
		t.Errorf("keyOut = %q, want /tmp/k", keyOut)
	}
	if !reflect.DeepEqual(args, []string{"./pack"}) {
		t.Errorf("args = %#v, want [./pack]", args)
	}
}
