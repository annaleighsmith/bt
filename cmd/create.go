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

func newCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create [title]",
		Short: "", // set by helpGroups in root.go
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			quiet, _ := cmd.Flags().GetBool("quiet")
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

			prefix, err := internal.GetPrefix(beadsDir)
			if err != nil {
				return err
			}

			existingIDs := make(map[string]bool)
			for _, r := range records {
				existingIDs[r.Issue.ID] = true
			}

			now := internal.NowRFC3339()
			creator := internal.CurrentUser()

			var title, desc, priority, issueType, assignee, parent string

			if len(args) == 0 && !quiet {
				// Interactive form
				if !isInteractive() {
					return fmt.Errorf("title required (or run interactively in a terminal)")
				}

				formValues, err := interactiveCreateForm(records)
				if err != nil {
					return err
				}
				title = formValues.title
				desc = formValues.description
				priority = formValues.priority
				issueType = formValues.issueType
				assignee = formValues.assignee
				parent = formValues.parent
			} else if len(args) == 0 {
				return fmt.Errorf("title required")
			} else {
				title = strings.Join(args, " ")
				desc, _ = cmd.Flags().GetString("description")
				priority, _ = cmd.Flags().GetString("priority")
				issueType, _ = cmd.Flags().GetString("type")
				assignee, _ = cmd.Flags().GetString("assignee")
				parent, _ = cmd.Flags().GetString("parent")
			}

			if title == "" {
				return fmt.Errorf("title cannot be empty")
			}

			id := internal.GenerateID(prefix, existingIDs, title, desc, creator, now)

			fields := map[string]any{
				"id":               id,
				"title":            title,
				"status":           "open",
				"created_at":       now,
				"created_by":       creator,
				"updated_at":       now,
				"source_repo":      ".",
				"compaction_level": 0,
				"original_size":    0,
			}

			if desc != "" {
				fields["description"] = desc
			}
			if priority != "" && priority != "none" {
				pv, err := internal.ParsePriority(priority)
				if err != nil {
					return err
				}
				fields["priority"] = pv
			}
			if issueType != "" {
				if !internal.ValidTypes[issueType] {
					return fmt.Errorf("invalid type: %s (valid: task, bug, feature, epic, chore, docs, question)", issueType)
				}
				fields["issue_type"] = issueType
			}
			if assignee != "" {
				fields["assignee"] = assignee
			}

			rec, err := internal.NewRecord(fields)
			if err != nil {
				return err
			}

			if parent != "" {
				parentIdx, err := internal.ResolveID(parent, records)
				if err != nil {
					return err
				}
				dep := map[string]any{
					"issue_id":      id,
					"depends_on_id": records[parentIdx].Issue.ID,
					"type":          "parent-child",
					"created_at":    now,
					"created_by":    creator,
					"metadata":      "{}",
					"thread_id":     "",
				}
				if err := internal.UpdateRecord(&rec, map[string]any{"dependencies": []any{dep}}); err != nil {
					return err
				}
			}

			records = append(records, rec)
			if err := internal.SaveIssues(issuesPath, records); err != nil {
				return err
			}

			if quiet {
				fmt.Println(id)
			} else {
				printResult("Created", id, title)
			}
			return nil
		},
	}
	cmd.Flags().StringP("priority", "p", "", "Priority (0-4 or P0-P4)")
	cmd.Flags().StringP("type", "t", "", "Issue type")
	cmd.Flags().StringP("assignee", "a", "", "Assignee")
	cmd.Flags().StringP("description", "d", "", "Description")
	cmd.Flags().String("parent", "", "Parent issue ID")
	cmd.Flags().BoolP("quiet", "q", false, "Print only the issue ID")
	return cmd
}

type createFormValues struct {
	title       string
	description string
	priority    string
	issueType   string
	assignee    string
	parent      string
}

func interactiveCreateForm(records []internal.IssueRecord) (*createFormValues, error) {
	v := &createFormValues{
		priority:  "none",
		issueType: "task",
	}

	priorityOptions := []huh.Option[string]{
		huh.NewOption("none", "none"),
		huh.NewOption("P0 (critical)", "P0"),
		huh.NewOption("P1 (urgent)", "P1"),
		huh.NewOption("P2 (normal)", "P2"),
		huh.NewOption("P3 (low)", "P3"),
		huh.NewOption("P4 (backlog)", "P4"),
	}

	typeOptions := []huh.Option[string]{
		huh.NewOption("task", "task"),
		huh.NewOption("bug", "bug"),
		huh.NewOption("feature", "feature"),
		huh.NewOption("epic", "epic"),
		huh.NewOption("chore", "chore"),
		huh.NewOption("docs", "docs"),
		huh.NewOption("question", "question"),
	}

	parentOptions := []huh.Option[string]{
		huh.NewOption("none", ""),
	}
	for _, rec := range records {
		if internal.TerminalStatuses[rec.Issue.Status] {
			continue
		}
		label := fmt.Sprintf("%s  %s", rec.Issue.ID, rec.Issue.Title)
		parentOptions = append(parentOptions, huh.NewOption(label, rec.Issue.ID))
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Title").
				Value(&v.title),
			huh.NewInput().
				Title("Description").
				Value(&v.description),
			huh.NewSelect[string]().
				Title("Type").
				Options(typeOptions...).
				Value(&v.issueType),
			huh.NewSelect[string]().
				Title("Priority").
				Options(priorityOptions...).
				Value(&v.priority),
			huh.NewInput().
				Title("Assignee").
				Value(&v.assignee),
			huh.NewSelect[string]().
				Title("Parent issue").
				Options(parentOptions...).
				Filtering(true).
				Height(10).
				Value(&v.parent),
		),
	).WithTheme(huh.ThemeBase())

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, fmt.Errorf("cancelled")
		}
		return nil, err
	}

	return v, nil
}

func init() {
	cmd := newCreateCmd()
	cmd.Aliases = []string{"add"}
	rootCmd.AddCommand(cmd)
}
