//go:build js && wasm

// Package opfs implements the billy.Filesystem interface backed by OPFS store.
// It is only usable in a js/wasm build targeting a browser environment.
// https://developer.mozilla.org/en-US/docs/Web/API/File_System_API/Origin_private_file_system
package opfs

import (
	"fmt"
	"io"
	"io/fs"
	"math/rand"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"syscall/js"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/helper/chroot"
)

// OPFS implements billy.Filesystem over an origin-private directory handle.
type OPFS struct {
	// RootHandle is the FileSystemDirectoryHandle used as the filesystem root
	RootHandle js.Value
}

// Verify at compile time that OPFS implements billy.Filesystem.
var _ billy.Filesystem = (*OPFS)(nil)

// New returns an OPFS filesystem rooted at baseDirHandle.
func New(baseDirHandle js.Value) *OPFS {
	return &OPFS{
		RootHandle: baseDirHandle,
	}
}

// MkdirAll creates path and its parents; OPFS ignores perm.
func (fs *OPFS) MkdirAll(path string, perm fs.FileMode) error {
	_, err := fs.getDirectoryHandle(path, true)
	return err
}

// Join joins path elements using slash-separated filesystem semantics.
func (fs *OPFS) Join(elem ...string) string {
	return pathpkg.Join(elem...)
}

