//go:build !windows

package main

import (
	"fmt"
	"os"
)

// runGUI on non-Windows platforms falls back to a headless CLI mode.
// Pass one or more markdown paths as arguments; each is converted to a
// sibling `.pdf` file.
func runGUI() {
	if len(os.Args) > 1 {
		results, err := convertBatch(os.Args[1:], "", ConvertOptions{Theme: DefaultTheme}, func(_ int, plan batchOutput) {
			fmt.Printf("Converting: %s\n", plan.Input)
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return
		}
		for _, result := range results {
			if result.Error != "" {
				fmt.Fprintf(os.Stderr, "Error (%s): %s\n", result.Input, result.Error)
				continue
			}
			fmt.Printf("Done: %s\n", result.Output)
		}
		return
	}

	fmt.Printf("md2pdf v%s\n", version)
	fmt.Println("Usage: md2pdf <file.md> [file2.md ...]")
	fmt.Println("GUI is available on Windows only.")
}
