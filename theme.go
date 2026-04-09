package main

import "image/color"

// ThemeColors defines the color palette for PDF rendering.
type ThemeColors struct {
	Description                      string
	H1, H2, H3, H4, H5, H6         color.RGBA
	TextNormal, TextMuted, LinkColor color.RGBA
	TableHeaderBg, TableRowBg        color.RGBA
	CodeBg, CodeText                 string // hex for chroma style
}

// ThemeEntry holds a theme name and description for GUI display.
type ThemeEntry struct {
	Name        string
	Description string
}

// DefaultTheme is the default theme name.
const DefaultTheme = "Blue Topaz"

// Themes maps theme names to their color palettes.
var Themes = map[string]ThemeColors{
	"Blue Topaz": {
		Description:   "네이비→스카이 그라데이션 헤딩",
		H1:            color.RGBA{R: 7, G: 42, B: 110, A: 255},
		H2:            color.RGBA{R: 0, G: 71, B: 169, A: 255},
		H3:            color.RGBA{R: 14, G: 94, B: 177, A: 255},
		H4:            color.RGBA{R: 53, G: 128, B: 185, A: 255},
		H5:            color.RGBA{R: 93, G: 160, B: 214, A: 255},
		H6:            color.RGBA{R: 137, G: 187, B: 223, A: 255},
		TextNormal:    color.RGBA{R: 14, G: 14, B: 14, A: 255},
		TextMuted:     color.RGBA{R: 127, G: 127, B: 127, A: 255},
		LinkColor:     color.RGBA{R: 70, G: 142, B: 235, A: 255},
		TableHeaderBg: color.RGBA{R: 232, G: 240, B: 251, A: 255},
		TableRowBg:    color.RGBA{R: 244, G: 244, B: 244, A: 255},
		CodeBg:        "#ebebeb",
		CodeText:      "#333333",
	},
	"Minimal": {
		Description:   "회색 계열, 미니멀",
		H1:            color.RGBA{R: 26, G: 26, B: 26, A: 255},
		H2:            color.RGBA{R: 51, G: 51, B: 51, A: 255},
		H3:            color.RGBA{R: 68, G: 68, B: 68, A: 255},
		H4:            color.RGBA{R: 85, G: 85, B: 85, A: 255},
		H5:            color.RGBA{R: 102, G: 102, B: 102, A: 255},
		H6:            color.RGBA{R: 119, G: 119, B: 119, A: 255},
		TextNormal:    color.RGBA{R: 30, G: 30, B: 30, A: 255},
		TextMuted:     color.RGBA{R: 153, G: 153, B: 153, A: 255},
		LinkColor:     color.RGBA{R: 74, G: 140, B: 199, A: 255},
		TableHeaderBg: color.RGBA{R: 240, G: 240, B: 240, A: 255},
		TableRowBg:    color.RGBA{R: 248, G: 248, B: 248, A: 255},
		CodeBg:        "#f0f0f0",
		CodeText:      "#383a42",
	},
	"Catppuccin Latte": {
		Description:   "파스텔 톤, 무지개 헤딩",
		H1:            color.RGBA{R: 136, G: 57, B: 239, A: 255},  // Mauve
		H2:            color.RGBA{R: 30, G: 102, B: 245, A: 255},  // Blue
		H3:            color.RGBA{R: 23, G: 146, B: 153, A: 255},  // Teal
		H4:            color.RGBA{R: 64, G: 160, B: 43, A: 255},   // Green
		H5:            color.RGBA{R: 223, G: 142, B: 29, A: 255},  // Yellow
		H6:            color.RGBA{R: 254, G: 100, B: 11, A: 255},  // Peach
		TextNormal:    color.RGBA{R: 76, G: 79, B: 105, A: 255},
		TextMuted:     color.RGBA{R: 156, G: 160, B: 176, A: 255},
		LinkColor:     color.RGBA{R: 30, G: 102, B: 245, A: 255},
		TableHeaderBg: color.RGBA{R: 230, G: 233, B: 239, A: 255},
		TableRowBg:    color.RGBA{R: 239, G: 241, B: 245, A: 255},
		CodeBg:        "#e6e9ef",
		CodeText:      "#4c4f69",
	},
	"Things": {
		Description:   "파란 액센트, 깔끔한 타이포",
		H1:            color.RGBA{R: 16, G: 107, B: 163, A: 255},
		H2:            color.RGBA{R: 24, G: 121, B: 178, A: 255},
		H3:            color.RGBA{R: 32, G: 135, B: 193, A: 255},
		H4:            color.RGBA{R: 58, G: 149, B: 200, A: 255},
		H5:            color.RGBA{R: 74, G: 155, B: 207, A: 255},
		H6:            color.RGBA{R: 106, G: 173, B: 214, A: 255},
		TextNormal:    color.RGBA{R: 43, G: 43, B: 43, A: 255},
		TextMuted:     color.RGBA{R: 136, G: 136, B: 136, A: 255},
		LinkColor:     color.RGBA{R: 16, G: 107, B: 163, A: 255},
		TableHeaderBg: color.RGBA{R: 232, G: 240, B: 248, A: 255},
		TableRowBg:    color.RGBA{R: 245, G: 245, B: 245, A: 255},
		CodeBg:        "#f0f0f0",
		CodeText:      "#2b2b2b",
	},
	"Nord": {
		Description:   "북유럽 청록 팔레트",
		H1:            color.RGBA{R: 94, G: 129, B: 172, A: 255},
		H2:            color.RGBA{R: 129, G: 161, B: 193, A: 255},
		H3:            color.RGBA{R: 136, G: 192, B: 208, A: 255},
		H4:            color.RGBA{R: 143, G: 188, B: 187, A: 255},
		H5:            color.RGBA{R: 163, G: 190, B: 140, A: 255},
		H6:            color.RGBA{R: 180, G: 142, B: 173, A: 255},
		TextNormal:    color.RGBA{R: 46, G: 52, B: 64, A: 255},
		TextMuted:     color.RGBA{R: 123, G: 136, B: 161, A: 255},
		LinkColor:     color.RGBA{R: 94, G: 129, B: 172, A: 255},
		TableHeaderBg: color.RGBA{R: 229, G: 233, B: 240, A: 255},
		TableRowBg:    color.RGBA{R: 236, G: 239, B: 244, A: 255},
		CodeBg:        "#e5e9f0",
		CodeText:      "#2e3440",
	},
}

// themeOrder defines the display order for GUI.
var themeOrder = []string{"Blue Topaz", "Minimal", "Catppuccin Latte", "Things", "Nord"}

// ThemeList returns ordered theme entries for GUI display.
func ThemeList() []ThemeEntry {
	entries := make([]ThemeEntry, len(themeOrder))
	for i, name := range themeOrder {
		entries[i] = ThemeEntry{Name: name, Description: Themes[name].Description}
	}
	return entries
}
