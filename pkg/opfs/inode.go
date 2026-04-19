//go:build js && wasm

package opfs

import (
	"sync"
	"syscall/js"
	"time"
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
	path   string
	refs   int // count the number of "references" to this file, so that we can close it after all "refs" are done with it

	buf     []byte // nil means not loaded, empty means empty
	lastMod time.Time
	dirty   bool
}

func (i *opfsInode) ensureLoaded() {
	if i.buf != nil {
		return
	}
	i.loadBuffer() // called with mu held
}

func (i *opfsInode) loadBuffer() {
	// read the contents
	fileJS, err := Await(i.handle.Call("getFile"))
	if err != nil {
		// file probably doesn't exist
		i.buf = make([]byte, 0)
	} else {
		arrayBuffer, err := Await(fileJS.Call("arrayBuffer"))
		if err != nil {
			i.buf = make([]byte, 0)
		} else {
			uint8 := js.Global().Get("Uint8Array").New(arrayBuffer)
			i.buf = make([]byte, uint8.Get("length").Int())
			js.CopyBytesToGo(i.buf, uint8)
		}
	}
}
