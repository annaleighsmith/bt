# Enums and Data Formats

Field values bt must accept and produce to stay br-compatible.

## Statuses

```
open          Default for new issues
in_progress   Work started
blocked       Manually marked blocked (not auto-set by deps)
deferred      Pushed to later
closed        Terminal — excluded from list by default
tombstone     Soft-deleted — excluded from everything by default
```

`bt ready` treats `closed`, `tombstone`, `deferred`, and `blocked` as excluded.

Note: br also has `draft` and `pinned` — bt should accept them on read but doesn't need to produce them.

## Issue Types

```
task          Default
bug
feature
epic          Parent issue — children linked via parent-child deps
chore
docs
question
```

Accept unknown types on read (br's data has `not_a_real_type`). Store as string, don't hard-fail on unrecognized values.

## Priority

Integer 0-4. Accept `P0`-`P4` or `0`-`4` on input. Store as integer.

```
0   Critical
1   High
2   Medium (default)
3   Low
4   Backlog
```

## Dependency Types

Stored inline in the issue's `dependencies` array:

```
blocks          Hard blocker — ready excludes issues blocked by open deps of this type
parent-child    Epic/subtask relationship — not a blocker
relates-to      Informational link — not a blocker
discovered-from Provenance tracking — not a blocker
```

**Only `blocks` affects `ready` filtering.** Everything else is informational.

Note: br's data also has `parent_child` (underscore variant) — treat as equivalent to `parent-child`.

### Dependency object shape

```json
{
  "issue_id": "bt-a1b2",
  "depends_on_id": "bt-c3d4",
  "type": "blocks",
  "created_at": "2026-03-13T10:00:00Z",
  "created_by": "anna",
  "metadata": "{}",
  "thread_id": ""
}
```

`metadata` and `thread_id` are pass-through — bt writes `"{}"` and `""` for new deps, preserves existing values.

## Comments

Stored inline in the issue's `comments` array:

```json
{
  "id": "uuid-or-generated",
  "issue_id": "bt-a1b2",
  "author": "anna",
  "text": "Comment body",
  "created_at": "2026-03-13T10:00:00Z"
}
```

bt doesn't need a comment command in v1 — but must preserve existing comments on round-trip.

## Labels

Array of strings on the issue. Free-form, no predefined values.

```json
"labels": ["cli", "reliability", "sync"]
```

bt supports `--add-label` and `--rm-label` on update. No separate label management commands needed.

## Timestamps

RFC3339 with nanosecond precision and UTC timezone, matching br's output:

```
2026-03-13T10:00:00.123456789Z
```

Go's `time.RFC3339Nano` handles this.
