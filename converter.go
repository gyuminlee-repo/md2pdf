package main

import (
	"bytes"
	"context"
	"fmt"
	"image/color"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/go-swiss/fonts"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"

	gpdf "github.com/stephenafamo/goldmark-pdf"
)

// ConvertFile reads a markdown file and writes a PDF to the output path.
func ConvertFile(inputPath, outputPath, theme string) error {
	mdBytes, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}

	baseDir, err := filepath.Abs(filepath.Dir(inputPath))
	if err != nil {
		return fmt.Errorf("resolving base dir: %w", err)
	}

	pdfBytes, err := Convert(mdBytes, baseDir, theme)
	if err != nil {
		return err
	}

	if err := os.WriteFile(outputPath, pdfBytes, 0644); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}

	return nil
}

// Convert takes markdown bytes and returns PDF bytes.
func Convert(mdBytes []byte, baseDir, theme string) ([]byte, error) {
	ctx := context.Background()
	mdBytes = normalizePlainTextArrows(mdBytes)

	// Create PDF with footer (page number)
	fpdfObj := gpdf.NewFpdf(ctx, gpdf.FpdfConfig{
		Orientation: "Portrait",
		PaperSize:   "A4",
		FooterFunc:  pageFooter,
	}, nil)

	// Set page margins: left 20mm, top 18mm, right 20mm
	fpdfObj.Fpdf.SetMargins(
		mmToPt(20), // left
		mmToPt(18), // top
		mmToPt(20), // right
	)
	fpdfObj.Fpdf.SetAutoPageBreak(true, mmToPt(22)) // bottom margin 22mm

	// Register custom fonts
	textFont, codeFont, err := registerFonts(fpdfObj)
	if err != nil {
		return nil, fmt.Errorf("loading fonts: %w", err)
	}

	// Build styles
	st := pdfStyles(textFont, codeFont, theme)

	// Configure renderer
	renderer := gpdf.New(
		gpdf.WithPDF(fpdfObj),
		gpdf.WithImageFS(http.Dir(baseDir)),
		gpdf.WithContext(ctx),
		gpdf.OptionFunc(func(c *gpdf.Config) {
			c.Styles = st
		}),
	)

	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRenderer(renderer),
	)

	var buf bytes.Buffer
	if err := md.Convert(mdBytes, &buf); err != nil {
		return nil, fmt.Errorf("converting markdown: %w", err)
	}

	return buf.Bytes(), nil
}

// normalizePlainTextArrows replaces ASCII flow arrows with a Unicode arrow in
// prose, while leaving fenced code blocks unchanged. goldmark-pdf currently
// emits `>` as `&gt;` in rendered PDF text for plain prose, so normalizing here
// preserves the intended visual flow.
func normalizePlainTextArrows(mdBytes []byte) []byte {
	lines := strings.Split(string(mdBytes), "\n")
	inFence := false

	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}

		// Replace the longer token first so `-->` does not become `-→`.
		line = strings.ReplaceAll(line, "-->", "→")
		line = strings.ReplaceAll(line, "->", "→")
		lines[i] = line
	}

	return []byte(strings.Join(lines, "\n"))
}

// pdfStyles returns a themed style set based on the given theme name.
func pdfStyles(text, code gpdf.Font, theme string) gpdf.Styles {
	white := color.RGBA{255, 255, 255, 0}

	tc, ok := Themes[theme]
	if !ok {
		tc = Themes[DefaultTheme]
	}

	return gpdf.Styles{
		H1: &gpdf.Style{Font: text, Size: 18, Spacing: 6, TextColor: tc.H1, FillColor: white},
		H2: &gpdf.Style{Font: text, Size: 14, Spacing: 5, TextColor: tc.H2, FillColor: white},
		H3: &gpdf.Style{Font: text, Size: 12, Spacing: 4, TextColor: tc.H3, FillColor: white},
		H4: &gpdf.Style{Font: text, Size: 11, Spacing: 3, TextColor: tc.H4, FillColor: white},
		H5: &gpdf.Style{Font: text, Size: 10, Spacing: 3, TextColor: tc.H5, FillColor: white},
		H6: &gpdf.Style{Font: text, Size: 10, Spacing: 3, TextColor: tc.H6, FillColor: white},

		Normal:     &gpdf.Style{Font: text, Size: 10, Spacing: 2, TextColor: tc.TextNormal, FillColor: white},
		Blockquote: &gpdf.Style{Font: text, Size: 10, Spacing: 1.5, TextColor: tc.TextMuted, FillColor: white},

		THeader: &gpdf.Style{Font: text, Size: 9, Spacing: 2, TextColor: tc.TextNormal, FillColor: tc.TableHeaderBg},
		TBody:   &gpdf.Style{Font: text, Size: 9, Spacing: 2, TextColor: tc.TextNormal, FillColor: tc.TableRowBg},

		CodeFont:       code,
		CodeBlockTheme: codeHighlightTheme(tc.CodeBg, tc.CodeText),
		LinkColor:      tc.LinkColor,
	}
}

// pageFooter draws a centered page number at the bottom.
func pageFooter(impl gpdf.Fpdf, _ fonts.Cache) func() {
	return func() {
		f := impl.Fpdf
		f.SetY(-mmToPt(14))
		f.SetFont("Helvetica", "", 8)
		f.SetTextColor(160, 160, 160)
		_, lineHt := f.GetFontSize()
		w, _ := f.GetPageSize()
		f.CellFormat(w-mmToPt(40), lineHt, fmt.Sprintf("- %d -", f.PageNo()), "", 0, "C", false, 0, "")
	}
}

// codeHighlightTheme returns a GitHub-based chroma style with explicit
// Background/Text fallback to work around a goldmark-pdf bug where
// unset chroma colors render as white (invisible on white background).
func codeHighlightTheme(bgHex, textHex string) *chroma.Style {
	base := styles.Get("github")
	if base == nil {
		base = styles.Fallback
	}
	entry := textHex + " bg:" + bgHex
	s, err := base.Builder().
		Add(chroma.Background, entry).
		Add(chroma.Text, entry).
		Build()
	if err != nil {
		return base
	}
	return s
}

// mmToPt converts millimeters to points (gofpdf uses points internally).
func mmToPt(mm float64) float64 {
	return mm * 72.0 / 25.4
}
