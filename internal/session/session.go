package session

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cx009/tgconn/internal/provider"
)

var sessionApprovalPatterns = []*regexp.Regexp{
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
	regexp.MustCompile(`(?i)requested permissions? to`),
}

var ansiEscapeSession = regexp.MustCompile(`\x1b(?:\[[0-9;]*[a-zA-Z]|\][^\x07]*\x07)`)

func stripANSISession(s string) string {
	return ansiEscapeSession.ReplaceAllString(s, "")
}

func detectApprovalSession(cleanText string) bool {
	for _, p := range sessionApprovalPatterns {
		if p.MatchString(cleanText) {
			return true
		}
	}
	return false
}

const IdleTimeout = 30 * time.Minute

// Session holds a persistent claude process for a single Telegram chat.
type Session struct {
	ChatID    int64
	StartedAt time.Time
	MsgCount  int

	mu         sync.Mutex
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	scanner    *bufio.Scanner
	lastMsg    time.Time
	cancel     context.CancelFunc
	closed     bool
	approvalFn provider.ApprovalFunc
}

// New creates an uninitialised Session for the given chat.
func New(chatID int64) *Session {
	return &Session{ChatID: chatID}
}

// Start launches claude with stream-json I/O.  execMode controls permission flags.
// approvalFn, when non-nil, is called whenever Claude outputs a permission prompt;
// the user's allow/deny response is written back to Claude's stdin.
func (s *Session) Start(ctx context.Context, execMode string, approvalFn provider.ApprovalFunc) error {
	s.approvalFn = approvalFn
	args := []string{
		"-p",
		"--output-format", "stream-json",
		"--input-format", "stream-json",
		"--verbose",
	}
	switch execMode {
	case "auto":
		args = append(args, "--dangerously-skip-permissions")
	case "safe":
		args = append(args, "--permission-mode", "plan")
	// "ask": no extra flag; session mode does not support interactive PTY approval yet
	}

	sCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(sCtx, "claude", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("session: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("session: stdout pipe: %w", err)
	}
	cmd.Stderr = nil // discard stderr

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("session: start claude: %w", err)
	}

	s.mu.Lock()
	s.cmd = cmd
	s.stdin = stdin
	s.scanner = bufio.NewScanner(stdout)
	s.scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	s.cancel = cancel
	s.StartedAt = time.Now()
	s.lastMsg = time.Now()
	s.closed = false
	s.mu.Unlock()

	slog.Info("interactive session started", "chat_id", s.ChatID, "pid", cmd.Process.Pid, "exec_mode", execMode)
	return nil
}

// inMessage is the inner message object Claude expects.
type inMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// inEvent is what we write to Claude's stdin (stream-json input format).
type inEvent struct {
	Type    string    `json:"type"`
	Message inMessage `json:"message"`
}

// outEvent is one line from Claude's stdout.
type outEvent struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype,omitempty"`
	Result  string `json:"result,omitempty"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Send writes a user message to Claude and blocks until the result event arrives.
func (s *Session) Send(ctx context.Context, message string) (string, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return "", fmt.Errorf("session is closed")
	}
	s.MsgCount++
	s.lastMsg = time.Now()
	stdin := s.stdin
	scanner := s.scanner
	s.mu.Unlock()

	payload, err := json.Marshal(inEvent{Type: "user", Message: inMessage{Role: "user", Content: message}})
	if err != nil {
		return "", fmt.Errorf("session: marshal: %w", err)
	}

	if _, err := fmt.Fprintf(stdin, "%s\n", payload); err != nil {
		return "", fmt.Errorf("session: write stdin: %w", err)
	}
	slog.Debug("session: sent message", "chat_id", s.ChatID, "runes", len([]rune(message)))

	// Read JSON event lines until we get a "result" event.
	resultCh := make(chan string, 1)
	errCh := make(chan error, 1)
	approvalFn := s.approvalFn
	go func() {
		const approvalWindowMax = 2048
		var approvalWindow strings.Builder
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			var ev outEvent
			if err := json.Unmarshal([]byte(line), &ev); err != nil {
				// Non-JSON output — accumulate and check for permission prompts.
				approvalWindow.WriteString(line)
				approvalWindow.WriteByte('\n')
				windowStr := approvalWindow.String()
				if len(windowStr) > approvalWindowMax {
					windowStr = windowStr[len(windowStr)-approvalWindowMax:]
					approvalWindow.Reset()
					approvalWindow.WriteString(windowStr)
				}
				cleanText := stripANSISession(windowStr)
				if approvalFn != nil && detectApprovalSession(cleanText) {
					approvalWindow.Reset()
					slog.Info("session: approval prompt detected, forwarding to user")
					allowed, ferr := approvalFn(ctx, cleanText)
					answer := "n\n"
					if ferr == nil && allowed {
						answer = "y\n"
						slog.Info("session: approval granted")
					} else {
						slog.Info("session: approval denied", "err", ferr)
					}
					if _, werr := fmt.Fprint(stdin, answer); werr != nil {
						slog.Warn("session: failed to write approval answer", "error", werr)
					}
				}
				preview := line
				if len(preview) > 120 {
					preview = preview[:120]
				}
				slog.Debug("session: non-JSON line", "line", preview)
				continue
			}
			approvalWindow.Reset()
			slog.Debug("session: event", "type", ev.Type, "subtype", ev.Subtype)
			if ev.Type == "result" {
				if ev.Subtype == "error" || ev.Error != "" {
					errCh <- fmt.Errorf("claude error: %s", ev.Error)
					return
				}
				resultCh <- ev.Result
				return
			}
		}
		if err := scanner.Err(); err != nil {
			errCh <- fmt.Errorf("session: read: %w", err)
		} else {
			errCh <- fmt.Errorf("session: stream closed unexpectedly")
		}
	}()

	select {
	case result := <-resultCh:
		slog.Debug("session: got result", "chat_id", s.ChatID, "runes", len([]rune(result)))
		return result, nil
	case err := <-errCh:
		return "", err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// Close terminates the claude process and marks the session as closed.
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	_ = s.stdin.Close()
	s.cancel()
	slog.Info("interactive session closed", "chat_id", s.ChatID, "msg_count", s.MsgCount)
	return nil
}

// IsIdle returns true if the session has had no activity for IdleTimeout.
func (s *Session) IsIdle() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.closed && time.Since(s.lastMsg) >= IdleTimeout
}

// IsClosed reports whether the session has been closed.
func (s *Session) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

