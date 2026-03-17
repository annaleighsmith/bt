package cmd

import (
	"fmt"
	"os"
	"strings"

	"bt/internal"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

const agentsBlurb = `## Issue Tracking

This project uses **bt** for issue tracking. Issues live in ` + "`.beads/issues.jsonl`" + `. Run ` + "`bt --help`" + ` for full usage.
`

func init() {
	cmd := &cobra.Command{
		Use:   "prompt",
		Short: "", // set by helpGroups in root.go
		Long:  "Run interactive prompts for project setup (e.g. adding issue tracking docs to AGENTS.md/CLAUDE.md).\nUse --inject <file> to skip prompts and write directly.",
		RunE: func(cmd *cobra.Command, args []string) error {
			inject, _ := cmd.Flags().GetString("inject")
			if inject != "" {
				return injectAgentsDocs(inject)
			}
			return interactiveAgentsDocs()
		},
	}
	cmd.Flags().String("inject", "", "Directly inject issue tracking docs into a file (creates if needed)")
	rootCmd.AddCommand(cmd)
}

// injectAgentsDocs writes the blurb to a specific file, creating it if needed.
func injectAgentsDocs(target string) error {
	if data, err := os.ReadFile(target); err == nil {
		if strings.Contains(string(data), "## Issue Tracking") {
			fmt.Printf("Issue tracking section already present in %s\n", target)
			return nil
		}
	}

	f, err := os.OpenFile(target, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("could not open %s: %w", target, err)
	}
	defer f.Close()

	// If new file, don't prepend extra newline
	info, _ := f.Stat()
	if info.Size() > 0 {
		f.WriteString("\n")
	}
	f.WriteString(agentsBlurb)

	done := lipgloss.NewStyle().Foreground(internal.ColorGreen).Render("✓")
	fmt.Printf("%s Added issue tracking section to %s\n", done, target)
	return nil
}

// interactiveAgentsDocs uses huh prompts to pick or create a target file.
func interactiveAgentsDocs() error {
	candidates := []string{"AGENTS.md", "CLAUDE.md"}
	var existing []string
	for _, f := range candidates {
		if _, err := os.Stat(f); err == nil {
			existing = append(existing, f)
		}
	}

	// Check if any already contain the blurb
	for _, f := range existing {
		data, err := os.ReadFile(f)
		if err == nil && strings.Contains(string(data), "## Issue Tracking") {
			fmt.Printf("Issue tracking section already present in %s\n", f)
			return nil
		}
	}

	banner := lipgloss.NewStyle().
		Bold(true).
		Foreground(internal.ColorGreen).
		Render("Add issue tracking docs?")
	fmt.Println()
	fmt.Println(banner)

	preview := lipgloss.NewStyle().
		Foreground(internal.ColorDim).
		PaddingLeft(2).
		Render(strings.TrimSpace(agentsBlurb))
	fmt.Println(preview)
	fmt.Println()

	// Build options: existing files + create options for missing ones + skip
	options := make([]huh.Option[string], 0, len(candidates)+1)
	for _, f := range candidates {
		if _, err := os.Stat(f); err == nil {
			options = append(options, huh.NewOption(f, f))
		} else {
			options = append(options, huh.NewOption(fmt.Sprintf("Create %s", f), f))
		}
	}
	options = append(options, huh.NewOption("Skip", ""))

	var target string
	err := huh.NewSelect[string]().
		Title("Which file?").
		Options(options...).
		Value(&target).
		WithTheme(huh.ThemeBase()).
		Run()
	if err != nil || target == "" {
		return nil
	}

	return injectAgentsDocs(target)
}
