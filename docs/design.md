# cale**md**ar — design

> **Status:** v1 shipped. All CLI subcommands listed below are implemented
> and covered by tests / smoke runs. Deferred items (notifications, ICS) are
> explicitly called out as such.


## Folder layout (in the Obsidian vault)

```
<vault>/
├── events/                      # FC reads these. one source per subfolder = one color
│   ├── health/                  # flat — FC does NOT recurse into subfolders
│   │   ├── 2026-05-03-workout.md
│   │   └── 2026-05-05-meds-refill.md
│   ├── tech/
│   ├── work/
│   ├── life/
│   ├── friends-family/
│   └── special/
├── recurring/                   # NOT an FC source. server's source of truth
│   ├── workout.md
│   └── monday-standup.md
└── archive/                     # NOT an FC source. past events >6 months old
    └── 2025/
        ├── health/
        └── tech/
```

### Why this shape

- **Six FC sources, one per calendar** → each gets its own color without any
  per-event color property.
- **Events flat under each calendar** → Full Calendar's local-folder source
  does not recurse. Year subfolders hide events from the index. Filenames
  are date-prefixed so sort-by-name is chronological.
- **`recurring/` outside `events/`** → FC never sees the root templates.
- **`archive/` outside `events/`** → FC never indexes old data. Full-text search
  and grep still work for the human.

## Server behavior

The server has two jobs: **expand** recurring roots into concrete occurrences,
and **archive** old occurrences. It runs as a daemon (`calemdar serve`) or via
one-shot CLI commands.

### Expansion

- **Watch** `recurring/` and `events/` via `fsnotify`. Debounce 500ms to coalesce
  burst writes.
- **On root add/change:**
  1. Parse the root's frontmatter.
  2. Compute the set of occurrence dates for the window `[today, today + 12 months]`,
     respecting `freq`, `interval`, `byday`/`bymonthday`, `start-date`, `until`,
     and `exceptions`.
  3. For each date in the set:
     - Compute target path: `events/<calendar>/<year>/<YYYY-MM-DD>-<slug>.md`.
     - If target exists AND `user-owned: true` → skip.
     - If target exists AND `user-owned: false` → overwrite (future only; see below).
     - If target does not exist → create.
  4. Orphan sweep: for any existing expanded event with this `series-id` that
     is NOT in the computed set AND has `user-owned: false` AND `date >= today`
     → delete. (User-owned occurrences are preserved even when orphaned from the
     series — they stand on their own.)
- **Past is immutable.** The server never touches any event where `date < today`,
  regardless of `user-owned` status.

### User-ownership detection

- Each expanded event has `user-owned: false` when first written.
- The server watches `events/` via `fsnotify`. When an `events/...` file is
  modified AND the modification was not from the server itself, the server
  reads the file, sets `user-owned: true`, and writes it back.
- Detecting "modification was from the server itself" is done by comparing the
  mtime to the server's own last-write timestamp (tracked in the SQLite cache).
- After `user-owned: true`, the server never regenerates this occurrence.

### Horizon extension

- Nightly timer (default 03:00 local). For each active root, extend the window
  by adding any new occurrences that now fall inside `[today, today + 12 months]`.
- Idempotent. Safe to run ad-hoc via `calemdar extend`.

### Archive

- Nightly timer. For any expanded event where `date < today - 6 months`:
  move to `archive/<year>/<calendar>/<filename>`.
- Filenames preserved. Directory structure mirrors `events/` but rooted at `archive/`.
- Idempotent. Safe to run ad-hoc via `calemdar archive`.

## SQLite cache

**Not a source of truth.** Rebuilt from `recurring/` + `events/` on `calemdar reindex`.
The cache exists to make lookups fast. If deleted, the server rebuilds it.

### Schema (sketch)

```sql
CREATE TABLE series (
  id TEXT PRIMARY KEY,             -- UUIDv7
  slug TEXT NOT NULL,              -- current filename (without .md)
  calendar TEXT NOT NULL,
  title TEXT NOT NULL,
  freq TEXT NOT NULL,
  interval INTEGER NOT NULL,
  byday TEXT,                      -- JSON array
  bymonthday TEXT,                 -- JSON array
  start_date TEXT NOT NULL,        -- YYYY-MM-DD
  until_date TEXT,                 -- YYYY-MM-DD, nullable
  start_time TEXT,                 -- HH:MM, nullable for all-day
  end_time TEXT,
  all_day INTEGER NOT NULL,        -- 0 or 1
  exceptions TEXT,                 -- JSON array
  root_path TEXT NOT NULL,         -- relative path for quick lookup
  root_mtime INTEGER NOT NULL      -- unix seconds
);

CREATE TABLE occurrences (
  path TEXT PRIMARY KEY,           -- relative path from vault root
  series_id TEXT,                  -- nullable for one-offs
  date TEXT NOT NULL,              -- YYYY-MM-DD
  calendar TEXT NOT NULL,
  user_owned INTEGER NOT NULL,
  expanded_at TEXT NOT NULL,       -- RFC 3339
  server_last_write INTEGER NOT NULL, -- unix seconds
  FOREIGN KEY (series_id) REFERENCES series(id)
);

CREATE INDEX occurrences_date ON occurrences(date);
CREATE INDEX occurrences_series ON occurrences(series_id);
```

## CLI surface (v1)

