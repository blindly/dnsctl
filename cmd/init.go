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

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new dnsctl repository",
	Long:  "Authenticates with Cloudflare, lets you pick zones to track, and does an initial pull.",
	RunE:  runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	dir, _ := os.Getwd()

	if _, err := os.Stat(filepath.Join(dir, ".dnsctl")); err == nil {
		return fmt.Errorf("already a dnsctl repository")
	}

	token := os.Getenv("CLOUDFLARE_API_TOKEN")
	client, err := cfclient.NewClient(token)
	if err != nil {
		return err
	}

	fmt.Println("Verifying API token...")
	if err := client.VerifyToken(ctx); err != nil {
		return err
	}
	fmt.Println("Authenticated successfully.")

	zones, err := client.ListZones(ctx)
	if err != nil {
		return err
	}

	if len(zones) == 0 {
		return fmt.Errorf("no zones found in your Cloudflare account")
	}

	var options []huh.Option[string]
	for _, z := range zones {
		options = append(options, huh.NewOption(z.Name, z.Name))
	}

	var selectedZones []string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select zones to track").
				Options(options...).
				Value(&selectedZones),
		),
	)

	if err := form.Run(); err != nil {
		return fmt.Errorf("zone selection: %w", err)
	}

	if len(selectedZones) == 0 {
		return fmt.Errorf("no zones selected")
	}

	repo, err := gitpkg.Init(dir)
	if err != nil {
		return err
	}

	dnsctlDir := filepath.Join(dir, ".dnsctl")
	if err := os.MkdirAll(dnsctlDir, 0755); err != nil {
		return fmt.Errorf("creating .dnsctl directory: %w", err)
	}

	zoneIDMap := make(map[string]string)
	for _, z := range zones {
		zoneIDMap[z.Name] = z.ID
	}

	cfg := &config.Config{
		Provider: "cloudflare",
		Zones:    selectedZones,
	}
	if err := config.Save(config.ConfigPath(dir), cfg); err != nil {
		return err
	}

	st := &state.State{Zones: make(map[string]*state.ZoneState)}

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
		if err := zone.WriteFile(filepath.Join(dir, zoneName+".yaml"), zf); err != nil {
			return err
		}

		st.Zones[zoneName] = &state.ZoneState{
			ZoneID:  zoneID,
			Records: idMap,
		}

		fmt.Printf("  %d records\n", len(records))
	}

	if err := state.Save(config.StatePath(dir), st); err != nil {
		return err
	}

	if err := repo.AddAllAndCommit("init: import zones from Cloudflare"); err != nil {
		return err
	}

	fmt.Println("Initialized dnsctl repository.")
	return nil
}