// OpenFile opens fullPath with the supplied flags; OPFS ignores perm.
func (fs *OPFS) OpenFile(fullPath string, flag int, perm os.FileMode) (billy.File, error) {
	create := flag&os.O_CREATE != 0
	fullPath = normalizePath(fullPath)

	var err error
	defer func() { // recover any panic that could happen along the way: Call()
		if r := recover(); r != nil {
			err = fmt.Errorf("OPFS OpenFile %q failed: %+v", fullPath, r)
		}
	}()

	// check the cache
	inodeCacheMu.Lock()
	defer inodeCacheMu.Unlock()
	if inode, ok := inodeCache[fullPath]; ok && inode != nil {
		inode.refs++
		f := &OPFSFile{
			inode:  inode,
			offset: 0,
		}

		if err := fs.applyFlags(f, flag); err != nil {
			inode.refs--
			return nil, err
		}
		return f, nil
	}

	// "cache miss", create the inode

	// get direct parent dir handle
	pathOnly, fileName := filepath.Split(fullPath)
	dirHandle, err := fs.getDirectoryHandle(pathOnly, create)
	if err != nil {
		if strings.Contains(err.Error(), "NotFoundError") {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("failed to traverse to dir %q: %w", pathOnly, err)
	}

	// https://developer.mozilla.org/en-US/docs/Web/API/FileSystemDirectoryHandle/getFileHandle
	handle, err := Await(dirHandle.Call("getFileHandle", fileName, map[string]any{"create": create})) // returns Promise<FileSystemFileHandle>
	if err != nil {
		if strings.Contains(err.Error(), "NotFoundError") {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("failed to get file handle: %w", err)
	}

	// cache the inode
	inode := &opfsInode{
		handle: handle,
		path:   fullPath,
		refs:   1,
	}
	inodeCache[fullPath] = inode

	f := &OPFSFile{
		inode:  inode,
		offset: 0,
	}

	if err := fs.applyFlags(f, flag); err != nil {
		return nil, err
	}

	return f, err
}

func (fs *OPFS) Remove(path string) error {
	if path == "" {
		return fmt.Errorf("invalid remove path: %q", path)
	}

	path = normalizePath(path)

	inodeCacheMu.Lock()
	if inode, ok := inodeCache[path]; ok {
		tmpFile := &OPFSFile{inode: inode}
		tmpFile.closeAccess() // ignore error, were removing it anyway
		delete(inodeCache, path)
	}
	inodeCacheMu.Unlock()

	// get direct parent dir handle
	dirPath, name := pathpkg.Split(path)
	dirHandle, err := fs.getDirectoryHandle(dirPath, false)
	if err != nil {
		if strings.Contains(err.Error(), "NotFoundError") {
			return os.ErrNotExist
		}
		return fmt.Errorf("failed to traverse to dir %q: %w", dirPath, err)
	}

	// OPFS FileSystemDirectoryHandle provides a native removeEntry method
	// https://developer.mozilla.org/en-US/docs/Web/API/FileSystemDirectoryHandle/removeEntry
	// a non-empty directory will not be removed
	_, err = Await(dirHandle.Call("removeEntry", name))
	if err == nil {
		return nil // removed ok
	}

	errMsg := err.Error()
	switch {
	case strings.Contains(errMsg, "NotFoundError"):
		return os.ErrNotExist
	case strings.Contains(errMsg, "NoModificationAllowedError"):
		return os.ErrPermission
	default:
		return fmt.Errorf("failed to remove %s: %w", name, err)
	}
}

// Rename copies oldpath to newpath and removes the original because a native
// move operation is not consistently available across browsers.
// TODO: Prefer FileSystemHandle.move when available, retaining copy/remove as the fallback.
// Browser API: https://developer.mozilla.org/en-US/docs/Web/API/File_System_API#api.FileSystemHandle
func (fs *OPFS) Rename(oldpath, newpath string) error {

	src, err := fs.Open(oldpath)
	if err != nil {
		return err
	}
	defer src.Close()

	// create and open new file
	dst, err := fs.Create(newpath)
	if err != nil {
		return err
	}
	defer dst.Close()

	// copy the data from old to new
	if _, err := io.Copy(dst, src); err != nil {
		return err
	}

	// remove the old one
	return fs.Remove(oldpath)
}

// Root returns the filesystem root path.
func (fs *OPFS) Root() string {
	return "/"
}

// Chroot returns a filesystem scoped to path.
func (fs *OPFS) Chroot(path string) (billy.Filesystem, error) {
	return chroot.New(fs, path), nil
}

// ReadDir returns the direct children of path.
func (fs *OPFS) ReadDir(path string) (infos []os.FileInfo, err error) {
	defer func() { // recover any panic that could happen along the way: Get(), Index()
		if r := recover(); r != nil {
			err = fmt.Errorf("OPFS ReadDir %q failed: %+v", path, r)
		}
	}()

	// traverse to the target directory
	dirHandle, err := fs.getDirectoryHandle(path, false)
	if err != nil {
		return nil, err
	}

	// get the AsyncIterator from entries() https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/AsyncIterator
	itValue := dirHandle.Call("entries")
	for {
		// get one entry
		result, err := Await(itValue.Call("next")) // {done, value}
		if err != nil {
			return nil, err
		}

		// if done (last), end loop
		if result.Get("done").Bool() {
			break
		}

		// if not last, ge the value
		pair := result.Get("value") // {name, handle}
		name := pair.Index(0).String()
		handle := pair.Index(1)

		// for directories, mark them as directories
		kind := handle.Get("kind").String() // "file" or "directory"
		dir := kind == "directory"

		// create info for this entry
		fi := &OPFSFileInfo{
			name:  name,
			isDir: dir,
		}

		infos = append(infos, fi)
	}

	return
}

// Lstat returns the same information as Stat because symlinks are unsupported.
func (fs *OPFS) Lstat(filename string) (fs.FileInfo, error) {
	// Lstat() is just Stat(), which doesnt follow links, but we do not have links in OPFS
	return fs.Stat(filename)
}

// TempFile creates a temporary file in dir with the supplied prefix.
func (fs *OPFS) TempFile(dir string, prefix string) (billy.File, error) {
	// generate a unique filename: prefix + timestamp + random
	tempName := fmt.Sprintf("%s%d%d", prefix, time.Now().UnixNano(), rand.Intn(1000))
	fullPath := fs.Join(dir, tempName)

	// ensure the temp directory exists
	if dir != "" && dir != "." {
		_ = fs.MkdirAll(dir, 0o755)
	}

	// use your existing Create method to get a billy.File (OPFSFile)
	return fs.Create(fullPath)
}

// Create creates or truncates name and opens it for reading and writing.
func (fs *OPFS) Create(name string) (billy.File, error) {
	// wrapper around OpenFile()
	return fs.OpenFile(
		name,
		os.O_RDWR|os.O_CREATE|os.O_TRUNC,
		0, // can be whatever, perm gets ignored
	)
}

// Open opens name for reading.
func (fs *OPFS) Open(name string) (billy.File, error) {
	// wrapper around OpenFile() but read-only
	return fs.OpenFile(name, os.O_RDONLY, 0)
}

// Stat returns file information for path. OPFS has no generic stat operation, so it probes for a file first and then a directory.
func (fs *OPFS) Stat(path string) (os.FileInfo, error) {
	// get direct parent dir handle
	path, name := pathpkg.Split(path)
	parentDirHandle, err := fs.getDirectoryHandle(path, false)
	if err != nil {
		if strings.Contains(err.Error(), "NotFoundError") {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("failed to traverse to dir %q: %w", path, err)
	}

	defer func() { // recover any panic
		if r := recover(); r != nil {
			err = fmt.Errorf("OPFS Stat %q failed: %+v", path, r)
		}
	}()

	// try as file first
	// https://developer.mozilla.org/en-US/docs/Web/API/FileSystemDirectoryHandle/getFileHandle
	handle, err := Await(parentDirHandle.Call("getFileHandle", name))
	if err == nil {
		// https://developer.mozilla.org/en-US/docs/Web/API/FileSystemFileHandle/getFile
		file, err := Await(handle.Call("getFile")) // returns Promise<File>
		if err != nil {
			return nil, err
		}
		return &OPFSFileInfo{
			name:    name,
			size:    int64(file.Get("size").Int()),                         // native File(Blob) "size" property
			modTime: time.UnixMilli(int64(file.Get("lastModified").Int())), // native File "lastModified" property
			isDir:   false,
			// https://developer.mozilla.org/en-US/docs/Web/API/File
		}, nil
	}

	// if file failed, try as directory
	_, err = Await(parentDirHandle.Call("getDirectoryHandle", name))
	if err == nil {
		return &OPFSFileInfo{
			name:  name,
			isDir: true,
		}, nil
	}

	// neither file nor directory exists -> ErrNotExist
	if strings.Contains(err.Error(), "NotFoundError") || strings.Contains(err.Error(), "NotFound") {
		return nil, os.ErrNotExist
	}

	return nil, err
}

// Symlink reports billy.ErrNotSupported so callers can apply their own fallback; OPFS has no symbolic-link operation.
func (fs *OPFS) Symlink(target, link string) error {
	return billy.ErrNotSupported
}

// Readlink reports billy.ErrNotSupported so callers can apply their own fallback; OPFS has no symbolic-link operation.
func (fs *OPFS) Readlink(link string) (string, error) {
	return "", billy.ErrNotSupported
}

// ---------------------------------------------------------

// applyFlags applies O_TRUNC and O_APPEND to f.
func (fs *OPFS) applyFlags(f *OPFSFile, flag int) error {
	if flag&os.O_TRUNC != 0 {
		// truncate the file and then return it empty
		if err := f.Truncate(0); err != nil {
			return fmt.Errorf("failed to truncate file: %w", err)
		}
	}

	if flag&os.O_APPEND != 0 {
		// prepare the file for appending
		size := f.inode.access.Call("getSize").Int() // https://developer.mozilla.org/en-US/docs/Web/API/FileSystemSyncAccessHandle/getSize
		f.offset = int64(size)                       // set the offset to the end so that future Write() calls append
	}

	return nil
}

// getDirectoryHandle traverses path and optionally creates missing directories.
func (fs *OPFS) getDirectoryHandle(path string, create bool) (js.Value, error) {
	parts := strings.Split(path, "/")

	dir := fs.RootHandle
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}

		d, err := Await(dir.Call("getDirectoryHandle", part, map[string]any{"create": create}))
		if err != nil {
			return js.Undefined(), err
		}
		dir = d
	}
	return dir, nil
}

// Await bridges a JavaScript Promise into a blocking Go result.
// Rejections use the JavaScript error name and message so callers can identify DOM exceptions.
//
// An example of what this does:
//
//	FileSystemDirectoryHandle.removeEntry(name).then(() => {
//		// something
//	}).catch(() => {
//		// something
//	});
//
// But instead of "something", we pass the value/error to Go.
func Await(p js.Value) (js.Value, error) {
	// create channel for each callback
	valCh := make(chan js.Value, 1)
	errCh := make(chan error, 1)

	// create a callback "then" function
	then := js.FuncOf(func(this js.Value, args []js.Value) any {
		valCh <- args[0]
		return nil
	})

	// create a callback "catch" function
	catch := js.FuncOf(func(this js.Value, args []js.Value) any {
		jsErr := args[0]
		// extract the message and the "name" (e.g., NotFoundError)
		msg := jsErr.Get("message").String()
		name := jsErr.Get("name").String()

		if msg == "" {
			msg = "unknown JS error"
		}

		// we wrap it in a custom struct or just check the name
		errCh <- fmt.Errorf("%s: %s", name, msg)
		return nil
	})

	// call the "p" function with both callbacks
	p.Call("then", then).Call("catch", catch)

	// wait for one of them to finish
	select {
	case v := <-valCh:
		// success, we return the value
		then.Release()
		catch.Release()
		return v, nil
	case err := <-errCh:
		// error, we return an error
		then.Release()
		catch.Release()
		return js.Undefined(), err
	}
}
