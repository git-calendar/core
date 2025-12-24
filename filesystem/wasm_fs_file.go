//go:build js && wasm

package filesystem

import (
	"fmt"
	"io"
	"syscall/js"

	"github.com/go-git/go-billy/v5"
)

type OPFSFile struct {
	handle js.Value // FileSystemFileHandle       - used for opening/creating files (careful, its async)
	access js.Value // FileSystemSyncAccessHandle - used for reading/writing to files (sync)
	offset int64    // current read/write offset
}

var _ billy.File = (*OPFSFile)(nil) // makes sure that it implements all the interface methods, it wont compile without it

// helper function to open sync access for file
func (f *OPFSFile) openAccess() error {
	// skip if already has access
	if f.access.Truthy() { // basically "if f.access != nil"
		return nil
	}

	var err error
	// https://developer.mozilla.org/en-US/docs/Web/API/FileSystemFileHandle/createSyncAccessHandle
	f.access, err = await(f.handle.Call("createSyncAccessHandle")) // returns Promise<FileSystemSyncAccessHandle>
	return err
}

func (f *OPFSFile) Write(p []byte) (int, error) {
	// use already implemented WriteAt
	n, err := f.WriteAt(p, f.offset)

	// update the offset
	f.offset += int64(n)

	return n, err
}

func (f *OPFSFile) WriteAt(p []byte, off int64) (n int, err error) {
	if err := f.openAccess(); err != nil {
		return 0, fmt.Errorf("failed to open access: %w", err)
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
	n = f.access.Call("write", buf, map[string]any{"at": off}).Int()

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
		return 0, fmt.Errorf("failed to open access: %w", err)
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
	n = f.access.Call("read", buf, map[string]any{"at": off}).Int()

	if n == 0 {
		return 0, io.EOF
	}

	// copy all the data from JS to Go
	js.CopyBytesToGo(p[:n], buf) // p[:n] so that it copies less bytes when less were returned

	return // returns n, err actually (named return values)
}

func (f *OPFSFile) Seek(offset int64, whence int) (newOffset int64, err error) {
	f.openAccess()

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
		size := f.access.Call("getSize").Int() // https://developer.mozilla.org/en-US/docs/Web/API/FileSystemSyncAccessHandle/getSize
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

	if f.handle.Truthy() { // basically "if f.access != nil"
		return f.handle.Get("name").String()
	}

	return ""
}

func (f *OPFSFile) Truncate(size int64) error {
	f.openAccess()

	var err error
	defer func() { // recover a panic from Call("truncate")
		if r := recover(); r != nil {
			err = fmt.Errorf("OPFS File Truncate failed: %+v", r)
		}
	}()

	// https://developer.mozilla.org/en-US/docs/Web/API/FileSystemSyncAccessHandle/truncate
	f.access.Call("truncate", size)

	if f.offset > size {
		f.offset = size
	}
	return err
}

func (f *OPFSFile) Close() error {
	if f.access.IsUndefined() || f.access.IsNull() {
		return nil // already closed
	}

	var err error
	defer func() { // recover a panic from Call("flush"/"close")
		if r := recover(); r != nil {
			err = fmt.Errorf("OPFS File Close failed: %+v", r)
			f.access = js.Undefined()
		}
	}()

	f.access.Call("flush")
	f.access.Call("close")
	f.access = js.Undefined() // reset
	return err
}

func (f *OPFSFile) Lock() error {
	return nil // will do nothing i guess
}

func (f *OPFSFile) Unlock() error {
	return nil // will do nothing i guess
}
