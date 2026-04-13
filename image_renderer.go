package main

import (
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	gpdf "github.com/stephenafamo/goldmark-pdf"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/util"
)

// imageNodeRenderer overrides goldmark-pdf's default image rendering:
//   - clamps image width to half the usable page width
//   - scales height proportionally via the intrinsic PNG size
//   - if the image overflows the current page AND the directly preceding
//     block is a heading, moves both to the next page (re-emitting the
//     heading at the top) to avoid orphaning the title.
type imageNodeRenderer struct {
	fpdf   *gpdf.Fpdf
	styles *gpdf.Styles
}

func newImageNodeRenderer(fpdf *gpdf.Fpdf, styles *gpdf.Styles) util.PrioritizedValue {
	return util.Prioritized(&imageNodeRenderer{fpdf: fpdf, styles: styles}, 500)
}

func (r *imageNodeRenderer) RegisterFuncs(reg gpdf.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindImage, r.renderImage)
	reg.Register(ast.KindHeading, r.renderHeading)
}

// renderHeading pre-checks whether the next block (typically an image) will
// overflow, and forces a page break BEFORE writing the heading so the title
// and image land on the same page without leaving a dangling heading on the
// previous one. Heading text is written directly (bypassing child walking)
// because goldmark-pdf's internal state stack is unexported.
func (r *imageNodeRenderer) renderHeading(w *gpdf.Writer, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	h := node.(*ast.Heading)
	style := r.headingStyle(h.Level)

	nextH := r.nextBlockOverflowHeight(w, node)
	_, pageH := r.fpdf.GetPageSize()
	_, _, _, mbottom := r.fpdf.GetMargins()

	headingH := style.Size + style.Spacing*3
	curY := r.fpdf.GetY()
	needed := headingH + nextH
	remaining := pageH - mbottom - curY
	if nextH > 0 && needed > remaining {
		r.fpdf.AddPage()
	}

	gpdf.SetStyle(w.Pdf, style)
	w.Pdf.BR(style.Size + style.Spacing*2)
	if id, _ := h.AttributeString("id"); id != nil {
		if anchor, ok := id.([]byte); ok {
			w.Pdf.AddInternalLink(string(anchor))
		}
	}

	var buf strings.Builder
	for c := h.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			buf.Write(t.Segment.Value(source))
		}
	}
	w.Pdf.WriteText(style.Size+style.Spacing, buf.String())
	w.Pdf.BR(style.Size + style.Spacing)

	return ast.WalkSkipChildren, nil
}

func (r *imageNodeRenderer) headingStyle(level int) gpdf.Style {
	switch level {
	case 1:
		return *r.styles.H1
	case 2:
		return *r.styles.H2
	case 3:
		return *r.styles.H3
	case 4:
		return *r.styles.H4
	case 5:
		return *r.styles.H5
	default:
		return *r.styles.H6
	}
}

// nextBlockOverflowHeight inspects the block directly after this heading.
// If that block contains an image resolvable via ImageFS, it returns the
// PDF height that image will occupy after our width clamp; otherwise 0.
func (r *imageNodeRenderer) nextBlockOverflowHeight(w *gpdf.Writer, n ast.Node) float64 {
	next := n.NextSibling()
	if next == nil {
		return 0
	}
	var img *ast.Image
	ast.Walk(next, func(nn ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if i, ok := nn.(*ast.Image); ok {
			img = i
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	if img == nil {
		return 0
	}

	imgPath := localFsPath(string(img.Destination))
	file, err := w.ImageFS.Open(imgPath)
	if err != nil {
		return 0
	}
	defer file.Close()
	imgMime := strings.TrimPrefix(fileMime(file, imgPath), "image/")
	w.Pdf.RegisterImage(imgPath, imgMime, file)
	info := r.fpdf.Fpdf.GetImageInfo(imgPath)
	if info == nil {
		return 0
	}

	pageW, _ := r.fpdf.GetPageSize()
	mleft, _, mright, _ := r.fpdf.GetMargins()
	usableW := pageW - mleft*2 - mright*2
	maxW := usableW / 3
	iw := maxW
	if info.Width() < iw {
		iw = info.Width()
	}
	return info.Height() * (iw / info.Width())
}

func (r *imageNodeRenderer) renderImage(w *gpdf.Writer, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	img := node.(*ast.Image)
	imgPath := localFsPath(string(img.Destination))
	file, err := w.ImageFS.Open(imgPath)
	if err != nil {
		log.Printf("IMAGE ERROR: %s, %v", imgPath, err)
		return ast.WalkContinue, nil
	}
	defer file.Close()

	imgMime := strings.TrimPrefix(fileMime(file, imgPath), "image/")
	w.Pdf.RegisterImage(imgPath, imgMime, file)

	info := r.fpdf.Fpdf.GetImageInfo(imgPath)
	if info == nil {
		return ast.WalkContinue, nil
	}

	pageW, pageH := r.fpdf.GetPageSize()
	mleft, _, mright, mbottom := r.fpdf.GetMargins()
	usableW := pageW - mleft*2 - mright*2
	maxW := usableW / 3

	targetW := maxW
	if info.Width() < targetW {
		targetW = info.Width()
	}
	targetH := info.Height() * (targetW / info.Width())

	curY := r.fpdf.GetY()
	remaining := pageH - mbottom - curY
	if targetH > remaining {
		r.fpdf.AddPage()
		curY = r.fpdf.GetY()
	}

	r.fpdf.UseImage(imgPath, mleft*2, curY, targetW, 0)
	r.fpdf.SetY(curY + targetH)

	return ast.WalkSkipChildren, nil
}

// localFsPath mirrors goldmark-pdf's internal localPath: trim leading "./".
func localFsPath(path string) string {
	if strings.HasPrefix(path, "../") {
		return path
	}
	return strings.TrimLeft(path, "./")
}

func fileMime(f http.File, path string) string {
	if typed, ok := f.(interface{ MimeType() string }); ok {
		if m := typed.MimeType(); m != "" {
			return m
		}
	}
	if ext := filepath.Ext(path); ext != "" {
		if m := mime.TypeByExtension(ext); m != "" {
			return m
		}
	}
	return "image/png"
}
