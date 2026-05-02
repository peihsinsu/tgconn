## ADDED Requirements

### Requirement: Sticker Handling
The system SHALL respond with a friendly unsupported message when a Telegram sticker is received, including the sticker's associated emoji if available.

#### Scenario: Sticker received with emoji
- **WHEN** an authorized chat sends a sticker with emoji "😂"
- **THEN** the bot replies "收到貼圖 😂，但我只能處理文字、圖片和檔案喔！"

#### Scenario: Sticker received without emoji
- **WHEN** an authorized chat sends a sticker with no emoji metadata
- **THEN** the bot replies with a generic unsupported message

---

### Requirement: Caption Extraction
The system SHALL use the Caption field of a photo, document, video, or audio message as the question when no download is required or as supplementary context when a file is downloaded.

#### Scenario: Document with caption only
- **WHEN** a document is sent with caption "幫我整理這份 log"
- **THEN** the caption is used as the question text forwarded to the LLM provider (in addition to the file path)

#### Scenario: Photo with caption, no download needed
- **WHEN** a photo is sent with a non-empty caption
- **THEN** the caption is extracted and used as the question; the image is also downloaded and its path injected into the prompt

---

### Requirement: File Download
The system SHALL download Document messages from Telegram to `.tgconn/tmp/<chat_id>/` and inject the local file path and original filename into the LLM prompt.

#### Scenario: Successful document download
- **WHEN** an authorized chat sends a file `report.csv` (≤ 20 MB)
- **THEN** the file is saved to `.tgconn/tmp/<chat_id>/report.csv` and the prompt includes the path and filename

#### Scenario: File exceeds size limit
- **WHEN** a file larger than 20 MB is sent
- **THEN** the bot replies with an error message explaining the size limit, without attempting download

#### Scenario: Download failure
- **WHEN** the Telegram file download fails due to a network error
- **THEN** the bot replies with an error message; no LLM call is made

#### Scenario: File with no caption
- **WHEN** a file is sent with no caption
- **THEN** the prompt instructs the LLM that a file has been received and asks what to do with it

---

### Requirement: Photo Download
The system SHALL download the highest-resolution version of a Photo message to `.tgconn/tmp/<chat_id>/` and inject the local path into the LLM prompt.

#### Scenario: Photo downloaded successfully
- **WHEN** an authorized chat sends a photo (with or without caption)
- **THEN** the highest-resolution image is saved as `<message_id>.jpg` and its path is injected into the prompt

#### Scenario: Photo with caption
- **WHEN** a photo is sent with caption "這個 error 怎麼解"
- **THEN** the prompt includes both the image path and the caption text

---

### Requirement: Voice Transcription
The system SHALL transcribe Voice messages to text using the `whisper` CLI and forward the transcription to the LLM provider, when voice support is enabled via `--enable-voice`.

#### Scenario: Voice transcribed successfully
- **WHEN** `--enable-voice` is set and an authorized chat sends a voice message
- **THEN** the voice file is downloaded, `whisper` is called, and the transcribed text is forwarded to the LLM as the question

#### Scenario: Voice support disabled
- **WHEN** `--enable-voice` is not set and a voice message is received
- **THEN** the bot replies "語音訊息目前未啟用，請以文字傳送指令"

#### Scenario: whisper binary not found
- **WHEN** `--enable-voice` is set but `whisper` is not on `$PATH`
- **THEN** the process exits at startup with a clear error message indicating whisper is required

#### Scenario: whisper transcription fails
- **WHEN** whisper exits with a non-zero code
- **THEN** the bot replies with an error; the original voice file is preserved for debugging

---

### Requirement: Unsupported Media Types
The system SHALL reply with a friendly unsupported message for Video, Audio (music), and Animation (GIF) messages.

#### Scenario: Video received
- **WHEN** an authorized chat sends a video
- **THEN** the bot replies "影片目前不支援，請改用文字或傳送截圖"

#### Scenario: Audio (music file) received
- **WHEN** an authorized chat sends an audio file (music)
- **THEN** the bot replies "音訊檔目前不支援，如需語音轉文字請啟用 --enable-voice 並傳送語音訊息"
