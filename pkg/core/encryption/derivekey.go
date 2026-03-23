//go:build !js

package encryption

import (
	"golang.org/x/crypto/argon2"
)

// Creates a key of length 'size' based on the provided 'password' plus 'salt'.
func deriveKey(password string, salt []byte, size uint32) []byte {
	return argon2.IDKey(
		[]byte(password),
		salt,
		1,       // iterations
		64*1024, // memory (64 MB)
		4,       // threads
		size,    // 32 bytes for AES-256
	)
}
