//go:build !windows

package main

import "os"

func publishOutput(staged, destination string, replaceExisting bool) error {
	if replaceExisting {
		return os.Rename(staged, destination)
	}
	// Linking is an atomic create-if-absent operation. A filesystem without
	// hard-link support fails safely; never fall back to a clobbering rename.
	return os.Link(staged, destination)
}
