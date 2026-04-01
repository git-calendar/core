package encryption

import (
	"reflect"
	"testing"
)

func Test_getFieldName(t *testing.T) {
	type testStruct struct {
		Normal    string `encrypt:"field"`
		NoTag     string
		Ignore    string `encrypt:"-"`
		EmptyName string `encrypt:""`
	}
	typ := reflect.TypeOf(testStruct{})

	tests := []struct {
		name  string
		field reflect.StructField
		want  string
	}{
		{
			name:  "simple name",
			field: typ.Field(0),
			want:  "field",
		},
		{
			name:  "no tag uses field name",
			field: typ.Field(1),
			want:  "NoTag",
		},
		{
			name:  "ignored field",
			field: typ.Field(2),
			want:  "-",
		},
		{
			name:  "empty name in tag falls back to field name",
			field: typ.Field(3),
			want:  "EmptyName",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getFieldName(tt.field); got != tt.want {
				t.Errorf("getFieldName() = %v, want %v", got, tt.want)
			}
		})
	}
}
