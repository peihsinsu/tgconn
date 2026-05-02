package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/cx009/tgconn/internal/config"
	"github.com/cx009/tgconn/internal/downloader"
	"github.com/cx009/tgconn/internal/provider"
	"github.com/cx009/tgconn/internal/recorder"
	"github.com/cx009/tgconn/internal/session"
	"github.com/cx009/tgconn/internal/transcriber"
)

const approvalTimeout = 5 * time.Minute

const maxMessageLen = 4096

type job struct {
	id        int
	chatID    int64
	from      string
	preview   string
	startedAt time.Time
	cancel    context.CancelFunc
	stopped   bool
}

type pendingApproval struct {
	prompt string
	ch     chan bool
}

type Bot struct {
	api        *tgbotapi.BotAPI
	provider   provider.Provider
	config     *config.Config
	recorder   *recorder.Recorder
	dl         *downloader.Downloader
	jobsMu     sync.Mutex
	jobs       map[int]*job
	nextJobID  int
	startTime  time.Time
	hostname   string

	approvalMu       sync.Mutex
	pendingApprovals map[string]*pendingApproval // key: callbackData prefix (approval_<id>)
	approvalSeq      int

	sessionsMu sync.Mutex
	sessions   map[int64]*session.Session
}

func New(cfg *config.Config, p provider.Provider, rec *recorder.Recorder) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Telegram: %w", err)
	}
	slog.Info("telegram API connection established", "bot_username", api.Self.UserName)
	hostname, _ := os.Hostname()
	return &Bot{
		api:              api,
		provider:         p,
		config:           cfg,
		recorder:         rec,
		dl:               downloader.New(api, ".tgconn/tmp"),
		jobs:             make(map[int]*job),
		startTime:        time.Now(),
		hostname:         hostname,
		pendingApprovals: make(map[string]*pendingApproval),
		sessions:         make(map[int64]*session.Session),
	}, nil
}

func (b *Bot) Start(ctx context.Context) error {
	slog.Info("bot starting",
		"username", b.api.Self.UserName,
		"provider", b.config.Provider,
		"allowed_chats", b.config.AllowedChats,
		"timeout", b.config.Timeout,
	)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := b.api.GetUpdatesChan(u)

	slog.Info("polling for updates", "poll_timeout_sec", u.Timeout)
	b.broadcast(fmt.Sprintf("✅ 已連線，provider: %s  模式: %s", b.config.Provider, b.config.ExecMode))

	go b.sessionIdleSweep(ctx)

	var wg sync.WaitGroup

loop:
	for {
		select {
		case <-ctx.Done():
			slog.Info("shutdown signal received, stopping update polling")
			b.broadcast("🔌 Bot 即將關閉，已斷線")
			b.api.StopReceivingUpdates()
			break loop
		case update, ok := <-updates:
			if !ok {
				slog.Info("update channel closed")
				break loop
			}
			if update.CallbackQuery != nil {
				b.handleCallbackQuery(update.CallbackQuery)
				continue
			}
			if update.Message == nil {
				slog.Debug("skipping non-message update", "update_id", update.UpdateID)
				continue
			}
			msg := update.Message
			fromUser := senderName(msg)
			if !b.config.IsAllowed(msg.Chat.ID) {
				slog.Warn("message from unauthorized chat — ignored",
					"chat_id", msg.Chat.ID,
					"from", fromUser,
					"update_id", update.UpdateID,
				)
				continue
			}
			slog.Debug("dispatching message",
				"chat_id", msg.Chat.ID,
				"from", fromUser,
				"message_id", msg.MessageID,
				"text_runes", utf8.RuneCountInString(msg.Text),
			)
			wg.Add(1)
			go func(msg *tgbotapi.Message) {
				defer wg.Done()
				b.handleMessage(ctx, msg)
			}(msg)
		}
	}

	slog.Info("waiting for in-flight provider calls to finish")
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		slog.Info("all in-flight calls finished, shutdown clean")
	case <-time.After(10 * time.Second):
		slog.Warn("timed out waiting for in-flight calls, forcing exit")
	}
	return nil
}

const waitingNoticeDelay = 30 * time.Second
const waitingNoticeMsg = "⏳ 正在處理，請稍候..."
const progressNoticeInterval = 3 * time.Minute

