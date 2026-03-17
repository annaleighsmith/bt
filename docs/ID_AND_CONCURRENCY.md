# ID Generation & File Concurrency

## ID Generation

IDs must match br's format for compatibility: `<prefix>-<base36hash>`

### Algorithm

- Seed: `title|description|creator|created_at_nanos|nonce`
- Hash: SHA256 of seed, take first 8 bytes as uint64, base36 encode (0-9a-z)
- Truncate to adaptive length based on issue count (birthday problem)
  - <50 issues: 3 chars (`bt-a1b`)
  - <1600 issues: 4 chars (`bt-a1b2`)
  - Grows as needed, max 8 chars
- Collision check: if ID exists, increment nonce (0–9), retry. If all collide, increase length by 1
- IDs are deterministic from content, not random — same title+timestamp+creator always produces the same ID

### Child IDs

Epic subtasks use `<parent>.<n>` format: `bt-a1b2.1`, `bt-a1b2.2`

### Prefix

Configured at `bt init --prefix <pfx>`, stored in `.beads/metadata.json`, defaults to directory name.

## File Concurrency

JSONL is read-modify-write on every mutation. Two protections:

### Atomic writes

Never write directly to `issues.jsonl`. Write to a temp file in the same directory, then `os.Rename`. This is atomic on POSIX — readers always see either the old or new file, never a partial write.

```go
tmp, _ := os.CreateTemp(beadsDir, "issues-*.jsonl")
// write all lines to tmp
tmp.Close()
os.Rename(tmp.Name(), jsonlPath)
```

### Advisory file lock (if needed)

Not implemented initially — the write is microseconds and real concurrent access is prevented by git's serialization. If it ever matters:

```go
f, _ := os.OpenFile(path, os.O_RDWR, 0644)
syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
```

This blocks a second bt process until the first finishes. Overkill for now, trivial to add later.

## Round-trip Safety

bt must preserve fields it doesn't understand. Strategy:

- Unmarshal each line into both a typed `Issue` struct and a `map[string]any`
- On write, start from the raw map, overlay typed fields back in
- Unknown fields (e.g., br's `compaction_level`, `source_repo`, future additions) survive untouched

This is the most important compatibility guarantee — a bt write must never silently drop br data.
