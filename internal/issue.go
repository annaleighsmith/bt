package internal

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

type Dependency struct {
	IssueID     string `json:"issue_id"`
	DependsOnID string `json:"depends_on_id"`
	Type        string `json:"type"`
	CreatedAt   string `json:"created_at"`
	CreatedBy   string `json:"created_by"`
	Metadata    string `json:"metadata"`
	ThreadID    string `json:"thread_id"`
}

type Issue struct {
	ID           string       `json:"id"`
	Title        string       `json:"title"`
	Description  string       `json:"description"`
	Status       string       `json:"status"`
	Priority     *int         `json:"priority"`
	IssueType    string       `json:"issue_type"`
	Assignee     *string      `json:"assignee"`
	Owner        string       `json:"owner"`
	Labels       []string     `json:"labels"`
	CreatedAt    string       `json:"created_at"`
	CreatedBy    string       `json:"created_by"`
	UpdatedAt    string       `json:"updated_at"`
	ClosedAt     *string      `json:"closed_at"`
	CloseReason  *string      `json:"close_reason"`
	Dependencies []Dependency `json:"dependencies"`
}

type IssueRecord struct {
	Issue Issue
	Raw   json.RawMessage
}

var (
	TerminalStatuses = map[string]bool{"closed": true, "tombstone": true}
	ValidStatuses    = map[string]bool{
		"open": true, "in_progress": true, "blocked": true,
		"deferred": true, "closed": true, "tombstone": true,
	}
	ValidTypes = map[string]bool{
		"task": true, "bug": true, "feature": true, "epic": true,
		"chore": true, "docs": true, "question": true,
	}
)

// LockBeads acquires a file lock on .beads/issues.lock.
// Pass syscall.LOCK_EX for writes, syscall.LOCK_SH for reads.
// Returns the lock file — caller must defer UnlockBeads(f).
func LockBeads(beadsDir string, how int) (*os.File, error) {
	lockPath := filepath.Join(beadsDir, "issues.lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("open lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), how); err != nil {
		f.Close()
		return nil, fmt.Errorf("flock: %w", err)
	}
	return f, nil
}

// UnlockBeads releases the lock and closes the file.
func UnlockBeads(f *os.File) {
	if f != nil {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}
}

func LoadIssues(path string) ([]IssueRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no issues file found — run 'bt init' first")
		}
		return nil, err
	}
	defer f.Close()

	var records []IssueRecord
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		raw := make([]byte, len(line))
		copy(raw, line)
		var issue Issue
		if err := json.Unmarshal(raw, &issue); err != nil {
			return nil, fmt.Errorf("unmarshal issue: %w", err)
		}
		records = append(records, IssueRecord{Issue: issue, Raw: json.RawMessage(raw)})
	}
	return records, scanner.Err()
}

func SaveIssues(path string, records []IssueRecord) error {
	sort.Slice(records, func(i, j int) bool {
		return records[i].Issue.ID < records[j].Issue.ID
	})

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "issues-*.jsonl")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	for _, rec := range records {
		if _, err := tmp.Write(rec.Raw); err != nil {
			tmp.Close()
			os.Remove(tmpName)
			return err
		}
		if _, err := tmp.WriteString("\n"); err != nil {
			tmp.Close()
			os.Remove(tmpName)
			return err
		}
	}

	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

