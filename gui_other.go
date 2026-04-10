//go:build !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// runGUI on non-Windows platforms falls back to a headless CLI mode.
// Pass one or more markdown paths as arguments; each is converted to a
// sibling `.pdf` file.
func runGUI() {
	if len(os.Args) > 1 {
		for _, input := range os.Args[1:] {
			output := strings.TrimSuffix(input, filepath.Ext(input)) + ".pdf"
			fmt.Printf("Converting: %s\n", input)
			if err := ConvertFile(input, output, DefaultTheme); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				continue
			}
			fmt.Printf("Done: %s\n", output)
		}
		return
	}

	fmt.Printf("md2pdf v%s\n", version)
	fmt.Println("Usage: md2pdf <file.md> [file2.md ...]")
	fmt.Println("GUI is available on Windows only.")
}
