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
