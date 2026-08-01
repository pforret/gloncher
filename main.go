// Command gloncher launches several programs at once and shows their output
// in one terminal screen, driven by an INI file.
//
// Usage: gloncher <name.ini>
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/pforret/gloncher/internal/config"
	"github.com/pforret/gloncher/internal/proc"
	"github.com/pforret/gloncher/internal/ui"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

// shutdownGrace bounds how long we wait for children after asking them to stop.
const shutdownGrace = 5 * time.Second

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gloncher:", err)
		os.Exit(1)
	}
}

func run() error {
	showVersion := flag.Bool("version", false, "print version and exit")
	check := flag.Bool("check", false, "parse the INI file and exit")
	flag.Usage = usage
	flag.Parse()

	if *showVersion {
		fmt.Println("gloncher", version)
		return nil
	}
	if flag.NArg() != 1 {
		flag.Usage()
		return fmt.Errorf("expected exactly one .ini file")
	}

	cfg, err := config.Load(flag.Arg(0))
	if err != nil {
		return err
	}
	if *check {
		describe(cfg)
		return nil
	}

	group := proc.NewGroup(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigs
		cancel()
	}()

	// A raw terminal lets a bare `q` quit without the user pressing enter. If
	// the terminal cannot be put in raw mode (piped stdin, no tty) we simply
	// run without keyboard input; ctrl-c still works.
	restore, raw := ui.MakeRaw()
	if raw {
		go watchKeys(ctx, cancel)
	}

	out := bufio.NewWriterSize(os.Stdout, 64*1024)
	enterScreen(out)

	group.Start(ctx, cancel)
	render(ctx, out, cfg, group)

	cancel()
	if !group.WaitTimeout(shutdownGrace) {
		defer fmt.Fprintln(os.Stderr, "gloncher: some processes did not exit in time")
	}

	leaveScreen(out)
	if restore != nil {
		restore()
	}
	summarize(group)

	// A program with on_exit = quit ending is the reason the session is over;
	// report it as a failure so scripts wrapping gloncher can see it.
	if fatal := group.FatalProcess(); fatal != nil {
		st := fatal.Status()
		return fmt.Errorf("%s ended (exit %d) and its on_exit policy is quit",
			fatal.Program.Name, st.ExitCode)
	}
	return nil
}

// render redraws on a ticker rather than per output line, so a chatty process
// cannot starve the screen.
func render(ctx context.Context, out *bufio.Writer, cfg *config.Config, group *proc.Group) {
	r := ui.New(cfg, group, time.Now())
	tick := time.NewTicker(cfg.Refresh)
	defer tick.Stop()

	// Re-measure the terminal on resize rather than on every frame; stty is a
	// subprocess and far too costly to run at the refresh rate.
	width, height := ui.TermSize()
	resize := make(chan os.Signal, 1)
	notifyResize(resize)

	for {
		out.WriteString(r.Frame(width, height, time.Now()))
		out.Flush()

		select {
		case <-ctx.Done():
			// One final frame so the last output is on screen.
			out.WriteString(r.Frame(width, height, time.Now()))
			out.Flush()
			return
		case <-resize:
			width, height = ui.TermSize()
		case <-tick.C:
		}
	}
}

// watchKeys quits on q or ctrl-c read from a raw terminal.
func watchKeys(ctx context.Context, cancel context.CancelFunc) {
	buf := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil || ctx.Err() != nil {
			return
		}
		if n == 1 && (buf[0] == 'q' || buf[0] == 'Q' || buf[0] == 3) {
			cancel()
			return
		}
	}
}

func enterScreen(out *bufio.Writer) {
	out.WriteString("\x1b[?1049h\x1b[?25l\x1b[2J\x1b[H")
	out.Flush()
}

func leaveScreen(out *bufio.Writer) {
	out.WriteString("\x1b[?25h\x1b[?1049l")
	out.Flush()
}

// summarize prints, on the normal screen after the UI is torn down, what each
// process ended up doing. Without it a crash scrolls away with the alt screen.
func summarize(group *proc.Group) {
	for _, p := range group.Processes {
		st := p.Status()
		switch {
		case st.Fatal:
			fmt.Printf("%-20s exit %d — took the session down (on_exit = quit)\n",
				p.Program.Name, st.ExitCode)
		case st.Stopped:
			fmt.Printf("%-20s stopped\n", p.Program.Name)
		case st.Err != nil && st.ExitCode > 0:
			fmt.Printf("%-20s exit %d\n", p.Program.Name, st.ExitCode)
		case st.Err != nil:
			fmt.Printf("%-20s %v\n", p.Program.Name, st.Err)
		default:
			fmt.Printf("%-20s ok\n", p.Program.Name)
		}
	}
}

func describe(cfg *config.Config) {
	fmt.Printf("%s — %d programs, refresh %s\n", cfg.Title, len(cfg.Programs), cfg.Refresh)
	for _, p := range cfg.Programs {
		show := fmt.Sprintf("%d lines, %s", p.Lines, p.Width)
		if p.Hidden() {
			show = "hidden"
		}
		if p.Filter != nil {
			show += ", filter /" + p.Filter.String() + "/"
		}
		show += ", on exit: " + p.OnExit.String()
		fmt.Printf("  %-16s %s\n      %s\n", p.Name, show, p.Cmd)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, strings.TrimLeft(`
gloncher — run several programs at once in one terminal screen

usage:
  gloncher [flags] <name.ini>

flags:
  -check      parse the INI file, print what it would run, and exit
  -version    print version and exit

keys:
  q, ctrl-c   stop every program and quit
`, "\n"))
}
