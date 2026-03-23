package encryption

import (
	"encoding/base64"
	"errors"
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

// Decrypts everything from raw map to v. v has to be a pointer to struct.
func DecryptFields(v any, raw map[string]any) error {
	if siv == nil {
		return nil // skip if no key/instance
	}

	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Pointer || val.Elem().Kind() != reflect.Struct {
		return errors.New("v must be a pointer to a struct")
	}

	val = val.Elem()
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		fieldValue := val.Field(i)
		fieldType := typ.Field(i)
		fieldName := fieldType.Name

		// skip if field is unexported or marked to be ignored
		if !fieldValue.CanSet() || fieldType.Tag.Get("json") == "-" {
			continue
		}

		// get the value from the map using the struct field name
		data, ok := raw[getJsonFieldName(fieldType)]
		if !ok || data == nil {
			continue
		}

		// recursive structs
		if fieldValue.Kind() == reflect.Struct || (fieldValue.Kind() == reflect.Pointer && fieldType.Type.Elem().Kind() == reflect.Struct) {
			// don't recurse time.Time
			if _, isTime := fieldValue.Interface().(time.Time); !isTime {
				nestedMap, isMap := data.(map[string]any)
				if isMap {
					// initialize pointer if nil
					if fieldValue.Kind() == reflect.Pointer && fieldValue.IsNil() {
						fieldValue.Set(reflect.New(fieldValue.Type().Elem()))
					}
					if err := DecryptFields(fieldValue.Addr().Interface(), nestedMap); err != nil {
						return err
					}
					continue
				}
			}
		}

		// decrypt encrypted fields
		cipherStr, ok := data.(string)
		if !ok {
			continue
		}

		ciphertext, err := base64.StdEncoding.DecodeString(cipherStr)
		if err != nil {
			return fmt.Errorf("failed to decode base64 string of field %s: %w", fieldName, err)
		}
		plaintext, err := siv.Open(nil, nil, ciphertext, []byte(fieldName))
		if err != nil {
			return fmt.Errorf("failed to decrypt field %s: %w", fieldName, err)
		}

		// convert decrypted string back to the field's actual type
		if err := decodeValue(fieldValue, string(plaintext)); err != nil {
			return fmt.Errorf("failed to parse field %s: %w", fieldName, err)
		}
	}

	return nil
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

// Helper that converts string representation of a value back into its field type.
func decodeValue(field reflect.Value, val string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(val)
	case reflect.Int, reflect.Int64, reflect.Int32, reflect.Int16, reflect.Int8:
		i, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return err
		}
		field.SetInt(i)
	case reflect.Struct:
		if _, ok := field.Interface().(time.Time); ok {
			t, err := time.Parse(time.RFC3339, val)
			if err != nil {
				return err
			}
			field.Set(reflect.ValueOf(t))
		}
	}
	return nil
}

// Returns true if `json:"field_name,omitzero"` and false if `json:"field_name"`.
func hasOmitzero(field reflect.StructField) bool {
	tag := field.Tag.Get("json")
	tagParts := strings.Split(tag, ",")
	return slices.Contains(tagParts, "omitzero")
}

// Returns the field_name from `json:"field_name"`.
func getJsonFieldName(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	tagParts := strings.Split(tag, ",")
	return tagParts[0] // always len > 0 -> safe
}
