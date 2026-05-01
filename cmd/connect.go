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

	_ = viper.BindPFlag("allowed_chats", connectCmd.Flags().Lookup("allow-chat"))
	_ = viper.BindPFlag("timeout", connectCmd.Flags().Lookup("timeout"))
	_ = viper.BindPFlag("history_size", connectCmd.Flags().Lookup("history-size"))

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
