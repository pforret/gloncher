// Package pane holds the last N output lines of one process, plus the
// activity state that drives its indicator light.
package pane

import (
	"regexp"
	"strings"
	"sync"
	"time"
)

// Stream identifies which pipe a line arrived on.
type Stream int

const (
	// Stdout is the child's standard output.
	Stdout Stream = iota
	// Stderr is the child's standard error.
	Stderr
)

// Line is one captured output line.
type Line struct {
	Text   string
	Stream Stream
	At     time.Time
}

// Pane is a fixed-capacity ring buffer of output lines. It is safe for
// concurrent use: writer goroutines call Add, the renderer calls Lines and
// Activity.
type Pane struct {
	mu     sync.Mutex
	buf    []Line
	next   int // index of the oldest entry once full
	full   bool
	filter *regexp.Regexp

	lastOut time.Time
	lastErr time.Time
	total   uint64
}

// New returns a pane keeping the most recent capacity lines. A capacity of 0
// (a hidden pane) still tracks activity, so lights work for hidden programs.
func New(capacity int, filter *regexp.Regexp) *Pane {
	if capacity < 0 {
		capacity = 0
	}
	return &Pane{buf: make([]Line, capacity), filter: filter}
}

// Add records one output line, dropping it if a filter is set and does not
// match. Activity is only recorded for lines that survive the filter, so a
// pane filtered to ERROR stays dark until an actual error shows up.
func (p *Pane) Add(text string, stream Stream, at time.Time) {
	text = strings.TrimRight(text, "\r\n")
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.filter != nil && !p.filter.MatchString(text) {
		return
	}
	p.total++
	if stream == Stderr {
		p.lastErr = at
	} else {
		p.lastOut = at
	}
	if len(p.buf) == 0 {
		return
	}
	p.buf[p.next] = Line{Text: text, Stream: stream, At: at}
	p.next = (p.next + 1) % len(p.buf)
	if p.next == 0 {
		p.full = true
	}
}

// Lines returns the retained lines, oldest first.
func (p *Pane) Lines() []Line {
	p.mu.Lock()
	defer p.mu.Unlock()

	n := p.next
	if p.full {
		n = len(p.buf)
	}
	out := make([]Line, 0, n)
	if p.full {
		out = append(out, p.buf[p.next:]...)
	}
	return append(out, p.buf[:p.next]...)
}

// Capacity is the number of lines the pane retains.
func (p *Pane) Capacity() int { return len(p.buf) }

// Total is the number of lines accepted since start, filter applied.
func (p *Pane) Total() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.total
}

// Activity describes recent output, for the indicator light.
type Activity struct {
	// Out and Err are true when that stream produced a line within window.
	Out, Err bool
	// Seen is true once the process has ever produced output.
	Seen bool
}

// Activity reports which streams were active within window of now.
func (p *Pane) Activity(now time.Time, window time.Duration) Activity {
	p.mu.Lock()
	defer p.mu.Unlock()
	return Activity{
		Out:  !p.lastOut.IsZero() && now.Sub(p.lastOut) <= window,
		Err:  !p.lastErr.IsZero() && now.Sub(p.lastErr) <= window,
		Seen: p.total > 0,
	}
}
