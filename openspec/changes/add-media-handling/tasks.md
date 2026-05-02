## 1. Downloader 模組 (`internal/downloader/`)
- [x] 1.1 定義 `Downloader` struct（持有 tgbotapi.BotAPI 參考、下載根目錄）
- [x] 1.2 實作 `Download(fileID, fileSize, chatID, filename) (string, error)`
- [x] 1.3 確認下載目錄 `.tgconn/tmp/<chat_id>/` 自動建立
- [x] 1.4 大小限制：`FileSize > 20MB` 回傳 `ErrFileTooLarge`
- [x] 1.5 寫 unit tests（httptest.Server）

## 2. Config 擴充
- [x] 2.1 `Config` 加入 `EnableVoice bool`
- [x] 2.2 `cmd/connect.go` 加入 `--enable-voice` flag 及 viper binding
- [x] 2.3 Voice 啟用時 preflight check `whisper` binary

## 3. Bot 媒體分派邏輯 (`internal/bot/bot.go`)
- [x] 3.1 重構 `handleMessage`：在文字判斷前先偵測媒體類型（switch 分派）
- [x] 3.2 實作 `handleSticker`：回覆含 emoji 的不支援提示
- [x] 3.3 實作 `handlePhoto`：下載最高畫質 → 注入路徑與 caption 到 prompt
- [x] 3.4 實作 `handleDocument`：下載 → 注入路徑、檔名、caption 到 prompt
- [x] 3.5 實作 `handleVoice`：下載 → 呼叫 whisper → 注入轉文字結果
- [x] 3.6 實作 `handleUnsupportedMedia`（Video、Audio、Animation）
- [x] 3.7 Caption 自動帶入 displayQuestion 與 rawPrompt

## 4. Prompt Builder
- [x] 4.1 實作 `buildMediaPrompt(mediaType, localPath, filename, caption string) string`
- [x] 4.2 格式：媒體描述 + 本地路徑 + caption + 任務提示
- [x] 4.3 history context 注入透過 `executeAndReply` → `buildPrompt` 自動套用

## 5. Whisper 整合 (`internal/transcriber/`)
- [x] 5.1 實作 `Transcribe(ctx, audioPath string) (string, error)`：呼叫 whisper，讀取 .txt 輸出
- [x] 5.2 轉錄結果截斷至 4000 chars
- [x] 5.3 實作 `Check() error`（preflight binary check）

## 6. 測試補充
- [x] 6.1 downloader unit tests（httptest mock HTTP server）
- [x] 6.2 `go vet ./...` 與 `go test ./...` 全部通過

## 7. 文件
- [x] 7.1 README 加入媒體支援說明（支援/不支援列表）
- [x] 7.2 README 加入 `--enable-voice` 安裝說明
