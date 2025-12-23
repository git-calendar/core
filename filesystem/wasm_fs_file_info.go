//go:build js && wasm

package filesystem

import (
	"io/fs"
	"os"
	"time"
)

type OPFSFileInfo struct {
	name    string
	size    int64
	modTime time.Time
	isDir   bool
}

var _ os.FileInfo = (*OPFSFileInfo)(nil) // makes sure that it implements all the interface methods, it wont compile without it

func (fi *OPFSFileInfo) Name() string       { return fi.name }
func (fi *OPFSFileInfo) Size() int64        { return fi.size }
func (fi *OPFSFileInfo) ModTime() time.Time { return fi.modTime }
func (fi *OPFSFileInfo) IsDir() bool        { return fi.isDir }
func (fi *OPFSFileInfo) Sys() any           { return nil }              // Sys can return the underlying data source, usually nil for virtual FS
func (fi *OPFSFileInfo) Type() fs.FileMode  { return fi.Mode().Type() } // use Mode() which we already implemented and extract just the type bits

// Mode returns the file permissions.
// Git checks this to see if a file is executable or a directory.
func (fi *OPFSFileInfo) Mode() os.FileMode {
	if fi.isDir {
		return os.ModeDir | 0o755
	}
	return 0o644
}

func (fi *OPFSFileInfo) Info() (fs.FileInfo, error) {
	// This is required if you are implementing the fs.DirEntry interface
	return fi, nil
}
