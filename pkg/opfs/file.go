//go:build js && wasm

package opfs

import (
	"errors"
	"fmt"
	"io"
	"syscall/js"
	"time"

	"github.com/go-git/go-billy/v5"
)

// This struct represents a file in our OPFS FileSystem, but there can by multiple instances of the single file, with different offsets, pointing at the same inode (aka. the REAL file)
type OPFSFile struct {
	inode  *opfsInode // the real file underneath
	offset int64      // current read/write offset
}

var _ billy.File = (*OPFSFile)(nil) // makes sure that it implements all the interface methods, it wont compile without it

func (f *OPFSFile) Write(p []byte) (int, error) {
	// use already implemented WriteAt
	n, err := f.WriteAt(p, f.offset)

	// update the offset
	f.offset += int64(n)

	return n, err
}

func (f *OPFSFile) WriteAt(p []byte, off int64) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	f.inode.ensureLoaded()

	end := off + int64(len(p))

	// grow buffer if needed
	if end > int64(len(f.inode.buf)) {
		newBuf := make([]byte, end)
		copy(newBuf, f.inode.buf)
		f.inode.buf = newBuf
	}

	n := copy(f.inode.buf[off:end], p)

	f.inode.dirty = true
	f.inode.lastMod = time.Now()

	// std os.File.WriteAt does NOT move the file offset, but rather only returns the n of written bytes
	return n, nil
}

func (f *OPFSFile) Read(p []byte) (int, error) {
	// use already implemented ReadAt
	n, err := f.ReadAt(p, f.offset)

	// update the offset
	f.offset += int64(n)

	return n, err
}

func (f *OPFSFile) ReadAt(p []byte, off int64) (n int, err error) {
	f.inode.ensureLoaded()

	n = copy(p, f.inode.buf[off:])
	if n < len(p) {
		err = io.EOF // short read means we hit the end
	}
	return n, err
}

func (f *OPFSFile) Seek(offset int64, whence int) (int64, error) {
	f.inode.ensureLoaded()

	var newOffset int64

	switch whence {
	case io.SeekStart:
		// if seek from start, just set the value
		newOffset = offset
	case io.SeekCurrent:
		// if seek from currect offset, add the offset to the current one
		newOffset = f.offset + offset
	case io.SeekEnd:
		// if seek from end, add the offset to file size
		newOffset = int64(len(f.inode.buf)) + offset
	default:
		return 0, fmt.Errorf("invalid whence: %d", whence)
	}

	// check for invalid value
	if newOffset < 0 {
		return 0, fmt.Errorf("invalid seek: negative offset")
	}

	// set the new offset
	f.offset = newOffset

	return newOffset, nil
}

func (f *OPFSFile) Name() (name string) {
	defer func() { // recover a panic from Get("name")
		if r := recover(); r != nil {
			name = ""
		}
	}()

	if f.inode.handle.Truthy() { // basically "if f.inode.handle != nil"
		return f.inode.handle.Get("name").String()
	}

	return ""
}

func (f *OPFSFile) Truncate(size int64) error {
	if size < 0 {
		return errors.New("negative size")
	}
	f.inode.ensureLoaded()

	curSize := int64(len(f.inode.buf))

	switch {
	case size < curSize:
		// shrink
		f.inode.buf = f.inode.buf[:size]

	case size > curSize:
		// grow (zero-fill)
		newBuf := make([]byte, size)
		copy(newBuf, f.inode.buf)
		f.inode.buf = newBuf
	}

	if f.offset > size {
		f.offset = size
	}
	f.inode.dirty = true
	f.inode.lastMod = time.Now()
	return nil
}

func (f *OPFSFile) Close() (err error) {
	inodeCacheMu.Lock()
	defer inodeCacheMu.Unlock()

	if f.inode == nil {
		return nil // already closed
	}

	f.inode.refs--

	if f.inode.refs <= 0 {
		if f.inode.dirty {
			defer func() { // recover a panic from Call()
				if r := recover(); r != nil {
					err = fmt.Errorf("OPFS File Close paniced: %+v", r)
				}
			}()

			// create a byte array in js
			buf := js.Global().Get("Uint8Array").New(len(f.inode.buf))
			// copy the data from Go to JS
			js.CopyBytesToJS(buf, f.inode.buf)

			access, err := Await(f.inode.handle.Call("createSyncAccessHandle"))
			if err == nil {
				access.Call("write", buf, map[string]any{"at": 0})
				access.Call("truncate", len(f.inode.buf))
				// access.Call("flush")
				access.Call("close")
			} else {
				// fallback to slower async write
				writable, err2 := Await(f.inode.handle.Call("createWritable"))
				if err2 != nil {
					return errors.Join(err, err2)
				}

				_, _ = Await(writable.Call("write", map[string]any{
					"type":     "write",
					"position": 0,
					"data":     buf,
				}))

				_, _ = Await(writable.Call("truncate", len(f.inode.buf)))
				// _, _ = Await(writable.Call("flush"))
				_, _ = Await(writable.Call("close"))
			}
		}

		// remove from inode cache
		// (important bcs we can't have a dangling reference to a inode, we wouldn't be able to delete it!)
		delete(inodeCache, f.inode.path)
	}

	f.inode = nil
	return nil
}

func (f *OPFSFile) Lock() error {
	// implementing like so: "f.inode.mu.Lock()" breaks it for some reason
	return nil // will do nothing i guess
}

func (f *OPFSFile) Unlock() error {
	// implementing like so: "f.inode.mu.Lock()" breaks it for some reason
	return nil // will do nothing i guess
}
