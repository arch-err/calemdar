# cale**md**ar — frontmatter schema

Locked v1. All frontmatter keys are stable — new keys may be added; existing
keys will not change shape without a version bump.

## Recurring root

**Location:** `recurring/<slug>.md`. `<slug>` is human-chosen and cosmetic — identity
is the UUID, not the filename. Rename at will.

**Author:** human (via editor or CLI). Server reads, never writes.

```yaml
---
id: 019073c4-d7e0-7d8f-a1f3-8b2c9e5f4a10   # UUIDv7. stable identity. generate via `calemdar event new`
calendar: health                            # one of: health | tech | work | life | friends-family | special
title: "Workout"
start-date: 2026-05-01                      # first occurrence (YYYY-MM-DD)
until: 2027-05-01                           # optional. series ends on/before this date
start-time: "10:00"                         # HH:MM, 24h. omit for all-day
end-time: "11:00"                           # HH:MM, 24h. omit for all-day
all-day: false
freq: weekly                                # daily | weekly | monthly
interval: 1                                 # every N periods. 1 = every period
byday: [mon, wed, fri]                      # weekly only. lowercase three-letter weekday
bymonthday: [1, 15]                         # monthly only. optional. days of month
exceptions:                                 # occurrence dates to skip
  - 2026-05-08
  - 2026-06-24
notify:                                     # optional. inherited by every expansion
  - lead: 5m                                # required. "0", "5m", "1h", "23h" — bare integer = minutes
    via: [system, ntfy]                     # optional. backends to dispatch via
    action: pre-meeting                     # optional. action name in actions.yaml (laptop-local)
---

Notes body is free-form markdown. Copied verbatim into each expanded event
at expansion time.
```

### `notify:` rules

Each entry runs at `event.start - lead`. At least one of `via` or
`action` must be set. Maximum 16 rules per event/root. Min lead 1m, max
lead 23h. See [Configuration / notifications](configuration.md) and
[Actions](actions.md) for the daemon-side wiring.

Rules attached to a root inherit into every expanded occurrence. An
expanded occurrence may override by writing its own `notify:` (which
also flips `user-owned: true`). An empty list (`notify: []`) on an
occurrence opts that single occurrence out of all notifications.

### v1 recurrence coverage

- `freq: daily` + `interval: N` — every N days from `start-date`.
- `freq: weekly` + `interval: N` + `byday: [...]` — every N weeks on listed weekdays.
- `freq: monthly` + `interval: N` + `bymonthday: [...]` — every N months on listed month-days.

Not v1: "last friday of month", "second weekday of month", count-based termination
(use `until` instead). Add via full RFC 5545 RRULE when needed, not before.

## Expanded event

**Location:** `events/<calendar>/<YYYY-MM-DD>-<slug>.md`. Flat — Full
Calendar's local-folder source does not recurse, so a year subfolder would
hide events from its index.

**Author:** server (on series expansion) or human (for true one-offs, via CLI or editor).
Human may drag/edit an expanded event in Obsidian — this flips `user-owned: true`
and the server will not touch it again from root regen.

```yaml
---
title: "Workout"
date: 2026-05-03
startTime: "10:00"
endTime: "11:00"
allDay: false
type: single                                 # FC native field. always "single" for calemdar
series-id: 019073c4-d7e0-7d8f-a1f3-8b2c9e5f4a10   # empty/absent = true one-off
series-expanded-at: 2026-04-24T11:00:00Z    # when server wrote this file (RFC 3339 UTC)
user-owned: false                            # true = server will not regenerate this
notify:                                      # optional. copied from root at expansion time
  - lead: 5m
    via: [system]
---

[[workout]]    # body-level backlink to root. human convenience, server ignores.

Notes body copied from root at expansion time.
```

### Field ownership

| field                | set by | mutable by |
|----------------------|--------|------------|
| `title`              | server | human (drag/edit in Obsidian) |
| `date`               | server | human (drag) |
| `startTime`/`endTime`| server | human (drag/edit) |
| `allDay`             | server | human |
| `type`               | server | — (always `single`) |
| `series-id`          | server | — (immutable; breaks the link if changed) |
| `series-expanded-at` | server | — |
| `user-owned`         | server | server (flipped to `true` on any human edit) |

### One-offs

A true one-off event has `series-id` absent or empty. Server ignores it for
expansion logic; still subject to archive rule.

## Calendars

The six v1 calendars are stable strings. Each maps to a top-level folder under
`events/` and to a Full Calendar source with its own color.

- `health`
- `tech`
- `work`
- `life`
- `friends-family`
- `special`

Adding a calendar is a schema change (new subfolder, new FC source, new color).
Removing one requires migrating or deleting its events.

## Identity

- Every recurring series has a **UUIDv7** in its `id` field — stable across
  renames, immutable.
- Expanded events reference the root by `series-id`, not by wikilink.
- The wikilink in the body (`[[slug]]`) is a human convenience for Obsidian
  backlinks. The server ignores it.

## Timezones

All times are in `Europe/Stockholm` for v1. Not stored per event. If multi-tz
support is ever added, a `tz:` field is introduced and the default stays
`Europe/Stockholm`.
