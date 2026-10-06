package main

import (
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Every test is offline, including any accidentally introduced network path.
func TestMain(m *testing.M) {
	http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network disabled in tests")
	})
	os.Exit(m.Run())
}

func TestConversionCachesAreIsolated(t *testing.T) {
	base := t.TempDir()
	a, b := &conversionCache{baseDir: base}, &conversionCache{baseDir: base}
	defer a.cleanup()
	defer b.cleanup()
	if err := a.ensure(); err != nil {
		t.Fatal(err)
	}
	if err := b.ensure(); err != nil {
		t.Fatal(err)
	}
	if a.dir == b.dir {
		t.Fatal("conversions share a cache")
	}
	if err := os.WriteFile(filepath.Join(b.dir, "keep"), []byte("other conversion"), 0600); err != nil {
		t.Fatal(err)
	}
	a.cleanup()
	if _, err := os.Stat(filepath.Join(b.dir, "keep")); err != nil {
		t.Fatal(err)
	}
}

func TestAttachmentCacheNamesAvoidCollisions(t *testing.T) {
	base := t.TempDir()
	cache := &conversionCache{baseDir: base}
	defer cache.cleanup()
	seen := map[string]bool{}
	for _, name := range []string{"space name.png", "space_name.png", "image).png", "image[1].png"} {
		src := filepath.Join(base, name)
		if err := os.WriteFile(src, []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
		rel, err := cache.copyAttachment(src)
		if err != nil {
			t.Fatal(err)
		}
		if seen[rel] {
			t.Fatalf("duplicate path: %s", rel)
		}
		seen[rel] = true
		if strings.ContainsAny(rel, " ()[]") {
			t.Fatalf("unsafe Markdown path: %s", rel)
		}
		got, err := os.ReadFile(filepath.Join(base, filepath.FromSlash(rel)))
		if err != nil || string(got) != name {
			t.Fatalf("wrong cached content: %q, %v", got, err)
		}
	}
}

type brokenReader struct{}

func (brokenReader) Read(p []byte) (int, error) {
	copy(p, "partial")
	return len("partial"), errors.New("synthetic read failure")
}

func TestMermaidFailedDownloadIsNotCached(t *testing.T) {
	old := http.DefaultTransport
	defer func() { http.DefaultTransport = old }()
	calls := 0
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "POST" || r.URL.String() != "https://kroki.io/mermaid/png" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != "graph TD; syntheticA-->syntheticB" {
			t.Fatalf("unexpected synthetic source: %q, %v", body, err)
		}
		var response io.Reader = strings.NewReader("synthetic image response")
		if calls == 1 {
			response = brokenReader{}
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(response), Header: make(http.Header)}, nil
	})
	cache := &conversionCache{baseDir: t.TempDir()}
	defer cache.cleanup()
	source := "graph TD; syntheticA-->syntheticB"
	if _, err := renderMermaidToFile(source, cache); err == nil {
		t.Fatal("expected synthetic read failure")
	}
	entries, err := os.ReadDir(cache.dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatal("partial response cached")
	}
	rel, err := renderMermaidToFile(source, cache)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(cache.baseDir, filepath.FromSlash(rel)))
	if err != nil || string(got) != "synthetic image response" {
		t.Fatalf("bad retry: %q, %v", got, err)
	}
	if _, err := renderMermaidToFile(source, cache); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("expected failed request, retry, then cache hit; got %d requests", calls)
	}
}

func TestVaultIndexSkipsConversionCaches(t *testing.T) {
	base := t.TempDir()
	for _, dir := range []string{"_md2pdf_cache", "_md2pdf_cache-123", "images"} {
		if err := os.Mkdir(filepath.Join(base, dir), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(base, dir, "same.png"), []byte(dir), 0600); err != nil {
			t.Fatal(err)
		}
	}
	idx := newAttachmentIndex(base, base)
	idx.build()
	if got := idx.byBase["same.png"]; len(got) != 1 || got[0] != filepath.Join(base, "images", "same.png") {
		t.Fatalf("index included temporary images: %v", got)
	}
}

func TestConvertFileWritesPDFAndReportsOutputFailure(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input.md")
	if err := os.WriteFile(input, []byte("# Synthetic document\n\n![[image.png]]"), 0600); err != nil {
		t.Fatal(err)
	}
	img, err := os.Create(filepath.Join(dir, "image.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(img, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	if err := img.Close(); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "output.pdf")
	if err := ConvertFile(input, output, ConvertOptions{Mermaid: MermaidSkip}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(output)
	if err != nil || !strings.HasPrefix(string(got), "%PDF-") {
		t.Fatalf("bad output: %v", err)
	}
	if err := ConvertFile(input, dir, ConvertOptions{Mermaid: MermaidSkip}); err == nil {
		t.Fatal("expected directory output error")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("output failure left temporary files: %v", entries)
	}
}
