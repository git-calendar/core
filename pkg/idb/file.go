//go:build js && wasm

package idb

import (
	"fmt"
	"io"
	"syscall/js"
	"time"
)

type IDBFile struct {
	fs      *IndexedDB // A reference to it's fs.
	key     string     // The key used in IndexedDB (absolute filepath).
	relPath string     // Path relative to fs.
	offset  int64      // Current offset in bytes.
}

func (f *IDBFile) Name() string {
	return f.relPath // returns the filepath RELATIVE to current fs root
}

func (f *IDBFile) Read(p []byte) (int, error) {
	n, err := f.ReadAt(p, f.offset)
	f.offset += int64(n)
	return n, err
}

func (f *IDBFile) Write(p []byte) (int, error) {
	n, err := f.WriteAt(p, f.offset)
	f.offset += int64(n)
	return n, err
}

func (f *IDBFile) ReadAt(p []byte, off int64) (x int, err error) {
	tx := NewTx()
	req := tx.Get(contentStoreName, f.key)

	if err := tx.Commit(f.fs.jsDB); err != nil {
		return 0, err
	}

	existingVal := req.Result()
	if !existingVal.Truthy() {
		return 0, io.EOF
	}

	length := existingVal.Get("length").Int()
	if int(off) >= length {
		return 0, io.EOF
	}

	sub := existingVal.Call("subarray", off)
	n := js.CopyBytesToGo(p, sub)

	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (f *IDBFile) WriteAt(p []byte, off int64) (s int, err error) {
	// read existing data and info
	txRead := NewTx()
	contentReq := txRead.Get(contentStoreName, f.key)
	infoReq := txRead.Get(infoStoreName, f.key)
	if err := txRead.Commit(f.fs.jsDB); err != nil {
		return 0, err
	}
	existingVal := contentReq.Result()

	// prepare buffer
	var buffer []byte
	if existingVal.Truthy() {
		end := int(off) + len(p)
		size := max(existingVal.Length(), end)
		buffer = make([]byte, size)
		js.CopyBytesToGo(buffer, existingVal)
	} else {
		end := int(off) + len(p)
		buffer = make([]byte, end)
	}

	// write into buffer at offset
	copy(buffer[off:], p)

	// convert to JS array
	jsBuf := js.Global().Get("Uint8Array").New(len(buffer))
	js.CopyBytesToJS(jsBuf, buffer)

	// update FileInfo
	info := FileInfoFromJS(infoReq.Result())
	info.size = int64(len(buffer)) // update size
	info.modTime = time.Now()      // update modification time

	// store Content and Info in a fresh transaction
	txWrite := NewTx()
	txWrite.Put(contentStoreName, f.key, jsBuf)
	txWrite.Put(infoStoreName, f.key, info.toJS())
	if err := txWrite.Commit(f.fs.jsDB); err != nil {
		return 0, err
	}

	return len(p), nil
}

func (f *IDBFile) Seek(offset int64, whence int) (int64, error) {
	var base int64

	switch whence {
	case io.SeekStart:
		base = 0

	case io.SeekCurrent:
		base = f.offset

	case io.SeekEnd:
		stat, err := f.fs.Stat(f.relPath)
		if err != nil {
			return 0, err
		}
		base = stat.Size()

	default:
		return 0, fmt.Errorf("invalid whence")
	}

	newOffset := base + offset
	if newOffset < 0 {
		return 0, fmt.Errorf("negative position")
	}

	f.offset = newOffset
	return f.offset, nil
}

func (f *IDBFile) Close() error {
	return nil
}

func (f *IDBFile) Lock() error {
	return nil
}

func (f *IDBFile) Unlock() error {
	return nil
}

func (f *IDBFile) Truncate(size int64) error {
	txRead := NewTx()
	contentReq := txRead.Get(contentStoreName, f.key)
	infoReq := txRead.Get(infoStoreName, f.key)

	if err := txRead.Commit(f.fs.jsDB); err != nil {
		return err
	}

	existingVal := contentReq.Result()
	var data []byte
	if existingVal.Truthy() {
		data = make([]byte, existingVal.Length())
		js.CopyBytesToGo(data, existingVal)
	}

	if int64(len(data)) > size {
		data = data[:size]
	} else if int64(len(data)) < size {
		newBuf := make([]byte, size)
		copy(newBuf, data)
		data = newBuf
	}

	jsBuf := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(jsBuf, data)

	info := FileInfoFromJS(infoReq.Result())
	info.size = size
	info.modTime = time.Now()

	txWrite := NewTx()
	txWrite.Put(contentStoreName, f.key, jsBuf)
	txWrite.Put(infoStoreName, f.key, info.toJS())

	if err := txWrite.Commit(f.fs.jsDB); err != nil {
		return err
	}

	return nil
}
