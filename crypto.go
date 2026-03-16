package main

import (
	"crypto/aes"
	"fmt"
)

// decryptAES256CFB8 decrypts data encrypted with AES-256 in CFB8 mode.
// Bedrock uses CFB8 (1-byte feedback) rather than Go's default full-block CFB.
// The IV is the first 16 bytes of the 32-byte key.
func decryptAES256CFB8(data []byte, key []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	iv := key[:aes.BlockSize]

	out := make([]byte, len(data))
	prev := make([]byte, aes.BlockSize)
	copy(prev, iv)
	encBlock := make([]byte, aes.BlockSize)

	for i := range data {
		block.Encrypt(encBlock, prev)
		out[i] = data[i] ^ encBlock[0]
		copy(prev[:aes.BlockSize-1], prev[1:])
		prev[aes.BlockSize-1] = data[i]
	}
	return out, nil
}

// encryptAES256CFB8 encrypts data with AES-256 in CFB8 mode.
// The only difference from decrypt: the feedback register shifts in
// the ciphertext byte (out[i]) instead of the input byte (data[i]).
func encryptAES256CFB8(data []byte, key []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
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
	return out, nil
}
