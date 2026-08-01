// Package ui renders the gloncher screen.
package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/pforret/gloncher/internal/config"
	"github.com/pforret/gloncher/internal/layout"
	"github.com/pforret/gloncher/internal/pane"
	"github.com/pforret/gloncher/internal/proc"
)

// activityWindow is how long after a line arrives the light stays bright.
const activityWindow = 750 * time.Millisecond

// Renderer draws the screen for a group of processes.
type Renderer struct {
	cfg     *config.Config
	group   *proc.Group
	started time.Time
}

// New creates a renderer for cfg and group.
func New(cfg *config.Config, group *proc.Group, started time.Time) *Renderer {
	return &Renderer{cfg: cfg, group: group, started: started}
}

// Frame renders the full screen for the given terminal size and moment. The
// result is a sequence of complete lines, each ending in a clear-to-EOL so a
// shrinking line leaves no debris behind.
func (r *Renderer) Frame(width, height int, now time.Time) string {
	var b strings.Builder
	b.WriteString(cursorHome)

	lines := []string{r.lightsLine(now), r.statusLine(now, width)}

	rows := layout.Build(r.cfg.Programs, width)
	for _, row := range rows {
		lines = append(lines, r.rowLines(row, now)...)
	}

	// Leave the last screen line free so the terminal does not scroll.
	max := height - 1
	if max < 1 {
		max = 1
	}
	if len(lines) > max {
		lines = lines[:max]
	}
	for _, l := range lines {
		b.WriteString(l)
		b.WriteString(clearLine)
		b.WriteString("\r\n")
	}
	b.WriteString("\x1b[J") // clear everything below the last line drawn
	return b.String()
}

// lightsLine is the indicator row: one light per process, including hidden
// ones, in declaration order.
func (r *Renderer) lightsLine(now time.Time) string {
	var b strings.Builder
	b.WriteString(bold)
	b.WriteString(r.cfg.Title)
	b.WriteString(reset)
	b.WriteString("  ")

	for i, p := range r.group.Processes {
		act := p.Pane.Activity(now, activityWindow)
		st := p.Status()
		colors := colorFor(p.Program.Color, i)

		glyph, style := "●", colors[0] // idle: the dim variant
		switch {
		case st.Fatal:
			glyph, style = "✗", palette["red"][1]
		case st.State == proc.Exited && st.ExitCode != 0 && !st.Stopped:
			glyph, style = "✗", palette["red"][1]
		case st.State == proc.Exited:
			glyph, style = "○", dim
		case act.Err:
			style = palette["red"][1]
		case act.Out:
			style = colors[1] // bright: recent output
		}
		fmt.Fprintf(&b, "%s%s %s%s", style, glyph, p.Program.Name, reset)
		if p.Program.Hidden() {
			b.WriteString(dim + "·" + reset)
		}
		b.WriteString("  ")
	}
	return b.String()
}

func (r *Renderer) statusLine(now time.Time, width int) string {
	running := 0
	for _, p := range r.group.Processes {
		if p.Status().State == proc.Running {
			running++
		}
	}
	up := now.Sub(r.started).Truncate(time.Second)
	s := fmt.Sprintf("%d/%d running · up %s · q or ctrl-c to quit",
		running, len(r.group.Processes), up)
	return dim + truncate(s, width) + reset
}

// rowLines renders one layout row: a title line, then the panes' content
// lines side by side.
func (r *Renderer) rowLines(row layout.Row, now time.Time) []string {
	tallest := 0
	for _, c := range row.Cells {
		if n := r.cfg.Programs[c.Index].Lines; n > tallest {
			tallest = n
		}
	}

	titles := make([]string, len(row.Cells))
	bodies := make([][]string, len(row.Cells))
	for i, c := range row.Cells {
		titles[i] = r.title(c, now)
		bodies[i] = r.body(c, tallest)
	}

	out := make([]string, 0, tallest+1)
	out = append(out, joinCells(titles))
	for line := 0; line < tallest; line++ {
		parts := make([]string, len(row.Cells))
		for i := range row.Cells {
			parts[i] = bodies[i][line]
		}
		out = append(out, joinCells(parts))
	}
	return out
}

// joinCells places already-width-fitted cell strings side by side. Cells are
// padded by their own renderers, so only the gap is added here.
func joinCells(parts []string) string {
	return strings.Join(parts, strings.Repeat(" ", layout.Gap))
}

func (r *Renderer) title(c layout.Cell, now time.Time) string {
	p := r.group.Processes[c.Index]
	prog := r.cfg.Programs[c.Index]
	st := p.Status()
	colors := colorFor(prog.Color, c.Index)

	label := prog.Name
	switch {
	case st.Fatal:
		label += fmt.Sprintf(" (exit %d — quitting)", st.ExitCode)
	case st.State == proc.Exited && st.Stopped:
		label += " (stopped)"
	case st.State == proc.Exited && st.ExitCode != 0:
		label += fmt.Sprintf(" (exit %d)", st.ExitCode)
	case st.State == proc.Exited && prog.OnExit == config.ExitRestart:
		label += " (restarting)"
	case st.State == proc.Exited:
		label += " (done)"
	case st.State == proc.Starting:
		label += " (starting)"
	}
	if prog.Filter != nil {
		label += " /" + prog.Filter.String() + "/"
	}
	if st.Restarts > 0 {
		label += fmt.Sprintf(" ×%d", st.Restarts+1)
	}

	label = truncate(label, c.Width)
	rule := c.Width - len([]rune(label)) - 1
	if rule < 0 {
		rule = 0
	}
	return colors[1] + label + reset + dim + " " + strings.Repeat("─", rule) + reset
}

// body returns exactly height content lines for a cell, bottom-aligned so the
// newest output sits closest to the next pane, and padded to the cell width.
func (r *Renderer) body(c layout.Cell, height int) []string {
	p := r.group.Processes[c.Index]
	lines := p.Pane.Lines()

	out := make([]string, height)
	// Right-align the available lines against the bottom of the cell.
	offset := height - len(lines)
	for i := 0; i < height; i++ {
		idx := i - offset
		if idx < 0 || idx >= len(lines) {
			out[i] = strings.Repeat(" ", c.Width)
			continue
		}
		l := lines[idx]
		text := pad(truncate(sanitize(l.Text), c.Width), c.Width)
		if l.Stream == pane.Stderr {
			text = palette["red"][0] + text + reset
		}
		out[i] = text
	}
	return out
}
