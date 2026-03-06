package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/sgx-labs/same-telegram/internal/config"
)

var rootCmd = &cobra.Command{
	Use:   "same-telegram",
	Short: "Telegram plugin for SAME — remote vault management from your phone",
	Long: `same-telegram turns Telegram into a remote management GUI for SAME.

It runs as both a Claude Code hook plugin (receives lifecycle events)
and a Telegram bot daemon (enables vault management from your phone).`,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&config.OverrideConfigPath, "config", "", "Path to config file (default: ~/.same/telegram.toml)")
	rootCmd.AddCommand(hookCmd)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(messageCmd)
}
