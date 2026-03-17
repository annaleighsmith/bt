// +build ignore

package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"strings"
	"time"
)

func base36ID(i int, r *rand.Rand) string {
	input := fmt.Sprintf("bench|%d|%d", i, r.Int63())
	hash := sha256.Sum256([]byte(input))
	num := new(big.Int).SetBytes(hash[:8])
	b36 := strings.ToLower(num.Text(36))
	for len(b36) < 4 {
		b36 = "0" + b36
	}
	return "bench-" + b36[:4]
}

func main() {
	n := flag.Int("n", 5000, "number of issues to generate")
	out := flag.String("o", "", "output file path (default: stdout)")
	flag.Parse()

	statuses := []string{"open", "in_progress", "blocked", "deferred", "closed", "closed", "closed", "tombstone"}
	types := []string{"task", "bug", "feature", "epic", "chore", "docs", "question"}
	titles := []string{
		"Fix login redirect loop", "Add dark mode toggle", "Refactor auth middleware",
		"Update deps to latest", "Write API docs for v2", "Cache database queries",
		"Add rate limiting", "Fix memory leak in worker", "Migrate to new ORM",
		"Add CSV export", "Fix timezone handling", "Implement retry logic",
		"Add health check endpoint", "Fix flaky test suite", "Optimize image pipeline",
		"Add webhook support", "Fix CORS headers", "Implement audit logging",
		"Add search indexing", "Fix deadlock in queue", "Refactor error handling",
		"Add SSO integration", "Fix pagination offset", "Implement batch API",
		"Add metrics dashboard", "Fix race condition", "Optimize SQL queries",
		"Add email notifications", "Fix session expiry", "Implement file uploads",
	}
	creators := []string{"anna", "claude", "alex", "jordan", "sam"}
	labelPool := []string{"frontend", "backend", "infra", "urgent", "p0-fire", "tech-debt", "security", "perf", "ux", "docs"}
	descriptions := []string{
		"This needs to be fixed before the next release.\nSteps to reproduce:\n1. Open the app\n2. Click the button\n3. Observe the error",
		"We should add this feature to improve user experience.\n\nAcceptance criteria:\n- Works on mobile\n- Has loading state\n- Handles errors gracefully",
		"The current implementation is inefficient and causes slowdowns under load.\nProfiling shows 80% of time spent in JSON parsing.",
		"Refactoring needed to support the new architecture.\nThis is blocking several other tasks.",
		"Documentation is outdated and missing several endpoints.\nNeed to update OpenAPI spec as well.",
	}

	r := rand.New(rand.NewSource(42))
	base := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	// Track generated IDs to avoid collisions
	usedIDs := make(map[string]bool)
	ids := make([]string, *n)

	w := os.Stdout
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		w = f
	}

	// Pre-generate all IDs
	for i := 0; i < *n; i++ {
		for {
			id := base36ID(i, r)
			if !usedIDs[id] {
				usedIDs[id] = true
				ids[i] = id
				break
			}
		}
	}

	for i := 0; i < *n; i++ {
		created := base.Add(time.Duration(i*37+r.Intn(60)) * time.Minute)
		updated := created.Add(time.Duration(r.Intn(1440)) * time.Minute)
		status := statuses[r.Intn(len(statuses))]
		priority := r.Intn(5)
		creator := creators[r.Intn(len(creators))]
		title := fmt.Sprintf("%s (#%d)", titles[r.Intn(len(titles))], i)
		id := ids[i]
		desc := descriptions[r.Intn(len(descriptions))]

		// Content hash from title+desc
		hash := sha256.Sum256([]byte(title + desc))
		contentHash := fmt.Sprintf("%x", hash)

		// 0-3 random labels
		numLabels := r.Intn(4)
		var labels []string
		if numLabels > 0 {
			perm := r.Perm(len(labelPool))
			for j := 0; j < numLabels && j < len(perm); j++ {
				labels = append(labels, labelPool[perm[j]])
			}
		}

		m := map[string]any{
			// Core fields (bt knows about)
			"id":           id,
			"title":        title,
			"description":  desc,
			"status":       status,
			"priority":     priority,
			"issue_type":   types[r.Intn(len(types))],
			"created_at":   created.Format(time.RFC3339Nano),
			"created_by":   creator,
			"updated_at":   updated.Format(time.RFC3339Nano),
			"owner":        "bench@test.com",
			"labels":       labels,

			// br fields — mostly zero-valued but must exist for round-trip testing
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
			"content_hash":        contentHash,
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

		if status == "closed" || status == "tombstone" {
			closed := created.Add(time.Duration(r.Intn(10080)) * time.Minute)
			m["closed_at"] = closed.Format(time.RFC3339Nano)
			m["close_reason"] = "Done"
		}

		// ~20% have 1-3 dependencies on earlier issues
		if i > 0 && r.Float64() < 0.2 {
			numDeps := 1 + r.Intn(3) // 1-3 deps
			if numDeps > i {
				numDeps = i
			}
			deps := []map[string]any{}
			used := map[int]bool{}
			for d := 0; d < numDeps; d++ {
				target := r.Intn(i)
				if used[target] {
					continue
				}
				used[target] = true
				deps = append(deps, map[string]any{
					"issue_id":      id,
					"depends_on_id": ids[target],
					"type":          "blocks",
					"created_at":    created.Format(time.RFC3339Nano),
					"created_by":    creator,
					"metadata":      "{}",
					"thread_id":     "",
				})
			}
			if len(deps) > 0 {
				m["dependencies"] = deps
			}
		}

		line, _ := json.Marshal(m)
		w.Write(line)
		w.Write([]byte("\n"))
	}

	if *out != "" {
		fmt.Fprintf(os.Stderr, "Generated %d issues → %s\n", *n, *out)
	}
}
