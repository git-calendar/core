//go:build js && wasm

package filesystem

import (
	"errors"
	"fmt"
	"syscall/js"

	"github.com/git-calendar/core/pkg/opfs"
	"github.com/go-git/go-billy/v5"
)

const DirName = "git-calendar-data"

func GetFS() (billy.Filesystem, error) {
	// gets the handle from js window.opfsRootHandle
	rootHandle := js.Global().Get("opfsRootHandle")
	if rootHandle.IsUndefined() {
		return nil, errors.New("opfsRootHandle not initialized")
	}

	// get or create subdirectory
	dirHandlePromise := rootHandle.Call("getDirectoryHandle", DirName, map[string]any{
		"create": true,
	})

	dirHandle, err := opfs.Await(dirHandlePromise)
	if err != nil {
		return nil, fmt.Errorf("cant get the git-calendar-data folder handle: %w", err)
	}

	return opfs.New(dirHandle), nil
}
