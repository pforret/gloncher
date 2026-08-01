// Package config parses gloncher INI files.
//
// A file consists of an optional [gloncher] global section followed by one
// section per process. Section order is preserved: it drives the on-screen
// layout order.
package config

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Width is how much of the terminal row a process pane occupies.
type Width int

const (
	// WidthFull makes the pane span the whole terminal row.
	WidthFull Width = iota
	// WidthHalf lets the pane share a row with the next half-width pane.
	WidthHalf
)

func (w Width) String() string {
	if w == WidthHalf {
		return "half"
	}
	return "full"
}

// ExitPolicy is what gloncher does when a program ends, whether it exited
// cleanly or crashed.
type ExitPolicy int

const (
	// ExitKeep leaves the program stopped and keeps everything else running.
	ExitKeep ExitPolicy = iota
	// ExitRestart respawns the program and counts the restart.
	ExitRestart
	// ExitQuit shuts down gloncher and every other program. Use it for the
	// process the session exists for: if the web server dies, the rest is
	// pointless.
	ExitQuit
)

func (e ExitPolicy) String() string {
	switch e {
	case ExitRestart:
		return "restart"
	case ExitQuit:
		return "quit"
	default:
		return "keep"
	}
}

// Program is one child process and how its output is displayed.
type Program struct {
	Name   string
	Cmd    string
	Dir    string
	Lines  int            // number of output lines to display; 0 means hidden
	Width  Width          //
	Filter *regexp.Regexp // when set, only matching lines are kept
	Shell  bool           // run via the system shell (default true)
	Color  string         // named color for the activity light
	Env    []string       // extra KEY=VALUE entries
	OnExit ExitPolicy     // what to do when the process ends
}

// Hidden reports whether the program's output pane is suppressed. Hidden
// programs still run and still get an activity light.
func (p Program) Hidden() bool { return p.Lines <= 0 }

// Config is a parsed INI file.
type Config struct {
	Dir      string // working directory applied to programs that don't set one
	Refresh  time.Duration
	Title    string
	Programs []Program
}

const (
	globalSection  = "gloncher"
	defaultRefresh = 100 * time.Millisecond
	defaultLines   = 2
)

// Load reads and parses the INI file at path.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cfg, err := Parse(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if cfg.Title == "" {
		cfg.Title = strings.TrimSuffix(filepathBase(path), ".ini")
	}
	return cfg, nil
}

func filepathBase(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

// Parse reads INI content. Keys are case-insensitive; `#` and `;` start a
// comment when they begin a line.
func Parse(r io.Reader) (*Config, error) {
	cfg := &Config{Refresh: defaultRefresh}

	var (
		section string
		current *Program
		lineNo  int
	)
	// flush appends the section being accumulated, if it is a program.
	flush := func() {
		if current != nil {
			cfg.Programs = append(cfg.Programs, *current)
			current = nil
		}
	}

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		if strings.HasPrefix(line, "[") {
			if !strings.HasSuffix(line, "]") {
				return nil, fmt.Errorf("line %d: unterminated section header %q", lineNo, line)
			}
			flush()
			section = strings.TrimSpace(line[1 : len(line)-1])
			if section == "" {
				return nil, fmt.Errorf("line %d: empty section name", lineNo)
			}
			if !strings.EqualFold(section, globalSection) {
				current = &Program{Name: section, Lines: defaultLines, Shell: true}
			}
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("line %d: expected key = value, got %q", lineNo, line)
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if section == "" {
			return nil, fmt.Errorf("line %d: %q appears before any section", lineNo, key)
		}

		if strings.EqualFold(section, globalSection) {
			if err := applyGlobal(cfg, key, value); err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNo, err)
			}
			continue
		}
		if err := applyProgram(current, key, value); err != nil {
			return nil, fmt.Errorf("line %d: [%s] %w", lineNo, section, err)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	flush()

	if len(cfg.Programs) == 0 {
		return nil, fmt.Errorf("no programs defined")
	}
	for i, p := range cfg.Programs {
		if p.Cmd == "" {
			return nil, fmt.Errorf("[%s] has no cmd", p.Name)
		}
		if p.Dir == "" {
			cfg.Programs[i].Dir = cfg.Dir
		}
	}
	return cfg, nil
}

func applyGlobal(cfg *Config, key, value string) error {
	switch key {
	case "dir":
		cfg.Dir = expand(value)
	case "title":
		cfg.Title = value
	case "refresh":
		d, err := parseDuration(value)
		if err != nil {
			return fmt.Errorf("refresh: %w", err)
		}
		cfg.Refresh = d
	default:
		return fmt.Errorf("unknown key %q in [%s]", key, globalSection)
	}
	return nil
}

func applyProgram(p *Program, key, value string) error {
	switch key {
	case "cmd", "command":
		p.Cmd = value
	case "dir":
		p.Dir = expand(value)
	case "lines":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("lines: %q is not a number", value)
		}
		if n < 0 {
			n = 0
		}
		p.Lines = n
	case "width":
		switch strings.ToLower(value) {
		case "full", "1", "":
			p.Width = WidthFull
		case "half", "0.5", "1/2":
			p.Width = WidthHalf
		default:
			return fmt.Errorf("width: want full or half, got %q", value)
		}
	case "filter":
		re, err := regexp.Compile(value)
		if err != nil {
			return fmt.Errorf("filter: %w", err)
		}
		p.Filter = re
	case "show":
		// show = false is shorthand for lines = 0
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("show: %w", err)
		}
		if !b {
			p.Lines = 0
		} else if p.Lines == 0 {
			p.Lines = defaultLines
		}
	case "shell":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("shell: %w", err)
		}
		p.Shell = b
	case "on_exit", "onexit":
		switch strings.ToLower(value) {
		case "keep", "stop", "nothing", "none":
			p.OnExit = ExitKeep
		case "restart":
			p.OnExit = ExitRestart
		case "quit", "exit", "shutdown", "fatal":
			p.OnExit = ExitQuit
		default:
			return fmt.Errorf("on_exit: want keep, restart or quit, got %q", value)
		}
	case "restart":
		// Kept as a shorthand for on_exit = restart.
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("restart: %w", err)
		}
		if b {
			p.OnExit = ExitRestart
		} else {
			p.OnExit = ExitKeep
		}
	case "color":
		p.Color = strings.ToLower(value)
	case "env":
		if !strings.Contains(value, "=") {
			return fmt.Errorf("env: want KEY=VALUE, got %q", value)
		}
		p.Env = append(p.Env, value)
	default:
		return fmt.Errorf("unknown key %q", key)
	}
	return nil
}

func parseBool(v string) (bool, error) {
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	}
	return false, fmt.Errorf("want yes/no, got %q", v)
}

// parseDuration accepts Go durations, and bare numbers as milliseconds.
func parseDuration(v string) (time.Duration, error) {
	if n, err := strconv.Atoi(v); err == nil {
		return time.Duration(n) * time.Millisecond, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("want a duration like 200ms, got %q", v)
	}
	return d, nil
}

func expand(v string) string {
	if strings.HasPrefix(v, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return home + v[1:]
		}
	}
	return os.ExpandEnv(v)
}
