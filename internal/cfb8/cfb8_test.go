package cfb8

import "testing"

const testKey = "ABCDEFGHIJKLMNOPQRSTUVWXYZ123456"

func mustEncrypt(t testing.TB, data, key []byte) []byte {
	t.Helper()
	ct, err := Encrypt(data, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	return ct
}

func TestRoundTrip(t *testing.T) {
	key := []byte(testKey)
	plaintext := []byte("Hello, Bedrock resource packs!")

	ciphertext := mustEncrypt(t, plaintext, key)
	decrypted, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Errorf("got %q, want %q", decrypted, plaintext)
	}
}

func TestEmptyData(t *testing.T) {
	decrypted, err := Decrypt([]byte{}, []byte(testKey))
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if len(decrypted) != 0 {
		t.Errorf("expected empty, got %d bytes", len(decrypted))
	}
}

func TestSingleByte(t *testing.T) {
	key := []byte(testKey)
	plaintext := []byte{0x42}

	ciphertext := mustEncrypt(t, plaintext, key)
	decrypted, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if decrypted[0] != 0x42 {
		t.Errorf("got 0x%02x, want 0x42", decrypted[0])
	}
}

func TestLargeData(t *testing.T) {
	key := []byte(testKey)
	plaintext := make([]byte, 64*1024)
	for i := range plaintext {
		plaintext[i] = byte(i % 256)
	}

	ciphertext := mustEncrypt(t, plaintext, key)
	decrypted, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	for i := range plaintext {
		if decrypted[i] != plaintext[i] {
			t.Fatalf("mismatch at byte %d: got 0x%02x, want 0x%02x", i, decrypted[i], plaintext[i])
		}
	}
}

func TestInvalidKeyLength(t *testing.T) {
	tests := []struct {
		name string
		key  []byte
	}{
		{"empty", []byte{}},
		{"16 bytes", []byte("0123456789abcdef")},
		{"31 bytes", []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ12345")},
		{"33 bytes", []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Decrypt([]byte("test"), tc.key); err == nil {
				t.Error("Decrypt accepted invalid key length")
			}
			if _, err := Encrypt([]byte("test"), tc.key); err == nil {
				t.Error("Encrypt accepted invalid key length")
			}
		})
	}
}

func TestDifferentKeysProduceDifferentOutput(t *testing.T) {
	key1 := []byte(testKey)
	key2 := []byte("ZYXWVUTSRQPONMLKJIHGFEDCBA654321")
	plaintext := []byte("same input data for both keys!!")

	ct1 := mustEncrypt(t, plaintext, key1)
	ct2 := mustEncrypt(t, plaintext, key2)

	if string(ct1) == string(ct2) {
		t.Error("different keys should produce different ciphertext")
	}
}

func BenchmarkDecrypt(b *testing.B) {
	key := []byte(testKey)
	data := make([]byte, 256*1024)
	ciphertext, _ := Encrypt(data, key)

	b.SetBytes(int64(len(data)))
	for b.Loop() {
		_, _ = Decrypt(ciphertext, key)
	}
}
