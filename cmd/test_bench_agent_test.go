package cmd

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

// benchSetup creates a workspace and returns the temp dir path.
func benchSetup(b *testing.B) string {
	b.Helper()
	dir := b.TempDir()
	c := runBTBench(b, dir, "init", "--prefix", "bch")
	if c != 0 {
		b.Fatal("init failed")
	}
	return dir
}

// runBTBench executes bt and returns exit code. Fails on error unless expected.
func runBTBench(b *testing.B, dir string, args ...string) int {
	b.Helper()
	cmd := newBTCmd(dir, args...)
	if err := cmd.Run(); err != nil {
		return 1
	}
	return 0
}

// runBTOut executes bt and returns stdout.
func runBTOut(b *testing.B, dir string, args ...string) string {
	b.Helper()
	cmd := newBTCmd(dir, args...)
	var out strings.Builder
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		b.Fatalf("bt %v: %v", args, err)
	}
	return out.String()
}

func newBTCmd(dir string, args ...string) *exec.Cmd {
	c := exec.Command(btBinary, args...)
	c.Dir = dir
	return c
}

func BenchmarkAgentTriageSprint(b *testing.B) {
	// q x20 → list --json → update x20 (set priority) → ready --json
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		dir := benchSetup(b)

		// Create 20 issues
		ids := make([]string, 20)
		for j := 0; j < 20; j++ {
			ids[j] = strings.TrimSpace(runBTOut(b, dir, "create", "-q", fmt.Sprintf("Sprint issue %d", j)))
		}

		// List all as JSON
		listOut := runBTOut(b, dir, "list", "--json")
		var issues []json.RawMessage
		json.Unmarshal([]byte(listOut), &issues)

		// Triage: set priorities
		for j, id := range ids {
			p := fmt.Sprintf("P%d", j%5)
			runBTBench(b, dir, "update", id, "--priority", p)
		}

		// Check ready
		runBTOut(b, dir, "ready", "--json")
	}
}

func BenchmarkAgentDepChain(b *testing.B) {
	// q x10 → dep add x9 (chain) → close from bottom up, check ready after each
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		dir := benchSetup(b)

		// Create chain of 10
		ids := make([]string, 10)
		for j := 0; j < 10; j++ {
			ids[j] = strings.TrimSpace(runBTOut(b, dir, "create", "-q", fmt.Sprintf("Chain %d", j)))
		}

		// Wire deps: each depends on previous
		for j := 1; j < 10; j++ {
			runBTBench(b, dir, "dep", "add", ids[j], ids[j-1])
		}

		// Close from bottom up, verify ready after each
		for j := 0; j < 10; j++ {
			runBTBench(b, dir, "close", ids[j])
			runBTOut(b, dir, "ready", "--json")
		}
	}
}

func BenchmarkAgentBulkClose(b *testing.B) {
	// q x50 → update --claim x10 → close x10 → verify via list --json
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		dir := benchSetup(b)

		ids := make([]string, 50)
		for j := 0; j < 50; j++ {
			ids[j] = strings.TrimSpace(runBTOut(b, dir, "create", "-q", fmt.Sprintf("Bulk issue %d", j)))
		}

		// Claim first 10
		for j := 0; j < 10; j++ {
			runBTBench(b, dir, "update", ids[j], "--claim")
		}

		// Close first 10
		for j := 0; j < 10; j++ {
			runBTBench(b, dir, "close", ids[j])
		}

		// Verify
		listOut := runBTOut(b, dir, "list", "--json")
		var remaining []json.RawMessage
		json.Unmarshal([]byte(listOut), &remaining)
		if len(remaining) != 40 {
			b.Fatalf("expected 40 open, got %d", len(remaining))
		}
	}
}

func BenchmarkAgentShortIDAtScale(b *testing.B) {
	// Pre-load 1000 issues → show <full-id> x100 (short IDs too ambiguous at scale)
	dir := benchSetup(b)

	ids := make([]string, 1000)
	for j := 0; j < 1000; j++ {
		ids[j] = strings.TrimSpace(runBTOut(b, dir, "create", "-q", fmt.Sprintf("Scale issue %d", j)))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < 100; j++ {
			runBTOut(b, dir, "show", ids[j*10])
		}
	}
}

func BenchmarkAgentJSONPipeLoop(b *testing.B) {
	// create → ready --json → parse JSON → claim first → repeat x10
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		dir := benchSetup(b)

		for j := 0; j < 10; j++ {
			runBTOut(b, dir, "create", "-q", fmt.Sprintf("Pipe issue %d", j))

			readyOut := runBTOut(b, dir, "ready", "--json")
			var issues []map[string]any
			json.Unmarshal([]byte(readyOut), &issues)

			if len(issues) > 0 {
				firstID, _ := issues[0]["id"].(string)
				if firstID != "" {
					runBTBench(b, dir, "update", firstID, "--claim")
				}
			}
		}
	}
}
