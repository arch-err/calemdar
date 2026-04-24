# CLI reference

Every cale**md**ar subcommand, grouped by purpose. Run `calemdar --help` or
`calemdar <command> --help` for the built-in version.

## Global flags

```
--vault <path>    override vault (config + env still ignored when set)
```

Vault resolution: `--vault` flag → `$CALEMDAR_VAULT` → `vault:` in config →
error.

## Daemon

### `calemdar serve`

Run the long-lived daemon: filesystem watcher plus nightly timers. Reacts
to changes in `recurring/` and `events/` live, and runs reconcile + archive
on the schedule defined by `nightly_at`.

```sh
calemdar serve
```

Usually invoked from the systemd user unit —
see [examples/calemdar.service](https://github.com/arch-err/calemdar/blob/main/examples/calemdar.service).

## One-shot maintenance

These all do what the daemon does continuously, but once. Safe to run while
`serve` is also running; they share the same vault state.

### `calemdar setup`

Create the calendar subfolders under the vault (or under `base_path`).
Idempotent. The daemon does this at startup too.

```sh
calemdar setup
```

### `calemdar reindex`

Rebuild the SQLite cache from disk. The cache is a projection, not a source
of truth — delete it and `reindex` will put it back exactly as it was.

```sh
calemdar reindex
```

### `calemdar reactor`

One-shot scan of `events/` for Full-Calendar-authored recurring events
(events that still carry FC's recurrence frontmatter). Each found event is
translated into a root under `recurring/` and expanded into flat
occurrences.

```sh
calemdar reactor
```

You rarely need to run this by hand — the daemon runs it on startup and
reacts to new FC recurring events live.

### `calemdar extend`

Reconcile every recurring series, extending each to the configured horizon
(`horizon_months`, default 12). Runs nightly inside the daemon; useful to
invoke after a horizon bump.

```sh
calemdar extend
```

### `calemdar expand <id-or-slug>`

Reconcile a single series. Identify the series by either its UUIDv7 `id`
field or the current filename slug (without `.md`).

```sh
calemdar expand workout
calemdar expand 019073c4-d7e0-7d8f-a1f3-8b2c9e5f4a10
```

### `calemdar archive`

Move events older than `archive_cutoff_months` (default 6) into `archive/`.
Filenames are preserved; directory structure mirrors `events/` but rooted
at `archive/`.

```sh
calemdar archive
```

## Series management

### `calemdar series new`

Interactive prompt-driven recurring root creation. Writes the new root to
`recurring/<slug>.md` and immediately expands it.

```sh
calemdar series new
```

### `calemdar series list`

Tabular listing of all recurring series.

```sh
calemdar series list
```

Columns: slug, calendar, title, freq, interval, start date, until date.

### `calemdar series show <id-or-slug>`

Detailed view of one series — every frontmatter field plus the file path.

```sh
calemdar series show workout
```

### `calemdar series except <id-or-slug> <date>`

Add a date to the series' `exceptions` list and reconcile. The occurrence
on that date is dropped (or swept if already materialised and not
user-owned).

```sh
calemdar series except workout 2026-06-24
```

Warns if `<date>` is in the past — past occurrences are immutable, so the
exception will have no effect on them.

## One-off events

### `calemdar event new`

Interactive prompt-driven one-off event creation. Writes directly under
`events/<calendar>/<YYYY-MM-DD>-<slug>.md` with no `series-id` — the
daemon will not touch it.

```sh
calemdar event new
```

### `calemdar event list [--range=<range>]`

List events in a date range.

```sh
calemdar event list --range=today
calemdar event list --range=week       # default
calemdar event list --range=month
calemdar event list --range=all
```

### `calemdar event show <path>`

Detailed view of one event. Path can be absolute or relative to the vault.

```sh
calemdar event show events/health/2026-05-03-workout.md
```

## Config

### `calemdar config path`

Print the config file lookup path (does not check existence).

```sh
calemdar config path
```

### `calemdar config show`

Print the active config — defaults merged with the file on disk. If no
file exists, prints defaults with a note at the top.

```sh
calemdar config show
```

### `calemdar config init [--override]`

Write a default config file. Errors if one already exists, unless
`--override` is passed. Drops into `$EDITOR` after writing if `$EDITOR`
is set.

```sh
calemdar config init
calemdar config init --override    # overwrite an existing file
```

### `calemdar config edit`

Open the config in `$EDITOR` and validate on save. If the file does not
exist, a default stub is written first. Invalid YAML or out-of-range
values print diagnostics and leave the in-process config unchanged.

```sh
calemdar config edit
```
