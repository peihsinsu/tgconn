## ADDED Requirements

### Requirement: Bot Startup
The system SHALL start a Telegram bot long-polling loop when `tgconn connect` is executed with a valid bot token and at least one allowed chat ID.

#### Scenario: Successful startup
- **WHEN** `tgconn --provider claude connect` is run with `TELEGRAM_BOT_TOKEN` set and `--allow-chat` provided
- **THEN** the bot connects to Telegram, logs "connected" with the bot username, and begins polling for messages

#### Scenario: Missing token fails fast
- **WHEN** `tgconn connect` is run without a bot token (no env var, no config)
- **THEN** the process exits with a non-zero code and prints a human-readable error before attempting to connect

#### Scenario: Provider binary not found fails fast
- **WHEN** `tgconn --provider claude connect` is run but `claude` is not on `$PATH`
- **THEN** the process exits with a non-zero code and prints which binary is missing

---

### Requirement: Message Forwarding to LLM Provider
The system SHALL execute the configured LLM provider CLI as a subprocess in the current working directory for each incoming Telegram message, passing the message text as the question.

#### Scenario: Successful provider call
- **WHEN** an authorized chat sends the message "list files in this directory"
- **THEN** the system runs `claude -p "list files in this directory" --dangerously-skip-permissions` from CWD and captures stdout

#### Scenario: Provider execution timeout
- **WHEN** the provider subprocess does not complete within the configured timeout (default 120s)
- **THEN** the subprocess is killed and an error message is sent back to the Telegram chat

#### Scenario: Provider returns non-zero exit code
- **WHEN** the provider subprocess exits with a non-zero code
- **THEN** stderr is captured and an error message is sent back to the chat

---

### Requirement: Response Delivery
The system SHALL send the LLM provider output back to the originating Telegram chat, splitting into multiple messages if the output exceeds Telegram's 4096-character limit.

#### Scenario: Short response
- **WHEN** provider output is ≤ 4096 characters
- **THEN** a single Telegram message is sent with the full output

#### Scenario: Long response split
- **WHEN** provider output exceeds 4096 characters
- **THEN** the output is split into sequential messages of ≤ 4096 characters each, sent in order

---

### Requirement: Provider Configuration
The system SHALL support selecting the LLM provider via `--provider` CLI flag, environment variable, or config file, with CLI flag taking highest priority.

#### Scenario: Provider flag overrides config
- **WHEN** config file sets `provider: codex` but `--provider claude` is passed at runtime
- **THEN** the claude provider is used

#### Scenario: Unsupported provider rejected
- **WHEN** `--provider unknown` is passed
- **THEN** the process exits with a non-zero code listing supported providers

---

### Requirement: Chat ID Access Control
The system SHALL only process messages from explicitly allowed Telegram chat IDs; all other messages MUST be silently ignored.

#### Scenario: Allowed chat processed
- **WHEN** a message arrives from a chat ID in the `--allow-chat` list
- **THEN** the message is forwarded to the LLM provider

#### Scenario: Unauthorized chat ignored
- **WHEN** a message arrives from a chat ID not in the allowlist
- **THEN** the message is silently dropped (no response sent, debug log emitted)

#### Scenario: No allowlist means deny all
- **WHEN** `tgconn connect` is run without `--allow-chat` or config equivalent
- **THEN** the process exits with a non-zero code requiring at least one allowed chat to be configured

---

### Requirement: Debug Mode
The system SHALL emit verbose logs of all inbound messages, provider invocations, and outbound responses when `--debug` is set.

#### Scenario: Debug flag enables verbose logging
- **WHEN** `tgconn --provider claude connect --debug` is run
- **THEN** each received message, the full subprocess command, and the truncated response are printed to stderr

#### Scenario: Normal mode suppresses verbose output
- **WHEN** `--debug` is not set
- **THEN** only startup, shutdown, and error events are logged

---

### Requirement: Configuration Management
The system SHALL provide `tgconn config init` and `tgconn config show` subcommands for managing persistent configuration stored at `~/.tgconn/config.yaml`.

#### Scenario: Config init creates file
- **WHEN** `tgconn config init` is run interactively
- **THEN** the user is prompted for bot token, default provider, and allowed chat IDs, and the values are written to `~/.tgconn/config.yaml`

#### Scenario: Config show prints active config
- **WHEN** `tgconn config show` is run
- **THEN** the resolved configuration (after merging file + env + flags) is printed, with the token value masked

---

### Requirement: Graceful Shutdown
The system SHALL handle SIGINT and SIGTERM by stopping the Telegram polling loop and waiting for any in-flight provider subprocess to complete or timeout before exiting.

#### Scenario: Ctrl-C triggers clean exit
- **WHEN** the user sends SIGINT while no provider call is in progress
- **THEN** the bot disconnects and the process exits with code 0

#### Scenario: In-flight call completes before exit
- **WHEN** SIGINT arrives while a provider subprocess is running
- **THEN** the system waits up to 10s for the subprocess to finish, then force-kills it and exits
