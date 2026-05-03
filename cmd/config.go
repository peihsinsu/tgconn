package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cx009/tgconn/internal/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage tgconn configuration",
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Interactively create ~/.config/tgconn/config.yaml",
	RunE:  runConfigInit,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print resolved configuration (token masked)",
	RunE:  runConfigShow,
}

func init() {
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configShowCmd)
	rootCmd.AddCommand(configCmd)
}

func runConfigInit(_ *cobra.Command, _ []string) error {
	scanner := bufio.NewScanner(os.Stdin)
	prompt := func(label, def string) string {
		if def != "" {
			fmt.Printf("%s [%s]: ", label, def)
		} else {
			fmt.Printf("%s: ", label)
		}
		scanner.Scan()
		val := strings.TrimSpace(scanner.Text())
		if val == "" {
			return def
		}
		return val
	}

	token := prompt("Telegram bot token", "")
	providerName := prompt("LLM provider (claude/codex/gemini)", "claude")
	chats := prompt("Allowed chat IDs (comma-separated)", "")

	var chatList []string
	for _, s := range strings.Split(chats, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			chatList = append(chatList, s)
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	dir := filepath.Join(home, ".config", "tgconn")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("cannot create config directory: %w", err)
	}

	path := filepath.Join(dir, "config.yaml")
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	v.Set("token", token)
	v.Set("provider", providerName)
	v.Set("allowed_chats", chatList)

	if err := v.WriteConfigAs(path); err != nil {
		return fmt.Errorf("cannot write config: %w", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		return fmt.Errorf("cannot set config file permissions: %w", err)
	}
	fmt.Printf("config written to %s\n", path)
	return nil
}

func runConfigShow(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	token := cfg.Token
	switch {
	case len(token) > 8:
		token = token[:4] + strings.Repeat("*", len(token)-8) + token[len(token)-4:]
	case token != "":
		token = "***"
	}

	fmt.Printf("token:         %s\n", token)
	fmt.Printf("provider:      %s\n", cfg.Provider)
	fmt.Printf("allowed_chats: %v\n", cfg.AllowedChats)
	fmt.Printf("timeout:       %s\n", cfg.Timeout)
	fmt.Printf("debug:         %v\n", cfg.Debug)
	if f := viper.ConfigFileUsed(); f != "" {
		fmt.Printf("config file:   %s\n", f)
	}
	return nil
}
