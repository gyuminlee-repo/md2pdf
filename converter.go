package main

import (
	"bytes"
	"context"
	"fmt"
	"image/color"
	"net/http"
	"os"
	"path/filepath"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/go-swiss/fonts"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"

	gpdf "github.com/stephenafamo/goldmark-pdf"
)

// ConvertFile reads a markdown file and writes a PDF to the output path.
func ConvertFile(inputPath, outputPath string) error {
	mdBytes, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}

	baseDir, err := filepath.Abs(filepath.Dir(inputPath))
	if err != nil {
		return fmt.Errorf("resolving base dir: %w", err)
	}

	pdfBytes, err := Convert(mdBytes, baseDir)
	if err != nil {
		return err
	}

	if err := os.WriteFile(outputPath, pdfBytes, 0644); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}

	return nil
}

// Convert takes markdown bytes and returns PDF bytes.
func Convert(mdBytes []byte, baseDir string) ([]byte, error) {
	ctx := context.Background()

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
	st := pdfStyles(textFont, codeFont)

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

// pdfStyles returns a Blue Topaz (light mode) themed style set.
func pdfStyles(text, code gpdf.Font) gpdf.Styles {
	white := color.RGBA{255, 255, 255, 0}

	// Blue Topaz heading colors (light mode, navy-to-sky gradient)
	h1Color := color.RGBA{R: 7, G: 42, B: 110, A: 255}    // hsl(216,88%,26%)  진한 네이비
	h2Color := color.RGBA{R: 0, G: 71, B: 169, A: 255}    // hsl(212,100%,33%) 딥 블루
	h3Color := color.RGBA{R: 14, G: 94, B: 177, A: 255}   // hsl(210,86%,39%)  미디엄 블루
	h4Color := color.RGBA{R: 53, G: 128, B: 185, A: 255}  // hsl(208,58%,49%)  스카이 블루
	h5Color := color.RGBA{R: 93, G: 160, B: 214, A: 255}  // hsl(209,70%,62%)  밝은 블루
	h6Color := color.RGBA{R: 137, G: 187, B: 223, A: 255} // hsl(209,65%,72%)  연한 블루

	textNormal := color.RGBA{R: 14, G: 14, B: 14, A: 255}    // #0e0e0e
	textMuted := color.RGBA{R: 127, G: 127, B: 127, A: 255}  // #7f7f7f
	linkBlue := color.RGBA{R: 70, G: 142, B: 235, A: 255}    // #468EEB

	// Table: Blue Topaz 스타일
	tableHeaderBg := color.RGBA{R: 232, G: 240, B: 251, A: 255} // accent 10% tint
	tableRowBg := color.RGBA{R: 244, G: 244, B: 244, A: 255}    // #f4f4f4

	return gpdf.Styles{
		H1: &gpdf.Style{Font: text, Size: 18, Spacing: 6, TextColor: h1Color, FillColor: white},
		H2: &gpdf.Style{Font: text, Size: 14, Spacing: 5, TextColor: h2Color, FillColor: white},
		H3: &gpdf.Style{Font: text, Size: 12, Spacing: 4, TextColor: h3Color, FillColor: white},
		H4: &gpdf.Style{Font: text, Size: 11, Spacing: 3, TextColor: h4Color, FillColor: white},
		H5: &gpdf.Style{Font: text, Size: 10, Spacing: 3, TextColor: h5Color, FillColor: white},
		H6: &gpdf.Style{Font: text, Size: 10, Spacing: 3, TextColor: h6Color, FillColor: white},

		Normal:     &gpdf.Style{Font: text, Size: 10, Spacing: 2, TextColor: textNormal, FillColor: white},
		Blockquote: &gpdf.Style{Font: text, Size: 10, Spacing: 1.5, TextColor: textMuted, FillColor: white},

		THeader: &gpdf.Style{Font: text, Size: 9, Spacing: 2, TextColor: textNormal, FillColor: tableHeaderBg},
		TBody:   &gpdf.Style{Font: text, Size: 9, Spacing: 2, TextColor: textNormal, FillColor: tableRowBg},

		CodeFont:       code,
		CodeBlockTheme: codeHighlightTheme(),
		LinkColor:      linkBlue,
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
func codeHighlightTheme() *chroma.Style {
	base := styles.Get("github")
	if base == nil {
		base = styles.Fallback
	}
	s, err := base.Builder().
		Add(chroma.Background, "#333333 bg:#ebebeb").
		Add(chroma.Text, "#333333 bg:#ebebeb").
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
