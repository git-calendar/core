package encryption

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	aessiv "github.com/jedisct1/go-aes-siv"
)

const tagName string = "encrypt"

func EncryptFields(v any, key, ad []byte) (any, error) {
	siv, err := aessiv.New(key)
	if err != nil {
		return v, fmt.Errorf("failed to create encryption instance from password: %w", err)
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
				nested, err := EncryptFields(fieldValue.Interface(), key, ad)
				if err != nil {
					return nil, err
				}
				out[jsonFieldName] = nested
				continue
			}
		}

		if fieldKind == reflect.Array || fieldKind == reflect.Slice {
			var final []string
			for i := 0; i < fieldValue.Len(); i++ {
				elValue := fieldValue.Index(i)

				additionalData := append([]byte(fieldName), ad...)
				final = append(final, encryptToString(elValue, siv, additionalData))
			}

			out[jsonFieldName] = final
			continue
		}

		// --- encrypt basic types ---
		additionalData := append([]byte(fieldName), ad...)
		out[jsonFieldName] = encryptToString(fieldValue, siv, additionalData)
	}

	return out, nil
}

// Decrypts everything from raw map to v. v has to be a pointer to struct.
func DecryptFields(v any, raw map[string]any, key, ad []byte) error {
	siv, err := aessiv.New(key)
	if err != nil {
		return fmt.Errorf("failed to create encryption instance from password: %w", err)
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
					if err := DecryptFields(fieldValue.Interface(), nestedMap, key, ad); err != nil {
						return err
					}
					continue
				}
			}
		}

		// --- handle slices and arrays ---
		if fieldValue.Kind() == reflect.Array || fieldValue.Kind() == reflect.Slice {
			encryptedSlice, ok := data.([]any) // arrays are always []any after json.Unmarshal (not []string etc.)
			if !ok {
				continue // this should not happen ever
			}

			if len(encryptedSlice) == 0 {
				fieldValue.Set(reflect.MakeSlice(fieldValue.Type(), 0, 0))
				continue
			}

			elemType := fieldValue.Type().Elem() // type of elements in array
			newSlice := reflect.MakeSlice(fieldValue.Type(), len(encryptedSlice), len(encryptedSlice))

			for i, ciphertextB64 := range encryptedSlice {
				ciphertext, ok := ciphertextB64.(string)
				if !ok {
					fmt.Printf("unexpected type of element in encrypted array %s\n", fieldName)
					continue
				}
				plaintext, err := decryptString(ciphertext, fieldName, siv, ad)
				if err != nil {
					fmt.Printf("failed to decrypt element of field %s\n", fieldName)
					continue
				}

				originalValue := decodeValue(plaintext, elemType)
				newSlice.Index(i).Set(originalValue)
			}

			fieldValue.Set(newSlice)
			continue
		}

		cipherStr, ok := data.(string)
		if !ok {
			continue // the value (still encrypted) is not string => skip ig?
		}

		// --- decrypt basic types ---
		plaintext, err := decryptString(cipherStr, fieldName, siv, ad)
		if err != nil {
			return fmt.Errorf("failed to decrypt field %s: %w", fieldName, err)
		}

		// convert decrypted string back to the field type
		originalValue := decodeValue(plaintext, fieldType.Type)
		fieldValue.Set(originalValue)
	}

	return nil
}

// Decodes ciphertext from Base64 and decrypts it to plaintext
func decryptString(cipherStr, fieldName string, siv *aessiv.AESSIV, ad []byte) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(cipherStr)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64 for field %s: %w", fieldName, err)
	}

	additionalData := append([]byte(fieldName), ad...)
	plaintext, err := siv.Open(nil, nil, ciphertext, additionalData)
	if err != nil {
		return "", fmt.Errorf("siv decryption failed for field %s: %w", fieldName, err)
	}

	return string(plaintext), nil
}

func encryptToString(val reflect.Value, siv *aessiv.AESSIV, ad []byte) string {
	plaintext := encodeValue(val)
	ciphertext := siv.Seal(nil, nil, []byte(plaintext), ad)
	return base64.StdEncoding.EncodeToString(ciphertext)
}

// encodeValue returns a deterministic string representation for encryption
func encodeValue(v any) string {
	if v == nil {
		return ""
	}

	// handle reflect.Value
	if rv, ok := v.(reflect.Value); ok {
		v = rv.Interface()
	}

	switch val := v.(type) {
	case string:
		return val

	case []byte:
		return string(val)

	case int, int8, int16, int32, int64:
		return strconv.FormatInt(reflect.ValueOf(val).Int(), 10)

	case uint, uint8, uint16, uint32, uint64:
		return strconv.FormatUint(reflect.ValueOf(val).Uint(), 10)

	case float32, float64:
		return strconv.FormatFloat(reflect.ValueOf(val).Float(), 'f', -1, 64)

	case bool:
		return strconv.FormatBool(val)

	case time.Time:
		return val.UTC().Format(time.RFC3339)

	case uuid.UUID: // ← Important for your case
		return val.String()

	case fmt.Stringer:
		return val.String()

	default:
		// ehhh
		fmt.Println("encodeValue() fallback")
		if b, err := json.Marshal(val); err == nil {
			return string(b)
		}
		return fmt.Sprintf("%v", val)
	}
}

// decodeValue tries sets the value from its string representation.
func decodeValue(plain string, targetType reflect.Type) reflect.Value {
	// unwrap pointer
	if targetType.Kind() == reflect.Pointer {
		targetType = targetType.Elem()
	}

	switch targetType {
	case reflect.TypeOf(""):
		return reflect.ValueOf(plain)

	case reflect.TypeOf(true):
		b, _ := strconv.ParseBool(plain)
		return reflect.ValueOf(b)

	case reflect.TypeOf(float32(0)), reflect.TypeOf(float64(0)):
		f, _ := strconv.ParseFloat(plain, 64)
		return reflect.ValueOf(f).Convert(targetType)

	case reflect.TypeOf(time.Time{}):
		t, _ := time.Parse(time.RFC3339, plain)
		return reflect.ValueOf(t)

	case reflect.TypeOf(uuid.UUID{}):
		u, _ := uuid.Parse(plain)
		return reflect.ValueOf(u)

	default:
		switch targetType.Kind() {

		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			i, _ := strconv.ParseInt(plain, 10, 64)
			return reflect.ValueOf(i).Convert(targetType)

		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			u, _ := strconv.ParseUint(plain, 10, 64)
			return reflect.ValueOf(u).Convert(targetType)

		case reflect.String:
			return reflect.ValueOf(plain).Convert(targetType)

		case reflect.Bool:
			b, _ := strconv.ParseBool(plain)
			return reflect.ValueOf(b).Convert(targetType)

		case reflect.Float32, reflect.Float64:
			f, _ := strconv.ParseFloat(plain, 64)
			return reflect.ValueOf(f).Convert(targetType)
		}
	}

	return reflect.Zero(targetType)
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
