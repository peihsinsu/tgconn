## 1. Session Package (`internal/session/session.go`)
- [x] 1.1 定義 `Session` struct（cmd、stdin writer、stdout scanner、ChatID、StartedAt、MsgCount、lastMsg、cancel）
- [x] 1.2 實作 `New(chatID int64) *Session`
- [x] 1.3 實作 `Start(ctx context.Context, execMode string) error`
- [x] 1.4 實作 `Send(ctx context.Context, message string) (string, error)`
- [x] 1.5 實作 `Close() error`
- [x] 1.6 `Session` 記錄 `lastMsg` 時間，供 idle timeout 判斷

## 2. Bot 整合 (`internal/bot/bot.go`)
- [x] 2.1 `Bot` struct 加入 `sessionsMu` 與 `sessions map[int64]*session.Session`
- [x] 2.2 `New()` 初始化 sessions map
- [x] 2.3 訊息路由：`handleMessage` 前段判斷 session → `handleSessionMessage`
- [x] 2.4 實作 `handleSessionMessage`
- [x] 2.5 實作 `/session` 指令分派
- [x] 2.6 實作 `cmdSessionStart`
- [x] 2.7 實作 `cmdSessionEnd`
- [x] 2.8 實作 `cmdSessionStatus`
- [x] 2.9 `/session` 指令攔截在 `handleMessage` 最前段

## 3. Idle Timeout
- [x] 3.1 `sessionIdleSweep` goroutine，每分鐘掃描
- [x] 3.2 idle 超過 30 分鐘 → 自動 Close，通知 chatID

## 4. 測試與驗證
- [x] 4.1 `go build ./...` 通過
- [x] 4.2 `go test ./...` 全部通過
- [ ] 4.3 實機測試：`/session start` → 送問題 → 收到回答 → `/session end`
- [ ] 4.4 驗證無 session 時 one-shot 行為不受影響
- [ ] 4.5 驗證 idle timeout 自動關閉並通知
