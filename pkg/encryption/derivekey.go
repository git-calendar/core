package encryption

import (
	aessiv "github.com/jedisct1/go-aes-siv"
	"golang.org/x/crypto/argon2"
)

// DeriveKey derives an AES-SIV key from password and salt using Argon2id.
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

// appendPath builds new AAD (additional authenticated data).
func appendPath(aad []byte, suffix string) []byte {
	newAd := make([]byte, 0, len(aad)+len(suffix))
	newAd = append(newAd, aad...)
	newAd = append(newAd, suffix...)
	return newAd
}
