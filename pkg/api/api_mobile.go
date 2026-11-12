//go:build !js

package api

import (
	"github.com/git-calendar/core/pkg/core"
	"github.com/git-calendar/core/pkg/filesystem"
)

// NewApiAt creates an API that stores its data in dir, for mobile platforms without a writable home directory.
func NewApiAt(dir string) (*Api, error) {
	fs, err := filesystem.GetFSAt(dir)
	if err != nil {
		return nil, err
	}
	return &Api{inner: core.NewCoreWithFS(fs)}, nil
}
