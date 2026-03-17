package cmd

import (
	"errors"
	"fmt"
	"os"

	"bt/internal"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
)

// formKeyMap returns a keymap with esc added as a quit key.
func formKeyMap() *huh.KeyMap {
	km := huh.NewDefaultKeyMap()
	km.Quit = key.NewBinding(key.WithKeys("ctrl+c", "esc"))
	return km
}

var (
	successStyle = lipgloss.NewStyle()
	dimStyle     = lipgloss.NewStyle()
	boldStyle    = lipgloss.NewStyle()
)

func init() {
	if isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd()) {
		successStyle = successStyle.Foreground(internal.ColorGreen).Bold(true)
		dimStyle = dimStyle.Foreground(internal.ColorDim)
		boldStyle = boldStyle.Bold(true)
	}
}

// printResult prints a styled action result, e.g. "Created  bt-abc  My Title"
func printResult(action, id, detail string) {
	fmt.Printf("%s %s %s\n", successStyle.Render(action), boldStyle.Render(id), dimStyle.Render(detail))
}

// isInteractive returns true when stdin is a TTY.
func isInteractive() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())
}

// pickIssue shows a filterable select list and returns the chosen issue's ID.
// includeTerminal controls whether closed/tombstoned issues appear.
func pickIssue(title string, records []internal.IssueRecord, includeTerminal bool) (string, error) {
	if !isInteractive() {
		return "", fmt.Errorf("issue ID required (or run interactively in a terminal)")
	}

	var options []huh.Option[string]
	for _, rec := range records {
		if !includeTerminal && internal.TerminalStatuses[rec.Issue.Status] {
			continue
		}
		label := fmt.Sprintf("%s  %s  (%s)", rec.Issue.ID, rec.Issue.Title, rec.Issue.Status)
		options = append(options, huh.NewOption(label, rec.Issue.ID))
	}
	if len(options) == 0 {
		return "", fmt.Errorf("no issues to select from")
	}

	// Note: picker uses default keymap (no esc-to-quit) because esc is
	// used to exit filter mode in Select with Filtering(true).
	var selected string
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(title).
				Options(options...).
				Filtering(true).
				Height(15).
				Value(&selected),
		),
	).WithTheme(huh.ThemeBase()).Run()
	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", fmt.Errorf("cancelled")
		}
		return "", err
	}
	return selected, nil
}
