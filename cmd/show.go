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
	cmd := &cobra.Command{
		Use:   "show [id]",
		Short: "", // set by helpGroups in root.go
		Args:  cobra.MaximumNArgs(1),
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

			var idx int
			if len(args) == 0 {
				selected, err := pickIssue("Select issue to show", records, true)
				if err != nil {
					return err
				}
				idx, err = internal.ResolveID(selected, records)
				if err != nil {
					return err
				}
			} else {
				idx, err = internal.ResolveID(args[0], records)
				if err != nil {
					return err
				}
			}
			rec := records[idx]

			jsonOut, _ := cmd.Flags().GetBool("json")
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(rec.Raw)
			}

			issue := rec.Issue
			fmt.Printf("ID:          %s\n", issue.ID)
			fmt.Printf("Title:       %s\n", issue.Title)
			fmt.Printf("Status:      %s\n", issue.Status)
			if issue.Priority != nil {
				fmt.Printf("Priority:    P%d\n", *issue.Priority)
			}
			if issue.IssueType != "" {
				fmt.Printf("Type:        %s\n", issue.IssueType)
			}
			if issue.Assignee != nil {
				fmt.Printf("Assignee:    %s\n", *issue.Assignee)
			}
			if issue.Owner != "" {
				fmt.Printf("Owner:       %s\n", issue.Owner)
			}
			if issue.Description != "" {
				fmt.Printf("Description: %s\n", issue.Description)
			}
			fmt.Printf("Created:     %s by %s\n", issue.CreatedAt, issue.CreatedBy)
			if issue.UpdatedAt != "" {
				fmt.Printf("Updated:     %s\n", issue.UpdatedAt)
			}
			if issue.ClosedAt != nil {
				fmt.Printf("Closed:      %s\n", *issue.ClosedAt)
			}
			if issue.CloseReason != nil {
				fmt.Printf("Reason:      %s\n", *issue.CloseReason)
			}
			if len(issue.Labels) > 0 {
				fmt.Printf("Labels:      %v\n", issue.Labels)
			}

			if len(issue.Dependencies) > 0 {
				fmt.Println("Dependencies:")
				statusMap := make(map[string]string)
				for _, r := range records {
					statusMap[r.Issue.ID] = r.Issue.Status
				}
				for _, dep := range issue.Dependencies {
					depStatus := statusMap[dep.DependsOnID]
					if depStatus == "" {
						depStatus = "unknown"
					}
					fmt.Printf("  depends on %s (%s)\n", dep.DependsOnID, depStatus)
				}
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "JSON output")
	rootCmd.AddCommand(cmd)
}
