package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setMtime(t *testing.T, path string, age time.Duration) {
	t.Helper()
	ts := time.Now().Add(-age)
	if err := os.Chtimes(path, ts, ts); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

func TestCleanTmp_RemovesStaleAndKeepsFresh(t *testing.T) {
	tmp := t.TempDir()
	tmpRoot := filepath.Join(tmp, "tmp")

	stale := filepath.Join(tmpRoot, "42", "old.jpg")
	fresh := filepath.Join(tmpRoot, "42", "new.jpg")
	writeFile(t, stale, "old-data-xxxxxx")
	writeFile(t, fresh, "new")
	setMtime(t, stale, 48*time.Hour)
	// fresh keeps default (now) mtime

	removed, freed, err := cleanTmp(tmpRoot, 24*time.Hour)
	if err != nil {
		t.Fatalf("cleanTmp: %v", err)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}
	if freed != int64(len("old-data-xxxxxx")) {
		t.Errorf("freed = %d, want %d", freed, len("old-data-xxxxxx"))
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale should be deleted, stat=%v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("fresh should remain: %v", err)
	}
}

func TestCleanTmp_RemovesEmptySubdirs(t *testing.T) {
	tmp := t.TempDir()
	tmpRoot := filepath.Join(tmp, "tmp")

	stale := filepath.Join(tmpRoot, "99", "only.txt")
	writeFile(t, stale, "x")
	setMtime(t, stale, 100*time.Hour)

	_, _, err := cleanTmp(tmpRoot, 24*time.Hour)
	if err != nil {
		t.Fatalf("cleanTmp: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpRoot, "99")); !os.IsNotExist(err) {
		t.Errorf("empty subdir should be removed, stat=%v", err)
	}
	if _, err := os.Stat(tmpRoot); err != nil {
		t.Errorf("tmp root must be preserved: %v", err)
	}
}

func TestCleanTmp_MissingDirIsNoop(t *testing.T) {
	tmp := t.TempDir()
	removed, freed, err := cleanTmp(filepath.Join(tmp, "tmp-not-there"), time.Hour)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if removed != 0 || freed != 0 {
		t.Errorf("expected zero stats on missing dir, got %d/%d", removed, freed)
	}
}

func TestCleanLogs_RemovesOldDailyOnly(t *testing.T) {
	tmp := t.TempDir()

	staleDaily := filepath.Join(tmp, "2026-04-01.jsonl")
	freshDaily := filepath.Join(tmp, "2026-05-14.jsonl")
	history := filepath.Join(tmp, "history_42.jsonl")
	staleCron := filepath.Join(tmp, "cron", "2026-04-01.jsonl")
	cronDef := filepath.Join(tmp, "cron", "abc123.json")

	for _, p := range []string{staleDaily, freshDaily, history, staleCron, cronDef} {
		writeFile(t, p, "data\n")
	}
	setMtime(t, staleDaily, 60*24*time.Hour)
	setMtime(t, history, 60*24*time.Hour) // also aged — must NOT be removed
	setMtime(t, staleCron, 60*24*time.Hour)
	setMtime(t, cronDef, 60*24*time.Hour) // also aged — must NOT be removed

	removed, _, err := cleanLogs(tmp, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("cleanLogs: %v", err)
	}
	if removed != 2 {
		t.Errorf("removed = %d, want 2 (staleDaily + staleCron)", removed)
	}
	if _, err := os.Stat(staleDaily); !os.IsNotExist(err) {
		t.Errorf("staleDaily should be gone")
	}
	if _, err := os.Stat(staleCron); !os.IsNotExist(err) {
		t.Errorf("staleCron should be gone")
	}
	for _, p := range []string{freshDaily, history, cronDef} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s should remain: %v", filepath.Base(p), err)
		}
	}
}

