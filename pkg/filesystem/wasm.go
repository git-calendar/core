//go:build js && wasm

// Package filesystem provides the platform-specific application data filesystem.
package filesystem

import (
	"github.com/git-calendar/core/pkg/idb"
	"github.com/go-git/go-billy/v5"
)

// DirName is the IndexedDB database name used by the browser filesystem.
const DirName = "git-calendar-data"

// GetFS returns a filesystem backed by browser IndexedDB.
func GetFS() (billy.Filesystem, error) {
	return idb.New(DirName, 1)
}
