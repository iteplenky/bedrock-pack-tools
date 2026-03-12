package main

import (
	"crypto/aes"
	"testing"
)

func encryptAES256CFB8(data, key []byte) []byte {
	block, _ := aes.NewCipher(key)
	iv := key[:aes.BlockSize]

	out := make([]byte, len(data))
	prev := make([]byte, aes.BlockSize)
	copy(prev, iv)
	encBlock := make([]byte, aes.BlockSize)

	for i := range data {
		block.Encrypt(encBlock, prev)
		out[i] = data[i] ^ encBlock[0]
		copy(prev[:aes.BlockSize-1], prev[1:])
		prev[aes.BlockSize-1] = out[i]
	}
	return out
}

func TestDecryptAES256CFB8_RoundTrip(t *testing.T) {
	key := []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ123456")
	plaintext := []byte("Hello, Bedrock resource packs!")

	ciphertext := encryptAES256CFB8(plaintext, key)
	decrypted, err := decryptAES256CFB8(ciphertext, key)
	if err != nil {
		t.Fatalf("decrypt error: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Errorf("got %q, want %q", decrypted, plaintext)
	}
}

func TestDecryptAES256CFB8_EmptyData(t *testing.T) {
	key := []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ123456")
	decrypted, err := decryptAES256CFB8([]byte{}, key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(decrypted) != 0 {
		t.Errorf("expected empty, got %d bytes", len(decrypted))
	}
}

func TestDecryptAES256CFB8_SingleByte(t *testing.T) {
	key := []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ123456")
	plaintext := []byte{0x42}

	ciphertext := encryptAES256CFB8(plaintext, key)
	decrypted, err := decryptAES256CFB8(ciphertext, key)
	if err != nil {
		t.Fatalf("decrypt error: %v", err)
	}
	if decrypted[0] != 0x42 {
		t.Errorf("got 0x%02x, want 0x42", decrypted[0])
	}
}

func TestDecryptAES256CFB8_LargeData(t *testing.T) {
	key := []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ123456")
	plaintext := make([]byte, 64*1024)
	for i := range plaintext {
		plaintext[i] = byte(i % 256)
	}

	ciphertext := encryptAES256CFB8(plaintext, key)
	decrypted, err := decryptAES256CFB8(ciphertext, key)
	if err != nil {
		t.Fatalf("decrypt error: %v", err)
	}
	for i := range plaintext {
		if decrypted[i] != plaintext[i] {
			t.Fatalf("mismatch at byte %d: got 0x%02x, want 0x%02x", i, decrypted[i], plaintext[i])
		}
	}
}

func TestDecryptAES256CFB8_InvalidKeyLength(t *testing.T) {
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
			_, err := decryptAES256CFB8([]byte("test"), tc.key)
			if err == nil {
				t.Error("expected error for invalid key length")
			}
		})
	}
}

func TestDecryptAES256CFB8_DifferentKeysProduceDifferentOutput(t *testing.T) {
	key1 := []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ123456")
	key2 := []byte("ZYXWVUTSRQPONMLKJIHGFEDCBA654321")
	plaintext := []byte("same input data for both keys!!")

	ct1 := encryptAES256CFB8(plaintext, key1)
	ct2 := encryptAES256CFB8(plaintext, key2)

	if string(ct1) == string(ct2) {
		t.Error("different keys should produce different ciphertext")
	}
}

func BenchmarkDecryptAES256CFB8(b *testing.B) {
	key := []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ123456")
	data := make([]byte, 256*1024)
	ciphertext := encryptAES256CFB8(data, key)

	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for range b.N {
		decryptAES256CFB8(ciphertext, key)
	}
}
