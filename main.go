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

// defaultTemplateName is what -init writes when given no file name.
const defaultTemplateName = "gloncher.ini"

// Header fields for the usage block, mirroring the bashew script layout.
const (
	programName = "gloncher"
	author      = "peter@forret.com"
	description = "run several programs at once in one terminal screen"
	homepage    = "https://github.com/pforret/gloncher"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gloncher:", err)
		os.Exit(1)
	}
}

// boolFlag registers a flag under both its long and short name. Go's flag
// package accepts one or two leading dashes for either, so this gives all of
// -v, --v, -version and --version.
func boolFlag(long, short, usage string) *bool {
	v := new(bool)
	flag.BoolVar(v, long, false, usage)
	flag.BoolVar(v, short, false, usage+" (shorthand)")
	return v
}

func run() error {
	showVersion := boolFlag("version", "v", "print version and exit")
	check := boolFlag("check", "c", "parse the INI file and exit")
	showHelp := boolFlag("help", "h", "print this help and exit")
	initFile := boolFlag("init", "i", "write a commented template INI file and exit")
	prompt := boolFlag("prompt", "p", "print a coding-agent prompt for configuring this project and exit")
	flag.Usage = usage
	flag.Parse()

	if *showHelp {
		usage()
		return nil
	}
	// Printed to stdout, not stderr like the usage: the point is to pipe it
	// into an agent, or into pbcopy.
	if *prompt {
		fmt.Print(config.Prompt)
		return nil
	}
	if *initFile {
		path := defaultTemplateName
		if flag.NArg() == 1 {
			path = flag.Arg(0)
		}
		if err := config.WriteTemplate(path); err != nil {
			return err
		}
		fmt.Printf("wrote %s — edit it, then run: gloncher %s\n", path, path)
		return nil
	}
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

// usage prints help in the same shape as pforret's bash scripts (bashew,
// setver): a Program/Updated/Description header, a one-line synopsis, then one
// aligned line per flag and parameter.
func usage() {
	out := os.Stderr
	fmt.Fprintf(out, "Program: %s %s by %s\n", programName, version, author)
	if built := buildTime(); !built.IsZero() {
		fmt.Fprintf(out, "Updated: %s\n", built.Format(time.ANSIC))
	}
	fmt.Fprintf(out, "Description: %s\n", description)
	fmt.Fprintf(out, "Homepage: %s\n", homepage)
	fmt.Fprintf(out, "Usage: %s [-h] [-v] [-c] [-i] [-p] <input.ini?>\n", programName)
	fmt.Fprintln(out, "Flags, options and parameters:")
	fmt.Fprintln(out, "    -h|--help        : [flag] show usage [default: off]")
	fmt.Fprintln(out, "    -v|--version     : [flag] show version and exit [default: off]")
	fmt.Fprintln(out, "    -c|--check       : [flag] parse the ini file, show what it would run [default: off]")
	fmt.Fprintln(out, "    -i|--init        : [flag] write a commented template ini file [default: off]")
	fmt.Fprintln(out, "    -p|--prompt      : [flag] print a prompt for a coding agent to configure this project [default: off]")
	fmt.Fprintf(out, "    <input>          : [parameter] ini file to run (with --init: file to write) [default: %s]\n", defaultTemplateName)
	fmt.Fprintln(out, "Keys, while running:")
	fmt.Fprintln(out, "    q|ctrl-c         : stop every program and quit")
}

// buildTime reports when the running binary was last written, so the header can
// show an Updated: line the way the bash scripts do with their own mtime.
func buildTime() time.Time {
	exe, err := os.Executable()
	if err != nil {
		return time.Time{}
	}
	fi, err := os.Stat(exe)
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}
