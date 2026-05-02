## Context

初版 tgconn：單一 Go binary，long-polling Telegram，以 `os/exec` 呼叫 LLM CLI，無持久狀態。

## Goals / Non-Goals

- Goals:
  - Telegram bot 可接收訊息並轉發給 LLM provider
  - 支援 Claude 作為第一個 provider（extensible 到 Codex/Gemini）
  - CLI 可透過 flag / env var / config 檔設定
  - Chat ID 白名單安全管控
- Non-Goals:
  - Webhook 模式（MVP 用 long-polling）
  - 對話歷史 / session 管理（每則訊息獨立呼叫）
  - Web UI
  - Docker image（用戶自行打包）

## Decisions

- **CLI framework**: `cobra` — Go 生態系標準，支援 subcommand 與 flag 繼承
- **Config**: `viper` — 支援 file / env / flag 三層優先，減少樣板
- **Telegram SDK**: `go-telegram-bot-api/v5` — 成熟、API 完整、long-polling 內建
- **Provider execution**: `os/exec.CommandContext` — 直接呼叫系統 CLI，繼承 CWD；context 支援 timeout/cancel
- **Provider interface**: 單一 `Execute(ctx, question) (string, error)` — 讓不同 provider 可互換，未來加新 provider 只需新增 struct
- **Message truncation**: Telegram 單則訊息上限 4096 chars；超過時分批發送（最多 N 則）

## Risks / Trade-offs

- LLM CLI 必須在 `$PATH`，環境相依性高 → 在 `connect` 啟動時做 preflight check，provider binary 不存在就早失敗
- `--dangerously-skip-permissions` 讓 Claude 可執行任意指令 → 必須搭配 `--allow-chat` 白名單，README 需明確警示
- Long-polling 在網路不穩時會 retry → 使用 bot-api SDK 內建 retry，並記錄 error log 但不 crash

## Migration Plan

N/A（全新專案）

## Open Questions

- Codex / Gemini 的 CLI 呼叫格式？（待確認後在各自 provider 實作中補充）
- 是否需要 per-message timeout（防止 LLM 執行太久佔用）？建議預設 120s，可透過 `--timeout` 覆寫
