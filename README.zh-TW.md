# tgconn — Telegram LLM 連接器

將 Telegram 橋接到 LLM 提供者（Claude、Codex），透過在當前工作目錄執行 CLI 子程序的方式，把每則 Telegram 訊息轉發給 LLM，並將結果推送回聊天室。

## 運作原理

```
Telegram 訊息
    ↓
tgconn（Go 常駐程式）  ←  立即回覆任務編號 #N
    ↓
claude -p "<訊息>"   ← 在當前工作目錄執行，可讀取本機檔案
    ↓
完成後將結果推送回 Telegram 聊天室
```

子程序從**當前工作目錄**啟動，因此 LLM 可以像在終端機一樣讀取本機檔案與專案內容。

---

## 系統需求

- Go 1.24+
- LLM 提供者 CLI 在 `$PATH` 中（`claude` 或 `codex`）
- Telegram Bot Token（向 [@BotFather](https://t.me/BotFather) 申請）
- *（選用）* `whisper` CLI，用於語音訊息轉錄

---

## 安裝

```bash
git clone https://github.com/cx009/tgconn
cd tgconn
make install          # 建置並安裝到 ~/go/bin（不需要 sudo）
```

其他建置目標：

```bash
make build            # 建置 ./tgconn（當前平台）
make build-all        # 跨平台編譯 linux/amd64、linux/arm64、darwin/amd64、darwin/arm64 → dist/
make install INSTALL_DIR=/usr/local/bin   # 安裝到自訂路徑（需要 sudo）
```

---

## 設定

### 快速開始（推薦）

```bash
tgconn init
```

`tgconn init` 以一個指令完成所有首次設定：

1. 詢問 Bot Token（[@BotFather](https://t.me/BotFather) 取得）與 LLM 提供者。
2. 寫入 `~/.config/tgconn/config.yaml`（權限 `0600`）。
3. 連線到 Telegram，等待你向 Bot 傳送任意訊息。
4. 產生 6 位數驗證碼，顯示在終端機，並通知 Telegram 端輸入。
5. 驗證碼比對正確後，將 Chat ID 儲存至設定檔。

`tgconn init` 完成後，直接執行 `tgconn connect` 即可使用。

---

### 手動設定

#### 1. 建立 Telegram Bot

1. 開啟 [@BotFather](https://t.me/BotFather)。
2. 傳送 `/newbot` 並依指示操作。
3. 複製 **Bot Token**（格式如 `123456789:ABCDefGhIJKlmNoPQRSTuvWXyz`）。

#### 2. 取得 Chat ID

向 [@userinfobot](https://t.me/userinfobot) 傳送任意訊息，它會回覆你的數字 Chat ID。

#### 3. 設定 tgconn

**方式 A — `tgconn config init`（互動式，不含 Chat ID 捕捉）：**

```bash
tgconn config init
```

建立 `~/.config/tgconn/config.yaml`，權限 0600。

**方式 B — 環境變數：**

```bash
export TELEGRAM_BOT_TOKEN=123456789:ABCDefGhIJKlmNoPQRSTuvWXyz
export TGCONN_PROVIDER=claude
export TGCONN_ALLOWED_CHATS=123456789
```

**方式 C — CLI flags：**

```bash
tgconn --provider claude connect \
  --allow-chat 123456789 \
  --exec-mode ask
```

---

## 設定參考

優先順序：**CLI flag > 環境變數 > 設定檔**

| 設定鍵              | 環境變數               | CLI flag           | 說明 |
|---------------------|------------------------|--------------------|------|
| `token`             | `TELEGRAM_BOT_TOKEN`   | —                  | Telegram Bot Token（必填） |
| `provider`          | `TGCONN_PROVIDER`      | `--provider`       | LLM 提供者：`claude`、`codex` |
| `allowed_chats`     | `TGCONN_ALLOWED_CHATS` | `--allow-chat`     | 允許的 Chat ID 白名單（必填） |
| `exec_mode`         | —                      | `--exec-mode`      | 權限模式：`auto`、`ask`、`safe`（預設：`ask`） |
| `timeout`           | —                      | `--timeout`        | 提供者執行逾時秒數（預設：7200） |
| `history_size`      | —                      | `--history-size`   | 注入為上下文的過去對話筆數（預設：10，0 為停用） |
| `enable_voice`      | —                      | `--enable-voice`   | 啟用語音訊息轉錄（需 whisper CLI） |
| `anthropic_api_key` | `ANTHROPIC_API_KEY`    | `--api-key`        | Anthropic API Key（替代 `~/.claude` session 認證） |
| `max_jobs`          | —                      | `--max-jobs`       | 最大同時執行的一般任務數（0 = 無限制） |
| `max_cron_jobs`     | —                      | `--max-cron-jobs`  | 最大同時執行的排程任務數（0 = 無限制） |
| `debug`             | —                      | `--debug`          | 啟用詳細 debug 日誌 |

### 設定檔範例

`~/.config/tgconn/config.yaml`：

```yaml
token: "123456789:ABCDefGhIJKlmNoPQRSTuvWXyz"
provider: claude
allowed_chats:
  - 123456789
exec_mode: ask
timeout: 7200
history_size: 10
anthropic_api_key: ""   # 留空則使用 ~/.claude session 認證
max_jobs: 0             # 最大同時一般任務數（0 = 無限制）
max_cron_jobs: 0        # 最大同時排程任務數（0 = 無限制）
```

---

## 認證方式

tgconn 支援兩種 Claude 認證方式：

### Session（OAuth）

預設方式。Claude CLI 從 `~/.claude/` 讀取憑證（在主機執行 `claude` 登入取得），無需額外設定。

### API Key

在設定檔設定 `anthropic_api_key`、傳入 `--api-key`，或匯出 `ANTHROPIC_API_KEY`。優先於 session 認證，建議在 Docker 環境中使用。

```bash
export ANTHROPIC_API_KEY=sk-ant-...
tgconn --provider claude connect --allow-chat 123456789
```

---

## 執行模式

透過 `--exec-mode` 控制 Claude 處理權限提示的方式：

| 模式 | 行為 | Claude flag |
|------|------|-------------|
| `auto` | 允許所有操作，不出現提示 | `--dangerously-skip-permissions` |
| `ask` *（預設）* | 每次權限提示透過 Telegram 內嵌按鈕轉發（✅ 允許 / ❌ 拒絕） | — |
| `safe` | 唯讀分析，禁止寫入檔案或執行 shell | `--permission-mode plan` |

在 `ask` 模式下，Claude 請求工具使用權限時，tgconn 會發送 Telegram 訊息附上按鈕，Claude 等待你回應後才繼續。未回應的提示在 5 分鐘後自動拒絕。

> **PTY 退回機制：** `ask` 模式需要 PTY 來攔截 Claude 的權限提示。若 PTY 分配失敗（例如部分 Docker 設定），tgconn 會自動退回 `auto` 模式並記錄警告。如需在 Docker 中保留提示功能，請使用 `docker run -it ...` 或明確設定 `--exec-mode auto`。

> **Codex 限制：** Codex 僅支援 `auto` 模式。以 `--provider codex` 搭配 `ask` 或 `safe` 時，Bot 會回傳錯誤訊息，請改用 `--exec-mode auto` 或改用 `--provider claude`。

> **注意：** 排程觸發的任務無論全域設定為何，一律以 `auto` 模式執行（因為是無人值守執行）。

---

## Bot 指令

連線後，直接在 Telegram 聊天室傳送以下指令：

| 指令 | 說明 |
|------|------|
| 任意文字 / 媒體 | 轉發給 LLM，立即回覆任務編號，完成後推送結果 |
| `/list` | 列出執行中的任務及已耗時間 |
| `/stop <id>` | 取消指定的執行中任務 |
| `/status` | 顯示 Bot 狀態：uptime、提供者、執行模式、執行中任務 |
| `/history` | 顯示此聊天室最近 N 筆對話記錄 |
| `/cron <expr> <prompt>` | 新增排程任務 |
| `/cron list` | 列出此聊天室的排程任務 |
| `/cron del <id>` | 刪除排程任務 |
| `/session start` | 開啟互動 session（持久化 Claude 程序） |
| `/session end` | 結束互動 session |
| `/session status` | 顯示 session 運行時間與訊息數 |
| `/help` 或 `/?` | 顯示指令說明 |

---

## 排程任務（Cron）

使用標準 cron 表達式設定週期性 LLM 任務，任務執行完畢後自動推送結果到聊天室。

```
# 標準 5 欄位 cron（分 時 日 月 星期）
/cron 0 9 * * * 幫我檢查 emqx cluster 狀態

# 每週一早上 9 點
/cron 0 9 * * 1 "產生本週報告"

# 預設排程
/cron @hourly  統計最近一小時的錯誤數
/cron @daily   每日系統健康摘要

# 列出所有排程任務
/cron list

# 刪除指定排程
/cron del abc123
```

任務持久化儲存在 `.tgconn/cron/<id>.json`，重啟後自動重新載入。

---

## 互動 Session 模式

tgconn 預設為**單次執行**模式：每則訊息啟動一個新的提供者子程序。
互動 session 模式保持單一 Claude 程序持續運行，讓 Claude 原生記住對話脈絡。

```
/session start     → Claude 以 stream-json I/O 模式啟動
傳送訊息           → 直接送進執行中的 Claude 程序
後續訊息           → Claude 完整記得所有上下文
/session end       → 終止 Claude 程序
```

- Session 以**聊天室為單位**，不同聊天室各自獨立。
- 有**進行中請求**的 session 絕不會被自動關閉。
- 最後一次完成回應後閒置 **30 分鐘**的 session 會自動關閉。
- Session 模式繼承當前 `--exec-mode` 設定。

---

## 媒體支援

| 類型 | 行為 |
|------|------|
| 文字 | 直接轉發給 LLM |
| 圖片 | 下載至 `.tgconn/tmp/<chat_id>/`，路徑與說明注入 prompt |
| 文件 / 檔案 | 下載（≤ 20 MB），路徑、檔名與說明注入 prompt |
| 語音訊息 | 以 `whisper` 轉錄（需 `--enable-voice`），轉錄文字轉發給 LLM |
| 貼圖 | 回覆不支援提示 |
| 影片 / 音訊 / GIF | 回覆不支援提示 |

### 語音轉錄設定

```bash
pip install openai-whisper
tgconn --provider claude connect --allow-chat 123456789 --enable-voice
```

超過 **20 MB** 的檔案會拒絕並回覆友善錯誤訊息。

---

## Docker

內附多階段 Dockerfile，已預裝 Go 1.24.7、Node.js 24.7.0、Python 3.13.11 與 Claude CLI。

```bash
make docker-build              # 建置 tgconn:latest
make docker-build VERSION=1.0.0
```

### 使用 API Key 執行（Docker 推薦方式）

```bash
docker run --rm \
  -v ~/.config/tgconn:/home/tgconn/.config/tgconn:ro \
  -v $(pwd):/workspace \
  -e ANTHROPIC_API_KEY=sk-ant-... \
  tgconn:latest connect
```

> **注意：** Session 認證（`~/.claude`）依賴 macOS Keychain，無法移植到 Linux 容器，請改用 `ANTHROPIC_API_KEY`。

### Makefile 目標

```bash
make docker-run-apikey  ALLOW_CHAT=123456789   # 使用 API Key 認證執行
make docker-run-session ALLOW_CHAT=123456789   # 使用 session 掛載執行（僅限本機）
```

---

## 執行日誌

每則處理的訊息都會附加到當前工作目錄下 `.tgconn/` 的每日 JSONL 檔案：

```
.tgconn/
├── 2026-05-01.jsonl             ← 每日審計日誌（所有訊息）
├── history_123456789.jsonl      ← 每聊天室歷史（僅成功的對話）
└── cron/
    ├── abc123.json              ← 排程任務定義
    └── 2026-05-01.jsonl         ← 排程執行日誌（每日，每次觸發一筆）
```

---

## 對話歷史

tgconn 維護每個聊天室的對話歷史，並在每次單次執行的 prompt 中注入最近 N 筆對話作為上下文。

| Flag / 設定鍵 | 預設值 | 說明 |
|--------------|--------|------|
| `--history-size` / `history_size` | `10` | 注入的過去對話筆數（0 為停用） |

> 互動 session 模式下，Claude 自行管理上下文，不使用歷史注入。

---

## 安全性

tgconn 以 Telegram Chat ID 限制存取，僅白名單內的 ID 可觸發執行。

- **`ask`**（預設）：敏感操作前透過 Telegram 向你請求授權。
- **`safe`**：唯讀操作，禁止寫入檔案或執行 shell。
- **`auto`**：略過所有權限檢查，僅在可信任環境中使用。

如需細粒度工具控制，可在專案目錄建立 `.claude/settings.local.json`。

### 流量限制（Rate Limiting）

透過設定同時執行上限防止資源耗盡：

```yaml
max_jobs: 5        # 最大同時一般任務數（0 = 無限制）
max_cron_jobs: 2   # 最大同時排程任務數（0 = 無限制）
```

兩個限制**各自獨立**，排程任務不佔用一般任務的 quota。達到上限時：
- 一般任務：立即拒絕並回傳錯誤訊息。
- 排程任務：跳過本次執行（並發送 Telegram 通知），下次排程到點時正常執行。

---

## 支援的提供者

| 提供者 | 狀態 | 執行檔 |
|--------|------|--------|
| `claude` | ✅ 支援 | `claude`（Claude Code CLI） |
| `codex`  | ✅ 支援 | `codex`（OpenAI Codex CLI） |
| `gemini` | 🚧 尚未實作 | — |

---

## 專案結構

```
tgconn/
├── Dockerfile
├── Makefile
├── main.go
├── cmd/
│   ├── root.go              根 cobra 指令，viper 初始化
│   ├── connect.go           `connect` 子指令 — 啟動 Bot
│   ├── init.go              `init` 子指令 — 首次設定精靈
│   ├── config.go            `config init` / `config show`
│   └── version.go           `version` 子指令
└── internal/
    ├── bot/
    │   └── bot.go           Telegram polling 迴圈、訊息分派、所有指令
    ├── config/
    │   └── config.go        Config struct、Load()、Validate()
    ├── cronjob/
    │   └── manager.go       Cron 排程器，任務持久化（.tgconn/cron/）
    ├── downloader/
    │   └── downloader.go    Telegram 檔案下載至 .tgconn/tmp/
    ├── provider/
    │   ├── provider.go      Provider 介面 + factory
    │   ├── approval.go      ApprovalFunc 類型 + context helpers
    │   ├── claude.go        Claude 子程序 adapter（exec-mode 路由）
    │   ├── codex.go         Codex 子程序 adapter
    │   ├── pty_runner.go    PTY-based runner（ask 模式授權流程）
    │   └── subprocess.go    程序生命週期管理（SIGTERM/SIGKILL）
    ├── recorder/
    │   └── recorder.go      每日 JSONL 審計日誌 + 對話歷史 + 排程執行日誌
    ├── session/
    │   └── session.go       持久化互動 Claude session（stream-json）
    └── transcriber/
        └── transcriber.go   Whisper CLI 封裝，語音轉錄
```

---

## 開發

```bash
make build      # 建置 ./tgconn
make build-all  # 跨平台編譯 → dist/
make test       # go test ./...
make lint       # go vet ./...
make install    # 安裝到 ~/go/bin
```

### 新增提供者

1. 建立 `internal/provider/<name>.go`，實作 `Provider` 介面：

```go
type Provider interface {
    Execute(ctx context.Context, question string) (string, error)
    Check() error
}
```

2. 在 `internal/provider/provider.go` 的 `New()` switch 中註冊。
3. 在 `internal/config/config.go` 的 `Validate()` 中加入有效提供者列表。
