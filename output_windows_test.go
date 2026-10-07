//go:build windows

package main

import "testing"

func TestExtendedOutputPath(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{`C:\reports\output.pdf`, `\\?\C:\reports\output.pdf`},
		{`\\server\share\output.pdf`, `\\?\UNC\server\share\output.pdf`},
		{`\\?\C:\reports\output.pdf`, `\\?\C:\reports\output.pdf`},
		{`\\?\UNC\server\share\output.pdf`, `\\?\UNC\server\share\output.pdf`},
	} {
		got, err := extendedOutputPath(tc.input)
		if err != nil || got != tc.want {
			t.Errorf("extendedOutputPath(%q) = %q, %v; want %q", tc.input, got, err, tc.want)
		}
	}
}
