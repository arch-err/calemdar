# Architecture

cale**md**ar is a single static Go binary. One process, one vault, one host.
The internals split into a small set of cooperating packages.

## The pieces

```
                         ┌──────────────────────────────┐
                         │        calemdar serve        │
                         │  (daemon; long-running)      │
                         └─────────────┬────────────────┘
                                       │
         ┌─────────────────────────────┼─────────────────────────────┐
         │                             │                             │
         ▼                             ▼                             ▼
  ┌───────────────┐            ┌──────────────┐            ┌─────────────────┐
  │    watcher    │            │   reactor    │            │ nightly timer   │
  │  (fsnotify +  │            │ (FC → root   │            │ (reconcile all  │
  │   debounce)   │            │  migration)  │            │  + archive)     │
  └───────┬───────┘            └──────┬───────┘            └────────┬────────┘
          │                           │                             │
          └───────────────┬───────────┴─────────────────────────────┘
                          │
                          ▼
                  ┌───────────────┐
                  │   reconcile   │
                  │ (root → flat  │
                  │  occurrences) │
                  └───────┬───────┘
                          │
             ┌────────────┼────────────┐
             ▼            ▼            ▼
         ┌───────┐   ┌────────┐   ┌─────────┐
         │ vault │   │ writer │   │  store  │
         │ (fs + │   │ (atomic│   │ (sqlite │
         │ paths)│   │ writes)│   │  cache) │
         └───────┘   └────────┘   └─────────┘
```

## Source-of-truth layering

1. **Markdown files on disk** — the only source of truth. Recurring roots in
   `recurring/<slug>.md`, expanded events in `events/<calendar>/<YYYY-MM-DD>-<slug>.md`.
2. **SQLite cache** at `.calemdar/cache.db` — a projection of the markdown,
   rebuildable at any time via `calemdar reindex`. Fast lookups for the
   watcher, reactor, and CLI listers.
3. **In-memory config** populated once at startup from `config.yaml`.

If you delete the cache the daemon rebuilds it on next start. If you delete
a markdown file the daemon notices and reconciles accordingly.

## Components

### `internal/watch` — filesystem watcher

Wraps `fsnotify` with recursive-directory walking and a debounce layer.
Coalesces bursts of events on the same path within `debounce_ms` (default
500ms) so a single Obsidian save doesn't trigger a reconcile storm.

Also suppresses **self-writes**: each time the writer touches a file it
records the path + mtime in the store. When a matching event comes back
from fsnotify the watcher swallows it, preventing feedback loops.

### `internal/reactor` — Full Calendar translator

Watches `events/` for files whose frontmatter carries Full Calendar's
recurrence shape (`type: recurring` or `type: rrule`). When one appears,
reactor:

1. Reads the FC frontmatter.
2. Builds an equivalent calemdar root (`freq`, `interval`, `byday` / `bymonthday`).
3. Writes it to `recurring/<slug>.md`.
4. Deletes the FC-authored source file.
5. Hands the new root to reconcile for immediate expansion.

End result: the user does the FC-native "new recurring event" gesture and
cale**md**ar converts it to its own shape without requiring the user to
learn the CLI.

Rejects v1-unsupported rules (positional `BYDAY`, `COUNT=`, etc.) with a
clear error rather than silently losing data.

### `internal/reconcile` — root → occurrences

Given a root, computes the occurrence set for the window
`[today, today + horizon_months]`, respecting `freq`, `interval`, `byday`,
`bymonthday`, `start-date`, `until`, and `exceptions`. Then:

- For each target date, check if the event file exists.
  - If it exists and `user-owned: true` → skip.
  - If it exists and `user-owned: false` → overwrite (future only).
  - If it does not exist → create.
- **Orphan sweep:** any existing event with this `series-id` that is NOT
  in the target set, NOT `user-owned`, and has `date >= today` → delete.
- **Past is immutable.** `reconcile` never touches an event whose `date`
  is earlier than today, regardless of `user-owned`.

Reconcile is idempotent; running it twice in a row does nothing the second
time.

### `internal/autoown` — user-owned flag flipper

Watches `events/`. When an event file is modified and the write was NOT
from the server (self-write suppression), autoown reads the file, sets
`user-owned: true`, and writes it back. After that point, reconcile leaves
it alone forever.

### `internal/store` — SQLite cache

Pure-Go SQLite (`modernc.org/sqlite`, no CGO). Two tables, `series` and
`occurrences` — see [design](design.md#schema-sketch) for the schema.
Exposes:

- `Open(vault)` — open or create the cache.
- `Reindex(vault)` — scan disk and rebuild from scratch.
- Lookup helpers for the watcher and CLI listers.
- Self-write tracking (path + mtime) for fsnotify feedback suppression.

### `internal/writer` — atomic markdown writes

Writes expanded events via write-to-temp + rename. Records the mtime in
the store so the watcher's self-write suppressor can identify its own
events. Uses `0644` on files and `0755` on intermediate directories.

### `internal/vault` — path resolution

Resolves the vault root (tilde expansion, absolute path normalisation) and
scaffolds the directory tree (`recurring/`, `archive/`, `events/<cal>/`).

### `internal/serve` — daemon glue

Wires watcher → dispatcher → reactor / autoown / reconcile. Owns the
nightly timer (extend + archive at `nightly_at`). Stops cleanly on
SIGINT / SIGTERM.

## Data flow walkthroughs

### Create a new recurring series via the CLI

```
calemdar series new
  └─▶ prompt.go gathers fields
  └─▶ writer.Write(root) → recurring/<slug>.md
  └─▶ watcher sees it
      └─▶ dispatch to reconcile.Series
          └─▶ writer.Write(...) × N → events/<cal>/<date>-<slug>.md
          └─▶ store updates occurrences table
```

### Drag an occurrence in Obsidian

```
obsidian writes events/<cal>/<date>-<slug>.md with new date / time
  └─▶ watcher sees it (NOT a self-write)
      └─▶ dispatch to autoown
          └─▶ writer.Write(<same path>) with user-owned: true
  └─▶ reconcile of the root later sees user-owned, skips this file forever
```

### Nightly pass

```
03:00 local (timezone-aware)
  └─▶ for each root: reconcile.Series (extends horizon by ~1 day)
  └─▶ archive.Run (moves events past archive_cutoff_months)
```

## Concurrency model

- Exactly one `calemdar serve` process per vault. A lease file in the store
  directory prevents two daemons from fighting.
- Watcher events are serialised through the dispatcher — reconcile,
  reactor, and autoown never run concurrently on the same file.
- CLI maintenance commands (`reindex`, `extend`, `archive`) are safe to run
  while the daemon is up; they share the store and contend on SQLite
  transactions.

## What cale**md**ar does NOT do

- No CalDAV, no ICS. If you want those, run
  [Radicale](https://radicale.org/) beside it.
- No notifications in v1. The intended shape is a separate
  `calemdar-notify` binary reading the SQLite cache and pushing via ntfy.
- No multi-device daemon. The vault is Syncthing-friendly, but exactly one
  device runs `serve`.
