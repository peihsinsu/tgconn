package downloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const maxFileBytes = 20 * 1024 * 1024 // 20 MB

var ErrFileTooLarge = errors.New("file exceeds the 20 MB limit")

type Downloader struct {
	api     *tgbotapi.BotAPI
	baseDir string
}

func New(api *tgbotapi.BotAPI, baseDir string) *Downloader {
	return &Downloader{api: api, baseDir: baseDir}
}

// Download fetches a Telegram file by fileID into baseDir/<chatID>/<filename>.
// Returns the absolute local path on success.
func (d *Downloader) Download(ctx context.Context, fileID string, fileSize int, chatID int64, filename string) (string, error) {
	if fileSize > maxFileBytes {
		return "", ErrFileTooLarge
	}

	tgFile, err := d.api.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		return "", fmt.Errorf("failed to get file info from Telegram: %w", err)
	}

	// Check size from Telegram metadata if caller didn't have it.
	if tgFile.FileSize > maxFileBytes {
		return "", ErrFileTooLarge
	}

	url := tgFile.Link(d.api.Token)

	destDir := filepath.Join(d.baseDir, fmt.Sprintf("%d", chatID))
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create download directory: %w", err)
	}

	localPath := filepath.Join(destDir, filename)
	path, err := downloadURL(ctx, url, localPath)
	if err != nil {
		return "", err
	}
	slog.Info("file downloaded", "chat_id", chatID, "filename", filename, "path", path)
	return path, nil
}

// downloadURL fetches url and writes the response body to localPath (creating parent dirs).
func downloadURL(ctx context.Context, url, localPath string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(localPath)
	if err != nil {
		return "", fmt.Errorf("failed to create local file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}
	return localPath, nil
}
