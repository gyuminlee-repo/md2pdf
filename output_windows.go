//go:build windows

package main

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func publishOutput(staged, destination string, replaceExisting bool) error {
	fromPath, err := extendedOutputPath(staged)
	if err != nil {
		return err
	}
	toPath, err := extendedOutputPath(destination)
	if err != nil {
		return err
	}
	from, err := windows.UTF16PtrFromString(fromPath)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(toPath)
	if err != nil {
		return err
	}
	flags := uint32(windows.MOVEFILE_WRITE_THROUGH)
	if replaceExisting {
		flags |= windows.MOVEFILE_REPLACE_EXISTING
	}
	// Without REPLACE_EXISTING, a destination created after preflight wins.
	return windows.MoveFileEx(from, to, flags)
}

// Raw Win32 calls do not get the long-path normalization used by os.*. Use an
// absolute extended path for both local and UNC destinations, including on
// older Windows builds without process-wide long-path support.
func extendedOutputPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(abs, `\\?\`) {
		return abs, nil
	}
	if strings.HasPrefix(abs, `\\`) {
		return `\\?\UNC\` + abs[2:], nil
	}
	return `\\?\` + abs, nil
}
