package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sgx-labs/same-telegram/internal/config"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Interactive setup: configure Telegram bot token and user ID",
	Long: `Walks you through configuring same-telegram:

1. Enter your Telegram bot token (from @BotFather)
2. Enter your Telegram user ID (from @userinfobot)
3. Writes ~/.same/telegram.toml`,
	RunE: runSetup,
}

func runSetup(cmd *cobra.Command, args []string) error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("same-telegram setup")
	fmt.Println("===================")
	fmt.Println()

	// Check for existing config
	if _, err := os.Stat(config.ConfigPath()); err == nil {
		fmt.Printf("Config already exists at %s\n", config.ConfigPath())
		fmt.Print("Overwrite? [y/N] ")
		answer, _ := reader.ReadString('\n')
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(answer)), "y") {
			fmt.Println("Setup cancelled.")
			return nil
		}
		fmt.Println()
	}

	// Get bot token
	fmt.Println("Step 1: Create a Telegram bot")
	fmt.Println("  1. Open Telegram and message @BotFather")
	fmt.Println("  2. Send /newbot and follow the prompts")
	fmt.Println("  3. Copy the bot token")
	fmt.Println()
	fmt.Print("Bot token: ")
	token, _ := reader.ReadString('\n')
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("bot token is required")
	}

	// Validate token format (should contain a colon)
	if !strings.Contains(token, ":") {
		return fmt.Errorf("invalid token format — should look like 123456:ABC-DEF...")
	}

	fmt.Println()

	// Get user ID
	fmt.Println("Step 2: Find your Telegram user ID")
	fmt.Println("  1. Open Telegram and message @userinfobot")
	fmt.Println("  2. It will reply with your user ID")
	fmt.Println()
	fmt.Print("Your Telegram user ID: ")
	userIDStr, _ := reader.ReadString('\n')
	userIDStr = strings.TrimSpace(userIDStr)
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil || userID <= 0 {
		return fmt.Errorf("invalid user ID — must be a positive number")
	}

	fmt.Println()

	// Write config
	if err := config.GenerateTemplate(token, userID); err != nil {
		return err
	}

	fmt.Printf("Config written to %s\n", config.ConfigPath())
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Start the daemon:  same-telegram serve")
	fmt.Println("  2. Message your bot on Telegram")
	fmt.Println("  3. Send /status to verify it works")
	fmt.Println()
	fmt.Println("To register as a SAME plugin, add to your vault's .same/plugins.json:")
	fmt.Println()

	exe, _ := os.Executable()
	if exe == "" {
		exe = "same-telegram"
	}
	fmt.Printf(`  {
    "name": "telegram",
    "event": "Stop",
    "command": "%s",
    "args": ["hook"],
    "timeout_ms": 5000,
    "enabled": true
  }
`, exe)

	return nil
}
