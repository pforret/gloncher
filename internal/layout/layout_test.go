package layout

import (
	"testing"

	"github.com/pforret/gloncher/internal/config"
)

func progs(specs ...config.Program) []config.Program { return specs }

func full(name string, lines int) config.Program {
	return config.Program{Name: name, Lines: lines, Width: config.WidthFull}
}

func half(name string, lines int) config.Program {
	return config.Program{Name: name, Lines: lines, Width: config.WidthHalf}
}

func TestHalfPanesPairUp(t *testing.T) {
	rows := Build(progs(half("queue", 2), half("schedule", 2)), 82)
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	cells := rows[0].Cells
	if len(cells) != 2 {
		t.Fatalf("got %d cells, want 2", len(cells))
	}
	if cells[0].Width+cells[1].Width+Gap != 82 {
		t.Errorf("widths %d + %d + gap %d should fill 82", cells[0].Width, cells[1].Width, Gap)
	}
	if cells[1].Col != cells[0].Width+Gap {
		t.Errorf("second cell starts at %d, want %d", cells[1].Col, cells[0].Width+Gap)
	}
}

func TestOddWidthSplitsWithoutLosingColumns(t *testing.T) {
	rows := Build(progs(half("a", 1), half("b", 1)), 81)
	c := rows[0].Cells
	if c[0].Width+c[1].Width+Gap != 81 {
		t.Errorf("widths %d + %d lose columns at odd terminal width", c[0].Width, c[1].Width)
	}
}

func TestLonelyHalfPaneSpansTheRow(t *testing.T) {
	rows := Build(progs(half("queue", 2)), 80)
	if len(rows) != 1 || len(rows[0].Cells) != 1 {
		t.Fatalf("got %+v, want one row with one cell", rows)
	}
	if w := rows[0].Cells[0].Width; w != 80 {
		t.Errorf("width = %d, want the full 80", w)
	}
}

func TestHalfNextToFullDoesNotPair(t *testing.T) {
	rows := Build(progs(half("a", 1), full("b", 1)), 80)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0].Cells[0].Width != 80 {
		t.Errorf("an unpaired half pane should span the row, got %d", rows[0].Cells[0].Width)
	}
}

func TestHiddenProgramsTakeNoSpace(t *testing.T) {
	// The hidden program sits between the two halves; they must still pair.
	p := progs(half("queue", 2), config.Program{Name: "vite", Lines: 0}, half("schedule", 2))
	rows := Build(p, 80)
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if len(rows[0].Cells) != 2 {
		t.Fatalf("got %d cells, want the two halves paired", len(rows[0].Cells))
	}
	if rows[0].Cells[1].Index != 2 {
		t.Errorf("second cell index = %d, want 2 (indexes address the original list)", rows[0].Cells[1].Index)
	}
}

func TestLaravelLayout(t *testing.T) {
	p := progs(
		full("server", 2),
		full("logs", 5),
		config.Program{Name: "vite", Lines: 0},
		half("queue", 2),
		half("schedule", 2),
	)
	rows := Build(p, 100)
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3 (server, logs, queue+schedule)", len(rows))
	}
	// 2+1 + 5+1 + 2+1 title lines
	if h := Height(rows, p); h != 12 {
		t.Errorf("Height = %d, want 12", h)
	}
}

func TestNarrowTerminalHasAFloor(t *testing.T) {
	rows := Build(progs(half("a", 1), half("b", 1)), 4)
	for _, c := range rows[0].Cells {
		if c.Width < 1 {
			t.Errorf("cell width %d is unusable", c.Width)
		}
	}
}
