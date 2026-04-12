package encryption

import (
	"reflect"
	"strings"
	"testing"
)

func TestEncryptDecryptFields(t *testing.T) {
	tests := []struct {
		name  string
		input any
	}{
		{
			name: "simple",
			input: map[string]any{
				"name": "alice",
				"age":  float64(30),
			},
		},
		{
			name: "nested",
			input: map[string]any{
				"nested": map[string]any{
					"city": "prague",
				},
				"arr": []any{"one", float64(2)},
			},
		},
		{
			name: "map pointer",
			input: &map[string]any{
				"nested": map[string]any{
					"city": "prague",
				},
			},
		},
		{
			name: "nested map pointer",
			input: &map[string]any{
				"nested": &map[string]any{
					"city": "brno",
				},
			},
		},
		{
			name:  "empty map",
			input: map[string]any{},
		},
		{
			name: "deep nesting",
			input: map[string]any{
				"a": map[string]any{
					"b": map[string]any{
						"c": []any{
							map[string]any{"d": 1},
						},
					},
				},
			},
		},
	}

	key := DeriveKey("somepassword", []byte("salt"))
	aad := []byte("user|123")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encrypted, err := EncryptFields(tt.input, key, aad)
			if err != nil {
				t.Fatalf("EncryptFields() error = %v", err)
			}

			decrypted, err := DecryptFields(encrypted, key, aad)
			if err != nil {
				t.Fatalf("DecryptFields() error = %v", err)
			}

			want := deepDereference(tt.input) // so that we compare with flat (no pointers)
			if !reflect.DeepEqual(decrypted, want) {
				t.Fatalf("mismatch\n got: %#v\nwant: %#v", decrypted, want)
			}
		})
	}
}

func TestEncryptFieldsErrors(t *testing.T) {
	tests := []struct {
		name string
		key  []byte
	}{
		{
			name: "short key",
			key:  []byte("short"),
		},
		{
			name: "long key",
			key:  []byte("tooooooooo long key that exceeds 32 bytes"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := EncryptFields(map[string]any{"a": "b"}, tt.key, nil)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), "failed to create aes instance") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestDecryptFieldsErrors(t *testing.T) {
	key := DeriveKey("somepassword", []byte("salt"))

	tests := []struct {
		name  string
		input any
		key   []byte
	}{
		{
			name:  "invalid base64",
			input: "not-base64",
			key:   key,
		},
		{
			name:  "unexpected type",
			input: 123,
			key:   key,
		},
		{
			name: "wrong key",
			input: func() any {
				enc, _ := EncryptFields(map[string]any{"a": "b"}, key, nil)
				return enc
			}(),
			key: DeriveKey("anotherpassword", []byte("salt2")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecryptFields(tt.input, tt.key, nil)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestAppendPath(t *testing.T) {
	tests := []struct {
		name   string
		aad    []byte
		suffix string
		want   []byte
	}{
		{
			name:   "basic",
			aad:    []byte("user123"),
			suffix: "name",
			want:   []byte("user123name"),
		},
		{
			name:   "empty aad",
			aad:    []byte(""),
			suffix: "x",
			want:   []byte("x"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := appendPath(tt.aad, tt.suffix)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// A helper that dereferences pointers recursively.
func deepDereference(v any) any {
	switch val := v.(type) {

	case *map[string]any:
		return deepDereference(*val)

	case map[string]any:
		out := make(map[string]any)
		for k, v2 := range val {
			out[k] = deepDereference(v2)
		}
		return out

	case []any:
		for i, v2 := range val {
			val[i] = deepDereference(v2)
		}
		return val

	default:
		// handle pointer to scalar if needed
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Pointer && !rv.IsNil() {
			return deepDereference(rv.Elem().Interface())
		}

		return v
	}
}
