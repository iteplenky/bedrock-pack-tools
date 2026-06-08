package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeServerAddr(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"my-server.net:19132", "my_server_net_19132"},
		{"simple", "simple"},
		{"ABC123", "ABC123"},
		{"a-b_c.d", "a_b_c_d"},
		{"", ""},
		{"hello world!", "hello_world_"},
		{"кириллица", "_________"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := sanitizeServerAddr(tc.input)
			if got != tc.want {
				t.Errorf("sanitizeServerAddr(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestParseResourcePacksRecoversFromPanic confirms the panic-recover
// in parseResourcePacks turns a malformed payload into
// errPackBadProtocol instead of crashing the process.
func TestParseResourcePacksRecoversFromPanic(t *testing.T) {
	garbage := [][]byte{
		nil,
		{},
		{0x00},
		{0xFF, 0xFF, 0xFF, 0xFF},
		make([]byte, 1024),
	}
	for i, payload := range garbage {
		_, err := parseResourcePacks(payload)
		if err == nil {
			continue
		}
		if !errors.Is(err, errPackBadProtocol) {
			t.Errorf("payload[%d]: expected errPackBadProtocol, got %v", i, err)
		}
	}
}

// TestReadPackUUIDSentinels confirms readPackUUID wraps each failure
// mode in the right sentinel so humanize can classify the chain.
func TestReadPackUUIDSentinels(t *testing.T) {
	t.Run("missing manifest", func(t *testing.T) {
		_, err := readPackUUID(t.TempDir())
		if !errors.Is(err, errPackBadManifest) {
			t.Errorf("expected errPackBadManifest, got %v", err)
		}
	})
	t.Run("malformed json", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, manifestJSON), []byte("not json"), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := readPackUUID(dir)
		if !errors.Is(err, errPackBadManifest) {
			t.Errorf("expected errPackBadManifest, got %v", err)
		}
	})
	t.Run("missing header.uuid", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, manifestJSON), []byte(`{"header":{}}`), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := readPackUUID(dir)
		if !errors.Is(err, errPackNoManifest) {
			t.Errorf("expected errPackNoManifest, got %v", err)
		}
	})
}

func TestSanitizePackName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"My Cool Pack", "My_Cool_Pack"},
		{"pack-name_v1", "pack-name_v1"},
		{"Generic Pack™", "Generic_Pack"},
		{"hello.world", "helloworld"},
		{"", ""},
		{"a b  c", "a_b__c"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := sanitizePackName(tc.input)
			if got != tc.want {
				t.Errorf("sanitizePackName(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
