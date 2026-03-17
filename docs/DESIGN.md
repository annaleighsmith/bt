# bt — Design Document

A Go CLI that reads and writes br-compatible `.beads/issues.jsonl`. No database — JSONL is the source of truth.

## Why

br ships a v0.1 pure-Rust SQLite reimplementation (frankensqlite) and 75K lines of code to track issues. The actual problem needs a JSONL file and a few hundred lines of Go.

## Scope

### Commands

```
bt init [--prefix <pfx>]         Create .beads/ and empty issues.jsonl
bt q <title> [-p N] [-t TYPE]    Quick-create issue, print ID only
bt create <title> [flags]        Create issue with full options
bt list [--status S] [--all]     List issues (default: open)
bt show <id>                     Show full issue details
bt ready                         Open + unblocked + not deferred
bt update <id> [flags]           Update fields (--claim shortcut)
bt close <id> [--reason "..."]   Close issue
bt dep add <id> <blocked-by>     Add dependency
bt dep rm <id> <blocked-by>      Remove dependency
bt dep list <id>                 Show dependencies
```

### Non-goals

- No SQL, no database, no sync protocol
- No TUI, no rich rendering, no themes
- No self-update, no schema command, no audit log
- No AGENTS.md management
- No config file system (flags and env vars only)

## Data model

### Storage

Single file: `.beads/issues.jsonl` — one JSON object per line, entire file read into memory on every command, rewritten on mutation.

At the scale this operates (hundreds to low-thousands of issues), full file read/write is sub-millisecond.

### JSONL format (br-compatible)

bt reads and writes the same fields br uses. Unknown fields are preserved on round-trip.

**Core fields** (bt reads and writes):
```json
{
  "id": "pfx-a1b2",
  "title": "Fix the thing",
  "description": "Details here",
  "status": "open",
  "priority": 1,
  "issue_type": "bug",
  "assignee": "anna",
  "labels": ["frontend"],
  "created_at": "2026-03-13T10:00:00Z",
  "created_by": "anna",
  "updated_at": "2026-03-13T10:05:00Z",
  "closed_at": null,
  "close_reason": null,
  "dependencies": [
    {
      "issue_id": "pfx-a1b2",
      "depends_on_id": "pfx-c3d4",
      "type": "blocks",
      "created_at": "2026-03-13T10:00:00Z",
      "created_by": "anna"
    }
  ]
}
```

**Pass-through fields** (bt preserves but doesn't use):
- `notes`, `acceptance_criteria`, `owner`, `estimated_minutes`
- `compaction_level`, `compacted_at_commit`, `original_size`, `source_repo`
- `deleted_at`, `deleted_by`, `delete_reason`, `original_type`
- `external_ref`, `comments`
- Any future fields br adds

### ID generation

`<prefix>-<4 chars>` where chars are base36 (a-z0-9), derived from a hash of timestamp + random bytes. Collisions checked against existing IDs.

### Statuses

`open`, `in_progress`, `blocked`, `deferred`, `closed`, `tombstone`

bt treats `closed` and `tombstone` as terminal (excluded from `list` by default).

### Priority

Integer 0-4. Accepts `P0`-`P4` or `0`-`4` on input, stores as integer.

### Types

`task`, `bug`, `feature`, `epic`, `chore`, `docs`, `question`

## Command details

### `bt init`

- Create `.beads/` directory
- Write empty `.beads/issues.jsonl`
- Write `.beads/config.yaml` with `issue_prefix: <prefix>`
- Write `.beads/.gitignore` containing `*.db`, `*.db-wal`, `*.db-shm`
- Prefix resolution: `--prefix` flag, else directory name
- For existing workspaces: read prefix from `.beads/config.yaml` (`issue_prefix` key)

### `bt q` / `bt create`

- `q` prints only the new ID (for scripting / agent use)
- `create` prints full confirmation
- Both append one line to issues.jsonl
- `--parent <id>` adds a parent-child dependency

### `bt list`

- Default: open + in_progress + blocked + deferred (non-terminal)
- `--all`: include closed/tombstone
- `--status <s>`: filter to specific status
- Sort by priority (ascending), then created_at (ascending)
- Output: `<id>  P<n>  [<type>]  <title>`

### `bt ready`

- Start with `list` (open + in_progress only)
- Exclude deferred
- Build blocker map: for each issue, collect `depends_on_id` where type is `blocks` and the depended-on issue is still open
- Exclude issues that have any open blocker
- Sort by priority then age

### `bt show`

- Print all non-null fields, formatted
- List dependencies with their status
- `--json`: output raw JSON for the issue

### `--json` flag (global)

All read commands (`list`, `show`, `ready`, `dep list`) support `--json` for machine-readable output. Just marshal and print — no custom formatting.

### Short ID resolution

Users can type `a1b` instead of `bt-a1b2`. If the fragment matches exactly one issue, resolve it. If ambiguous, error with the matches.

### `bt update`

- `--claim`: set assignee to `$USER` + status to `in_progress` in one shot
- `--status`, `--priority`, `--type`, `--title`, `--description`, `--assignee`
- `--add-label`, `--rm-label`
- Rewrites the full JSONL (finds line by ID, replaces it)

### `bt close`

- Set status to `closed`, `closed_at` to now
- Optional `--reason`

### `bt dep add/rm/list`

- Dependencies stored inline on the issue (in the `dependencies` array)
- `dep add <id> <blocked-by>`: append to `<id>`'s dependencies array
- `dep rm <id> <blocked-by>`: remove from array
- `dep list <id>`: print dependencies with status of each target

## Implementation

### Structure

```
main.go              CLI routing (cobra)
issue.go             Issue struct, JSONL read/write, ID gen
cmd/
  init.go
  create.go          (also handles q)
  list.go
  show.go
  ready.go
  update.go
  close.go
  dep.go
```

### JSONL I/O

```go
// Read: scan lines, unmarshal each into map[string]any + typed Issue
// Write: marshal each issue, preserve unknown fields, write atomically (temp file + rename)
```

Key detail: unmarshal into both a typed `Issue` struct and a `map[string]any`. On write, merge typed fields back into the map so unknown br fields survive round-trip.

### Dependencies

- `github.com/spf13/cobra` — CLI framework
- Standard library for everything else (encoding/json, os, time, crypto/rand)

That's it. No SQLite, no HTTP, no TLS.

## Compatibility

- bt can read any issues.jsonl produced by br
- br can read any issues.jsonl produced by bt
- Projects can switch between bt and br without migration
- bt ignores `.beads/beads.db` entirely — it's br's problem

## Testing

- `go test ./...` — unit tests for JSONL round-trip, ready filtering, dep graph, ID generation
- A handful of integration tests that run bt commands against a temp directory
- No snapshot tests, no golden files, no baseline fixtures
