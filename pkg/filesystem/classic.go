//go:build !js

package filesystem

import (
	"os"
	"path/filepath"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/helper/chroot"
	"github.com/go-git/go-billy/v5/osfs"
)

const DirName string = ".git-calendar-data"

// easy as that you bozo
//
// Returns a FS starting from users home directory
func GetFS() (billy.Filesystem, error) {
	// get user home dir
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	// ensure the directory exists on the real filesystem
	rootPath := filepath.Join(home, DirName)
	if err := os.MkdirAll(rootPath, 0o755); err != nil {
		return nil, err
	}

	// base filesystem rooted at home
	base := osfs.New(home)

	// chroot into DirName
	scoped := chroot.New(base, DirName)
	return scoped, nil
}
