package i18n

// Messages holds every user-facing string sent by the bot.
// All strings that contain format verbs (%s, %d, %v) must be used with fmt.Sprintf.
type Messages struct {
	// ── Bot lifecycle ────────────────────────────────────────────────────────
	Connected    string // args: provider, mode
	Disconnecting string

	// ── Job status ───────────────────────────────────────────────────────────
	JobACK      string // args: id, id
	JobProgress string // args: id, elapsed
	JobStopped  string // args: id, elapsed
	JobFailed   string // args: id, elapsed, err
	JobDone     string // args: id, elapsed
	JobLimitHit  string // args: max
	CronLimitHit string // args: max

	// ── /cron ────────────────────────────────────────────────────────────────
	CronLabel      string // args: prompt
	CronSkipped    string // args: jobID, err
	CronStarted    string // args: id
	CronFailed     string // args: id, elapsed, err
	CronDone       string // args: id, elapsed
	CronDelUsage   string
	CronDelFailed  string // args: err
	CronDeleted    string // args: jobID
	CronAddFailed  string // args: err
	CronCreated    string // args: ID, expr, prompt
	CronEmpty      string
	CronListHeader string // args: count
	CronListItem   string // args: ID, expr, prompt, nextRun, ID

	// ── /session ─────────────────────────────────────────────────────────────
	SessionUsage       string
	SessionAlreadyOpen string // args: uptime
	SessionStarting    string
	SessionStartFailed string // args: err
	SessionOpened      string
	SessionNoActive    string
	SessionEnded       string // args: msgCount
	SessionNoStatus    string
	SessionStatus      string // args: startedAt, uptime, msgCount
	SessionTextOnly    string
	SessionError       string // args: err
	SessionIdleTimeout string // args: idleTimeout

	// ── Media types (used in LLM prompts) ────────────────────────────────────
	MediaTypeImage string
	MediaTypeFile  string

	// ── Media messages ───────────────────────────────────────────────────────
	MediaUnsupportedVideo  string
	MediaUnsupportedAudio  string
	MediaUnsupportedAnim   string
	MediaSticker           string // args: emoji (may be empty string)
	MediaVoiceDisabled     string
	MediaVoiceTranscribing string
	MediaVoiceFailed       string // args: err
	MediaVoiceResult       string // args: text
	MediaPhotoDisplay      string
	MediaPhotoCaption      string // args: caption
	MediaDocDisplay        string // args: filename
	MediaDocCaption        string // args: filename, caption
	MediaFileTooLarge      string
	MediaDownloadFailed    string // args: err

	// ── Media LLM prompt fragments ────────────────────────────────────────────
	MediaPromptSent    string // args: mediaType, filename
	MediaPromptSavedTo string // args: localPath
	MediaPromptCaption string // args: caption
	MediaPromptTask    string // args: mediaType
	MediaPromptAsk     string // args: mediaType

	// ── History LLM prompt context (injected into the prompt sent to the LLM) ─
	HistoryCtxHeader string
	HistoryCtxEntry  string // args: question, answer
	HistoryCtxTail   string // args: question

	// ── /history command ─────────────────────────────────────────────────────
	HistoryDisabled  string
	HistoryLoadError string // args: err
	HistoryEmpty     string
	HistoryHeader    string // args: count

	// ── /status command ──────────────────────────────────────────────────────
	StatusRunning    string
	StatusBot        string // args: username
	StatusHost       string // args: hostname
	StatusStartedAt  string // args: time
	StatusUptime     string // args: duration
	StatusProvider   string // args: provider
	StatusNoJobs     string
	StatusJobsHeader string // args: count
	StatusJobLine    string // args: id, from, preview, elapsed

	// ── /list command ────────────────────────────────────────────────────────
	ListEmpty  string
	ListHeader string // args: count
	ListLine   string // args: id, from, preview, elapsed

	// ── /stop command ────────────────────────────────────────────────────────
	StopUsage    string
	StopBadID    string // args: arg
	StopNotFound string // args: id
	StopSent     string // args: id

	// ── Approval (ask mode) ──────────────────────────────────────────────────
	ApprovalPrompt  string // args: displayPrompt
	ApprovalAllow   string
	ApprovalDeny    string
	ApprovalTimeout string
	ApprovalAllowed string
	ApprovalDenied  string

	// ── /help ────────────────────────────────────────────────────────────────
	Help string
}

