package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"bt/internal"
)

// btBinary returns the absolute path to the bt binary.
// It builds once per test run via TestMain if needed.
var btBinary string

func TestMain(m *testing.M) {
	// Build the binary into a temp location
	tmp, err := os.MkdirTemp("", "bt-test-bin-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmp)

	btBinary = filepath.Join(tmp, "bt")
	if runtime.GOOS == "windows" {
		btBinary += ".exe"
	}

	// Find the project root (one level up from cmd/)
	projRoot, err := filepath.Abs("..")
	if err != nil {
		fmt.Fprintf(os.Stderr, "abs: %v\n", err)
		os.Exit(1)
	}

	build := exec.Command("go", "build", "-o", btBinary, ".")
	build.Dir = projRoot
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build bt: %s\n%v\n", out, err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// runBT executes bt in the given directory and returns stdout.
// It fails the test on non-zero exit unless wantErr is true.
func runBT(t *testing.T, dir string, wantErr bool, args ...string) string {
	t.Helper()
	c := exec.Command(btBinary, args...)
	c.Dir = dir
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	err := c.Run()
	if wantErr {
		if err == nil {
			t.Fatalf("bt %v: expected error but got success\nstdout: %s", args, stdout.String())
		}
		return stderr.String() + stdout.String()
	}
	if err != nil {
		t.Fatalf("bt %v: %v\nstdout: %s\nstderr: %s", args, err, stdout.String(), stderr.String())
	}
	return stdout.String()
}

// setupWorkspace creates a temp dir, runs bt init, and returns the path.
func setupWorkspace(t *testing.T, prefix string) string {
	t.Helper()
	dir := t.TempDir()
	runBT(t, dir, false, "init", "--prefix", prefix)
	return dir
}

func TestIntegration_InitCreatesWorkspace(t *testing.T) {
	dir := t.TempDir()
	out := runBT(t, dir, false, "init", "--prefix", "tst")
	if !strings.Contains(out, "Initialized") {
		t.Fatalf("unexpected init output: %s", out)
	}

	// Verify files exist
	for _, f := range []string{".beads/issues.jsonl", ".beads/config.yaml", ".beads/.gitignore"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("missing %s: %v", f, err)
		}
	}

	// Verify config content
	cfg, _ := os.ReadFile(filepath.Join(dir, ".beads/config.yaml"))
	if !strings.Contains(string(cfg), "issue_prefix: tst") {
		t.Fatalf("config missing prefix: %s", cfg)
	}
}

func TestIntegration_InitIdempotent(t *testing.T) {
	dir := t.TempDir()
	runBT(t, dir, false, "init", "--prefix", "tst")
	out := runBT(t, dir, false, "init", "--prefix", "tst")
	if !strings.Contains(out, "already exists") {
		t.Fatalf("second init should say already exists: %s", out)
	}
}

func TestIntegration_QuickCreate(t *testing.T) {
	dir := setupWorkspace(t, "tst")

	// bt create -q returns just the ID (plus newline) -- key for agent scripting
	id := strings.TrimSpace(runBT(t, dir, false, "create", "-q", "Fix login bug"))
	if !strings.HasPrefix(id, "tst-") {
		t.Fatalf("expected tst-xxx, got %q", id)
	}
	// Should be exactly one line, no extra prose
	if strings.Count(id, "\n") > 0 {
		t.Fatalf("q should output single line ID, got %q", id)
	}
}

func TestIntegration_CreateWithFlags(t *testing.T) {
	dir := setupWorkspace(t, "proj")

	out := runBT(t, dir, false, "create", "Add caching layer",
		"-p", "P1", "-t", "feature", "-d", "Redis caching for API responses")
	if !strings.Contains(out, "Created proj-") {
		t.Fatalf("unexpected create output: %s", out)
	}

	// Verify via show --json
	id := extractID(t, out)
	showJSON := runBT(t, dir, false, "show", id, "--json")
	var issue map[string]any
	if err := json.Unmarshal([]byte(showJSON), &issue); err != nil {
		t.Fatalf("parse show json: %v\n%s", err, showJSON)
	}
	if issue["title"] != "Add caching layer" {
		t.Fatalf("title mismatch: %v", issue["title"])
	}
	if fmt.Sprintf("%v", issue["priority"]) != "1" {
		t.Fatalf("priority mismatch: %v", issue["priority"])
	}
	if issue["issue_type"] != "feature" {
		t.Fatalf("type mismatch: %v", issue["issue_type"])
	}
	if issue["description"] != "Redis caching for API responses" {
		t.Fatalf("description mismatch: %v", issue["description"])
	}
}

func TestIntegration_ListDefault(t *testing.T) {
	dir := setupWorkspace(t, "ls")

	id1 := strings.TrimSpace(runBT(t, dir, false, "create", "-q", "Open issue"))
	id2 := strings.TrimSpace(runBT(t, dir, false, "create", "-q", "Will close"))
	runBT(t, dir, false, "close", id2)

	// Default list excludes closed
	out := runBT(t, dir, false, "list")
	if !strings.Contains(out, id1) {
		t.Fatalf("open issue missing from list: %s", out)
	}
	if strings.Contains(out, id2) {
		t.Fatalf("closed issue should not appear in default list: %s", out)
	}

	// --all includes closed
	outAll := runBT(t, dir, false, "list", "--all")
	if !strings.Contains(outAll, id1) || !strings.Contains(outAll, id2) {
		t.Fatalf("--all should show both: %s", outAll)
	}
}

func TestIntegration_ListJSON(t *testing.T) {
	dir := setupWorkspace(t, "js")
	runBT(t, dir, false, "create", "-q", "First issue")
	runBT(t, dir, false, "create", "-q", "Second issue")

	out := runBT(t, dir, false, "list", "--json")
	var issues []json.RawMessage
	if err := json.Unmarshal([]byte(out), &issues); err != nil {
		t.Fatalf("parse list json: %v\n%s", err, out)
	}
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues in json, got %d", len(issues))
	}
}

