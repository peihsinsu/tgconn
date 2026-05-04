package cmd

import (
	"bufio"
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "First-time setup: configure token and capture your Telegram chat ID",
	RunE:  runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(_ *cobra.Command, _ []string) error {
	stdin := bufio.NewReader(os.Stdin)

	ask := func(label, def string) (string, error) {
		if def != "" {
			fmt.Printf("%s [%s]: ", label, def)
		} else {
			fmt.Printf("%s: ", label)
		}
		line, err := stdin.ReadString('\n')
		if err != nil {
			return "", err
		}
		val := strings.TrimSpace(line)
		if val == "" {
			return def, nil
		}
		return val, nil
	}

	yesno := func(label string) (bool, error) {
		fmt.Printf("%s (y/n): ", label)
		line, err := stdin.ReadString('\n')
		if err != nil {
			return false, err
		}
		return strings.ToLower(strings.TrimSpace(line)) == "y", nil
	}

	// Check for existing config.
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	configPath := filepath.Join(home, ".config", "tgconn", "config.yaml")
	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("⚠️  Config already exists at %s\n", configPath)
		ok, err := yesno("Overwrite?")
		if err != nil || !ok {
			fmt.Println("Aborted.")
			return nil
		}
	}

	fmt.Println()
	fmt.Println("=== tgconn setup ===")
	fmt.Println()

	token, err := ask("Telegram bot token (from @BotFather)", "")
	if err != nil {
		return err
	}
	if token == "" {
		return fmt.Errorf("bot token is required")
	}

	providerName, err := ask("LLM provider", "claude")
	if err != nil {
		return err
	}

	// Write initial config without allowed_chats.
	if err := saveConfig(configPath, token, providerName, nil); err != nil {
		return err
	}
	fmt.Printf("\n✅ Config written to %s\n", configPath)

	// Connect to Telegram in open mode (no whitelist).
	fmt.Println("\n📱 Open Telegram and send any message to your bot.")
	fmt.Println("   A 6-digit verification code will appear here and in your chat.")
	fmt.Println("   Reply with the code in Telegram to confirm your chat ID.")
	fmt.Println("   Press Ctrl+C to abort.")
	fmt.Println()

	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return fmt.Errorf("cannot connect to Telegram (check token): %w", err)
	}
	fmt.Printf("🤖 Connected as @%s — waiting for a message...\n", api.Self.UserName)
	fmt.Println()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := api.GetUpdatesChan(u)

	// pending maps chatID → verification code currently awaiting reply.
	pending := make(map[int64]string)
	var savedChats []string

loop:
	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nAborted.")
			api.StopReceivingUpdates()
			return nil
		case update, ok := <-updates:
			if !ok {
				break loop
			}
			if update.Message == nil {
				continue
			}
			msg := update.Message
			chatID := msg.Chat.ID
			idStr := fmt.Sprintf("%d", chatID)
			displayName := senderLabel(msg)

			// Already saved — nothing to do.
			if slices.Contains(savedChats, idStr) {
				_, _ = api.Send(tgbotapi.NewMessage(chatID,
					"ℹ️ 此 Chat ID 已儲存。若還有其他帳號要加入，請從該帳號傳訊息。"))
				fmt.Printf("📨 Message from %s — Chat ID: %d (already saved)\n\n", displayName, chatID)
				continue
			}

			// Awaiting code reply from this chat.
			if code, waiting := pending[chatID]; waiting {
				if strings.TrimSpace(msg.Text) == code {
					delete(pending, chatID)
					savedChats = append(savedChats, idStr)
					_, _ = api.Send(tgbotapi.NewMessage(chatID,
						fmt.Sprintf("✅ 驗證成功！Chat ID %d 已儲存。\n設定完成後執行 tgconn connect 即可使用。", chatID)))
					fmt.Printf("   ✅ Code matched — Chat ID %d saved.\n\n", chatID)

					more, err := yesno("Add another chat ID (from a different account)?")
					if err != nil {
						return err
					}
					if !more {
						api.StopReceivingUpdates()
						break loop
					}
					fmt.Println("Waiting for a message from a different chat...")
					fmt.Println()
				} else {
					_, _ = api.Send(tgbotapi.NewMessage(chatID,
						fmt.Sprintf("❌ 驗證碼不正確，請重新輸入 %s", code)))
					fmt.Printf("   ❌ Wrong code from %s (chat %d)\n", displayName, chatID)
				}
				continue
			}

			// First message from this chat — generate and send a verification code.
			code, err := initVerifCode()
			if err != nil {
				return fmt.Errorf("failed to generate verification code: %w", err)
			}
			pending[chatID] = code

			_, _ = api.Send(tgbotapi.NewMessage(chatID,
				"🔐 收到連線請求。\n請輸入終端機上顯示的 6 位驗證碼。"))
			fmt.Printf("📨 Message from %s — Chat ID: %d\n", displayName, chatID)
			fmt.Printf("   🔐 Verification code: %s\n   Waiting for user to reply in Telegram...\n\n", code)
		}
	}

	if len(savedChats) == 0 {
		fmt.Println("No chat IDs saved; config left without allowed_chats.")
		return nil
	}

	if err := saveConfig(configPath, token, providerName, savedChats); err != nil {
		return err
	}
	fmt.Printf("\n✅ Setup complete — %d chat ID(s) saved.\n", len(savedChats))
	fmt.Println("   Run: tgconn connect")
	return nil
}

// initVerifCode generates a cryptographically random 6-digit code (000000–999999).
func initVerifCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func saveConfig(path, token, providerName string, chats []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("cannot create config directory: %w", err)
	}
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	v.Set("token", token)
	v.Set("provider", providerName)
	if chats == nil {
		chats = []string{}
	}
	v.Set("allowed_chats", chats)
	if err := v.WriteConfigAs(path); err != nil {
		return fmt.Errorf("cannot write config: %w", err)
	}
	return os.Chmod(path, 0600)
}

func senderLabel(msg *tgbotapi.Message) string {
	if msg.From == nil {
		return fmt.Sprintf("chat %d", msg.Chat.ID)
	}
	if msg.From.UserName != "" {
		return "@" + msg.From.UserName
	}
	return strings.TrimSpace(msg.From.FirstName + " " + msg.From.LastName)
}
