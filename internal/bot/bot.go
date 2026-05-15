package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/cx009/tgconn/internal/config"
	"github.com/cx009/tgconn/internal/cronjob"
	"github.com/cx009/tgconn/internal/downloader"
	"github.com/cx009/tgconn/internal/i18n"
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
	isCron    bool
}

type pendingApproval struct {
	prompt string
	ch     chan bool
}

type Bot struct {
	api        *tgbotapi.BotAPI
	provider   provider.Provider
	config     *config.Config
	msgs       *i18n.Messages
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

	cronMgr    *cronjob.Manager
	cronCtx    context.Context
	cronCancel context.CancelFunc
}

// New constructs a Bot. projectDir is the per-project storage root
// (~/.tgconn/projects/<encoded-cwd>/); tmp downloads and cron jobs live below it.
func New(cfg *config.Config, p provider.Provider, rec *recorder.Recorder, projectDir string) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Telegram: %w", err)
	}
	slog.Info("telegram API connection established", "bot_username", api.Self.UserName)
	hostname, _ := os.Hostname()

	cronCtx, cronCancel := context.WithCancel(context.Background())
	b := &Bot{
		api:              api,
		provider:         p,
		config:           cfg,
		msgs:             i18n.Get(cfg.Language),
		recorder:         rec,
		dl:               downloader.New(api, filepath.Join(projectDir, "tmp")),
		jobs:             make(map[int]*job),
		startTime:        time.Now(),
		hostname:         hostname,
		pendingApprovals: make(map[string]*pendingApproval),
		sessions:         make(map[int64]*session.Session),
		cronCtx:          cronCtx,
		cronCancel:       cronCancel,
	}

	mgr, err := cronjob.New(filepath.Join(projectDir, "cron"), b.triggerCronJob)
	if err != nil {
		cronCancel()
		return nil, fmt.Errorf("failed to init cron manager: %w", err)
	}
	b.cronMgr = mgr
	return b, nil
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
	b.broadcast(fmt.Sprintf(b.msgs.Connected, b.config.Provider, b.config.ExecMode))

	b.cronMgr.Start()
	defer func() {
		b.cronMgr.Stop()
		b.cronCancel()
	}()

	go b.sessionIdleSweep(ctx)

	var wg sync.WaitGroup

