//go:build js && wasm

// Package indexeddb implements the billy.Filesystem interface backed by IndexedDB store
// It is only usable in a js/wasm build targeting a browser environment.
// https://developer.mozilla.org/en-US/docs/Web/API/IndexedDB_API
package idb

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"syscall/js"
	"time"

	"github.com/go-git/go-billy/v5"
)

type IndexedDB struct {
	name string
	jsDB js.Value
	root string
}

const (
	contentStoreName = "content"
	infoStoreName    = "info"
)

func New(name string, version int) (*IndexedDB, error) {
	jsIdb := js.Global().Get("indexedDB")
	if jsIdb.IsUndefined() {
		return nil, fmt.Errorf("indexedDB not supported")
	}
	req := jsIdb.Call("open", name, version)

	dbValue, err := awaitOpen(req, func(db js.Value) {
		storeNames := db.Get("objectStoreNames")
		if !storeNames.Call("contains", contentStoreName).Bool() {
			db.Call("createObjectStore", contentStoreName) // create "content" store
		}
		if !storeNames.Call("contains", infoStoreName).Bool() {
			db.Call("createObjectStore", infoStoreName) // create "info" store
		}
	})
	if err != nil {
		return nil, err
	}

	idb := IndexedDB{
		name: name,
		jsDB: dbValue,
		root: "/",
	}

	// check if root exists
	tx := NewTx()
	rootReq := tx.Get(infoStoreName, "/")
	_ = tx.Commit(idb.jsDB)

	if !rootReq.Result().Truthy() {
		info := IDBFileInfo{
			name:    "/",
			modTime: time.Now(),
			mode:    os.ModeDir | 0o755,
		}

		txInit := NewTx()
		txInit.Put(infoStoreName, "/", info.toJS())
		if err := txInit.Commit(idb.jsDB); err != nil {
			return nil, err
		}
	}

	return &idb, nil
}

func (idb *IndexedDB) Create(filename string) (billy.File, error) {
	key := idb.absolutePath(filename)
	fileInfo := IDBFileInfo{
		name:    path.Base(filename),
		size:    0,
		modTime: time.Now(),
		mode:    0o666,
	}

	tx := NewTx()
	tx.Put(infoStoreName, key, fileInfo.toJS())
	if err := tx.Commit(idb.jsDB); err != nil {
		return nil, fmt.Errorf("failed to create file %s in idb: %w", filename, err)
	}

	return &IDBFile{
		fs:      idb,
		key:     key,
		relPath: filename,
		offset:  0,
	}, nil
}

func (idb *IndexedDB) Open(filename string) (billy.File, error) {
	key := idb.absolutePath(filename)

	info, err := idb.Stat(filename)
	exists := err == nil
	if !exists {
		return nil, os.ErrNotExist
	}
	if info.IsDir() {
		return nil, fmt.Errorf("cannot open directory: %s", filename)
	}

	return &IDBFile{
		fs:      idb,
		key:     key,
		relPath: filename,
		offset:  0,
	}, nil
}

func (idb *IndexedDB) OpenFile(filename string, flag int, perm os.FileMode) (billy.File, error) {
	key := idb.absolutePath(filename)
	create := flag&os.O_CREATE != 0

	info, err := idb.Stat(filename)
	exists := err == nil

	if exists && info.IsDir() {
		return nil, fmt.Errorf("cannot open directory: %s", filename)
	}

	if !exists && !create {
		return nil, os.ErrNotExist
	}

	if !exists && create {
		// check if parent exists
		if _, err := idb.Stat(path.Dir(filename)); err != nil {
			return nil, err
		}

		// create new file
		newInfo := IDBFileInfo{
			name:    path.Base(filename),
			modTime: time.Now(),
			mode:    perm,
		}

		tx := NewTx()
		tx.Put(infoStoreName, key, newInfo.toJS())
		if err := tx.Commit(idb.jsDB); err != nil {
			return nil, err
		}
	}

	file := IDBFile{
		fs:      idb,
		key:     key,
		relPath: filename,
		offset:  0,
	}
	if err := idb.applyFlags(&file, flag); err != nil {
		return nil, err
	}
	return &file, nil
}

