package encryption

import (
	"encoding/base64"
	"fmt"
	"reflect"
	"strconv"
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
	if val.Kind() == reflect.Pointer {
		val = val.Elem()
	}
	typ := val.Type()

	out := make(map[string]any)

	// iterate through fields
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)
		fieldName := fieldType.Name

		if fieldType.Tag.Get("json") != "-" { // if the field doesn't directly specify `json:"-"`
			out[fieldName] = field.Interface() // keep plaintext as default
		}

		if fieldType.Tag.Get("encrypt") != "true" { // skip if field doesn't have `encrypt:"true"`
			continue
		}

		var plaintext string
		switch field.Kind() {
		case reflect.String:
			plaintext = field.String()
		case reflect.Int, reflect.Int64, reflect.Int32, reflect.Int16, reflect.Int8:
			plaintext = strconv.FormatInt(field.Int(), 10)
		case reflect.Struct:
			if t, ok := field.Interface().(time.Time); ok {
				plaintext = t.UTC().Format(time.RFC3339)
			}
		default:
			continue
		}

		ciphertext := siv.Seal(nil, nil, []byte(plaintext), []byte(fieldName))
		out[fieldName] = base64.StdEncoding.EncodeToString(ciphertext)
	}

	return out, nil
}
