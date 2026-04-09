package encryption

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	aessiv "github.com/jedisct1/go-aes-siv"
)

func EncryptFields(v any, key, ad []byte) (any, error) {
	siv, err := aessiv.New(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create aes instance: %w", err)
	}
	return encryptAll(v, "", siv)
}

func encryptAll(v any, path string, siv *aessiv.AESSIV) (any, error) {
	switch val := v.(type) {

	case map[string]any:
		// recursively for nested objects
		out := make(map[string]any)
		for k, nestedVal := range val {
			newPath := k
			if path != "" {
				newPath = path + "." + k
			}
			ev, err := encryptAll(nestedVal, newPath, siv)
			if err != nil {
				return nil, err
			}
			out[k] = ev
		}
		return out, nil

	case []any:
		// recursively for arrays/slices
		for i, nestedVal := range val {
			newPath := fmt.Sprintf("%s[%d]", path, i)
			ev, err := encryptAll(nestedVal, newPath, siv)
			if err != nil {
				return nil, err
			}
			val[i] = ev
		}
		return val, nil

	default:
		// leaf value
		b, err := json.Marshal(val)
		if err != nil {
			return nil, err
		}

		return encryptValue(b, []byte(path), siv), nil
	}
}

func encryptValue(v []byte, ad []byte, siv *aessiv.AESSIV) string {
	ciphertext := siv.Seal(nil, nil, v, ad)
	return base64.StdEncoding.EncodeToString(ciphertext)
}
