package proc

import (
	"context"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/pforret/gloncher/internal/config"
)

func TestSplitArgs(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{`php artisan serve`, []string{"php", "artisan", "serve"}},
		{`  spaced   out  `, []string{"spaced", "out"}},
		{`say "hello world"`, []string{"say", "hello world"}},
		{`say 'it''s fine'`, []string{"say", "its fine"}},
		{``, nil},
	}
	for _, c := range cases {
		got := splitArgs(c.in)
		if len(got) != len(c.want) {
			t.Errorf("splitArgs(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("splitArgs(%q) = %v, want %v", c.in, got, c.want)
				break
			}
		}
	}
}

func TestRunCapturesBothStreams(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell command")
	}
	p := New(config.Program{
		Name:  "t",
		Cmd:   "echo out; echo err 1>&2",
		Lines: 10,
		Shell: true,
	})
	p.Run(context.Background(), nil)

	var out, errs []string
	for _, l := range p.Pane.Lines() {
		if l.Stream == 1 {
			errs = append(errs, l.Text)
		} else {
			out = append(out, l.Text)
		}
	}
	if len(out) != 1 || out[0] != "out" {
		t.Errorf("stdout = %v, want [out]", out)
	}
	if len(errs) != 1 || errs[0] != "err" {
		t.Errorf("stderr = %v, want [err]", errs)
	}
	if st := p.Status(); st.State != Exited || st.ExitCode != 0 {
		t.Errorf("status = %+v, want a clean exit", st)
	}
}

func TestRunRecordsExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell command")
	}
	p := New(config.Program{Name: "t", Cmd: "exit 3", Lines: 1, Shell: true})
	p.Run(context.Background(), nil)
	if st := p.Status(); st.ExitCode != 3 {
		t.Errorf("ExitCode = %d, want 3", st.ExitCode)
	}
}

func TestFilterAppliesToLiveOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell command")
	}
	p := New(config.Program{
		Name:   "t",
		Cmd:    "echo info; echo ERROR bad; echo more",
		Lines:  5,
		Shell:  true,
		Filter: regexp.MustCompile("ERROR"),
	})
	p.Run(context.Background(), nil)
	lines := p.Pane.Lines()
	if len(lines) != 1 || !strings.Contains(lines[0].Text, "ERROR") {
		t.Errorf("lines = %v, want only the ERROR line", lines)
	}
}

// A long-running process must stop when the context is cancelled, and Run must
// return rather than block on a pipe that a surviving grandchild holds open.
func TestCancelStopsLongRunningProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell command")
	}
	p := New(config.Program{Name: "t", Cmd: "sleep 30", Lines: 1, Shell: true})
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() { p.Run(ctx, nil); close(done) }()

	time.Sleep(150 * time.Millisecond)
	if st := p.Status(); st.State != Running {
		t.Fatalf("state = %v, want running", st.State)
	}
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after the context was cancelled")
	}
}

// Killing the shell must take its children with it: the grandchild here holds
// the output pipe open, so a leak shows up as Run blocking forever.
func TestCancelKillsGrandchildren(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell command")
	}
	p := New(config.Program{Name: "t", Cmd: "sh -c 'sleep 30' & wait", Lines: 1, Shell: true})
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() { p.Run(ctx, nil); close(done) }()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a grandchild survived and held the output pipe open")
	}
}

func TestGroupStartsEveryProgram(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell command")
	}
	cfg := &config.Config{Programs: []config.Program{
		{Name: "a", Cmd: "echo a", Lines: 1, Shell: true},
		{Name: "b", Cmd: "echo b", Lines: 1, Shell: true},
	}}
	g := NewGroup(cfg)
	g.Start(context.Background(), nil)
	if !g.WaitTimeout(5 * time.Second) {
		t.Fatal("processes did not finish")
	}
	for _, p := range g.Processes {
		if got := p.Pane.Total(); got != 1 {
			t.Errorf("%s captured %d lines, want 1", p.Program.Name, got)
		}
	}
}

func TestStartFailureIsVisible(t *testing.T) {
	p := New(config.Program{Name: "t", Cmd: "definitely-not-a-real-binary", Lines: 5, Shell: false})
	p.Run(context.Background(), nil)
	st := p.Status()
	if st.Err == nil {
		t.Fatal("expected an error for a missing binary")
	}
	if p.Pane.Total() == 0 {
		t.Error("the start failure should also be written to the pane")
	}
}

// --- exit policies ---------------------------------------------------------

