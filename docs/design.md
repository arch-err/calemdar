# cale**md**ar — design

> **Status:** v1 shipped. All CLI subcommands listed below are implemented
> and covered by tests / smoke runs. Deferred items (notifications, ICS) are
> explicitly called out as such.


## Folder layout (in the Obsidian vault)

```
<vault>/
├── events/                      # FC reads these. one source per subfolder = one color
│   ├── health/
│   │   └── 2026/
│   │       ├── 2026-05-03-workout.md
│   │       └── 2026-05-05-meds-refill.md
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
- **Year subfolder inside each calendar** → keeps any single directory from
  exploding (~365 files/yr for a daily event, manageable).
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
calemdar reindex                   # rebuild SQLite from disk
calemdar expand <series-id>        # force-expand a single series
calemdar extend                    # extend the 12mo horizon for all series
calemdar archive                   # archive events older than 6 months

calemdar event new                 # create a one-off event interactively
calemdar event list [--range=today|week|month|...]
calemdar event show <path>

calemdar series new                # create a recurring root interactively
calemdar series list
calemdar series show <id-or-slug>
calemdar series except <id> <date> # add to exceptions list
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
- Hosting: laptop for v1. Architecture is server-ready — move to the home
  server when notifications land.

## Notifications (deferred)

- Not in v1.
- When added: a separate `calemdar-notify` binary reads the SQLite cache for
  upcoming events in the next N minutes and pushes via ntfy (user's existing
  channel). Runs via `systemd --user` timer, NOT the main daemon.
- No browser notifications. No per-event config. One ntfy topic, one lead-time
  setting per calendar.

## ICS export (deferred, maybe-never)

- Not in v1.
- If ever added: a read-only ICS projection of `events/` + expanded recurring
  series. Served via tiny HTTP handler in `calemdar serve`.
- CalDAV is out of scope. If the user ever wants CalDAV, deploy Radicale and
  point Obsidian at it — don't reinvent.
