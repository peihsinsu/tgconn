package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Token        string
	Provider     string
	AllowedChats []int64
	Timeout      time.Duration
	Debug        bool
	HistorySize  int
}

func Load() (*Config, error) {
	cfg := &Config{
		Token:       viper.GetString("token"),
		Provider:    viper.GetString("provider"),
		Timeout:     time.Duration(viper.GetInt("timeout")) * time.Second,
		Debug:       viper.GetBool("debug"),
		HistorySize: viper.GetInt("history_size"),
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 15 * time.Minute
	}
	if cfg.HistorySize == 0 {
		cfg.HistorySize = 10
	}
	for _, s := range viper.GetStringSlice("allowed_chats") {
		s = strings.TrimSpace(s)
		if id, err := strconv.ParseInt(s, 10, 64); err == nil {
			cfg.AllowedChats = append(cfg.AllowedChats, id)
		}
	}
	return cfg, nil
}

func (c *Config) Validate() error {
	if c.Token == "" {
		return fmt.Errorf("bot token is required (set TELEGRAM_BOT_TOKEN or token in config file)")
	}
	if c.Provider == "" {
		return fmt.Errorf("provider is required (use --provider flag or set in config)")
	}
	valid := map[string]bool{"claude": true, "codex": true, "gemini": true}
	if !valid[c.Provider] {
		return fmt.Errorf("unsupported provider %q; supported: claude, codex, gemini", c.Provider)
	}
	if len(c.AllowedChats) == 0 {
		return fmt.Errorf("at least one allowed chat ID is required (use --allow-chat or set allowed_chats in config)")
	}
	return nil
}

func (c *Config) IsAllowed(chatID int64) bool {
	for _, id := range c.AllowedChats {
		if id == chatID {
			return true
		}
	}
	return false
}
