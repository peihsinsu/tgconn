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
	WhisperBackend  string // "auto" | "openai-whisper" | "whisper.cpp"; empty = auto
	WhisperModel    string // backend-specific model identifier; empty = backend default (openai-whisper only)
	ExecMode        string
	AnthropicAPIKey string
	MaxJobs         int    // max concurrent regular jobs per bot (0 = unlimited)
	MaxCronJobs     int    // max concurrent cron executions (0 = unlimited)
	Language        string // BCP 47 language tag for bot messages; empty = "en"

	TmpRetentionHours int // hours before tmp/ files are eligible for deletion (default 24)
	LogRetentionDays  int // days before daily *.jsonl logs are deleted; 0 disables (default 30)
	HistoryMaxEntries int // max entries per history_<chatID>.jsonl before startup compaction; 0 disables (default 100)
}

func Load() (*Config, error) {
	// Storage retention defaults — setting these via viper means an explicit
	// `0` in the config file is preserved (used to disable retention) while an
	// unset key falls back to the default.
	viper.SetDefault("tmp_retention_hours", 24)
	viper.SetDefault("log_retention_days", 30)
	viper.SetDefault("history_max_entries", 100)

	cfg := &Config{
		Token:             viper.GetString("token"),
		Provider:          viper.GetString("provider"),
		Timeout:           time.Duration(viper.GetInt("timeout")) * time.Second,
		Debug:             viper.GetBool("debug"),
		HistorySize:       viper.GetInt("history_size"),
		EnableVoice:       viper.GetBool("enable_voice"),
		WhisperBackend:    strings.TrimSpace(viper.GetString("whisper_backend")),
		WhisperModel:      strings.TrimSpace(viper.GetString("whisper_model")),
		ExecMode:          viper.GetString("exec_mode"),
		AnthropicAPIKey:   viper.GetString("anthropic_api_key"),
		MaxJobs:           viper.GetInt("max_jobs"),
		MaxCronJobs:       viper.GetInt("max_cron_jobs"),
		Language:          viper.GetString("language"),
		TmpRetentionHours: viper.GetInt("tmp_retention_hours"),
		LogRetentionDays:  viper.GetInt("log_retention_days"),
		HistoryMaxEntries: viper.GetInt("history_max_entries"),
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
	if c.TmpRetentionHours < 0 {
		return fmt.Errorf("tmp_retention_hours must be >= 0, got %d", c.TmpRetentionHours)
	}
	if c.LogRetentionDays < 0 {
		return fmt.Errorf("log_retention_days must be >= 0, got %d", c.LogRetentionDays)
	}
	if c.HistoryMaxEntries < 0 {
		return fmt.Errorf("history_max_entries must be >= 0, got %d", c.HistoryMaxEntries)
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
