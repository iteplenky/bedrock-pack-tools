package main

import (
	"testing"

	"github.com/iteplenky/bedrock-pack-tools/v3/internal/cfb8"
)

// testMasterKey is shared by decrypt / encrypt tests. The cipher's
// own round-trip / boundary tests live in the cfb8 package.
const testMasterKey = "ABCDEFGHIJKLMNOPQRSTUVWXYZ123456"

func mustEncrypt(t testing.TB, data, key []byte) []byte {
	t.Helper()
	ct, err := cfb8.Encrypt(data, key)
	if err != nil {
		t.Fatalf("cfb8.Encrypt: %v", err)
	}
	return ct
}
