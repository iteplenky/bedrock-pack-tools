package main

import "testing"

func TestSanitize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"play.example.net:19132", "play_example_net_19132"},
		{"simple", "simple"},
		{"ABC123", "ABC123"},
		{"a-b_c.d", "a_b_c_d"},
		{"", ""},
		{"hello world!", "hello_world_"},
		{"кириллица", "_________"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := sanitize(tc.input)
			if got != tc.want {
				t.Errorf("sanitize(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
