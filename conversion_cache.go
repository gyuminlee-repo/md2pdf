package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// conversionCache owns only the private directory it creates. Keeping it under
// baseDir lets the PDF image filesystem resolve paths without changing local
// Markdown image semantics. Creation is lazy, so text-only documents need no
// write access to their source directory.
type conversionCache struct {
	baseDir string
	dir     string
}

func (c *conversionCache) ensure() error {
	if c.dir != "" {
		return nil
	}
	baseDir := c.baseDir
	if baseDir == "" {
		baseDir = "."
	} // Match http.Dir("") and local image resolution.
	dir, err := os.MkdirTemp(baseDir, "_md2pdf_cache-*")
	if err != nil {
		return err
	}
	c.dir = dir
	return nil
}

func (c *conversionCache) cleanup() {
	if c.dir != "" {
		_ = os.RemoveAll(c.dir)
	}
}

func (c *conversionCache) relative(name string) string {
	return filepath.ToSlash(filepath.Join(filepath.Base(c.dir), name))
}

func (c *conversionCache) copyAttachment(src string) (string, error) {
	if err := c.ensure(); err != nil {
		return "", err
	}
	absolute, err := filepath.Abs(src)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(absolute))
	name := hex.EncodeToString(sum[:]) + strings.ToLower(filepath.Ext(src))
	dst := filepath.Join(c.dir, name)
	if !fileExists(dst) {
		in, err := os.Open(src)
		if err != nil {
			return "", err
		}
		defer in.Close()
		if err := writeCacheFile(dst, in); err != nil {
			return "", err
		}
	}
	return c.relative(name), nil
}

// Failed reads/writes must not leave a truncated file that later looks cached.
func writeCacheFile(path string, src io.Reader) (err error) {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := out.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(path)
		}
	}()
	_, err = io.Copy(out, src)
	return err
}
