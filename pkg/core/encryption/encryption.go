package encryption

import (
	"encoding/base64"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	aessiv "github.com/jedisct1/go-aes-siv"
)

var siv *aessiv.AESSIV

func SetPassword(password string) error {
	var err error
	key := deriveKey(password, []byte("some aditional data"), aessiv.KeySize256)
	siv, err = aessiv.New(key)
	if err != nil {
		return fmt.Errorf("failed to create encryption instance from password: %w", err)
	}
	return nil
}

func EncryptFields(v any) (any, error) {
	if siv == nil {
		return v, nil // skip if no key/instance
	}

	val := reflect.ValueOf(v)
	// handle pointers by dereferencing to get to the actual value
	for val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return "", nil
		}
		val = val.Elem()
	}
	typ := val.Type()

	out := make(map[string]any)

	// iterate through fields
	for i := 0; i < val.NumField(); i++ {
		fieldValue := val.Field(i)
		fieldType := typ.Field(i)
		fieldName := fieldType.Name
		fieldKind := fieldValue.Kind()

		// respect the `json:"-"`
		if fieldType.Tag.Get("json") == "-" {
			continue
		}

		// respect the `json:"omitempty"`
		if fieldValue.IsZero() && hasOmitzero(fieldType) {
			continue
		}

		// recursively for structs (or pointer to struct)
		if fieldKind == reflect.Struct || (fieldKind == reflect.Pointer && !fieldValue.IsNil() && fieldValue.Elem().Kind() == reflect.Struct) {
			// we only recurse if it's NOT a time.Time
			if _, ok := fieldValue.Interface().(time.Time); !ok {
				nested, err := EncryptFields(fieldValue.Interface())
				if err != nil {
					return nil, err
				}
				out[fieldName] = nested
				continue
			}
		}

		// encrypt basic types
		plaintext, err := encodeValue(fieldValue)
		if err != nil {
			return nil, fmt.Errorf("failed to encode field %s to string: %w", fieldName, err)
		}
		ciphertext := siv.Seal(nil, nil, []byte(plaintext), []byte(fieldName))
		out[fieldName] = base64.StdEncoding.EncodeToString(ciphertext)
	}

	return out, nil
}

// Helper that returns the string representation of field type.
func encodeValue(field reflect.Value) (string, error) {
	// dereference pointer if necessary
	for field.Kind() == reflect.Pointer {
		if field.IsNil() {
			return "", nil
		}
		field = field.Elem()
	}

	switch field.Kind() {
	case reflect.String:
		return field.String(), nil
	case reflect.Int, reflect.Int64, reflect.Int32, reflect.Int16, reflect.Int8:
		return strconv.FormatInt(field.Int(), 10), nil
	case reflect.Struct:
		if t, ok := field.Interface().(time.Time); ok {
			return t.UTC().Format(time.RFC3339), nil
		}
		return "", nil
	default:
		return fmt.Sprintf("%v", field.Interface()), nil
	}
}

// Returns true if `json:"field_name,omitzero"` and false if `json:"field_name"`.
func hasOmitzero(field reflect.StructField) bool {
	tag := field.Tag.Get("json")
	tagParts := strings.Split(tag, ",")
	return slices.Contains(tagParts, "omitzero")
}
