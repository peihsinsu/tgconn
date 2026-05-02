## Why

目前 tgconn 只處理純文字訊息，用戶傳送貼圖、圖片、檔案或語音時會被靜默丟棄，沒有任何回應。加入媒體訊息處理可大幅提升實用性，尤其是「傳檔案給 claude 分析」的使用情境。

## What Changes

- **P0 — Caption 擷取**：圖片或檔案附帶的說明文字（Caption）直接當作問題送 LLM
- **P0 — 貼圖友善提示**：收到貼圖時回覆不支援提示，附上 emoji 資訊
- **P1 — 檔案下載**：Document（任意檔案）下載至 `.tgconn/tmp/`，注入檔案路徑給 LLM
- **P1 — 圖片下載**：Photo 下載至 `.tgconn/tmp/`，注入路徑給 LLM，LLM 可透過 Read/Bash tool 處理
- **P2 — 語音轉文字**：Voice 訊息下載後呼叫 `whisper` CLI 轉成文字，再送 LLM（需 `--enable-voice` flag）

## Impact

- Affected specs: `telegram-connector`（ADDED requirements）
- Affected code:
  - `internal/bot/bot.go`：handleMessage 擴充媒體分派邏輯
  - `internal/downloader/`（新）：Telegram 檔案下載模組
  - `internal/bot/bot.go`：`cmdHistory`、`cmdStatus` 不受影響
  - `cmd/connect.go`：新增 `--enable-voice` flag
