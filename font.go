package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	pdf "github.com/stephenafamo/goldmark-pdf"
)

// fontSearchPaths returns candidate directories for font files.
func fontSearchPaths() []string {
	var paths []string

	if runtime.GOOS == "windows" {
		winDir := os.Getenv("WINDIR")
		if winDir == "" {
			winDir = `C:\Windows`
		}
		paths = append(paths, filepath.Join(winDir, "Fonts"))

		// User fonts (Windows 10+)
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData != "" {
			paths = append(paths, filepath.Join(localAppData, "Microsoft", "Windows", "Fonts"))
		}
	} else {
		// WSL: access Windows fonts via /mnt/c
		paths = append(paths, "/mnt/c/Windows/Fonts")
		// Linux system fonts
		paths = append(paths,
			"/usr/share/fonts/truetype",
			"/usr/local/share/fonts",
			filepath.Join(os.Getenv("HOME"), ".local/share/fonts"),
		)
	}

	return paths
}

// findFont searches for a font file in known directories.
func findFont(filename string) (string, error) {
	for _, dir := range fontSearchPaths() {
		path := filepath.Join(dir, filename)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("font not found: %s", filename)
}

// FontSet holds the font metadata and raw bytes for PDF registration.
type FontSet struct {
	Font    pdf.Font
	Regular []byte
	Bold    []byte
	Italic  []byte
	BoldItalic []byte
}

// registerFonts loads custom fonts onto the PDF object and returns Font descriptors.
func registerFonts(pdfObj pdf.PDF) (textFont, codeFont pdf.Font, err error) {
	textFont, codeFont = pdf.FontHelvetica, pdf.FontCourier // defaults

	// Try Malgun Gothic for text (Korean support)
	malgunSet, malgunErr := loadFontSet("MalgunGothic", "malgun.ttf", "malgunbd.ttf", "", "")
	if malgunErr == nil {
		malgunSet.Font.CanUseForText = true
		if err := registerFontSet(pdfObj, malgunSet); err != nil {
			return textFont, codeFont, fmt.Errorf("registering MalgunGothic: %w", err)
		}
		textFont = malgunSet.Font
	}

	// For code blocks: use Malgun Gothic as well (Consolas lacks Korean glyphs).
	// Register it under a separate family name so code styling is independent.
	codeSet, codeErr := loadFontSet("MalgunCode", "malgun.ttf", "malgunbd.ttf", "", "")
	if codeErr == nil {
		codeSet.Font.CanUseForCode = true
		if err := registerFontSet(pdfObj, codeSet); err != nil {
			return textFont, codeFont, fmt.Errorf("registering MalgunCode: %w", err)
		}
		codeFont = codeSet.Font
	}

	return textFont, codeFont, nil
}

// loadFontSet reads font files from system paths.
// Empty italic/boldItalic filenames fall back to regular/bold.
func loadFontSet(family, regular, bold, italic, boldItalic string) (*FontSet, error) {
	regularPath, err := findFont(regular)
	if err != nil {
		return nil, err
	}
	regularBytes, err := os.ReadFile(regularPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", regular, err)
	}

	// Bold: fall back to regular if not found
	boldBytes := regularBytes
	if bold != "" {
		if bp, err := findFont(bold); err == nil {
			if b, err := os.ReadFile(bp); err == nil {
				boldBytes = b
			}
		}
	}

	// Italic: fall back to regular
	italicBytes := regularBytes
	if italic != "" {
		if ip, err := findFont(italic); err == nil {
			if i, err := os.ReadFile(ip); err == nil {
				italicBytes = i
			}
		}
	}

	// BoldItalic: fall back to bold
	boldItalicBytes := boldBytes
	if boldItalic != "" {
		if bip, err := findFont(boldItalic); err == nil {
			if bi, err := os.ReadFile(bip); err == nil {
				boldItalicBytes = bi
			}
		}
	}

	return &FontSet{
		Font: pdf.Font{
			Family: family,
			Type:   pdf.FontTypeCustom,
		},
		Regular:    regularBytes,
		Bold:       boldBytes,
		Italic:     italicBytes,
		BoldItalic: boldItalicBytes,
	}, nil
}

// registerFontSet registers all font styles onto a PDF object.
func registerFontSet(pdfObj pdf.PDF, fs *FontSet) error {
	styles := []struct {
		style string
		data  []byte
	}{
		{pdf.FontStyleRegular, fs.Regular},
		{pdf.FontStyleBold, fs.Bold},
		{pdf.FontStyleItalic, fs.Italic},
		{pdf.FontStyleBoldItalic, fs.BoldItalic},
	}
	for _, s := range styles {
		if err := pdfObj.AddFont(fs.Font.Family, s.style, s.data); err != nil {
			return fmt.Errorf("adding font %s/%s: %w", fs.Font.Family, s.style, err)
		}
	}
	return nil
}
