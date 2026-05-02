package provider

import (
	"context"
	"errors"
	"log/slog"
)

// execModeKey is the context key for the exec mode string.
type execModeKey struct{}

// WithExecMode stores the exec mode in ctx for use by ClaudeProvider.Execute.
func WithExecMode(ctx context.Context, mode string) context.Context {
	return context.WithValue(ctx, execModeKey{}, mode)
}

func execModeFromContext(ctx context.Context) string {
	mode, _ := ctx.Value(execModeKey{}).(string)
	return mode
}

type ClaudeProvider struct{}

func (c *ClaudeProvider) Check() error { return checkBinary("claude") }

// Execute runs claude with flags appropriate for the exec mode stored in ctx.
//
//   auto (or unset): --dangerously-skip-permissions — zero interruptions
//   ask + ApprovalFunc: PTY runner — Telegram inline keyboard for each permission prompt
//   safe:            --permission-mode plan — read/analyse only, no execution
func (c *ClaudeProvider) Execute(ctx context.Context, question string) (string, error) {
	switch execModeFromContext(ctx) {
	case "safe":
		return runSubprocess(ctx, "claude", []string{"-p", question, "--permission-mode", "plan"})
	case "ask":
		if fn := ApprovalFromContext(ctx); fn != nil {
			result, err := runClaudeWithPTY(ctx, question, fn)
			if errors.Is(err, ErrPTYUnavailable) {
				slog.Warn("PTY unavailable on this system, falling back to auto mode", "error", err)
				return runSubprocess(ctx, "claude", []string{"-p", question, "--dangerously-skip-permissions"})
			}
			return result, err
		}
		// No ApprovalFunc set — fall through to auto.
		return runSubprocess(ctx, "claude", []string{"-p", question, "--dangerously-skip-permissions"})
	default: // "auto" or unset
		return runSubprocess(ctx, "claude", []string{"-p", question, "--dangerously-skip-permissions"})
	}
}
