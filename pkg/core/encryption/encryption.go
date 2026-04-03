package encryption

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	aessiv "github.com/jedisct1/go-aes-siv"
	"golang.org/x/crypto/argon2"
)

const tagName string = "encrypt"

// Creates a key of length 'size' based on the provided 'password' plus 'salt'.
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