var en = &Messages{
	Connected:    "✅ Connected — provider: %s  mode: %s",
	Disconnecting: "🔌 Bot is shutting down",

	JobACK:      "⚙️ Processing #%d — will notify you when done. Cancel: /stop %d",
	JobProgress: "⏳ #%d still running (%s elapsed)...",
	JobStopped:  "🛑 #%d stopped (ran for %s)",
	JobFailed:   "❌ #%d failed (%s): %v",
	JobDone:     "✅ #%d done (took %s)",
	JobLimitHit:  "Max concurrent jobs reached (%d) — please wait or /stop an existing job",
	CronLimitHit: "Max concurrent cron jobs reached (%d)",

	CronLabel:      "[cron] %s",
	CronSkipped:    "⏰ Cron %s skipped: %v",
	CronStarted:    "⏰ Cron job #%d started...",
	CronFailed:     "❌ Cron job #%d failed (%s): %v",
	CronDone:       "✅ Cron job #%d done (took %s)",
	CronDelUsage:   "Usage: /cron del <id>",
	CronDelFailed:  "❌ Delete failed: %v",
	CronDeleted:    "🗑 Cron job %s deleted",
	CronAddFailed:  "❌ Add failed: %v",
	CronCreated:    "✅ Cron job created\nID: %s\nexpr: %s\nprompt: %s",
	CronEmpty:      "📋 No cron jobs.\nUse /cron <expr> <prompt> to add one.",
	CronListHeader: "📋 Cron jobs (%d):\n",
	CronListItem:   "\n🆔 %s\n  expr: %s\n  prompt: %s\n  next: %s\n  delete: /cron del %s",

	SessionUsage:       "Usage: /session start | end | status",
	SessionAlreadyOpen: "⚡ Session already active (running %s).\nUse /session end to close it first.",
	SessionStarting:    "⏳ Starting interactive session...",
	SessionStartFailed: "❌ Start failed: %v",
	SessionOpened:      "✅ Interactive session opened — just send messages to chat.\nUse /session end to close.",
	SessionNoActive:    "❌ No active session.",
	SessionEnded:       "🔌 Session ended (%d messages).",
	SessionNoStatus:    "📭 No active session.\nUse /session start to begin.",
	SessionStatus:      "⚡ Session active\n⏰ Started: %s\n⌛ Uptime: %s\n💬 Messages: %d",
	SessionTextOnly:    "Interactive session supports text only.",
	SessionError:       "❌ Session error — closed automatically: %v",
	SessionIdleTimeout: "⏰ Session idle for %s — closed automatically.",

	MediaTypeImage: "image",
	MediaTypeFile:  "file",

	MediaUnsupportedVideo:  "Video is not supported — please send text or a screenshot instead.",
	MediaUnsupportedAudio:  "Audio files are not supported — use a voice message with --enable-voice, or send text.",
	MediaUnsupportedAnim:   "GIF/animation is not supported — please send text.",
	MediaSticker:           "Got a sticker%s, but I can only handle text, images, and files!",
	MediaVoiceDisabled:     "Voice messages are not enabled. Send text instead, or restart tgconn with --enable-voice.",
	MediaVoiceTranscribing: "🎙️ Transcribing voice message...",
	MediaVoiceFailed:       "Voice transcription failed: %v",
	MediaVoiceResult:       "🎙️ Transcription:\n%s",
	MediaPhotoDisplay:      "📷 Image",
	MediaPhotoCaption:      "📷 Image: %s",
	MediaDocDisplay:        "📎 %s",
	MediaDocCaption:        "📎 %s: %s",
	MediaFileTooLarge:      "❌ File exceeds the 20 MB limit",
	MediaDownloadFailed:    "❌ Download failed: %v",

	MediaPromptSent:    "User sent a %s: %s",
	MediaPromptSavedTo: "Downloaded to: %s",
	MediaPromptCaption: "Caption: %s",
	MediaPromptTask:    "Please complete the task based on the caption and the %s above.",
	MediaPromptAsk:     "How would you like to handle this %s?",

	HistoryCtxHeader: "Here is our previous conversation — use it as context for the new question:\n\n",
	HistoryCtxEntry:  "[User]: %s\n[Assistant]: %s\n\n",
	HistoryCtxTail:   "---\nNew question:\n%s",

	HistoryDisabled:  "❌ History is not enabled",
	HistoryLoadError: "❌ Failed to load history: %v",
	HistoryEmpty:     "📭 No conversation history yet.",
	HistoryHeader:    "📖 Last %d conversation(s):\n\n",

	StatusRunning:    "🟢 tgconn running\n\n",
	StatusBot:        "🤖 Bot: @%s\n",
	StatusHost:       "🖥  Host: %s\n",
	StatusStartedAt:  "⏰ Started: %s\n",
	StatusUptime:     "⌛ Uptime: %s\n",
	StatusProvider:   "🔧 Provider: %s\n",
	StatusNoJobs:     "\n📋 Active jobs: (none)",
	StatusJobsHeader: "\n📋 Active jobs (%d):\n",
	StatusJobLine:    "  • #%d %s — %q — running for %s\n",

	ListEmpty:  "📋 No active jobs.",
	ListHeader: "📋 Active jobs (%d):\n",
	ListLine:   "• #%d %s — %q — running for %s\n",

	StopUsage:    "Usage: /stop <job ID>, e.g. /stop 1",
	StopBadID:    "❌ Invalid job ID: %q",
	StopNotFound: "❌ Job #%d not found (may have already finished)",
	StopSent:     "🛑 Stop request sent for job #%d",

	ApprovalPrompt:  "🔐 Claude needs permission:\n\n%s",
	ApprovalAllow:   "✅ Allow",
	ApprovalDeny:    "❌ Deny",
	ApprovalTimeout: "⏰ Timed out — auto-denied",
	ApprovalAllowed: "Allowed ✅",
	ApprovalDenied:  "Denied ❌",

	Help: `📖 tgconn commands:

Any text — forwarded to the LLM; ACKed immediately, result pushed when done
📷 Image / 📎 File — downloaded and analysed by the LLM
🎙️ Voice — transcribed then forwarded (requires --enable-voice)

/cron <expr> <prompt> — add a cron job (5-field cron or @daily etc.)
/cron list — list cron jobs
/cron del <id> — delete a cron job
/? or /help — show this help
/list — list running jobs
/stop <id> — stop a running job
/status — bot status (uptime, provider, active jobs)
/history — recent conversation history

/session start — start an interactive session (Claude keeps conversation context natively)
/session end — end the session
/session status — session info (uptime, message count)`,
}

