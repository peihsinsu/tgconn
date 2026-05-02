package provider

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// ErrPTYUnavailable is returned when the PTY cannot be started on this system.
// Callers may fall back to a non-PTY execution path.
var ErrPTYUnavailable = errors.New("PTY unavailable")

const (
	ptyCols           = 220
	ptyRows           = 50
	approvalWindowMax = 2048
)

// approvalPatterns matches the various ways Claude may present a permission prompt.
var approvalPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\(y/n\)\s*:?\s*$`),
	regexp.MustCompile(`(?i)\[y/n\]\s*:?\s*$`),
	regexp.MustCompile(`(?i)\(Y/n\)\s*:?\s*$`),
	regexp.MustCompile(`(?i)\(y/N\)\s*:?\s*$`),
	regexp.MustCompile(`(?i)allow\?\s*\[y/n\]`),
	regexp.MustCompile(`(?i)proceed\?\s*\[y/n\]`),
	regexp.MustCompile(`(?i)allow this tool`),
	regexp.MustCompile(`(?i)do you want to (?:allow|run|execute)`),
	regexp.MustCompile(`(?i)y to allow`),
	regexp.MustCompile(`(?i)press enter to allow`),
}

// ansiEscape matches ANSI terminal escape sequences.
var ansiEscape = regexp.MustCompile(`\x1b(?:\[[0-9;]*[a-zA-Z]|\][^\x07]*\x07)`)

func stripANSI(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
}

func detectApproval(cleanText string) bool {
	for _, p := range approvalPatterns {
		if p.MatchString(cleanText) {
			return true
		}
	}
	return false
}

// runClaudeWithPTY runs claude -p inside a PTY so its TUI-based permission prompts
// are captured. When a prompt is detected, fn is called; the user's y/n response
// is written back to the PTY master so Claude can continue.
func runClaudeWithPTY(ctx context.Context, question string, fn ApprovalFunc) (string, error) {
	cmd := exec.Command("claude", "-p", question)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrPTYUnavailable, err)
	}
	defer func() { _ = ptmx.Close() }()

	if err := pty.Setsize(ptmx, &pty.Winsize{Rows: ptyRows, Cols: ptyCols}); err != nil {
		slog.Warn("failed to set PTY size", "error", err)
	}

	pid := cmd.Process.Pid
	start := time.Now()
	slog.Info("subprocess started with PTY", "provider", "claude", "pid", pid)

	// allOut accumulates everything written to the PTY slave.
	// window is a sliding buffer used for approval pattern matching.
	var allOut bytes.Buffer
	var window []byte

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buf := make([]byte, 4096)
		for {
			n, rerr := ptmx.Read(buf)
			if n > 0 {
				chunk := buf[:n]
				allOut.Write(chunk)

				window = append(window, chunk...)
				if len(window) > approvalWindowMax {
					window = window[len(window)-approvalWindowMax:]
				}

				if detectApproval(stripANSI(string(window))) {
					slog.Info("PTY: approval prompt detected, forwarding to user")
					promptText := stripANSI(string(window))
					window = nil

					// fn blocks until the user taps Allow/Deny (or times out).
					// Claude is also blocked waiting for stdin input — no deadlock.
					allowed, ferr := fn(ctx, promptText)
					answer := "n\n"
					if ferr == nil && allowed {
						answer = "y\n"
						slog.Info("PTY: approval granted")
					} else {
						slog.Info("PTY: approval denied", "err", ferr)
					}
					if _, werr := io.WriteString(ptmx, answer); werr != nil {
						slog.Warn("PTY: failed to write answer", "error", werr)
					}
				}
			}
			if rerr != nil {
				// EIO / EOF is normal when the slave PTY closes after process exit.
				return
			}
		}
	}()

	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()

	select {
	case <-ctx.Done():
		elapsed := time.Since(start).Round(time.Millisecond)
		isTimeout := errors.Is(ctx.Err(), context.DeadlineExceeded)
		reason := "cancelled"
		if isTimeout {
			reason = "timeout"
		}
		slog.Warn("killing PTY subprocess",
			"provider", "claude", "pid", pid,
			"reason", reason, "elapsed", elapsed,
		)
		killGroup(cmd.Process)
		<-waitDone
		<-readDone

		msg := "claude cancelled"
		if isTimeout {
			msg = "claude timed out"
		}
		return "", errors.New(msg)

	case err := <-waitDone:
		<-readDone
		elapsed := time.Since(start).Round(time.Millisecond)
		output := cleanPTYOutput(allOut.String())

		if err != nil {
			slog.Error("PTY subprocess failed",
				"provider", "claude", "pid", pid,
				"elapsed", elapsed, "error", err,
			)
			if output != "" {
				return "", fmt.Errorf("claude: %s", output)
			}
			return "", fmt.Errorf("claude exited with error: %w", err)
		}
		slog.Info("PTY subprocess completed",
			"provider", "claude", "pid", pid,
			"elapsed", elapsed, "output_bytes", len(output),
		)
		return output, nil
	}
}

// cleanPTYOutput strips ANSI codes and removes lines that are purely approval-UI
// artifacts (box-drawing characters, echoed y/n answers).
func cleanPTYOutput(raw string) string {
	raw = stripANSI(raw)

	var kept []string
	for _, line := range strings.Split(raw, "\n") {
		if isApprovalUILine(line) {
			continue
		}
		kept = append(kept, line)
	}

	out := strings.Join(kept, "\n")
	// Remove stray echoed y/n from PTY echo.
	out = strings.ReplaceAll(out, "\r\ny\r\n", "\n")
	out = strings.ReplaceAll(out, "\r\nn\r\n", "\n")
	// Collapse \r\n → \n.
	out = strings.ReplaceAll(out, "\r\n", "\n")
	out = strings.ReplaceAll(out, "\r", "\n")
	// Collapse excessive blank lines.
	for strings.Contains(out, "\n\n\n") {
		out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(out)
}

// isApprovalUILine returns true for lines that are part of Claude's permission TUI.
func isApprovalUILine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	// Lines matching approval patterns.
	if detectApproval(strings.ToLower(trimmed)) {
		return true
	}
	// Lines dominated by box-drawing / TUI characters.
	boxRunes := []rune("╭─╮│╰╯├┤┼╔═╗║╚╝┌┐└┘●◆▶❯")
	boxCount := 0
	for _, r := range trimmed {
		for _, b := range boxRunes {
			if r == b {
				boxCount++
				break
			}
		}
	}
	return boxCount > 2
}