loop:
	for {
		select {
		case <-ctx.Done():
			slog.Info("shutdown signal received, stopping update polling")
			b.broadcast(b.msgs.Disconnecting)
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

const progressNoticeInterval = 3 * time.Minute

func (b *Bot) handleMessage(ctx context.Context, msg *tgbotapi.Message) {
	// /session commands are intercepted before everything else.
	if msg.Text != "" && strings.HasPrefix(msg.Text, "/session") {
		b.cmdSession(ctx, msg)
		return
	}

	// In group/supergroup chats, only respond when the bot is explicitly mentioned
	// or when the message is a direct reply to the bot.
	if isGroupChat(msg) && !b.botAddressed(msg) {
		slog.Debug("group message not addressed to bot — ignoring",
			"chat_id", msg.Chat.ID, "message_id", msg.MessageID)
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
		b.handleUnsupportedMedia(msg.Chat.ID, b.msgs.MediaUnsupportedVideo)
		return
	case msg.Audio != nil:
		b.handleUnsupportedMedia(msg.Chat.ID, b.msgs.MediaUnsupportedAudio)
		return
	case msg.Animation != nil:
		b.handleUnsupportedMedia(msg.Chat.ID, b.msgs.MediaUnsupportedAnim)
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

	// Strip @botusername from the question so the LLM doesn't see the mention noise.
	// Also normalises "/cmd@botusername" → "/cmd" for group commands.
	question := stripBotMention(msg.Text, b.api.Self.UserName, msg.Entities)
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
	if strings.HasPrefix(question, "/cron") && (len(question) == 5 || question[5] == ' ') {
		b.cmdCron(msg, strings.TrimSpace(question[5:]))
		return
	}

	b.executeAndReply(ctx, msg, question, question)
}

// executeAndReply runs the LLM provider and sends the result back to the chat.
// Immediately ACKs with a job ID, then pushes the result when done.
// displayQuestion is stored in the job tracker and recorder (human-readable description).
// rawPrompt is the content sent to the LLM; history context is injected inside this function.
func (b *Bot) executeAndReply(ctx context.Context, msg *tgbotapi.Message, displayQuestion, rawPrompt string) {
	fromUser := senderName(msg)
	chatID := msg.Chat.ID

	slog.Info("handling message",
		"chat_id", chatID,
		"from", fromUser,
		"message_id", msg.MessageID,
		"question_runes", utf8.RuneCountInString(displayQuestion),
	)
	slog.Debug("prompt text", "chat_id", chatID, "raw_prompt", rawPrompt)

	callCtx, cancel := context.WithTimeout(ctx, b.config.Timeout)
	callCtx = provider.WithExecMode(callCtx, b.config.ExecMode)
	if b.config.ExecMode == config.ExecModeAsk {
		callCtx = provider.WithApproval(callCtx, b.makeApprovalFunc(chatID))
	}
	j, err := b.addJob(chatID, fromUser, displayQuestion, false, cancel)
	if err != nil {
		cancel()
		b.send(chatID, fmt.Sprintf("❌ %v", err))
		return
	}
	defer func() {
		cancel()
		b.removeJob(j.id)
	}()

	b.send(chatID, fmt.Sprintf(b.msgs.JobACK, j.id, j.id))

	type result struct {
		response string
		err      error
	}
	resultCh := make(chan result, 1)
	start := time.Now()

	prompt := b.buildPrompt(chatID, rawPrompt)
	slog.Info("invoking provider",
		"provider", b.config.Provider,
		"chat_id", chatID,
		"job_id", j.id,
		"timeout", b.config.Timeout,
		"history_injected", prompt != rawPrompt,
	)
	go func() {
		resp, err := b.provider.Execute(callCtx, prompt)
		resultCh <- result{resp, err}
	}()

	ticker := time.NewTicker(progressNoticeInterval)
	defer ticker.Stop()

	var res result
	for {
		select {
		case res = <-resultCh:
			goto done
		case <-ticker.C:
			elapsed := time.Since(start).Round(time.Second)
			slog.Info("provider still running — sending progress notice",
				"chat_id", chatID, "job_id", j.id, "elapsed", elapsed,
			)
			b.send(chatID, fmt.Sprintf(b.msgs.JobProgress, j.id, elapsed))
		}
	}
done:
	elapsed := time.Since(start).Round(time.Millisecond)

	b.jobsMu.Lock()
	wasStopped := j.stopped
	b.jobsMu.Unlock()

	if res.err != nil {
		if wasStopped {
			slog.Info("job stopped by user", "chat_id", chatID, "job_id", j.id, "elapsed", elapsed)
			b.send(chatID, fmt.Sprintf(b.msgs.JobStopped, j.id, elapsed))
			b.record(recorder.Entry{
				Time: start, ChatID: chatID, From: fromUser,
				Question: displayQuestion, Error: "stopped by user", ElapsedMs: elapsed.Milliseconds(),
			})
			return
		}
		slog.Error("provider error",
			"chat_id", chatID, "from", fromUser, "job_id", j.id, "elapsed", elapsed, "error", res.err,
		)
		b.send(chatID, fmt.Sprintf(b.msgs.JobFailed, j.id, elapsed.Round(time.Second), res.err))
		b.record(recorder.Entry{
			Time: start, ChatID: chatID, From: fromUser,
			Question: displayQuestion, Error: res.err.Error(), ElapsedMs: elapsed.Milliseconds(),
		})
		return
	}

	elapsed = elapsed.Round(time.Second)
	chunks := splitMessage(res.response)
	slog.Info("provider response ready",
		"chat_id", chatID, "job_id", j.id, "elapsed", elapsed,
		"response_runes", utf8.RuneCountInString(res.response), "chunks", len(chunks),
	)

	b.send(chatID, fmt.Sprintf(b.msgs.JobDone, j.id, elapsed))
	for i, chunk := range chunks {
		if len(chunks) > 1 {
			b.send(chatID, fmt.Sprintf("[%d/%d]\n%s", i+1, len(chunks), chunk))
		} else {
			b.send(chatID, chunk)
		}
	}

	b.record(recorder.Entry{
		Time: start, ChatID: chatID, From: fromUser,
		Question: displayQuestion, Response: res.response, ElapsedMs: elapsed.Milliseconds(),
	})
	slog.Info("message handled",
		"chat_id", chatID, "message_id", msg.MessageID, "job_id", j.id, "elapsed", elapsed,
	)
}

// ── Cron jobs ─────────────────────────────────────────────────────────────────

// triggerCronJob is the callback fired by the cron scheduler.
// Runs unattended: uses auto exec mode (no approval prompts).
func (b *Bot) triggerCronJob(chatID int64, jobID, expr, prompt string) {
	callCtx, cancel := context.WithTimeout(b.cronCtx, b.config.Timeout)
	callCtx = provider.WithExecMode(callCtx, config.ExecModeAuto)

	j, err := b.addJob(chatID, "cron", fmt.Sprintf(b.msgs.CronLabel, prompt), true, cancel)
	if err != nil {
		cancel()
		slog.Warn("cron job skipped due to rate limit", "cron_id", jobID, "error", err)
		b.send(chatID, fmt.Sprintf(b.msgs.CronSkipped, jobID, err))
		return
	}
	b.send(chatID, fmt.Sprintf(b.msgs.CronStarted, j.id))
	slog.Info("cron job triggered", "chat_id", chatID, "job_id", j.id, "cron_id", jobID)

	go func() {
		defer func() {
			cancel()
			b.removeJob(j.id)
		}()

		start := time.Now()
		resp, err := b.provider.Execute(callCtx, prompt)
		elapsed := time.Since(start).Round(time.Second)

		entry := recorder.CronEntry{
			Time:      start,
			ChatID:    chatID,
			JobID:     jobID,
			Expr:      expr,
			Prompt:    prompt,
			ElapsedMs: elapsed.Milliseconds(),
		}

		if err != nil {
			slog.Error("cron job failed", "job_id", j.id, "cron_id", jobID, "elapsed", elapsed, "error", err)
			b.send(chatID, fmt.Sprintf(b.msgs.CronFailed, j.id, elapsed, err))
			entry.Error = err.Error()
			b.recordCron(entry)
			return
		}

		slog.Info("cron job completed", "job_id", j.id, "cron_id", jobID, "elapsed", elapsed)
		b.send(chatID, fmt.Sprintf(b.msgs.CronDone, j.id, elapsed))
		chunks := splitMessage(resp)
		for i, chunk := range chunks {
			if len(chunks) > 1 {
				b.send(chatID, fmt.Sprintf("[%d/%d]\n%s", i+1, len(chunks), chunk))
			} else {
				b.send(chatID, chunk)
			}
		}
		entry.Response = resp
		b.recordCron(entry)
	}()
}

// cmdCron dispatches /cron subcommands.
func (b *Bot) cmdCron(msg *tgbotapi.Message, args string) {
	chatID := msg.Chat.ID

	switch {
	case args == "list" || args == "":
		b.cmdCronList(chatID)
	case strings.HasPrefix(args, "del "):
		id := strings.TrimSpace(args[4:])
		if id == "" {
			b.send(chatID, b.msgs.CronDelUsage)
			return
		}
		if err := b.cronMgr.Delete(id); err != nil {
			b.send(chatID, fmt.Sprintf(b.msgs.CronDelFailed, err))
			return
		}
		b.send(chatID, fmt.Sprintf(b.msgs.CronDeleted, id))
	default:
		expr, prompt, err := cronjob.ParseArgs(args)
		if err != nil {
			b.send(chatID, fmt.Sprintf("❌ %v", err))
			return
		}
		j, err := b.cronMgr.Add(chatID, expr, prompt)
		if err != nil {
			b.send(chatID, fmt.Sprintf(b.msgs.CronAddFailed, err))
			return
		}
		b.send(chatID, fmt.Sprintf(b.msgs.CronCreated, j.ID, j.Expr, j.Prompt))
	}
}

func (b *Bot) cmdCronList(chatID int64) {
	jobs := b.cronMgr.List(chatID)
	if len(jobs) == 0 {
		b.send(chatID, b.msgs.CronEmpty)
		return
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, b.msgs.CronListHeader, len(jobs))
	for _, ji := range jobs {
		fmt.Fprintf(&sb, b.msgs.CronListItem,
			ji.ID, ji.Expr, ji.Prompt,
			ji.NextRun.Local().Format("01/02 15:04:05"),
			ji.ID,
		)
	}
	b.send(chatID, sb.String())
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
		b.send(msg.Chat.ID, b.msgs.SessionUsage)
	}
}

func (b *Bot) cmdSessionStart(ctx context.Context, msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	b.sessionsMu.Lock()
	if s, ok := b.sessions[chatID]; ok && !s.IsClosed() {
		b.sessionsMu.Unlock()
		uptime := time.Since(s.StartedAt).Round(time.Second)
		b.send(chatID, fmt.Sprintf(b.msgs.SessionAlreadyOpen, uptime))
		return
	}
	sess := session.New(chatID)
	b.sessions[chatID] = sess
	b.sessionsMu.Unlock()

	b.send(chatID, b.msgs.SessionStarting)
	var sessionApprovalFn provider.ApprovalFunc
	if b.config.ExecMode == config.ExecModeAsk {
		sessionApprovalFn = b.makeApprovalFunc(chatID)
	}
	if err := sess.Start(ctx, b.config.ExecMode, sessionApprovalFn); err != nil {
		b.sessionsMu.Lock()
		delete(b.sessions, chatID)
		b.sessionsMu.Unlock()
		b.send(chatID, fmt.Sprintf(b.msgs.SessionStartFailed, err))
		return
	}
	b.send(chatID, b.msgs.SessionOpened)
}

func (b *Bot) cmdSessionEnd(chatID int64) {
	b.sessionsMu.Lock()
	sess, ok := b.sessions[chatID]
	if ok {
		delete(b.sessions, chatID)
	}
	b.sessionsMu.Unlock()

	if !ok {
		b.send(chatID, b.msgs.SessionNoActive)
		return
	}
	_ = sess.Close()
	b.send(chatID, fmt.Sprintf(b.msgs.SessionEnded, sess.MsgCount))
}

func (b *Bot) cmdSessionStatus(chatID int64) {
	b.sessionsMu.Lock()
	sess, ok := b.sessions[chatID]
	b.sessionsMu.Unlock()

	if !ok || sess.IsClosed() {
		b.send(chatID, b.msgs.SessionNoStatus)
		return
	}
	uptime := time.Since(sess.StartedAt).Round(time.Second)
	b.send(chatID, fmt.Sprintf(b.msgs.SessionStatus,
		sess.StartedAt.Format("15:04:05"), uptime, sess.MsgCount))
}

func (b *Bot) handleSessionMessage(ctx context.Context, msg *tgbotapi.Message, sess *session.Session) {
	text := msg.Text
	if text == "" {
		b.send(msg.Chat.ID, b.msgs.SessionTextOnly)
		return
	}
	slog.Info("session: forwarding message",
		"chat_id", msg.Chat.ID, "runes", utf8.RuneCountInString(text))

	callCtx, cancel := context.WithTimeout(ctx, b.config.Timeout)
	defer cancel()

	type result struct {
		text string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		t, err := sess.Send(callCtx, text)
		ch <- result{t, err}
	}()

	res := <-ch

	if res.err != nil {
		slog.Error("session send error", "chat_id", msg.Chat.ID, "error", res.err)
		// Session is likely broken; close and remove it.
		b.sessionsMu.Lock()
		delete(b.sessions, msg.Chat.ID)
		b.sessionsMu.Unlock()
		_ = sess.Close()
		b.send(msg.Chat.ID, fmt.Sprintf(b.msgs.SessionError, res.err))
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
				b.send(chatID, fmt.Sprintf(b.msgs.SessionIdleTimeout, session.IdleTimeout))
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

func (b *Bot) recordCron(e recorder.CronEntry) {
	if b.recorder == nil {
		return
	}
	if err := b.recorder.LogCron(e); err != nil {
		slog.Warn("failed to write cron log entry", "error", err)
	}
}

func (b *Bot) addJob(chatID int64, from, question string, isCron bool, cancel context.CancelFunc) (*job, error) {
	preview := question
	if r := []rune(question); len(r) > 60 {
		preview = string(r[:60]) + "…"
	}
	b.jobsMu.Lock()
	defer b.jobsMu.Unlock()

	if isCron && b.config.MaxCronJobs > 0 {
		count := 0
		for _, jj := range b.jobs {
			if jj.isCron {
				count++
			}
		}
		if count >= b.config.MaxCronJobs {
			return nil, fmt.Errorf(b.msgs.CronLimitHit, b.config.MaxCronJobs)
		}
	} else if !isCron && b.config.MaxJobs > 0 {
		count := 0
		for _, jj := range b.jobs {
			if !jj.isCron {
				count++
			}
		}
		if count >= b.config.MaxJobs {
			return nil, fmt.Errorf(b.msgs.JobLimitHit, b.config.MaxJobs)
		}
	}

	b.nextJobID++
	j := &job{
		id:        b.nextJobID,
		chatID:    chatID,
		from:      from,
		preview:   preview,
		startedAt: time.Now(),
		cancel:    cancel,
		isCron:    isCron,
	}
	b.jobs[j.id] = j
	slog.Debug("job registered", "job_id", j.id, "chat_id", chatID, "from", from, "is_cron", isCron)
	return j, nil
}

func (b *Bot) removeJob(id int) {
	b.jobsMu.Lock()
	defer b.jobsMu.Unlock()
	delete(b.jobs, id)
	slog.Debug("job unregistered", "job_id", id)
}

func (b *Bot) cmdHelp(chatID int64) {
	b.send(chatID, b.msgs.Help)
}

func (b *Bot) cmdList(chatID int64) {
	b.jobsMu.Lock()
	snapshot := make([]*job, 0, len(b.jobs))
	for _, j := range b.jobs {
		snapshot = append(snapshot, j)
	}
	b.jobsMu.Unlock()

	if len(snapshot) == 0 {
		b.send(chatID, b.msgs.ListEmpty)
		return
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, b.msgs.ListHeader, len(snapshot))
	for _, j := range snapshot {
		elapsed := time.Since(j.startedAt).Round(time.Second)
		fmt.Fprintf(&sb, b.msgs.ListLine, j.id, j.from, j.preview, elapsed)
	}
	b.send(chatID, strings.TrimRight(sb.String(), "\n"))
}

func (b *Bot) cmdStop(chatID int64, arg string) {
	if arg == "" {
		b.send(chatID, b.msgs.StopUsage)
		return
	}
	id, err := strconv.Atoi(arg)
	if err != nil || id <= 0 {
		b.send(chatID, fmt.Sprintf(b.msgs.StopBadID, arg))
		return
	}

	b.jobsMu.Lock()
	j, ok := b.jobs[id]
	if ok {
		j.stopped = true
	}
	b.jobsMu.Unlock()

	if !ok {
		b.send(chatID, fmt.Sprintf(b.msgs.StopNotFound, id))
		return
	}

	j.cancel()
	slog.Info("job stop requested", "job_id", id, "chat_id", chatID)
	b.send(chatID, fmt.Sprintf(b.msgs.StopSent, id))
}

// ── Media handlers ────────────────────────────────────────────────────────────

func (b *Bot) handleSticker(msg *tgbotapi.Message) {
	emoji := ""
	if msg.Sticker != nil && msg.Sticker.Emoji != "" {
		emoji = " " + msg.Sticker.Emoji
	}
	slog.Debug("sticker received", "chat_id", msg.Chat.ID, "emoji", emoji)
	b.send(msg.Chat.ID, fmt.Sprintf(b.msgs.MediaSticker, emoji))
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
		b.send(msg.Chat.ID, b.mediaDownloadError(err))
		return
	}

	caption := msg.Caption
	displayQuestion := b.msgs.MediaPhotoDisplay
	if caption != "" {
		displayQuestion = fmt.Sprintf(b.msgs.MediaPhotoCaption, caption)
	}
	rawPrompt := b.buildMediaPrompt(b.msgs.MediaTypeImage, localPath, filename, caption)
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
		b.send(msg.Chat.ID, b.mediaDownloadError(err))
		return
	}

	caption := msg.Caption
	displayQuestion := fmt.Sprintf(b.msgs.MediaDocDisplay, filename)
	if caption != "" {
		displayQuestion = fmt.Sprintf(b.msgs.MediaDocCaption, filename, caption)
	}
	rawPrompt := b.buildMediaPrompt(b.msgs.MediaTypeFile, localPath, filename, caption)
	b.executeAndReply(ctx, msg, displayQuestion, rawPrompt)
}

func (b *Bot) handleVoice(ctx context.Context, msg *tgbotapi.Message) {
	if !b.config.EnableVoice {
		b.send(msg.Chat.ID, b.msgs.MediaVoiceDisabled)
		return
	}

	filename := fmt.Sprintf("voice_%d.ogg", msg.MessageID)
	slog.Info("voice received, downloading", "chat_id", msg.Chat.ID, "message_id", msg.MessageID)
	localPath, err := b.dl.Download(ctx, msg.Voice.FileID, msg.Voice.FileSize, msg.Chat.ID, filename)
	if err != nil {
		b.send(msg.Chat.ID, b.mediaDownloadError(err))
		return
	}

	slog.Info("transcribing voice", "chat_id", msg.Chat.ID, "path", localPath)
	b.send(msg.Chat.ID, b.msgs.MediaVoiceTranscribing)
	text, err := transcriber.Transcribe(ctx, localPath, b.config.WhisperBackend, b.config.WhisperModel)
	if err != nil {
		slog.Error("whisper transcription failed", "chat_id", msg.Chat.ID, "error", err)
		b.send(msg.Chat.ID, fmt.Sprintf(b.msgs.MediaVoiceFailed, err))
		return
	}

	slog.Info("voice transcribed", "chat_id", msg.Chat.ID, "runes", utf8.RuneCountInString(text))
	b.send(msg.Chat.ID, fmt.Sprintf(b.msgs.MediaVoiceResult, text))
	b.executeAndReply(ctx, msg, fmt.Sprintf(b.msgs.MediaVoiceResult, text), text)
}

// buildMediaPrompt formats a prompt that includes file context for the LLM.
func (b *Bot) buildMediaPrompt(mediaType, localPath, filename, caption string) string {
	m := b.msgs
	var sb strings.Builder
	fmt.Fprintf(&sb, m.MediaPromptSent+"\n", mediaType, filename)
	fmt.Fprintf(&sb, m.MediaPromptSavedTo+"\n", localPath)
	if caption != "" {
		fmt.Fprintf(&sb, m.MediaPromptCaption+"\n", caption)
		fmt.Fprintf(&sb, "\n"+m.MediaPromptTask, mediaType)
	} else {
		fmt.Fprintf(&sb, "\n"+m.MediaPromptAsk, mediaType)
	}
	return sb.String()
}

func (b *Bot) mediaDownloadError(err error) string {
	if errors.Is(err, downloader.ErrFileTooLarge) {
		return b.msgs.MediaFileTooLarge
	}
	return fmt.Sprintf(b.msgs.MediaDownloadFailed, err)
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
	fmt.Fprintf(&sb, "%s", b.msgs.HistoryCtxHeader)
	for _, ex := range history {
		fmt.Fprintf(&sb, b.msgs.HistoryCtxEntry, ex.Question, ex.Answer)
	}
	fmt.Fprintf(&sb, b.msgs.HistoryCtxTail, question)

	slog.Debug("prompt with history built",
		"chat_id", chatID,
		"history_count", len(history),
		"prompt_runes", utf8.RuneCountInString(sb.String()),
	)
	return sb.String()
}

func (b *Bot) cmdHistory(chatID int64) {
	if b.recorder == nil {
		b.send(chatID, b.msgs.HistoryDisabled)
		return
	}
	history, err := b.recorder.LoadRecent(chatID, b.config.HistorySize)
	if err != nil {
		b.send(chatID, fmt.Sprintf(b.msgs.HistoryLoadError, err))
		return
	}
	if len(history) == 0 {
		b.send(chatID, b.msgs.HistoryEmpty)
		return
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, b.msgs.HistoryHeader, len(history))
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
	fmt.Fprintf(&sb, "%s", b.msgs.StatusRunning)
	fmt.Fprintf(&sb, b.msgs.StatusBot, b.api.Self.UserName)
	fmt.Fprintf(&sb, b.msgs.StatusHost, b.hostname)
	fmt.Fprintf(&sb, b.msgs.StatusStartedAt, startedAt)
	fmt.Fprintf(&sb, b.msgs.StatusUptime, uptime)
	fmt.Fprintf(&sb, b.msgs.StatusProvider, b.config.Provider)

	if len(activeJobs) == 0 {
		fmt.Fprintf(&sb, "%s", b.msgs.StatusNoJobs)
	} else {
		fmt.Fprintf(&sb, b.msgs.StatusJobsHeader, len(activeJobs))
		for _, j := range activeJobs {
			elapsed := time.Since(j.startedAt).Round(time.Second)
			fmt.Fprintf(&sb, b.msgs.StatusJobLine, j.id, j.from, j.preview, elapsed)
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
		text := fmt.Sprintf(b.msgs.ApprovalPrompt, displayPrompt)

		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(b.msgs.ApprovalAllow, key+"_y"),
				tgbotapi.NewInlineKeyboardButtonData(b.msgs.ApprovalDeny, key+"_n"),
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
				text+"\n\n"+b.msgs.ApprovalTimeout)
			_, _ = b.api.Send(edit)
			return false, fmt.Errorf("approval timed out")
		case <-ctx.Done():
			return false, ctx.Err()
		}

		label := b.msgs.ApprovalDenied
		if allowed {
			label = b.msgs.ApprovalAllowed
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

// isGroupChat reports whether a message came from a group or supergroup.
func isGroupChat(msg *tgbotapi.Message) bool {
	return msg.Chat.Type == "group" || msg.Chat.Type == "supergroup"
}

// botAddressed reports whether the message was directed at the bot:
// either an explicit @mention in the text entities or a direct reply to the bot.
func (b *Bot) botAddressed(msg *tgbotapi.Message) bool {
	if msg.ReplyToMessage != nil && msg.ReplyToMessage.From != nil &&
		msg.ReplyToMessage.From.ID == b.api.Self.ID {
		return true
	}
	want := strings.ToLower("@" + b.api.Self.UserName)
	runes := []rune(msg.Text)
	for _, e := range msg.Entities {
		if e.Type != "mention" || e.Offset+e.Length > len(runes) {
			continue
		}
		if strings.ToLower(string(runes[e.Offset:e.Offset+e.Length])) == want {
			return true
		}
	}
	return false
}

// stripBotMention removes @botUsername from standalone mention entities and strips
// the "@botUsername" suffix from bot_command entities (e.g. "/help@bot" → "/help").
// Entities are processed in reverse order so earlier offsets remain valid.
func stripBotMention(text, botUsername string, entities []tgbotapi.MessageEntity) string {
	suffix := strings.ToLower("@" + botUsername)
	runes := []rune(text)
	for i := len(entities) - 1; i >= 0; i-- {
		e := entities[i]
		end := e.Offset + e.Length
		if end > len(runes) {
			continue
		}
		entityText := strings.ToLower(string(runes[e.Offset:end]))
		switch e.Type {
		case "mention":
			if entityText == suffix {
				runes = append(runes[:e.Offset], runes[end:]...)
			}
		case "bot_command":
			if strings.HasSuffix(entityText, suffix) {
				cutAt := end - len([]rune(suffix))
				runes = append(runes[:cutAt], runes[end:]...)
			}
		}
	}
	return strings.TrimSpace(string(runes))
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
