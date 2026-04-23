//go:build js && wasm

package filesystem

import (
	"github.com/git-calendar/core/pkg/idb"
	"github.com/go-git/go-billy/v5"
)

const DirName = "git-calendar-data" // the storeName for IndexedDB

func GetFS() (billy.Filesystem, error) {
	return idb.New(DirName, 1)
}
