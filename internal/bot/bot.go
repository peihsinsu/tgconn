package bot

import (
	"context"
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
	"github.com/cx009/tgconn/internal/provider"
	"github.com/cx009/tgconn/internal/recorder"
)

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

type Bot struct {
	api       *tgbotapi.BotAPI
	provider  provider.Provider
	config    *config.Config
	recorder  *recorder.Recorder
	jobsMu    sync.Mutex
	jobs      map[int]*job
	nextJobID int
	startTime time.Time
	hostname  string
}

func New(cfg *config.Config, p provider.Provider, rec *recorder.Recorder) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Telegram: %w", err)
	}
	slog.Info("telegram API connection established", "bot_username", api.Self.UserName)
	hostname, _ := os.Hostname()
	return &Bot{
		api:       api,
		provider:  p,
		config:    cfg,
		recorder:  rec,
		jobs:      make(map[int]*job),
		startTime: time.Now(),
		hostname:  hostname,
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
	b.broadcast(fmt.Sprintf("✅ 已連線，provider: %s", b.config.Provider))

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
	question := msg.Text
	if question == "" {
		slog.Debug("ignoring empty message", "chat_id", msg.Chat.ID, "message_id", msg.MessageID)
		return
	}

	fromUser := senderName(msg)

	// Intercept built-in commands before forwarding to LLM.
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

	slog.Info("handling message",
		"chat_id", msg.Chat.ID,
		"from", fromUser,
		"message_id", msg.MessageID,
		"question_runes", utf8.RuneCountInString(question),
	)
	slog.Debug("message text", "chat_id", msg.Chat.ID, "question", question)

	callCtx, cancel := context.WithTimeout(ctx, b.config.Timeout)
	j := b.addJob(msg.Chat.ID, fromUser, question, cancel)
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

	prompt := b.buildPrompt(msg.Chat.ID, question)

	slog.Info("invoking provider",
		"provider", b.config.Provider,
		"chat_id", msg.Chat.ID,
		"job_id", j.id,
		"timeout", b.config.Timeout,
		"history_injected", prompt != question,
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
		// provider responded before the notice threshold
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
			slog.Info("job stopped by user",
				"chat_id", msg.Chat.ID,
				"job_id", j.id,
				"elapsed", elapsed,
			)
			b.send(msg.Chat.ID, fmt.Sprintf("🛑 指令 #%d 已停止（執行了 %s）", j.id, elapsed))
			b.record(recorder.Entry{
				Time:      start,
				ChatID:    msg.Chat.ID,
				From:      fromUser,
				Question:  question,
				Error:     "stopped by user",
				ElapsedMs: elapsed.Milliseconds(),
			})
			return
		}
		slog.Error("provider error",
			"chat_id", msg.Chat.ID,
			"from", fromUser,
			"job_id", j.id,
			"elapsed", elapsed,
			"error", res.err,
		)
		b.send(msg.Chat.ID, fmt.Sprintf("error: %v", res.err))
		b.record(recorder.Entry{
			Time:      start,
			ChatID:    msg.Chat.ID,
			From:      fromUser,
			Question:  question,
			Error:     res.err.Error(),
			ElapsedMs: elapsed.Milliseconds(),
		})
		return
	}

	responseRunes := utf8.RuneCountInString(res.response)
	chunks := splitMessage(res.response)
	slog.Info("provider response ready",
		"chat_id", msg.Chat.ID,
		"job_id", j.id,
		"elapsed", elapsed,
		"response_runes", responseRunes,
		"chunks", len(chunks),
	)
	slog.Debug("response text",
		"chat_id", msg.Chat.ID,
		"response", res.response[:min(300, len(res.response))],
	)

	for i, chunk := range chunks {
		if len(chunks) > 1 {
			slog.Debug("sending chunk", "chat_id", msg.Chat.ID, "chunk", i+1, "total", len(chunks))
			b.send(msg.Chat.ID, fmt.Sprintf("[%d/%d]\n%s", i+1, len(chunks), chunk))
		} else {
			b.send(msg.Chat.ID, chunk)
		}
	}

	b.record(recorder.Entry{
		Time:      start,
		ChatID:    msg.Chat.ID,
		From:      fromUser,
		Question:  question,
		Response:  res.response,
		ElapsedMs: elapsed.Milliseconds(),
	})

	slog.Info("message handled",
		"chat_id", msg.Chat.ID,
		"message_id", msg.MessageID,
		"job_id", j.id,
		"elapsed", elapsed,
	)
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
		end := maxMessageLen
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[:end]))
		runes = runes[end:]
	}
	return chunks
}
