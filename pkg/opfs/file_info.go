//go:build js && wasm

package opfs

import (
	"io/fs"
	"os"
	"time"
)

// OPFSFileInfo describes an OPFS file or directory.
type OPFSFileInfo struct {
	name    string
	size    int64
	modTime time.Time
	isDir   bool
}

// Verify at compile time that OPFSFileInfo implements os.FileInfo.
var _ os.FileInfo = (*OPFSFileInfo)(nil)

// Name returns the base name.
func (fi *OPFSFileInfo) Name() string { return fi.name }

// Size returns the file size in bytes.
func (fi *OPFSFileInfo) Size() int64 { return fi.size }

// ModTime returns the last modification time.
func (fi *OPFSFileInfo) ModTime() time.Time { return fi.modTime }

// IsDir reports whether the entry is a directory.
func (fi *OPFSFileInfo) IsDir() bool { return fi.isDir }

// Sys returns no underlying system data.
func (fi *OPFSFileInfo) Sys() any { return nil }

// Type returns the type bits from Mode.
func (fi *OPFSFileInfo) Type() fs.FileMode { return fi.Mode().Type() }

// Mode returns synthetic permissions; Git uses the type and executable bits
// when interpreting filesystem entries.
func (fi *OPFSFileInfo) Mode() os.FileMode {
	if fi.isDir {
		return os.ModeDir | 0o755
	}
	return 0o644
}

// Info implements fs.DirEntry by returning fi as fs.FileInfo.
func (fi *OPFSFileInfo) Info() (fs.FileInfo, error) {
	return fi, nil
}
