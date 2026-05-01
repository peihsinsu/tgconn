package provider

import "context"

type CodexProvider struct{}

func (c *CodexProvider) Check() error { return checkBinary("codex") }

func (c *CodexProvider) Execute(ctx context.Context, question string) (string, error) {
	return runSubprocess(ctx, "codex", []string{"--dangerously-bypass-approvals-and-sandbox", question})
}
