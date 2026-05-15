## ADDED Requirements

### Requirement: Centralized Per-Project Storage

The system SHALL store all per-project runtime state under
`~/.tgconn/projects/<encoded-cwd>/`, where `<encoded-cwd>` is the current working
directory with all `/` replaced by `-`. The directory layout MUST be:

```
~/.tgconn/projects/<encoded-cwd>/
├── YYYY-MM-DD.jsonl         # daily audit logs
├── history_<chatID>.jsonl   # per-chat history (prompt injection cache)
├── tmp/<chatID>/            # downloaded media (photos, docs, voice)
└── cron/
    ├── <jobID>.json         # cron job definitions
    └── YYYY-MM-DD.jsonl     # cron execution audit logs
```

The system MUST NOT write any of these files to the current working directory.

#### Scenario: Daily logs go to centralized location
- **WHEN** `tgconn connect` is run from `/Users/alice/myproject`
- **THEN** daily audit logs are written to `~/.tgconn/projects/-Users-alice-myproject/YYYY-MM-DD.jsonl`
- **AND** no `.tgconn/` directory is created under `/Users/alice/myproject`

#### Scenario: Media downloads go to centralized tmp
- **WHEN** a Telegram user sends a photo to a bot running from `/Users/alice/myproject`
- **THEN** the photo is saved under `~/.tgconn/projects/-Users-alice-myproject/tmp/<chatID>/<filename>`

#### Scenario: Cron jobs stored centrally
- **WHEN** `/cron 0 9 * * * 每天九點報告` is issued from a chat
- **THEN** the job definition `<id>.json` is persisted under `~/.tgconn/projects/<encoded>/cron/`

---

### Requirement: Automatic Migration From Legacy Location

The system SHALL detect a legacy `<cwd>/.tgconn/` directory at startup and migrate
its contents to `~/.tgconn/projects/<encoded-cwd>/` exactly once, leaving a
breadcrumb file `<cwd>/.tgconn/MOVED_TO_<encoded>.txt` so the user can trace where
the data went.

#### Scenario: Legacy directory migrated on first startup
- **WHEN** `tgconn connect` is run in a CWD that contains `.tgconn/` from a prior version
- **AND** `~/.tgconn/projects/<encoded-cwd>/` does not yet exist
- **THEN** the contents of `<cwd>/.tgconn/` are moved to `~/.tgconn/projects/<encoded-cwd>/`
- **AND** an empty `<cwd>/.tgconn/` is left containing only `MOVED_TO_<encoded>.txt`
- **AND** the breadcrumb file contains the absolute destination path and a timestamp

#### Scenario: Both locations present aborts startup
- **WHEN** both `<cwd>/.tgconn/` (with real data, not just the breadcrumb) and `~/.tgconn/projects/<encoded-cwd>/` exist at startup
- **THEN** the system logs an error explaining the conflict and exits with a non-zero code
- **AND** no data is moved or overwritten

#### Scenario: Cross-filesystem migration falls back to copy
- **WHEN** the legacy and target directories are on different filesystems and `os.Rename` fails with `EXDEV`
- **THEN** the system copies all files recursively, verifies the copy, and only then removes the source

#### Scenario: No legacy directory means no migration
- **WHEN** `tgconn connect` is run in a CWD without `.tgconn/`
- **THEN** the system skips migration silently and proceeds to use the centralized location

---

### Requirement: Startup Retention Cleanup

The system SHALL run retention cleanup once at startup, based on configurable
thresholds, covering temporary downloads and daily audit logs. History files
follow a separate compaction rule (see *History File Compaction*).

Cleanup MUST run after migration completes and before the bot accepts messages.
Errors during cleanup SHALL be logged but MUST NOT prevent startup.

#### Scenario: Stale tmp files removed
- **WHEN** files under `tmp/` have modification time older than `tmp_retention_hours` (default 24)
- **THEN** those files are deleted at startup
- **AND** empty `tmp/<chatID>/` directories are removed

#### Scenario: Old daily logs removed
- **WHEN** `*.jsonl` files at the storage root or under `cron/` have modification time older than `log_retention_days` (default 30)
- **AND** `log_retention_days` is greater than 0
- **THEN** those files are deleted at startup

#### Scenario: Retention disabled with zero
- **WHEN** `log_retention_days` is set to 0
- **THEN** no daily logs are deleted regardless of age

#### Scenario: History files exempt from log retention
- **WHEN** retention cleanup runs
- **THEN** `history_*.jsonl` files are not touched by log retention (they follow compaction instead)

