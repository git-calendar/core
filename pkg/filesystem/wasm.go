//go:build js && wasm

package filesystem

import (
	"errors"
	"syscall/js"

	"github.com/git-calendar/core/pkg/opfs"
	"github.com/go-git/go-billy/v5"
)

const DirName = "git-calendar-data"

func GetFS() (billy.Filesystem, error) {
	rootHandle := js.Global().Get("opfsRootHandle")
	if rootHandle.IsUndefined() {
		return nil, errors.New("opfsRootHandle not initialized")
	}

	return &opfs.OPFS{
		RootHandle: rootHandle,
	}, nil
}
