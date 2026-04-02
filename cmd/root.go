// cmd/root.go
package cmd

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags
var Version = "dev"

func init() {
	godotenv.Load() // silently load .env if present
}

var rootCmd = &cobra.Command{
	Use:     "dnsctl",
	Short:   "Git-like CLI for DNS management",
	Long:    "A CLI tool that brings a Git-like workflow to DNS record management via Cloudflare.",
	Version: Version,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