func TestIntegration_ShowByFullID(t *testing.T) {
	dir := setupWorkspace(t, "shw")
	id := strings.TrimSpace(runBT(t, dir, false, "create", "-q", "Show me"))

	out := runBT(t, dir, false, "show", id)
	if !strings.Contains(out, "Show me") {
		t.Fatalf("show output missing title: %s", out)
	}
	if !strings.Contains(out, id) {
		t.Fatalf("show output missing ID: %s", out)
	}
}

func TestIntegration_ShowJSON(t *testing.T) {
	dir := setupWorkspace(t, "sj")
	id := strings.TrimSpace(runBT(t, dir, false, "create", "-q", "JSON show test"))

	out := runBT(t, dir, false, "show", id, "--json")
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if m["id"] != id {
		t.Fatalf("id mismatch: %v vs %s", m["id"], id)
	}
}

func TestIntegration_ShortIDResolution(t *testing.T) {
	dir := setupWorkspace(t, "sid")
	id := strings.TrimSpace(runBT(t, dir, false, "create", "-q", "Short ID test"))

	// Extract suffix (after "sid-")
	suffix := strings.TrimPrefix(id, "sid-")
	// Use just the first 2 chars of suffix as short ID
	short := suffix[:2]

	out := runBT(t, dir, false, "show", short)
	if !strings.Contains(out, "Short ID test") {
		t.Fatalf("short ID resolution failed: %s", out)
	}
}

func TestIntegration_Ready(t *testing.T) {
	dir := setupWorkspace(t, "rdy")

	id1 := strings.TrimSpace(runBT(t, dir, false, "create", "-q", "Ready task"))
	id2 := strings.TrimSpace(runBT(t, dir, false, "create", "-q", "Deferred task"))
	id3 := strings.TrimSpace(runBT(t, dir, false, "create", "-q", "Blocked task"))

	runBT(t, dir, false, "update", id2, "--status", "deferred")
	// Block id3 on id1
	runBT(t, dir, false, "dep", "add", id3, id1)

	out := runBT(t, dir, false, "ready")
	if !strings.Contains(out, id1) {
		t.Fatalf("ready should include open unblocked issue: %s", out)
	}
	if strings.Contains(out, id2) {
		t.Fatalf("deferred should not be ready: %s", out)
	}
	if strings.Contains(out, id3) {
		t.Fatalf("blocked issue should not be ready: %s", out)
	}
}

