package config

import (
	"os"
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

// The template is the first thing a new user sees, so it must parse, and it
// must exercise the keys it documents. If a key is renamed and the template
// is not updated, this fails.
func TestTemplateParses(t *testing.T) {
	cfg, err := Parse(strings.NewReader(Template))
	if err != nil {
		t.Fatalf("the generated template does not parse: %v", err)
	}
	if cfg.Title != "my project" {
		t.Errorf("Title = %q", cfg.Title)
	}
	if len(cfg.Programs) != 5 {
		t.Fatalf("got %d programs, want 5", len(cfg.Programs))
	}

	var (
		sawFilter, sawHalf, sawHidden bool
		sawRestart, sawQuit           bool
	)
	for _, p := range cfg.Programs {
		if p.Filter != nil {
			sawFilter = true
		}
		if p.Width == WidthHalf {
			sawHalf = true
		}
		if p.Hidden() {
			sawHidden = true
		}
		switch p.OnExit {
		case ExitRestart:
			sawRestart = true
		case ExitQuit:
			sawQuit = true
		}
	}
	for name, ok := range map[string]bool{
		"a filtered pane":   sawFilter,
		"a half-width pane": sawHalf,
		"a hidden program":  sawHidden,
		"on_exit = restart": sawRestart,
		"on_exit = quit":    sawQuit,
	} {
		if !ok {
			t.Errorf("the template no longer demonstrates %s", name)
		}
	}
}

func TestWriteTemplateRefusesToOverwrite(t *testing.T) {
	path := t.TempDir() + "/existing.ini"
	if err := os.WriteFile(path, []byte("[mine]\ncmd = precious\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteTemplate(path); err == nil {
		t.Fatal("WriteTemplate overwrote an existing file")
	}
	// The original must be untouched.
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "[mine]\ncmd = precious\n" {
		t.Errorf("the existing file was modified: %q", got)
	}
}

func TestWriteTemplateThenLoad(t *testing.T) {
	path := t.TempDir() + "/new.ini"
	if err := WriteTemplate(path); err != nil {
		t.Fatalf("WriteTemplate: %v", err)
	}
	if _, err := Load(path); err != nil {
		t.Errorf("the file just written does not load: %v", err)
	}
}

// The agent prompt describes the INI format from memory, so it can go stale the
// moment a key is renamed. Assert that every key the parser understands is
// named in it, and that nothing it names has been dropped.
func TestPromptCoversEveryKey(t *testing.T) {
	keys := []string{
		"title", "dir", "refresh",
		"cmd", "lines", "show", "width", "filter", "env", "color", "shell", "on_exit",
		"keep", "restart", "quit", "full", "half",
	}
	for _, k := range keys {
		if !strings.Contains(Prompt, k) {
			t.Errorf("Prompt never mentions %q", k)
		}
	}
	// Everything the prompt tells an agent to run must still be a real flag.
	for _, cmd := range []string{"gloncher -c ", "gloncher -i ", "gloncher dev.ini"} {
		if !strings.Contains(Prompt, cmd) {
			t.Errorf("Prompt no longer shows %q", cmd)
		}
	}
}