func (b *Bot) handleMessage(ctx context.Context, msg *tgbotapi.Message) {
	// /session commands are intercepted before everything else.
	if msg.Text != "" && strings.HasPrefix(msg.Text, "/session") {
		b.cmdSession(ctx, msg)
		return
	}

	// Active session: route all messages directly into the Claude process.
	b.sessionsMu.Lock()
	sess, hasSession := b.sessions[msg.Chat.ID]
	b.sessionsMu.Unlock()
	if hasSession && !sess.IsClosed() {
		b.handleSessionMessage(ctx, msg, sess)
		return
	}

	// Media type dispatch — handle before text check.
	switch {
	case msg.Sticker != nil:
		b.handleSticker(msg)
		return
	case msg.Video != nil:
		b.handleUnsupportedMedia(msg.Chat.ID, "影片目前不支援，請改用文字或傳送截圖")
		return
	case msg.Audio != nil:
		b.handleUnsupportedMedia(msg.Chat.ID, "音訊檔目前不支援，如需語音轉文字請啟用 --enable-voice 並傳送語音訊息")
		return
	case msg.Animation != nil:
		b.handleUnsupportedMedia(msg.Chat.ID, "GIF/動畫目前不支援，請改用文字")
		return
	case msg.Voice != nil:
		b.handleVoice(ctx, msg)
		return
	case msg.Photo != nil:
		b.handlePhoto(ctx, msg)
		return
	case msg.Document != nil:
		b.handleDocument(ctx, msg)
		return
	}

	question := msg.Text
	if question == "" {
		slog.Debug("ignoring empty message", "chat_id", msg.Chat.ID, "message_id", msg.MessageID)
		return
	}

	// Intercept built-in commands before forwarding to LLM.
	if question == "/?" || question == "/help" || strings.HasPrefix(question, "/? ") || strings.HasPrefix(question, "/help ") {
		b.cmdHelp(msg.Chat.ID)
		return
	}
	if strings.HasPrefix(question, "/list") && (len(question) == 5 || question[5] == ' ') {
		b.cmdList(msg.Chat.ID)
		return
	}
	if strings.HasPrefix(question, "/stop") && (len(question) == 5 || question[5] == ' ') {
		b.cmdStop(msg.Chat.ID, strings.TrimSpace(question[5:]))
		return
	}
	if strings.HasPrefix(question, "/status") && (len(question) == 7 || question[7] == ' ') {
		b.cmdStatus(msg.Chat.ID)
		return
	}
	if strings.HasPrefix(question, "/history") && (len(question) == 8 || question[8] == ' ') {
		b.cmdHistory(msg.Chat.ID)
		return
	}

	b.executeAndReply(ctx, msg, question, question)
}

