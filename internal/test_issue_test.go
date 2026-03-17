package internal

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	fixtures := []string{
		"../testdata/nora_issues.jsonl",
		"../testdata/microdash_issues.jsonl",
	}
	for _, fixture := range fixtures {
		t.Run(filepath.Base(fixture), func(t *testing.T) {
			original, err := os.ReadFile(fixture)
			if err != nil {
				if os.IsNotExist(err) {
					t.Skipf("fixture not available: %s", fixture)
				}
				t.Fatalf("read fixture: %v", err)
			}

			issues, err := LoadIssues(fixture)
			if err != nil {
				t.Fatalf("load issues: %v", err)
			}

			tmp := filepath.Join(t.TempDir(), "issues.jsonl")
			if err := SaveIssues(tmp, issues); err != nil {
				t.Fatalf("save issues: %v", err)
			}

			output, err := os.ReadFile(tmp)
			if err != nil {
				t.Fatalf("read output: %v", err)
			}

			if !bytes.Equal(original, output) {
				// Find first diff
				lines1 := bytes.Split(original, []byte("\n"))
				lines2 := bytes.Split(output, []byte("\n"))
				for i := 0; i < len(lines1) && i < len(lines2); i++ {
					if !bytes.Equal(lines1[i], lines2[i]) {
						t.Fatalf("line %d differs:\n  got:  %s\n  want: %s", i+1, lines2[i][:min(200, len(lines2[i]))], lines1[i][:min(200, len(lines1[i]))])
					}
				}
				if len(lines1) != len(lines2) {
					t.Fatalf("line count differs: got %d, want %d", len(lines2), len(lines1))
				}
				t.Fatal("round-trip produced different output")
			}
		})
	}
}

func TestResolveID(t *testing.T) {
	records := []IssueRecord{
		{Issue: Issue{ID: "test-abc"}},
		{Issue: Issue{ID: "test-def"}},
		{Issue: Issue{ID: "test-ab1"}},
	}

	// Exact match
	idx, err := ResolveID("test-abc", records)
	if err != nil || idx != 0 {
		t.Fatalf("exact match: idx=%d err=%v", idx, err)
	}

	// Suffix match
	idx, err = ResolveID("def", records)
	if err != nil || idx != 1 {
		t.Fatalf("suffix match: idx=%d err=%v", idx, err)
	}

	// Prefix match (after dash)
	idx, err = ResolveID("de", records)
	if err != nil || idx != 1 {
		t.Fatalf("prefix match: idx=%d err=%v", idx, err)
	}

	// Ambiguous
	_, err = ResolveID("ab", records)
	if err == nil {
		t.Fatal("expected ambiguous error")
	}

	// No match
	_, err = ResolveID("zzz", records)
	if err == nil {
		t.Fatal("expected no match error")
	}
}

func TestGenerateID(t *testing.T) {
	existing := map[string]bool{}

	id1 := GenerateID("test", existing, "title", "desc", "user", "2026-01-01T00:00:00Z")
	if len(id1) != len("test-")+3 {
		t.Fatalf("expected 3-char suffix for <50 issues, got %s", id1)
	}

	// Deterministic
	id2 := GenerateID("test", existing, "title", "desc", "user", "2026-01-01T00:00:00Z")
	if id1 != id2 {
		t.Fatalf("not deterministic: %s != %s", id1, id2)
	}

	// Collision avoidance
	existing[id1] = true
	id3 := GenerateID("test", existing, "title", "desc", "user", "2026-01-01T00:00:00Z")
	if id3 == id1 {
		t.Fatal("collision not avoided")
	}

	// Adaptive length: 50+ issues → 4 chars
	big := make(map[string]bool)
	for i := 0; i < 50; i++ {
		big["test-"+string(rune('a'+i/26))+string(rune('a'+i%26))+"x"] = true
	}
	id4 := GenerateID("test", big, "something", "", "u", "2026-01-01T00:00:00Z")
	parts := splitID(id4)
	if len(parts) != 2 || len(parts[1]) != 4 {
		t.Fatalf("expected 4-char suffix for 50+ issues, got %s (suffix len %d)", id4, len(parts[1]))
	}
}

func splitID(id string) []string {
	for i := len(id) - 1; i >= 0; i-- {
		if id[i] == '-' {
			return []string{id[:i], id[i+1:]}
		}
	}
	return []string{id}
}

