//go:build js && wasm

package idb

import (
	"io/fs"
	"os"
	"syscall/js"
	"time"
)

// IDBFileInfo describes an IndexedDB-backed file or directory.
type IDBFileInfo struct {
	name    string
	size    int64
	modTime time.Time
	mode    os.FileMode
}

// Name returns the base name.
func (fi *IDBFileInfo) Name() string { return fi.name }

// Size returns the file size in bytes.
func (fi *IDBFileInfo) Size() int64 { return fi.size }

// ModTime returns the last modification time.
func (fi *IDBFileInfo) ModTime() time.Time { return fi.modTime }

// Sys returns no underlying system data.
func (fi *IDBFileInfo) Sys() any { return nil }

// Mode returns the file mode.
func (fi *IDBFileInfo) Mode() os.FileMode { return fi.mode }

// Type returns the type bits from Mode.
func (fi *IDBFileInfo) Type() fs.FileMode { return fi.Mode().Type() }

// IsDir reports whether the entry is a directory.
func (fi *IDBFileInfo) IsDir() bool { return fi.mode.IsDir() }

// toJS converts file information to a JavaScript object.
func (fi *IDBFileInfo) toJS() js.Value {
	obj := js.Global().Get("Object").New()
	obj.Set("name", fi.name)
	obj.Set("size", fi.size)
	obj.Set("mod_time", fi.modTime.UnixMilli())
	obj.Set("mode", int(fi.mode))
	return obj
}

// FileInfoFromJS converts a JavaScript object to IDBFileInfo.
func FileInfoFromJS(jsVal js.Value) *IDBFileInfo {
	return &IDBFileInfo{
		name:    jsVal.Get("name").String(),
		size:    int64(jsVal.Get("size").Int()),
		modTime: time.UnixMilli(int64(jsVal.Get("mod_time").Int())),
		mode:    os.FileMode(jsVal.Get("mode").Int()),
	}
}
