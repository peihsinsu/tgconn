# tgconn — Telegram LLM Connector

Bridge Telegram to an LLM provider (Claude, Codex) by executing it as a subprocess in your current working directory. Each Telegram message triggers the provider CLI and streams the response back to the chat.
Please aware about this is release your LLM provider (claude) that you can remote work with it through telegram. You may become more busy...

## How It Works

```
Telegram message
    ↓
tgconn (Go daemon)  ←  immediately ACKs with job #N
    ↓
claude -p "<message>"   ← runs in CWD with configurable permission mode
    ↓
result pushed back to Telegram chat when done
```

The subprocess runs from the **current working directory**, so the LLM can read local files and project context just like you would from the terminal.

---

## Requirements

- Go 1.24+
- The LLM provider CLI on `$PATH` (`claude` or `codex`)
- A Telegram bot token (obtain from [@BotFather](https://t.me/BotFather))
- *(Optional)* `whisper` CLI for voice message transcription

---

## Installation

```bash
git clone https://github.com/cx009/tgconn
cd tgconn
make install          # build + install to ~/go/bin (no sudo needed)
```

Other build targets:

```bash
make build            # build ./tgconn (current platform)
make build-all        # cross-compile for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64 → dist/
make install INSTALL_DIR=/usr/local/bin   # install to custom path (requires sudo)
```

---

## Setup

### Quickstart (recommended)

```bash
tgconn init
```

`tgconn init` walks you through the entire first-time setup in one command:

1. Prompts for your bot token (from [@BotFather](https://t.me/BotFather)) and LLM provider.
2. Writes `~/.config/tgconn/config.yaml` (permissions `0600`).
3. Connects to Telegram and waits for you to send any message to the bot.
4. Captures the sender's chat ID and asks you to confirm it in the terminal.
5. Saves the confirmed chat ID into the config.

After `tgconn init` finishes, just run `tgconn connect`.

---

### Manual Setup

#### 1. Create a Telegram Bot

1. Open [@BotFather](https://t.me/BotFather) in Telegram.
2. Send `/newbot` and follow the prompts.
3. Copy the **bot token** (looks like `123456789:ABCDefGhIJKlmNoPQRSTuvWXyz`).

#### 2. Find Your Chat ID

Send any message to [@userinfobot](https://t.me/userinfobot) — it replies with your numeric chat ID.

#### 3. Configure tgconn

**Option A — `tgconn config init` (prompts, no chat-ID capture):**

```bash
tgconn config init
```

Creates `~/.config/tgconn/config.yaml` with restricted permissions (0600).

**Option B — Environment variables:**

```bash
export TELEGRAM_BOT_TOKEN=123456789:ABCDefGhIJKlmNoPQRSTuvWXyz
export TGCONN_PROVIDER=claude
export TGCONN_ALLOWED_CHATS=123456789
```

**Option C — CLI flags:**

```bash
tgconn --provider claude connect \
  --allow-chat 123456789 \
  --exec-mode ask
```

---

## Configuration Reference

Priority order: **CLI flag > environment variable > config file**

| Config key          | Env var                | CLI flag           | Description |
|---------------------|------------------------|--------------------|-------------|
| `token`             | `TELEGRAM_BOT_TOKEN`   | —                  | Telegram bot token (required) |
| `provider`          | `TGCONN_PROVIDER`      | `--provider`       | LLM provider: `claude`, `codex` |
| `allowed_chats`     | `TGCONN_ALLOWED_CHATS` | `--allow-chat`     | Chat ID whitelist (required) |
| `exec_mode`         | —                      | `--exec-mode`      | Permission mode: `auto`, `ask`, `safe` (default: `ask`) |
| `timeout`           | —                      | `--timeout`        | Provider execution timeout in seconds (default: 7200) |
| `history_size`      | —                      | `--history-size`   | Past Q&A exchanges injected as context (default: 10, 0 = disabled) |
| `enable_voice`      | —                      | `--enable-voice`   | Enable voice message transcription via whisper CLI |
| `anthropic_api_key` | `ANTHROPIC_API_KEY`    | `--api-key`        | Anthropic API key (alternative to `~/.claude` session auth) |
| `max_jobs`          | —                      | `--max-jobs`       | Max concurrent regular jobs (default: 0 = unlimited) |
| `max_cron_jobs`     | —                      | `--max-cron-jobs`  | Max concurrent cron executions (default: 0 = unlimited) |
| `debug`             | —                      | `--debug`          | Enable verbose debug logging |

### Config File

`~/.config/tgconn/config.yaml`:

```yaml
token: "123456789:ABCDefGhIJKlmNoPQRSTuvWXyz"
provider: claude
allowed_chats:
  - 123456789
exec_mode: ask
timeout: 7200
history_size: 10
anthropic_api_key: ""   # leave empty to use ~/.claude session
max_jobs: 0             # max concurrent regular jobs (0 = unlimited)
max_cron_jobs: 0        # max concurrent cron executions (0 = unlimited)
```

---

## Authentication

tgconn supports two Claude authentication methods:

### Session (OAuth)

The default. Claude CLI reads credentials from `~/.claude/` (obtained via `claude` login on the host). No extra configuration needed.

### API Key

Set `anthropic_api_key` in the config file, pass `--api-key`, or export `ANTHROPIC_API_KEY`. Takes precedence over session auth. Recommended when running in Docker.

```bash
export ANTHROPIC_API_KEY=sk-ant-...
tgconn --provider claude connect --allow-chat 123456789
```

---

## Execution Modes

Control how Claude handles permission prompts with `--exec-mode`:

| Mode | Behaviour | Claude flag |
|------|-----------|-------------|
| `auto` | All operations permitted; no approval prompts | `--dangerously-skip-permissions` |
| `ask` *(default)* | Each permission prompt forwarded to Telegram as ✅ Allow / ❌ Deny buttons | — |
| `safe` | Read-only analysis; file writes and shell execution blocked | `--permission-mode plan` |

In `ask` mode, when Claude requests permission to use a tool, tgconn sends a Telegram message with inline buttons. Claude waits until you approve or deny before continuing. Unanswered prompts are auto-denied after 5 minutes.

> **PTY fallback:** `ask` mode requires a PTY to intercept Claude's permission prompts. On systems where PTY allocation fails (e.g. some Docker setups), tgconn automatically falls back to `auto` mode and logs a warning. To guarantee approval prompts in Docker, use a TTY (`docker run -it ...`) or set `--exec-mode auto` explicitly.

> **Codex provider limitation:** Codex only supports `auto` mode. If `ask` or `safe` is configured with `--provider codex`, the bot returns an error message for each request rather than invoking Codex — switch to `--exec-mode auto` or use `--provider claude` instead.

> **Note:** Cron-triggered tasks always run in `auto` mode regardless of the global setting, since they execute unattended.

---

## Bot Commands

Once connected, send these commands directly in the Telegram chat:

| Command | Description |
|---------|-------------|
| Any text / media | Forwarded to the LLM; immediately ACKs with job #N, result pushed when done |
| `/list` | List running jobs with IDs and elapsed time |
| `/stop <id>` | Cancel a running job |
| `/status` | Show bot status: uptime, provider, exec mode, active jobs |
| `/history` | Display the last N conversation exchanges for this chat |
| `/cron <expr> <prompt>` | Schedule a recurring task |
| `/cron list` | List scheduled tasks for this chat |
| `/cron del <id>` | Delete a scheduled task |
| `/session start` | Open an interactive session (persistent Claude process) |
| `/session end` | Close the current interactive session |
| `/session status` | Show session uptime and message count |
| `/help` or `/?` | Show command reference |

---

## Scheduled Tasks (Cron)

Schedule recurring LLM tasks using standard cron expressions. Tasks fire unattended and push results to the chat when done.

```
# Standard 5-field cron (min hour dom month dow)
/cron 0 9 * * * Hi devops help me to check my emqx cluster status

# Every Monday at 9am
/cron 0 9 * * 1 "create healthcheck report for last week"

# Predefined schedules
/cron @hourly  help me to calcuate the error rate for last hour
/cron @daily   please do weekly healthcheck for my environment

# List all scheduled tasks
/cron list

# Delete a task by ID
/cron del abc123
```

Tasks are persisted to `.tgconn/cron/<id>.json` and automatically reloaded on restart.

---

## Interactive Session Mode

By default tgconn is **one-shot**: each message spawns a fresh provider subprocess.
Interactive session mode keeps a single Claude process alive for the duration of a
conversation, enabling true multi-turn dialogue where Claude remembers context natively.

```
/session start     → Claude starts in stream-json I/O mode
Your message       → sent directly to the running Claude process
Follow-up          → Claude has full context of everything above
/session end       → Claude process is terminated
```

- Sessions are **per-chat** — different chats have independent sessions.
- Sessions with an **in-flight request** are never auto-closed mid-execution.
- Sessions idle for **30 minutes** after the last completed response are closed automatically.
- Session mode inherits the current `--exec-mode` setting.

---

## Media Support

| Type | Behaviour |
|------|-----------|
| Text | Forwarded directly to the LLM |
| Photo | Downloaded to `.tgconn/tmp/<chat_id>/`; path + caption injected into prompt |
| Document / File | Downloaded (≤ 20 MB); path, filename, and caption injected into prompt |
| Voice message | Transcribed with `whisper` (requires `--enable-voice`); transcript forwarded as text |
| Sticker | Friendly unsupported-type reply (emoji included) |
| Video / Audio / GIF | Unsupported-type reply with guidance |

### Voice Transcription Setup

```bash
pip install openai-whisper
tgconn --provider claude connect --allow-chat 123456789 --enable-voice
```

Files larger than **20 MB** are rejected with a user-friendly error message.

---

## Docker

A multi-stage Dockerfile is included with Go 1.24.7, Node.js 24.7.0, Python 3.13.11, and the Claude CLI pre-installed.

```bash
make docker-build              # build tgconn:latest
make docker-build VERSION=1.0.0
```

### Run with API key (recommended for Docker)

```bash
docker run --rm \
  -v ~/.config/tgconn:/home/tgconn/.config/tgconn:ro \
  -v $(pwd):/workspace \
  -e ANTHROPIC_API_KEY=sk-ant-... \
  tgconn:latest connect
```

> **Note:** Session-based auth (`~/.claude`) relies on the macOS Keychain and is not portable to Linux containers. Use `ANTHROPIC_API_KEY` instead.

### Makefile targets

```bash
make docker-run-apikey  ALLOW_CHAT=123456789   # run with API key auth
make docker-run-session ALLOW_CHAT=123456789   # run with session mount (host only)
```

---

## Execution Log

Every message handled is appended to a daily JSONL file in `.tgconn/` inside the current working directory:

```
.tgconn/
├── 2026-05-01.jsonl             ← daily audit log (all messages)
├── history_123456789.jsonl      ← per-chat history (successful exchanges only)
└── cron/
    ├── abc123.json              ← persisted cron task definition
    └── 2026-05-01.jsonl         ← cron execution log (daily, one entry per firing)
```

---

## Conversation History

tgconn maintains a per-chat history and injects the last N exchanges as context into every new one-shot prompt.

| Flag / key | Default | Description |
|------------|---------|-------------|
| `--history-size` / `history_size` | `10` | Number of past exchanges to inject (0 to disable) |

> In interactive session mode, Claude manages its own context — history injection is not used.

---

## Security

tgconn restricts access by Telegram chat ID — only whitelisted IDs can trigger execution.

- **`ask`** (default): Claude asks for approval via Telegram before sensitive operations.
- **`safe`**: Read-only operations only — no file writes or shell execution.
- **`auto`**: All permission checks bypassed. Use only in trusted environments.

For fine-grained tool control, create a `.claude/settings.local.json` in the project directory.

### Rate Limiting

Prevent resource exhaustion by capping concurrent executions:

```yaml
max_jobs: 5        # max concurrent regular jobs (0 = unlimited)
max_cron_jobs: 2   # max concurrent cron executions (0 = unlimited)
```

The two limits are **independent** — cron jobs do not count against `max_jobs`. When a limit is reached:
- Regular jobs: the request is rejected immediately with an error message.
- Cron jobs: the scheduled firing is skipped (with a Telegram notification); the next scheduled run proceeds normally.

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
├── Dockerfile
├── Makefile
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
    ├── cronjob/
    │   └── manager.go       Cron scheduler, job persistence (.tgconn/cron/)
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
    │   └── recorder.go      Daily JSONL audit log + per-chat history + cron execution log
    ├── session/
    │   └── session.go       Persistent interactive Claude session (stream-json)
    └── transcriber/
        └── transcriber.go   Whisper CLI wrapper for voice transcription
```

---

## Development

```bash
make build      # build ./tgconn
make build-all  # cross-compile all platforms → dist/
make test       # go test ./...
make lint       # go vet ./...
make install    # install to ~/go/bin
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