func TestUpdateRecord(t *testing.T) {
	fields := map[string]any{
		"id":     "test-abc",
		"title":  "Original",
		"status": "open",
	}
	rec, err := NewRecord(fields)
	if err != nil {
		t.Fatal(err)
	}

	if err := UpdateRecord(&rec, map[string]any{
		"title":  "Updated",
		"status": "in_progress",
	}); err != nil {
		t.Fatal(err)
	}

	if rec.Issue.Title != "Updated" {
		t.Fatalf("title not updated: %s", rec.Issue.Title)
	}
	if rec.Issue.Status != "in_progress" {
		t.Fatalf("status not updated: %s", rec.Issue.Status)
	}

	// Verify JSON is valid
	var m map[string]any
	if err := json.Unmarshal(rec.Raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["title"] != "Updated" {
		t.Fatalf("raw title not updated: %v", m["title"])
	}
}

func TestUpdateRecordNilRemovesKey(t *testing.T) {
	rec, _ := NewRecord(map[string]any{
		"id":          "test-abc",
		"title":       "Test",
		"description": "to be removed",
	})

	if err := UpdateRecord(&rec, map[string]any{"description": nil}); err != nil {
		t.Fatal(err)
	}

	var m map[string]any
	json.Unmarshal(rec.Raw, &m)
	if _, ok := m["description"]; ok {
		t.Fatal("description should have been removed")
	}
}

func TestUpdateRecordPreservesUnknownFields(t *testing.T) {
	// Unknown fields must survive mutations — this is the br compatibility guarantee
	fields := map[string]any{
		"id":               "test-abc",
		"title":            "Original",
		"status":           "open",
		"compaction_level": 3,
		"source_repo":      "foo",
		"custom_nested":    map[string]any{"key": "value"},
	}
	rec, err := NewRecord(fields)
	if err != nil {
		t.Fatal(err)
	}

	if err := UpdateRecord(&rec, map[string]any{
		"title":  "Updated",
		"status": "in_progress",
	}); err != nil {
		t.Fatal(err)
	}

	var m map[string]any
	if err := json.Unmarshal(rec.Raw, &m); err != nil {
		t.Fatal(err)
	}

	// Typed fields updated
	if m["title"] != "Updated" {
		t.Fatalf("title not updated: %v", m["title"])
	}
	if m["status"] != "in_progress" {
		t.Fatalf("status not updated: %v", m["status"])
	}

	// Unknown fields preserved
	if m["compaction_level"] != float64(3) {
		t.Fatalf("compaction_level lost: %v", m["compaction_level"])
	}
	if m["source_repo"] != "foo" {
		t.Fatalf("source_repo lost: %v", m["source_repo"])
	}
	nested, ok := m["custom_nested"].(map[string]any)
	if !ok || nested["key"] != "value" {
		t.Fatalf("custom_nested lost: %v", m["custom_nested"])
	}
}

func TestIsBlockingDepType(t *testing.T) {
	blocking := []string{"blocks", "parent-child", "conditional-blocks", "waits-for"}
	for _, dt := range blocking {
		if !IsBlockingDepType(dt) {
			t.Errorf("IsBlockingDepType(%q) = false, want true", dt)
		}
	}
	nonBlocking := []string{"", "informational", "related", "duplicate"}
	for _, dt := range nonBlocking {
		if IsBlockingDepType(dt) {
			t.Errorf("IsBlockingDepType(%q) = true, want false", dt)
		}
	}
}

func TestEpicChildCounts(t *testing.T) {
	makeRec := func(id, status string, deps []Dependency) IssueRecord {
		rec, _ := NewRecord(map[string]any{"id": id, "status": status})
		if len(deps) > 0 {
			var depsAny []any
			for _, d := range deps {
				depsAny = append(depsAny, map[string]any{
					"issue_id": d.IssueID, "depends_on_id": d.DependsOnID, "type": d.Type,
				})
			}
			UpdateRecord(&rec, map[string]any{"dependencies": depsAny})
		}
		return rec
	}

	epic := makeRec("test-epic", "open", nil)
	child1 := makeRec("test-c1", "closed", []Dependency{
		{IssueID: "test-c1", DependsOnID: "test-epic", Type: "parent-child"},
	})
	child2 := makeRec("test-c2", "open", []Dependency{
		{IssueID: "test-c2", DependsOnID: "test-epic", Type: "parent-child"},
	})
	child3 := makeRec("test-c3", "tombstone", []Dependency{
		{IssueID: "test-c3", DependsOnID: "test-epic", Type: "parent-child"},
	})
	// blocks dep should NOT count
	blocker := makeRec("test-blk", "open", []Dependency{
		{IssueID: "test-blk", DependsOnID: "test-epic", Type: "blocks"},
	})
	// Epic with no children
	emptyEpic := makeRec("test-empty", "open", nil)

	records := []IssueRecord{epic, child1, child2, child3, blocker, emptyEpic}
	counts := EpicChildCounts(records)

	// test-epic: 3 parent-child children (c1 closed, c2 open, c3 tombstone)
	c := counts["test-epic"]
	if c.Total != 3 {
		t.Fatalf("expected 3 total children, got %d", c.Total)
	}
	if c.Closed != 2 {
		t.Fatalf("expected 2 closed children (closed + tombstone), got %d", c.Closed)
	}

	// empty epic should have zero counts
	ec := counts["test-empty"]
	if ec.Total != 0 || ec.Closed != 0 {
		t.Fatalf("empty epic should have {0,0}, got {%d,%d}", ec.Total, ec.Closed)
	}
}

func TestParsePriority(t *testing.T) {
	tests := []struct {
		in   string
		want int
		err  bool
	}{
		{"0", 0, false},
		{"4", 4, false},
		{"P2", 2, false},
		{"p3", 3, false},
		{"5", 0, true},
		{"-1", 0, true},
		{"abc", 0, true},
	}
	for _, tt := range tests {
		got, err := ParsePriority(tt.in)
		if tt.err && err == nil {
			t.Errorf("ParsePriority(%q) expected error", tt.in)
		}
		if !tt.err && err != nil {
			t.Errorf("ParsePriority(%q) unexpected error: %v", tt.in, err)
		}
		if !tt.err && got != tt.want {
			t.Errorf("ParsePriority(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}
