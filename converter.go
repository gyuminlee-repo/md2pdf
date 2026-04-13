package main

import (
	"bytes"
	"context"
	"fmt"
	"image/color"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/go-swiss/fonts"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"

	gpdf "github.com/stephenafamo/goldmark-pdf"
)

// ConvertOptions controls conversion behavior. Zero values select defaults.
type ConvertOptions struct {
	Theme     string
	FontScale string // key into FontPresets; empty = default
	Mermaid   MermaidMode
}

// ConvertFile reads a markdown file and writes a PDF to the output path.
func ConvertFile(inputPath, outputPath string, opts ConvertOptions) error {
	mdBytes, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}

	baseDir, err := filepath.Abs(filepath.Dir(inputPath))
	if err != nil {
		return fmt.Errorf("resolving base dir: %w", err)
	}

	pdfBytes, err := Convert(mdBytes, baseDir, opts)
	if err != nil {
		return err
	}

	if err := os.WriteFile(outputPath, pdfBytes, 0644); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}

	return nil
}

// Convert takes markdown bytes and returns PDF bytes.
func Convert(mdBytes []byte, baseDir string, opts ConvertOptions) ([]byte, error) {
	ctx := context.Background()
	mdBytes = stripFrontmatter(mdBytes)
	mdBytes = resolveAttachments(mdBytes, baseDir)
	mdBytes = simplifyWikilinks(mdBytes)
	mdBytes = normalizePlainTextArrows(mdBytes)
	mdBytes = expandHardLineBreaks(mdBytes)
	mdBytes = transformMermaidBlocks(mdBytes, opts.Mermaid, baseDir)
	defer cleanupMermaidCache(baseDir)

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
	st := pdfStyles(textFont, codeFont, opts.Theme, opts.FontScale)

	// Configure renderer; a custom image renderer overrides the default to
	// clamp width and move an orphaned heading to the next page.
	renderer := gpdf.New(
		gpdf.WithPDF(fpdfObj),
		gpdf.WithImageFS(http.Dir(baseDir)),
		gpdf.WithContext(ctx),
		gpdf.WithEscapeHTML(false),
		gpdf.WithNodeRenderers(newImageNodeRenderer(fpdfObj, &st)),
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

// stripFrontmatter removes a leading YAML frontmatter block delimited by
// `---` lines. goldmark is not configured with a frontmatter parser, so the
// raw YAML would otherwise render as body text (with `"` escaped to `&quot;`
// due to a goldmark-pdf escaping bug).
func stripFrontmatter(mdBytes []byte) []byte {
	s := string(mdBytes)
	if !strings.HasPrefix(s, "---\n") && !strings.HasPrefix(s, "---\r\n") {
		return mdBytes
	}
	lines := strings.Split(s, "\n")
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == "---" {
			return []byte(strings.Join(lines[i+1:], "\n"))
		}
	}
	return mdBytes
}

var wikilinkRe = regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)

// simplifyWikilinks rewrites Obsidian-style `[[path/to/note]]` or
// `[[path|alias]]` references into plain readable text. CommonMark/GFM does
// not understand wikilinks, so without rewriting the raw brackets appear in
// the PDF. Fenced code blocks are left untouched.
func simplifyWikilinks(mdBytes []byte) []byte {
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

		lines[i] = wikilinkRe.ReplaceAllStringFunc(line, func(match string) string {
			inner := match[2 : len(match)-2]
			if pipe := strings.Index(inner, "|"); pipe >= 0 {
				return strings.TrimSpace(inner[pipe+1:])
			}
			if slash := strings.LastIndex(inner, "/"); slash >= 0 {
				return inner[slash+1:]
			}
			return inner
		})
	}

	return []byte(strings.Join(lines, "\n"))
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

