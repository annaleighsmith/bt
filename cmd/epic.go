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
	epicCmd := &cobra.Command{
		Use:   "epic",
		Short: "", // set by helpGroups in root.go
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show epic progress",
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

			counts := internal.EpicChildCounts(records)
			eligibleOnly, _ := cmd.Flags().GetBool("eligible-only")
			jsonOut, _ := cmd.Flags().GetBool("json")

			type epicInfo struct {
				ID               string `json:"id"`
				Title            string `json:"title"`
				Status           string `json:"status"`
				TotalChildren    int    `json:"total_children"`
				ClosedChildren   int    `json:"closed_children"`
				EligibleForClose bool   `json:"eligible_for_close"`
			}

			var epics []epicInfo
			for _, rec := range records {
				if rec.Issue.IssueType != "epic" || internal.TerminalStatuses[rec.Issue.Status] {
					continue
				}
				c := counts[rec.Issue.ID]
				eligible := c.Total > 0 && c.Closed == c.Total
				if eligibleOnly && !eligible {
					continue
				}
				epics = append(epics, epicInfo{
					ID:               rec.Issue.ID,
					Title:            rec.Issue.Title,
					Status:           rec.Issue.Status,
					TotalChildren:    c.Total,
					ClosedChildren:   c.Closed,
					EligibleForClose: eligible,
				})
			}

			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(epics)
			}

			if len(epics) == 0 {
				fmt.Println("No epics found")
				return nil
			}

			for _, e := range epics {
				status := "in progress"
				if e.EligibleForClose {
					status = "eligible to close"
				}
				fmt.Printf("%-12s  %d/%d closed  %-18s  %s\n",
					e.ID, e.ClosedChildren, e.TotalChildren, status, e.Title)
			}
			return nil
		},
	}
	statusCmd.Flags().Bool("eligible-only", false, "Show only epics eligible for closing")
	statusCmd.Flags().Bool("json", false, "JSON output")

	closeEligibleCmd := &cobra.Command{
		Use:   "close-eligible",
		Short: "Close epics with all children completed",
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

			counts := internal.EpicChildCounts(records)
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			jsonOut, _ := cmd.Flags().GetBool("json")

			// Find eligible epic indices
			var eligible []int
			for i, rec := range records {
				if rec.Issue.IssueType != "epic" || internal.TerminalStatuses[rec.Issue.Status] {
					continue
				}
				c := counts[rec.Issue.ID]
				if c.Total > 0 && c.Closed == c.Total {
					eligible = append(eligible, i)
				}
			}

			if len(eligible) == 0 {
				if jsonOut {
					fmt.Println("[]")
				} else {
					fmt.Println("No epics eligible for closing")
				}
				return nil
			}

			if dryRun {
				if jsonOut {
					var ids []string
					for _, idx := range eligible {
						ids = append(ids, records[idx].Issue.ID)
					}
					enc := json.NewEncoder(os.Stdout)
					enc.SetIndent("", "  ")
					return enc.Encode(ids)
				}
				fmt.Printf("Would close %d epic(s):\n", len(eligible))
				for _, idx := range eligible {
					fmt.Printf("  %s  %s\n", records[idx].Issue.ID, records[idx].Issue.Title)
				}
				return nil
			}

			now := internal.NowRFC3339()
			var closedIDs []string
			for _, idx := range eligible {
				if err := internal.UpdateRecord(&records[idx], map[string]any{
					"status":       "closed",
					"close_reason": "All children completed",
					"closed_at":    now,
					"updated_at":   now,
				}); err != nil {
					return fmt.Errorf("closing %s: %w", records[idx].Issue.ID, err)
				}
				closedIDs = append(closedIDs, records[idx].Issue.ID)
			}

			if err := internal.SaveIssues(issuesPath, records); err != nil {
				return err
			}

			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(closedIDs)
			}

			fmt.Printf("Closed %d epic(s):\n", len(closedIDs))
			for _, id := range closedIDs {
				fmt.Printf("  %s\n", id)
			}
			return nil
		},
	}
	closeEligibleCmd.Flags().Bool("dry-run", false, "Show what would be closed without closing")
	closeEligibleCmd.Flags().Bool("json", false, "JSON output")

	epicCmd.AddCommand(statusCmd, closeEligibleCmd)
	rootCmd.AddCommand(epicCmd)
}