var zhTW = &Messages{
	Connected:    "✅ 已連線，provider: %s  模式: %s",
	Disconnecting: "🔌 Bot 即將關閉，已斷線",

	JobACK:      "⚙️ 處理中 #%d，完成後通知你。取消：/stop %d",
	JobProgress: "⏳ #%d 仍在執行中（已等 %s）...",
	JobStopped:  "🛑 #%d 已停止（執行了 %s）",
	JobFailed:   "❌ #%d 失敗（%s）：%v",
	JobDone:     "✅ #%d 完成（耗時 %s）",
	JobLimitHit:  "已達同時執行上限（%d），請稍後再試或 /stop 取消現有任務",
	CronLimitHit: "已達排程同時執行上限（%d）",

	CronLabel:      "[排程] %s",
	CronSkipped:    "⏰ 排程 %s 跳過：%v",
	CronStarted:    "⏰ 排程任務 #%d 開始執行...",
	CronFailed:     "❌ 排程任務 #%d 失敗（%s）：%v",
	CronDone:       "✅ 排程任務 #%d 完成（耗時 %s）",
	CronDelUsage:   "用法：/cron del <id>",
	CronDelFailed:  "❌ 刪除失敗：%v",
	CronDeleted:    "🗑 排程 %s 已刪除",
	CronAddFailed:  "❌ 新增失敗：%v",
	CronCreated:    "✅ 排程已建立\nID：%s\nexpr：%s\nprompt：%s",
	CronEmpty:      "📋 目前沒有排程任務。\n用 /cron <expr> <prompt> 新增。",
	CronListHeader: "📋 排程任務（%d 個）：\n",
	CronListItem:   "\n🆔 %s\n  expr：%s\n  prompt：%s\n  下次執行：%s\n  刪除：/cron del %s",

	SessionUsage:       "用法：/session start | end | status",
	SessionAlreadyOpen: "⚡ 已有活躍的互動 session（運行 %s）。\n請先 /session end 結束。",
	SessionStarting:    "⏳ 正在啟動互動 session...",
	SessionStartFailed: "❌ 啟動失敗：%v",
	SessionOpened:      "✅ 互動 session 已開啟，直接傳訊息即可對話。\n輸入 /session end 結束。",
	SessionNoActive:    "❌ 目前沒有活躍的互動 session。",
	SessionEnded:       "🔌 互動 session 已結束（共 %d 則訊息）。",
	SessionNoStatus:    "📭 目前沒有活躍的互動 session。\n輸入 /session start 開啟。",
	SessionStatus:      "⚡ 互動 session 進行中\n⏰ 啟動時間：%s\n⌛ 已運行：%s\n💬 訊息數：%d",
	SessionTextOnly:    "互動 session 目前只支援文字訊息。",
	SessionError:       "❌ Session 發生錯誤，已自動關閉：%v",
	SessionIdleTimeout: "⏰ 互動 session 閒置超過 %s，已自動關閉。",

	MediaTypeImage: "圖片",
	MediaTypeFile:  "檔案",

	MediaUnsupportedVideo:  "影片目前不支援，請改用文字或傳送截圖",
	MediaUnsupportedAudio:  "音訊檔目前不支援，如需語音轉文字請啟用 --enable-voice 並傳送語音訊息",
	MediaUnsupportedAnim:   "GIF/動畫目前不支援，請改用文字",
	MediaSticker:           "收到貼圖%s，但我只能處理文字、圖片和檔案喔！",
	MediaVoiceDisabled:     "語音訊息目前未啟用，請以文字傳送指令（或使用 --enable-voice 啟動 tgconn）",
	MediaVoiceTranscribing: "🎙️ 正在轉錄語音...",
	MediaVoiceFailed:       "語音轉錄失敗：%v",
	MediaVoiceResult:       "🎙️ 識別結果：\n%s",
	MediaPhotoDisplay:      "📷 圖片",
	MediaPhotoCaption:      "📷 圖片：%s",
	MediaDocDisplay:        "📎 %s",
	MediaDocCaption:        "📎 %s：%s",
	MediaFileTooLarge:      "❌ 檔案超過 20 MB 限制，無法下載",
	MediaDownloadFailed:    "❌ 檔案下載失敗：%v",

	MediaPromptSent:    "使用者傳送了一個%s：%s",
	MediaPromptSavedTo: "已下載至：%s",
	MediaPromptCaption: "說明：%s",
	MediaPromptTask:    "請根據上述說明與%s完成任務。",
	MediaPromptAsk:     "請問你想如何處理這個%s？",

	HistoryCtxHeader: "以下是我們之前的對話記錄，請根據此背景回答新問題：\n\n",
	HistoryCtxEntry:  "[User]: %s\n[Assistant]: %s\n\n",
	HistoryCtxTail:   "---\n新問題：\n%s",

	HistoryDisabled:  "❌ 歷史記錄功能未啟用",
	HistoryLoadError: "❌ 無法讀取歷史記錄：%v",
	HistoryEmpty:     "📭 目前沒有對話記錄。",
	HistoryHeader:    "📖 最近 %d 筆對話記錄：\n\n",

	StatusRunning:    "🟢 tgconn 運行中\n\n",
	StatusBot:        "🤖 Bot：@%s\n",
	StatusHost:       "🖥  主機：%s\n",
	StatusStartedAt:  "⏰ 啟動時間：%s\n",
	StatusUptime:     "⌛ 已運行：%s\n",
	StatusProvider:   "🔧 Provider：%s\n",
	StatusNoJobs:     "\n📋 執行中的指令：（無）",
	StatusJobsHeader: "\n📋 執行中的指令（%d）：\n",
	StatusJobLine:    "  • #%d %s — %q — 已執行 %s\n",

	ListEmpty:  "📋 目前沒有執行中的指令。",
	ListHeader: "📋 執行中的指令（%d）：\n",
	ListLine:   "• #%d %s — %q — 已執行 %s\n",

	StopUsage:    "用法：/stop <指令編號>，例如 /stop 1",
	StopBadID:    "❌ 無效的指令編號：%q",
	StopNotFound: "❌ 找不到指令 #%d（可能已完成）",
	StopSent:     "🛑 已送出停止請求給指令 #%d",

	ApprovalPrompt:  "🔐 Claude 需要執行授權：\n\n%s",
	ApprovalAllow:   "✅ 允許",
	ApprovalDeny:    "❌ 拒絕",
	ApprovalTimeout: "⏰ 已逾時，自動拒絕",
	ApprovalAllowed: "已允許 ✅",
	ApprovalDenied:  "已拒絕 ❌",

	Help: `📖 tgconn 支援的指令：

任意文字 — 轉發給 LLM，立即 ACK，完成後推送結果
📷 圖片 / 📎 檔案 — 下載後讓 LLM 分析
🎙️ 語音訊息 — 轉錄為文字後送出（需 --enable-voice）

/cron <expr> <prompt> — 新增排程任務（標準 5 欄位 cron 或 @daily 等）
/cron list — 列出排程任務
/cron del <id> — 刪除排程任務
/? 或 /help — 顯示此說明
/list — 列出執行中的指令
/stop <id> — 停止指定的執行中指令
/status — 顯示 bot 狀態（uptime、provider、執行中指令）
/history — 顯示最近對話記錄

/session start — 開啟互動 session（Claude 原生記住對話脈絡）
/session end — 結束互動 session
/session status — 查看 session 狀態（uptime、訊息數）`,
}

// Get returns the Messages for the given language tag, falling back to English.
func Get(lang string) *Messages {
	switch lang {
	case "zh-TW", "zh_TW", "zh":
		return zhTW
	default:
		return en
	}
}
