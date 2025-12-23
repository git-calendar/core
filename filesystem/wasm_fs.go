//go:build js && wasm

package filesystem

import (
	"fmt"
	"io"
	"io/fs"
	"math/rand"
	"os"
	"strings"
	"syscall/js"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/helper/chroot"
)

func GetRepoFS(dirName string) (billy.Filesystem, error) {
	rootHandle := js.Global().Get("opfsRootHandle")

	return &OPFS{
		root:    rootHandle,
		dirName: dirName,
	}, nil
}

// Origin private file system
//
// https://developer.mozilla.org/en-US/docs/Web/API/File_System_API/Origin_private_file_system
type OPFS struct {
	root    js.Value // FileSystemDirectoryHandle
	dirName string
}

var _ billy.Filesystem = (*OPFS)(nil) // makes sure that it implements all the interface methods, it wont compile without it

func (fs *OPFS) MkdirAll(path string, perm fs.FileMode) error {
	// OPFS ignores permissions (perm)

	_, err := fs.getDirectoryHandle(path, true)
	return err
}

func (fs *OPFS) Join(elem ...string) string {
	var parts []string
	for _, e := range elem { // filter empty
		if e != "" {
			parts = append(parts, e)
		}
	}
	return strings.Join(parts, "/")
}

func (fs *OPFS) OpenFile(name string, flag int, perm os.FileMode) (billy.File, error) {
	// OPFS ignores permissions (perm)

	create := flag&os.O_CREATE != 0

	var handle js.Value
	var err error

	defer func() { // recover any panic that could happen along the way: Get(), Index()
		if r := recover(); r != nil {
			err = fmt.Errorf("OPFS OpenFile %q failed: %+v", name, r)
		}
	}()

	// https://developer.mozilla.org/en-US/docs/Web/API/FileSystemDirectoryHandle/getFileHandle
	handle, err = await(fs.root.Call("getFileHandle", name, map[string]any{"create": create})) // returns Promise<FileSystemFileHandle>

	f := &OPFSFile{
		handle: handle,
		offset: 0,
	}

	// handle O_TRUNC
	if flag&os.O_TRUNC != 0 {
		err = f.Truncate(0)
	}

	// handle O_APPEND
	if flag&os.O_APPEND != 0 {
		// prepare the file for appending
		f.openAccess()
		size := f.access.Call("getSize").Int() // https://developer.mozilla.org/en-US/docs/Web/API/FileSystemSyncAccessHandle/getSize
		f.offset = int64(size)                 // set the offset to the end so that future Write() calls append
	}

	return f, err
}

func (fs *OPFS) Remove(filename string) error {
	// OPFS FileSystemDirectoryHandle provides a native removeEntry method
	// https://developer.mozilla.org/en-US/docs/Web/API/FileSystemDirectoryHandle/removeEntry
	// a non-empty directory will not be removed
	_, err := await(fs.root.Call("removeEntry", filename))
	return err
}

func (fs *OPFS) Rename(oldpath, newpath string) error {
	// "rename" isnt a thing in OPFS
	// https://developer.mozilla.org/en-US/docs/Web/API/File_System_API#api.FileSystemHandle

	// try "move" if the browser supports it (Firefox and Safari as of 2025)
	// TODO

	// ------- copying workaround -------
	// open file
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

func (fs *OPFS) Root() string {
	return fs.dirName
}

func (fs *OPFS) Chroot(path string) (billy.Filesystem, error) {
	return chroot.New(fs, path), nil
}

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
	itValue, err := await(dirHandle.Call("entries"))
	if err != nil {
		return nil, err
	}

	// the JS AsyncIterator has a .next() -> {done, value}
	for {
		// get one entry
		result, err := await(itValue.Call("next")) // {done, value}
		if err != nil {
			return nil, err
		}

		// if done (last), end loop
		done := result.Get("done").Bool()
		if done {
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

func (s *OPFS) Lstat(filename string) (fs.FileInfo, error) {
	// Lstat() is just Stat(), which doesnt follow links, but we do not have links in OPFS
	return s.Stat(filename)
}

func (s *OPFS) TempFile(dir string, prefix string) (billy.File, error) {
	// generate a unique filename: prefix + timestamp + random
	tempName := fmt.Sprintf("%s%d%d", prefix, time.Now().UnixNano(), rand.Intn(1000))
	fullPath := s.Join(dir, tempName)

	// ensure the temp directory exists
	if dir != "" && dir != "." {
		_ = s.MkdirAll(dir, 0o755)
	}

	// use your existing Create method to get a billy.File (OPFSFile)
	return s.Create(fullPath)
}

func (fs *OPFS) Create(name string) (billy.File, error) {
	// wrapper around OpenFile()
	return fs.OpenFile(
		name,
		os.O_RDWR|os.O_CREATE|os.O_TRUNC,
		0, // can be whatever, perm gets ignored
	)
}

func (fs *OPFS) Open(name string) (billy.File, error) {
	// wrapper around OpenFile() but read only
	return fs.OpenFile(name, os.O_RDONLY, 0)
}

func (fs *OPFS) Stat(name string) (os.FileInfo, error) {
	// https://developer.mozilla.org/en-US/docs/Web/API/FileSystemDirectoryHandle/getFileHandle
	handle, err := await(fs.root.Call("getFileHandle", name)) // returns Promise<FileSystemFileHandle>
	if err != nil {
		return nil, err
	}

	// https://developer.mozilla.org/en-US/docs/Web/API/FileSystemFileHandle/getFile
	file, err := await(handle.Call("getFile")) // returns Promise<File>
	if err != nil {
		return nil, err
	}

	return &OPFSFileInfo{
		name:    file.Get("name").String(),                             // native File "name" property
		size:    int64(file.Get("size").Int()),                         // native File(Blob) "size" property
		modTime: time.UnixMilli(int64(file.Get("lastModified").Int())), // native File "lastModified" property
		// https://developer.mozilla.org/en-US/docs/Web/API/File
	}, nil
}

func (s *OPFS) Symlink(target, link string) error {
	return billy.ErrNotSupported // go-git will probably handle this
}

func (s *OPFS) Readlink(link string) (string, error) {
	return "", billy.ErrNotSupported // go-git will probably handle this
}

func (fs *OPFS) getDirectoryHandle(path string, create bool) (js.Value, error) {
	parts := strings.Split(path, "/")

	dir := fs.root
	for _, part := range parts {
		if part == "" {
			continue
		}
		d, err := await(dir.Call("getDirectoryHandle", part, map[string]any{"create": create}))
		if err != nil {
			return js.Undefined(), err
		}
		dir = d
	}
	return dir, nil
}

// A helper function which makes async calls to JS API synchronous.
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
func await(p js.Value) (js.Value, error) {
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
		errCh <- fmt.Errorf("js error: %s", args[0].String())
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
