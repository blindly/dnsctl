package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/wk/dnsctl/internal/config"
	gitpkg "github.com/wk/dnsctl/internal/git"
)

var logN int

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Show the commit log",
	Long:  "Shows the commit history for the dnsctl repository.",
	RunE:  runLog,
}

func init() {
	logCmd.Flags().IntVarP(&logN, "number", "n", 20, "Number of log entries to show")
	rootCmd.AddCommand(logCmd)
}

func runLog(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	root, err := config.FindRoot(cwd)
	if err != nil {
		return err
	}

	repo, err := gitpkg.Open(root)
	if err != nil {
		return err
	}

	entries, err := repo.Log(logN)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		msg := entry.Message
		// Trim trailing newline from commit messages
		if len(msg) > 0 && msg[len(msg)-1] == '\n' {
			msg = msg[:len(msg)-1]
		}
		fmt.Printf("%s %s\n", entry.Hash, msg)
	}

	return nil
}
