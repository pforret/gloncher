package ui

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ANSI control sequences used by the renderer.
const (
	altScreenOn  = "\x1b[?1049h"
	altScreenOff = "\x1b[?1049l"
	cursorHide   = "\x1b[?25l"
	cursorShow   = "\x1b[?25h"
	cursorHome   = "\x1b[H"
	clearScreen  = "\x1b[2J"
	clearLine    = "\x1b[K"
	reset        = "\x1b[0m"
	bold         = "\x1b[1m"
	dim          = "\x1b[2m"
)

// palette maps color names to their dim and bright SGR codes. Lights use the
// dim variant when idle and the bright variant on recent output.
var palette = map[string][2]string{
	"red":     {"\x1b[31m", "\x1b[91m"},
	"green":   {"\x1b[32m", "\x1b[92m"},
	"yellow":  {"\x1b[33m", "\x1b[93m"},
	"blue":    {"\x1b[34m", "\x1b[94m"},
	"magenta": {"\x1b[35m", "\x1b[95m"},
	"cyan":    {"\x1b[36m", "\x1b[96m"},
	"white":   {"\x1b[37m", "\x1b[97m"},
}

// defaultColors is the rotation used for programs that don't name a color.
var defaultColors = []string{"green", "cyan", "yellow", "magenta", "blue", "white"}

// colorFor resolves a program's configured color, falling back to the
// rotation by position.
func colorFor(name string, index int) [2]string {
	if c, ok := palette[name]; ok {
		return c
	}
	return palette[defaultColors[index%len(defaultColors)]]
}

// truncate shortens s to at most width display cells, adding an ellipsis when
// it cuts. It counts runes, which is right for the ASCII-ish output of dev
// servers and close enough elsewhere.
func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	r := []rune(s)
	return string(r[:width-1]) + "…"
}

// pad right-pads s with spaces to exactly width cells.
func pad(s string, width int) string {
	n := width - utf8.RuneCountInString(s)
	if n <= 0 {
		return s
	}
	return s + strings.Repeat(" ", n)
}

// sanitize strips control characters that would corrupt the layout, notably
// the escape sequences and carriage returns that build tools emit.
func sanitize(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	skip := false
	for _, r := range s {
		switch {
		case skip:
			// Consume until the final byte of a CSI sequence.
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				skip = false
			}
		case r == 0x1b:
			skip = true
		case r == '\t':
			b.WriteString("    ")
		case r < 0x20 || r == 0x7f:
			// drop
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// TermSize returns the terminal width and height, falling back to 80x24.
// stty is used because the standard library exposes no portable way to ask.
func TermSize() (width, height int) {
	width, height = 80, 24
	if w, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && w > 0 {
		width = w
	}
	if h, err := strconv.Atoi(os.Getenv("LINES")); err == nil && h > 0 {
		height = h
	}
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	out, err := cmd.Output()
	if err != nil {
		return width, height
	}
	parts := strings.Fields(string(out))
	if len(parts) != 2 {
		return width, height
	}
	if h, err := strconv.Atoi(parts[0]); err == nil && h > 0 {
		height = h
	}
	if w, err := strconv.Atoi(parts[1]); err == nil && w > 0 {
		width = w
	}
	return width, height
}
