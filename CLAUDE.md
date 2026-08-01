# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What gloncher is

A Go terminal UI tool that launches several long-running programs at once and multiplexes their
output into a single screen. Invoked as `gloncher <name.ini>`; the INI file is the entire
configuration — which commands to launch, and how each one's stdout/stderr is displayed.

Motivating use case is a local Laravel dev stack, all in one terminal:

| process                                   | display                         |
|-------------------------------------------|---------------------------------|
| `php artisan serve`                       | last 2 lines, full width        |
| `tail -f storage/logs/<most recent file>` | filter on `ERROR`, last 5 lines |
| `npm run dev`                             | output hidden                   |
| `php artisan queue:work`                  | last 2 lines, half width        |
| `php artisan schedule:work`               | last 2 lines, half width        |

## Package layout

- `main.go` — flags, signal handling, alt-screen setup/teardown, the render loop.
- `internal/config` — INI parsing. Section order is preserved; it *is* the layout order.
- `internal/pane` — per-process ring buffer of output lines plus activity tracking.
- `internal/layout` — pane widths to terminal rows.
- `internal/proc` — process supervision.
- `internal/ui` — frame rendering, ANSI, terminal size and raw mode.

Platform differences live in build-tagged files (`*_unix.go` / `*_windows.go`), never in
`runtime.GOOS` branches: process-group kill, the shell to use, raw mode, and resize signals.

## Core concepts

- **Process entry** — one INI section = one child process. Per-entry knobs: command line, number of
  output lines to show, width (full/half), an optional output filter (e.g. only lines matching
  `ERROR`), and the option to show nothing at all.
- **Exit policy** — `Program.OnExit` (`keep` / `restart` / `quit`) decides what happens when a
  process ends, and applies equally to a clean exit and a crash. `quit` is delivered through the
  `onFatal` callback `Group.Start` wires to the context cancel, so one process ending tears down
  the whole session. Cancellation always wins over a policy: a shutdown must not restart anything
  on the way out or look like a crash. Exactly one process can be `Status.Fatal`; `Group.Start`
  guards that with a `sync.Once`.
- **Activity light** — the first line of the UI carries one colored indicator per process. Dim when
  the process has produced no recent output; bright when stdout and/or stderr activity is detected.
  This is the at-a-glance health row, so keep the light logic independent of whether a process's
  output pane is visible (hidden processes still get a light).
- **Ring-buffered panes** — each process keeps only the last N lines it needs to render. Never
  accumulate full output in memory.
- **Layout** — panes are laid out by declared width (full row, or two half-width panes side by
  side) in INI order.

## Concurrency shape

Each child process runs under `exec.Cmd` with piped stdout/stderr, one scanner goroutine per pipe
feeding a shared update channel; the TUI renders on its own loop. Guard against the two classic
traps here: a chatty process must not starve rendering (coalesce updates, redraw on a tick rather
than per line), and shutdown must terminate the whole child process group — `php artisan serve`
and `npm run dev` both spawn grandchildren that outlive a plain `Kill()` on the parent.

## The template

`config.Template` in `internal/config/template.go` is the file `--init` writes. It lives beside
the parser deliberately: `TestTemplateParses` both parses it and asserts it still demonstrates a
filter, a half-width pane, a hidden program, and the restart and quit policies. Rename or drop a
key and that test fails, so the template cannot quietly drift out of date.

Its sample programs are shell loops, not real commands, so a generated file runs on a bare machine
— a template whose `on_exit = quit` program is missing would exit instantly and look broken. The
realistic commands (`php artisan serve`, `npm run dev`) sit next to them as comments.

`WriteTemplate` refuses to overwrite an existing file.

## Design decisions worth not re-litigating

- **No dependencies.** Everything is stdlib, including terminal sizing (`stty size`) and raw mode
  (`stty raw -echo` on unix, `SetConsoleMode` on Windows). This keeps `GOOS=… go build` trivial for
  all five release targets. Don't add a TUI framework without a concrete reason.
