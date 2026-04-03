package encryption

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	aessiv "github.com/jedisct1/go-aes-siv"
)

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
