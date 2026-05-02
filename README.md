# tgconn — Telegram LLM Connector

Bridge Telegram to an LLM provider (Claude, Codex) by executing it as a subprocess in your current working directory. Each Telegram message triggers the provider CLI and streams the response back to the chat.

## How It Works

```
Telegram message
    ↓
tgconn (Go daemon)
    ↓
claude -p "<message>"   ← runs in CWD with configurable permission mode
    ↓
response sent back to Telegram chat
```

The subprocess runs from the **current working directory**, so the LLM can read local files and project context just like you would from the terminal.

---

## Requirements

- Go 1.24+
- The LLM provider CLI on `$PATH` (`claude` or `codex`)
- A Telegram bot token (obtain from [@BotFather](https://t.me/BotFather))
- *(Optional)* `whisper` CLI on `$PATH` for voice message transcription

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
  --exec-mode ask
```

---

## Configuration Reference

Priority order: **CLI flag > environment variable > config file**

| Config key      | Env var                | CLI flag           | Description |
|-----------------|------------------------|--------------------|-------------|
| `token`         | `TELEGRAM_BOT_TOKEN`   | —                  | Telegram bot token (required) |
| `provider`      | `TGCONN_PROVIDER`      | `--provider`       | LLM provider: `claude`, `codex` |
| `allowed_chats` | `TGCONN_ALLOWED_CHATS` | `--allow-chat`     | Comma-separated chat ID whitelist (required) |
| `exec_mode`     | —                      | `--exec-mode`      | Permission mode: `auto`, `ask`, `safe` (default: `ask`) |
| `timeout`       | —                      | `--timeout`        | Provider execution timeout in seconds (default: 900) |
| `history_size`  | —                      | `--history-size`   | Past Q&A exchanges injected as context (default: 10, 0 = disabled) |
| `enable_voice`  | —                      | `--enable-voice`   | Enable voice message transcription via whisper CLI |
| `debug`         | —                      | `--debug`          | Enable verbose debug logging |

### Config File

`~/.tgconn/config.yaml` (created by `tgconn config init`):

```yaml
token: "123456789:ABCDefGhIJKlmNoPQRSTuvWXyz"
provider: "claude"
allowed_chats:
  - 123456789
exec_mode: "ask"
timeout: 900
history_size: 10
```

---

## Execution Modes

Control how Claude handles permission prompts with `--exec-mode`:

| Mode | Behaviour | Claude flag |
|------|-----------|-------------|
| `auto` | All operations permitted; no approval prompts | `--dangerously-skip-permissions` |
| `ask` *(default)* | Each permission prompt is forwarded to Telegram as ✅ Allow / ❌ Deny buttons | — |
| `safe` | Read-only analysis; file writes and shell execution are blocked | `--permission-mode plan` |

```bash
# Full trust — maximum productivity
tgconn --provider claude connect --allow-chat 123456789 --exec-mode auto

# Interactive approval (default)
tgconn --provider claude connect --allow-chat 123456789

# Read-only analysis — Claude can inspect but cannot modify files or run commands
tgconn --provider claude connect --allow-chat 123456789 --exec-mode safe
```

In `ask` mode, when Claude requests permission to use a tool, tgconn sends a Telegram
message with inline buttons. Claude waits until you approve or deny before continuing.
Unanswered prompts are auto-denied after 5 minutes.

---

## Media Support

tgconn handles several Telegram message types beyond plain text:

| Type | Behaviour |
|------|-----------|
| Text | Forwarded directly to the LLM |
| Photo | Downloaded to `.tgconn/tmp/<chat_id>/`; path + caption injected into prompt |
| Document / File | Downloaded (≤ 20 MB); path, filename, and caption injected into prompt |
| Voice message | Transcribed with `whisper` (requires `--enable-voice`); transcript forwarded as text |
| Sticker | Friendly unsupported-type reply (emoji included) |
| Video / Audio / GIF | Unsupported-type reply with guidance |

### Voice Transcription Setup

Install [Whisper](https://github.com/openai/whisper):

```bash
pip install openai-whisper
```

Enable at startup:

```bash
tgconn --provider claude connect --allow-chat 123456789 --enable-voice
```

tgconn checks for the `whisper` binary at startup and exits with an error if not found.

### File Size Limit

Files larger than **20 MB** are rejected with a user-friendly error message.

---

## Bot Commands

Once connected, send these commands directly in the Telegram chat:

| Command | Description |
|---------|-------------|
| Any text | Forwarded to the LLM provider; response is sent back |
| `/list` | List currently running provider jobs with IDs and elapsed time |
| `/stop <id>` | Cancel a running job (e.g. `/stop 1`) |
| `/status` | Show bot status: username, hostname, uptime, active jobs, provider, exec mode |
| `/history` | Display the last N conversation exchanges for this chat |
| `/session start` | Open an interactive session (persistent Claude process) |
| `/session end` | Close the current interactive session |
| `/session status` | Show session uptime and message count |

---

## Interactive Session Mode

By default tgconn is **one-shot**: each message spawns a fresh provider subprocess.
Interactive session mode keeps a single Claude process alive for the duration of a
conversation, enabling true multi-turn dialogue where Claude remembers context natively.

```
/session start
  → Claude starts in stream-json I/O mode

Your message here
  → sent directly to the running Claude process

Another follow-up
  → Claude has full context of everything above

/session end
  → Claude process is terminated
```

- Sessions are **per-chat** — different chats have independent sessions.
- Sessions idle for **30 minutes** are closed automatically with a notification.
- Without an active session, all messages use the standard one-shot path (unchanged).
- Session mode inherits the current `--exec-mode` setting.

---

## Usage Examples

```bash
# Minimal — read from ~/.tgconn/config.yaml
tgconn connect

# Claude, interactive approval, one allowed chat
tgconn --provider claude connect --allow-chat 123456789

# Full trust mode (CI-like environment)
tgconn --provider claude connect --allow-chat 123456789 --exec-mode auto

# Read-only code review
tgconn --provider claude connect --allow-chat 123456789 --exec-mode safe

# With voice transcription
tgconn --provider claude connect --allow-chat 123456789 --enable-voice

# Multiple allowed chats, 5-minute timeout
tgconn --provider claude connect --allow-chat 123456789,987654321 --timeout 300

# Verbose debug logging
tgconn --provider claude connect --allow-chat 123456789 --debug

# Inspect resolved configuration (token is masked)
tgconn config show

# Print version
tgconn version
```

---

## Execution Log

Every message handled is appended to a daily JSONL file in `.tgconn/` inside the current working directory:

```
.tgconn/
├── 2026-05-01.jsonl          ← daily audit log (all messages)
└── history_123456789.jsonl   ← per-chat history (successful exchanges only)
```

Each entry:

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

On error, `"error"` is set instead of `"response"`. The `.tgconn/` directory is created automatically on startup.

---

## Conversation History

tgconn maintains a per-chat conversation history and injects the last N exchanges as context into every new one-shot prompt, giving the LLM continuity across messages.

> **Note:** In interactive session mode, Claude manages its own context natively — history injection is not used.

| Flag / key | Default | Description |
|------------|---------|-------------|
| `--history-size` / `history_size` | `10` | Number of past exchanges to inject (0 to disable) |

Use `/history` in Telegram to review stored exchanges for the current chat.

---

## Security

tgconn restricts access by Telegram chat ID — only whitelisted IDs can trigger execution. Keep the allowlist to your own personal chat IDs.

The `--exec-mode` flag controls Claude's permission level:

- **`ask`** (default): Claude uses its own safety judgment and asks for approval via Telegram before performing sensitive operations.
- **`safe`**: Claude is restricted to read-only operations — no file writes or shell execution.
- **`auto`**: All permission checks are bypassed. Use only in trusted, sandboxed environments.

For additional fine-grained control, create a `.claude/settings.local.json` in the project directory to pre-approve or deny specific tools.

---

## Supported Providers

| Provider | Status | Binary |
|----------|--------|--------|
| `claude` | ✅ Supported | `claude` (Claude Code CLI) |
| `codex`  | ✅ Supported | `codex` (OpenAI Codex CLI) |
| `gemini` | 🚧 Not yet implemented | — |

---

## Project Layout

```
tgconn/
├── main.go
├── cmd/
│   ├── root.go              root cobra command, viper init
│   ├── connect.go           `connect` subcommand — starts the bot
│   ├── config.go            `config init` / `config show`
│   └── version.go           `version` subcommand
└── internal/
    ├── bot/
    │   └── bot.go           Telegram polling loop, message dispatch, all commands
    ├── config/
    │   └── config.go        Config struct, Load(), Validate()
    ├── downloader/
    │   └── downloader.go    Telegram file download to .tgconn/tmp/
    ├── provider/
    │   ├── provider.go      Provider interface + factory
    │   ├── approval.go      ApprovalFunc type + context helpers
    │   ├── claude.go        Claude subprocess adapter (exec-mode routing)
    │   ├── codex.go         Codex subprocess adapter
    │   ├── pty_runner.go    PTY-based runner for ask-mode approval flow
    │   └── subprocess.go    Process spawning, SIGTERM/SIGKILL lifecycle
    ├── recorder/
    │   └── recorder.go      Daily JSONL audit log + per-chat history
    ├── session/
    │   └── session.go       Persistent interactive Claude session (stream-json)
    └── transcriber/
        └── transcriber.go   Whisper CLI wrapper for voice transcription
```

---

## Development

```bash
make build      # build ./tgconn
make test       # go test ./...
make lint       # go vet ./...
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