func TestIntegration_ReadyJSON(t *testing.T) {
	dir := setupWorkspace(t, "rj")
	runBT(t, dir, false, "create", "-q", "Ready JSON task")

	out := runBT(t, dir, false, "ready", "--json")
	var issues []json.RawMessage
	if err := json.Unmarshal([]byte(out), &issues); err != nil {
		t.Fatalf("parse ready json: %v\n%s", err, out)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 ready issue, got %d", len(issues))
	}
}

func TestIntegration_UpdateClaim(t *testing.T) {
	dir := setupWorkspace(t, "ucl")
	id := strings.TrimSpace(runBT(t, dir, false, "create", "-q", "Claim me"))

	runBT(t, dir, false, "update", id, "--claim")

	showJSON := runBT(t, dir, false, "show", id, "--json")
	var m map[string]any
	if err := json.Unmarshal([]byte(showJSON), &m); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m["status"] != "in_progress" {
		t.Fatalf("claim should set in_progress, got %v", m["status"])
	}
	if m["assignee"] == nil || m["assignee"] == "" {
		t.Fatalf("claim should set assignee, got %v", m["assignee"])
	}
}

func TestIntegration_UpdateFields(t *testing.T) {
	dir := setupWorkspace(t, "uf")
	id := strings.TrimSpace(runBT(t, dir, false, "create", "-q", "Update me"))

	runBT(t, dir, false, "update", id,
		"--title", "Updated title",
		"--priority", "P2",
		"--type", "bug",
		"--status", "blocked")

	showJSON := runBT(t, dir, false, "show", id, "--json")
	var m map[string]any
	json.Unmarshal([]byte(showJSON), &m)

	if m["title"] != "Updated title" {
		t.Fatalf("title: %v", m["title"])
	}
	if fmt.Sprintf("%v", m["priority"]) != "2" {
		t.Fatalf("priority: %v", m["priority"])
	}
	if m["issue_type"] != "bug" {
		t.Fatalf("type: %v", m["issue_type"])
	}
	if m["status"] != "blocked" {
		t.Fatalf("status: %v", m["status"])
	}
}

func TestIntegration_Close(t *testing.T) {
	dir := setupWorkspace(t, "cls")
	id := strings.TrimSpace(runBT(t, dir, false, "create", "-q", "Close me"))

	out := runBT(t, dir, false, "close", id, "--reason", "done and dusted")
	if !strings.Contains(out, "Closed") {
		t.Fatalf("close output: %s", out)
	}

	showJSON := runBT(t, dir, false, "show", id, "--json")
	var m map[string]any
	json.Unmarshal([]byte(showJSON), &m)
	if m["status"] != "closed" {
		t.Fatalf("status not closed: %v", m["status"])
	}
	if m["close_reason"] != "done and dusted" {
		t.Fatalf("close_reason: %v", m["close_reason"])
	}
	if m["closed_at"] == nil {
		t.Fatal("closed_at not set")
	}
}

func TestIntegration_DepAddListRemove(t *testing.T) {
	dir := setupWorkspace(t, "dep")
	id1 := strings.TrimSpace(runBT(t, dir, false, "create", "-q", "Parent task"))
	id2 := strings.TrimSpace(runBT(t, dir, false, "create", "-q", "Child task"))

	// Add dep: id2 is blocked by id1
	runBT(t, dir, false, "dep", "add", id2, id1)

	// List deps
	out := runBT(t, dir, false, "dep", "list", id2)
	if !strings.Contains(out, id1) {
		t.Fatalf("dep list should show blocker: %s", out)
	}

	// JSON dep list
	jsonOut := runBT(t, dir, false, "dep", "list", id2, "--json")
	var deps []map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &deps); err != nil {
		t.Fatalf("parse dep json: %v\n%s", err, jsonOut)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(deps))
	}
	if deps[0]["depends_on_id"] != id1 {
		t.Fatalf("dep blocker mismatch: %v", deps[0]["depends_on_id"])
	}

	// Remove dep
	runBT(t, dir, false, "dep", "rm", id2, id1)
	outAfter := runBT(t, dir, false, "dep", "list", id2)
	if !strings.Contains(outAfter, "No dependencies") {
		t.Fatalf("dep should be removed: %s", outAfter)
	}
}

