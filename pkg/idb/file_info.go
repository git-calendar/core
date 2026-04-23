//go:build js && wasm

package idb

import (
	"io/fs"
	"os"
	"syscall/js"
	"time"
)

type IDBFileInfo struct {
	name    string
	size    int64
	modTime time.Time
	mode    os.FileMode
}

func (fi *IDBFileInfo) Name() string       { return fi.name }
func (fi *IDBFileInfo) Size() int64        { return fi.size }
func (fi *IDBFileInfo) ModTime() time.Time { return fi.modTime }
func (fi *IDBFileInfo) Sys() any           { return nil }
func (fi *IDBFileInfo) Mode() os.FileMode  { return fi.mode }
func (fi *IDBFileInfo) Type() fs.FileMode  { return fi.Mode().Type() }
func (fi *IDBFileInfo) IsDir() bool        { return fi.mode.IsDir() }

// Converts the file info to a JS Object.
func (fi *IDBFileInfo) toJS() js.Value {
	obj := js.Global().Get("Object").New()
	obj.Set("name", fi.name)
	obj.Set("size", fi.size)
	obj.Set("mod_time", fi.modTime.UnixMilli())
	obj.Set("mode", int(fi.mode))
	return obj
}

// Converts a JS Object into a IDBFileInfo struct.
func FileInfoFromJS(jsVal js.Value) *IDBFileInfo {
	return &IDBFileInfo{
		name:    jsVal.Get("name").String(),
		size:    int64(jsVal.Get("size").Int()),
		modTime: time.UnixMilli(int64(jsVal.Get("mod_time").Int())),
		mode:    os.FileMode(jsVal.Get("mode").Int()),
	}
}