func (idb *IndexedDB) Stat(name string) (os.FileInfo, error) {
	tx := NewTx()
	req := tx.Get(infoStoreName, idb.absolutePath(name))
	if err := tx.Commit(idb.jsDB); err != nil {
		return nil, err
	}

	result := req.Result()
	if !result.Truthy() {
		return nil, os.ErrNotExist
	}

	return FileInfoFromJS(result), nil
}

func (idb *IndexedDB) Rename(oldpath, newpath string) error {
	// check source exists
	info, err := idb.Stat(oldpath)
	if err != nil {
		return err
	}

	// fail if target exists
	if _, err := idb.Stat(newpath); err == nil {
		return errors.New("target already exists")
	} else if !os.IsNotExist(err) {
		return err
	}

	// call one of the helpers
	if info.IsDir() {
		return idb.renameDir(oldpath, newpath)
	}
	return idb.renameFile(oldpath, newpath)
}

func (idb *IndexedDB) Remove(name string) error {
	info, err := idb.Stat(name)
	if err != nil {
		return os.ErrNotExist
	}

	fullpath := idb.absolutePath(name)

	if !info.IsDir() {
		tx := NewTx()
		tx.Delete(infoStoreName, fullpath)
		tx.Delete(contentStoreName, fullpath)
		if err := tx.Commit(idb.jsDB); err != nil {
			return err
		}
	} else {
		entries, err := idb.ReadDir(name)
		if err != nil {
			return err
		}
		if len(entries) != 0 {
			return errors.New("not empty")
		}

		tx := NewTx()
		tx.Delete(infoStoreName, fullpath)
		if err := tx.Commit(idb.jsDB); err != nil {
			return err
		}
	}

	return nil
}

func (idb *IndexedDB) Join(elem ...string) string {
	return path.Join(elem...)
}

func (idb *IndexedDB) MkdirAll(p string, perm os.FileMode) error {
	parts := strings.Split(path.Clean(p), "/")
	var currentPath string = idb.root

	tx := NewTx()

	for _, part := range parts {
		if part == "." || part == "" {
			continue
		}

		currentPath = idb.Join(currentPath, part)
		info := IDBFileInfo{
			name:    part,
			modTime: time.Now(),
			mode:    os.ModeDir | perm,
		}

		tx.Put(infoStoreName, currentPath, info.toJS())
	}

	return tx.Commit(idb.jsDB)
}

func (idb *IndexedDB) ReadDir(p string) ([]os.FileInfo, error) {
	fullpath := idb.absolutePath(p)
	prefix := strings.TrimSuffix(fullpath, "/") + "/"

	rangeObj := js.Global().Get("IDBKeyRange").Call(
		"bound",
		fullpath,
		fullpath+"\uffff",
	)

	txKeys := NewTx()
	keysReq := txKeys.GetAllKeys(infoStoreName, rangeObj)
	if err := txKeys.Commit(idb.jsDB); err != nil {
		return nil, err
	}

	result := keysReq.Result()
	if result.IsNull() || result.IsUndefined() {
		return nil, os.ErrNotExist
	}

	length := result.Length()
	keys := make([]string, 0, length)
	for idx := range length {
		key := result.Index(idx).String()
		if key == fullpath {
			continue
		}
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		rest := key[len(prefix):]
		if strings.Contains(rest, "/") {
			continue
		}

		keys = append(keys, key)
	}

	txInfos := NewTx()
	infoReqs := make([]interface{ Result() js.Value }, len(keys))

	for idx, key := range keys {
		infoReqs[idx] = txInfos.Get(infoStoreName, key)
	}

	if err := txInfos.Commit(idb.jsDB); err != nil {
		return nil, err
	}

	files := make([]os.FileInfo, 0, len(keys))
	for idx := range keys {
		files = append(files, FileInfoFromJS(infoReqs[idx].Result()))
	}

	return files, nil
}

func (idb *IndexedDB) TempFile(dir, prefix string) (billy.File, error) {
	filename := prefix + randString(10)
	return idb.Create(idb.Join(dir, filename))
}