func TestIntegration_DepBlocksReady(t *testing.T) {
	dir := setupWorkspace(t, "dbr")
	blocker := strings.TrimSpace(runBT(t, dir, false, "create", "-q", "Blocker"))
	blocked := strings.TrimSpace(runBT(t, dir, false, "create", "-q", "Blocked"))

	runBT(t, dir, false, "dep", "add", blocked, blocker)

	// blocked should not be in ready
	readyOut := runBT(t, dir, false, "ready")
	if strings.Contains(readyOut, blocked) {
		t.Fatalf("blocked issue in ready list")
	}

	// Close blocker, now blocked should be ready
	runBT(t, dir, false, "close", blocker)
	readyOut2 := runBT(t, dir, false, "ready")
	if !strings.Contains(readyOut2, blocked) {
		t.Fatalf("unblocked issue missing from ready: %s", readyOut2)
	}
}

func TestIntegration_DepSelfDependency(t *testing.T) {
	dir := setupWorkspace(t, "dsd")
	id := strings.TrimSpace(runBT(t, dir, false, "create", "-q", "Self dep test"))

	out := runBT(t, dir, true, "dep", "add", id, id)
	if !strings.Contains(out, "cannot depend on itself") {
		t.Fatalf("expected self-dep error, got: %s", out)
	}
}

func TestIntegration_DepDuplicate(t *testing.T) {
	dir := setupWorkspace(t, "ddup")
	id1 := strings.TrimSpace(runBT(t, dir, false, "create", "-q", "Task A"))
	id2 := strings.TrimSpace(runBT(t, dir, false, "create", "-q", "Task B"))

	runBT(t, dir, false, "dep", "add", id1, id2)

	out := runBT(t, dir, true, "dep", "add", id1, id2)
	if !strings.Contains(out, "already exists") {
		t.Fatalf("expected duplicate dep error, got: %s", out)
	}
}

func TestIntegration_AgentScriptingWorkflow(t *testing.T) {
	// Simulates a full agent workflow: init, create issues, triage, work, close
	dir := setupWorkspace(t, "agent")

	// Step 1: Quick-create several issues (agent captures IDs)
	bugID := strings.TrimSpace(runBT(t, dir, false, "create", "-q", "Login fails on Safari", "-t", "bug", "-p", "P1"))
	featureID := strings.TrimSpace(runBT(t, dir, false, "create", "-q", "Add dark mode", "-t", "feature", "-p", "P3"))
	choreID := strings.TrimSpace(runBT(t, dir, false, "create", "-q", "Update dependencies", "-t", "chore"))

	// Step 2: Agent reads current state as JSON
	listJSON := runBT(t, dir, false, "list", "--json")
	var issues []map[string]any
	if err := json.Unmarshal([]byte(listJSON), &issues); err != nil {
		t.Fatalf("parse list: %v", err)
	}
	if len(issues) != 3 {
		t.Fatalf("expected 3 issues, got %d", len(issues))
	}

	// Step 3: Agent claims the bug (highest priority)
	runBT(t, dir, false, "update", bugID, "--claim")

	// Step 4: Agent checks ready (chore/feature should be there, bug is in_progress)
	readyJSON := runBT(t, dir, false, "ready", "--json")
	var ready []map[string]any
	json.Unmarshal([]byte(readyJSON), &ready)
	if len(ready) != 3 {
		t.Fatalf("expected 3 ready (in_progress counts), got %d", len(ready))
	}

	// Step 5: Agent closes the bug
	runBT(t, dir, false, "close", bugID, "--reason", "fixed Safari cookie handling")

	// Step 6: Wire a dependency: dark mode blocked by dep update
	runBT(t, dir, false, "dep", "add", featureID, choreID)

	// Step 7: Verify ready reflects the dependency
	readyJSON2 := runBT(t, dir, false, "ready", "--json")
	var ready2 []map[string]any
	json.Unmarshal([]byte(readyJSON2), &ready2)
	// Only chore should be ready (bug closed, feature blocked)
	if len(ready2) != 1 {
		t.Fatalf("expected 1 ready issue, got %d", len(ready2))
	}

	// Step 8: Close chore, feature unblocks
	runBT(t, dir, false, "close", choreID)
	readyJSON3 := runBT(t, dir, false, "ready", "--json")
	var ready3 []map[string]any
	json.Unmarshal([]byte(readyJSON3), &ready3)
	if len(ready3) != 1 {
		t.Fatalf("expected 1 ready issue after unblock, got %d", len(ready3))
	}

	// Verify the remaining ready issue is the feature
	showOut := runBT(t, dir, false, "show", featureID, "--json")
	var feat map[string]any
	json.Unmarshal([]byte(showOut), &feat)
	if feat["status"] != "open" {
		t.Fatalf("feature should be open: %v", feat["status"])
	}

	// Step 9: List --status filter
	closedOut := runBT(t, dir, false, "list", "--status", "closed", "--json")
	var closedIssues []map[string]any
	json.Unmarshal([]byte(closedOut), &closedIssues)
	if len(closedIssues) != 2 {
		t.Fatalf("expected 2 closed issues, got %d", len(closedIssues))
	}
}

