# tgconn — Telegram LLM Connector

A Go CLI that bridges Telegram with LLM providers (Claude, Codex, Gemini) by executing them as subprocesses in the current working directory.

## How It Works

Each Telegram message received triggers a shell execution of the configured LLM provider:
```
claude -p "${message}" --dangerously-skip-permissions
codex ...
gemini ...
```
The subprocess runs from the **current working directory** so the LLM can read local files and project context. Output is streamed back to the originating Telegram chat.

## CLI Usage

```bash
# Start bot with a provider
tgconn --provider claude connect
tgconn --provider codex connect
tgconn --provider gemini connect

# Debug / verbose output
tgconn --provider claude connect --debug

# Restrict to specific Telegram chat IDs
tgconn --provider claude connect --allow-chat 123456789

# Config management
tgconn config init              # interactive setup (token, defaults)
tgconn config show              # print active configuration

# Storage cleanup (per-project)
tgconn clean --tmp                # delete all tmp downloads
tgconn clean --logs               # delete daily *.jsonl (history excluded)
tgconn clean --history            # delete history_*.jsonl (interactive confirm)
tgconn clean --all --dry-run      # preview without deleting
tgconn clean --all --yes          # wipe everything, no prompt

# Misc
tgconn version
tgconn --help
```

## Storage Layout

All per-project state lives at `~/.tgconn/projects/<encoded-cwd>/`, where
`<encoded-cwd>` is the absolute working directory with every `/` replaced by `-`
(Claude Code's encoding scheme). Example for this repo:

```
~/.tgconn/projects/-Users-charles-work-charles-tgconn/
├── YYYY-MM-DD.jsonl         # daily audit log
├── history_<chatID>.jsonl   # per-chat prompt-injection cache
├── tmp/<chatID>/            # downloaded photos / docs / voice
└── cron/
    ├── <jobID>.json         # persisted cron job definitions
    └── YYYY-MM-DD.jsonl     # cron execution audit log
```

On startup tgconn auto-migrates any legacy `<cwd>/.tgconn/` into the centralized
location, leaving a `MOVED_TO_<encoded>.txt` breadcrumb in the original spot.

### Retention (runs once at startup)

| Key | Default | Behavior |
|-----|---------|----------|
| `tmp_retention_hours` | `24` | Delete `tmp/` files older than N hours; empty `<chatID>/` subdirs are also removed |
| `log_retention_days` | `30` | Delete daily `*.jsonl` (project root + `cron/`) older than N days; `0` disables |
| `history_max_entries` | `100` | Trim each `history_*.jsonl` to its last N entries; `0` disables compaction |

`history_*.jsonl` and `cron/<jobID>.json` are NEVER touched by `log_retention_days`.
History is managed only by `history_max_entries` (startup) or `tgconn clean --history` (manual).

## Configuration

Priority order: CLI flag → environment variable → config file (`~/.tgconn/config.yaml`)

| Key | Env Var | Description |
|-----|---------|-------------|
| `token` | `TELEGRAM_BOT_TOKEN` | Telegram bot token |
| `provider` | `TGCONN_PROVIDER` | Default provider (claude/codex/gemini) |
| `allowed_chats` | `TGCONN_ALLOWED_CHATS` | Comma-separated chat IDs whitelist |
| `tmp_retention_hours` | `TGCONN_TMP_RETENTION_HOURS` | Hours before tmp/ files are deleted (default 24) |
| `log_retention_days` | `TGCONN_LOG_RETENTION_DAYS` | Days before daily logs are deleted; 0 disables (default 30) |
| `history_max_entries` | `TGCONN_HISTORY_MAX_ENTRIES` | Max entries per history file before startup compaction; 0 disables (default 100) |

## Tech Stack

- **Language**: Go 1.22+
- **CLI**: [cobra](https://github.com/spf13/cobra)
- **Config**: [viper](https://github.com/spf13/viper)
- **Telegram**: [go-telegram-bot-api/v5](https://github.com/go-telegram-bot-api/telegram-bot-api)
- **Subprocess**: `os/exec`

## Development

```bash
go build -o tgconn .
go test ./...
```

## Project Conventions

- Package layout: `cmd/` (cobra commands), `internal/provider/` (LLM adapters), `internal/bot/` (Telegram logic), `internal/config/` (config loading)
- Provider abstraction via `Provider` interface: `Execute(ctx, question string) (string, error)`
- Errors surfaced back to Telegram chat as user-readable messages
- No global state; dependencies injected via struct fields

<!-- OPENSPEC:START -->
# OpenSpec Instructions

These instructions are for AI assistants working in this project.

Always open `@/openspec/AGENTS.md` when the request:
- Mentions planning or proposals (words like proposal, spec, change, plan)
- Introduces new capabilities, breaking changes, architecture shifts, or big performance/security work
- Sounds ambiguous and you need the authoritative spec before coding

Use `@/openspec/AGENTS.md` to learn:
- How to create and apply change proposals
- Spec format and conventions
- Project structure and guidelines

Keep this managed block so 'openspec update' can refresh the instructions.

<!-- OPENSPEC:END -->
