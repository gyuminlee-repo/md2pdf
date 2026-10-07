package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func assertNoOutputTemps(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".md2pdf-output-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary output left behind: %v, %v", matches, err)
	}
}

func TestWriteOutputFailuresPreserveExistingFile(t *testing.T) {
	for _, tc := range []struct {
		name  string
		write func(*os.File) error
	}{
		{"partial write", func(f *os.File) error { _, _ = f.Write([]byte("partial")); return io.ErrShortWrite }},
		{"closed staging file", func(f *os.File) error { return f.Close() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "output.pdf")
			if err := os.WriteFile(path, []byte("original"), 0644); err != nil {
				t.Fatal(err)
			}
			if err := writeOutput(path, true, tc.write); err == nil {
				t.Fatal("expected write failure")
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != "original" {
				t.Fatalf("original changed: %q, %v", got, err)
			}
			assertNoOutputTemps(t, dir)
		})
	}
}

func TestWriteOutputFailureLeavesNoNewPDF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.pdf")
	if err := writeOutput(path, false, func(f *os.File) error {
		_, _ = f.Write([]byte("partial"))
		return errors.New("disk full")
	}); err == nil {
		t.Fatal("expected failure")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("partial PDF exists: %v", err)
	}
	assertNoOutputTemps(t, dir)
}

func TestWriteOutputPublishRacePreservesWinner(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.pdf")
	if err := writeOutput(path, false, func(f *os.File) error {
		if _, err := f.Write([]byte("our PDF")); err != nil {
			return err
		}
		return os.WriteFile(path, []byte("winner"), 0644)
	}); err == nil {
		t.Fatal("publish overwrote a destination created during staging")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "winner" {
		t.Fatalf("winner changed: %q, %v", got, err)
	}
	assertNoOutputTemps(t, dir)
}

func TestSavePDFConcurrentWritersDoNotClobber(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.pdf")
	var wg sync.WaitGroup
	var successes atomic.Int32
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := savePDF(path, bytes.Repeat([]byte{byte('A' + i)}, 8192), false); err == nil {
				successes.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("successful writers = %d, want 1", successes.Load())
	}
	got, err := os.ReadFile(path)
	if err != nil || len(got) != 8192 || !bytes.Equal(got, bytes.Repeat(got[:1], 8192)) {
		t.Fatalf("incomplete or mixed output: %d bytes, %v", len(got), err)
	}
	assertNoOutputTemps(t, dir)
}

func TestConvertFileStillAllowsExplicitReplacement(t *testing.T) {
	dir := t.TempDir()
	input, output := filepath.Join(dir, "input.md"), filepath.Join(dir, "chosen.pdf")
	if err := os.WriteFile(input, []byte("# Replacement"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, []byte("old PDF"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ConvertFile(input, output, ConvertOptions{Mermaid: MermaidSkip}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(output)
	if err != nil || !bytes.HasPrefix(got, []byte("%PDF-")) {
		t.Fatalf("explicit replacement failed: %v", err)
	}
	assertNoOutputTemps(t, dir)
}

func TestSavePDFRejectsExistingNonRegularDestination(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.pdf")
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	for _, replace := range []bool{false, true} {
		if err := savePDF(path, []byte("pdf"), replace); err == nil {
			t.Fatal("expected directory destination error")
		}
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Fatalf("directory was changed: %v", err)
	}
	assertNoOutputTemps(t, dir)
}

func TestSavePDFRejectsOutputSymlinkWithoutTouchingTarget(t *testing.T) {
	dir := t.TempDir()
	target, output := filepath.Join(dir, "target.pdf"), filepath.Join(dir, "link.pdf")
	if err := os.WriteFile(target, []byte("keep target"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, output); err != nil {
		t.Skip(err)
	}
	for _, replace := range []bool{false, true} {
		if err := savePDF(output, []byte("new PDF"), replace); err == nil {
			t.Fatal("expected symlink destination error")
		}
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "keep target" {
		t.Fatalf("target changed: %q, %v", got, err)
	}
	if _, err := os.Readlink(output); err != nil {
		t.Fatalf("output symlink was replaced: %v", err)
	}
	assertNoOutputTemps(t, dir)
}
