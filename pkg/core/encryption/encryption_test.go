package encryption

import (
	"reflect"
	"testing"
)

func Test_hasOmitzero(t *testing.T) {
	type testStruct struct {
		WithOmitzero    string `json:"field,omitzero"`
		WithoutOmitzero string `json:"field2"`
	}
	typ := reflect.TypeOf(testStruct{})

	tests := []struct {
		name  string
		field reflect.StructField
		want  bool
	}{
		{
			name:  "has omitzero",
			field: typ.Field(0),
			want:  true,
		},
		{
			name:  "does not have omitzero",
			field: typ.Field(1),
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasOmitzero(tt.field); got != tt.want {
				t.Errorf("hasOmitzero() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getJsonFieldName(t *testing.T) {
	type testStruct struct {
		Normal          string `json:"field"`
		WithoutOmitzero string `json:"field2,omitzero"`
		NoTag           string
		Ignore          string `json:"-"`
		EmptyName       string `json:",omitzero"`
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
			name:  "name with option",
			field: typ.Field(1),
			want:  "field2",
		},
		{
			name:  "no tag uses field name",
			field: typ.Field(2),
			want:  "NoTag",
		},
		{
			name:  "ignored field",
			field: typ.Field(3),
			want:  "",
		},
		{
			name:  "empty name in tag falls back to field name",
			field: typ.Field(4),
			want:  "EmptyName",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getJsonFieldName(tt.field); got != tt.want {
				t.Errorf("getJsonFieldName() = %v, want %v", got, tt.want)
			}
		})
	}
}
