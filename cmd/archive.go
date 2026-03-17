package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"bt/internal"

	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "archive",
		Short: "", // set by helpGroups in root.go
		RunE: func(cmd *cobra.Command, args []string) error {
			before, _ := cmd.Flags().GetString("before")
			if before == "" {
				return fmt.Errorf("--before is required (YYYY-MM-DD)")
			}
			cutoff, err := time.Parse("2006-01-02", before)
			if err != nil {
				return fmt.Errorf("invalid date %q (expected YYYY-MM-DD): %w", before, err)
			}

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
			archivePath := filepath.Join(beadsDir, "archive.jsonl")

			records, err := internal.LoadIssues(issuesPath)
			if err != nil {
				return err
			}

			var keep, archive []internal.IssueRecord
			for _, rec := range records {
				if !internal.TerminalStatuses[rec.Issue.Status] || rec.Issue.ClosedAt == nil {
					keep = append(keep, rec)
					continue
				}
				closedAt, err := time.Parse(time.RFC3339Nano, *rec.Issue.ClosedAt)
				if err != nil {
					keep = append(keep, rec)
					continue
				}
				if closedAt.Before(cutoff) {
					archive = append(archive, rec)
				} else {
					keep = append(keep, rec)
				}
			}

			if len(archive) == 0 {
				fmt.Println("No issues to archive")
				return nil
			}

			sort.Slice(archive, func(i, j int) bool {
				return archive[i].Issue.ID < archive[j].Issue.ID
			})

			// Remove from issues first (atomic via temp+rename), then append to archive.
			// If we crash after step 1, issues are gone but not archived — recoverable from git.
			// The reverse (archive first) risks duplicates across both files.
			if err := internal.SaveIssues(issuesPath, keep); err != nil {
				return err
			}

			f, err := os.OpenFile(archivePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
			if err != nil {
				return err
			}
			for _, rec := range archive {
				if _, err := f.Write(rec.Raw); err != nil {
					f.Close()
					return err
				}
				if _, err := f.WriteString("\n"); err != nil {
					f.Close()
					return err
				}
			}
			if err := f.Close(); err != nil {
				return err
			}

			fmt.Printf("Archived %d issues to archive.jsonl\n", len(archive))
			return nil
		},
	}
	cmd.Flags().String("before", "", "Archive closed issues before this date (YYYY-MM-DD)")
	rootCmd.AddCommand(cmd)
}
