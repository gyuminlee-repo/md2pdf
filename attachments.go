package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

var embedImageRe = regexp.MustCompile(`!\[\[([^\[\]]+)\]\]`)

var imageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".webp": true, ".bmp": true,
}

// resolveAttachments rewrites Obsidian-style image embeds `![[name.png]]` into
// standard markdown image links by searching the enclosing
// vault for the file and copying it into the private conversion cache directory.
// Also rewrites `![alt](path/with spaces.png)` entries whose target file is
// missing relative to baseDir, searching the vault for the basename.
// Non-image wikilinks are left for simplifyWikilinks to handle.
func resolveAttachments(mdBytes []byte, baseDir string, cache *conversionCache) []byte {
	vaultRoot := findVaultRoot(baseDir)
	idx := newAttachmentIndex(vaultRoot, baseDir)

	lines := strings.Split(string(mdBytes), "\n")
	inFence := false
	out := make([]string, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			out = append(out, line)
			continue
		}
		if inFence {
			out = append(out, line)
			continue
		}

		standalone := embedImageRe.MatchString(strings.TrimSpace(line)) &&
			strings.TrimSpace(embedImageRe.ReplaceAllString(line, "")) == ""

		replaced := embedImageRe.ReplaceAllStringFunc(line, func(match string) string {
			inner := match[3 : len(match)-2]
			name, alias := splitPipe(inner)
			if !isImageRef(name) {
				return match // leave non-image embeds for wikilink simplifier
			}
			rel, err := idx.materialize(name, cache)
			if err != nil {
				return "_[이미지 없음: " + name + "]_"
			}
			if alias == "" {
				alias = filepath.Base(name)
			}
			return "![" + alias + "](" + rel + ")"
		})

		if standalone {
			if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
				out = append(out, "")
			}
			out = append(out, replaced, "")
		} else {
			out = append(out, replaced)
		}
	}

	return []byte(strings.Join(out, "\n"))
}

func splitPipe(s string) (name, alias string) {
	if i := strings.Index(s, "|"); i >= 0 {
		return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:])
	}
	return strings.TrimSpace(s), ""
}

func isImageRef(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return imageExts[ext]
}

// findVaultRoot walks up from startDir looking for a `.obsidian` folder.
// Returns "" if none is found.
func findVaultRoot(startDir string) string {
	dir := startDir
	for {
		if st, err := os.Stat(filepath.Join(dir, ".obsidian")); err == nil && st.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// attachmentIndex lazily indexes image files under the vault root by basename.
type attachmentIndex struct {
	vault  string
	base   string
	once   sync.Once
	byBase map[string][]string
}

func newAttachmentIndex(vault, base string) *attachmentIndex {
	return &attachmentIndex{vault: vault, base: base}
}

func (a *attachmentIndex) build() {
	a.byBase = make(map[string][]string)
	if a.vault == "" {
		return
	}
	_ = filepath.Walk(a.vault, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if info.Name() == "_md2pdf_cache" || strings.HasPrefix(info.Name(), "_md2pdf_cache-") {
				return filepath.SkipDir
			}
			return nil
		}
		if !isImageRef(info.Name()) {
			return nil
		}
		key := strings.ToLower(info.Name())
		a.byBase[key] = append(a.byBase[key], path)
		return nil
	})
}

// materialize copies the resolved source image into the private cache
// and returns the forward-slash relative path suitable for markdown.
func (a *attachmentIndex) materialize(name string, cache *conversionCache) (string, error) {
	// 1. try relative to baseDir first (fastest, matches commonmark semantics)
	if src := filepath.Join(a.base, name); fileExists(src) {
		return cache.copyAttachment(src)
	}

	// 2. fall back to vault-wide basename search
	a.once.Do(a.build)
	base := filepath.Base(name)
	matches := a.byBase[strings.ToLower(base)]
	if len(matches) == 0 {
		return "", fmt.Errorf("not found: %s", name)
	}
	return cache.copyAttachment(matches[0])
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
