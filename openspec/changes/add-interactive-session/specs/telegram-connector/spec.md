## ADDED Requirements

### Requirement: Interactive Session Mode
The system SHALL support per-chat interactive sessions where a persistent Claude
process is kept alive across multiple Telegram messages, enabling true back-and-forth
conversation without re-spawning Claude on each message.

#### Scenario: user starts an interactive session
- **WHEN** an authorized chat sends `/session start`
- **THEN** a Claude process is started with stream-json I/O
- **AND** the bot replies confirming the session is active

#### Scenario: message routed to active session
- **WHEN** a chat has an active session
- **AND** the user sends a plain text message
- **THEN** the message is forwarded to the running Claude process
- **AND** Claude's response is sent back to Telegram when complete

#### Scenario: no active session falls back to one-shot
- **WHEN** a chat has no active session
- **AND** the user sends a plain text message
- **THEN** the existing one-shot provider execution runs unchanged

#### Scenario: user ends a session
- **WHEN** an authorized chat sends `/session end`
- **AND** a session is active
- **THEN** the Claude process is terminated gracefully
- **AND** the bot replies confirming the session has ended

---

### Requirement: Session Idle Timeout
The system SHALL automatically close sessions that have been idle for more than
30 minutes and notify the user.

#### Scenario: idle session auto-closed
- **WHEN** a session receives no messages for 30 minutes
- **THEN** the session is closed automatically
- **AND** a Telegram message is sent informing the user the session has timed out

---

### Requirement: Session Status Command
The system SHALL provide a `/session status` command showing the state of the
current chat's session.

#### Scenario: active session status
- **WHEN** `/session status` is sent with an active session
- **THEN** the bot replies with session start time, duration, and message count

#### Scenario: no active session status
- **WHEN** `/session status` is sent with no active session
- **THEN** the bot replies indicating no session is running