func TestCompactHistory_TrimsOverlong(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "history_42.jsonl")
	var sb strings.Builder
	for i := 0; i < 50; i++ {
		fmt.Fprintf(&sb, `{"q":"q%d","a":"a%d"}`+"\n", i, i)
	}
	writeFile(t, path, sb.String())

	compacted, removed, err := compactHistory(tmp, 10)
	if err != nil {
		t.Fatalf("compactHistory: %v", err)
	}
	if compacted != 1 {
		t.Errorf("compacted = %d, want 1", compacted)
	}
	if removed != 40 {
		t.Errorf("removed = %d, want 40", removed)
	}

	data, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 10 {
		t.Fatalf("kept %d lines, want 10", len(lines))
	}
	if !strings.Contains(lines[0], `"q":"q40"`) {
		t.Errorf("first kept line wrong: %s", lines[0])
	}
	if !strings.Contains(lines[9], `"q":"q49"`) {
		t.Errorf("last kept line wrong: %s", lines[9])
	}
}

func TestCompactHistory_BelowThresholdUntouched(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "history_7.jsonl")
	writeFile(t, path, "line1\nline2\nline3\n")

	compacted, removed, err := compactHistory(tmp, 100)
	if err != nil {
		t.Fatalf("compactHistory: %v", err)
	}
	if compacted != 0 || removed != 0 {
		t.Errorf("expected no-op, got compacted=%d removed=%d", compacted, removed)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "line1\nline2\nline3\n" {
		t.Errorf("file modified unexpectedly: %q", data)
	}
}

func TestCompactHistory_IgnoresNonHistory(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "2026-05-14.jsonl"),
		strings.Repeat("daily\n", 200))

	compacted, _, err := compactHistory(tmp, 10)
	if err != nil {
		t.Fatalf("compactHistory: %v", err)
	}
	if compacted != 0 {
		t.Errorf("should ignore non-history files, got compacted=%d", compacted)
	}
}

func TestRunStartup_RespectsDisabled(t *testing.T) {
	tmp := t.TempDir()

	stale := filepath.Join(tmp, "tmp", "1", "old.bin")
	writeFile(t, stale, "x")
	setMtime(t, stale, 100*time.Hour)

	staleLog := filepath.Join(tmp, "2026-01-01.jsonl")
	writeFile(t, staleLog, "y")
	setMtime(t, staleLog, 100*24*time.Hour)

	bigHist := filepath.Join(tmp, "history_1.jsonl")
	writeFile(t, bigHist, strings.Repeat("entry\n", 500))

	// All retentions disabled.
	stats := RunStartup(StartupConfig{
		ProjectDir:        tmp,
		TmpRetentionHours: 0,
		LogRetentionDays:  0,
		HistoryMaxEntries: 0,
	})

	if stats.TmpFilesRemoved != 0 || stats.LogFilesRemoved != 0 || stats.HistoryFilesCompacted != 0 {
		t.Errorf("disabled retentions should leave everything: %+v", stats)
	}
	for _, p := range []string{stale, staleLog, bigHist} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s should still exist: %v", filepath.Base(p), err)
		}
	}
}

func TestRunStartup_EndToEnd(t *testing.T) {
	tmp := t.TempDir()

	// Stale tmp file
	stale := filepath.Join(tmp, "tmp", "1", "old.bin")
	writeFile(t, stale, "garbage")
	setMtime(t, stale, 48*time.Hour)

	// Stale daily log
	staleLog := filepath.Join(tmp, "2026-01-01.jsonl")
	writeFile(t, staleLog, "log")
	setMtime(t, staleLog, 100*24*time.Hour)

	// Oversize history
	var sb strings.Builder
	for i := 0; i < 30; i++ {
		fmt.Fprintf(&sb, "entry-%d\n", i)
	}
	hist := filepath.Join(tmp, "history_99.jsonl")
	writeFile(t, hist, sb.String())

	stats := RunStartup(StartupConfig{
		ProjectDir:        tmp,
		TmpRetentionHours: 24,
		LogRetentionDays:  30,
		HistoryMaxEntries: 10,
	})

	if stats.TmpFilesRemoved != 1 {
		t.Errorf("TmpFilesRemoved=%d, want 1", stats.TmpFilesRemoved)
	}
	if stats.LogFilesRemoved != 1 {
		t.Errorf("LogFilesRemoved=%d, want 1", stats.LogFilesRemoved)
	}
	if stats.HistoryFilesCompacted != 1 {
		t.Errorf("HistoryFilesCompacted=%d, want 1", stats.HistoryFilesCompacted)
	}
	if stats.HistoryEntriesRemoved != 20 {
		t.Errorf("HistoryEntriesRemoved=%d, want 20", stats.HistoryEntriesRemoved)
	}
}
