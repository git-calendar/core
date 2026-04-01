//go:build js && wasm

package filesystem

import "testing"

func Test_normalizePath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "Basic path",
			path: "foo/bar",
			want: "foo/bar",
		},
		{
			name: "Dot start path",
			path: "./foo/bar",
			want: "foo/bar",
		},
		{
			name: "Dot middle path",
			path: "foo/./bar",
			want: "foo/bar",
		},
		{
			name: "Double dot middle path",
			path: "foo/../bar",
			want: "bar",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizePath(tt.path); got != tt.want {
				t.Errorf("normalizePath() = %v, want %v", got, tt.want)
			}
		})
	}
}
