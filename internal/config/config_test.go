package config

import (
	"strings"
	"testing"
	"time"
)

const laravel = `
[gloncher]
title = laravel
refresh = 200ms

[server]
cmd = php artisan serve
lines = 2
width = full

[logs]
cmd = tail -f storage/logs/laravel.log
lines = 5
filter = ERROR

[vite]
cmd = npm run dev
show = no

[queue]
cmd = php artisan queue:work
width = half

[schedule]
cmd = php artisan schedule:work
width = half
`

func TestParseLaravelExample(t *testing.T) {
	cfg, err := Parse(strings.NewReader(laravel))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Title != "laravel" {
		t.Errorf("Title = %q, want laravel", cfg.Title)
	}
	if cfg.Refresh != 200*time.Millisecond {
		t.Errorf("Refresh = %v, want 200ms", cfg.Refresh)
	}
	if len(cfg.Programs) != 5 {
		t.Fatalf("got %d programs, want 5", len(cfg.Programs))
	}

	// Declaration order drives layout order, so it must be preserved.
	want := []string{"server", "logs", "vite", "queue", "schedule"}
	for i, name := range want {
		if cfg.Programs[i].Name != name {
			t.Errorf("program %d = %q, want %q", i, cfg.Programs[i].Name, name)
		}
	}

	logs := cfg.Programs[1]
	if logs.Lines != 5 {
		t.Errorf("logs.Lines = %d, want 5", logs.Lines)
	}
	if logs.Filter == nil || !logs.Filter.MatchString("ERROR: boom") {
		t.Errorf("logs.Filter did not match an ERROR line")
	}
	if vite := cfg.Programs[2]; !vite.Hidden() {
		t.Errorf("vite should be hidden, Lines = %d", vite.Lines)
	}
	if q := cfg.Programs[3]; q.Width != WidthHalf {
		t.Errorf("queue.Width = %v, want half", q.Width)
	}
}

func TestParseDefaults(t *testing.T) {
	cfg, err := Parse(strings.NewReader("[a]\ncmd = echo hi\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	p := cfg.Programs[0]
	if p.Lines != defaultLines {
		t.Errorf("Lines = %d, want %d", p.Lines, defaultLines)
	}
	if !p.Shell {
		t.Error("Shell should default to true")
	}
	if p.Width != WidthFull {
		t.Error("Width should default to full")
	}
	if cfg.Refresh != defaultRefresh {
		t.Errorf("Refresh = %v, want %v", cfg.Refresh, defaultRefresh)
	}
}

func TestGlobalDirIsInheritedNotOverridden(t *testing.T) {
	cfg, err := Parse(strings.NewReader(
		"[gloncher]\ndir = /srv/app\n\n[a]\ncmd = x\n\n[b]\ncmd = y\ndir = /other\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Programs[0].Dir != "/srv/app" {
		t.Errorf("a.Dir = %q, want the global dir", cfg.Programs[0].Dir)
	}
	if cfg.Programs[1].Dir != "/other" {
		t.Errorf("b.Dir = %q, want its own dir", cfg.Programs[1].Dir)
	}
}

func TestShowYesRestoresDefaultLines(t *testing.T) {
	cfg, err := Parse(strings.NewReader("[a]\ncmd = x\nlines = 0\nshow = yes\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Programs[0].Hidden() {
		t.Error("show = yes after lines = 0 should make the pane visible")
	}
}

func TestExitPolicies(t *testing.T) {
	cfg, err := Parse(strings.NewReader(`
[a]
cmd = x
[b]
cmd = x
on_exit = restart
[c]
cmd = x
on_exit = quit
[d]
cmd = x
on_exit = keep
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []ExitPolicy{ExitKeep, ExitRestart, ExitQuit, ExitKeep}
	for i, w := range want {
		if got := cfg.Programs[i].OnExit; got != w {
			t.Errorf("program %s: OnExit = %v, want %v", cfg.Programs[i].Name, got, w)
		}
	}
}

func TestExitPolicyAliases(t *testing.T) {
	cases := map[string]ExitPolicy{
		"on_exit = stop":     ExitKeep,
		"on_exit = nothing":  ExitKeep,
		"on_exit = QUIT":     ExitQuit,
		"on_exit = exit":     ExitQuit,
		"on_exit = shutdown": ExitQuit,
		"restart = yes":      ExitRestart,
		"restart = no":       ExitKeep,
	}
	for line, want := range cases {
		cfg, err := Parse(strings.NewReader("[a]\ncmd = x\n" + line + "\n"))
		if err != nil {
			t.Errorf("%q: %v", line, err)
			continue
		}
		if got := cfg.Programs[0].OnExit; got != want {
			t.Errorf("%q: OnExit = %v, want %v", line, got, want)
		}
	}
}

func TestBadExitPolicyIsRejected(t *testing.T) {
	if _, err := Parse(strings.NewReader("[a]\ncmd = x\non_exit = explode\n")); err == nil {
		t.Error("expected an error for an unknown on_exit value")
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		"no programs":      "[gloncher]\ntitle = x\n",
		"missing cmd":      "[a]\nlines = 2\n",
		"unknown key":      "[a]\ncmd = x\nnope = 1\n",
		"unknown global":   "[gloncher]\nnope = 1\n",
		"bad width":        "[a]\ncmd = x\nwidth = quarter\n",
		"bad regexp":       "[a]\ncmd = x\nfilter = [\n",
		"bad lines":        "[a]\ncmd = x\nlines = many\n",
		"key before block": "cmd = x\n",
		"unterminated":     "[a\ncmd = x\n",
		"no equals":        "[a]\ncmd\n",
	}
	for name, input := range cases {
		if _, err := Parse(strings.NewReader(input)); err == nil {
			t.Errorf("%s: expected an error, got none", name)
		}
	}
}

func TestCommentsAndBlankLines(t *testing.T) {
	cfg, err := Parse(strings.NewReader("# a comment\n; another\n\n[a]\ncmd = echo hi # not a comment\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// Trailing `#` is part of the value: shell commands legitimately contain it.
	if got := cfg.Programs[0].Cmd; got != "echo hi # not a comment" {
		t.Errorf("Cmd = %q, want the whole line after =", got)
	}
}

func TestValueMayContainEquals(t *testing.T) {
	cfg, err := Parse(strings.NewReader("[a]\ncmd = FOO=1 php artisan serve\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := cfg.Programs[0].Cmd; got != "FOO=1 php artisan serve" {
		t.Errorf("Cmd = %q", got)
	}
}
