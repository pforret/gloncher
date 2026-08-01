//go:build !windows

package ui

import (
	"os"
	"os/exec"
)

// MakeRaw puts the terminal in raw mode so single keypresses arrive without
// waiting for enter, and returns a function restoring the previous settings.
// It reports false when there is no terminal to configure, in which case the
// caller should skip keyboard handling.
func MakeRaw() (restore func(), ok bool) {
	saved, err := stty("-g")
	if err != nil {
		return nil, false
	}
	if _, err := stty("raw", "-echo"); err != nil {
		return nil, false
	}
	return func() { _, _ = stty(saved) }, true
}

// stty runs stty against the controlling terminal and returns its output.
func stty(args ...string) (string, error) {
	cmd := exec.Command("stty", args...)
	cmd.Stdin = os.Stdin
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return trimNewline(string(out)), nil
}

func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
