package cmd

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cx009/tgconn/internal/bot"
	"github.com/cx009/tgconn/internal/config"
	"github.com/cx009/tgconn/internal/provider"
	"github.com/cx009/tgconn/internal/recorder"
	"github.com/cx009/tgconn/internal/transcriber"
)

var connectCmd = &cobra.Command{
	Use:   "connect",
	Short: "Start the Telegram bot",
	RunE:  runConnect,
}

func init() {
	connectCmd.Flags().StringSlice("allow-chat", nil, "allowed Telegram chat IDs (repeatable or comma-separated)")
	connectCmd.Flags().Int("timeout", 0, "LLM provider execution timeout in seconds (default 900)")
	connectCmd.Flags().Int("history-size", 0, "number of past Q&A exchanges to inject as context (default 10, 0 = disable)")
	connectCmd.Flags().Bool("enable-voice", false, "enable voice message transcription via whisper CLI")
	connectCmd.Flags().String("exec-mode", "ask", "execution mode: auto (skip all prompts), ask (Telegram approval), safe (auto-deny dangerous ops)")

	_ = viper.BindPFlag("allowed_chats", connectCmd.Flags().Lookup("allow-chat"))
	_ = viper.BindPFlag("timeout", connectCmd.Flags().Lookup("timeout"))
	_ = viper.BindPFlag("history_size", connectCmd.Flags().Lookup("history-size"))
	_ = viper.BindPFlag("enable_voice", connectCmd.Flags().Lookup("enable-voice"))
	_ = viper.BindPFlag("exec_mode", connectCmd.Flags().Lookup("exec-mode"))

	rootCmd.AddCommand(connectCmd)
}

func runConnect(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	level := slog.LevelInfo
	if cfg.Debug {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	slog.Info("configuration loaded",
		"provider", cfg.Provider,
		"exec_mode", cfg.ExecMode,
		"allowed_chats_count", len(cfg.AllowedChats),
		"timeout", cfg.Timeout,
		"debug", cfg.Debug,
	)

	p, err := provider.New(cfg.Provider)
	if err != nil {
		return err
	}

	slog.Info("checking provider binary", "provider", cfg.Provider)
	if err := p.Check(); err != nil {
		slog.Error("provider binary not found", "provider", cfg.Provider, "error", err)
		return err
	}
	slog.Info("provider binary ready", "provider", cfg.Provider)

	if cfg.EnableVoice {
		slog.Info("checking whisper binary for voice transcription")
		if err := transcriber.Check(); err != nil {
			slog.Error("whisper binary not found", "error", err)
			return err
		}
		slog.Info("whisper binary ready")
	}

	rec, err := recorder.New()
	if err != nil {
		return err
	}
	slog.Info("execution recorder ready", "log_dir", ".tgconn")

	b, err := bot.New(cfg, p, rec)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	slog.Info("starting bot, press Ctrl+C to stop")
	return b.Start(ctx)
}
