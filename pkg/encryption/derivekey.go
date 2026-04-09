package encryption

import (
	aessiv "github.com/jedisct1/go-aes-siv"
	"golang.org/x/crypto/argon2"
)

// Creates a key of length 'size' based on the provided 'password' plus 'salt'.
func DeriveKey(password string, salt []byte) []byte {
	return argon2.IDKey(
		[]byte(password),
		salt,
		1,       // iterations
		64*1024, // memory (64 MB)
		4,       // threads (no real benefit in the browser as WASM is single-core)
		aessiv.KeySize256,
	)
}
