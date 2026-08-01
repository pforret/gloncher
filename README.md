[![basher install](https://www.basher.it/assets/logo/basher_install.svg)](https://www.basher.it/package/)

# gloncher

Launch several programs at once and watch them in one terminal screen.

Local development usually means four or five terminal tabs: a web server, a log
tail, an asset builder, a queue worker, a scheduler. `gloncher` runs them all
from one INI file and puts them on one screen — each with its own small output
pane and a colored light that brightens whenever the program says something.

```
gloncher examples/laravel.ini
```

```
laravel  ● server  ● logs  ● vite·  ● queue  ● schedule
3/5 running · up 1m12s · q or ctrl-c to quit

server ──────────────────────────────────────────────────────────
  INFO  Server running on [http://127.0.0.1:8000].
  Press Ctrl+C to stop the server

logs /ERROR/ ────────────────────────────────────────────────────
  [2026-08-01 09:14:02] local.ERROR: Undefined variable $user

queue ───────────────────────  schedule ─────────────────────────
  Processing: App\Jobs\Sync     Running [0 * * * *] cache:prune
  Processed:  App\Jobs\Sync     Done
```

## Install

With [basher](https://www.basher.it):

```sh
basher install pforret/gloncher
```

Basher puts a single `gloncher` command on your PATH. Prebuilt binaries for macOS, Linux and
Windows are committed to the repo, so this needs no Go toolchain. On any other
platform the first run builds one, which does need Go.

Or download a binary from the releases, or build from source:

```sh
git clone https://github.com/pforret/gloncher
cd gloncher
make build          # produces ./gloncher
```

`bin/gloncher` (also reachable as `gloncher.sh` at the repo root) detects the
OS and architecture and runs the matching binary, so you can ship one directory
that works everywhere:

```sh
make release        # dist/gloncher-darwin-arm64, -linux-amd64, -windows-amd64.exe, …
./gloncher.sh my-app.ini
```

In a source checkout with Go installed, `./gloncher.sh` builds the binary on
first use, so it works straight after a clone with no build step.

## The INI file

Start from a commented template rather than a blank file:

```sh
gloncher --init            # writes gloncher.ini
gloncher --init myapp.ini  # or pick the name
```

The generated file documents every key, runs as-is (the sample programs are
plain shell loops), and has real-world commands next to them, commented out.
It will not overwrite a file that already exists.

One section per program; section order is screen order. An optional
`[gloncher]` section holds global settings.

```ini
[gloncher]
title   = laravel
dir     = ~/Code/my-app     ; working directory for every program
refresh = 100ms             ; redraw interval

[server]
cmd   = php artisan serve
lines = 2                   ; show the last 2 lines
width = full

[logs]
cmd    = tail -f "$(ls -t storage/logs/*.log | head -1)"
lines  = 5
filter = ERROR              ; only keep lines matching this regexp

[vite]
cmd  = npm run dev
show = no                   ; runs, but its output is not displayed

[queue]
cmd   = php artisan queue:work
width = half                ; shares a row with the next half-width program
```

### Program keys

| key | default | meaning |
| --- | --- | --- |
| `cmd` | required | the command line to run |
| `lines` | `2` | output lines to display; `0` hides the pane |
| `show` | `yes` | `no` is shorthand for `lines = 0` |
| `width` | `full` | `full` or `half` (two consecutive halves share a row) |
| `filter` | none | regexp; non-matching lines are dropped, and don't light the indicator |
| `dir` | global `dir` | working directory |
| `env` | none | extra `KEY=VALUE`; repeat the key for several |
| `color` | auto | `red`, `green`, `yellow`, `blue`, `magenta`, `cyan`, `white` |
| `shell` | `yes` | run through the shell, so pipes, globs and `$(…)` work |
| `on_exit` | `keep` | what to do when the program ends — see below |

### Global keys

| key | default | meaning |
| --- | --- | --- |
| `title` | file name | shown at the left of the lights row |
| `dir` | current directory | working directory inherited by programs |
| `refresh` | `100ms` | redraw interval; a bare number means milliseconds |

## When a program ends

Every program has an `on_exit` policy that applies whether it exited cleanly or
crashed:

| `on_exit` | what happens |
| --- | --- |
| `keep` *(default)* | nothing — the program shows as stopped, everything else keeps running |
| `restart` | respawn it after a second; the pane title shows the restart count (`queue ×3`) |
| `quit` | stop every other program and exit gloncher |

```ini
[server]
cmd     = php artisan serve
on_exit = quit           ; the session exists to run this — no server, no point

[queue]
cmd     = php artisan queue:work
on_exit = restart        ; workers come and go; bring them straight back

[migrate]
cmd     = php artisan migrate
on_exit = keep           ; runs once, then sits there done
```

`on_exit = quit` makes gloncher exit non-zero and print which program caused
it, so a script wrapping gloncher can tell a real failure from a normal quit:

```
server               exit 7 — took the session down (on_exit = quit)
worker               stopped
gloncher: server ended (exit 7) and its on_exit policy is quit
```

Shutting down with `q` or ctrl-c never triggers a policy: nothing is restarted
on the way out, and the processes gloncher stopped are reported as `stopped`
rather than as crashes.

`restart = yes` / `restart = no` still work as a shorthand for
`on_exit = restart` / `on_exit = keep`.

## The lights

The first line carries one light per program, including hidden ones (marked
with a `·`):

| light | meaning |
| --- | --- |
| dim `●` | running, no output recently |
| bright `●` | output in the last ¾ second |
| bright red `●` | wrote to stderr recently |
| `○` | exited cleanly |
| red `✗` | exited with a non-zero status, or ended the session via `on_exit = quit` |

A pane with a `filter` only lights up for lines that pass the filter, so a log
pane filtered to `ERROR` stays dark until something actually breaks.

## Quitting

`q` or `ctrl-c` stops every program and quits. Children are signalled as a
process group, so the things they spawn — `php artisan serve`'s PHP process,
`npm run dev`'s bundler — go down too instead of being orphaned. After the
screen is restored, gloncher prints how each program ended.

## Releasing

`binaries/` holds the prebuilt binaries that ship in the repo. Rebuild them
whenever you cut a version, and commit the result:

```sh
make binaries      # binaries/gloncher-<os>-<arch>, ~11 MB for the five targets
make release       # a standalone dist/ bundle, not committed
```

Every rebuild adds a fresh copy of all five to git history, so do it on version
bumps rather than on every change.

## Flags

```
-i, --init      write a commented template INI file (default gloncher.ini)
-c, --check     parse the file, print what it would run, and exit
-v, --version   print the version
-h, --help      print help
```

The launcher script adds two of its own, which it handles before the binary
sees them:

```
-w, --which     print the path of the binary that would run
-B, --build     rebuild from source before running
```
