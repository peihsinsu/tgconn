# Project Context

## Purpose

**tgconn** (telegram-connector) is a CLI tool that connects a Telegram bot to LLM providers (Claude, Codex, Gemini). Each incoming Telegram message is forwarded to the configured provider by executing it as a subprocess in the current working directory, then the output is sent back to the Telegram chat.

Use case: run `tgconn` inside a project directory so the LLM (e.g., Claude Code) can read local files and execute commands with full project context — effectively giving you a Telegram interface to your dev environment.

## Tech Stack

- **Language**: Go 1.22+
- **CLI framework**: [cobra](https://github.com/spf13/cobra)
- **Config**: [viper](https://github.com/spf13/viper) — file + env var + flag priority chain
- **Telegram SDK**: [go-telegram-bot-api/v5](https://github.com/go-telegram-bot-api/telegram-bot-api)
- **LLM execution**: `os/exec` subprocess calls in current working directory
- **Build**: `go build`, no Docker required for local dev

## Project Conventions

### Code Style
- Standard `gofmt` / `goimports` formatting
- Exported types and functions have single-line doc comments
- Error strings are lowercase, no punctuation (Go convention)
- No `init()` functions; explicit initialization in `main`

### Architecture Patterns
- **Package layout**:
  - `cmd/` — cobra command definitions (one file per subcommand)
  - `internal/provider/` — LLM provider interface + per-provider implementations
  - `internal/bot/` — Telegram polling loop, message dispatch
  - `internal/config/` — config struct, loading, validation
- **Provider interface**: `Execute(ctx context.Context, question string) (string, error)`
- **No global state**: all dependencies injected via struct fields or constructor functions
- Graceful shutdown via `context.Context` cancellation on SIGINT/SIGTERM

### Testing Strategy
- Unit tests for provider adapters (mock `exec.Cmd`)
- Unit tests for config loading
- Integration tests are out of scope for MVP
- Test files co-located with the package they test (`*_test.go`)

### Git Workflow
- Feature branches: `feat/<short-description>`
- Commit convention: `type(scope): description` (Conventional Commits)
  - Types: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`
- PRs require passing `go vet`, `go test ./...`

## Domain Context

- Telegram bots use long-polling or webhooks; this project uses **long-polling** (simpler, no public endpoint needed)
- LLM CLI providers each have different invocation syntax:
  - Claude: `claude -p "${question}" --dangerously-skip-permissions`
  - Codex: TBD per OpenAI CLI interface
  - Gemini: TBD per Google CLI interface
- The subprocess CWD is the directory from which `tgconn` is invoked — this is intentional to give the LLM access to local project files
- Telegram bot tokens are sensitive; never log them

## Important Constraints

- Must run on macOS and Linux (primary targets)
- No persistent database; stateless per message
- Subprocess output may be large — truncate or paginate Telegram messages if over 4096 chars (Telegram API limit)
- Security: only whitelisted Telegram chat IDs should be able to trigger LLM execution (default: deny all, explicit allowlist)

## External Dependencies

- **Telegram Bot API**: requires a bot token from @BotFather
- **LLM provider CLIs**: must be installed and on `$PATH` in the execution environment
  - `claude` (Anthropic Claude Code CLI)
  - `codex` (OpenAI Codex CLI)
  - `gemini` (Google Gemini CLI)
