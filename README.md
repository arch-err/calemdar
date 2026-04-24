# cale**md**ar

*Recurring-event manager for Obsidian Full Calendar. Markdown as source of truth, server-expanded occurrences.*

## The problem

Obsidian's Full Calendar plugin has a long-standing footgun: drag a recurring
event to a new time and the plugin overwrites the recurrence rule, wiping out
every other occurrence.

## What cale**md**ar does

It sidesteps the footgun entirely by materializing each recurring series into
individual single-event files. Full Calendar sees only flat, non-recurring
events — so drag, drop, and edit without fear.

- Recurring **root** events live in `recurring/<slug>.md` (server-private,
  not indexed by Full Calendar).
- The server **expands** each root into concrete occurrences at
  `events/<calendar>/<year>/<YYYY-MM-DD>-<slug>.md` — Full Calendar reads these.
- Edit a single occurrence in Obsidian → it flips to `user-owned: true` and
  survives all future re-expansions.
- Edit the root → future non-user-owned occurrences regenerate. Past is
  immutable.
- Create a recurring event in Full Calendar's UI → the server detects it,
  translates it to a root, and expands it. Nothing to learn.

Markdown is the source of truth. A SQLite cache sits alongside for fast lookups.

## Install

```sh
go install github.com/arch-err/calemdar/cmd/calemdar@latest
# or clone + `just build` → ./bin/calemdar
```

## Configuration

cale**md**ar reads a YAML config from `$XDG_CONFIG_HOME/calemdar/config.yaml`
(defaults to `~/.config/calemdar/config.yaml`). See
[examples/config.yaml](./examples/config.yaml) for every available key with
comments.

Minimum needed:

```yaml
vault: /home/you/path/to/vault
```

Precedence, highest wins: `--vault` flag → `$CALEMDAR_VAULT` env → config
`vault:` key → error. Use `calemdar config path` to print the lookup path
and `calemdar config show` to dump the active (post-merge) values.

## Quickstart

```sh
# one-time: create the config
mkdir -p ~/.config/calemdar
cp examples/config.yaml ~/.config/calemdar/config.yaml
$EDITOR ~/.config/calemdar/config.yaml    # at minimum, set vault:

# create a recurring series interactively
calemdar series new

# run the daemon — watches recurring/ and events/, reacts live
calemdar serve
```

## Running as a systemd user service

Sample unit: [examples/calemdar.service](./examples/calemdar.service).

```sh
cp examples/calemdar.service ~/.config/systemd/user/calemdar.service
systemctl --user daemon-reload
systemctl --user enable --now calemdar.service
journalctl --user -u calemdar -f    # watch it work
```

All tuning lives in `config.yaml` — the unit itself takes no env or flags.

With `serve` running, in Obsidian:

1. Use the Full Calendar plugin's "new recurring event" UI normally.
2. cale**md**ar detects the file, translates to a root, expands into flat
   single events. The original FC file disappears.
3. Drag any occurrence to reschedule. It becomes user-owned. The series
   leaves it alone forever.

## CLI surface

| Command                                  | Does what |
|------------------------------------------|-----------|
| `calemdar serve`                         | Daemon: watcher + nightly timers |
| `calemdar reactor`                       | One-shot scan of events/ for FC recurring events → migrate |
| `calemdar reindex`                       | Rebuild the SQLite cache from disk |
| `calemdar extend`                        | Reconcile every series (12-month horizon) |
| `calemdar expand <id-or-slug>`           | Reconcile one series |
| `calemdar archive`                       | Move events >6 months old into `archive/` |
| `calemdar series new`                    | Interactive recurring root creation |
| `calemdar series list`                   | Tabular listing |
| `calemdar series show <id-or-slug>`      | Full detail |
| `calemdar series except <id-or-slug> <date>` | Add a skip-date + reconcile |
| `calemdar event new`                     | Interactive one-off event |
| `calemdar event list [--range=...]`      | `today` / `week` / `month` / `all` |
| `calemdar event show <path>`             | Event detail |

## Vault layout

```
<vault>/
├── events/                      # FC sources, one folder per calendar = one color
│   ├── health/ tech/ work/ life/ friends-family/ special/
│   │   └── 2026/                # year subfolders (server-created)
│   │       └── 2026-05-03-workout.md
├── recurring/                   # NOT an FC source. server's source of truth
│   └── workout.md
├── archive/                     # NOT an FC source. >6mo events moved here
│   └── 2025/<calendar>/
└── .calemdar/cache.db           # SQLite cache (projection, regenerable)
```

Configure Full Calendar with six local calendar sources (one per subfolder
under `events/`), each with its own color.

## Schema + design

- [docs/schema.md](./docs/schema.md) — frontmatter shapes for roots and events
- [docs/design.md](./docs/design.md) — folder layout + server behavior

## Known limits (v1)

- **Timezone:** `Europe/Stockholm` is hardcoded. Multi-tz isn't in v1.
- **Single-daemon:** run `calemdar serve` on exactly one host against a given
  vault. Two daemons against the same Syncthing-synced vault will fight.
- **Recurrence subset:** daily, weekly+byday, monthly+bymonthday, with
  `interval` and `until`. No "last friday of month", no `COUNT=`, no positional
  `BYDAY` (rejected with a clear error).
- **No ICS export / CalDAV server.** Deferred. Ship Radicale if you want it.
- **No notifications yet.** Intended path: a separate `calemdar-notify` binary
  reading the SQLite cache and pushing via ntfy. Deferred.

## Name

cale**md**ar = calendar + markdown. Always style it with the `md` bolded.
