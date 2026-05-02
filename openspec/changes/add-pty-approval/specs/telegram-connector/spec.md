## MODIFIED Requirements

### Requirement: Ask Mode Interactive Approval via Telegram
In `ask` execution mode the system SHALL forward each Claude permission prompt to the
Telegram chat as an inline keyboard message and pause execution until the user allows
or denies the operation.

#### Scenario: approval prompt forwarded to Telegram
- **WHEN** exec-mode is `ask`
- **AND** Claude outputs a permission prompt (e.g. "Allow Bash to run: git status?")
- **THEN** a Telegram message is sent with ✅ 允許 and ❌ 拒絕 inline keyboard buttons
- **AND** Claude execution is paused until the user taps a button

#### Scenario: user allows the operation
- **WHEN** the user taps ✅ 允許 on the approval keyboard
- **THEN** Claude receives "y" and continues execution
- **AND** the keyboard message is updated to show "已允許 ✅"

#### Scenario: user denies the operation
- **WHEN** the user taps ❌ 拒絕 on the approval keyboard
- **THEN** Claude receives "n" and skips the operation
- **AND** the keyboard message is updated to show "已拒絕 ❌"

#### Scenario: approval times out
- **WHEN** no button is tapped within 5 minutes
- **THEN** the operation is automatically denied
- **AND** the keyboard message is updated to show "已逾時，自動拒絕"

#### Scenario: multiple permission prompts in one session
- **WHEN** Claude requests permission more than once in a single execution
- **THEN** each prompt results in a separate Telegram keyboard message
- **AND** each is handled independently without interfering with the others
