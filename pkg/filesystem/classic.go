//go:build !js

// Package filesystem provides the platform-specific application data filesystem.
package filesystem

import (
	"os"
	"path/filepath"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/helper/chroot"
	"github.com/go-git/go-billy/v5/osfs"
)

// DirName is the native data directory under the user's home directory.
const DirName string = ".git-calendar-data"

// GetFS returns a filesystem rooted at the native application data directory.
func GetFS() (billy.Filesystem, error) {
	// get user home dir
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return GetFSAt(home)
}

// GetFSAt returns a filesystem rooted at the application data directory inside dir.
func GetFSAt(dir string) (billy.Filesystem, error) {
	// ensure the directory exists on the real filesystem
	rootPath := filepath.Join(dir, DirName)
	if err := os.MkdirAll(rootPath, 0o755); err != nil {
		return nil, err
	}

	base := osfs.New(dir)
	scoped := chroot.New(base, DirName)
	return scoped, nil
}
