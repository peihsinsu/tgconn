package cronjob

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// Job is a persisted cron task.
type Job struct {
	ID        string    `json:"id"`
	ChatID    int64     `json:"chat_id"`
	Expr      string    `json:"expr"`
	Prompt    string    `json:"prompt"`
	CreatedAt time.Time `json:"created_at"`
}

// JobInfo extends Job with runtime info (next scheduled run).
type JobInfo struct {
	Job
	NextRun time.Time
}

type entry struct {
	job    Job
	cronID cron.EntryID
}

// ExecuteFunc is called each time a scheduled job fires.
type ExecuteFunc func(chatID int64, jobID, expr, prompt string)

// Manager owns the cron scheduler and persists jobs to disk.
type Manager struct {
	dir     string
	c       *cron.Cron
	mu      sync.Mutex
	jobs    map[string]*entry
	execute ExecuteFunc
}

// New creates a Manager, loads persisted jobs from dir, and registers them.
// execute is called each time a job fires.
func New(dir string, execute ExecuteFunc) (*Manager, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("cronjob: mkdir %s: %w", dir, err)
	}
	m := &Manager{
		dir:     dir,
		c:       cron.New(),
		jobs:    make(map[string]*entry),
		execute: execute,
	}
	if err := m.load(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) Start() { m.c.Start() }
func (m *Manager) Stop()  { m.c.Stop() }

// Add schedules a new job and persists it to disk.
func (m *Manager) Add(chatID int64, expr, prompt string) (*Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := randID()
	j := Job{
		ID:        id,
		ChatID:    chatID,
		Expr:      expr,
		Prompt:    prompt,
		CreatedAt: time.Now(),
	}

	cronID, err := m.register(j)
	if err != nil {
		return nil, err
	}
	m.jobs[id] = &entry{job: j, cronID: cronID}

	if err := m.save(&j); err != nil {
		m.c.Remove(cronID)
		delete(m.jobs, id)
		return nil, err
	}
	slog.Info("cronjob added", "id", id, "expr", expr, "chat_id", chatID)
	return &j, nil
}

// Delete removes a job from the scheduler and disk.
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	e, ok := m.jobs[id]
	if !ok {
		return fmt.Errorf("job %q not found", id)
	}
	m.c.Remove(e.cronID)
	delete(m.jobs, id)
	slog.Info("cronjob deleted", "id", id)
	return os.Remove(filepath.Join(m.dir, id+".json"))
}

// List returns all jobs for a given chat with their next run time.
func (m *Manager) List(chatID int64) []JobInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	var out []JobInfo
	for _, e := range m.jobs {
		if e.job.ChatID != chatID {
			continue
		}
		out = append(out, JobInfo{
			Job:     e.job,
			NextRun: m.c.Entry(e.cronID).Next,
		})
	}
	return out
}

// register adds a job to the cron scheduler (must hold mu or be called before Start).
func (m *Manager) register(j Job) (cron.EntryID, error) {
	jCopy := j
	cronID, err := m.c.AddFunc(j.Expr, func() {
		m.execute(jCopy.ChatID, jCopy.ID, jCopy.Expr, jCopy.Prompt)
	})
	if err != nil {
		return 0, fmt.Errorf("invalid cron expression %q: %w", j.Expr, err)
	}
	return cronID, nil
}

func (m *Manager) load() error {
	des, err := os.ReadDir(m.dir)
	if err != nil {
		return nil // empty dir is fine
	}
	for _, de := range des {
		if !strings.HasSuffix(de.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(m.dir, de.Name()))
		if err != nil {
			slog.Warn("cronjob: failed to read", "file", de.Name(), "error", err)
			continue
		}
		var j Job
		if err := json.Unmarshal(data, &j); err != nil {
			slog.Warn("cronjob: failed to parse", "file", de.Name(), "error", err)
			continue
		}
		cronID, err := m.register(j)
		if err != nil {
			slog.Warn("cronjob: invalid expression, skipping", "id", j.ID, "expr", j.Expr, "error", err)
			continue
		}
		m.jobs[j.ID] = &entry{job: j, cronID: cronID}
		slog.Info("cronjob loaded", "id", j.ID, "expr", j.Expr)
	}
	return nil
}

func (m *Manager) save(j *Job) error {
	data, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(m.dir, j.ID+".json"), data, 0644)
}

// ParseArgs parses "/cron" arguments into (expr, prompt).
// Supports standard 5-field cron and @predefined schedules.
// Prompt may optionally be wrapped in double quotes.
func ParseArgs(args string) (expr, prompt string, err error) {
	args = strings.TrimSpace(args)
	if args == "" {
		return "", "", fmt.Errorf("usage: /cron <expr> <prompt>\nexample: /cron \"0 9 * * *\" daily morning report")
	}

	if strings.HasPrefix(args, "@") {
		idx := strings.IndexByte(args, ' ')
		if idx < 0 {
			return "", "", fmt.Errorf("missing prompt after schedule expression")
		}
		expr = args[:idx]
		prompt = strings.Trim(strings.TrimSpace(args[idx+1:]), `"`)
		return
	}

	// Standard 5-field cron: need at least 5 fields + 1 prompt word
	parts := strings.Fields(args)
	if len(parts) < 6 {
		return "", "", fmt.Errorf("usage: /cron <min> <hour> <dom> <month> <dow> <prompt>\nexample: /cron 0 9 * * 1 weekly Monday morning report")
	}
	expr = strings.Join(parts[:5], " ")
	raw := strings.Join(parts[5:], " ")
	prompt = strings.Trim(raw, `"`)
	return
}

func randID() string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 6)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
