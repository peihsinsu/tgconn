package recorder

import (
	"os"
	"testing"
	"time"
)

func tempRecorder(t *testing.T) (*Recorder, func()) {
	t.Helper()
	dir := t.TempDir()
	r := &Recorder{dir: dir}
	return r, func() { os.RemoveAll(dir) }
}

func TestLoadRecent_Empty(t *testing.T) {
	r, cleanup := tempRecorder(t)
	defer cleanup()

	history, err := r.LoadRecent(123, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("expected empty history, got %d entries", len(history))
	}
}

func TestLoadRecent_RoundTrip(t *testing.T) {
	r, cleanup := tempRecorder(t)
	defer cleanup()

	now := time.Now()
	entries := []Entry{
		{Time: now, ChatID: 42, From: "@alice", Question: "q1", Response: "a1", ElapsedMs: 100},
		{Time: now, ChatID: 42, From: "@alice", Question: "q2", Response: "a2", ElapsedMs: 200},
		{Time: now, ChatID: 42, From: "@alice", Question: "q3", Response: "a3", ElapsedMs: 300},
	}
	for _, e := range entries {
		if err := r.Log(e); err != nil {
			t.Fatalf("Log() error: %v", err)
		}
	}

	history, err := r.LoadRecent(42, 10)
	if err != nil {
		t.Fatalf("LoadRecent() error: %v", err)
	}
	if len(history) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(history))
	}
	if history[0].Question != "q1" || history[2].Question != "q3" {
		t.Errorf("unexpected history order: %+v", history)
	}
}

func TestLoadRecent_Limit(t *testing.T) {
	r, cleanup := tempRecorder(t)
	defer cleanup()

	now := time.Now()
	for i := 0; i < 15; i++ {
		e := Entry{
			Time: now, ChatID: 99, From: "@bob",
			Question: "q", Response: "a", ElapsedMs: 10,
		}
		if err := r.Log(e); err != nil {
			t.Fatalf("Log() error: %v", err)
		}
	}

	history, err := r.LoadRecent(99, 5)
	if err != nil {
		t.Fatalf("LoadRecent() error: %v", err)
	}
	if len(history) != 5 {
		t.Errorf("expected 5 entries (limit), got %d", len(history))
	}
}

func TestLoadRecent_ErrorEntriesNotPersisted(t *testing.T) {
	r, cleanup := tempRecorder(t)
	defer cleanup()

	now := time.Now()
	// Error entry — should NOT appear in history
	if err := r.Log(Entry{Time: now, ChatID: 7, From: "@x", Question: "q", Error: "boom", ElapsedMs: 10}); err != nil {
		t.Fatalf("Log() error: %v", err)
	}
	// Success entry — should appear
	if err := r.Log(Entry{Time: now, ChatID: 7, From: "@x", Question: "q2", Response: "a2", ElapsedMs: 20}); err != nil {
		t.Fatalf("Log() error: %v", err)
	}

	history, err := r.LoadRecent(7, 10)
	if err != nil {
		t.Fatalf("LoadRecent() error: %v", err)
	}
	if len(history) != 1 {
		t.Errorf("expected 1 success entry, got %d", len(history))
	}
	if history[0].Question != "q2" {
		t.Errorf("unexpected entry: %+v", history[0])
	}
}

func TestLoadRecent_ChatIsolation(t *testing.T) {
	r, cleanup := tempRecorder(t)
	defer cleanup()

	now := time.Now()
	_ = r.Log(Entry{Time: now, ChatID: 1, Question: "chat1-q", Response: "chat1-a", ElapsedMs: 10})
	_ = r.Log(Entry{Time: now, ChatID: 2, Question: "chat2-q", Response: "chat2-a", ElapsedMs: 10})

	h1, _ := r.LoadRecent(1, 10)
	h2, _ := r.LoadRecent(2, 10)

	if len(h1) != 1 || h1[0].Question != "chat1-q" {
		t.Errorf("chat 1 history wrong: %+v", h1)
	}
	if len(h2) != 1 || h2[0].Question != "chat2-q" {
		t.Errorf("chat 2 history wrong: %+v", h2)
	}
}
