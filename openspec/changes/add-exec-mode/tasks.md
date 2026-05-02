## 1. Config 擴充 (`internal/config/config.go`)
- [x] 1.1 新增 `ExecModeAuto`, `ExecModeAsk`, `ExecModeSafe` 常數
- [x] 1.2 `Config` struct 加入 `ExecMode string`
- [x] 1.3 `Load()` 讀取 `exec_mode`，預設值為 `"ask"`
- [x] 1.4 `Validate()` 加入 exec-mode 合法值檢查，未知值回傳錯誤

## 2. CLI flag (`cmd/connect.go`)
- [x] 2.1 加入 `--exec-mode` flag（預設 `"ask"`）
- [x] 2.2 加入 viper binding `exec_mode`
- [x] 2.3 啟動時 log 目前 exec-mode

## 3. Provider 調整 (`internal/provider/claude.go`)
- [x] 3.1 `Execute`：無 ApprovalFunc 時恢復使用 `--dangerously-skip-permissions`（auto 模式路徑）
- [x] 3.2 `Execute`：有 ApprovalFunc 時走 `runClaudeWithApproval`（ask / safe 模式路徑，已實作）

## 4. Bot 模式分派 (`internal/bot/bot.go`)
- [x] 4.1 `executeAndReply` 改為依 `b.config.ExecMode` 決定是否注入 ApprovalFunc
- [x] 4.2 實作 `makeSafeApprovalFunc(chatID)`：純本地自動決策，不傳送 Telegram 訊息
  - Read / Glob / Grep / LS → allow
  - Bash / Write / Edit / Delete → deny
  - 其他 → deny（safe default）
- [x] 4.3 `auto` 模式：不注入 ApprovalFunc（provider 自動走 `--dangerously-skip-permissions`）
- [x] 4.4 啟動廣播訊息加入 exec-mode 顯示

## 5. 測試補充
- [x] 5.1 config validation 測試：合法 / 非法 exec-mode
- [x] 5.2 `go vet ./...` 與 `go test ./...` 全部通過
