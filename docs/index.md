# cale**md**ar

<p align="center">
  <img src="assets/logo.svg" alt="calemdar logo" width="160">
</p>

*Recurring-event manager for Obsidian Full Calendar. Markdown as source of
truth, server-expanded occurrences.*

## What it is

cale**md**ar (calendar + **md**) is a small Go daemon plus CLI that turns
recurring events in an Obsidian vault into flat, per-date markdown files that
Obsidian's [Full Calendar plugin](https://github.com/obsidian-community/obsidian-full-calendar)
can read without surprises.

## Why

Full Calendar has a long-standing footgun: drag a single occurrence of a
recurring event to a new slot and the plugin overwrites the recurrence rule,
wiping every other occurrence in the series.

cale**md**ar sidesteps the footgun entirely. It keeps recurrence rules in
server-private root files under `recurring/` and materialises each occurrence
as an independent single-event file under `events/`. Full Calendar sees only
flat events, so drag, drop, and edit are all safe.

## How it works in one diagram

```
 recurring/workout.md ──▶  reconcile  ──▶  events/health/2026-05-03-workout.md
   (recurrence rule)                        events/health/2026-05-05-workout.md
                                            events/health/2026-05-07-workout.md
                                            ...
                                             │
                                             └─▶ Full Calendar reads these
```

When you drag an expanded event in Obsidian, the daemon notices the change,
flips `user-owned: true` on that file, and leaves it alone forever. The rest
of the series keeps regenerating from the root.

## Where to next

- [Quickstart](quickstart.md) — from zero to running daemon in five commands.
- [Architecture](architecture.md) — reactor, reconcile, watcher, store.
- [Configuration](configuration.md) — every config key.
- [CLI](cli.md) — every subcommand.
- [Schema](schema.md) — frontmatter shapes for roots and events.
- [Design](design.md) — folder layout + server behaviour.
- [FAQ](faq.md) — common questions.

## Status

v1 shipped. Single-host, single-timezone (`Europe/Stockholm`), single-vault.
Notifications and ICS export are deferred — see the [FAQ](faq.md) and
[design](design.md) for the intended shape.

## License

MIT. See [LICENSE](https://github.com/arch-err/calemdar/blob/main/LICENSE).
