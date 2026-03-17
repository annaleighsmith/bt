package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"syscall"

	"bt/internal"

	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "", // set by helpGroups in root.go
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

			filename := "issues.jsonl"
			if archived, _ := cmd.Flags().GetBool("archived"); archived {
				filename = "archive.jsonl"
			}
			records, err := internal.LoadIssues(filepath.Join(beadsDir, filename))
			if err != nil {
				return err
			}

			showAll, _ := cmd.Flags().GetBool("all")
			archived, _ := cmd.Flags().GetBool("archived")
			statusFilter, _ := cmd.Flags().GetString("status")
			jsonOut, _ := cmd.Flags().GetBool("json")

			var filtered []internal.IssueRecord
			for _, rec := range records {
				if statusFilter != "" {
					if rec.Issue.Status == statusFilter {
						filtered = append(filtered, rec)
					}
				} else if showAll || archived || !internal.TerminalStatuses[rec.Issue.Status] {
					filtered = append(filtered, rec)
				}
			}

			limit, _ := cmd.Flags().GetInt("limit")

			sort.Slice(filtered, func(i, j int) bool {
				pi, pj := 99, 99
				if filtered[i].Issue.Priority != nil {
					pi = *filtered[i].Issue.Priority
				}
				if filtered[j].Issue.Priority != nil {
					pj = *filtered[j].Issue.Priority
				}
				if pi != pj {
					return pi < pj
				}
				return filtered[i].Issue.CreatedAt < filtered[j].Issue.CreatedAt
			})

			if limit > 0 && len(filtered) > limit {
				filtered = filtered[:limit]
			}

			if jsonOut {
				out := make([]json.RawMessage, 0)
				for _, rec := range filtered {
					out = append(out, rec.Raw)
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}

			if len(filtered) == 0 {
				fmt.Println("No open issues found.")
				return nil
			}
			internal.PrintIssueTable(filtered, true)
			return nil
		},
	}
	cmd.Flags().Bool("all", false, "Include closed/tombstone issues")
	cmd.Flags().Bool("archived", false, "List archived issues")
	cmd.Flags().String("status", "", "Filter by status")
	cmd.Flags().IntP("limit", "n", 50, "Max results (0 = unlimited)")
	cmd.Flags().Bool("json", false, "JSON output")
	rootCmd.AddCommand(cmd)
}