func TestIntegration_ParentChildDepType(t *testing.T) {
	dir := setupWorkspace(t, "pcd")

	epicID := strings.TrimSpace(runBT(t, dir, false, "create", "-q", "Epic task"))
	childID := strings.TrimSpace(runBT(t, dir, false, "create", "Subtask", "--parent", epicID))
	childID = extractID(t, childID)

	// Verify dep type is parent-child
	depJSON := runBT(t, dir, false, "dep", "list", childID, "--json")
	var deps []map[string]any
	if err := json.Unmarshal([]byte(depJSON), &deps); err != nil {
		t.Fatalf("parse dep json: %v\n%s", err, depJSON)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(deps))
	}
	if deps[0]["type"] != "parent-child" {
		t.Fatalf("expected parent-child dep type, got %v", deps[0]["type"])
	}

	// Child should NOT be ready (epic is open)
	readyOut := runBT(t, dir, false, "ready")
	if strings.Contains(readyOut, childID) {
		t.Fatalf("child with open parent should not be ready: %s", readyOut)
	}
	if !strings.Contains(readyOut, epicID) {
		t.Fatalf("epic should be ready: %s", readyOut)
	}

	// Close epic, child should become ready
	runBT(t, dir, false, "close", epicID)
	readyOut2 := runBT(t, dir, false, "ready")
	if !strings.Contains(readyOut2, childID) {
		t.Fatalf("child should be ready after parent closed: %s", readyOut2)
	}
}

func TestIntegration_RoundTripNoraFixture(t *testing.T) {
	// Load nora fixture, write it back, verify byte-for-byte identical
	fixture := filepath.Join("..", "testdata", "nora_issues.jsonl")
	original, err := os.ReadFile(fixture)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("fixture not available: testdata/nora_issues.jsonl")
		}
		t.Fatalf("read fixture: %v", err)
	}

	records, err := internal.LoadIssues(fixture)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "issues.jsonl")
	if err := internal.SaveIssues(outPath, records); err != nil {
		t.Fatalf("save: %v", err)
	}

	output, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}

	if !bytes.Equal(original, output) {
		origLines := bytes.Split(original, []byte("\n"))
		outLines := bytes.Split(output, []byte("\n"))
		for i := 0; i < len(origLines) && i < len(outLines); i++ {
			if !bytes.Equal(origLines[i], outLines[i]) {
				t.Fatalf("line %d differs:\n  want: %.200s\n  got:  %.200s", i+1, origLines[i], outLines[i])
			}
		}
		if len(origLines) != len(outLines) {
			t.Fatalf("line count: want %d, got %d", len(origLines), len(outLines))
		}
		t.Fatal("round-trip mismatch")
	}
}

