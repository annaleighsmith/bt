# bt

A minimal, local-first issue tracker for AI agents. Issues live in plain JSONL files alongside your code.

## Install

```bash
# From source
go build -o bt . && mv bt ~/go/bin/

# Or via go install (once published)
go install github.com/annaleighsmith/bt@latest
```

## Quick Start

```bash
bt init                                    # create .beads/ workspace
bt create "Fix login bug" -t bug -p 1      # or: bt add
bt ready                                   # what to work on next
bt show <id>                               # full details + dependencies
bt close <id>                              # mark done
git add .beads/ && git commit -m "Sync beads"
```

## Commands

```bash
bt init                                    # create .beads/ workspace in current repo
bt create "title" -t bug -p 1 -d "details" # create an issue (aliases: add)
bt create "title" -q                      # quiet mode — prints only the ID (for scripting)
bt list [--all] [--status S] [--json]     # list issues (default: non-terminal)
bt show <id> [--json]                     # full issue details + dependencies
bt ready [--json]                         # open + unblocked, not deferred
bt update <id> --claim                    # assign to self + in_progress (aliases: edit)
bt update <id> -p 0 -t bug -a user       # set priority, type, assignee
bt update <id> --add-label fix            # add/remove labels
bt close <id> --reason "shipped"          # close an issue
bt dep add <id> <blocked-by>              # wire a dependency
bt dep rm|list <id>                       # remove / show dependencies
bt archive --before 2025-01-01            # move old closed issues to archive.jsonl
bt prompt --inject AGENTS.md              # add issue tracking docs to AGENTS.md or CLAUDE.md
```

Short IDs work: `a1b` resolves to `pfx-a1b2` if unambiguous. All read commands accept `--json`.

## Concurrency

bt uses `flock` to serialize writes to the JSONL file. Multiple agents can read concurrently, but writes queue behind a file lock. This is the intentional trade-off: no database means no WAL, no indexes, no connection overhead — but also no concurrent writes.

In practice this is fine for most setups. Each write holds the lock for a few milliseconds, so even 30 agents writing back-to-back serialize in ~120ms total. If you need dozens of agents writing *sustained* high-throughput, bt isn't the right tool — use something with a real database.

## Related Projects

bt is part of the [beads](https://github.com/steveyegge/beads) ecosystem. All three tools read and write the same `.beads/issues.jsonl` format and can share a workspace.

- **[beads](https://github.com/steveyegge/beads)** (Go) — the original. Part of a larger ecosystem (GasTown) with Dolt-backed storage, git hooks, merge drivers, Linear integration, and more.
- **[beads_rust](https://github.com/Dicklesworthstone/beads_rust)** (Rust) — a Rust port with a SQLite backend.
- **bt** (Go) — a minimal alternative that skips the database entirely. JSONL is the source of truth, `flock` handles concurrency. Optimized for small-to-medium projects (< 2-3K issues) where per-command speed matters more than query power.

## Suggested AGENTS.md Section

Add this to your project's `AGENTS.md` so AI agents know how to use bt:

````markdown
## Issue Tracking

This project uses **bt** for issue tracking. Issues live in `.beads/issues.jsonl`. Run `bt --help` for full usage.

```bash
bt ready                                   # what to work on next (open, unblocked, not deferred)
bt show <id>                               # full issue details + dependencies
bt create "title" -p 2 -t bug              # file an issue
bt create "title" -q                       # quiet mode — prints only the ID (for scripting)
bt update <id> --claim                     # assign to self + mark in_progress
bt close <id>                              # mark work done
bt dep add <issue> <blocked-by>            # wire a dependency
git add .beads/ && git commit -m "Sync beads"  # persist changes
```
````
