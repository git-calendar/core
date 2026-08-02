// Package export writes filesystem data to portable archive formats.
package export

import (
	"archive/zip"
	"io"
	"os"
	"path"
	"strings"

	"github.com/go-git/go-billy/v5"
)

// Zip writes all files from fs to a ZIP archive on w.
func Zip(fs billy.Filesystem, w io.Writer) error {
	zw := zip.NewWriter(w)

	entries, err := listAllFiles(fs, "/")
	if err != nil {
		_ = zw.Close()
		return err
	}

	for _, entry := range entries {
		if err := addFileToZip(fs, zw, entry); err != nil {
			_ = zw.Close()
			return err
		}
	}

	return zw.Close()
}

type fileEntry struct {
	Path string
	Info os.FileInfo
}

func listAllFiles(fs billy.Filesystem, root string) ([]fileEntry, error) {
	var files []fileEntry

	entries, err := fs.ReadDir(root)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		fullPath := path.Join(root, entry.Name())

		if entry.IsDir() {
			childFiles, err := listAllFiles(fs, fullPath)
			if err != nil {
				return nil, err
			}

			files = append(files, childFiles...)
			continue
		}

		files = append(files, fileEntry{
			Path: fullPath,
			Info: entry,
		})
	}

	return files, nil
}

func addFileToZip(fs billy.Filesystem, zw *zip.Writer, entry fileEntry) error {
	file, err := fs.Open(entry.Path)
	if err != nil {
		return err
	}
	defer file.Close()

	header, err := zip.FileInfoHeader(entry.Info)
	if err != nil {
		return err
	}

	header.Name = strings.TrimPrefix(path.Clean(entry.Path), "/")
	header.Method = zip.Deflate

	writer, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}

	_, err = io.Copy(writer, file)
	return err
}
