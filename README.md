# tgconn — Telegram LLM Connector

Bridge Telegram to an LLM provider (Claude, Codex, Gemini) by executing it as a subprocess in your current working directory. Each Telegram message triggers the provider CLI and streams the response back to the chat.

## How It Works

```
Telegram message
    ↓
tgconn (Go daemon)
    ↓
claude -p "<message>" --dangerously-skip-permissions   ← runs in CWD
    ↓
stdout streamed back to Telegram chat
```

The subprocess runs from the **current working directory**, so the LLM can read local files and project context just like you would from the terminal.

---

## Requirements

- Go 1.24+
- The LLM provider CLI on `$PATH` (e.g. `claude`, `codex`, `gemini`)
- A Telegram bot token (obtain from [@BotFather](https://t.me/BotFather))

---

## Installation

```bash
git clone https://github.com/cx009/tgconn
cd tgconn
make build          # produces ./tgconn
```

Move `tgconn` to somewhere on your `$PATH`, e.g.:

```bash
mv tgconn /usr/local/bin/
```

---

## Setup

### 1. Create a Telegram Bot

1. Open [@BotFather](https://t.me/BotFather) in Telegram.
2. Send `/newbot` and follow the prompts.
3. Copy the **bot token** (looks like `123456789:ABCDefGhIJKlmNoPQRSTuvWXyz`).

### 2. Find Your Chat ID

Send any message to [@userinfobot](https://t.me/userinfobot) — it replies with your numeric chat ID.

### 3. Configure tgconn

**Option A — Interactive setup (recommended):**

```bash
tgconn config init
```

This creates `~/.tgconn/config.yaml` with restricted file permissions (0600).

**Option B — Environment variables:**

```bash
export TELEGRAM_BOT_TOKEN=123456789:ABCDefGhIJKlmNoPQRSTuvWXyz
export TGCONN_PROVIDER=claude
export TGCONN_ALLOWED_CHATS=123456789
```

**Option C — CLI flags (override config/env at runtime):**

```bash
tgconn --provider claude connect \
  --allow-chat 123456789 \
  --timeout 120
```

---

## Configuration Reference

Priority order: **CLI flag > environment variable > config file**

| Config key      | Env var                | CLI flag           | Description                                              |
|-----------------|------------------------|--------------------|----------------------------------------------------------|
| `token`         | `TELEGRAM_BOT_TOKEN`   | —                  | Telegram bot token (required)                            |
| `provider`      | `TGCONN_PROVIDER`      | `--provider`       | LLM provider: `claude`, `codex`, `gemini`                |
| `allowed_chats` | `TGCONN_ALLOWED_CHATS` | `--allow-chat`     | Comma-separated chat ID whitelist (required)             |
| `timeout`       | —                      | `--timeout`        | Provider execution timeout in seconds (default: 900)     |
| `history_size`  | —                      | `--history-size`   | Past Q&A exchanges injected as context (default: 10, 0 = disabled) |
| `debug`         | —                      | `--debug`          | Enable verbose debug logging                             |

### Config File

`~/.tgconn/config.yaml` (created by `tgconn config init`):

```yaml
token: "123456789:ABCDefGhIJKlmNoPQRSTuvWXyz"
provider: "claude"
allowed_chats:
  - 123456789
  - 987654321   # add more chat IDs as needed
timeout: 900
history_size: 10
```

---

## Bot Commands

Once connected, you can send these commands directly in the Telegram chat:

| Command | Description |
|---------|-------------|
| Any text | Forwarded to the LLM provider; response is sent back |
| `/list` | List currently running provider jobs with their IDs and elapsed time |
| `/stop <id>` | Cancel a running job by its ID (e.g. `/stop 1`) |
| `/status` | Show bot status: username, hostname, uptime, active jobs, provider |
| `/history` | Display the last N conversation exchanges (Q&A pairs) for this chat |

When the bot starts, it broadcasts a connection notice to all allowed chats. On shutdown it sends a disconnect notice before exiting.

---

## Usage

```bash
# Start with Claude provider
tgconn --provider claude connect --allow-chat 123456789

# Multiple allowed chats
tgconn --provider claude connect --allow-chat 123456789,987654321

# Enable verbose debug logging
tgconn --provider claude connect --allow-chat 123456789 --debug

# Override provider execution timeout (seconds)
tgconn --provider claude connect --allow-chat 123456789 --timeout 300

# Set conversation history context (0 to disable)
tgconn --provider claude connect --allow-chat 123456789 --history-size 5

# Use config file — no flags needed if ~/.tgconn/config.yaml is set up
tgconn connect

# Inspect resolved configuration (token is masked)
tgconn config show

# Print version
tgconn version
```

---

## Execution Log

Every message handled by the bot is appended to a daily JSONL file in `.tgconn/` inside the current working directory:

```
.tgconn/
└── 2026-05-01.jsonl   ← one JSON object per line, rotated daily
```

Each entry contains:

```json
{
  "time": "2026-05-01T10:00:12.501Z",
  "chat_id": 123456789,
  "from": "@alice",
  "question": "what does main.go do?",
  "response": "main.go is the entry point...",
  "elapsed_ms": 7400
}
```

On error the `"error"` field is set instead of `"response"`. The `.tgconn/` directory is created automatically on startup.

---

## Conversation History

tgconn maintains a per-chat conversation history and automatically injects the last N exchanges as context into every new prompt, giving the LLM continuity across messages.

### History files

Successful Q&A exchanges are persisted in a separate per-chat JSONL file:

```
.tgconn/
├── 2026-05-01.jsonl             ← daily audit log (all messages)
└── history_123456789.jsonl      ← per-chat history (successful exchanges only)
```

### Context injection

Before forwarding your message to the provider, tgconn prepends the recent history:

```
[Previous conversation]
Q: what does main.go do?
A: main.go is the entry point...

Q: how is the bot started?
A: ...
[End of history]

<your new question here>
```

### Configuration

| Flag / key | Default | Description |
|------------|---------|-------------|
| `--history-size` / `history_size` | `10` | Number of past exchanges to inject (0 to disable) |

Use `/history` in Telegram to review the stored exchanges for the current chat.

---

## Logging

tgconn uses Go's standard `log/slog` package and writes structured text logs to **stderr**.

### Normal mode (INFO level)

```
time=2024-01-15T10:00:00.000Z level=INFO msg="configuration loaded" provider=claude allowed_chats_count=1 timeout=15m0s debug=false
time=2024-01-15T10:00:00.010Z level=INFO msg="provider binary ready" provider=claude
time=2024-01-15T10:00:00.020Z level=INFO msg="execution recorder ready" log_dir=.tgconn
time=2024-01-15T10:00:00.050Z level=INFO msg="telegram API connection established" bot_username=MyBot
time=2024-01-15T10:00:00.051Z level=INFO msg="bot starting" username=MyBot provider=claude allowed_chats=[123456789] timeout=15m0s
time=2024-01-15T10:00:00.051Z level=INFO msg="polling for updates" poll_timeout_sec=60
time=2024-01-15T10:00:05.100Z level=INFO msg="handling message" chat_id=123456789 from=@alice message_id=42 question_runes=25
time=2024-01-15T10:00:05.101Z level=INFO msg="invoking provider" provider=claude chat_id=123456789 job_id=1 timeout=15m0s
time=2024-01-15T10:00:05.200Z level=INFO msg="claude subprocess started" pid=54321 args=[claude -p ... --dangerously-skip-permissions]
time=2024-01-15T10:00:12.500Z level=INFO msg="claude subprocess completed" pid=54321 elapsed=7.3s stdout_bytes=842 stderr_bytes=0
time=2024-01-15T10:00:12.501Z level=INFO msg="provider response ready" chat_id=123456789 job_id=1 elapsed=7.4s response_runes=830 chunks=1
time=2024-01-15T10:00:12.600Z level=INFO msg="message handled" chat_id=123456789 message_id=42 job_id=1 elapsed=7.5s
```

### Debug mode (`--debug`)

Adds `level=DEBUG` entries including:
- Full question text
- Full response text (first 300 chars)
- Per-chunk send details
- Non-message update skips

### Log events reference

| Level | Event | Key fields |
|-------|-------|------------|
| INFO  | Configuration loaded | `provider`, `allowed_chats_count`, `timeout` |
| INFO  | Provider binary ready | `provider` |
| INFO  | Execution recorder ready | `log_dir` |
| INFO  | Bot connected to Telegram | `bot_username` |
| INFO  | Bot starting | `username`, `provider`, `allowed_chats`, `timeout` |
| INFO  | Message received | `chat_id`, `from`, `message_id`, `question_runes` |
| INFO  | Provider invoked | `provider`, `chat_id`, `job_id`, `timeout` |
| INFO  | Subprocess started | `pid`, `args` |
| INFO  | Subprocess completed | `pid`, `elapsed`, `stdout_bytes`, `stderr_bytes` |
| INFO  | Response ready | `chat_id`, `job_id`, `elapsed`, `response_runes`, `chunks` |
| INFO  | Message handled | `chat_id`, `message_id`, `job_id`, `elapsed` |
| INFO  | Slow provider — waiting notice sent | `chat_id`, `job_id`, `elapsed` |
| INFO  | Slow provider — progress notice sent | `chat_id`, `job_id`, `elapsed` |
| INFO  | Job stop requested | `job_id`, `chat_id` |
| INFO  | SIGTERM sent to process group | `pgid` |
| WARN  | Unauthorized chat ignored | `chat_id`, `from`, `update_id` |
| WARN  | Subprocess killed (timeout/cancel) | `pid`, `reason`, `elapsed` |
| WARN  | SIGKILL sent after grace period | `pgid` |
| WARN  | Shutdown timed out | — |
| ERROR | Provider binary not found | `provider`, `error` |
| ERROR | Provider error | `chat_id`, `elapsed`, `error` |
| ERROR | Subprocess failed | `pid`, `elapsed`, `error`, `stderr` |
| ERROR | Telegram send failed | `chat_id`, `error` |

---

## Security Warning

`tgconn` starts the LLM provider with `--dangerously-skip-permissions`, which allows it to read and execute anything in the current directory and beyond.

**Always restrict access using `--allow-chat`.**  Anyone whose chat ID is in the allowlist can trigger arbitrary command execution via the LLM. Keep the allowlist to your own chat IDs only.

---

## Supported Providers

| Provider | Status | Binary |
|----------|--------|--------|
| `claude` | Supported | `claude` (Claude Code CLI) |
| `codex`  | Stub (partial) | `codex` |
| `gemini` | Not yet implemented | — |

---

## Project Layout

```
tgconn/
├── main.go                     entry point
├── cmd/
│   ├── root.go                 root cobra command, viper init
│   ├── connect.go              `connect` subcommand — starts the bot
│   ├── config.go               `config init` / `config show`
│   └── version.go              `version` subcommand
└── internal/
    ├── bot/
    │   └── bot.go              Telegram polling loop, message dispatch, /list /stop /status /history
    ├── config/
    │   └── config.go           Config struct, Load(), Validate()
    ├── provider/
    │   ├── provider.go         Provider interface + factory
    │   ├── claude.go           Claude subprocess adapter
    │   ├── codex.go            Codex subprocess adapter (stub)
    │   └── subprocess.go       Process spawning, SIGTERM/SIGKILL lifecycle
    └── recorder/
        └── recorder.go         Daily JSONL audit log + per-chat history (.tgconn/)
```

---

## Development

```bash
make build      # build ./tgconn
make test       # go test ./...
make lint       # go vet ./...
```

Run tests:

```bash
go test ./...
```

### Adding a New Provider

1. Create `internal/provider/<name>.go` implementing the `Provider` interface:

```go
type Provider interface {
    Execute(ctx context.Context, question string) (string, error)
    Check() error
}
```

2. Register it in `internal/provider/provider.go` `New()` switch.
3. Add it to the valid provider list in `internal/config/config.go` `Validate()`.
