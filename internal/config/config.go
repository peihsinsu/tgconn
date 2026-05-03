package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

const (
	ExecModeAuto = "auto"
	ExecModeAsk  = "ask"
	ExecModeSafe = "safe"
)

type Config struct {
	Token           string
	Provider        string
	AllowedChats    []int64
	Timeout         time.Duration
	Debug           bool
	HistorySize     int
	EnableVoice     bool
	ExecMode        string
	AnthropicAPIKey string
	MaxJobs         int // max concurrent regular jobs per bot (0 = unlimited)
	MaxCronJobs     int // max concurrent cron executions (0 = unlimited)
}

func Load() (*Config, error) {
	cfg := &Config{
		Token:           viper.GetString("token"),
		Provider:        viper.GetString("provider"),
		Timeout:         time.Duration(viper.GetInt("timeout")) * time.Second,
		Debug:           viper.GetBool("debug"),
		HistorySize:     viper.GetInt("history_size"),
		EnableVoice:     viper.GetBool("enable_voice"),
		ExecMode:        viper.GetString("exec_mode"),
		AnthropicAPIKey: viper.GetString("anthropic_api_key"),
		MaxJobs:         viper.GetInt("max_jobs"),
		MaxCronJobs:     viper.GetInt("max_cron_jobs"),
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 2 * time.Hour
	}
	if cfg.HistorySize == 0 {
		cfg.HistorySize = 10
	}
	if cfg.ExecMode == "" {
		cfg.ExecMode = ExecModeAsk
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
	switch c.ExecMode {
	case ExecModeAuto, ExecModeAsk, ExecModeSafe, "":
	default:
		return fmt.Errorf("invalid exec-mode %q; valid values: auto, ask, safe", c.ExecMode)
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