// executeAndReply runs the LLM provider and sends the result back to the chat.
// displayQuestion is stored in the job tracker and recorder (human-readable description).
// rawPrompt is the content sent to the LLM; history context is injected inside this function.
func (b *Bot) executeAndReply(ctx context.Context, msg *tgbotapi.Message, displayQuestion, rawPrompt string) {
	fromUser := senderName(msg)

	slog.Info("handling message",
		"chat_id", msg.Chat.ID,
		"from", fromUser,
		"message_id", msg.MessageID,
		"question_runes", utf8.RuneCountInString(displayQuestion),
	)
	slog.Debug("prompt text", "chat_id", msg.Chat.ID, "raw_prompt", rawPrompt)

	callCtx, cancel := context.WithTimeout(ctx, b.config.Timeout)
	callCtx = provider.WithExecMode(callCtx, b.config.ExecMode)
	if b.config.ExecMode == config.ExecModeAsk {
		callCtx = provider.WithApproval(callCtx, b.makeApprovalFunc(msg.Chat.ID))
	}
	j := b.addJob(msg.Chat.ID, fromUser, displayQuestion, cancel)
	defer func() {
		cancel()
		b.removeJob(j.id)
	}()

	type result struct {
		response string
		err      error
	}
	resultCh := make(chan result, 1)
	start := time.Now()

	prompt := b.buildPrompt(msg.Chat.ID, rawPrompt)

	slog.Info("invoking provider",
		"provider", b.config.Provider,
		"chat_id", msg.Chat.ID,
		"job_id", j.id,
		"timeout", b.config.Timeout,
		"history_injected", prompt != rawPrompt,
	)
	go func() {
		resp, err := b.provider.Execute(callCtx, prompt)
		resultCh <- result{resp, err}
	}()

	waitTimer := time.NewTimer(waitingNoticeDelay)
	defer waitTimer.Stop()

	var res result
	select {
	case res = <-resultCh:
	case <-waitTimer.C:
		slog.Info("provider slow — sending waiting notice",
			"chat_id", msg.Chat.ID,
			"job_id", j.id,
			"elapsed", time.Since(start).Round(time.Second),
		)
		b.send(msg.Chat.ID, waitingNoticeMsg)
		ticker := time.NewTicker(progressNoticeInterval)
		defer ticker.Stop()
		waiting := true
		for waiting {
			select {
			case res = <-resultCh:
				waiting = false
			case <-ticker.C:
				elapsed := time.Since(start).Round(time.Second)
				slog.Info("provider still running — sending progress notice",
					"chat_id", msg.Chat.ID,
					"job_id", j.id,
					"elapsed", elapsed,
				)
				b.send(msg.Chat.ID, fmt.Sprintf("⏳ 仍在執行中（已等 %s）...", elapsed))
			}
		}
	}

	elapsed := time.Since(start).Round(time.Millisecond)

	b.jobsMu.Lock()
	wasStopped := j.stopped
	b.jobsMu.Unlock()

	if res.err != nil {
		if wasStopped {
			slog.Info("job stopped by user", "chat_id", msg.Chat.ID, "job_id", j.id, "elapsed", elapsed)
			b.send(msg.Chat.ID, fmt.Sprintf("🛑 指令 #%d 已停止（執行了 %s）", j.id, elapsed))
			b.record(recorder.Entry{
				Time: start, ChatID: msg.Chat.ID, From: fromUser,
				Question: displayQuestion, Error: "stopped by user", ElapsedMs: elapsed.Milliseconds(),
			})
			return
		}
		slog.Error("provider error",
			"chat_id", msg.Chat.ID, "from", fromUser, "job_id", j.id, "elapsed", elapsed, "error", res.err,
		)
		b.send(msg.Chat.ID, fmt.Sprintf("error: %v", res.err))
		b.record(recorder.Entry{
			Time: start, ChatID: msg.Chat.ID, From: fromUser,
			Question: displayQuestion, Error: res.err.Error(), ElapsedMs: elapsed.Milliseconds(),
		})
		return
	}

	chunks := splitMessage(res.response)
	slog.Info("provider response ready",
		"chat_id", msg.Chat.ID, "job_id", j.id, "elapsed", elapsed,
		"response_runes", utf8.RuneCountInString(res.response), "chunks", len(chunks),
	)

	for i, chunk := range chunks {
		if len(chunks) > 1 {
			b.send(msg.Chat.ID, fmt.Sprintf("[%d/%d]\n%s", i+1, len(chunks), chunk))
		} else {
			b.send(msg.Chat.ID, chunk)
		}
	}

	b.record(recorder.Entry{
		Time: start, ChatID: msg.Chat.ID, From: fromUser,
		Question: displayQuestion, Response: res.response, ElapsedMs: elapsed.Milliseconds(),
	})

	slog.Info("message handled",
		"chat_id", msg.Chat.ID, "message_id", msg.MessageID, "job_id", j.id, "elapsed", elapsed,
	)
}

// ── Interactive session ───────────────────────────────────────────────────────

func (b *Bot) cmdSession(ctx context.Context, msg *tgbotapi.Message) {
	parts := strings.Fields(msg.Text)
	sub := ""
	if len(parts) >= 2 {
		sub = parts[1]
	}
	switch sub {
	case "start":
		b.cmdSessionStart(ctx, msg)
	case "end", "stop":
		b.cmdSessionEnd(msg.Chat.ID)
	case "status":
		b.cmdSessionStatus(msg.Chat.ID)
	default:
		b.send(msg.Chat.ID, "用法：/session start | end | status")
	}
}