// expandHardLineBreaks converts CommonMark hard line breaks (trailing `  ` or
// `\` before a newline) into blank-line-separated paragraphs, since
// goldmark-pdf v0.4.2 does not render hard breaks (writer.go flattens `\n` to
// space). Skips fenced code blocks.
func expandHardLineBreaks(mdBytes []byte) []byte {
	lines := strings.Split(string(mdBytes), "\n")
	inFence := false
	out := make([]string, 0, len(lines)*2)

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

		stripped := strings.TrimRight(line, "\r")
		hardBreak := false
		switch {
		case strings.HasSuffix(stripped, "  "):
			stripped = strings.TrimRight(stripped, " ")
			hardBreak = true
		case strings.HasSuffix(stripped, "\\") && !strings.HasSuffix(stripped, "\\\\"):
			stripped = strings.TrimSuffix(stripped, "\\")
			hardBreak = true
		}

		out = append(out, stripped)
		if hardBreak {
			out = append(out, "")
		}
	}

	return []byte(strings.Join(out, "\n"))
}

// FontPreset scales the base font sizes uniformly.
type FontPreset struct {
	Name       string
	Label      string
	SizeMul    float64 // multiplier applied to font sizes
	SpacingMul float64 // multiplier applied to line spacing
}

// FontPresets enumerates the selectable font-size presets (GUI order).
var FontPresets = []FontPreset{
	{"default", "기본", 1.00, 1.00},
	{"small", "소폭 축소 (약 10%)", 0.90, 0.90},
	{"compact", "중간 축소 (약 17%)", 0.83, 0.80},
	{"dense", "조밀 (약 22%)", 0.78, 0.70},
}

func fontPreset(name string) FontPreset {
	for _, p := range FontPresets {
		if p.Name == name {
			return p
		}
	}
	return FontPresets[0]
}

// pdfStyles returns a themed style set scaled by the font preset.
func pdfStyles(text, code gpdf.Font, theme, scaleName string) gpdf.Styles {
	white := color.RGBA{255, 255, 255, 0}

	tc, ok := Themes[theme]
	if !ok {
		tc = Themes[DefaultTheme]
	}

	p := fontPreset(scaleName)
	sz := func(v float64) float64 { return v * p.SizeMul }
	sp := func(v float64) float64 { return v * p.SpacingMul }

	return gpdf.Styles{
		H1: &gpdf.Style{Font: text, Size: sz(18), Spacing: sp(6), TextColor: tc.H1, FillColor: white},
		H2: &gpdf.Style{Font: text, Size: sz(14), Spacing: sp(5), TextColor: tc.H2, FillColor: white},
		H3: &gpdf.Style{Font: text, Size: sz(12), Spacing: sp(4), TextColor: tc.H3, FillColor: white},
		H4: &gpdf.Style{Font: text, Size: sz(11), Spacing: sp(3), TextColor: tc.H4, FillColor: white},
		H5: &gpdf.Style{Font: text, Size: sz(10), Spacing: sp(3), TextColor: tc.H5, FillColor: white},
		H6: &gpdf.Style{Font: text, Size: sz(10), Spacing: sp(3), TextColor: tc.H6, FillColor: white},

		Normal:     &gpdf.Style{Font: text, Size: sz(10), Spacing: sp(2), TextColor: tc.TextNormal, FillColor: white},
		Blockquote: &gpdf.Style{Font: text, Size: sz(10), Spacing: sp(1.5), TextColor: tc.TextMuted, FillColor: white},

		THeader: &gpdf.Style{Font: text, Size: sz(9), Spacing: sp(2), TextColor: tc.TextNormal, FillColor: tc.TableHeaderBg},
		TBody:   &gpdf.Style{Font: text, Size: sz(9), Spacing: sp(2), TextColor: tc.TextNormal, FillColor: tc.TableRowBg},

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
		Add(chroma.Keyword, textHex).
		Add(chroma.KeywordType, textHex).
		Add(chroma.Error, entry).
		Add(chroma.GenericDeleted, entry).
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
