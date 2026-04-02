package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/wk/dnsctl/internal/config"
	gitpkg "github.com/wk/dnsctl/internal/git"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the working tree status",
	Long:  "Shows which local zone files have been modified since the last commit.",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
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

	result, err := repo.Status()
	if err != nil {
		return err
	}

	if result.IsClean() {
		fmt.Println("No local changes.")
		return nil
	}

	for file, statusCode := range result.Files() {
		fmt.Printf("%s %s\n", statusCode, file)
	}

	return nil
}
