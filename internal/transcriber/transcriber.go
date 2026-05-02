package transcriber

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const maxTranscriptRunes = 4000

// Check verifies the whisper binary is on $PATH.
func Check() error {
	if _, err := exec.LookPath("whisper"); err != nil {
		return fmt.Errorf("whisper binary not found in $PATH (install: pip install openai-whisper)")
	}
	return nil
}

// Transcribe runs whisper on audioPath and returns the transcribed text.
// whisper writes a .txt file alongside the audio; we read that file.
func Transcribe(ctx context.Context, audioPath string) (string, error) {
	dir := filepath.Dir(audioPath)
	base := strings.TrimSuffix(filepath.Base(audioPath), filepath.Ext(audioPath))
	outFile := filepath.Join(dir, base+".txt")

	cmd := exec.CommandContext(ctx, "whisper", audioPath,
		"--output_format", "txt",
		"--output_dir", dir,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("whisper failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		return "", fmt.Errorf("failed to read whisper output file: %w", err)
	}

	text := strings.TrimSpace(string(data))
	runes := []rune(text)
	if len(runes) > maxTranscriptRunes {
		runes = runes[:maxTranscriptRunes]
	}
	return string(runes), nil
}
