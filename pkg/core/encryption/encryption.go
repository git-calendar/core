package encryption

import (
	"encoding/base64"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	aessiv "github.com/jedisct1/go-aes-siv"
)

const tagName string = "encrypt"

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

func EncryptFields(v any, ad []byte) (any, error) {
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

	// iterate through fields and encrypt each one
	for i := 0; i < val.NumField(); i++ {
		fieldValue := val.Field(i)
		fieldType := typ.Field(i)
		fieldName := fieldType.Name
		jsonFieldName := getFieldName(fieldType)
		fieldKind := fieldValue.Kind()

		if jsonFieldName == "-" || fieldValue.IsZero() {
			continue // skip if zero or "-" as name
		}

		// --- recursively for structs (or pointer to struct) ---
		if fieldKind == reflect.Struct || (fieldKind == reflect.Pointer && !fieldValue.IsNil() && fieldValue.Elem().Kind() == reflect.Struct) {
			// we only recurse if it's NOT a time.Time
			if _, ok := fieldValue.Interface().(time.Time); !ok {
				nested, err := EncryptFields(fieldValue.Interface(), ad)
				if err != nil {
					return nil, err
				}
				out[jsonFieldName] = nested
				continue
			}
		}

		// --- encrypt basic types ---
		plaintext := encodeValue(fieldValue)
		additionalData := append([]byte(fieldName), ad...)
		ciphertext := siv.Seal(nil, nil, []byte(plaintext), additionalData)
		out[jsonFieldName] = base64.StdEncoding.EncodeToString(ciphertext)
	}

	return out, nil
}

// Decrypts everything from raw map to v. v has to be a pointer to struct.
func DecryptFields(v any, raw map[string]any, ad []byte) error {
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
		jsonFieldName := getFieldName(fieldType)
		fieldName := fieldType.Name

		if !fieldValue.CanSet() || jsonFieldName == "-" {
			continue // skip if field is unexported or marked to be ignored
		}

		// get the value from the map using the struct field name
		data, ok := raw[jsonFieldName]
		if !ok || data == nil {
			continue // no data => nothing to decrypt
		}

		// --- recursively for structs (or pointer to struct) ---
		if fieldValue.Kind() == reflect.Struct || (fieldValue.Kind() == reflect.Pointer && fieldType.Type.Elem().Kind() == reflect.Struct) {
			// do not recurse time.Time
			if _, isTime := fieldValue.Interface().(time.Time); !isTime {
				nestedMap, isMap := data.(map[string]any)
				if isMap {
					// initialize pointer if nil
					if fieldValue.Kind() == reflect.Pointer && fieldValue.IsNil() {
						fieldValue.Set(reflect.New(fieldValue.Type().Elem()))
					}
					if err := DecryptFields(fieldValue.Addr().Interface(), nestedMap, ad); err != nil {
						return err
					}
					continue
				}
			}
		}

		// --- decrypt basic types ---
		cipherStr, ok := data.(string)
		if !ok {
			continue // the value (still encrypted) is not string => skip?
		}

		ciphertext, err := base64.StdEncoding.DecodeString(cipherStr)
		if err != nil {
			return fmt.Errorf("failed to decode base64 string of field %s: %w", fieldName, err)
		}

		additionalData := append([]byte(fieldName), ad...)
		plaintext, err := siv.Open(nil, nil, ciphertext, additionalData)
		if err != nil {
			return fmt.Errorf("failed to decrypt field %s: %w", fieldName, err)
		}

		// convert decrypted string back to the field type
		if err := decodeValue(fieldValue, string(plaintext)); err != nil {
			return fmt.Errorf("failed to parse field %s: %w", fieldName, err)
		}
	}

	return nil
}

// encodeValue returns a simple string representation of the value.
func encodeValue(v reflect.Value) string {
	// dereference pointers
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.String:
		return v.String()

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10)

	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'f', -1, 64)

	case reflect.Bool:
		return strconv.FormatBool(v.Bool())

	case reflect.Struct:
		if t, ok := v.Interface().(time.Time); ok {
			return t.UTC().Format(time.RFC3339)
		}
		fallthrough // for other structs, fall through to fmt

	default:
		return fmt.Sprintf("%v", v.Interface())
	}
}

// decodeValue sets the value from its string representation.
func decodeValue(field reflect.Value, val string) error {
	// handle pointers
	if field.Kind() == reflect.Pointer {
		if val == "" {
			field.Set(reflect.Zero(field.Type()))
			return nil
		}
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		field = field.Elem()
	}

	switch field.Kind() {
	case reflect.String:
		field.SetString(val)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return err
		}
		field.SetInt(i)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		u, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			return err
		}
		field.SetUint(u)

	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return err
		}
		field.SetFloat(f)

	case reflect.Bool:
		b, err := strconv.ParseBool(val)
		if err != nil {
			return err
		}
		field.SetBool(b)

	case reflect.Struct:
		if field.Type() == reflect.TypeOf(time.Time{}) {
			t, err := time.Parse(time.RFC3339, val)
			if err != nil {
				return err
			}
			field.Set(reflect.ValueOf(t))
			return nil
		}
		fallthrough // for other structs fall through to unsupported error

	default:
		return fmt.Errorf("unsupported kind/type %s", field.Kind())
	}
	return nil
}

// Returns the "field_name" from:
//
//	FieldName `encrypt:"field_name"`
//
// Returns "FieldName" if not present.
func getFieldName(field reflect.StructField) string {
	tag := field.Tag.Get(tagName)
	tagParts := strings.Split(tag, ",") // always len > 0
	if len(tagParts[0]) == 0 {          // if empty string
		return field.Name // fallback to struct field name instead of the one from tag
	}
	return tagParts[0]
}
