package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MermaidMode controls how ```mermaid fenced blocks are rendered.
type MermaidMode string

const (
	MermaidImage   MermaidMode = "image"   // render via kroki.io as inline PNG
	MermaidSkip    MermaidMode = "skip"    // drop the block entirely
	MermaidCaption MermaidMode = "caption" // replace with a caption stub
)

// MermaidModes lists selectable modes for the GUI.
var MermaidModes = []struct {
	Value MermaidMode
	Label string
}{
	{MermaidImage, "이미지 렌더링 (kroki.io)"},
	{MermaidCaption, "캡션 스텁"},
	{MermaidSkip, "제거"},
}

// mermaidCacheDir returns a PDF-accessible cache directory under baseDir.
func mermaidCacheDir(baseDir string) string {
	return filepath.Join(baseDir, "_md2pdf_cache")
}

// cleanupMermaidCache removes the cache directory created during conversion.
func cleanupMermaidCache(baseDir string) {
	_ = os.RemoveAll(mermaidCacheDir(baseDir))
}

// transformMermaidBlocks rewrites ```mermaid fenced code blocks according to
// the selected mode. For MermaidImage it POSTs each diagram to kroki.io,
// saves the PNG under baseDir/_md2pdf_cache, and references it by relative
// path so goldmark-pdf's image FS can find it. Failures degrade to caption.
func transformMermaidBlocks(mdBytes []byte, mode MermaidMode, baseDir string) []byte {
	if mode == "" {
		mode = MermaidImage
	}

	lines := strings.Split(string(mdBytes), "\n")
	out := make([]string, 0, len(lines))

	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed != "```mermaid" {
			out = append(out, line)
			i++
			continue
		}

		start := i + 1
		end := start
		for end < len(lines) && strings.TrimSpace(lines[end]) != "```" {
			end++
		}
		source := strings.Join(lines[start:end], "\n")

		replacement := renderMermaidBlock(source, mode, baseDir)
		if replacement != "" {
			out = append(out, replacement, "")
		}

		i = end + 1
	}

	return []byte(strings.Join(out, "\n"))
}

func renderMermaidBlock(source string, mode MermaidMode, baseDir string) string {
	switch mode {
	case MermaidSkip:
		return ""
	case MermaidCaption:
		return "> _[Mermaid 다이어그램 — 원본 `.md` 참조]_"
	}

	rel, err := renderMermaidToFile(source, baseDir)
	if err != nil {
		return fmt.Sprintf("> _[Mermaid 렌더링 실패: %v — 원본 `.md` 참조]_", err)
	}
	return "![Mermaid diagram](" + rel + ")"
}

func renderMermaidToFile(source, baseDir string) (string, error) {
	cacheDir := mermaidCacheDir(baseDir)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", fmt.Errorf("cache dir: %w", err)
	}

	sum := sha1.Sum([]byte(source))
	name := hex.EncodeToString(sum[:]) + ".png"
	full := filepath.Join(cacheDir, name)

	if _, err := os.Stat(full); err == nil {
		return "_md2pdf_cache/" + name, nil
	}

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("POST", "https://kroki.io/mermaid/png", bytes.NewReader([]byte(source)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "text/plain")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("kroki %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	f, err := os.Create(full)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", err
	}

	return "_md2pdf_cache/" + name, nil
}