func TestIntegration_ErrorCases(t *testing.T) {
	dir := setupWorkspace(t, "err")

	// Show nonexistent ID
	runBT(t, dir, true, "show", "nonexistent-id")

	// Update with no flags
	id := strings.TrimSpace(runBT(t, dir, false, "create", "-q", "Error test"))
	runBT(t, dir, true, "update", id)

	// Invalid priority
	runBT(t, dir, true, "create", "-q", "Bad priority", "-p", "P9")

	// Invalid type
	runBT(t, dir, true, "create", "-q", "Bad type", "-t", "invalid")

	// Invalid status
	runBT(t, dir, true, "update", id, "--status", "bogus")
}

func TestIntegration_Labels(t *testing.T) {
	dir := setupWorkspace(t, "lbl")
	id := strings.TrimSpace(runBT(t, dir, false, "create", "-q", "Label test"))

	runBT(t, dir, false, "update", id, "--add-label", "frontend", "--add-label", "urgent")

	showJSON := runBT(t, dir, false, "show", id, "--json")
	var m map[string]any
	json.Unmarshal([]byte(showJSON), &m)

	labels, ok := m["labels"].([]any)
	if !ok || len(labels) != 2 {
		t.Fatalf("expected 2 labels, got %v", m["labels"])
	}

	// Remove one
	runBT(t, dir, false, "update", id, "--rm-label", "urgent")
	showJSON2 := runBT(t, dir, false, "show", id, "--json")
	var m2 map[string]any
	json.Unmarshal([]byte(showJSON2), &m2)
	labels2, _ := m2["labels"].([]any)
	if len(labels2) != 1 {
		t.Fatalf("expected 1 label after rm, got %v", m2["labels"])
	}
}

func TestIntegration_EpicStatus(t *testing.T) {
	dir := setupWorkspace(t, "es")

	epicOut := runBT(t, dir, false, "create", "My Epic", "-t", "epic")
	epicID := extractID(t, epicOut)

	c1 := strings.TrimSpace(runBT(t, dir, false, "create", "-q", "Child 1", "--parent", epicID))
	c2 := strings.TrimSpace(runBT(t, dir, false, "create", "-q", "Child 2", "--parent", epicID))
	_ = strings.TrimSpace(runBT(t, dir, false, "create", "-q", "Child 3", "--parent", epicID))

	// Close 2 of 3
	runBT(t, dir, false, "close", c1)
	runBT(t, dir, false, "close", c2)

	out := runBT(t, dir, false, "epic", "status")
	if !strings.Contains(out, "2/3") {
		t.Fatalf("expected 2/3 in epic status, got: %s", out)
	}

	// JSON output
	jsonOut := runBT(t, dir, false, "epic", "status", "--json")
	var epics []map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &epics); err != nil {
		t.Fatalf("parse epic status json: %v\n%s", err, jsonOut)
	}
	if len(epics) != 1 {
		t.Fatalf("expected 1 epic, got %d", len(epics))
	}
	if fmt.Sprintf("%v", epics[0]["total_children"]) != "3" {
		t.Fatalf("expected 3 total children, got %v", epics[0]["total_children"])
	}
	if fmt.Sprintf("%v", epics[0]["closed_children"]) != "2" {
		t.Fatalf("expected 2 closed children, got %v", epics[0]["closed_children"])
	}
	if epics[0]["eligible_for_close"] != false {
		t.Fatalf("epic should not be eligible for close yet")
	}

	// --eligible-only should return empty
	eligibleOut := runBT(t, dir, false, "epic", "status", "--eligible-only")
	if !strings.Contains(eligibleOut, "No epics") {
		t.Fatalf("expected no eligible epics, got: %s", eligibleOut)
	}
}

