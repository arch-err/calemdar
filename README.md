# cale**md**ar

*Recurring-event manager for Obsidian Full Calendar. Markdown as source of truth, server-expanded occurrences.*

## What

Obsidian's Full Calendar plugin has a long-standing footgun: drag a recurring
event to a new time and the plugin clobbers the recurrence rule, wiping out
every other occurrence. cale**md**ar sidesteps this by materializing each
recurring series into individual single-event files. Full Calendar sees only
flat, non-recurring events — so drag, drop, and edit without fear.

- Recurring "root" events live in `recurring/` (not an FC source, server-private).
- Expanded occurrences live in `events/<calendar>/<year>/` — Full Calendar reads these.
- Edit a single occurrence → it becomes "user-owned" and survives re-expansion.
- Edit the root → future non-user-owned occurrences regenerate. Past is immutable.

Markdown is the source of truth. A SQLite cache sits alongside for fast lookups.

## Status

Design stage — see [docs/schema.md](./docs/schema.md) for the locked frontmatter
schema and [docs/design.md](./docs/design.md) for folder layout + server behavior.

## Name

cale**md**ar = calendar + markdown. Always write it with the `md` bolded.
