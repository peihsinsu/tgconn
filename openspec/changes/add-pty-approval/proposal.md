# Proposal: add-pty-approval

## Why

`ask` 模式的設計目標是：Claude 遇到授權提示時，透過 Telegram inline keyboard 讓使用者
批准或拒絕，再讓 Claude 繼續執行。但 Claude 的授權 UI 是一個 TUI（terminal user interface），
寫進真實的 TTY，不會出現在 stdout/stderr pipe 裡，導致目前的 `approvalMonitor`（stderr 監聽）
永遠偵測不到提示，subprocess 卡死。

解法：用 PTY（pseudo-terminal）讓 Claude 以為自己在真實終端中執行，tgconn 從 master
PTY 讀取所有輸出（含 TUI 授權提示），偵測模式後轉發到 Telegram，再把 y/n 寫回 master。

## What Changes

- 加入依賴 `github.com/creack/pty`
- 新增 `internal/provider/pty_runner.go`：PTY-based subprocess runner
- `ask` 模式改呼叫 `runClaudeWithPTY`（帶 ApprovalFunc）
- 重新啟用 `bot.go` 的 `makeApprovalFunc` / `handleCallbackQuery` 路徑

## Impact

- Affected specs: `telegram-connector` (MODIFIED)
- Affected code: `internal/provider/`, `internal/bot/bot.go`, `go.mod`

## Design

### 依賴

```
github.com/creack/pty v1.1.x
```

支援 macOS 與 Linux，無 CGo，輕量。

### PTY runner (`internal/provider/pty_runner.go`)

```
runClaudeWithPTY(ctx, question, fn ApprovalFunc) (string, error)
```

執行流程：

```
1. exec.Command("claude", "-p", question)   // no --dangerously-skip-permissions
2. pty.Start(cmd)                           // 建立 PTY pair，slave 接到 cmd 的 stdin/stdout/stderr
3. goroutine: 從 master 持續讀取輸出
   - 累積 content buffer（Claude 的真實回答）
   - 累積 prompt buffer（近期輸出，用於 pattern match）
4. 偵測到授權模式 → 呼叫 ApprovalFunc(ctx, cleanedPrompt)
   - true  → 寫 "y\n" 到 master
   - false → 寫 "n\n" 到 master
   - 清空 prompt buffer，繼續讀
5. cmd.Wait() 完成 → 回傳累積的 content buffer
```

### 輸出分離策略

Claude 的 TUI 授權提示會包含 ANSI escape codes（顏色、方框字元）。兩個 buffer 分開處理：

- **content buffer**：去除 ANSI 後、不含授權提示行的輸出，作為最終回傳給使用者的文字
- **prompt window**：固定大小的滑動視窗（最近 2048 bytes），去除 ANSI 後套用 pattern

授權提示偵測結束後，prompt window 清空，不混入 content buffer。

### Pattern 更新

沿用現有 `approvalPatterns`，新增 Claude TUI 常見的格式：

```
● Do you want to proceed? [y/n]
│ Allow tool use? (y/n):
```

（可依實測結果補充）

### Mode routing（`internal/bot/bot.go`）

```
auto → provider.WithExecMode(ctx, "auto")   // --dangerously-skip-permissions
ask  → provider.WithApproval(ctx, makeApprovalFunc(chatID))  // PTY + Telegram keyboard
safe → provider.WithExecMode(ctx, "safe")   // --permission-mode plan
```

### 已有基礎（不需重做）

- `approval.go`：`ApprovalFunc` 型別、`WithApproval` / `ApprovalFromContext`
- `bot.go`：`makeApprovalFunc`、`handleCallbackQuery`、`pendingApprovals`（已實作，待重啟用）
- `approvalPatterns`、`stripANSI`（保留在 claude.go 或移至 shared util）

## 已知限制與風險

| 項目 | 說明 |
|------|------|
| PTY terminal size | 預設 80×24，Claude TUI 可能截斷長訊息。可用 `pty.Setsize` 設定較大尺寸（如 220×50） |
| Claude TUI 版本更新 | 若 Claude 改版授權提示格式，pattern 需同步更新 |
| Windows | PTY 在 Windows 上不支援，本專案 target macOS/Linux，不影響 |
| 並發授權 | 多個 job 同時等待授權：各自獨立的 ApprovalFunc channel，互不干擾（已有設計） |

## Files Changed

| File | Change |
|------|--------|
| `go.mod` / `go.sum` | 加入 `github.com/creack/pty` |
| `internal/provider/pty_runner.go` | 新增 PTY runner |
| `internal/provider/claude.go` | `ask` 模式改呼叫 `runClaudeWithPTY` |
| `internal/bot/bot.go` | `ask` 模式改回注入 `makeApprovalFunc` |

## Out of Scope

- `safe` 模式使用 PTY（`--permission-mode plan` 已足夠）
- Windows 支援
- PTY terminal size 動態調整（暫用固定大值）
