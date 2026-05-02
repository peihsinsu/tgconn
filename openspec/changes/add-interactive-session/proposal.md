# Proposal: add-interactive-session

## Why

目前 tgconn 是 **一問一答（one-shot）** 模式：每條訊息都啟動一個新的 `claude -p`
subprocess，跑完就結束。對話記憶靠 tgconn 手動注入 history，Claude 本身沒有狀態。

這個模式有幾個限制：
- Claude 無法在回答途中提問（因為跑到底就結束了）
- 長對話的 history 注入會讓 prompt 越來越肥
- 無法利用 Claude 原生的對話管理（工具呼叫的上下文連結等）

本提案新增**互動 session 模式**：在同一個 chat 開啟一個持久 Claude process，
每條 Telegram 訊息直接送進 Claude 的 stdin，Claude 的輸出串流回 Telegram。

## What Changes

- 新增 `internal/session/` package：Session 型別與生命週期管理
- `Bot` 持有 per-chat session map
- 新增 Telegram 指令：`/session start`、`/session end`
- 訊息路由邏輯：有活躍 session → 送進 session；否則走原本 one-shot 路徑
- 串流格式：`--input-format stream-json` + `--output-format stream-json`

## Impact

- Affected specs: `telegram-connector` (MODIFIED)
- Affected code: `internal/session/` (新增), `internal/bot/bot.go`, `internal/provider/claude.go`

## Design

### 串流協定

Claude 的 stream-json 模式：

**stdin（每行一個 JSON）：**
```json
{"type": "user", "message": "幫我列出所有 Go 檔案"}
```

**stdout（每行一個 JSON event）：**
```json
{"type": "assistant", "message": "..."}
{"type": "tool_use",  "tool": "Bash", "input": {"command": "find . -name '*.go'"}}
{"type": "tool_result", "content": "..."}
{"type": "result", "subtype": "success", "result": "最終回答文字"}
```

tgconn 解析 `type == "result"` 作為「Claude 說完了」的訊號，把 `result` 欄位傳回 Telegram。

### Session 生命週期

```
/session start
  → 啟動 claude --output-format stream-json --input-format stream-json
  → 記錄到 Bot.sessions[chatID]

Telegram 訊息（有活躍 session）
  → 寫 {"type":"user","message":"..."} 到 Claude stdin
  → 讀 stdout 直到 {"type":"result"} → 送回 Telegram

/session end（或 idle timeout）
  → 關閉 stdin → 等待 process 結束
  → 從 sessions map 移除
```

### Idle Timeout

Session 閒置超過 30 分鐘（可設定）自動結束，並通知使用者。

### 指令

| 指令 | 說明 |
|------|------|
| `/session start` | 開啟互動 session（若已有則提示） |
| `/session end` | 結束目前 session |
| `/session status` | 顯示 session 狀態（運行時間、訊息數）|

### Exec-Mode 整合

Session 啟動時繼承目前的 exec-mode 設定：
- `auto` → `--dangerously-skip-permissions`
- `ask` → 未來可整合 PTY approval（`add-pty-approval` 完成後）
- `safe` → `--permission-mode plan`

### 向下相容

Session 功能**完全 opt-in**：
- 沒有開 `/session start` → 行為與目前完全一致（one-shot）
- 現有的 `/list`、`/stop`、`/history` 等指令不受影響

### Bot struct 新增欄位

```go
sessionsMu sync.Mutex
sessions   map[int64]*session.Session  // key: chatID
```

### Session struct（`internal/session/session.go`）

```go
type Session struct {
    ChatID    int64
    StartedAt time.Time
    MsgCount  int
    cmd       *exec.Cmd
    stdin     io.WriteCloser   // JSON lines 寫入
    stdout    *bufio.Scanner   // JSON lines 讀取
    lastMsg   time.Time        // 用於 idle timeout
    cancel    context.CancelFunc
}
```

主要方法：
- `Start(ctx, execMode) error`
- `Send(ctx, message string) (string, error)`
- `Close() error`

## Files Changed

| File | Change |
|------|--------|
| `internal/session/session.go` | 新增 Session 型別與 Start/Send/Close |
| `internal/bot/bot.go` | 加入 sessions map、訊息路由、`/session` 指令 |

## Out of Scope

- Session 跨重啟持久化（重啟後 session 消失，需重開）
- 多個同時活躍的 session per chat
- Session 內的 `/list`、`/stop` 整合（one-shot job 與 session 分開管理）
- `ask` 模式的 Telegram 授權（依賴 `add-pty-approval` 完成）
