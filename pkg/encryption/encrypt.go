package encryption

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"

	aessiv "github.com/jedisct1/go-aes-siv"
)

// EncryptFields accepts anything and returns a new any with values encrypted using key+add and base64 encoded. It works recursively.
func EncryptFields(v any, key, aad []byte) (any, error) {
	siv, err := aessiv.New(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create aes instance: %w", err)
	}
	return encryptAll(v, aad, siv)
}

func encryptAll(v any, aad []byte, siv *aessiv.AESSIV) (any, error) {
	switch val := v.(type) {

	case map[string]any:
		// recursively for nested objects
		out := make(map[string]any)
		for k, nestedVal := range val {
			nestedAD := appendPath(aad, k) // uuid|...|fieldname
			ev, err := encryptAll(nestedVal, nestedAD, siv)
			if err != nil {
				return nil, err
			}
			out[k] = ev
		}
		return out, nil

	case []any:
		// recursively for arrays/slices
		for i, nestedVal := range val {
			nestedAD := appendPath(aad, strconv.Itoa(i)) // uuid|...|fieldname|i
			ev, err := encryptAll(nestedVal, nestedAD, siv)
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

		return encryptValue(b, aad, siv), nil
	}
}

func encryptValue(v []byte, aad []byte, siv *aessiv.AESSIV) string {
	ciphertext := siv.Seal(nil, nil, v, aad)
	return base64.StdEncoding.EncodeToString(ciphertext)
}
