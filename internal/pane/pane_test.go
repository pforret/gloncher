package pane

import (
	"regexp"
	"testing"
	"time"
)

func texts(lines []Line) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l.Text
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRingKeepsLastLines(t *testing.T) {
	p := New(3, nil)
	now := time.Now()
	for _, s := range []string{"a", "b", "c", "d", "e"} {
		p.Add(s, Stdout, now)
	}
	if got := texts(p.Lines()); !equal(got, []string{"c", "d", "e"}) {
		t.Errorf("Lines() = %v, want [c d e]", got)
	}
	if p.Total() != 5 {
		t.Errorf("Total() = %d, want 5", p.Total())
	}
}

func TestPartiallyFilledRingIsOrdered(t *testing.T) {
	p := New(5, nil)
	now := time.Now()
	p.Add("a", Stdout, now)
	p.Add("b", Stdout, now)
	if got := texts(p.Lines()); !equal(got, []string{"a", "b"}) {
		t.Errorf("Lines() = %v, want [a b]", got)
	}
}

func TestWrapExactlyAtCapacity(t *testing.T) {
	p := New(2, nil)
	now := time.Now()
	p.Add("a", Stdout, now)
	p.Add("b", Stdout, now)
	if got := texts(p.Lines()); !equal(got, []string{"a", "b"}) {
		t.Fatalf("Lines() = %v, want [a b]", got)
	}
	p.Add("c", Stdout, now)
	if got := texts(p.Lines()); !equal(got, []string{"b", "c"}) {
		t.Errorf("Lines() = %v, want [b c]", got)
	}
}

func TestFilterDropsNonMatchingLinesAndActivity(t *testing.T) {
	p := New(3, regexp.MustCompile("ERROR"))
	now := time.Now()
	p.Add("all good", Stdout, now)
	if act := p.Activity(now, time.Second); act.Out || act.Seen {
		t.Error("a filtered-out line should not light the indicator")
	}
	p.Add("ERROR: boom", Stderr, now)
	if got := texts(p.Lines()); !equal(got, []string{"ERROR: boom"}) {
		t.Errorf("Lines() = %v, want only the ERROR line", got)
	}
	if act := p.Activity(now, time.Second); !act.Err || !act.Seen {
		t.Error("a matching stderr line should light the error indicator")
	}
}

func TestHiddenPaneStillTracksActivity(t *testing.T) {
	p := New(0, nil)
	now := time.Now()
	p.Add("output", Stdout, now)
	if len(p.Lines()) != 0 {
		t.Error("a zero-capacity pane must retain no lines")
	}
	if act := p.Activity(now, time.Second); !act.Out || !act.Seen {
		t.Error("a hidden pane must still report activity so its light works")
	}
}

func TestActivityExpires(t *testing.T) {
	p := New(1, nil)
	now := time.Now()
	p.Add("x", Stdout, now)
	if act := p.Activity(now.Add(2*time.Second), time.Second); act.Out {
		t.Error("activity should go dim once the window has passed")
	} else if !act.Seen {
		t.Error("Seen should stay true after the window passes")
	}
}

func TestAddTrimsLineEndings(t *testing.T) {
	p := New(1, nil)
	p.Add("hello\r\n", Stdout, time.Now())
	if got := p.Lines()[0].Text; got != "hello" {
		t.Errorf("Text = %q, want %q", got, "hello")
	}
}

func TestConcurrentAddAndRead(t *testing.T) {
	p := New(8, nil)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 2000; i++ {
			p.Add("line", Stdout, time.Now())
		}
		close(done)
	}()
	for {
		select {
		case <-done:
			return
		default:
			p.Lines()
			p.Activity(time.Now(), time.Second)
		}
	}
}
