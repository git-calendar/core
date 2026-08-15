// Package export reads and writes filesystem data in portable archive formats.
package export

import (
	"archive/zip"
	"bytes"
	"fmt"
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

// ValidateZip checks that data contains a safe, supported ZIP archive.
func ValidateZip(data []byte) error {
	_, err := validatedZip(data)
	return err
}

// Unzip restores a validated ZIP archive into fs.
func Unzip(fs billy.Filesystem, data []byte) error {
	zr, err := validatedZip(data)
	if err != nil {
		return err
	}

	for _, file := range zr.File {
		name, _ := validZipPath(file.Name)
		if file.FileInfo().IsDir() {
			if err := fs.MkdirAll(name, directoryMode(file.Mode())); err != nil {
				return fmt.Errorf("create directory %q: %w", name, err)
			}
			continue
		}
		if err := extractZipFile(fs, file, name); err != nil {
			return err
		}
	}
	return nil
}

func validatedZip(data []byte) (*zip.Reader, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open ZIP: %w", err)
	}

	seen := make(map[string]struct{}, len(zr.File))
	for _, file := range zr.File {
		name, err := validZipPath(file.Name)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("duplicate ZIP path %q", name)
		}
		seen[name] = struct{}{}
		if !file.FileInfo().IsDir() && !file.Mode().IsRegular() {
			return nil, fmt.Errorf("unsupported ZIP entry %q", file.Name)
		}
	}
	return zr, nil
}

func validZipPath(name string) (string, error) {
	trimmed := strings.TrimSuffix(name, "/")
	cleaned := path.Clean(trimmed)
	if trimmed == "" || cleaned != trimmed || path.IsAbs(cleaned) || cleaned == ".." ||
		strings.HasPrefix(cleaned, "../") || strings.ContainsAny(cleaned, `\:`) {
		return "", fmt.Errorf("invalid ZIP path %q", name)
	}
	return cleaned, nil
}

func extractZipFile(fs billy.Filesystem, entry *zip.File, name string) error {
	if dir := path.Dir(name); dir != "." {
		if err := fs.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create directory %q: %w", dir, err)
		}
	}

	source, err := entry.Open()
	if err != nil {
		return fmt.Errorf("open ZIP entry %q: %w", name, err)
	}
	defer source.Close()

	mode := entry.Mode().Perm()
	if mode == 0 {
		mode = 0o644
	}
	destination, err := fs.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("create restored file %q: %w", name, err)
	}
	defer destination.Close()

	if _, err := io.Copy(destination, source); err != nil {
		return fmt.Errorf("restore file %q: %w", name, err)
	}
	return nil
}

func directoryMode(mode os.FileMode) os.FileMode {
	if mode = mode.Perm(); mode == 0 {
		return 0o755
	}
	return mode
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