func UpdateRecord(rec *IssueRecord, updates map[string]any) error {
	var m map[string]any
	if err := json.Unmarshal(rec.Raw, &m); err != nil {
		return err
	}
	for k, v := range updates {
		if v == nil {
			delete(m, k)
		} else {
			m[k] = v
		}
	}
	if _, labelsUpdated := updates["labels"]; labelsUpdated {
		if labels, ok := m["labels"]; ok {
			if arr, ok := labels.([]any); ok {
				sort.Slice(arr, func(i, j int) bool {
					return fmt.Sprint(arr[i]) < fmt.Sprint(arr[j])
				})
			}
		}
	}
	if deps, ok := m["dependencies"]; ok {
		if arr, ok := deps.([]any); ok {
			sort.Slice(arr, func(i, j int) bool {
				di, _ := arr[i].(map[string]any)
				dj, _ := arr[j].(map[string]any)
				ki := fmt.Sprintf("%s|%s", di["issue_id"], di["depends_on_id"])
				kj := fmt.Sprintf("%s|%s", dj["issue_id"], dj["depends_on_id"])
				return ki < kj
			})
		}
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	rec.Raw = raw
	return json.Unmarshal(raw, &rec.Issue)
}

func NewRecord(fields map[string]any) (IssueRecord, error) {
	raw, err := json.Marshal(fields)
	if err != nil {
		return IssueRecord{}, err
	}
	var issue Issue
	if err := json.Unmarshal(raw, &issue); err != nil {
		return IssueRecord{}, err
	}
	return IssueRecord{Issue: issue, Raw: raw}, nil
}

func GenerateID(prefix string, existingIDs map[string]bool, title, desc, creator, createdAt string) string {
	count := len(existingIDs)
	length := 3
	if count >= 50 {
		length = 4
	}
	if count >= 1600 {
		length = 5
	}

	nonce := 0
	for {
		input := fmt.Sprintf("%s|%s|%s|%s|%d", title, desc, creator, createdAt, nonce)
		hash := sha256.Sum256([]byte(input))

		num := new(big.Int).SetBytes(hash[:8])
		b36 := strings.ToLower(num.Text(36))
		for len(b36) < length {
			b36 = "0" + b36
		}
		suffix := b36[:length]
		id := prefix + "-" + suffix

		if !existingIDs[id] {
			return id
		}
		nonce++
	}
}

func ResolveID(fragment string, issues []IssueRecord) (int, error) {
	for i, rec := range issues {
		if rec.Issue.ID == fragment {
			return i, nil
		}
	}

	var matches []int
	for i, rec := range issues {
		id := rec.Issue.ID
		if parts := strings.SplitN(id, "-", 2); len(parts) == 2 && strings.HasPrefix(parts[1], fragment) {
			matches = append(matches, i)
		}
	}

	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) == 0 {
		return -1, fmt.Errorf("no issue matching '%s'", fragment)
	}

	var ids []string
	for _, i := range matches {
		ids = append(ids, issues[i].Issue.ID)
	}
	return -1, fmt.Errorf("ambiguous ID '%s' matches: %s", fragment, strings.Join(ids, ", "))
}

func GetPrefix(beadsDir string) (string, error) {
	configPath := filepath.Join(beadsDir, "config.yaml")
	data, err := os.ReadFile(configPath)
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "#") {
				continue
			}
			for _, key := range []string{"issue_prefix:", "issue-prefix:"} {
				if strings.HasPrefix(line, key) {
					val := strings.TrimSpace(strings.TrimPrefix(line, key))
					val = strings.Trim(val, "\"'")
					if val != "" {
						return val, nil
					}
				}
			}
		}
	}

	issuesPath := filepath.Join(beadsDir, "issues.jsonl")
	issues, err := LoadIssues(issuesPath)
	if err == nil && len(issues) > 0 {
		id := issues[0].Issue.ID
		if idx := strings.LastIndex(id, "-"); idx > 0 {
			return id[:idx], nil
		}
	}

	absPath, err := filepath.Abs(filepath.Dir(beadsDir))
	if err != nil {
		return "", err
	}
	return filepath.Base(absPath), nil
}

func FindBeadsDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		beads := filepath.Join(dir, ".beads")
		if info, err := os.Stat(beads); err == nil && info.IsDir() {
			if _, err := os.Stat(filepath.Join(beads, "issues.jsonl")); err == nil {
				return beads, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no .beads directory found (run 'bt init' first)")
		}
		dir = parent
	}
}

// IsBlockingDepType returns true for dependency types that block readiness.
// Matches br's is_blocking() set.
func IsBlockingDepType(t string) bool {
	switch t {
	case "blocks", "parent-child", "conditional-blocks", "waits-for":
		return true
	}
	return false
}

// EpicCounts holds child completion counts for an epic.
type EpicCounts struct {
	Total  int
	Closed int
}

// EpicChildCounts scans all records for parent-child deps and returns
// per-epic child counts. Keys are epic IDs (the depends_on_id in parent-child deps).
func EpicChildCounts(records []IssueRecord) map[string]EpicCounts {
	statusMap := make(map[string]string, len(records))
	for _, r := range records {
		statusMap[r.Issue.ID] = r.Issue.Status
	}

	counts := make(map[string]EpicCounts)
	for _, r := range records {
		for _, dep := range r.Issue.Dependencies {
			if dep.Type != "parent-child" {
				continue
			}
			c := counts[dep.DependsOnID]
			c.Total++
			if TerminalStatuses[statusMap[r.Issue.ID]] {
				c.Closed++
			}
			counts[dep.DependsOnID] = c
		}
	}
	return counts
}

func ParsePriority(s string) (int, error) {
	s = strings.TrimPrefix(strings.ToUpper(s), "P")
	var p int
	if _, err := fmt.Sscanf(s, "%d", &p); err != nil || p < 0 || p > 4 {
		return 0, fmt.Errorf("priority must be 0-4 or P0-P4")
	}
	return p, nil
}

func NowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func CurrentUser() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	if u := os.Getenv("LOGNAME"); u != "" {
		return u
	}
	return "unknown"
}
