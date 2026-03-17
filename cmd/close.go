package cmd

import (
	"errors"
	"fmt"
	"path/filepath"
	"syscall"

	"bt/internal"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "close [id]",
		Short: "", // set by helpGroups in root.go
		Args:  cobra.MaximumNArgs(1),
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

			var idx int
			if len(args) == 0 {
				selected, err := pickIssue("Select issue to close", records, false)
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

			reason, _ := cmd.Flags().GetString("reason")

			// Interactive confirm when picked interactively
			if len(args) == 0 {
				var confirm bool
				err := huh.NewForm(
					huh.NewGroup(
						huh.NewInput().
							Title("Close reason (optional)").
							Value(&reason),
						huh.NewConfirm().
							Title(fmt.Sprintf("Close %s: %s?", records[idx].Issue.ID, records[idx].Issue.Title)).
							Value(&confirm),
					),
				).WithTheme(huh.ThemeBase()).WithKeyMap(formKeyMap()).Run()
				if err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						return fmt.Errorf("cancelled")
					}
					return err
				}
				if !confirm {
					fmt.Println("Aborted")
					return nil
				}
			}

			now := internal.NowRFC3339()
			updates := map[string]any{
				"status":     "closed",
				"closed_at":  now,
				"updated_at": now,
			}

			if reason != "" {
				updates["close_reason"] = reason
			}

			if err := internal.UpdateRecord(&records[idx], updates); err != nil {
				return err
			}
			if err := internal.SaveIssues(issuesPath, records); err != nil {
				return err
			}

			printResult("Closed", records[idx].Issue.ID, records[idx].Issue.Title)
			return nil
		},
	}
	cmd.Flags().String("reason", "", "Close reason")
	rootCmd.AddCommand(cmd)
}
