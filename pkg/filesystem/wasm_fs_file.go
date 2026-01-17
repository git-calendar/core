//go:build js && wasm

package filesystem

import (
	"fmt"
	"io"
	"syscall/js"

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

func (f *OPFSFile) WriteAt(p []byte, off int64) (n int, err error) {
	if err := f.openAccess(); err != nil {
		return 0, fmt.Errorf("writeat: failed to open access: %w", err)
	}

	defer func() { // // recover a panic from Get("Uint8Array") or Call("write")
		if r := recover(); r != nil {
			n = 0
			err = fmt.Errorf("OPFS File WriteAt failed: %+v", r)
		}
	}()

	// create a byte array in js
	buf := js.Global().Get("Uint8Array").New(len(p))

	// copy the data from Go to JS
	js.CopyBytesToJS(buf, p)

	// call .write(data, {at: offset}) in JS
	n = f.inode.access.Call("write", buf, map[string]any{"at": off}).Int()

	// std os.File.WriteAt does NOT move the file offset
	return // returns n, err actually (named return values)
}

func (f *OPFSFile) Read(p []byte) (int, error) {
	// use already implemented ReadAt
	n, err := f.ReadAt(p, f.offset)

	// update the offset
	f.offset += int64(n)

	return n, err
}

func (f *OPFSFile) ReadAt(p []byte, off int64) (n int, err error) {
	if err := f.openAccess(); err != nil {
		return 0, fmt.Errorf("readat: failed to open access: %w", err)
	}

	defer func() { // recover a panic from Get("Uint8Array") or Call("read")
		if r := recover(); r != nil {
			n = 0
			err = fmt.Errorf("OPFS File ReadAt failed: %+v", r)
		}
	}()

	// create a byte array in JS
	buf := js.Global().Get("Uint8Array").New(len(p))

	// call .read(data, {at: offset}) in JS
	n = f.inode.access.Call("read", buf, map[string]any{"at": off}).Int()

	if n == 0 {
		return 0, io.EOF
	}

	// copy all the data from JS to Go
	js.CopyBytesToGo(p[:n], buf) // p[:n] so that it copies less bytes when less were returned

	return // returns n, err actually (named return values)
}

func (f *OPFSFile) Seek(offset int64, whence int) (newOffset int64, err error) {
	if err := f.openAccess(); err != nil {
		return 0, fmt.Errorf("seek: failed to open access: %w", err)
	}

	defer func() { // recover a panic from Call("getSize")
		if r := recover(); r != nil {
			newOffset = 0
			err = fmt.Errorf("OPFS File Seek failed: %+v", r)
		}
	}()

	switch whence {
	case io.SeekStart:
		// if seek from start, just set the value
		newOffset = offset
	case io.SeekCurrent:
		// if seek from currect offset, add the offset to the current one
		newOffset = f.offset + offset
	case io.SeekEnd:
		// if seek from end, get the file size and add the offset to the end
		size := f.inode.access.Call("getSize").Int() // https://developer.mozilla.org/en-US/docs/Web/API/FileSystemSyncAccessHandle/getSize
		newOffset = int64(size) + offset
	default:
		return 0, fmt.Errorf("invalid whence: %d", whence)
	}

	// check for invalid value
	if newOffset < 0 {
		return 0, fmt.Errorf("invalid seek: negative offset")
	}

	// set the new offset
	f.offset = newOffset

	return // returns newOffset, err actually (named return values)
}

func (f *OPFSFile) Name() (name string) {
	defer func() { // recover a panic from Get("name")
		if r := recover(); r != nil {
			name = ""
		}
	}()

	if f.inode.handle.Truthy() { // basically "if f.access != nil"
		return f.inode.handle.Get("name").String()
	}

	return ""
}

func (f *OPFSFile) Truncate(size int64) error {
	if err := f.openAccess(); err != nil {
		return fmt.Errorf("truncate: failed to open access: %w", err)
	}

	var err error
	defer func() { // recover a panic from Call("truncate")
		if r := recover(); r != nil {
			err = fmt.Errorf("OPFS File Truncate failed: %+v", r)
		}
	}()

	// https://developer.mozilla.org/en-US/docs/Web/API/FileSystemSyncAccessHandle/truncate
	f.inode.access.Call("truncate", size)

	if f.offset > size {
		f.offset = size
	}
	return err
}

func (f *OPFSFile) Close() error {
	inodeCacheMu.Lock()
	defer inodeCacheMu.Unlock()

	f.inode.refs--
	if f.inode.refs == 0 {
		err := f.closeAccess()
		delete(inodeCache, f.inode.path)
		f.inode = nil
		return err
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

// -----------------------------------------------------------

// Helper function to open sync access for file
func (f *OPFSFile) openAccess() error {
	f.inode.mu.Lock()
	defer f.inode.mu.Unlock()

	// skip if already has access
	if f.inode.access.Truthy() { // basically "if f.access != nil"
		return nil
	}

	var err error
	// https://developer.mozilla.org/en-US/docs/Web/API/FileSystemFileHandle/createSyncAccessHandle
	f.inode.access, err = await(f.inode.handle.Call("createSyncAccessHandle")) // returns Promise<FileSystemSyncAccessHandle>
	return err
}

// Helper to close just the access handle
func (f *OPFSFile) closeAccess() error {
	f.inode.mu.Lock()
	defer f.inode.mu.Unlock()

	if !f.inode.access.Truthy() {
		return nil // already closed
	}

	var err error
	defer func() { // recover a panic from Call("flush"/"close")
		if r := recover(); r != nil {
			err = fmt.Errorf("OPFS File Close failed: %+v", r)
			f.inode.access = js.Undefined()
		}
	}()

	f.inode.access.Call("flush")
	f.inode.access.Call("close")
	f.inode.access = js.Undefined()

	return err
}