func (b *Bot) cmdSessionStart(ctx context.Context, msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	b.sessionsMu.Lock()
	if s, ok := b.sessions[chatID]; ok && !s.IsClosed() {
		b.sessionsMu.Unlock()
		uptime := time.Since(s.StartedAt).Round(time.Second)
		b.send(chatID, fmt.Sprintf("⚡ 已有活躍的互動 session（運行 %s）。\n請先 /session end 結束。", uptime))
		return
	}
	sess := session.New(chatID)
	b.sessions[chatID] = sess
	b.sessionsMu.Unlock()

	b.send(chatID, "⏳ 正在啟動互動 session...")
	var sessionApprovalFn provider.ApprovalFunc
	if b.config.ExecMode == config.ExecModeAsk {
		sessionApprovalFn = b.makeApprovalFunc(chatID)
	}
	if err := sess.Start(ctx, b.config.ExecMode, sessionApprovalFn); err != nil {
		b.sessionsMu.Lock()
		delete(b.sessions, chatID)
		b.sessionsMu.Unlock()
		b.send(chatID, fmt.Sprintf("❌ 啟動失敗：%v", err))
		return
	}
	b.send(chatID, "✅ 互動 session 已開啟，直接傳訊息即可對話。\n輸入 /session end 結束。")
}

func (b *Bot) cmdSessionEnd(chatID int64) {
	b.sessionsMu.Lock()
	sess, ok := b.sessions[chatID]
	if ok {
		delete(b.sessions, chatID)
	}
	b.sessionsMu.Unlock()

	if !ok {
		b.send(chatID, "❌ 目前沒有活躍的互動 session。")
		return
	}
	_ = sess.Close()
	b.send(chatID, fmt.Sprintf("🔌 互動 session 已結束（共 %d 則訊息）。", sess.MsgCount))
}

func (b *Bot) cmdSessionStatus(chatID int64) {
	b.sessionsMu.Lock()
	sess, ok := b.sessions[chatID]
	b.sessionsMu.Unlock()

	if !ok || sess.IsClosed() {
		b.send(chatID, "📭 目前沒有活躍的互動 session。\n輸入 /session start 開啟。")
		return
	}
	uptime := time.Since(sess.StartedAt).Round(time.Second)
	b.send(chatID, fmt.Sprintf("⚡ 互動 session 進行中\n⏰ 啟動時間：%s\n⌛ 已運行：%s\n💬 訊息數：%d",
		sess.StartedAt.Format("15:04:05"), uptime, sess.MsgCount))
}

func (b *Bot) handleSessionMessage(ctx context.Context, msg *tgbotapi.Message, sess *session.Session) {
	text := msg.Text
	if text == "" {
		b.send(msg.Chat.ID, "互動 session 目前只支援文字訊息。")
		return
	}
	slog.Info("session: forwarding message",
		"chat_id", msg.Chat.ID, "runes", utf8.RuneCountInString(text))

	callCtx, cancel := context.WithTimeout(ctx, b.config.Timeout)
	defer cancel()

	waitTimer := time.NewTimer(waitingNoticeDelay)
	defer waitTimer.Stop()

	type result struct {
		text string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		t, err := sess.Send(callCtx, text)
		ch <- result{t, err}
	}()

	var res result
	select {
	case res = <-ch:
	case <-waitTimer.C:
		b.send(msg.Chat.ID, waitingNoticeMsg)
		res = <-ch
	}

	if res.err != nil {
		slog.Error("session send error", "chat_id", msg.Chat.ID, "error", res.err)
		// Session is likely broken; close and remove it.
		b.sessionsMu.Lock()
		delete(b.sessions, msg.Chat.ID)
		b.sessionsMu.Unlock()
		_ = sess.Close()
		b.send(msg.Chat.ID, fmt.Sprintf("❌ Session 發生錯誤，已自動關閉：%v", res.err))
		return
	}

	chunks := splitMessage(res.text)
	for i, chunk := range chunks {
		if len(chunks) > 1 {
			b.send(msg.Chat.ID, fmt.Sprintf("[%d/%d]\n%s", i+1, len(chunks), chunk))
		} else {
			b.send(msg.Chat.ID, chunk)
		}
	}
}

// sessionIdleSweep runs in a goroutine and closes sessions idle beyond IdleTimeout.
func (b *Bot) sessionIdleSweep(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.sessionsMu.Lock()
			var idle []int64
			for chatID, s := range b.sessions {
				if s.IsIdle() || s.IsClosed() {
					idle = append(idle, chatID)
				}
			}
			for _, chatID := range idle {
				sess := b.sessions[chatID]
				delete(b.sessions, chatID)
				_ = sess.Close()
				slog.Info("session: idle timeout, closing", "chat_id", chatID)
				b.send(chatID, fmt.Sprintf("⏰ 互動 session 閒置超過 %s，已自動關閉。", session.IdleTimeout))
			}
			b.sessionsMu.Unlock()
		}
	}
}

