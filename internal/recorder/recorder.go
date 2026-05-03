package recorder

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const logDir = ".tgconn"
const cronLogDir = ".tgconn/cron"

type Entry struct {
	Time      time.Time `json:"time"`
	ChatID    int64     `json:"chat_id"`
	From      string    `json:"from"`
	Question  string    `json:"question"`
	Response  string    `json:"response,omitempty"`
	Error     string    `json:"error,omitempty"`
	ElapsedMs int64     `json:"elapsed_ms"`
}

// Exchange is a single Q&A pair stored in per-chat history.
type Exchange struct {
	Time     time.Time `json:"ts"`
	Question string    `json:"q"`
	Answer   string    `json:"a"`
}

type Recorder struct {
	mu  sync.Mutex
	dir string
}

// CronEntry records a single cron job execution.
type CronEntry struct {
	Time      time.Time `json:"time"`
	ChatID    int64     `json:"chat_id"`
	JobID     string    `json:"job_id"`
	Expr      string    `json:"expr"`
	Prompt    string    `json:"prompt"`
	Response  string    `json:"response,omitempty"`
	Error     string    `json:"error,omitempty"`
	ElapsedMs int64     `json:"elapsed_ms"`
}

func New() (*Recorder, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create log directory %s: %w", logDir, err)
	}
	if err := os.MkdirAll(cronLogDir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create cron log directory %s: %w", cronLogDir, err)
	}
	return &Recorder{dir: logDir}, nil
}

// LogCron writes a cron execution entry to the separate daily cron log.
func (r *Recorder) LogCron(e CronEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	filename := filepath.Join(cronLogDir, e.Time.Format("2006-01-02")+".jsonl")
	if err := appendJSON(filename, e); err != nil {
		return fmt.Errorf("cannot write cron log: %w", err)
	}
	return nil
}

// Log writes a full entry to the daily audit log.
func (r *Recorder) Log(e Entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	filename := filepath.Join(r.dir, e.Time.Format("2006-01-02")+".jsonl")
	if err := appendJSON(filename, e); err != nil {
		return fmt.Errorf("cannot write daily log: %w", err)
	}

	// Only persist successful exchanges to the per-chat history.
	if e.Error == "" && e.Response != "" {
		chatFile := filepath.Join(r.dir, fmt.Sprintf("history_%d.jsonl", e.ChatID))
		ex := Exchange{Time: e.Time, Question: e.Question, Answer: e.Response}
		if err := appendJSON(chatFile, ex); err != nil {
			return fmt.Errorf("cannot write chat history: %w", err)
		}
	}
	return nil
}

// LoadRecent returns the last n successful exchanges for a chat.
func (r *Recorder) LoadRecent(chatID int64, n int) ([]Exchange, error) {
	if n <= 0 {
		return nil, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	filename := filepath.Join(r.dir, fmt.Sprintf("history_%d.jsonl", chatID))
	f, err := os.Open(filename)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot open chat history: %w", err)
	}
	defer f.Close()

	var all []Exchange
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1 MB per line
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var ex Exchange
		if err := json.Unmarshal([]byte(line), &ex); err == nil {
			all = append(all, ex)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading chat history: %w", err)
	}

	if len(all) <= n {
		return all, nil
	}
	return all[len(all)-n:], nil
}

func appendJSON(filename string, v any) error {
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}
