package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wk/dnsctl/internal/config"
	"github.com/wk/dnsctl/internal/state"
	"github.com/wk/dnsctl/internal/zone"
)

var diffRaw bool

var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show changes between local files and last committed state",
	Long: `Shows a structured record-level diff comparing local YAML files against state.json.
Use --raw to see the raw git diff output instead.`,
	RunE: runDiff,
}

func init() {
	diffCmd.Flags().BoolVar(&diffRaw, "raw", false, "Show raw git diff output")
	rootCmd.AddCommand(diffCmd)
}

func runDiff(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	root, err := config.FindRoot(cwd)
	if err != nil {
		return err
	}

	if diffRaw {
		gitCmd := exec.Command("git", "diff")
		gitCmd.Dir = root
		gitCmd.Stdout = os.Stdout
		gitCmd.Stderr = os.Stderr
		return gitCmd.Run()
	}

	cfg, err := config.Load(config.ConfigPath(root))
	if err != nil {
		return err
	}

	st, err := state.Load(config.StatePath(root))
	if err != nil {
		return err
	}

	anyDiff := false

	for _, zoneName := range cfg.Zones {
		zs, ok := st.Zones[zoneName]
		if !ok {
			fmt.Fprintf(os.Stderr, "warning: no state for zone %s, skipping\n", zoneName)
			continue
		}

		zoneFile := filepath.Join(root, zoneName+".yaml")
		zf, err := zone.ReadFile(zoneFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: cannot read %s: %v\n", zoneName, err)
			continue
		}

		cs := state.ComputeChangeset(zs, zf.Records)

		if len(cs.Create) == 0 && len(cs.Update) == 0 && len(cs.Delete) == 0 {
			continue
		}

		fmt.Printf("--- %s ---\n", zoneName)
		anyDiff = true

		for _, op := range cs.Create {
			r := op.Record
			fmt.Printf("+ %s %s %s\n", r.Type, r.Name, recordContent(r))
		}
		for _, op := range cs.Delete {
			// Parse the key to show type/name/content
			parts := strings.SplitN(op.RecordKey, "]", 3)
			if len(parts) == 3 {
				fmt.Printf("- %s %s %s\n", strings.ToUpper(parts[0]), parts[1], parts[2])
			} else {
				fmt.Printf("- %s\n", op.RecordKey)
			}
		}
		for _, op := range cs.Update {
			r := op.Record
			fmt.Printf("~ %s %s %s\n", r.Type, r.Name, recordContent(r))
		}
	}

	if !anyDiff {
		fmt.Println("No changes.")
	}

	return nil
}

func recordContent(r zone.Record) string {
	return r.Content
}