func (b *Bot) record(e recorder.Entry) {
	if b.recorder == nil {
		return
	}
	if err := b.recorder.Log(e); err != nil {
		slog.Warn("failed to write log entry", "error", err)
	}
}

func (b *Bot) addJob(chatID int64, from, question string, cancel context.CancelFunc) *job {
	preview := question
	if r := []rune(question); len(r) > 60 {
		preview = string(r[:60]) + "…"
	}
	b.jobsMu.Lock()
	defer b.jobsMu.Unlock()
	b.nextJobID++
	j := &job{
		id:        b.nextJobID,
		chatID:    chatID,
		from:      from,
		preview:   preview,
		startedAt: time.Now(),
		cancel:    cancel,
	}
	b.jobs[j.id] = j
	slog.Debug("job registered", "job_id", j.id, "chat_id", chatID, "from", from)
	return j
}

func (b *Bot) removeJob(id int) {
	b.jobsMu.Lock()
	defer b.jobsMu.Unlock()
	delete(b.jobs, id)
	slog.Debug("job unregistered", "job_id", id)
}

func (b *Bot) cmdHelp(chatID int64) {
	text := `📖 tgconn 支援的指令：

任意文字 — 轉發給 LLM 並回傳回應
📷 圖片 / 📎 檔案 — 下載後讓 LLM 分析
🎙️ 語音訊息 — 轉錄為文字後送出（需 --enable-voice）

/? 或 /help — 顯示此說明
/list — 列出執行中的指令
/stop <id> — 停止指定的執行中指令
/status — 顯示 bot 狀態（uptime、provider、執行中指令）
/history — 顯示最近對話記錄

/session start — 開啟互動 session（Claude 原生記住對話脈絡）
/session end — 結束互動 session
/session status — 查看 session 狀態（uptime、訊息數）`
	b.send(chatID, text)
}

func (b *Bot) cmdList(chatID int64) {
	b.jobsMu.Lock()
	snapshot := make([]*job, 0, len(b.jobs))
	for _, j := range b.jobs {
		snapshot = append(snapshot, j)
	}
	b.jobsMu.Unlock()

	if len(snapshot) == 0 {
		b.send(chatID, "📋 目前沒有執行中的指令。")
		return
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "📋 執行中的指令（%d）：\n", len(snapshot))
	for _, j := range snapshot {
		elapsed := time.Since(j.startedAt).Round(time.Second)
		fmt.Fprintf(&sb, "• #%d %s — %q — 已執行 %s\n", j.id, j.from, j.preview, elapsed)
	}
	b.send(chatID, strings.TrimRight(sb.String(), "\n"))
}

func (b *Bot) cmdStop(chatID int64, arg string) {
	if arg == "" {
		b.send(chatID, "用法：/stop <指令編號>，例如 /stop 1")
		return
	}
	id, err := strconv.Atoi(arg)
	if err != nil || id <= 0 {
		b.send(chatID, fmt.Sprintf("❌ 無效的指令編號：%q", arg))
		return
	}

	b.jobsMu.Lock()
	j, ok := b.jobs[id]
	if ok {
		j.stopped = true
	}
	b.jobsMu.Unlock()

	if !ok {
		b.send(chatID, fmt.Sprintf("❌ 找不到指令 #%d（可能已完成）", id))
		return
	}

	j.cancel()
	slog.Info("job stop requested", "job_id", id, "chat_id", chatID)
	b.send(chatID, fmt.Sprintf("🛑 已送出停止請求給指令 #%d", id))
}

// ── Media handlers ────────────────────────────────────────────────────────────

func (b *Bot) handleSticker(msg *tgbotapi.Message) {
	emoji := ""
	if msg.Sticker != nil && msg.Sticker.Emoji != "" {
		emoji = " " + msg.Sticker.Emoji
	}
	slog.Debug("sticker received", "chat_id", msg.Chat.ID, "emoji", emoji)
	b.send(msg.Chat.ID, fmt.Sprintf("收到貼圖%s，但我只能處理文字、圖片和檔案喔！", emoji))
}

func (b *Bot) handleUnsupportedMedia(chatID int64, msg string) {
	slog.Debug("unsupported media received", "chat_id", chatID)
	b.send(chatID, msg)
}