func TestExitKeepLeavesProcessStopped(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell command")
	}
	p := New(config.Program{Name: "t", Cmd: "exit 1", Lines: 1, Shell: true, OnExit: config.ExitKeep})

	fired := false
	p.Run(context.Background(), func(*Process) { fired = true })

	if fired {
		t.Error("on_exit = keep must not take the session down")
	}
	st := p.Status()
	if st.State != Exited || st.ExitCode != 1 {
		t.Errorf("status = %+v, want exited with code 1", st)
	}
	if st.Restarts != 0 {
		t.Errorf("Restarts = %d, want 0", st.Restarts)
	}
}

func TestExitRestartRespawnsAndCounts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell command")
	}
	p := New(config.Program{Name: "t", Cmd: "echo up", Lines: 4, Shell: true, OnExit: config.ExitRestart})
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() { p.Run(ctx, nil); close(done) }()

	// restartDelay is a second, so this window covers two respawns.
	time.Sleep(2500 * time.Millisecond)
	got := p.Status().Restarts
	cancel()
	<-done

	if got < 2 {
		t.Errorf("Restarts = %d after 2.5s, want at least 2", got)
	}
	if total := p.Pane.Total(); total < uint64(got) {
		t.Errorf("captured %d lines across %d restarts, want output from each run", total, got)
	}
}

func TestRestartStopsOnCancel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell command")
	}
	p := New(config.Program{Name: "t", Cmd: "exit 0", Lines: 1, Shell: true, OnExit: config.ExitRestart})
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() { p.Run(ctx, nil); close(done) }()

	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("a restarting process kept respawning after cancellation")
	}
}

func TestExitQuitFiresOnFatal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell command")
	}
	p := New(config.Program{Name: "t", Cmd: "exit 2", Lines: 1, Shell: true, OnExit: config.ExitQuit})

	var got *Process
	p.Run(context.Background(), func(f *Process) { got = f })

	if got != p {
		t.Fatal("on_exit = quit did not report the process that ended")
	}
	if st := p.Status(); !st.Fatal || st.ExitCode != 2 {
		t.Errorf("status = %+v, want Fatal with exit code 2", st)
	}
}

// A clean exit counts too: on_exit = quit is about the program ending at all,
// not only about it crashing.
func TestExitQuitFiresOnCleanExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell command")
	}
	p := New(config.Program{Name: "t", Cmd: "true", Lines: 1, Shell: true, OnExit: config.ExitQuit})
	fired := false
	p.Run(context.Background(), func(*Process) { fired = true })
	if !fired {
		t.Error("on_exit = quit should fire on a clean exit as well")
	}
}

func TestQuitPolicyTakesDownTheWholeGroup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell command")
	}
	cfg := &config.Config{Programs: []config.Program{
		{Name: "server", Cmd: "sleep 0.3; exit 1", Lines: 1, Shell: true, OnExit: config.ExitQuit},
		{Name: "worker", Cmd: "sleep 30", Lines: 1, Shell: true, OnExit: config.ExitKeep},
		{Name: "looper", Cmd: "sleep 30", Lines: 1, Shell: true, OnExit: config.ExitRestart},
	}}
	g := NewGroup(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	g.Start(ctx, cancel)

	if !g.WaitTimeout(10 * time.Second) {
		t.Fatal("the group did not shut down after the quit policy fired")
	}
	if got := g.FatalProcess(); got == nil || got.Program.Name != "server" {
		t.Fatalf("FatalProcess() = %v, want server", got)
	}
	// The survivors were stopped by us, so they must not look like crashes,
	// and the restart-policy one must not have respawned on the way out.
	for _, p := range g.Processes[1:] {
		st := p.Status()
		if !st.Stopped {
			t.Errorf("%s: Stopped = false, want true", p.Program.Name)
		}
		if st.Fatal {
			t.Errorf("%s: Fatal = true, only the causing process should be fatal", p.Program.Name)
		}
		if st.Restarts != 0 {
			t.Errorf("%s: restarted %d times during shutdown", p.Program.Name, st.Restarts)
		}
	}
}

// Only the first quit-policy process to end should be reported as the cause.
func TestFatalProcessIsRecordedOnce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell command")
	}
	cfg := &config.Config{Programs: []config.Program{
		{Name: "first", Cmd: "exit 1", Lines: 1, Shell: true, OnExit: config.ExitQuit},
		{Name: "second", Cmd: "sleep 0.5; exit 1", Lines: 1, Shell: true, OnExit: config.ExitQuit},
	}}
	g := NewGroup(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	g.Start(ctx, cancel)

	if !g.WaitTimeout(10 * time.Second) {
		t.Fatal("group did not shut down")
	}
	if got := g.FatalProcess(); got == nil || got.Program.Name != "first" {
		t.Fatalf("FatalProcess() = %v, want first", got)
	}
}
