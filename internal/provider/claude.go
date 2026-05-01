package provider

import "context"

type ClaudeProvider struct{}

func (c *ClaudeProvider) Check() error { return checkBinary("claude") }

func (c *ClaudeProvider) Execute(ctx context.Context, question string) (string, error) {
	return runSubprocess(ctx, "claude", []string{"-p", question, "--dangerously-skip-permissions"})
}
