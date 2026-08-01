package config

// Prompt is what `gloncher --prompt` writes to stdout: a self-contained brief a
// coding agent can be handed to make a gloncher.ini for the project it is
// working in. It lives beside Template because it documents the same key set —
// TestPromptCoversEveryKey fails if the two drift apart.
const Prompt = `You are working in an existing web project. Add a gloncher configuration to it.

# What gloncher is

gloncher (https://github.com/pforret/gloncher) is a small Go terminal tool that
launches several long-running programs at once and multiplexes their output into
one screen, instead of the developer opening one terminal tab per process. It is
driven entirely by an INI file:

    gloncher dev.ini          # run the stack
    gloncher -c dev.ini       # parse only: print what it would run, then exit
    gloncher -i dev.ini       # write a commented starter file
    q or ctrl-c               # stop every program and quit

The first line of the screen is a row of colored lights, one per program, which
brighten when that program produces output. Below it, each program gets a small
pane showing its last N lines.

# The INI format

One optional [gloncher] section, then one section per program. Section order is
screen order, top to bottom. ; and # start a comment.

[gloncher] keys:

    title     shown at the left of the lights row   (default: the file name)
    dir       working directory for every program   (default: cwd)
    refresh   how often the screen redraws          (default: 100ms)

Per-program keys — only cmd is required:

    cmd       the command line; it goes through a shell, so pipes, globs and
              $(...) all work
    lines     how many output lines the pane shows  (default 2; 0 hides it)
    show      no — shorthand for lines = 0: the program runs, shows nothing,
              but still gets an indicator light
    width     full or half                          (default full)
              two half-width programs in a row sit side by side
    filter    a regexp; only matching lines are kept, and only those light the
              indicator — so filter = ERROR stays dark until something breaks
    dir       working directory, overriding the global one
    env       KEY=VALUE; repeat the key for more than one
    color     red green yellow blue magenta cyan white   (default: auto)
    shell     no — run the command directly rather than through a shell
    on_exit   what happens when the program ends, on a clean exit and a crash
              alike                                 (default keep)
                keep      nothing; it shows as stopped, the rest keeps running
                restart   bring it back after a second, counting restarts
                quit      stop every other program and exit gloncher

# Your task

1. Work out what this project actually needs running during development. Read
   composer.json, package.json, requirements.txt / pyproject.toml, Procfile,
   docker-compose.yml, Makefile and the framework's own config — do not guess
   from the framework name alone. Typical sets:

   Laravel:  php artisan serve, npm run dev, php artisan queue:work,
             php artisan schedule:work, tail of storage/logs/laravel.log,
             plus php artisan reverb:start or horizon if those are installed
   Symfony:  symfony serve (or php -S), npm run watch / yarn encore dev --watch,
             php bin/console messenger:consume async,
             tail of var/log/dev.log
   Django:   python manage.py runserver, npm run dev if there is a frontend,
             celery -A <app> worker and celery -A <app> beat if celery is used
   Flask:    flask run --debug, npm run dev, an rq/celery worker if present

   Only include what this project has. Skip anything already provided by a
   docker-compose service the developer runs separately, and say so in a comment.

2. Write the INI file (dev.ini, or a name that fits the project's conventions)
   using these conventions:

   - The primary web server gets on_exit = quit: when it dies the session is
     over anyway, and gloncher exits non-zero naming it.
   - Workers and schedulers get on_exit = restart; they are expected to come
     and go.
   - Asset builders (npm run dev, encore watch) are noisy and rarely
     interesting, so give them show = no. The light still tells you they are
     alive.
   - Log tails get lines = 5 and filter = ERROR|CRITICAL|Exception (adjust to
     the framework's own level names) so the pane stays quiet until something
     is wrong.
   - Pair related short panes as width = half so they share a row; keep the
     server and the log tail full width.
   - Prefer commands that survive a missing newest-log-file, e.g.
     tail -n 100 -F storage/logs/laravel.log over a $(ls -t ...) that expands
     to nothing on a fresh checkout.
   - Comment each section with one line saying why it is there.

3. Verify it parses and runs the right things:

       gloncher -c dev.ini

   Fix anything that is wrong, then tell the developer how to start the stack
   and which single command each pane corresponds to.

Do not add gloncher itself as a project dependency, and do not modify existing
scripts in composer.json or package.json — the INI file is the whole change.
`
