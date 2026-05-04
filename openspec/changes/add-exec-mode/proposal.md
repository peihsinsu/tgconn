# Proposal: add-exec-mode

## Why

tgconn 目前移除了 `--dangerously-skip-permissions`，改以 Telegram inline keyboard
詢問使用者授權。但這個行為應該是可以設定的：有些場景需要完全自動（`auto`），
有些需要互動授權（`ask`），有些只需要讀取分析而不允許任何修改（`safe`）。
本提案加入 `--exec-mode` flag，讓使用者明確選擇執行模式。

## What Changes

- 新增 `exec-mode` capability spec
- `Config` 加入 `ExecMode` 欄位與常數，`Validate()` 加入合法值檢查
- `connect` 指令加入 `--exec-mode` flag（預設 `ask`）
- `ClaudeProvider.Execute` 依有無 ApprovalFunc 決定是否加 `--dangerously-skip-permissions`
- `bot.go` 依模式注入對應的 ApprovalFunc（Telegram keyboard / 自動決策 / 不注入）

## Impact

- Affected specs: `exec-mode` (新建)
- Affected code: `internal/config/`, `cmd/connect.go`, `internal/provider/claude.go`, `internal/bot/bot.go`

## Summary

Add a `--exec-mode` flag to the `connect` command that controls how Claude handles
permission prompts during execution. Three modes are provided:

| Mode | Behaviour |
|------|-----------|
| `auto` | Passes `--dangerously-skip-permissions` to Claude; zero interruptions, full trust |
| `ask` *(default)* | Every Claude permission prompt is forwarded to the Telegram user as an inline-keyboard message (✅ 允許 / ❌ 拒絕); execution waits for the response |
| `safe` | Read-only operations (Read / Glob / Grep / LS) are auto-allowed silently; Bash execution and file mutations (Write / Edit / Delete) are auto-denied silently; no Telegram interruptions |

> **Codex note**: Codex does not expose a comparable TTY-based approval mechanism.
> When `ask` or `safe` mode is configured, `CodexProvider.Execute` returns a
> user-visible Telegram message explaining the limitation and suggesting alternatives.
> Only `auto` (or unset) mode proceeds to invoke `codex --dangerously-bypass-approvals-and-sandbox`.

## Motivation

The previous implementation hard-coded `--dangerously-skip-permissions`.  After
removing that flag (to restore Claude's native safety judgment), users need a
first-class way to choose their preferred trust level:

- **auto**: CI-like environments or fully trusted personal machines where the user
  wants maximum productivity and no Telegram round-trips.
- **ask**: Interactive usage where the user wants oversight of every sensitive
  operation but still wants Claude to run freely for safe actions.
- **safe**: Read/analyse scenarios (code review, Q&A about a codebase) where file
  mutation and shell execution should be prevented entirely.

## Design

### CLI flag

```
--exec-mode auto|ask|safe   (default: ask)
```

Viper key: `exec_mode`.

### Config

```go
const (
    ExecModeAuto = "auto"
    ExecModeAsk  = "ask"
    ExecModeSafe = "safe"
)

type Config struct {
    ...
    ExecMode string  // "auto" | "ask" | "safe"
}
```

Validation: unknown value → startup error.

### Provider execution (Claude)

| ExecMode | ApprovalFunc injected? | Claude args |
|----------|------------------------|-------------|
| `auto`   | no                     | `-p <q> --dangerously-skip-permissions` |
| `ask`    | yes — Telegram keyboard | `-p <q>` |
| `safe`   | yes — auto-decision    | `-p <q>` |

`ClaudeProvider.Execute`:
- If `ApprovalFromContext` returns non-nil → `runClaudeWithApproval`
- Otherwise → `runSubprocess` with `--dangerously-skip-permissions`

### Safe-mode approval function (`makeSafeApprovalFunc`)

Implemented in `internal/bot/bot.go`:

```
pattern in prompt → action
─────────────────────────────────────────
Read / Glob / Grep / LS / list / view     → allow (log at Debug)
Bash / shell / execute / run              → deny  (log at Info)
Write / Edit / Create / Delete / Remove   → deny  (log at Info)
anything else                             → deny  (safe default, log at Warn)
```

No Telegram messages are sent in safe mode; decisions are fully automated.

### Status broadcast at startup

When the bot starts, it broadcasts the active exec-mode alongside the provider name:

```
✅ 已連線，provider: claude  模式: ask
```

## Files Changed

| File | Change |
|------|--------|
| `internal/config/config.go` | Add `ExecMode` field + constants + validation |
| `cmd/connect.go` | Add `--exec-mode` flag + viper binding |
| `internal/provider/claude.go` | Restore `--dangerously-skip-permissions` fallback when no ApprovalFunc |
| `internal/bot/bot.go` | Mode-aware ApprovalFunc injection; `makeSafeApprovalFunc`; broadcast update |

## Out of Scope

- Codex/Gemini mode differentiation
- Per-tool granular allowlist (use `.claude/settings.local.json` instead)
- Persisting exec-mode in `~/.tgconn/config.yaml` via `tgconn config init`
