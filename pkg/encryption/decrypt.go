package encryption

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	aessiv "github.com/jedisct1/go-aes-siv"
)

func DecryptFields(v any, key, ad []byte) (map[string]any, error) {
	siv, err := aessiv.New(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create aes instance: %w", err)
	}
	dec, err := decryptAll(v, "", siv)
	if err != nil {
		return nil, err
	}
	decMap, ok := dec.(map[string]any)
	if !ok {
		return nil, errors.New("HUH")
	}
	return decMap, nil
}

func decryptAll(v any, path string, siv *aessiv.AESSIV) (any, error) {
	switch val := v.(type) {

	case map[string]any:
		out := make(map[string]any)
		for k, nestedVal := range val {
			newPath := k
			if path != "" {
				newPath = path + "." + k
			}
			dv, err := decryptAll(nestedVal, newPath, siv)
			if err != nil {
				return nil, err
			}
			out[k] = dv
		}
		return out, nil

	case []any:
		for i, nestedVal := range val {
			newPath := fmt.Sprintf("%s[%d]", path, i)
			dv, err := decryptAll(nestedVal, newPath, siv)
			if err != nil {
				return nil, err
			}
			val[i] = dv
		}
		return val, nil

	case string:
		pt, err := decryptValue(val, []byte(path), siv)
		if err != nil {
			return nil, err
		}

		// unmarshal JSON to get the original type
		var orig any
		if err := json.Unmarshal(pt, &orig); err != nil {
			return nil, fmt.Errorf("json unmarshal failed at %s: %w", path, err)
		}

		return orig, nil

	default:
		return nil, fmt.Errorf("unexpected type at %s: %T", path, val)
	}
}

func decryptValue(ciphertext string, ad []byte, siv *aessiv.AESSIV) ([]byte, error) {
	ct, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 value: %w", err)
	}
	return siv.Open(nil, nil, ct, ad)
}
