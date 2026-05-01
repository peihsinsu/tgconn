package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var rootCmd = &cobra.Command{
	Use:   "tgconn",
	Short: "Telegram LLM connector",
	Long:  "tgconn bridges Telegram to LLM providers (claude, codex, gemini) by running them as subprocesses in the current directory.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().String("config", "", "config file (default: ~/.tgconn/config.yaml)")
	rootCmd.PersistentFlags().String("provider", "", "LLM provider: claude|codex|gemini")
	rootCmd.PersistentFlags().Bool("debug", false, "enable verbose debug logging")

	_ = viper.BindPFlag("provider", rootCmd.PersistentFlags().Lookup("provider"))
	_ = viper.BindPFlag("debug", rootCmd.PersistentFlags().Lookup("debug"))
}

func initConfig() {
	cfgFile, _ := rootCmd.PersistentFlags().GetString("config")
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err == nil {
			viper.AddConfigPath(filepath.Join(home, ".tgconn"))
		}
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}
	viper.SetEnvPrefix("TGCONN")
	viper.AutomaticEnv()
	// TELEGRAM_BOT_TOKEN is a conventional env var name, not prefixed with TGCONN_
	_ = viper.BindEnv("token", "TELEGRAM_BOT_TOKEN")
	_ = viper.ReadInConfig()
}