#### Scenario: Cron job definitions exempt from retention
- **WHEN** retention cleanup runs
- **THEN** `cron/<jobID>.json` files are not deleted regardless of age (managed only via `/cron del`)

---

### Requirement: History File Compaction

The system SHALL compact each `history_<chatID>.jsonl` at startup if it contains
more entries than `history_max_entries` (default 100), keeping only the most
recent N entries. Setting `history_max_entries` to 0 disables compaction.

#### Scenario: Oversized history compacted
- **WHEN** `history_<chatID>.jsonl` contains 500 entries and `history_max_entries` is 100
- **THEN** the file is rewritten in place to contain only the most recent 100 entries
- **AND** the previous 400 entries are discarded from the history file (audit trail remains in daily logs)

#### Scenario: Compaction disabled with zero
- **WHEN** `history_max_entries` is set to 0
- **THEN** no compaction occurs and history files grow without bound

#### Scenario: History below threshold untouched
- **WHEN** `history_<chatID>.jsonl` contains fewer entries than `history_max_entries`
- **THEN** the file is not modified

---

### Requirement: Voice Intermediate File Cleanup

The system SHALL delete transcription intermediate files (`.wav` files produced
for whisper.cpp input, `.txt` files produced by whisper output) immediately after
transcription completes, regardless of success or failure. The original `.ogg`
upload remains in `tmp/` subject to standard tmp retention.

#### Scenario: Successful transcription cleans intermediates
- **WHEN** voice transcription completes successfully
- **THEN** the intermediate `.wav` and `.txt` files are removed
- **AND** the original `.ogg` is left in `tmp/<chatID>/`

#### Scenario: Failed transcription still cleans intermediates
- **WHEN** voice transcription fails partway through
- **THEN** any partially created `.wav` or `.txt` files are removed

---

### Requirement: Manual Cleanup Command

The system SHALL provide a `tgconn clean` subcommand for manual cleanup of
storage with optional dry-run and interactive confirmation.

Supported flags:
- `--tmp`: delete all files under `tmp/`
- `--logs`: delete all `*.jsonl` daily logs (root + `cron/`)
- `--history`: delete `history_*.jsonl` files
- `--all`: delete `--tmp` + `--logs` + `--history` together
- `--dry-run`: list files that would be deleted with total size; do not delete
- `--yes`: skip interactive confirmation

`--history` and `--all` MUST prompt for `yes` confirmation unless `--yes` is set
or `--dry-run` is in effect.

#### Scenario: Clean tmp without confirmation
- **WHEN** the user runs `tgconn clean --tmp`
- **THEN** all files under `tmp/` are deleted immediately
- **AND** the command prints the number of files and total size freed

#### Scenario: Dry-run lists without deleting
- **WHEN** the user runs `tgconn clean --all --dry-run`
- **THEN** the command lists every file that would be deleted and the total size
- **AND** no files are modified or deleted

#### Scenario: History clean requires confirmation
- **WHEN** the user runs `tgconn clean --history` without `--yes`
- **THEN** the command lists the files and prompts for `yes` to proceed
- **AND** anything other than `yes` aborts without deleting

#### Scenario: Yes flag skips confirmation
- **WHEN** the user runs `tgconn clean --all --yes`
- **THEN** all targeted files are deleted without an interactive prompt

#### Scenario: No flags shows usage
- **WHEN** the user runs `tgconn clean` without any flag
- **THEN** the command prints usage help and exits non-zero

---

### Requirement: Storage Configuration

The system SHALL expose three retention settings via the standard config
precedence chain (CLI flag → environment variable → `~/.tgconn/config.yaml`):

| Key | Type | Default | Meaning |
|-----|------|---------|---------|
| `tmp_retention_hours` | int | 24 | Hours before `tmp/` files are eligible for deletion |
| `log_retention_days` | int | 30 | Days before `*.jsonl` daily logs are deleted; 0 disables |
| `history_max_entries` | int | 100 | Max entries per `history_<chatID>.jsonl` before compaction; 0 disables |

#### Scenario: Defaults applied when unset
- **WHEN** no retention values are configured anywhere
- **THEN** the system uses 24 hours, 30 days, and 100 entries respectively

#### Scenario: Config file overrides applied
- **WHEN** `~/.tgconn/config.yaml` sets `log_retention_days: 7`
- **THEN** daily logs older than 7 days are deleted at startup

#### Scenario: Invalid negative value rejected
- **WHEN** any retention setting is negative
- **THEN** the system exits with a non-zero code and a clear error message at startup
