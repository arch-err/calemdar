# FAQ

## Why is the past immutable?

Two reasons:

1. **Log integrity.** If you edited a root a year ago and reconcile
   retroactively rewrote last year's occurrences to match, you would lose
   the history of what actually happened. Calendar entries double as a
   journal — a workout that happened on Tuesday belongs on Tuesday even if
   the recurring rule has since moved to Wednesday.
2. **User-ownership consistency.** Past occurrences often carry notes,
   attendees, or post-hoc edits. Regenerating them would clobber that work
   silently. Making past immutable regardless of `user-owned` means you
   never have to reason about which flag a year-old event has.

If you really want to rewrite the past, edit the markdown file in Obsidian
directly. The server treats that as a human edit and lets it stand.

## What happens if the daemon is off?

Obsidian and Full Calendar keep working. Drags and edits happen as normal.
When you start the daemon again:

- Any FC-authored recurring events created while it was off are caught by
  the reactor on startup.
- Any drags that happened while it was off are caught by autoown on the
  next write to those files (or via `calemdar reindex` — a reindex
  normalises `user-owned` based on mtime vs the store's last recorded
  write).
- Reconcile runs to close the horizon gap for each series.

There is no per-hour state the daemon carries in memory that can't be
rebuilt from disk. Missed wake-ups are fine.

## How do I migrate from a different folder layout?

Short version: move the markdown files into the layout cale**md**ar wants,
then run `calemdar reindex`.

Longer version, step by step:

1. Back up the vault.
2. Stop the daemon: `systemctl --user stop calemdar` (or Ctrl-C).
3. Move existing per-date event files into `events/<calendar>/<YYYY-MM-DD>-<slug>.md`
   (one folder per calendar). Filenames MUST start with `YYYY-MM-DD-`.
4. If you have recurring rules in Full Calendar format already, leave them
   where Full Calendar wrote them — the reactor will translate them on
   first run.
5. If you want to write cale**md**ar-native roots by hand, drop them in
   `recurring/<slug>.md` matching the [schema](schema.md).
6. Run `calemdar reindex` to rebuild the cache.
7. Run `calemdar extend` to expand all roots to the full horizon.
8. Start the daemon.

If you have a pile of legacy events with inconsistent naming, it is usually
faster to rename them with a short shell script (`mv "$f" "$(date -d ... +%Y-%m-%d)-$slug.md"`)
than to teach cale**md**ar to parse alternatives.

## Can I install via AUR / Homebrew?

Not yet packaged. For now:

```sh
go install github.com/arch-err/calemdar/cmd/calemdar@latest
```

Release binaries are published via goreleaser on tagged versions — see
[releases](https://github.com/arch-err/calemdar/releases). AUR and
Homebrew formulae may land if there's demand.

## Why a separate `recurring/` folder? Why not put roots next to events?

Full Calendar reads every `.md` under a configured source folder. If a
recurring root lived in `events/<calendar>/`, Full Calendar would render it
as an event (usually on the wrong date, or as an all-day), which is worse
than useless.

Putting roots in `recurring/` — which is NOT a Full Calendar source — keeps
them invisible to the plugin while still being plain markdown editable from
anywhere in Obsidian.

## Why `modernc.org/sqlite` instead of `mattn/go-sqlite3`?

No CGO. Clean cross-compile. Static binary. Good enough performance for
the cache sizes cale**md**ar deals with (tens of thousands of events at
worst).

## Can I run two daemons?

No. A lease file in the SQLite cache directory prevents it. The vault is
Syncthing-friendly — run the daemon on one host and let the vault sync
normally. If you run the daemon on your laptop but open Obsidian on your
phone, the phone only reads; all writes get picked up by the laptop daemon
when the sync lands.

If you try to start a second daemon anyway, it will exit with a clear
"another daemon holds the lease at <host>" error.

## What happens when Obsidian and the daemon race on a write?

They don't, in practice:

- Obsidian writes only to `events/<cal>/<...>.md`. The daemon also writes
  there, but only during reconcile of that specific file's series.
- Writes are atomic (write-to-temp + rename). No reader sees a half-written
  file.
- The daemon's self-write suppressor ignores fsnotify events for its own
  writes, so there's no ping-pong.

If Obsidian and the daemon both write the *same* file within ~ms of each
other, the later write wins (fs semantics). Given the daemon only touches
a file during a reconcile triggered by the root — and the root doesn't
change that fast — the race window is tiny.

## What about `.sync-conflict-*` files?

The daemon logs them at warn level and does not touch them. Resolve
conflicts the way you normally would in a Syncthing vault: open both
versions in Obsidian, pick one, delete the other. Once resolved, the
daemon sees the surviving file on the next event.

## Can I have more than six calendars?

Yes. Edit the `calendars:` list in `config.yaml`, run `calemdar setup` to
scaffold the new subfolder, and add a Full Calendar source in Obsidian
pointing at it. Existing events in other calendars are untouched.

Removing a calendar is a schema change — move or delete its events first,
then remove it from the list.

## Why is the timezone hardcoded?

v1 is a personal tool for one person in one timezone. Multi-timezone
correctness (DST transitions, per-event overrides, UTC storage vs local
render) is a nontrivial chunk of work and not useful for the v1 user. If
you need it, open an issue — the schema leaves room for a future `tz:`
field.

## Where are notifications?

Shipped, in-daemon. Configure a ntfy URL + topic under
`notifications:` in `config.yaml`, flip `enabled: true`, and the
`serve` daemon starts pushing to `<ntfy_url>/<ntfy_topic>` whenever an
upcoming event crosses one of the `lead_minutes` windows (default `[5, 60]`).
All-day events are skipped (no natural trigger time).

Verify end-to-end before flipping the switch on the daemon:

```sh
calemdar notify test
```

Full reference: [configuration](configuration.md#notifications).

No browser notifications. No per-event config. One topic, one lead-time
list, optional per-calendar filter.

## I found a bug / want a feature

[Open an issue](https://github.com/arch-err/calemdar/issues). Short
reproducer, what you expected, what happened. For features, explain the
use case before proposing an implementation — the simpler the feature the
likelier it lands.