func (b *Bot) handlePhoto(ctx context.Context, msg *tgbotapi.Message) {
	if len(msg.Photo) == 0 {
		return
	}
	// Use highest resolution (last element).
	photo := msg.Photo[len(msg.Photo)-1]
	filename := fmt.Sprintf("%d.jpg", msg.MessageID)

	slog.Info("photo received, downloading", "chat_id", msg.Chat.ID, "message_id", msg.MessageID, "size", photo.FileSize)
	localPath, err := b.dl.Download(ctx, photo.FileID, photo.FileSize, msg.Chat.ID, filename)
	if err != nil {
		b.send(msg.Chat.ID, mediaDownloadError(err))
		return
	}

	caption := msg.Caption
	displayQuestion := "📷 圖片"
	if caption != "" {
		displayQuestion = fmt.Sprintf("📷 圖片：%s", caption)
	}
	rawPrompt := buildMediaPrompt("圖片", localPath, filename, caption)
	b.executeAndReply(ctx, msg, displayQuestion, rawPrompt)
}

func (b *Bot) handleDocument(ctx context.Context, msg *tgbotapi.Message) {
	doc := msg.Document
	filename := doc.FileName
	if filename == "" {
		filename = fmt.Sprintf("file_%d", msg.MessageID)
	}

	slog.Info("document received, downloading", "chat_id", msg.Chat.ID, "filename", filename, "size", doc.FileSize)
	localPath, err := b.dl.Download(ctx, doc.FileID, doc.FileSize, msg.Chat.ID, filename)
	if err != nil {
		b.send(msg.Chat.ID, mediaDownloadError(err))
		return
	}

	caption := msg.Caption
	displayQuestion := fmt.Sprintf("📎 %s", filename)
	if caption != "" {
		displayQuestion = fmt.Sprintf("📎 %s：%s", filename, caption)
	}
	rawPrompt := buildMediaPrompt("檔案", localPath, filename, caption)
	b.executeAndReply(ctx, msg, displayQuestion, rawPrompt)
}

func (b *Bot) handleVoice(ctx context.Context, msg *tgbotapi.Message) {
	if !b.config.EnableVoice {
		b.send(msg.Chat.ID, "語音訊息目前未啟用，請以文字傳送指令（或使用 --enable-voice 啟動 tgconn）")
		return
	}

	filename := fmt.Sprintf("voice_%d.ogg", msg.MessageID)
	slog.Info("voice received, downloading", "chat_id", msg.Chat.ID, "message_id", msg.MessageID)
	localPath, err := b.dl.Download(ctx, msg.Voice.FileID, msg.Voice.FileSize, msg.Chat.ID, filename)
	if err != nil {
		b.send(msg.Chat.ID, mediaDownloadError(err))
		return
	}

	slog.Info("transcribing voice", "chat_id", msg.Chat.ID, "path", localPath)
	b.send(msg.Chat.ID, "🎙️ 正在轉錄語音...")
	text, err := transcriber.Transcribe(ctx, localPath)
	if err != nil {
		slog.Error("whisper transcription failed", "chat_id", msg.Chat.ID, "error", err)
		b.send(msg.Chat.ID, fmt.Sprintf("語音轉錄失敗：%v", err))
		return
	}

	slog.Info("voice transcribed", "chat_id", msg.Chat.ID, "runes", utf8.RuneCountInString(text))
	b.executeAndReply(ctx, msg, fmt.Sprintf("🎙️ %s", text), text)
}

// buildMediaPrompt formats a prompt that includes file context for the LLM.
func buildMediaPrompt(mediaType, localPath, filename, caption string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "使用者傳送了一個%s：%s\n", mediaType, filename)
	fmt.Fprintf(&sb, "已下載至：%s\n", localPath)
	if caption != "" {
		fmt.Fprintf(&sb, "說明：%s\n", caption)
		fmt.Fprintf(&sb, "\n請根據上述說明與%s完成任務。", mediaType)
	} else {
		fmt.Fprintf(&sb, "\n請問你想如何處理這個%s？", mediaType)
	}
	return sb.String()
}

func mediaDownloadError(err error) string {
	if errors.Is(err, downloader.ErrFileTooLarge) {
		return "❌ 檔案超過 20 MB 限制，無法下載"
	}
	return fmt.Sprintf("❌ 檔案下載失敗：%v", err)
}

