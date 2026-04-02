package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	cfclient "github.com/wk/dnsctl/internal/cloudflare"
	"github.com/wk/dnsctl/internal/config"
	gitpkg "github.com/wk/dnsctl/internal/git"
	"github.com/wk/dnsctl/internal/state"
	"github.com/wk/dnsctl/internal/zone"
)

var commitMessage string

var commitCmd = &cobra.Command{
	Use:   "commit",
	Short: "Apply local DNS changes to Cloudflare",
	Long: `Applies local zone file changes to Cloudflare. Performs a remote drift check first,
then shows a summary of changes and asks for confirmation before making any API calls.`,
	RunE: runCommit,
}

func init() {
	commitCmd.Flags().StringVarP(&commitMessage, "message", "m", "", "Commit message (required)")
	_ = commitCmd.MarkFlagRequired("message")
	rootCmd.AddCommand(commitCmd)
}

func runCommit(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	root, err := config.FindRoot(cwd)
	if err != nil {
		return err
	}

	cfg, err := config.Load(config.ConfigPath(root))
	if err != nil {
		return err
	}

	st, err := state.Load(config.StatePath(root))
	if err != nil {
		return err
	}

	token := os.Getenv("CLOUDFLARE_API_TOKEN")
	client, err := cfclient.NewClient(token)
	if err != nil {
		return err
	}

	repo, err := gitpkg.Open(root)
	if err != nil {
		return err
	}

	// Remote drift check: compare current CF state against our state.json
	fmt.Println("Checking for remote drift...")
	for _, zoneName := range cfg.Zones {
		zs, ok := st.Zones[zoneName]
		if !ok {
			continue
		}

		remoteRecords, remoteIDMap, err := client.ListRecords(ctx, zs.ZoneID, zoneName)
		if err != nil {
			return fmt.Errorf("checking remote state for %s: %w", zoneName, err)
		}
		_ = remoteRecords

		// Check if any record IDs in state are no longer on remote, or remote has new records
		remoteKeys := make(map[string]bool)
		for key := range remoteIDMap {
			remoteKeys[key] = true
		}

		for key := range zs.Records {
			if !remoteKeys[key] {
				return fmt.Errorf("Remote has changed. Run 'dnsctl pull' first")
			}
		}

		// Also check if remote has records not in state (someone added records remotely)
		for key := range remoteIDMap {
			if _, inState := zs.Records[key]; !inState {
				return fmt.Errorf("Remote has changed. Run 'dnsctl pull' first")
			}
		}
	}

	// Compute changesets for each zone
	type zoneChangeset struct {
		zoneName string
		zoneID   string
		cs       state.Changeset
		zs       *state.ZoneState
		records  []zone.Record
	}

	var zoneChanges []zoneChangeset
	totalCreate, totalUpdate, totalDelete := 0, 0, 0

	for _, zoneName := range cfg.Zones {
		zs, ok := st.Zones[zoneName]
		if !ok {
			fmt.Fprintf(os.Stderr, "warning: no state for zone %s, skipping\n", zoneName)
			continue
		}

		zoneFile := filepath.Join(root, zoneName+".yaml")
		zf, err := zone.ReadFile(zoneFile)
		if err != nil {
			return fmt.Errorf("reading zone file for %s: %w", zoneName, err)
		}

		cs := state.ComputeChangeset(zs, zf.Records)
		if len(cs.Create) == 0 && len(cs.Update) == 0 && len(cs.Delete) == 0 {
			continue
		}

		zoneChanges = append(zoneChanges, zoneChangeset{
			zoneName: zoneName,
			zoneID:   zs.ZoneID,
			cs:       cs,
			zs:       zs,
			records:  zf.Records,
		})

		totalCreate += len(cs.Create)
		totalUpdate += len(cs.Update)
		totalDelete += len(cs.Delete)
	}

	if len(zoneChanges) == 0 {
		fmt.Println("Nothing to commit.")
		return nil
	}

	// Print summary
	fmt.Printf("Will create %d, update %d, delete %d records.\n", totalCreate, totalUpdate, totalDelete)

	// Show deletions explicitly
	for _, zc := range zoneChanges {
		for _, op := range zc.cs.Delete {
			parts := strings.SplitN(op.RecordKey, "]", 3)
			if len(parts) == 3 {
				fmt.Printf("Will DELETE: %s record '%s' (%s)\n", strings.ToUpper(parts[0]), parts[1], parts[2])
			} else {
				fmt.Printf("Will DELETE: %s\n", op.RecordKey)
			}
		}
	}

	// Confirm
	fmt.Print("Proceed? [y/N] ")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
	if answer != "y" && answer != "yes" {
		fmt.Println("Aborted.")
		return nil
	}

	// Execute API calls and track failures
	var failures []string
	allSucceeded := true

	for _, zc := range zoneChanges {
		zoneName := zc.zoneName
		zoneID := zc.zoneID
		cs := zc.cs
		zs := zc.zs

		// Deletes first
		for _, op := range cs.Delete {
			if err := client.DeleteRecord(ctx, zoneID, op.RecordID); err != nil {
				fmt.Fprintf(os.Stderr, "error deleting %s from %s: %v\n", op.RecordKey, zoneName, err)
				failures = append(failures, fmt.Sprintf("delete %s/%s", zoneName, op.RecordKey))
				allSucceeded = false
				continue
			}
			delete(zs.Records, op.RecordKey)
		}

		// Creates
		for _, op := range cs.Create {
			newID, err := client.CreateRecord(ctx, zoneID, op.Record, zoneName)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error creating %s %s in %s: %v\n", op.Record.Type, op.Record.Name, zoneName, err)
				failures = append(failures, fmt.Sprintf("create %s/%s %s", zoneName, op.Record.Type, op.Record.Name))
				allSucceeded = false
				continue
			}
			zs.Records[op.Record.Key()] = newID
		}

		// Updates
		for _, op := range cs.Update {
			if err := client.UpdateRecord(ctx, zoneID, op.RecordID, op.Record, zoneName); err != nil {
				fmt.Fprintf(os.Stderr, "error updating %s %s in %s: %v\n", op.Record.Type, op.Record.Name, zoneName, err)
				failures = append(failures, fmt.Sprintf("update %s/%s %s", zoneName, op.Record.Type, op.Record.Name))
				allSucceeded = false
				continue
			}
			// Record ID stays the same for updates
		}
	}

	// Save updated state
	if err := state.Save(config.StatePath(root), st); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	if allSucceeded {
		if err := repo.AddAllAndCommit(commitMessage); err != nil {
			return fmt.Errorf("git commit: %w", err)
		}
		fmt.Printf("Committed: %s\n", commitMessage)
	} else {
		fmt.Fprintf(os.Stderr, "\nSome operations failed. State has been saved for successful operations.\n")
		fmt.Fprintf(os.Stderr, "Failed operations:\n")
		for _, f := range failures {
			fmt.Fprintf(os.Stderr, "  - %s\n", f)
		}
		fmt.Fprintf(os.Stderr, "Fix the issues and run 'dnsctl commit' again.\n")
		return fmt.Errorf("commit completed with %d failure(s)", len(failures))
	}

	return nil
}
