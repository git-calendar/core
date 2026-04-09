package encryption

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	aessiv "github.com/jedisct1/go-aes-siv"
)

func DecryptFields(v any, key, ad []byte) (map[string]any, error) {
	siv, err := aessiv.New(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create aes instance: %w", err)
	}
	dec, err := decryptAll(v, ad, siv)
	if err != nil {
		return nil, err
	}
	decMap, ok := dec.(map[string]any)
	if !ok {
		return nil, errors.New("HUH")
	}
	return decMap, nil
}

func decryptAll(v any, ad []byte, siv *aessiv.AESSIV) (any, error) {
	switch val := v.(type) {

	case map[string]any:
		// recursively for nested objects
		out := make(map[string]any)
		for k, nestedVal := range val {
			nestedAD := appendPath(ad, k) // uuid|...|fieldname
			dv, err := decryptAll(nestedVal, nestedAD, siv)
			if err != nil {
				return nil, err
			}
			out[k] = dv
		}
		return out, nil

	case []any:
		// recursively for arrays/slices
		for i, nestedVal := range val {
			nestedAD := appendPath(ad, strconv.Itoa(i)) // uuid|...|fieldname|i
			dv, err := decryptAll(nestedVal, nestedAD, siv)
			if err != nil {
				return nil, err
			}
			val[i] = dv
		}
		return val, nil

	case string:
		// leaf value
		pt, err := decryptValue(val, ad, siv)
		if err != nil {
			return nil, err
		}

		// unmarshal JSON to get the original type
		var orig any
		if err := json.Unmarshal(pt, &orig); err != nil {
			return nil, fmt.Errorf("json unmarshal failed: %w", err)
		}

		return orig, nil

	default:
		return nil, fmt.Errorf("unexpected type: %T", val)
	}
}

func decryptValue(ciphertext string, ad []byte, siv *aessiv.AESSIV) ([]byte, error) {
	ct, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 value: %w", err)
	}
	return siv.Open(nil, nil, ct, ad)
}
