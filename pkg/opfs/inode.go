//go:build js && wasm

package opfs

import (
	"path/filepath"
	"strings"
	"sync"
	"syscall/js"
)

var (
	// inodeCache makes all opens of a normalized path share one inode and access handle.
	// Im not using sync.Map cuz we frequently add and delete from the map; sync.Map is best for high reads, low writes
	inodeCache   = make(map[string]*opfsInode)
	inodeCacheMu sync.Mutex
)

// opfsInode is the shared browser-file state for one normalized path.
// Multiple OPFSFile values may reference it while maintaining independent offsets.
type opfsInode struct {
	handle js.Value // FileSystemFileHandle       - used for opening/creating files (careful, its async)
	access js.Value // FileSystemSyncAccessHandle - used for reading/writing to files (sync)
	path   string
	refs   int // count the number of "references" to this file, so that we can close it after all "refs" are done with it
	mu     sync.Mutex
}

// ----------------------------------------------------

// normalizePath cleans lexical path elements and removes a leading "./";
// for example, "a/./b" and "a/../a/b" both become "a/b".
func normalizePath(p string) string {
	p = filepath.Clean(p)
	p = strings.TrimPrefix(p, "./")
	return p
}
