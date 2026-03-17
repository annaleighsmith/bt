package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// generateBenchIssues creates n realistic IssueRecords with all br fields.
func generateBenchIssues(n int) []IssueRecord {
	records := make([]IssueRecord, 0, n)
	base := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	existingIDs := make(map[string]bool)
	for i := 0; i < n; i++ {
		created := base.Add(time.Duration(i*37) * time.Minute)
		createdStr := created.Format(time.RFC3339Nano)
		id := GenerateID("bench", existingIDs, fmt.Sprintf("Issue %d", i), "", "tester", createdStr)
		existingIDs[id] = true

		p := i % 5
		m := map[string]any{
			"id":                  id,
			"title":               fmt.Sprintf("Benchmark issue %d", i),
			"description":         "A multi-line description\nfor benchmark testing.",
			"status":              "open",
			"priority":            p,
			"issue_type":          "task",
			"created_at":          createdStr,
			"created_by":          "tester",
			"updated_at":          createdStr,
			"owner":               "bench@test.com",
			"labels":              []string{"bench"},
			"acceptance_criteria": "",
			"actor":               "",
			"agent_state":         "",
			"assignee":            nil,
			"await_id":            "",
			"await_type":          "",
			"closed_by_session":   "",
			"compacted_at":        nil,
			"compacted_at_commit": nil,
			"compaction_level":    0,
			"content_hash":        "",
			"crystallizes":        false,
			"defer_until":         nil,
			"design":              "",
			"due_at":              nil,
			"ephemeral":           false,
			"estimated_minutes":   nil,
			"event_kind":          "",
			"external_ref":        nil,
			"hook_bead":           "",
			"is_template":         false,
			"last_activity":       nil,
			"metadata":            "{}",
			"mol_type":            "",
			"notes":               "",
			"original_size":       nil,
			"payload":             "",
			"pinned":              false,
			"quality_score":       nil,
			"rig":                 "",
			"role_bead":           "",
			"role_type":           "",
			"sender":              "",
			"source_repo":         ".",
			"source_system":       "",
			"spec_id":             "",
			"target":              "",
			"timeout_ns":          0,
			"waiters":             "",
			"wisp_type":           "",
			"work_type":           "",
		}

		raw, _ := json.Marshal(m)
		var issue Issue
		json.Unmarshal(raw, &issue)
		records = append(records, IssueRecord{Issue: issue, Raw: raw})
	}
	return records
}

// writeBenchFile writes issues to a temp JSONL file and returns the path.
func writeBenchFile(b *testing.B, records []IssueRecord) string {
	b.Helper()
	dir := b.TempDir()
	path := filepath.Join(dir, "issues.jsonl")
	if err := SaveIssues(path, records); err != nil {
		b.Fatalf("write bench file: %v", err)
	}
	return path
}

func BenchmarkLoadIssues(b *testing.B) {
	for _, size := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("n=%d", size), func(b *testing.B) {
			records := generateBenchIssues(size)
			path := writeBenchFile(b, records)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := LoadIssues(path); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkSaveIssues(b *testing.B) {
	for _, size := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("n=%d", size), func(b *testing.B) {
			records := generateBenchIssues(size)
			dir := b.TempDir()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				path := filepath.Join(dir, fmt.Sprintf("issues-%d.jsonl", i))
				if err := SaveIssues(path, records); err != nil {
					b.Fatal(err)
				}
				os.Remove(path)
			}
		})
	}
}

func BenchmarkRoundTrip(b *testing.B) {
	for _, size := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("n=%d", size), func(b *testing.B) {
			records := generateBenchIssues(size)
			dir := b.TempDir()
			srcPath := filepath.Join(dir, "src.jsonl")
			SaveIssues(srcPath, records)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				loaded, err := LoadIssues(srcPath)
				if err != nil {
					b.Fatal(err)
				}
				outPath := filepath.Join(dir, fmt.Sprintf("out-%d.jsonl", i))
				if err := SaveIssues(outPath, loaded); err != nil {
					b.Fatal(err)
				}
				os.Remove(outPath)
			}
		})
	}
}

func BenchmarkUpdateRecord(b *testing.B) {
	records := generateBenchIssues(100)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := records[i%len(records)]
		UpdateRecord(&rec, map[string]any{
			"status":     "in_progress",
			"updated_at": "2025-07-01T00:00:00Z",
			"priority":   2,
		})
	}
}

func BenchmarkResolveID(b *testing.B) {
	for _, size := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("n=%d", size), func(b *testing.B) {
			records := generateBenchIssues(size)
			// Resolve the last issue's ID (worst case linear scan)
			fullID := records[size-1].Issue.ID
			// Short ID: last 3 chars of suffix
			parts := fullID[len("bench-"):]
			shortID := parts[:2]

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if i%2 == 0 {
					ResolveID(fullID, records)
				} else {
					ResolveID(shortID, records)
				}
			}
		})
	}
}

func BenchmarkGenerateID(b *testing.B) {
	for _, size := range []int{0, 50, 500, 2000} {
		b.Run(fmt.Sprintf("existing=%d", size), func(b *testing.B) {
			existing := make(map[string]bool)
			for i := 0; i < size; i++ {
				existing[fmt.Sprintf("bench-%04d", i)] = true
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				GenerateID("bench", existing,
					fmt.Sprintf("title-%d", i), "desc", "user",
					"2025-07-01T00:00:00Z")
			}
		})
	}
}
