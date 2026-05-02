## 1. 依賴
- [x] 1.1 `go get github.com/creack/pty`

## 2. PTY Runner (`internal/provider/pty_runner.go`)
- [x] 2.1 實作 `runClaudeWithPTY(ctx, question, fn ApprovalFunc) (string, error)`
- [x] 2.2 用 `pty.Start(cmd)` 建立 PTY，設定 terminal size（220×50）
- [x] 2.3 goroutine 從 master PTY 持續讀取，維護 content buffer 與 prompt window
- [x] 2.4 pattern match 偵測授權提示 → 呼叫 fn → 寫 y/n 到 master
- [x] 2.5 `cmd.Wait()` 完成後回傳去除 ANSI 的 content buffer
- [x] 2.6 context 取消/超時時 killGroup + 清理 PTY fd

## 3. Provider routing (`internal/provider/claude.go`)
- [x] 3.1 `ask` 模式：若有 ApprovalFunc → 呼叫 `runClaudeWithPTY`

## 4. Bot routing (`internal/bot/bot.go`)
- [x] 4.1 `ask` 模式：改回注入 `makeApprovalFunc(msg.Chat.ID)`（重啟用已有程式碼）

## 5. Pattern 補充與整理
- [x] 5.1 將 `approvalPatterns` 與 `stripANSI` 定義在 `pty_runner.go`
- [x] 5.2 補充 Claude TUI 常見格式的 pattern（allow?/proceed?/press enter to allow）

## 6. 測試
- [x] 6.1 `go build ./...` 通過
- [ ] 6.2 實機測試：`ask` 模式下傳送需要授權的指令，確認 Telegram keyboard 出現
- [ ] 6.3 確認授權後 Claude 繼續執行並回傳結果
- [ ] 6.4 確認拒絕授權後 Claude 報告無法執行
