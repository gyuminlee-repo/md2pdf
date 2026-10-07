//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOutputPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.pdf")
	if err := savePDF(path, []byte("new PDF"), false); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm()&0077 != 0 {
		t.Fatalf("new PDF must remain private: %v, %v", info, err)
	}
	if err := os.Chmod(path, 0640); err != nil {
		t.Fatal(err)
	}
	if err := savePDF(path, []byte("replacement PDF"), true); err != nil {
		t.Fatal(err)
	}
	info, err = os.Stat(path)
	if err != nil || info.Mode().Perm() != 0640 {
		t.Fatalf("replacement changed permissions: %v, %v", info, err)
	}
}
