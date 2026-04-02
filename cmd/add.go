package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	cfclient "github.com/wk/dnsctl/internal/cloudflare"
	"github.com/wk/dnsctl/internal/config"
	gitpkg "github.com/wk/dnsctl/internal/git"
	"github.com/wk/dnsctl/internal/state"
	"github.com/wk/dnsctl/internal/zone"
)

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add zones to track",
	Long:  "Fetches available zones from Cloudflare and lets you pick new ones to add.",
	RunE:  runAdd,
}

func init() {
	rootCmd.AddCommand(addCmd)
}

func runAdd(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	root, err := config.FindRoot(".")
	if err != nil {
		return err
	}

	cfg, err := config.Load(config.ConfigPath(root))
	if err != nil {
		return err
	}

	token := os.Getenv("CLOUDFLARE_API_TOKEN")
	client, err := cfclient.NewClient(token)
	if err != nil {
		return err
	}

	zones, err := client.ListZones(ctx)
	if err != nil {
		return err
	}

	// Filter out already-tracked zones
	tracked := make(map[string]bool)
	for _, z := range cfg.Zones {
		tracked[z] = true
	}

	var options []huh.Option[string]
	for _, z := range zones {
		if !tracked[z.Name] {
			options = append(options, huh.NewOption(z.Name, z.Name))
		}
	}

	if len(options) == 0 {
		fmt.Println("All available zones are already tracked.")
		return nil
	}

	var selectedZones []string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select zones to add").
				Options(options...).
				Value(&selectedZones),
		),
	)

	if err := form.Run(); err != nil {
		return fmt.Errorf("zone selection: %w", err)
	}

	if len(selectedZones) == 0 {
		fmt.Println("No zones selected.")
		return nil
	}

	// Build zone ID map
	zoneIDMap := make(map[string]string)
	for _, z := range zones {
		zoneIDMap[z.Name] = z.ID
	}

	// Load existing state
	st, err := state.Load(config.StatePath(root))
	if err != nil {
		return err
	}

	repo, err := gitpkg.Open(root)
	if err != nil {
		return err
	}

	// Pull records for new zones
	for _, zoneName := range selectedZones {
		zoneID := zoneIDMap[zoneName]
		fmt.Printf("Pulling records for %s...\n", zoneName)

		records, idMap, err := client.ListRecords(ctx, zoneID, zoneName)
		if err != nil {
			return fmt.Errorf("pulling %s: %w", zoneName, err)
		}

		zf := &zone.ZoneFile{
			Zone:    zoneName,
			Records: records,
		}
		if err := zone.WriteFile(filepath.Join(root, zoneName+".yaml"), zf); err != nil {
			return err
		}

		st.Zones[zoneName] = &state.ZoneState{
			ZoneID:  zoneID,
			Records: idMap,
		}

		cfg.Zones = append(cfg.Zones, zoneName)
		fmt.Printf("  %d records\n", len(records))
	}

	// Save updated config and state
	if err := config.Save(config.ConfigPath(root), cfg); err != nil {
		return err
	}
	if err := state.Save(config.StatePath(root), st); err != nil {
		return err
	}

	if err := repo.AddAllAndCommit(fmt.Sprintf("add: track %d new zone(s)", len(selectedZones))); err != nil {
		return err
	}

	fmt.Printf("Added %d zone(s).\n", len(selectedZones))
	return nil
}
