## 1. Project Scaffolding
- [x] 1.1 `go mod init github.com/cx009/tgconn`
- [x] 1.2 建立 package 結構：`cmd/`, `internal/provider/`, `internal/bot/`, `internal/config/`
- [x] 1.3 加入依賴：`cobra`, `viper`, `go-telegram-bot-api/v5`
- [x] 1.4 建立 `main.go` 作為 cobra root command 入口

## 2. Config Layer (`internal/config/`)
- [x] 2.1 定義 `Config` struct（token, provider, allowedChats, timeout, debug）
- [x] 2.2 實作 `Load()` — viper 整合 flag / env / `~/.tgconn/config.yaml` 三層優先
- [x] 2.3 實作 `Validate()` — 確認 token 非空、allowedChats 非空、provider 有效
- [x] 2.4 寫 unit tests

## 3. Provider Abstraction (`internal/provider/`)
- [x] 3.1 定義 `Provider` interface：`Execute(ctx context.Context, question string) (string, error)`
- [x] 3.2 實作 `ClaudeProvider` — 呼叫 `claude -p "${question}" --dangerously-skip-permissions`，CWD 繼承自 process
- [x] 3.3 實作 `New(name string) (Provider, error)` factory（支援 claude；codex/gemini 回傳 ErrNotImplemented）
- [x] 3.4 Preflight check：`provider.Check()` 確認 binary 存在於 `$PATH`
- [x] 3.5 寫 unit tests（mock exec.Cmd）

## 4. Telegram Bot Layer (`internal/bot/`)
- [x] 4.1 實作 `Bot` struct（包含 tgbotapi client、provider、config）
- [x] 4.2 實作 `Start(ctx)` — 啟動 long-polling loop
- [x] 4.3 實作訊息 dispatch：chat ID 白名單過濾 → 呼叫 provider → 送回回應
- [x] 4.4 實作 response splitting（超過 4096 chars 分批送）
- [x] 4.5 實作 graceful shutdown（context cancel → 等待 in-flight call）
- [x] 4.6 寫 unit tests

## 5. CLI Commands (`cmd/`)
- [x] 5.1 Root command：`--provider`、`--debug` persistent flags；從 viper 載入 config
- [x] 5.2 `connect` subcommand：`--allow-chat`（repeatable）、`--timeout`；執行 preflight checks 後啟動 bot
- [x] 5.3 `config init` subcommand：互動式設定 token / provider / allowed chats，寫入 `~/.tgconn/config.yaml`
- [x] 5.4 `config show` subcommand：印出 resolved config（token masked）
- [x] 5.5 `version` subcommand：印出版本號（build-time ldflags 注入）

## 6. Error Handling & Logging
- [x] 6.1 統一 error message 格式（lowercase，no trailing punctuation）
- [x] 6.2 Debug mode：所有收發訊息、subprocess command、回應前 200 chars 輸出到 stderr
- [x] 6.3 Provider 執行失敗時，將 user-friendly 錯誤訊息送回 Telegram chat

## 7. Build & Documentation
- [x] 7.1 `Makefile`：`build`, `test`, `lint` targets
- [x] 7.2 `README.md`：安裝、取得 bot token、啟動範例、安全警示
- [x] 7.3 確認 `go vet ./...` 與 `go test ./...` 全部通過
