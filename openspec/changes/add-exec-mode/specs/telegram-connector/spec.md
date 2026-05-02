## ADDED Requirements

### Requirement: Execution Mode Selection
The system SHALL support an `--exec-mode` flag on the `connect` command accepting
the values `auto`, `ask`, and `safe`, controlling how Claude handles permission
prompts during execution. The default value is `ask`.

#### Scenario: auto mode bypasses all permission prompts
- **WHEN** `--exec-mode auto` is set
- **THEN** Claude is invoked with `--dangerously-skip-permissions`
- **AND** no Telegram message is sent requesting approval

#### Scenario: ask mode forwards permission prompt to Telegram
- **WHEN** `--exec-mode ask` is set (default)
- **AND** Claude encounters a permission prompt
- **THEN** a Telegram message with ✅ 允許 and ❌ 拒絕 inline buttons is sent to the chat
- **AND** Claude waits for the user to tap a button before continuing

#### Scenario: safe mode auto-allows read-only operations
- **WHEN** `--exec-mode safe` is set
- **AND** Claude requests permission to use a read-only tool (Read / Glob / Grep / LS)
- **THEN** the request is automatically allowed without sending a Telegram message

#### Scenario: safe mode auto-denies dangerous operations
- **WHEN** `--exec-mode safe` is set
- **AND** Claude requests permission to run Bash or mutate a file (Write / Edit / Delete)
- **THEN** the request is automatically denied without sending a Telegram message

---

### Requirement: Exec Mode Validation
The system SHALL reject unknown `--exec-mode` values at startup with a human-readable error.

#### Scenario: invalid exec-mode value
- **WHEN** `--exec-mode turbo` is provided
- **THEN** the process exits before connecting with an error listing valid values

---

### Requirement: Exec Mode Shown in Startup Broadcast
The bot's startup broadcast message SHALL include the active exec-mode alongside
the provider name.

#### Scenario: startup broadcast includes exec-mode
- **WHEN** the bot connects with `--provider claude --exec-mode safe`
- **THEN** the broadcast message contains both `claude` and `safe`
