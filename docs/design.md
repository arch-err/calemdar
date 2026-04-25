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

## Notifications (v1: minimal, in-daemon)

What shipped in v1 is intentionally small: a single ntfy backend, wired into
`calemdar serve` as a goroutine, with a "test ping" CLI for preflighting the
URL/topic. No multi-backend, no per-event opt-in, no action scripts — those
land in the next iteration (see "Next" below).

- **In-daemon, not a separate binary.** Earlier sketches proposed a
  `calemdar-notify` binary on a `systemd --user` timer; the real shape is
  simpler: when `notifications.enabled` is true, `internal/serve` spawns
  `internal/notify.Notifier` as a goroutine on startup. One process, one
  ticker.
- **Backend: ntfy only.** A 60-second ticker (`tickInterval`) queries the
  store via `ListUpcoming` for non-all-day occurrences whose start time
  falls inside `now + lead ± 30s` for each configured `lead_minutes` value
  (default `[5, 60]`). Matches POST to `<ntfy_url>/<ntfy_topic>` with a
  short body (`title — in Nm @ HH:MM–HH:MM`) and `Tags: calendar,<cal>`.
- **Single topic.** One URL, one topic, optional per-calendar allow-list
  via `notifications.calendars` (empty = all).
- **Dedupe.** In-memory map keyed by `<path>|<lead>`; GC'd daily so the
  same event-lead pair never fires twice. Lost on daemon restart — a
  daemon restart inside a lead window can re-fire, by design (cheap and
  acceptable for v1).
- **All-day skipped.** No natural trigger time, no notification.
- **Preflight CLI:** `calemdar notify test` POSTs a single canned message
  regardless of `notifications.enabled` so the user can confirm wiring
  before flipping the daemon switch on. Errors if URL/topic unset.
- **Config validation:** when `enabled: true`, `ntfy_url` and `ntfy_topic`
  are required; topic must match `^[A-Za-z0-9_-]{1,64}$`; lead-minutes
  must be positive. Enforced in `config.Validate`.

### Next: rich notification system (designed, not yet built)

Planned for the iteration after v1, not in current code:

- **Multi-backend.** Add a system-notifications backend (libnotify /
  desktop notifications) alongside ntfy, selectable per route.
- **Per-event opt-in / opt-out.** Frontmatter flag on events (and roots,
  inheritable) to override the global notify policy. Today, every
  non-all-day event in a watched calendar fires.
- **Pre-notifications / lead-time per route.** Today every backend fires
  on the same `lead_minutes` list. The next iteration lets each route
  define its own lead-time policy (e.g. ntfy at `[5, 60]`, desktop at
  `[15]`).
- **Action runner.** Run a user-defined script on notification fire (or
  on event start), with the event's frontmatter passed as env. Use case:
  pre-meeting "open the agenda doc" or "join the call" automation.
- **Reload / reconfigure without daemon restart.** Today the notifier
  copies its config at start-up; rich version exposes a SIGHUP path.

These are intentionally not in v1. Don't read the code expecting them.

## ICS export (deferred, maybe-never)

- Not in v1.
- If ever added: a read-only ICS projection of `events/` + expanded recurring
  series. Served via tiny HTTP handler in `calemdar serve`.
- CalDAV is out of scope. If the user ever wants CalDAV, deploy Radicale and
  point Obsidian at it — don't reinvent.
