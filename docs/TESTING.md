# Testing Strategy

## The one test that matters most

Read a br-produced `issues.jsonl`, write it back with bt, diff. Zero diff means bt is safe to use on br projects.

```go
func TestRoundTrip(t *testing.T) {
    original, _ := os.ReadFile("testdata/br_issues.jsonl")
    issues := loadJSONL(original)
    output := writeJSONL(issues)
    if !bytes.Equal(original, output) {
        t.Fatal("round-trip produced different output")
    }
}
```

### Test fixtures

Round-trip tests use real br-produced JSONL fixtures in `testdata/` (not checked into the repo). Tests skip gracefully if fixtures are missing. To run them locally, copy a br workspace's `issues.jsonl` into `testdata/nora_issues.jsonl` and/or `testdata/microdash_issues.jsonl`.

**Important**: Real JSONL files contain mixed serialization styles from bd-to-br migrations. Some lines are verbose bd exports (all 50+ fields present, including nulls and empty arrays), others are sparse br exports (omitted fields). Both coexist in the same file. bt must preserve each line's exact byte representation — do not normalize on write. The raw `map[string]any` is the authority, not the typed struct.

## The null vs absent problem

br omits empty fields. bt must do the same.

```json
// br writes this (field absent):
{"id": "bt-a1b2", "title": "Fix thing"}

// NOT this (field present as null):
{"id": "bt-a1b2", "title": "Fix thing", "labels": null, "assignee": null}
```

If bt writes nulls where br expects absence, br may interpret "explicitly cleared" instead of "never set." Data silently corrupts on the next br import.

### Go implementation

Use `omitempty` on all optional fields:

```go
type Issue struct {
    ID        string `json:"id"`
    Title     string `json:"title"`
    Assignee  string `json:"assignee,omitempty"`
    Labels    []string `json:"labels,omitempty"`
    ClosedAt  string `json:"closed_at,omitempty"`
    // ...
}
```

Caveat: `omitempty` on `int` omits `0`, but `priority` and `compaction_level` can legitimately be `0`. These need the pointer trick:

```go
Priority *int `json:"priority,omitempty"` // nil omits, 0 serializes as 0
```

Or handle them in the raw map merge — overlay typed fields onto the preserved `map[string]any` and let the map's existing value win for fields like `compaction_level`.

## Field ordering

Go's `encoding/json` serializes struct fields in declaration order, and `map[string]any` in sorted key order. br sorts by key. So the raw-map approach naturally matches br's output — don't fight it.

## Other tests

- **ID generation**: same seed produces same ID as br
- **Ready filtering**: issues with open `blocks` deps excluded, `parent-child` deps ignored
- **Dep cycle**: `ready` doesn't infinite-loop on circular deps (just exclude both)
- **Unknown status/type**: bt doesn't crash on values it doesn't recognize
- **Deterministic output**: issues sorted by ID, labels sorted, deps sorted by compound key