func (idb *IndexedDB) Lstat(filename string) (os.FileInfo, error) {
	return idb.Stat(filename) // Lstat is same as Stat, but if the file is a symbolic link, it doesn't resolve it
}

func (idb *IndexedDB) Symlink(target, link string) error {
	return billy.ErrNotSupported
}

func (idb *IndexedDB) Readlink(link string) (string, error) {
	return "", billy.ErrNotSupported
}

func (idb *IndexedDB) Chroot(path string) (billy.Filesystem, error) {
	// make sure it exists
	if err := idb.MkdirAll(path, 0o777); err != nil {
		return nil, err
	}
	// return a whole new instance
	return &IndexedDB{
		name: idb.name,
		jsDB: idb.jsDB,
		root: idb.absolutePath(path),
	}, nil
}

func (idb *IndexedDB) Root() string {
	return idb.root
}

// ----- HELPERS -----

func (idb *IndexedDB) renameFile(oldpath, newpath string) error {
	txRead := NewTx()

	oldFull := idb.absolutePath(oldpath)
	newFull := idb.absolutePath(newpath)
	if oldFull == newFull {
		return nil
	}

	oldInfo := txRead.Get(infoStoreName, oldFull)
	oldContent := txRead.Get(contentStoreName, oldFull)
	newInfo := txRead.Get(infoStoreName, newFull)

	// execute reads
	if err := txRead.Commit(idb.jsDB); err != nil {
		return fmt.Errorf("rename failed during read: %w", err)
	}

	if !oldInfo.Result().Truthy() {
		return os.ErrNotExist
	}
	if newInfo.Result().Truthy() {
		return os.ErrExist
	}

	info := FileInfoFromJS(oldInfo.Result())
	info.name = path.Base(newpath)
	info.modTime = time.Now()

	txWrite := NewTx()

	txWrite.Put(infoStoreName, newFull, info.toJS())
	txWrite.Put(contentStoreName, newFull, oldContent.Result())
	txWrite.Delete(infoStoreName, oldFull)
	txWrite.Delete(contentStoreName, oldFull)

	if err := txWrite.Commit(idb.jsDB); err != nil {
		return fmt.Errorf("rename failed during write: %w", err)
	}

	return nil
}

func (idb *IndexedDB) renameDir(oldpath, newpath string) error {
	oldFull := idb.absolutePath(oldpath)
	newFull := idb.absolutePath(newpath)

	// create new dir
	txRead := NewTx()
	oldInfoReq := txRead.Get(infoStoreName, oldFull)
	if err := txRead.Commit(idb.jsDB); err != nil {
		return err
	}

	txWrite := NewTx()
	txWrite.Put(infoStoreName, newFull, oldInfoReq.Result())
	if err := txWrite.Commit(idb.jsDB); err != nil {
		return err
	}

	// list children
	children, err := idb.ReadDir(oldpath)
	if err != nil {
		return err
	}

	for _, child := range children {
		oldChild := path.Join(oldFull, child.Name())
		newChild := path.Join(newFull, child.Name())

		if child.IsDir() {
			if err := idb.renameDir(oldChild, newChild); err != nil {
				return err
			}
		} else {
			if err := idb.renameFile(oldChild, newChild); err != nil {
				return err
			}
		}
	}

	// remove old dir (now empty)
	return idb.Remove(oldpath)
}

// Applies the O_TRUNC and O_APPEND flags to a file.
func (idb *IndexedDB) applyFlags(f *IDBFile, flag int) error {
	if flag&os.O_TRUNC != 0 {
		// truncate the file and then return it empty
		if err := f.Truncate(0); err != nil {
			return fmt.Errorf("failed to truncate file: %w", err)
		}
	}

	if flag&os.O_APPEND != 0 {
		// prepare the file for appending
		info, err := idb.Stat(f.key)
		if err != nil {
			return fmt.Errorf("failed to stat file: %w", err)
		}
		f.offset = info.Size() // set the offset to the end so that future Write() calls append
	}

	return nil
}

func (idb *IndexedDB) absolutePath(p string) string {
	p = path.Clean(p)
	return path.Join(idb.root, p)
}
