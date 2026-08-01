package config

import (
	"fmt"
	"os"
)

// Template is a starter INI file, documenting every key the parser accepts.
// It lives next to the parser so the two stay in step: TestTemplateParses
// fails if a key here stops being understood.
//
// The sample programs are deliberately plain shell loops rather than real
// commands, so a freshly generated file runs on any machine. Real-world
// examples sit next to them, commented out.
const Template = `; gloncher configuration — https://github.com/pforret/gloncher
;
; Run it with:  gloncher this-file.ini
;
; One [section] per program. Section order is screen order, top to bottom.
; Lines starting with ; or # are comments.

[gloncher]
; title   — shown at the left of the lights row (default: this file's name)
title   = my project
; dir     — working directory for every program (default: where you run gloncher)
;dir     = ~/Code/my-project
; refresh — how often the screen redraws (default: 100ms)
refresh = 100ms

; ----------------------------------------------------------------------------
; Every key a program can take. Only cmd is required.
;
;   cmd     the command line to run; it goes through the shell, so pipes,
;           globs and $(...) all work
;   lines   how many output lines to show      (default 2; 0 hides the pane)
;   show    no — shorthand for lines = 0: the program runs, but shows nothing
;   width   full or half                       (default full)
;           two half-width programs in a row sit side by side
;   filter  a regexp; only matching lines are kept, and only those light the
;           indicator — so an ERROR filter stays dark until something breaks
;   dir     working directory, overriding the global one
;   env     KEY=VALUE; repeat the key for more than one
;   color   red green yellow blue magenta cyan white   (default: auto)
;   shell   no — run the command directly instead of through the shell
;   on_exit what to do when the program ends, whether it exited cleanly or
;           crashed                            (default keep)
;             keep     nothing; it shows as stopped, the rest keeps running
;             restart  bring it back after a second, counting the restarts
;             quit     stop every other program and exit gloncher
; ----------------------------------------------------------------------------

; Replace the three programs below with your own.

; A long-running server. on_exit = quit means the whole session ends if it
; dies — useful when everything else exists to support this one process.
;   cmd = php artisan serve
;   cmd = npm run start
[server]
cmd     = while true; do echo "server is up"; sleep 5; done
lines   = 2
width   = full
color   = green
on_exit = quit

; A log tail, filtered down to the lines you actually care about. Because the
; command runs through a shell, $(...) picks the newest log file for you:
;   cmd = tail -n 100 -f "$(ls -t storage/logs/*.log | head -1)"
[logs]
cmd     = while true; do echo "ERROR example failure"; sleep 8; done
lines   = 5
width   = full
filter  = ERROR
color   = red

; Two half-width programs share one row. Workers are expected to come and go,
; so bring them straight back.
;   cmd = php artisan queue:work
[worker]
cmd     = while true; do echo "job done"; sleep 3; done
lines   = 2
width   = half
color   = cyan
on_exit = restart

;   cmd = php artisan schedule:work
[cron]
cmd     = while true; do date; sleep 10; done
lines   = 2
width   = half
color   = magenta
on_exit = restart

; A program with show = no still runs and still gets an indicator light — it
; just has no output pane. Good for noisy build watchers.
;   cmd = npm run dev
[builder]
cmd     = while true; do echo building; sleep 2; done
show    = no
color   = yellow
`

// WriteTemplate writes Template to path. It refuses to overwrite an existing
// file: the whole point is to bootstrap a config, never to clobber one
// somebody has already edited.
func WriteTemplate(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists — delete it first, or pick another name", path)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.WriteFile(path, []byte(Template), 0o644); err != nil {
		return err
	}
	return nil
}
