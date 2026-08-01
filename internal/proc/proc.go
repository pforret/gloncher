// Package proc supervises the child processes described by a config file.
package proc

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/pforret/gloncher/internal/config"
	"github.com/pforret/gloncher/internal/pane"
)

// State is where a process is in its lifecycle.
type State int

const (
	// Starting means the process has not been spawned yet.
	Starting State = iota
	// Running means the process is alive.
	Running
	// Exited means the process finished, cleanly or not.
	Exited
)

func (s State) String() string {
	switch s {
	case Running:
		return "running"
	case Exited:
		return "exited"
	default:
		return "starting"
	}
}

// restartDelay is how long to wait before respawning a process configured
// with on_exit = restart. It keeps a command that fails instantly from
// spinning the CPU.
const restartDelay = time.Second

// Process is one supervised child.
type Process struct {
	Program config.Program
	Pane    *pane.Pane

	mu       sync.Mutex
	state    State
	pid      int
	exitErr  error
	exitCode int
	restarts int
	stopped  bool // we asked it to stop; its exit status is not a failure
	fatal    bool // its on_exit = quit policy fired and took the session down
}

// New creates a supervised process. It does not start it.
func New(p config.Program) *Process {
	return &Process{Program: p, Pane: pane.New(p.Lines, p.Filter)}
}

// Status is a snapshot of a process, for rendering.
type Status struct {
	State    State
	PID      int
	ExitCode int
	Err      error
	Restarts int
	// Stopped is true when gloncher terminated the process on shutdown, so
	// callers can tell a deliberate stop from a crash.
	Stopped bool
	// Fatal is true for the process whose on_exit = quit policy ended the
	// session. Exactly one process can be the cause; the rest are Stopped.
	Fatal bool
}

// Status returns the current lifecycle snapshot.
func (p *Process) Status() Status {
	p.mu.Lock()
	defer p.mu.Unlock()
	return Status{
		State: p.state, PID: p.pid, ExitCode: p.exitCode,
		Err: p.exitErr, Restarts: p.restarts, Stopped: p.stopped, Fatal: p.fatal,
	}
}

// Run starts the process and blocks until it exits and both of its output
// pipes are drained, or until ctx is cancelled. What happens when the process
// ends is decided by the program's on_exit policy:
//
//   - keep:    Run returns, leaving the process shown as stopped.
//   - restart: Run respawns it after restartDelay, counting the restart.
//   - quit:    Run calls onFatal, which is expected to cancel ctx and so take
//     down gloncher and every other process.
//
// A process gloncher itself stopped never triggers a policy: shutdown must not
// look like a crash, and must not restart anything on the way out. onFatal may
// be nil.
func (p *Process) Run(ctx context.Context, onFatal func(*Process)) {
	for {
		p.runOnce(ctx)

		// Cancellation wins over any policy: we are already shutting down.
		if ctx.Err() != nil || p.Status().Stopped {
			return
		}

		switch p.Program.OnExit {
		case config.ExitQuit:
			p.mu.Lock()
			p.fatal = true
			p.mu.Unlock()
			if onFatal != nil {
				onFatal(p)
			}
			return
		case config.ExitRestart:
			select {
			case <-ctx.Done():
				return
			case <-time.After(restartDelay):
			}
			p.mu.Lock()
			p.restarts++
			p.mu.Unlock()
		default: // config.ExitKeep
			return
		}
	}
}

func (p *Process) runOnce(ctx context.Context) {
	cmd := p.command()
	cmd.Dir = p.Program.Dir
	if len(p.Program.Env) > 0 {
		cmd.Env = append(os.Environ(), p.Program.Env...)
	}
	setProcessGroup(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		p.finish(err, -1)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		p.finish(err, -1)
		return
	}

	if err := cmd.Start(); err != nil {
		p.note(fmt.Sprintf("failed to start: %v", err))
		p.finish(err, -1)
		return
	}

	p.mu.Lock()
	p.state = Running
	p.pid = cmd.Process.Pid
	p.exitErr = nil
	p.exitCode = 0
	p.mu.Unlock()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); p.scan(stdout, pane.Stdout) }()
	go func() { defer wg.Done(); p.scan(stderr, pane.Stderr) }()

	// Terminate the whole process group on cancellation: `php artisan serve`
	// and `npm run dev` both spawn grandchildren that survive killing only
	// the direct child.
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			p.mu.Lock()
			p.stopped = true
			p.mu.Unlock()
			terminate(cmd)
		case <-done:
		}
	}()

	wg.Wait() // pipes closed => no more output
	err = cmd.Wait()
	close(done)

	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		code = -1
	}
	p.finish(err, code)
}

