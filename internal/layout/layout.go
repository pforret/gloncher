// Package layout turns a list of pane widths into terminal rows.
package layout

import "github.com/pforret/gloncher/internal/config"

// Cell is one pane placed in a row.
type Cell struct {
	Index int // index into the original program list
	Width int // usable character columns for this pane
	Col   int // starting column, 0-based
}

// Row is one horizontal band of the screen holding one or two panes.
type Row struct {
	Cells []Cell
}

// Gap is the number of spaces between two half-width panes.
const Gap = 2

// Build places visible programs into rows. Half-width panes pair up with the
// next half-width pane; a half-width pane with no partner spans the row so no
// screen space is wasted. Programs are visited in declaration order, and
// hidden programs are skipped entirely.
func Build(programs []config.Program, termWidth int) []Row {
	if termWidth < 20 {
		termWidth = 20
	}

	visible := make([]int, 0, len(programs))
	for i, p := range programs {
		if !p.Hidden() {
			visible = append(visible, i)
		}
	}

	var rows []Row
	for i := 0; i < len(visible); i++ {
		idx := visible[i]
		if programs[idx].Width == config.WidthHalf && i+1 < len(visible) &&
			programs[visible[i+1]].Width == config.WidthHalf {
			left := (termWidth - Gap) / 2
			right := termWidth - Gap - left
			rows = append(rows, Row{Cells: []Cell{
				{Index: idx, Width: left, Col: 0},
				{Index: visible[i+1], Width: right, Col: left + Gap},
			}})
			i++ // consumed the partner
			continue
		}
		rows = append(rows, Row{Cells: []Cell{{Index: idx, Width: termWidth, Col: 0}}})
	}
	return rows
}

// Height is the number of terminal lines the rows occupy, given one title
// line per row plus the tallest pane in each row.
func Height(rows []Row, programs []config.Program) int {
	total := 0
	for _, r := range rows {
		tallest := 0
		for _, c := range r.Cells {
			if n := programs[c.Index].Lines; n > tallest {
				tallest = n
			}
		}
		total += tallest + 1
	}
	return total
}
