
# bt — Minimal Go CLI Issue Tracker

br and bd-compatible issue tracker that reads/writes `.beads/issues.jsonl`. No database — JSONL is the source of truth. Target: ~500 lines of Go (excluding tests).

## Build / Test / Run

```bash
go build -o bt .
go test ./...
go test -run TestName ./...
```

External dependencies: `cobra` (CLI), `lipgloss` (styling), `huh` (interactive prompts), `isatty` (TTY detection). Everything else is stdlib.

## Architecture

- `cmd/` — CLI wiring only (cobra commands, flag parsing, output formatting). Should never contain business logic.
- `internal/` — data layer (structs, JSONL I/O, validation, table rendering). Should never import cobra.
- `root.go` — single source of truth for all help text and command descriptions

### Code Organization Philosophy

- **Separate concerns by layer, not by feature** — a command file should never contain business logic; a data file should never import cobra
- **Single source of truth** — help text and command descriptions live in `root.go` only. Subcommand `Short` fields are synced from `helpGroups` at init time. Adding a command means one edit to `helpGroups`, not two files.
- **Tests live with the code they test** — unit tests in `internal/` test the data layer directly. Integration tests in `cmd/` run the compiled binary. Prefix test files with `test_` so they sort together in directory listings.
- **Export at the boundary, not before** — only export symbols from `internal/` that `cmd/` actually needs
- **No abstraction without duplication** — don't create helpers, interfaces, or packages for one-time operations. Three similar lines is better than a premature abstraction.

### JSONL I/O — Dual Deserialization

Each line is unmarshaled into **both** a typed `Issue` struct and a `map[string]any`. On write, typed fields merge back into the map so unknown br fields survive round-trip. Write atomically via temp file + rename.

This is the critical design pattern — br compatibility depends on it.

### ID Generation

Format: `<prefix>-<N base36 chars>` (a-z0-9). Derived from SHA256 hash of `title|desc|creator|timestamp|nonce` — deterministic, no random bytes. Nonce increments on collision.

Adaptive suffix length based on issue count:
- <50 issues → 3 chars
- 50–1599 issues → 4 chars
- 1600+ issues → 5 chars

Aligned with br's approach (both use deterministic SHA256, both use adaptive length).

## Data Model

### Statuses
`open`, `in_progress`, `blocked`, `deferred`, `closed`, `tombstone`

Terminal statuses (excluded from `list` by default): `closed`, `tombstone`

### Priority
Integer 0-4. Accept `P0`-`P4` or `0`-`4` on input, store as integer.

### Types
`task`, `bug`, `feature`, `epic`, `chore`, `docs`, `question`

### Dependencies
Stored inline in each issue's `dependencies` array. Each entry has `issue_id`, `depends_on_id`, `type` ("blocks"), `created_at`, `created_by`.

## Key Constraints

- **Unknown field round-trip**: Any field br writes that bt doesn't know about must be preserved exactly. This is non-negotiable for compatibility.
- **br compatibility**: bt reads br's JSONL, br reads bt's JSONL, no migration needed.
- **~500 line budget**: Keep it minimal. No over-engineering.
- **Prefix from config**: Read `issue_prefix` from `.beads/config.yaml` for existing workspaces. `bt init` writes it. No other config needed.
- **Short ID resolution**: `a1b` resolves to `pfx-a1b2` if unambiguous.
- **Full file read/write**: Entire JSONL read into memory on every command, full rewrite on mutation. Fine at this scale.

## Commands Quick Reference

- `bt init [--prefix pfx]` — create `.beads/` dir, empty `issues.jsonl`, `.gitignore` for SQLite files
- `bt create <title>` — create an issue (`-q` for ID-only output)
- `bt list [--status S] [--all]` — list issues, default non-terminal
- `bt show <id>` — full issue details
- `bt ready` — open + in_progress, not deferred, no open blockers
- `bt update <id> [flags]` — update fields; `--claim` sets assignee=$USER + status=in_progress
- `bt close <id> [--reason "..."]` — close issue
- `bt dep add|rm|list <id> [<blocked-by>]` — manage dependencies
- All read commands support `--json` for machine-readable output

## Testing

- Unit tests: JSONL round-trip, ready filtering, dep graph, ID generation
- Integration tests: run bt commands against temp directories
- Round-trip tests skip gracefully if `testdata/` fixtures are not present

## Related Repositories

Local clones of upstream projects bt is compatible with, located in `~/projects/`:

- **beads** — `steveyegge/beads` pinned at `v0.60.0`
- **beads_rust** — `Dicklesworthstone/beads_rust` pinned at `v0.1.26` (matches installed `br` version)

## Issue Tracking

This project uses **bt** for issue tracking. Issues live in `.beads/issues.jsonl`. Run `bt --help` for full usage.


