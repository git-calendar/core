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

	// create OPFS rooted at /
	fs := opfs.New(rootHandle)

	// ensure directory exists
	if err := fs.MkdirAll(DirName, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create dir: %w", err)
	}

	// chroot into the directory
	chrooted, err := fs.Chroot(DirName)
	if err != nil {
		return nil, fmt.Errorf("failed to chroot: %w", err)
	}

	return chrooted, nil
}
