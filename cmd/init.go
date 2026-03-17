package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "", // set by helpGroups in root.go
		RunE: func(cmd *cobra.Command, args []string) error {
			prefix, _ := cmd.Flags().GetString("prefix")
			if prefix == "" {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				prefix = filepath.Base(cwd)
			}

			beadsDir := ".beads"
			if _, err := os.Stat(beadsDir); err == nil {
				fmt.Println(".beads/ already exists")
				return nil
			}

			if err := os.MkdirAll(beadsDir, 0755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(beadsDir, "issues.jsonl"), nil, 0644); err != nil {
				return err
			}
			config := fmt.Sprintf("issue_prefix: %s\n", prefix)
			if err := os.WriteFile(filepath.Join(beadsDir, "config.yaml"), []byte(config), 0644); err != nil {
				return err
			}
			gitignore := "*.db\n*.db-wal\n*.db-shm\nissues.lock\n"
			if err := os.WriteFile(filepath.Join(beadsDir, ".gitignore"), []byte(gitignore), 0644); err != nil {
				return err
			}

			fmt.Printf("Initialized .beads/ with prefix '%s'\n", prefix)
			fmt.Println("Run 'bt prompt' to add issue tracking docs to your CLAUDE.md or AGENTS.md")
			return nil
		},
	}
	cmd.Flags().String("prefix", "", "Issue ID prefix")
	rootCmd.AddCommand(cmd)
}
