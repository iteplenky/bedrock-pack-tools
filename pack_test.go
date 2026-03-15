package main

import "testing"

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

func TestSanitizePackName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"My Cool Pack", "My_Cool_Pack"},
		{"pack-name_v1", "pack-name_v1"},
		{"Hive Cosmetics™", "Hive_Cosmetics"},
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