- **Shell by default.** `shell = yes` is the default because the useful command lines need it —
  `tail -f "$(ls -t storage/logs/*.log | head -1)"` is the motivating example. `shell = no` uses
  the quote-aware splitter in `proc.splitArgs`.
- **Redraw on a tick, not per line.** The renderer polls the panes at `refresh`; writers never
  trigger a draw. That is what keeps a chatty `npm run dev` from starving the screen.
- **Filtered lines don't light the lamp.** A pane filtered to `ERROR` stays dark until an actual
  error arrives — the filter applies in `Pane.Add`, before activity is recorded.
- **Stopped ≠ crashed.** `Status.Stopped` marks processes gloncher itself terminated, so shutdown
  doesn't paint every light red. `Status.Fatal` marks the one process whose `on_exit = quit` ended
  the session; `main` turns that into a non-zero exit code naming the culprit.

## Build & distribution

Cross-compiled to darwin/{amd64,arm64}, linux/{amd64,arm64} and windows/amd64. `bin/gloncher`
detects the host OS/arch and execs the matching binary, so the release layout and the names that
script expects must stay in sync — change one, change the other. `gloncher.sh` at the repo root is
a symlink to it, kept because the original spec and the docs name it. It stays at the root rather
than in `bin/` on purpose: two files in `bin/` would put both `gloncher` and `gloncher.sh` on the
user's PATH.

The launcher lives in `bin/` so `basher install pforret/gloncher` exposes it as `gloncher`, and
`package.sh` declares `BINS=bin/gloncher`. That declaration matters: with no `BINS`, basher globs
`bin/*` (see its `libexec/basher-_link-bins`) and publishes *every* file in `bin/` as a PATH
command, executable or not. Keep `bin/` to the one entry point, and add new helper scripts
elsewhere — or they become part of the public interface by accident. Basher
installs by symlinking the script onto PATH, so it resolves its own symlink chain (hand-rolled;
`readlink -f` is not portable to older macOS) before looking for binaries — `${BASH_SOURCE[0]}`
alone points at basher's bin directory, not the repo. It also refuses to exec anything that is
`-ef` itself, because release bundles symlink `dist/gloncher` back to the script.

`binaries/` holds prebuilt binaries committed to the repo, so `basher install` and a plain clone
work with no Go toolchain — basher forbids install hooks, and this is the alternative to building
at install time. Rebuild with `make binaries` on a version bump, not on every change: each rebuild
writes ~11 MB of fresh blobs into git history permanently. `make release` builds a standalone
`dist/` bundle instead, which is not committed.

When no binary is found at all and `go.mod` is present, the launcher builds one and execs it. That
covers platforms outside the committed matrix.

Search order matters: `$root/$name` (a local `go build` output) wins over `binaries/`, so a
developer's own build is never shadowed by a stale committed one.

```sh
make build                            # ./gloncher
make check                            # gofmt check + vet + tests
make release                          # dist/ for all five targets, plus gloncher.sh
make run                              # build and run examples/demo.ini

go test ./...
go test -run TestName ./internal/proc   # single test
./gloncher -check examples/laravel.ini  # parse an INI and print what it would run
```

`examples/demo.ini` is a self-contained stack (shell loops only, nothing to install) — use it to
exercise the UI rather than a real Laravel app.

## Testing notes

INI parsing, the ring buffer/filter, and layout are pure and fully unit-tested. `internal/proc`
tests spawn short-lived shell commands and are skipped on Windows.

`TestCancelKillsGrandchildren` is the load-bearing one: it backgrounds a `sleep` inside the shell
so a grandchild holds the output pipe open, and fails by hanging if the process-group kill
regresses. Verified to fail when `Setpgid`/`kill(-pgid)` is removed — keep it that way.

The exit policies have a test each (`TestExitKeep…`, `TestExitRestart…`, `TestExitQuit…`), plus
`TestQuitPolicyTakesDownTheWholeGroup`, which is the one that checks the survivors are reported as
stopped rather than crashed and that a restart-policy process does not respawn during shutdown.

`internal/ui` has no tests. `Renderer.Frame` is pure given a width, height and time, so it can be
golden-tested if the rendering gets more intricate.
