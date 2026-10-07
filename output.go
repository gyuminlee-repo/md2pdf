package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func savePDF(path string, data []byte, replaceExisting bool) error {
	return writeOutput(path, replaceExisting, func(f *os.File) error {
		_, err := io.Copy(f, bytes.NewReader(data))
		return err
	})
}

// Stage in the destination directory, then publish only a fully written, synced
// and closed file. On failure the old destination is untouched and the temporary
// file is removed. The callback also lets tests simulate interrupted writes.
func writeOutput(path string, replaceExisting bool, write func(*os.File) error) error {
	var mode os.FileMode
	preserveMode := false
	if info, err := os.Lstat(path); err == nil {
		if !replaceExisting {
			return fmt.Errorf("output already exists: %s: %w", path, os.ErrExist)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("output is not a regular file: %s", path)
		}
		mode = info.Mode().Perm()
		preserveMode = true
	} else if !os.IsNotExist(err) {
		return err
	}

	f, err := os.CreateTemp(filepath.Dir(path), ".md2pdf-output-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err := write(f); err != nil {
		return err
	}
	// New PDFs keep CreateTemp's private permissions (0600, restricted by the
	// umask). Only an explicit replacement inherits the existing file mode.
	if preserveMode {
		if err := f.Chmod(mode); err != nil {
			return err
		}
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return publishOutput(f.Name(), path, replaceExisting)
}
