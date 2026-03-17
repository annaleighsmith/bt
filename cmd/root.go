package cmd

import (
	"fmt"
	"os"
	"strings"

	"bt/internal"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:          "bt",
	Short:        "An AI issue tracker...for the minimalist",
	Version:      "0.1.0",
	SilenceUsage: true,
}

type helpEntry struct {
	name string
	desc string
}

type commandGroup struct {
	heading  string
	commands []helpEntry
}

var helpGroups = []commandGroup{
	{"Setup", []helpEntry{
		{"init", "Initialize a beads workspace"},
		{"prompt", "Add issue tracking docs to AGENTS.md or CLAUDE.md"},
	}},
	{"Track", []helpEntry{
		{"create", "Create a new issue (-q for ID-only output)"},
		{"update", "Update an issue"},
		{"close", "Close an issue"},
	}},
	{"View", []helpEntry{
		{"list", "List issues"},
		{"show", "Show issue details"},
		{"ready", "List ready issues (open, unblocked, not deferred)"},
	}},
	{"Manage", []helpEntry{
		{"dep", "Manage dependencies"},
		{"epic", "Manage epics"},
		{"archive", "Move old closed issues to archive.jsonl"},
	}},
}

func init() {
	rootCmd.SetHelpFunc(customHelp)

	// Sync descriptions from helpGroups → subcommand Short fields,
	// so per-command help (e.g. "bt create --help") stays accurate.
	cobra.OnInitialize(func() {
		cmdMap := make(map[string]*cobra.Command)
		for _, sub := range rootCmd.Commands() {
			cmdMap[sub.Name()] = sub
		}
		for _, g := range helpGroups {
			for _, e := range g.commands {
				if sub, ok := cmdMap[e.name]; ok {
					sub.Short = e.desc
				}
			}
		}
	})
}

func customHelp(cmd *cobra.Command, args []string) {
	if cmd != rootCmd {
		fmt.Fprint(cmd.OutOrStdout(), cmd.UsageString())
		return
	}

	isTTY := isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())

	bold := lipgloss.NewStyle()
	dim := lipgloss.NewStyle()
	if isTTY {
		bold = bold.Bold(true)
		dim = dim.Foreground(internal.ColorDim)
	}

	// Commands that support interactive mode (no args → picker/form)
	interactiveCmd := map[string]bool{
		"create": true, "update": true, "close": true, "show": true,
	}

	// Find max command name width for alignment (include reference labels and * suffix)
	maxWidth := 0
	refLabels := []string{"Statuses", "Types", "Priority"}
	for _, g := range helpGroups {
		for _, e := range g.commands {
			w := len(e.name)
			if interactiveCmd[e.name] {
				w += 2 // " *"
			}
			if w > maxWidth {
				maxWidth = w
			}
		}
	}
	for _, label := range refLabels {
		if len(label) > maxWidth {
			maxWidth = len(label)
		}
	}

	var b strings.Builder

	fmt.Fprintf(&b, "%s\n", bold.Render(fmt.Sprintf("bt v%s", rootCmd.Version)))
	fmt.Fprintf(&b, "A minimal issue tracker for AI agents and humans.\n\n")

	fmt.Fprintf(&b, "%s\n", bold.Render("Usage:"))
	fmt.Fprintf(&b, "  bt <command> [flags]\n\n")

	for _, g := range helpGroups {
		fmt.Fprintf(&b, "%s\n", bold.Render(g.heading+":"))
		for _, e := range g.commands {
			display := e.name
			if interactiveCmd[e.name] {
				display += " *"
			}
			fmt.Fprintf(&b, "  %-*s  %s\n", maxWidth, display, e.desc)
		}
		fmt.Fprintln(&b)
	}

	fmt.Fprintf(&b, "%s\n", bold.Render("Reference:"))
	fmt.Fprintf(&b, "  %-*s  %s\n", maxWidth, "Statuses", "open, in_progress, blocked, deferred, closed, tombstone")
	fmt.Fprintf(&b, "  %-*s  %s\n", maxWidth, "Types", "task, bug, feature, epic, chore, docs, question")
	fmt.Fprintf(&b, "  %-*s  %s\n\n", maxWidth, "Priority", "P0 (critical) → P4 (backlog)")

	fmt.Fprintf(&b, "%s\n", bold.Render("Tips:"))
	fmt.Fprintf(&b, "  * = interactive picker/form when run without args\n")
	fmt.Fprintf(&b, "  All read commands accept --json for machine-readable output.\n")
	fmt.Fprintf(&b, "  Short IDs work: %s resolves to %s if unambiguous.\n\n",
		dim.Render("\"a1b\""), dim.Render("\"pfx-a1b2\""))

	fmt.Fprintf(&b, "%s\n", bold.Render("Options:"))
	fmt.Fprintf(&b, "  -h, --help      Print help\n")
	fmt.Fprintf(&b, "  -v, --version   Print version\n")

	fmt.Fprint(cmd.OutOrStdout(), b.String())
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
