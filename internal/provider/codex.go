package provider

import "context"

type CodexProvider struct{}

func (c *CodexProvider) Check() error { return checkBinary("codex") }

func (c *CodexProvider) Execute(ctx context.Context, question string) (string, error) {
	switch execModeFromContext(ctx) {
	case "ask":
		return "⚠️ Codex 不支援 ask 模式（互動式授權需要 Claude provider）。請切換至 auto 模式，或改用 claude provider。", nil
	case "safe":
		return "⚠️ Codex 不支援 safe 模式（唯讀分析模式）。請切換至 auto 模式，或改用 claude provider。", nil
	default: // "auto" or unset
		return runSubprocess(ctx, "codex", []string{"--dangerously-bypass-approvals-and-sandbox", question})
	}
}
