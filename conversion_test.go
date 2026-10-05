package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestConvertPreservesExistingCache(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "_md2pdf_cache")
	if err := os.Mkdir(cache, 0755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(cache, "user-file.txt")
	if err := os.WriteFile(sentinel, []byte("keep me"), 0644); err != nil {
		t.Fatal(err)
	}
	pdf, err := Convert([]byte("# Synthetic document\n\nHello."), dir, ConvertOptions{Mermaid: MermaidSkip})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatal("not a PDF")
	}
	got, err := os.ReadFile(sentinel)
	if err != nil || string(got) != "keep me" {
		t.Fatalf("existing cache changed: %q, %v", got, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("temporary files left behind: %v", entries)
	}
}

func TestConvertKeepsDistinctAttachmentImages(t *testing.T) {
	dir := t.TempDir()
	for i, folder := range []string{"first", "second"} {
		if err := os.Mkdir(filepath.Join(dir, folder), 0755); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(filepath.Join(dir, folder, "same name.png"))
		if err != nil {
			t.Fatal(err)
		}
		img := image.NewRGBA(image.Rect(0, 0, 2, 2))
		img.Set(0, 0, color.RGBA{R: uint8(100 + i*100), A: 255})
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	pdf, err := Convert([]byte("![[first/same name.png]]\n\n![[second/same name.png]]"), dir, ConvertOptions{Mermaid: MermaidSkip})
	if err != nil {
		t.Fatal(err)
	}
	if got := bytes.Count(pdf, []byte("/Subtype /Image")); got != 4 { // two RGB images plus two alpha masks
		t.Fatalf("expected two distinct images and alpha masks, got %d image objects", got)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("conversion left temporary files: %v", entries)
	}
}

func TestConvertFileRejectsInputOutputAliases(t *testing.T) {
	for _, kind := range []string{"same", "cleaned", "hardlink", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "input.md")
			source := []byte("# Synthetic source\n")
			if err := os.WriteFile(input, source, 0644); err != nil {
				t.Fatal(err)
			}
			output := input
			switch kind {
			case "cleaned":
				output = dir + string(os.PathSeparator) + "." + string(os.PathSeparator) + "input.md"
			case "hardlink":
				output = filepath.Join(dir, "output.pdf")
				if err := os.Link(input, output); err != nil {
					t.Skip(err)
				}
			case "symlink":
				output = filepath.Join(dir, "output.pdf")
				if err := os.Symlink(input, output); err != nil {
					t.Skip(err)
				}
			}
			if err := ConvertFile(input, output, ConvertOptions{Mermaid: MermaidSkip}); err == nil {
				t.Error("expected an input/output alias error")
			}
			got, err := os.ReadFile(input)
			if err != nil || !bytes.Equal(got, source) {
				t.Fatalf("source was changed: %v", err)
			}
		})
	}
}

func TestConvertEmptyBaseResolvesAttachments(t *testing.T) {
	// No tests run in parallel: cwd and the offline HTTP transport are process-wide.
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(old); err != nil {
			t.Fatal(err)
		}
	}()
	f, err := os.Create("image.png")
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	pdf, err := Convert([]byte("![[image.png]]"), "", ConvertOptions{Mermaid: MermaidSkip})
	if err != nil {
		t.Fatal(err)
	}
	if got := bytes.Count(pdf, []byte("/Subtype /Image")); got != 2 {
		t.Fatalf("expected image plus alpha mask, got %d", got)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("temporary files left behind: %v", entries)
	}
}
