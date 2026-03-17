# bd vs br vs bt

Three implementations of the beads issue tracker, each with different tradeoffs.

## bd — Original (Go, ~267K LOC)

The original — full-featured, SQLite + JSONL + optional Dolt, actively evolving toward GasTown. It has everything: git hooks, merge drivers, Linear integration, RPC daemon, 40+ DB migrations. It's the kitchen-sink version built for scale.

## br — Rust Port (~20K LOC)

A frozen snapshot of bd's "classic" architecture — before the GasTown pivot. Pure-Rust SQLite via `fsqlite`, no system dependencies. Forked to keep a stable foundation for Agent Flywheel tooling while bd evolves in a different direction. Same conceptual model, different runtime tradeoffs (Rust safety, single binary, no C FFI for SQLite).

## bt — Minimal (Go, ~3K LOC)

What if we skip the database entirely? Same JSONL format as br (they interoperate on the same `.beads/` workspace), but the JSONL *is* the database. `flock` instead of WAL. Full file scan instead of indexed queries. The result is dramatically less code and — for the sub-2K issue range that covers most projects — dramatically faster per-invocation, because there's no startup tax for opening a database connection, running migrations, or setting up WAL mode.

## Why the performance gap

br and bd pay a fixed cost per invocation (~20-140ms) that amortizes well at scale but dominates at small scale. bt pays a variable cost (linear in issue count) that starts near zero and only catches up around 2-3K issues. For agent workflows that chain 10-50 commands, that fixed cost difference compounds into 20-100x speedups.

## Open question

Whether br's per-invocation cost is fundamental to SQLite or an artifact of the current implementation (cold-start overhead, connection setup, etc.). If they optimize that path, the gap narrows. bt's advantage is structural only as long as that startup tax persists.