// ── Context / history ─────────────────────────────────────────────────────────

func (b *Bot) buildPrompt(chatID int64, question string) string {
	if b.recorder == nil || b.config.HistorySize <= 0 {
		return question
	}
	history, err := b.recorder.LoadRecent(chatID, b.config.HistorySize)
	if err != nil {
		slog.Warn("failed to load chat history", "chat_id", chatID, "error", err)
		return question
	}
	if len(history) == 0 {
		return question
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "以下是我們之前的對話記錄，請根據此背景回答新問題：\n\n")
	for _, ex := range history {
		fmt.Fprintf(&sb, "[User]: %s\n[Assistant]: %s\n\n", ex.Question, ex.Answer)
	}
	fmt.Fprintf(&sb, "---\n新問題：\n%s", question)

	slog.Debug("prompt with history built",
		"chat_id", chatID,
		"history_count", len(history),
		"prompt_runes", utf8.RuneCountInString(sb.String()),
	)
	return sb.String()
}

func (b *Bot) cmdHistory(chatID int64) {
	if b.recorder == nil {
		b.send(chatID, "❌ 歷史記錄功能未啟用")
		return
	}
	history, err := b.recorder.LoadRecent(chatID, b.config.HistorySize)
	if err != nil {
		b.send(chatID, fmt.Sprintf("❌ 無法讀取歷史記錄：%v", err))
		return
	}
	if len(history) == 0 {
		b.send(chatID, "📭 目前沒有對話記錄。")
		return
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "📖 最近 %d 筆對話記錄：\n\n", len(history))
	for i, ex := range history {
		ts := ex.Time.Format("01/02 15:04")
		qPreview := []rune(ex.Question)
		if len(qPreview) > 60 {
			qPreview = append(qPreview[:60], '…')
		}
		aPreview := []rune(ex.Answer)
		if len(aPreview) > 80 {
			aPreview = append(aPreview[:80], '…')
		}
		fmt.Fprintf(&sb, "#%d [%s]\nQ: %s\nA: %s\n\n", i+1, ts, string(qPreview), string(aPreview))
	}
	b.send(chatID, strings.TrimRight(sb.String(), "\n"))
}

func (b *Bot) cmdStatus(chatID int64) {
	b.jobsMu.Lock()
	activeJobs := make([]*job, 0, len(b.jobs))
	for _, j := range b.jobs {
		activeJobs = append(activeJobs, j)
	}
	b.jobsMu.Unlock()

	uptime := time.Since(b.startTime).Round(time.Second)
	startedAt := b.startTime.Format("2006-01-02 15:04:05")

	var sb strings.Builder
	fmt.Fprintf(&sb, "🟢 tgconn 運行中\n\n")
	fmt.Fprintf(&sb, "🤖 Bot：@%s\n", b.api.Self.UserName)
	fmt.Fprintf(&sb, "🖥  主機：%s\n", b.hostname)
	fmt.Fprintf(&sb, "⏰ 啟動時間：%s\n", startedAt)
	fmt.Fprintf(&sb, "⌛ 已運行：%s\n", uptime)
	fmt.Fprintf(&sb, "🔧 Provider：%s\n", b.config.Provider)

	if len(activeJobs) == 0 {
		fmt.Fprintf(&sb, "\n📋 執行中的指令：（無）")
	} else {
		fmt.Fprintf(&sb, "\n📋 執行中的指令（%d）：\n", len(activeJobs))
		for _, j := range activeJobs {
			elapsed := time.Since(j.startedAt).Round(time.Second)
			fmt.Fprintf(&sb, "  • #%d %s — %q — 已執行 %s\n", j.id, j.from, j.preview, elapsed)
		}
	}

	b.send(chatID, strings.TrimRight(sb.String(), "\n"))
}

func (b *Bot) send(chatID int64, text string) {
	slog.Debug("sending telegram message",
		"chat_id", chatID,
		"text_runes", utf8.RuneCountInString(text),
		"text_preview", text[:min(80, len(text))],
	)
	if _, err := b.api.Send(tgbotapi.NewMessage(chatID, text)); err != nil {
		slog.Error("failed to send message", "chat_id", chatID, "error", err)
	}
}