// command builds the exec.Cmd. Shell mode is the default because the useful
// command lines here rely on it (pipes, globs, and picking the newest log
// file with a command substitution).
func (p *Process) command() *exec.Cmd {
	if p.Program.Shell {
		shell, flag := shellCommand()
		return exec.Command(shell, flag, p.Program.Cmd)
	}
	args := splitArgs(p.Program.Cmd)
	if len(args) == 0 {
		return exec.Command("false")
	}
	return exec.Command(args[0], args[1:]...)
}

func (p *Process) scan(r io.Reader, stream pane.Stream) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		p.Pane.Add(sc.Text(), stream, time.Now())
	}
}

// note records a gloncher-generated message in the pane, so start failures
// are visible on screen rather than only in the exit status.
func (p *Process) note(msg string) {
	p.Pane.Add("gloncher: "+msg, pane.Stderr, time.Now())
}

func (p *Process) finish(err error, code int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state = Exited
	p.exitErr = err
	p.exitCode = code
	p.pid = 0
}

// Group supervises all processes of a config.
type Group struct {
	Processes []*Process
	wg        sync.WaitGroup

	fatalOnce sync.Once
	fatalProc *Process
	fatalMu   sync.Mutex
}

// NewGroup builds a supervised process per program, in declaration order.
func NewGroup(cfg *config.Config) *Group {
	g := &Group{}
	for _, prog := range cfg.Programs {
		g.Processes = append(g.Processes, New(prog))
	}
	return g
}

// Start launches every process and returns immediately. shutdown is called at
// most once, when a process with on_exit = quit ends; it is expected to cancel
// the context Start was given, which stops every other process.
func (g *Group) Start(ctx context.Context, shutdown func()) {
	onFatal := func(p *Process) {
		g.fatalOnce.Do(func() {
			g.fatalMu.Lock()
			g.fatalProc = p
			g.fatalMu.Unlock()
			if shutdown != nil {
				shutdown()
			}
		})
	}
	for _, p := range g.Processes {
		g.wg.Add(1)
		go func(p *Process) {
			defer g.wg.Done()
			p.Run(ctx, onFatal)
		}(p)
	}
}

// FatalProcess returns the process whose on_exit = quit policy ended the
// session, or nil if the session ended for any other reason.
func (g *Group) FatalProcess() *Process {
	g.fatalMu.Lock()
	defer g.fatalMu.Unlock()
	return g.fatalProc
}

// Wait blocks until every process has exited and its output is drained.
func (g *Group) Wait() { g.wg.Wait() }

// WaitTimeout waits for all processes, giving up after d. It reports whether
// everything shut down in time.
func (g *Group) WaitTimeout(d time.Duration) bool {
	done := make(chan struct{})
	go func() { g.wg.Wait(); close(done) }()
	select {
	case <-done:
		return true
	case <-time.After(d):
		return false
	}
}

// splitArgs splits a command line on whitespace, honouring single and double
// quotes. Only used when shell = no.
func splitArgs(s string) []string {
	var (
		args  []string
		cur   []rune
		quote rune
		have  bool
	)
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur = append(cur, r)
			}
		case r == '\'' || r == '"':
			quote = r
			have = true
		case r == ' ' || r == '\t':
			if have {
				args = append(args, string(cur))
				cur, have = cur[:0], false
			}
		default:
			cur = append(cur, r)
			have = true
		}
	}
	if have {
		args = append(args, string(cur))
	}
	return args
}
