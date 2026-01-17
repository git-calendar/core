//go:build js && wasm

package filesystem

import (
	"path/filepath"
	"strings"
	"sync"
	"syscall/js"
)

var (
	// for making sure we dont create a new struct for the same file.
	//
	// im not using sync.Map cuz we frequently add and delete from the map; sync.Map is best for high reads, low writes

	inodeCache   map[string]*opfsInode = make(map[string]*opfsInode)
	inodeCacheMu sync.Mutex
)

// This structs represents the REAL file in browsers OPFS. There cannot be multiple instances of this struct. All OPFSFiles pointing to same file share the one and only instance of opfsInode.
type opfsInode struct {
	handle js.Value // FileSystemFileHandle       - used for opening/creating files (careful, its async)
	access js.Value // FileSystemSyncAccessHandle - used for reading/writing to files (sync)
	path   string
	refs   int // count the number of "references" to this file, so that we can close it after all "refs" are done with it
	mu     sync.Mutex
}

// ----------------------------------------------------

// A helper function to normalize paths
//
//	a/./b -> a/b
//	./a/b -> a/b
//	a/../a/b -> a/b
func normalizePath(p string) string {
	p = filepath.Clean(p)
	p = strings.TrimPrefix(p, "./")
	return p
}
