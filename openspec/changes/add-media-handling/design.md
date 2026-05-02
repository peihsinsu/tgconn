## Context

Telegram 訊息除了純文字外，有六種主要媒體類型：Sticker、Photo、Document、Voice、Video、Audio。tgconn 的 LLM provider 是外部 CLI，不直接支援接收 binary 資料，因此所有媒體都需要「先落地成本地檔案，再注入路徑到 prompt」的模式。

## Goals / Non-Goals

- Goals:
  - Caption 文字零成本取用（不需下載）
  - 檔案/圖片下載後 LLM 可透過本地路徑讀取
  - 貼圖、Video、Audio 有明確的不支援提示
  - Voice 轉文字透過 `--enable-voice` 選擇性啟用
- Non-Goals:
  - 直接傳 binary 給 LLM（CLI 不支援）
  - 影片內容理解（檔案太大，意義不明確）
  - 雲端儲存或永久保留下載檔案

## Decisions

- **下載目錄**：`.tgconn/tmp/<chat_id>/` — 按 chat 隔離，避免不同 chat 的同名檔案衝突
- **檔名**：保留原始 `FileName`；若無（Photo 沒有 FileName）則用 `<message_id>.jpg`
- **大小限制**：Telegram bot API 最大 20MB，超過的檔案 API 不會提供下載 URL，直接告知用戶
- **Prompt 注入格式**：
  ```
  使用者傳送了一個檔案：report.csv
  已下載至：.tgconn/tmp/310154327/report.csv
  Caption：請幫我分析這份報告

  請根據上述檔案完成任務。
  ```
- **Voice 轉文字**：呼叫 `whisper <file> --output_format txt`，讀取 stdout，注入原始問題位置；`whisper` binary 不在 PATH 時啟動報錯
- **tmp 清理**：不自動清理（保留供 LLM 後續參考）；提供說明讓使用者自行管理

## Risks / Trade-offs

- 下載失敗（網路問題）→ 回覆錯誤訊息給 chat，不轉交 LLM
- 惡意檔案（`.exe`、`.sh`）下載後 LLM 可能被要求執行 → 不在 tgconn 層過濾（已有 allow-chat 白名單，使用者自己信任自己）
- whisper 輸出可能很長 → 截斷至 4000 chars 再送 LLM

## Open Questions

- 是否需要 P1 圖片 caption-only 模式（不下載）？暫定：有 caption 直接用 caption；無 caption 才下載
