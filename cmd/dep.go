package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"bt/internal"

	"github.com/spf13/cobra"
)

func init() {
	depCmd := &cobra.Command{
		Use:   "dep",
		Short: "", // set by helpGroups in root.go
	}

	addCmd := &cobra.Command{
		Use:   "add <id> <blocked-by>",
		Short: "Add dependency (id is blocked by blocked-by)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			beadsDir, err := internal.FindBeadsDir()
			if err != nil {
				return err
			}

			lf, err := internal.LockBeads(beadsDir, syscall.LOCK_EX)
			if err != nil {
				return err
			}
			defer internal.UnlockBeads(lf)

			issuesPath := filepath.Join(beadsDir, "issues.jsonl")

			records, err := internal.LoadIssues(issuesPath)
			if err != nil {
				return err
			}

			idx, err := internal.ResolveID(args[0], records)
			if err != nil {
				return err
			}
			blockerIdx, err := internal.ResolveID(args[1], records)
			if err != nil {
				return err
			}

			issueID := records[idx].Issue.ID
			blockerID := records[blockerIdx].Issue.ID

			if issueID == blockerID {
				return fmt.Errorf("an issue cannot depend on itself")
			}

			// Check for duplicate edge
			for _, dep := range records[idx].Issue.Dependencies {
				if dep.DependsOnID == blockerID {
					return fmt.Errorf("dependency on %s already exists", blockerID)
				}
			}

			// Build new dep
			now := internal.NowRFC3339()
			newDep := map[string]any{
				"issue_id":      issueID,
				"depends_on_id": blockerID,
				"type":          "blocks",
				"created_at":    now,
				"created_by":    internal.CurrentUser(),
				"metadata":      "{}",
				"thread_id":     "",
			}

			// Get existing deps from raw
			var m map[string]any
			if err := json.Unmarshal(records[idx].Raw, &m); err != nil {
				return err
			}

			var deps []any
			if existing, ok := m["dependencies"]; ok {
				if arr, ok := existing.([]any); ok {
					deps = arr
				}
			}
			deps = append(deps, newDep)

			if err := internal.UpdateRecord(&records[idx], map[string]any{
				"dependencies": deps,
				"updated_at":   now,
			}); err != nil {
				return err
			}
			return internal.SaveIssues(issuesPath, records)
		},
	}

	rmCmd := &cobra.Command{
		Use:   "rm <id> <blocked-by>",
		Short: "Remove dependency",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			beadsDir, err := internal.FindBeadsDir()
			if err != nil {
				return err
			}

			lf, err := internal.LockBeads(beadsDir, syscall.LOCK_EX)
			if err != nil {
				return err
			}
			defer internal.UnlockBeads(lf)

			issuesPath := filepath.Join(beadsDir, "issues.jsonl")

			records, err := internal.LoadIssues(issuesPath)
			if err != nil {
				return err
			}

			idx, err := internal.ResolveID(args[0], records)
			if err != nil {
				return err
			}
			blockerIdx, err := internal.ResolveID(args[1], records)
			if err != nil {
				return err
			}

			blockerID := records[blockerIdx].Issue.ID

			var m map[string]any
			if err := json.Unmarshal(records[idx].Raw, &m); err != nil {
				return err
			}

			var original, kept []any
			if existing, ok := m["dependencies"]; ok {
				if arr, ok := existing.([]any); ok {
					original = arr
					for _, d := range arr {
						dm, _ := d.(map[string]any)
						if fmt.Sprint(dm["depends_on_id"]) != blockerID {
							kept = append(kept, d)
						}
					}
				}
			}

			if len(kept) == len(original) {
				return fmt.Errorf("no dependency on %s found", blockerID)
			}

			updates := map[string]any{"updated_at": internal.NowRFC3339()}
			if len(kept) > 0 {
				updates["dependencies"] = kept
			} else {
				updates["dependencies"] = nil // remove field if empty
			}

			if err := internal.UpdateRecord(&records[idx], updates); err != nil {
				return err
			}
			return internal.SaveIssues(issuesPath, records)
		},
	}

	listCmd := &cobra.Command{
		Use:   "list <id>",
		Short: "List dependencies",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			beadsDir, err := internal.FindBeadsDir()
			if err != nil {
				return err
			}

			lf, err := internal.LockBeads(beadsDir, syscall.LOCK_SH)
			if err != nil {
				return err
			}
			defer internal.UnlockBeads(lf)

			records, err := internal.LoadIssues(filepath.Join(beadsDir, "issues.jsonl"))
			if err != nil {
				return err
			}

			idx, err := internal.ResolveID(args[0], records)
			if err != nil {
				return err
			}

			jsonOut, _ := cmd.Flags().GetBool("json")
			deps := records[idx].Issue.Dependencies

			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(deps)
			}

			if len(deps) == 0 {
				fmt.Println("No dependencies")
				return nil
			}

			statusMap := make(map[string]string)
			for _, r := range records {
				statusMap[r.Issue.ID] = r.Issue.Status
			}

			for _, dep := range deps {
				depStatus := statusMap[dep.DependsOnID]
				if depStatus == "" {
					depStatus = "unknown"
				}
				fmt.Printf("%s depends on %s (%s)\n", dep.IssueID, dep.DependsOnID, depStatus)
			}
			return nil
		},
	}
	listCmd.Flags().Bool("json", false, "JSON output")

	depCmd.AddCommand(addCmd, rmCmd, listCmd)
	rootCmd.AddCommand(depCmd)
}