func TestIntegration_EpicCloseEligible(t *testing.T) {
	dir := setupWorkspace(t, "ece")

	epicOut := runBT(t, dir, false, "create", "Closeable Epic", "-t", "epic")
	epicID := extractID(t, epicOut)

	c1 := strings.TrimSpace(runBT(t, dir, false, "create", "-q", "Child A", "--parent", epicID))
	c2 := strings.TrimSpace(runBT(t, dir, false, "create", "-q", "Child B", "--parent", epicID))

	runBT(t, dir, false, "close", c1)
	runBT(t, dir, false, "close", c2)

	// Verify eligible
	statusJSON := runBT(t, dir, false, "epic", "status", "--eligible-only", "--json")
	var epics []map[string]any
	json.Unmarshal([]byte(statusJSON), &epics)
	if len(epics) != 1 {
		t.Fatalf("expected 1 eligible epic, got %d", len(epics))
	}

	// Close eligible
	out := runBT(t, dir, false, "epic", "close-eligible")
	if !strings.Contains(out, epicID) {
		t.Fatalf("close-eligible output should contain epic ID: %s", out)
	}
	if !strings.Contains(out, "Closed 1") {
		t.Fatalf("expected 'Closed 1', got: %s", out)
	}

	// Verify epic is actually closed
	showJSON := runBT(t, dir, false, "show", epicID, "--json")
	var m map[string]any
	json.Unmarshal([]byte(showJSON), &m)
	if m["status"] != "closed" {
		t.Fatalf("epic should be closed, got %v", m["status"])
	}
	if m["close_reason"] != "All children completed" {
		t.Fatalf("unexpected close reason: %v", m["close_reason"])
	}
}

func TestIntegration_EpicCloseEligibleDryRun(t *testing.T) {
	dir := setupWorkspace(t, "edr")

	epicOut := runBT(t, dir, false, "create", "DryRun Epic", "-t", "epic")
	epicID := extractID(t, epicOut)

	c1 := strings.TrimSpace(runBT(t, dir, false, "create", "-q", "Child X", "--parent", epicID))
	runBT(t, dir, false, "close", c1)

	// Dry run should show what would close but not close
	out := runBT(t, dir, false, "epic", "close-eligible", "--dry-run")
	if !strings.Contains(out, epicID) {
		t.Fatalf("dry-run should list epic: %s", out)
	}
	if !strings.Contains(out, "Would close") {
		t.Fatalf("expected 'Would close', got: %s", out)
	}

	// Verify epic is still open
	showJSON := runBT(t, dir, false, "show", epicID, "--json")
	var m map[string]any
	json.Unmarshal([]byte(showJSON), &m)
	if m["status"] != "open" {
		t.Fatalf("epic should still be open after dry-run, got %v", m["status"])
	}
}

func TestIntegration_UpdateNoArgNonTTY(t *testing.T) {
	dir := setupWorkspace(t, "unt")
	runBT(t, dir, false, "create", "-q", "Some issue")

	// Running update with no args in non-TTY (test env) should error
	out := runBT(t, dir, true, "update", "--status", "in_progress")
	if !strings.Contains(out, "issue ID required") {
		t.Fatalf("expected 'issue ID required' error, got: %s", out)
	}
}

func TestIntegration_ShowNoArgNonTTY(t *testing.T) {
	dir := setupWorkspace(t, "snt")
	runBT(t, dir, false, "create", "-q", "Some issue")

	out := runBT(t, dir, true, "show")
	if !strings.Contains(out, "issue ID required") {
		t.Fatalf("expected 'issue ID required' error, got: %s", out)
	}
}

func TestIntegration_CloseNoArgNonTTY(t *testing.T) {
	dir := setupWorkspace(t, "cnt")
	runBT(t, dir, false, "create", "-q", "Some issue")

	out := runBT(t, dir, true, "close")
	if !strings.Contains(out, "issue ID required") {
		t.Fatalf("expected 'issue ID required' error, got: %s", out)
	}
}

// extractID pulls the issue ID from "Created proj-xxx: title" output
func extractID(t *testing.T, output string) string {
	t.Helper()
	// Format: "Created <id>: <title>"
	parts := strings.SplitN(output, " ", 3)
	if len(parts) < 2 {
		t.Fatalf("cannot extract ID from: %s", output)
	}
	return strings.TrimSuffix(parts[1], ":")
}
