package config

import (
	"testing"
	"time"

	"github.com/spf13/viper"
)

func reset() { viper.Reset() }

func TestValidate_MissingToken(t *testing.T) {
	cfg := &Config{Provider: "claude", AllowedChats: []int64{123}}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for missing token")
	}
}

func TestValidate_MissingProvider(t *testing.T) {
	cfg := &Config{Token: "tok", AllowedChats: []int64{123}}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for missing provider")
	}
}

func TestValidate_InvalidProvider(t *testing.T) {
	cfg := &Config{Token: "tok", Provider: "gpt5", AllowedChats: []int64{123}}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid provider")
	}
}

func TestValidate_NoAllowedChats(t *testing.T) {
	cfg := &Config{Token: "tok", Provider: "claude"}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for no allowed chats")
	}
}

func TestValidate_Valid(t *testing.T) {
	cfg := &Config{Token: "tok", Provider: "claude", AllowedChats: []int64{123}}
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestIsAllowed(t *testing.T) {
	cfg := &Config{AllowedChats: []int64{100, 200}}
	if !cfg.IsAllowed(100) {
		t.Error("expected 100 to be allowed")
	}
	if cfg.IsAllowed(300) {
		t.Error("expected 300 to be denied")
	}
}

func TestLoad_DefaultTimeout(t *testing.T) {
	defer reset()
	viper.Set("token", "tok")
	viper.Set("provider", "claude")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Timeout != 15*time.Minute {
		t.Errorf("expected 15m default timeout, got %s", cfg.Timeout)
	}
}

func TestValidate_InvalidExecMode(t *testing.T) {
	cfg := &Config{Token: "tok", Provider: "claude", AllowedChats: []int64{123}, ExecMode: "turbo"}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid exec-mode")
	}
}

func TestValidate_ValidExecModes(t *testing.T) {
	for _, mode := range []string{ExecModeAuto, ExecModeAsk, ExecModeSafe} {
		cfg := &Config{Token: "tok", Provider: "claude", AllowedChats: []int64{123}, ExecMode: mode}
		if err := cfg.Validate(); err != nil {
			t.Errorf("unexpected error for mode %q: %v", mode, err)
		}
	}
}

func TestLoad_DefaultExecMode(t *testing.T) {
	defer reset()
	viper.Set("token", "tok")
	viper.Set("provider", "claude")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ExecMode != ExecModeAsk {
		t.Errorf("expected default exec-mode %q, got %q", ExecModeAsk, cfg.ExecMode)
	}
}

func TestLoad_DefaultRetention(t *testing.T) {
	defer reset()
	viper.Set("token", "tok")
	viper.Set("provider", "claude")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TmpRetentionHours != 24 {
		t.Errorf("TmpRetentionHours default = %d, want 24", cfg.TmpRetentionHours)
	}
	if cfg.LogRetentionDays != 30 {
		t.Errorf("LogRetentionDays default = %d, want 30", cfg.LogRetentionDays)
	}
	if cfg.HistoryMaxEntries != 100 {
		t.Errorf("HistoryMaxEntries default = %d, want 100", cfg.HistoryMaxEntries)
	}
}

func TestLoad_ExplicitZeroPreserved(t *testing.T) {
	defer reset()
	viper.Set("token", "tok")
	viper.Set("provider", "claude")
	viper.Set("log_retention_days", 0)
	viper.Set("history_max_entries", 0)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogRetentionDays != 0 {
		t.Errorf("explicit 0 should be preserved, got %d", cfg.LogRetentionDays)
	}
	if cfg.HistoryMaxEntries != 0 {
		t.Errorf("explicit 0 should be preserved, got %d", cfg.HistoryMaxEntries)
	}
}

func TestValidate_NegativeRetention(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{"tmp negative", Config{Token: "t", Provider: "claude", AllowedChats: []int64{1}, TmpRetentionHours: -1}},
		{"logs negative", Config{Token: "t", Provider: "claude", AllowedChats: []int64{1}, LogRetentionDays: -1}},
		{"history negative", Config{Token: "t", Provider: "claude", AllowedChats: []int64{1}, HistoryMaxEntries: -1}},
	}
	for _, c := range cases {
		if err := c.cfg.Validate(); err == nil {
			t.Errorf("%s: expected validation error", c.name)
		}
	}
}

func TestLoad_ParseAllowedChats_CSV(t *testing.T) {
	defer reset()
	viper.Set("allowed_chats", []string{"111", "222", "333"})
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.AllowedChats) != 3 {
		t.Errorf("expected 3 chats, got %d: %v", len(cfg.AllowedChats), cfg.AllowedChats)
	}
	if cfg.AllowedChats[0] != 111 {
		t.Errorf("expected first chat 111, got %d", cfg.AllowedChats[0])
	}
}
