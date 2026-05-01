package provider

import (
	"context"
	"fmt"
	"os/exec"
)

// Provider executes an LLM prompt and returns the response text.
type Provider interface {
	Execute(ctx context.Context, question string) (string, error)
	// Check verifies the provider binary is available on $PATH.
	Check() error
}

func New(name string) (Provider, error) {
	switch name {
	case "claude":
		return &ClaudeProvider{}, nil
	case "codex":
		return &CodexProvider{}, nil
	case "gemini":
		return nil, fmt.Errorf("gemini provider not yet implemented")
	default:
		return nil, fmt.Errorf("unsupported provider %q; supported: claude, codex, gemini", name)
	}
}

func checkBinary(name string) error {
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("provider binary %q not found in $PATH", name)
	}
	return nil
}
