// Package cfb8 implements AES-CFB8 (1-byte feedback), the cipher mode
// Bedrock uses for resource-pack encryption. Go's stdlib provides only
// full-block CFB, so the mode is implemented manually here.
//
// The key is also the IV source: the IV is the first 16 bytes of the
// 32-byte key. Each call processes one byte at a time, XORing the
// plaintext byte with the high byte of AES(prev_register), then shifting
// the ciphertext byte into the register. Encrypt and Decrypt are
// symmetric except for which byte enters the feedback register: during
// encryption it's the freshly-produced ciphertext (out[i]), during
// decryption it's the input ciphertext (data[i]).
package cfb8

import (
	"crypto/aes"
	"fmt"
)

// Encrypt encrypts data with AES-256-CFB8. key must be exactly 32 bytes.
func Encrypt(data, key []byte) ([]byte, error) {
	return process(data, key, true)
}

// Decrypt reverses Encrypt. key must be exactly 32 bytes.
func Decrypt(data, key []byte) ([]byte, error) {
	return process(data, key, false)
}

// process runs AES-256-CFB8 over data. Encrypt and Decrypt are symmetric
// except for the byte fed back into the register each step: the output
// byte when encrypting, the input byte when decrypting.
func process(data, key []byte, encrypting bool) ([]byte, error) {
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
		if encrypting {
			prev[aes.BlockSize-1] = out[i]
		} else {
			prev[aes.BlockSize-1] = data[i]
		}
	}
	return out, nil
}
