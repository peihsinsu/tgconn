# Design: centralize-storage

## Context

tgconn 目前的儲存策略是「跟著 CWD 走」：所有狀態都寫到 `<cwd>/.tgconn/`。
這對單一專案測試夠用，但長期使用後浮現問題：
- 每個 repo 都被污染 `git status`
- 多個專案沒有統一視角
- 無清理機制，磁碟用量單調遞增
- `history_<chatID>.jsonl` 用全檔讀進記憶體切 tail，越肥越慢

Claude Code 的解法是把 per-project 狀態集中到 `~/.claude/projects/<encoded-cwd>/`，
這個模式經過驗證、使用者已熟悉。本提案套用相同模式。

## Goals / Non-Goals

**Goals**
- 所有 per-project 狀態統一放在 `~/.tgconn/projects/<encoded-cwd>/`
- 既有 `<cwd>/.tgconn/` 自動無痛遷移（一次性，留麵包屑）
- 啟動時自動 retention 清理（tmp + daily logs + history compaction）
- 手動 `tgconn clean` 提供 dry-run 與互動確認，避免誤刪

**Non-Goals**
- 改變 cron job 定義的儲存格式或位置（仍是 `<base>/cron/<jobID>.json`）
- 跨機器共享 `~/.tgconn/`
- 改變 `~/.tgconn/config.yaml` 的位置

## Decisions

### Decision 1: Path encoding 採 Claude 風格

採 `strings.ReplaceAll(cwd, "/", "-")`，例如：
- `/Users/charles/work/charles/tgconn` → `-Users-charles-work-charles-tgconn`

**Alternatives**
- URL-encode：完整但難讀
- Base64：完整但完全不可讀
- Hash（SHA-1 前 N 碼）：固定長度但破壞「一眼看出是哪個專案」

**Why**：使用者已熟悉這個格式（Claude Code 同款），可讀性夠，碰撞風險可忽略
（同一 host 上不會有兩個不同路徑編碼出同一字串）。

### Decision 2: Auto migration with breadcrumb

啟動時若 `<cwd>/.tgconn/` 存在且 `<centralized>` 不存在：
1. 確認 `<centralized>` 父目錄存在
2. `os.Rename(<cwd>/.tgconn, <centralized>)`（atomic on same fs）
3. 在 `<cwd>/.tgconn/` 重建空目錄（用 `os.MkdirAll`）並寫入
   `MOVED_TO_<encoded>.txt`，內容含目標絕對路徑與時間戳

**Edge cases**
- 兩處都存在 → log 警告、不動，由使用者決定（避免覆蓋）
- 跨檔案系統 → `os.Rename` 失敗 → fallback 用 copy + remove，最後驗證才刪原處
- Migration 失敗 → 報錯並中止啟動（避免一半遷移一半沒遷移）

### Decision 3: Retention 在啟動時跑一次，非背景常駐

**Alternatives**
- 背景 goroutine 每 N 小時掃一次
- 寫入時 lazy 清理

**Why**：tgconn 不是 daemon、平均 uptime 不長；啟動時跑一次最簡單，對使用者直觀
（重啟即清理）。`history` compaction 同理在啟動時做。

### Decision 4: `history_*.jsonl` 用 compaction 而非 ring buffer

啟動時若條目數 > `HistoryMaxEntries`（預設 100），讀取末尾 N 筆，重寫整個檔案。

**Alternatives**
- 寫入時檢查條目數：每次寫都要 scan，成本累加
- Ring buffer 格式：要重新設計檔案結構，破壞 jsonl 簡單性

**Why**：tgconn 啟動頻率低、每次 compaction 成本可接受；保持 jsonl append-only 寫入路徑
不變，邏輯最簡單。預設 100 是 `HistorySize`（預設 10）的 10 倍，buffer 足夠且總檔案
大小可控（粗估每筆 5KB → 500KB 上限）。

### Decision 5: Voice 中間檔即時刪除，不走 retention

transcriber 產出 `.wav`（whisper.cpp 需要）與 `.txt`（whisper 輸出）後立刻 `os.Remove`。
原始 `.ogg` 仍走 `tmp/` retention（24h），因為呼叫端可能還要存取。

**Why**：中間檔只在轉錄過程中有意義，沒有 audit 價值；保留只是浪費空間。

### Decision 6: `tgconn clean` 預設互動式，flag 解鎖自動化

| flag 組合 | 行為 |
|----------|------|
| `--tmp` / `--logs` | 直接刪（已有 retention 過濾，門檻夠高）|
| `--history` / `--all` | 列出影響範圍 → 等使用者輸入 `yes` 確認 |
| `--dry-run` | 永遠不刪，只列出 |
| `--yes` | 跳過確認，CI / 腳本用 |

**Why**：危險操作（清光 history、全清）要二次確認，符合 unix 慣例；想自動化的人加 `--yes`。

## Risks / Trade-offs

| 風險 | 緩解 |
|------|------|
| 跨檔案系統 rename 失敗 | Fallback 到 copy + remove + verify |
| 多個 tgconn process 同 cwd 同時啟動，搶 migration | 用 `os.Mkdir` 互斥建立目錄；失敗者直接走中央路徑 |
| Migration 中途 crash | `<centralized>` 殘留 + `<cwd>/.tgconn/` 部分搬走 → 重啟時偵測到兩處都存在 → 警告 + 停止讓 user 處理 |
| Compaction 截斷使用者剛好要看的 history | daily logs 仍有完整 audit；user 可用 `--history-size` 調整注入筆數；`HistoryMaxEntries=0` 關閉 compaction |
| `~/.tgconn/projects/<encoded>/` 變數展開出問題 | 明確用 `os.UserHomeDir()`，不依賴 shell `~` 展開 |

## Migration Plan

**首次升級時**（user 把舊 binary 換成新 binary 後第一次跑 `tgconn connect`）：
1. 偵測 `<cwd>/.tgconn/` 存在 + `~/.tgconn/projects/<encoded>/` 不存在
2. 自動搬遷
3. 寫 `MOVED_TO_<encoded>.txt` 麵包屑

**Rollback**：使用者可手動把 `~/.tgconn/projects/<encoded>/` 搬回 `<cwd>/.tgconn/`，
然後降級 binary。新版本不會把舊版資料改格式，所以是無痛 rollback。

**完全重置**：使用者可 `rm -rf ~/.tgconn/projects/<encoded>/` 或跑 `tgconn clean --all`。

## Open Questions

無（B 選項 history 處理已敲定；其他細節已對齊）。