func (b *Bot) broadcast(text string) {
	if len(b.config.AllowedChats) == 0 {
		slog.Debug("broadcast skipped — no allowed_chats configured")
		return
	}
	slog.Info("broadcasting to allowed chats", "count", len(b.config.AllowedChats), "text", text)
	for _, chatID := range b.config.AllowedChats {
		b.send(chatID, text)
	}
}

// makeApprovalFunc returns a provider.ApprovalFunc that sends an inline keyboard
// to chatID and blocks until the user taps Allow or Deny (or the context expires).
func (b *Bot) makeApprovalFunc(chatID int64) provider.ApprovalFunc {
	return func(ctx context.Context, prompt string) (bool, error) {
		b.approvalMu.Lock()
		b.approvalSeq++
		key := fmt.Sprintf("ap_%d", b.approvalSeq)
		pa := &pendingApproval{
			prompt: prompt,
			ch:     make(chan bool, 1),
		}
		b.pendingApprovals[key] = pa
		b.approvalMu.Unlock()

		defer func() {
			b.approvalMu.Lock()
			delete(b.pendingApprovals, key)
			b.approvalMu.Unlock()
		}()

		// Trim prompt for display.
		displayPrompt := prompt
		if r := []rune(displayPrompt); len(r) > 400 {
			displayPrompt = string(r[:400]) + "…"
		}
		text := fmt.Sprintf("🔐 Claude 需要執行授權：\n\n%s", displayPrompt)

		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("✅ 允許", key+"_y"),
				tgbotapi.NewInlineKeyboardButtonData("❌ 拒絕", key+"_n"),
			),
		)
		m := tgbotapi.NewMessage(chatID, text)
		m.ReplyMarkup = keyboard
		sent, err := b.api.Send(m)
		if err != nil {
			slog.Error("failed to send approval keyboard", "chat_id", chatID, "error", err)
			return false, err
		}

		timer := time.NewTimer(approvalTimeout)
		defer timer.Stop()

		var allowed bool
		select {
		case allowed = <-pa.ch:
		case <-timer.C:
			slog.Warn("approval timed out", "chat_id", chatID, "key", key)
			// Edit the message to show it expired.
			edit := tgbotapi.NewEditMessageText(chatID, sent.MessageID,
				text+"\n\n⏰ 已逾時，自動拒絕")
			_, _ = b.api.Send(edit)
			return false, fmt.Errorf("approval timed out")
		case <-ctx.Done():
			return false, ctx.Err()
		}

		label := "已拒絕 ❌"
		if allowed {
			label = "已允許 ✅"
		}
		edit := tgbotapi.NewEditMessageText(chatID, sent.MessageID,
			text+"\n\n"+label)
		_, _ = b.api.Send(edit)

		return allowed, nil
	}
}

// handleCallbackQuery resolves a pending approval when the user taps Allow/Deny.
func (b *Bot) handleCallbackQuery(cq *tgbotapi.CallbackQuery) {
	// Acknowledge the callback immediately so Telegram stops the loading spinner.
	cb := tgbotapi.NewCallback(cq.ID, "")
	_, _ = b.api.Request(cb)

	data := cq.Data
	if !strings.HasPrefix(data, "ap_") {
		return
	}

	var key string
	var allowed bool
	if k, ok := strings.CutSuffix(data, "_y"); ok {
		key = k
		allowed = true
	} else if k, ok := strings.CutSuffix(data, "_n"); ok {
		key = k
		allowed = false
	} else {
		return
	}

	b.approvalMu.Lock()
	pa, ok := b.pendingApprovals[key]
	b.approvalMu.Unlock()

	if !ok {
		slog.Warn("approval callback for unknown key", "key", key)
		return
	}

	select {
	case pa.ch <- allowed:
	default:
	}
}

func senderName(msg *tgbotapi.Message) string {
	if msg.From == nil {
		return "<unknown>"
	}
	if msg.From.UserName != "" {
		return "@" + msg.From.UserName
	}
	return fmt.Sprintf("%s %s", msg.From.FirstName, msg.From.LastName)
}

func splitMessage(text string) []string {
	if utf8.RuneCountInString(text) <= maxMessageLen {
		return []string{text}
	}
	runes := []rune(text)
	var chunks []string
	for len(runes) > 0 {
		end := min(maxMessageLen, len(runes))
		chunks = append(chunks, string(runes[:end]))
		runes = runes[end:]
	}
	return chunks
}
