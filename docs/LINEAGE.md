# Lineage

bt is the third generation of the beads issue tracker family.

## bd (beads) — the original

Steve Yegge's Go project (~276K lines). Started as a git-backed issue tracker for AI agents, then grew into "GasTown" — a multi-agent orchestration system with molecules, gates, rigs, HOP protocol, daemon mode, auto-hooking git, etc. Uses Dolt (version-controlled MySQL) as its backend. ~30MB binary.

## br (beads_rust) — the Rust rewrite

Jeffrey Emanuel's Rust port (~20-33K lines). Freezes at the "classic" pre-GasTown architecture. SQLite primary storage + JSONL as the git-friendly collaboration surface. Still carries significant complexity: 35+ columns on the issues table, 4-phase import collision resolution, frankensqlite (pure-Rust SQLite reimplementation), blocked-issues cache with recursive CTEs. ~5-8MB binary.

## bt (beads-tracker) — this project

Drops SQLite entirely — JSONL is the storage. ~500 lines of Go target. Stays compatible with br's JSONL format so projects can switch freely between bt and br.

## Compatibility notes

bt reads and writes br's `issues.jsonl`. These details matter for round-trip safety:

### Sparse serialization

br omits fields rather than writing nulls:
- `None` / empty string fields are omitted entirely (not `null`)
- Empty arrays are omitted
- `false` booleans are omitted
- Exception: `compaction_level` always serializes as `0`

bt must handle both: a field being absent and a field being `null`. On write, bt should match br's convention — omit rather than null — so br doesn't choke on unexpected nulls.

### Pass-through fields

br has many fields bt doesn't use. All must survive round-trip:
- `design`, `acceptance_criteria`, `due_at`, `defer_until`
- `external_ref`, `source_system`, `source_repo`
- `ephemeral`, `pinned`, `is_template`
- `sender`, `closed_by_session`
- `compaction_level`, `compacted_at_commit`, `original_size`, `original_type`
- `deleted_at`, `deleted_by`, `delete_reason`
- Any future fields br adds

### Deterministic export order

br sorts its JSONL output:
- Issues sorted by ID
- Labels sorted alphabetically
- Dependencies sorted by compound key (`issue_id`, `depends_on_id`, `type`)

bt should match this ordering so `git diff` stays clean when both tools touch the same file.
