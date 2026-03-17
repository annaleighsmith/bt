# Benchmarks

Results from an Apple M4, bt v0.1.0, br v0.1.26. All times in milliseconds.

## How to run

```bash
bash benchmark/run.sh                           # shell benchmarks (bt vs br)
bash benchmark/run.sh --bt-only 100 1000 5000   # bt only, custom sizes
bash benchmark/agent_run.sh                     # agent workflow benchmarks
go test -bench=. -benchmem ./cmd/               # Go-native function benchmarks
```

## Single-command benchmarks (bt vs br)

### Reads (median of 3 runs)

| Count | tool | list | list -a | show | ready | list -j | ready -j |
|------:|------|-----:|--------:|-----:|------:|--------:|---------:|
| 100 | bt | 5 ms | 4 ms | 4 ms | 4 ms | 5 ms | 4 ms |
| 100 | br | 20 ms | 21 ms | 11 ms | 21 ms | 22 ms | 22 ms |
| 1000 | bt | 10 ms | 10 ms | 10 ms | 10 ms | 11 ms | 12 ms |
| 1000 | br | 21 ms | 20 ms | 11 ms | 20 ms | 21 ms | 22 ms |
| 5000 | bt | 38 ms | 38 ms | 38 ms | 39 ms | 39 ms | 48 ms |
| 5000 | br | 23 ms | 19 ms | 11 ms | 19 ms | 23 ms | 22 ms |

- bt is faster below ~2-3K issues (near-zero startup, linear scan)
- br is flat regardless of count (SQLite indexed queries)
- Crossover around 2-3K issues for reads

### Writes (median of 3 runs)

| Count | tool | q | update | close | dep add |
|------:|------|--:|-------:|------:|--------:|
| 100 | bt | 5 ms | 5 ms | 5 ms | 5 ms |
| 100 | br | 68 ms | 25 ms | 23 ms | 24 ms |
| 1000 | bt | 13 ms | 13 ms | 13 ms | 13 ms |
| 1000 | br | 65 ms | 24 ms | 23 ms | 24 ms |
| 5000 | bt | 51 ms | 50 ms | 51 ms | 51 ms |
| 5000 | br | 66 ms | 25 ms | 24 ms | 25 ms |

- bt writes scale linearly (full file rewrite) but stay fast under 5K
- br writes are constant-time but have higher fixed cost (SQLite WAL, fsync)
- br's `q` (create) is notably slow (~65ms) due to index overhead

## Agent workflow benchmarks (bt vs br)

These simulate real agent scripting patterns — multi-command chains with JSON piping.

| Workflow | bt | br | Speedup |
|----------|---:|---:|--------:|
| AI Sprint (q×20 → list --json → claim×5 → close×5) | 97 ms | 2,811 ms | **29x** |
| Dep Chain (q×10 → chain deps → close bottom-up) | 105 ms | 3,548 ms | **34x** |
| Bulk Ops (q×50 → claim×10 → close×10 → verify) | 206 ms | 8,831 ms | **43x** |
| JSON Pipe (create → ready --json → jq → claim) ×10 | 84 ms | 1,879 ms | **22x** |

bt is **20-43x faster** for agent workflows. br's per-command overhead (~60-180ms for writes) compounds across multi-command chains. bt's JSONL read-all/write-all approach has lower per-command cost at typical project scale.

## Concurrent benchmarks (bt vs br)

N processes launched simultaneously against a workspace with 50 pre-seeded issues.

```bash
bash benchmark/concurrent_run.sh
```

### Concurrent writes (N parallel `q` creates)

| N | bt | br | Speedup |
|--:|---:|---:|--------:|
| 1 | 5 ms | 144 ms | **29x** |
| 5 | 8 ms | 550 ms | **69x** |
| 10 | 12 ms | 1,006 ms | **84x** |
| 50 | 49 ms | 5,256 ms | **107x** |

### Concurrent reads (N parallel `list --json`)

| N | bt | br | Speedup |
|--:|---:|---:|--------:|
| 1 | 5 ms | 53 ms | **11x** |
| 5 | 6 ms | 331 ms | **55x** |
| 10 | 9 ms | 784 ms | **87x** |
| 50 | 28 ms | 3,711 ms | **133x** |

bt never loses — not at any concurrency level tested. bt's flock serializes writes in ~1ms each, so 50 concurrent writes complete in ~49ms total. br's SQLite lock contention scales poorly: each concurrent process pays the full WAL/fsync overhead while waiting for its turn, and that overhead compounds (up to 107x slower for writes, 133x for reads at n=50).

The expected breakpoint (where br's constant-time queries beat bt's linear scan) would require both high concurrency *and* large issue counts (5K+). At that point bt's per-process cost (linear scan + full rewrite) would dominate. But for typical project sizes (< 2K issues), bt's lower fixed overhead wins even under contention.

## Go-native function benchmarks

Core function performance with `b.ReportAllocs()`:

| Function | n=100 | n=1000 | n=5000 |
|----------|------:|-------:|-------:|
| LoadIssues | 0.6 ms / 2K allocs | 5.5 ms / 20K allocs | 28 ms / 100K allocs |
| SaveIssues | 0.4 ms / 17 allocs | 2.6 ms / 17 allocs | 13.7 ms / 17 allocs |
| RoundTrip | 1.1 ms | 8.4 ms | 40 ms |
| UpdateRecord | 26 µs / 267 allocs | — | — |
| ResolveID | 2 µs / 35 allocs | 13 µs / 338 allocs | 63 µs / 1.7K allocs |
| GenerateID | 2 µs / 13 allocs | — | — |

- LoadIssues is the bottleneck: allocations scale linearly with issue count
- SaveIssues is nearly allocation-free (17 allocs regardless of size)
- ResolveID is linear scan — fine at this scale, would need indexing past ~10K

## Takeaways

- bt's sweet spot is projects with < 2-3K issues — faster than br for everything
- Agent workflows are where bt really shines (20-43x faster) because per-command overhead dominates
- Under concurrency, bt's advantage *increases* — flock serialization is nearly free (~1ms/op) while SQLite contention compounds (up to 107x faster at 50 concurrent writers, 133x for reads)
- The expected br breakpoint (high concurrency + 5K+ issues) is well beyond typical project scale
- Above 5K issues, bt reads slow down but writes remain competitive
- The JSONL tradeoff (no indexes, full scan) is the right call for the target use case