```
calemdar serve                     # run the watcher + nightly timers
calemdar setup                     # scaffold vault folders (idempotent)
calemdar reindex                   # rebuild SQLite from disk
calemdar reactor                   # one-shot scan for FC-authored recurring events
calemdar expand <id-or-slug>       # force-expand a single series
calemdar extend                    # extend the 12mo horizon for all series
calemdar archive                   # archive events older than 6 months

calemdar event new                 # create a one-off event interactively
calemdar event list [--range=today|week|month|all]
calemdar event show <path>

calemdar series new                # create a recurring root interactively
calemdar series list
calemdar series show <id-or-slug>
calemdar series except <id> <date> # add to exceptions list

calemdar config path|show|init|edit
calemdar notify test               # one-shot ntfy test push
```

## Concurrency / sync

- **The vault is Syncthing-synced.** Multiple devices may write.
- **Only one device runs the `calemdar serve` daemon** at a time. Running two
  daemons against the same vault is unsupported (lease file in SQLite dir).
- **Obsidian and the server will not fight** over individual event files:
  obsidian writes to `events/...` freely, the server only writes to `events/...`
  during expansion. On human edit, the server notices and sets `user-owned:
  true` — it does not fight the human.
- **`.sync-conflict-*` files:** if they appear, surface in logs, don't delete.

## Language + runtime

- **Go.** Single static binary. Embeds daemon + CLI. Low memory, fast startup,
  fsnotify stdlib-adjacent, `modernc.org/sqlite` for pure-Go SQLite, no CGO.
- Binary name: `calemdar`. Installed via `go install` or `just install`.
- Hosting: laptop for v1. Architecture is server-ready — moving to the
  home server is a packaging exercise, not a redesign.

## Notifications

The notification subsystem has three pieces: per-event `notify:` rules
declared in vault frontmatter, a small set of pluggable **backends**
that deliver the message, and an **action runner** that may also spawn
a local script when a rule fires.

### Frontmatter rules

Every event and recurring root may carry a `notify:` list. Each entry
is `{lead, via, action}` — `lead` is a duration string (`5m`, `1h`,
`0`), `via` is the list of backends to dispatch to, `action` references
a named entry in `~/.config/calemdar/actions.yaml`. At least one of
`via` or `action` must be set per entry. Up to 16 entries per event;
min lead 1m, max lead 23h.

Rules attached to a root inherit into every expanded occurrence (copied
verbatim into the expansion). An expanded occurrence may override by
writing its own `notify:` (which flips `user-owned: true`). An empty
list (`notify: []`) on an occurrence opts that single occurrence out.
Edits to the root propagate to non-user-owned occurrences via the
existing reconcile path.

### Backends

Backends register through `internal/notify.Register` at daemon start
based on the resolved config:

- `system` — libnotify-via-`notify-send`. Title + body + tags.
- `ntfy` — POSTs to `<url>/<topic>` with title in headers.

A backend is registered iff its `enabled` flag is true. Adding a new
backend (Discord webhook, Slack, etc.) is a Go file plus a couple of
config struct fields — no scheduler changes.

### Scheduler

`internal/notify.Scheduler` runs as a goroutine inside `calemdar serve`.

- **Tick:** `tick_interval` (default `1m`).
- **Lookahead:** queries the SQLite cache (`occurrences` table, filtered
  by `notify_json IS NOT NULL`) for events with start in
  `[now, now + max_lead]`. `max_lead` defaults to 23h, so the lookahead
  is effectively today's events.
- **Fire:** for each rule, `fire_at = event.start - lead`. The scheduler
  fires rules whose `fire_at` lands in `(last_tick, now]`.
- **Dispatch:** for each backend in `via`, call `Backend.Send(ctx, n)`.
  If `action` is set and the actions runner is enabled, spawn the named
  action with curated env (no parent-process env inheritance — keeps
  daemon secrets out of action subprocesses).
- **Dedupe:** persistent table `notify_fired(event_path, notify_index,
  fire_at_planned)`. `IsFired` consulted on every candidate; `RecordFired`
  inserts before dispatch (a crash mid-fire suppresses replay rather
  than risking a double-fire).
- **Restart safety:** on startup, `last_tick` is initialised to
  `now - 2 * tick_interval` so the daemon picks up rules whose fire time
  fell in the last couple of minutes but does NOT replay older history
  (which would spam the user when a closed laptop wakes up).
- **Pruning:** the nightly loop deletes `notify_fired` rows older than
  14 days.

### Actions

Actions live in `~/.config/calemdar/actions.yaml` — local, NOT synced.
This split is deliberate: vault frontmatter cannot contain script paths,
only action *names*. The laptop-local actions file resolves names to
commands. See [Actions](actions.md) for the file format and full trust
rationale.

The runner spawns via `exec.CommandContext` with a curated env (only
`PATH`, `HOME`, `USER`, `CALEMDAR_*`). A semaphore caps concurrency
(`max_concurrent_spawns`, default 4). A per-action timeout (default 30s)
kills runaway scripts.

### Preflight CLI

- `calemdar notify test [backend]` — fire a test through every enabled
  backend, or just one named.
- `calemdar notify actions` — list registered actions.

## ICS export (deferred, maybe-never)

- Not in v1.
- If ever added: a read-only ICS projection of `events/` + expanded recurring
  series. Served via tiny HTTP handler in `calemdar serve`.
- CalDAV is out of scope. If the user ever wants CalDAV, deploy Radicale and
  point Obsidian at it — don't reinvent.
