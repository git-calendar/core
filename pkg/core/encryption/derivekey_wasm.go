//go:build js && wasm

package encryption

import (
	"golang.org/x/crypto/argon2"
)

// WASM version with smaller memory and CPUs.
//
// Creates a key of length 'size' based on the provided 'password' plus 'salt'.
func deriveKey(password string, salt []byte, size uint32) []byte {
	return argon2.IDKey(
		[]byte(password),
		salt,
		1,       // iterations
		16*1024, // memory (16 MB)
		1,       // 1 thread in browser
		size,
	)
}
