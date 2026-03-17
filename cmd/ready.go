package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"syscall"

	"bt/internal"

	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "ready",
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

			records, err := internal.LoadIssues(filepath.Join(beadsDir, "issues.jsonl"))
			if err != nil {
				return err
			}

			statusMap := make(map[string]string)
			for _, r := range records {
				statusMap[r.Issue.ID] = r.Issue.Status
			}

			var ready []internal.IssueRecord
			for _, rec := range records {
				s := rec.Issue.Status
				if internal.TerminalStatuses[s] || s == "deferred" || s == "blocked" {
					continue
				}

				blocked := false
				for _, dep := range rec.Issue.Dependencies {
					if internal.IsBlockingDepType(dep.Type) {
						depStatus := statusMap[dep.DependsOnID]
						if !internal.TerminalStatuses[depStatus] {
							blocked = true
							break
						}
					}
				}
				if blocked {
					continue
				}

				ready = append(ready, rec)
			}

			sort.Slice(ready, func(i, j int) bool {
				pi, pj := 99, 99
				if ready[i].Issue.Priority != nil {
					pi = *ready[i].Issue.Priority
				}
				if ready[j].Issue.Priority != nil {
					pj = *ready[j].Issue.Priority
				}
				if pi != pj {
					return pi < pj
				}
				return ready[i].Issue.CreatedAt < ready[j].Issue.CreatedAt
			})

			jsonOut, _ := cmd.Flags().GetBool("json")
			if jsonOut {
				out := make([]json.RawMessage, 0)
				for _, rec := range ready {
					out = append(out, rec.Raw)
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}

			internal.PrintIssueTable(ready, false)
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "JSON output")
	rootCmd.AddCommand(cmd)
}
