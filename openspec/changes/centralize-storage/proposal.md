# Proposal: centralize-storage

## Why

目前 tgconn 把所有狀態（daily logs、per-chat history、cron 任務、下載的媒體）
寫到 CWD 底下的 `.tgconn/`。這個設計有三個問題：

1. **污染專案 git 狀態**：每個 `tgconn` 跑過的 repo 都會出現 `.tgconn/` 在 `git status`，
   寫進 `.gitignore` 又不對勁（這資料跟專案本身無關）。
2. **多專案混亂**：每個 cwd 各自有一份，沒有統一視角能看「我有哪些專案在用 tgconn」。
3. **無自動清理**：`tmp/`、daily logs、history 都是 append-only。長期跑下來會無限增長，
   尤其 `history_<chatID>.jsonl` 每次 prompt 注入都要全檔讀進記憶體切 tail，越肥越慢。

仿照 Claude Code 的做法，把資料集中到 `~/.tgconn/projects/<encoded-cwd>/`，
同時加入啟動時的 retention 清理與手動 `tgconn clean` 子指令。

## What Changes

- **集中化**：所有 per-project 狀態改寫到 `~/.tgconn/projects/<encoded-cwd>/`
  - 編碼規則：`strings.ReplaceAll(cwd, "/", "-")`，與 Claude Code 一致
  - 子目錄結構：`<encoded>/tmp/<chatID>/`、`<encoded>/cron/`、daily logs 與 history 平鋪在根
- **自動遷移**：啟動時若偵測到 `<cwd>/.tgconn/` 存在 → 整個搬到中央位置，並在原處留下
  `.tgconn/MOVED_TO_<新路徑>.txt` 麵包屑檔案
- **新增 `internal/storage/` package**：負責 cwd 編碼、路徑解析、migration（Part 1）、retention 清理（Part 2）
- **依賴注入**：`recorder.New`、`downloader.New`、`cronjob.New` 改為接收 base dir 參數
- **Retention 清理（啟動時執行一次）**：
  - `tmp/`：刪除 mtime 超過 `tmp_retention_hours`（預設 24h）的檔案
  - daily logs（regular + cron）：刪除超過 `log_retention_days`（預設 30 天）的 `*.jsonl`；`0` = 永不刪
  - `history_*.jsonl`：若條目數超過 `history_max_entries`（預設 100）→ 重寫只保留末尾 N 筆；`0` = 不 compact
- **Voice 中間檔**：transcriber 處理完 `.wav` / `.txt` 後立刻 `os.Remove`
- **新增 `tgconn clean` 子指令**：
  - `--tmp` / `--logs` / `--history` / `--all`
  - `--dry-run`：只列出會刪的檔案與大小，不實際刪除
  - `--yes`：跳過互動確認（`--history` 與 `--all` 預設互動確認）

## Impact

- **Affected specs**: `storage-management`（新建）
- **Affected code**:
  - `internal/storage/`（新增；含 `resolver.go`、`migrate.go`，Part 2 再加 `cleanup.go`）
  - `internal/recorder/recorder.go`：移除 const `logDir` / `cronLogDir`，改為 struct field；`New` 接收 baseDir
  - `internal/downloader/downloader.go`：baseDir 由呼叫方注入（已是此設計，只是路徑來源改變）
  - `internal/cronjob/manager.go`：dir 已是參數（不需改）
  - `internal/bot/bot.go`：`New` 接收 `projectDir`，改傳給 downloader / cronjob
  - `internal/transcriber/transcriber.go`：voice intermediate 檔處理完立刻刪
  - `internal/config/config.go`：新增 `TmpRetentionHours` / `LogRetentionDays` / `HistoryMaxEntries`
  - `cmd/connect.go`：啟動時呼叫 migration + cleanup
  - `cmd/clean.go`（新增）：實作 `tgconn clean` 子指令

## Out of Scope

- 跨 host 同步、共享 `~/.tgconn/`（單機行為）
- 把 cron 任務定義也納入 retention 清理（user 用 `/cron del` 顯式管理）
- `~/.tgconn/config.yaml` 本身的搬移（位置已在 home，不變）
- Windows 支援（既有限制不變）
