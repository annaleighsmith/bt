package cmd

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"syscall"

	"bt/internal"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:     "update [id]",
		Aliases: []string{"edit"},
		Short:   "", // set by helpGroups in root.go
		Args:    cobra.MaximumNArgs(1),
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
				selected, err := pickIssue("Select issue to update", records, false)
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

			updates := make(map[string]any)

			claim, _ := cmd.Flags().GetBool("claim")
			if claim {
				updates["assignee"] = internal.CurrentUser()
				updates["status"] = "in_progress"
			}

			if cmd.Flags().Changed("status") {
				s, _ := cmd.Flags().GetString("status")
				if !internal.ValidStatuses[s] {
					return fmt.Errorf("invalid status: %s", s)
				}
				updates["status"] = s
			}
			if cmd.Flags().Changed("priority") {
				p, _ := cmd.Flags().GetString("priority")
				pv, err := internal.ParsePriority(p)
				if err != nil {
					return err
				}
				updates["priority"] = pv
			}
			if cmd.Flags().Changed("type") {
				t, _ := cmd.Flags().GetString("type")
				if !internal.ValidTypes[t] {
					return fmt.Errorf("invalid type: %s", t)
				}
				updates["issue_type"] = t
			}
			if cmd.Flags().Changed("title") {
				v, _ := cmd.Flags().GetString("title")
				updates["title"] = v
			}
			if cmd.Flags().Changed("description") {
				v, _ := cmd.Flags().GetString("description")
				updates["description"] = v
			}
			if cmd.Flags().Changed("assignee") {
				v, _ := cmd.Flags().GetString("assignee")
				updates["assignee"] = v
			}

			// Handle label modifications
			if cmd.Flags().Changed("add-label") {
				labels, _ := cmd.Flags().GetStringSlice("add-label")
				existing := records[idx].Issue.Labels
				labelSet := make(map[string]bool)
				for _, l := range existing {
					labelSet[l] = true
				}
				for _, l := range labels {
					labelSet[l] = true
				}
				var merged []any
				for l := range labelSet {
					merged = append(merged, l)
				}
				updates["labels"] = merged
			}
			if cmd.Flags().Changed("rm-label") {
				labels, _ := cmd.Flags().GetStringSlice("rm-label")
				remove := make(map[string]bool)
				for _, l := range labels {
					remove[l] = true
				}
				existing := records[idx].Issue.Labels
				// If add-label also set, start from updated labels
				if updated, ok := updates["labels"]; ok {
					var kept []any
					for _, l := range updated.([]any) {
						if !remove[fmt.Sprint(l)] {
							kept = append(kept, l)
						}
					}
					updates["labels"] = kept
				} else {
					var kept []any
					for _, l := range existing {
						if !remove[l] {
							kept = append(kept, l)
						}
					}
					updates["labels"] = kept
				}
			}

			// Interactive form when picked interactively and no flags given
			if len(args) == 0 && len(updates) == 0 && !claim {
				formUpdates, err := interactiveUpdateForm(records[idx])
				if err != nil {
					return err
				}
				for k, v := range formUpdates {
					updates[k] = v
				}
			}

			if len(updates) == 0 {
				return fmt.Errorf("no updates specified")
			}

			updates["updated_at"] = internal.NowRFC3339()

			if err := internal.UpdateRecord(&records[idx], updates); err != nil {
				return err
			}
			if err := internal.SaveIssues(issuesPath, records); err != nil {
				return err
			}

			printResult("Updated", records[idx].Issue.ID, records[idx].Issue.Title)
			return nil
		},
	}
	cmd.Flags().String("status", "", "Set status")
	cmd.Flags().StringP("priority", "p", "", "Set priority (0-4 or P0-P4)")
	cmd.Flags().StringP("type", "t", "", "Set type")
	cmd.Flags().String("title", "", "Set title")
	cmd.Flags().StringP("description", "d", "", "Set description")
	cmd.Flags().StringP("assignee", "a", "", "Set assignee")
	cmd.Flags().StringSlice("add-label", nil, "Add label(s)")
	cmd.Flags().StringSlice("rm-label", nil, "Remove label(s)")
	cmd.Flags().Bool("claim", false, "Set assignee=$USER and status=in_progress")
	rootCmd.AddCommand(cmd)
}

// interactiveUpdateForm presents an edit form for common issue fields.
// Returns only the fields the user actually changed.
func interactiveUpdateForm(rec internal.IssueRecord) (map[string]any, error) {
	issue := rec.Issue

	title := issue.Title
	status := issue.Status
	priority := "none"
	if issue.Priority != nil {
		priority = fmt.Sprintf("P%d", *issue.Priority)
	}
	assignee := ""
	if issue.Assignee != nil {
		assignee = *issue.Assignee
	}

	statusOptions := make([]huh.Option[string], 0, len(internal.ValidStatuses))
	for _, s := range []string{"open", "in_progress", "blocked", "deferred", "closed", "tombstone"} {
		statusOptions = append(statusOptions, huh.NewOption(s, s))
	}

	priorityOptions := []huh.Option[string]{
		huh.NewOption("none", "none"),
		huh.NewOption("P0 (critical)", "P0"),
		huh.NewOption("P1 (urgent)", "P1"),
		huh.NewOption("P2 (normal)", "P2"),
		huh.NewOption("P3 (low)", "P3"),
		huh.NewOption("P4 (backlog)", "P4"),
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Title").
				Value(&title),
			huh.NewSelect[string]().
				Title("Status").
				Options(statusOptions...).
				Value(&status),
			huh.NewSelect[string]().
				Title("Priority").
				Options(priorityOptions...).
				Value(&priority),
			huh.NewInput().
				Title("Assignee").
				Value(&assignee),
		),
	).WithTheme(huh.ThemeBase()).WithKeyMap(formKeyMap())

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, fmt.Errorf("cancelled")
		}
		return nil, err
	}

	updates := make(map[string]any)

	if title != issue.Title {
		updates["title"] = title
	}
	if status != issue.Status {
		updates["status"] = status
	}

	origPri := "none"
	if issue.Priority != nil {
		origPri = fmt.Sprintf("P%d", *issue.Priority)
	}
	if priority != origPri {
		if priority == "none" {
			updates["priority"] = nil
		} else {
			pv, err := internal.ParsePriority(priority)
			if err != nil {
				return nil, err
			}
			updates["priority"] = pv
		}
	}

	assignee = strings.TrimSpace(assignee)
	origAssignee := ""
	if issue.Assignee != nil {
		origAssignee = *issue.Assignee
	}
	if assignee != origAssignee {
		if assignee == "" {
			updates["assignee"] = nil
		} else {
			updates["assignee"] = assignee
		}
	}

	return updates, nil
}
