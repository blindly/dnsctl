package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	cfclient "github.com/wk/dnsctl/internal/cloudflare"
	"github.com/wk/dnsctl/internal/config"
	gitpkg "github.com/wk/dnsctl/internal/git"
	"github.com/wk/dnsctl/internal/state"
	"github.com/wk/dnsctl/internal/zone"
)

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Fetch DNS records from Cloudflare and update local files",
	Long:  "Fetches the current DNS records from Cloudflare for each tracked zone, writes YAML files, updates state.json, and auto-commits if anything changed.",
	RunE:  runPull,
}

func init() {
	rootCmd.AddCommand(pullCmd)
}

func runPull(cmd *cobra.Command, args []string) error {
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

	// Build a map of zone name -> zone ID from state
	zoneIDMap := make(map[string]string)
	for zoneName, zs := range st.Zones {
		zoneIDMap[zoneName] = zs.ZoneID
	}

	// For zones in config that aren't in state yet, fetch zone list
	needsZoneList := false
	for _, zoneName := range cfg.Zones {
		if _, ok := zoneIDMap[zoneName]; !ok {
			needsZoneList = true
			break
		}
	}
	if needsZoneList {
		zones, err := client.ListZones(ctx)
		if err != nil {
			return fmt.Errorf("listing zones: %w", err)
		}
		for _, z := range zones {
			zoneIDMap[z.Name] = z.ID
		}
	}

	var failures []string

	for _, zoneName := range cfg.Zones {
		zoneID, ok := zoneIDMap[zoneName]
		if !ok {
			fmt.Fprintf(os.Stderr, "warning: zone %s not found in Cloudflare account, skipping\n", zoneName)
			failures = append(failures, zoneName)
			continue
		}

		fmt.Printf("Pulling %s...\n", zoneName)

		records, idMap, err := client.ListRecords(ctx, zoneID, zoneName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to pull %s: %v\n", zoneName, err)
			failures = append(failures, zoneName)
			continue
		}

		zf := &zone.ZoneFile{
			Zone:    zoneName,
			Records: records,
		}

		zoneFile := filepath.Join(root, zoneName+".yaml")
		if err := zone.WriteFile(zoneFile, zf); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to write %s: %v\n", zoneName, err)
			failures = append(failures, zoneName)
			continue
		}

		st.Zones[zoneName] = &state.ZoneState{
			ZoneID:  zoneID,
			Records: idMap,
		}

		fmt.Printf("  %d records\n", len(records))
	}

	if err := state.Save(config.StatePath(root), st); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	// Check if git actually has changes before committing
	status, err := repo.Status()
	if err != nil {
		return fmt.Errorf("checking status: %w", err)
	}

	if status.IsClean() {
		fmt.Println("Already up to date.")
		return nil
	}

	if err := repo.AddAllAndCommit("pull: sync records from Cloudflare"); err != nil {
		return fmt.Errorf("auto-commit: %w", err)
	}

	fmt.Println("Pulled and committed.")

	if len(failures) > 0 {
		fmt.Fprintf(os.Stderr, "warning: some zones failed to pull: %v\n", failures)
	}

	return nil
}
