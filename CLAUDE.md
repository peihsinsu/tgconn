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

# Misc
tgconn version
tgconn --help
```

## Configuration

Priority order: CLI flag → environment variable → config file (`~/.tgconn/config.yaml`)

| Key | Env Var | Description |
|-----|---------|-------------|
| `token` | `TELEGRAM_BOT_TOKEN` | Telegram bot token |
| `provider` | `TGCONN_PROVIDER` | Default provider (claude/codex/gemini) |
| `allowed_chats` | `TGCONN_ALLOWED_CHATS` | Comma-separated chat IDs whitelist |

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
