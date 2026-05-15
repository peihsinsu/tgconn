package storage

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// StartupConfig drives the once-per-startup retention pass.
type StartupConfig struct {
	ProjectDir        string
	TmpRetentionHours int // 0 = skip tmp cleanup
	LogRetentionDays  int // 0 = skip log cleanup
	HistoryMaxEntries int // 0 = skip history compaction
}

// Stats summarises one cleanup pass.
type Stats struct {
	TmpFilesRemoved       int
	TmpBytesFreed         int64
	LogFilesRemoved       int
	LogBytesFreed         int64
	HistoryFilesCompacted int
	HistoryEntriesRemoved int
}

// RunStartup applies all configured retention rules. Errors during cleanup are
// logged but never propagate up — startup must continue even if cleanup fails.
func RunStartup(cfg StartupConfig) Stats {
	var stats Stats

	if cfg.TmpRetentionHours > 0 {
		removed, freed, err := cleanTmp(filepath.Join(cfg.ProjectDir, "tmp"),
			time.Duration(cfg.TmpRetentionHours)*time.Hour)
		if err != nil {
			slog.Warn("storage: tmp cleanup failed", "error", err)
		}
		stats.TmpFilesRemoved = removed
		stats.TmpBytesFreed = freed
	}

	if cfg.LogRetentionDays > 0 {
		removed, freed, err := cleanLogs(cfg.ProjectDir,
			time.Duration(cfg.LogRetentionDays)*24*time.Hour)
		if err != nil {
			slog.Warn("storage: log cleanup failed", "error", err)
		}
		stats.LogFilesRemoved = removed
		stats.LogBytesFreed = freed
	}

	if cfg.HistoryMaxEntries > 0 {
		compacted, removed, err := compactHistory(cfg.ProjectDir, cfg.HistoryMaxEntries)
		if err != nil {
			slog.Warn("storage: history compaction failed", "error", err)
		}
		stats.HistoryFilesCompacted = compacted
		stats.HistoryEntriesRemoved = removed
	}

	return stats
}

// cleanTmp removes files under tmpDir whose mtime is older than olderThan, then
// removes any resulting empty <chatID>/ subdirectories. The tmp root itself is
// preserved.
func cleanTmp(tmpDir string, olderThan time.Duration) (int, int64, error) {
	info, err := os.Stat(tmpDir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("stat tmp: %w", err)
	}
	if !info.IsDir() {
		return 0, 0, fmt.Errorf("tmp path %s is not a directory", tmpDir)
	}

	cutoff := time.Now().Add(-olderThan)
	var removed int
	var freed int64

	walkErr := filepath.WalkDir(tmpDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			slog.Debug("storage: tmp walk error", "path", path, "error", err)
			return nil
		}
		if d.IsDir() || path == tmpDir {
			return nil
		}
		fi, ferr := d.Info()
		if ferr != nil {
			return nil
		}
		if fi.ModTime().After(cutoff) {
			return nil
		}
		size := fi.Size()
		if rmErr := os.Remove(path); rmErr != nil {
			slog.Debug("storage: tmp remove failed", "path", path, "error", rmErr)
			return nil
		}
		removed++
		freed += size
		return nil
	})
	if walkErr != nil {
		return removed, freed, walkErr
	}

	// Second pass: remove empty <chatID>/ subdirectories.
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return removed, freed, nil
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub := filepath.Join(tmpDir, e.Name())
		children, rerr := os.ReadDir(sub)
		if rerr != nil || len(children) > 0 {
			continue
		}
		_ = os.Remove(sub)
	}
	return removed, freed, nil
}

// cleanLogs deletes daily *.jsonl files (excluding history_*.jsonl) in
// projectDir/ and projectDir/cron/ whose mtime is older than olderThan.
// history files and cron job definitions (<id>.json) are left untouched.
func cleanLogs(projectDir string, olderThan time.Duration) (int, int64, error) {
	cutoff := time.Now().Add(-olderThan)
	var totalRemoved int
	var totalFreed int64

	for _, dir := range []string{projectDir, filepath.Join(projectDir, "cron")} {
		removed, freed, err := removeStaleJSONL(dir, cutoff)
		if err != nil {
			return totalRemoved, totalFreed, err
		}
		totalRemoved += removed
		totalFreed += freed
	}
	return totalRemoved, totalFreed, nil
}

func removeStaleJSONL(dir string, cutoff time.Time) (int, int64, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("read %s: %w", dir, err)
	}

	var removed int
	var freed int64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		if strings.HasPrefix(name, "history_") {
			continue
		}
		path := filepath.Join(dir, name)
		fi, ferr := e.Info()
		if ferr != nil {
			continue
		}
		if fi.ModTime().After(cutoff) {
			continue
		}
		size := fi.Size()
		if rmErr := os.Remove(path); rmErr != nil {
			slog.Debug("storage: log remove failed", "path", path, "error", rmErr)
			continue
		}
		removed++
		freed += size
	}
	return removed, freed, nil
}

// compactHistory rewrites each history_<chatID>.jsonl whose line count exceeds
// maxEntries, keeping only the last maxEntries lines. Returns the number of
// files compacted and the total entries trimmed.
func compactHistory(projectDir string, maxEntries int) (int, int, error) {
	entries, err := os.ReadDir(projectDir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("read project dir: %w", err)
	}

	var filesCompacted int
	var entriesRemoved int
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "history_") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		path := filepath.Join(projectDir, name)
		removed, ok, cerr := compactOne(path, maxEntries)
		if cerr != nil {
			slog.Warn("storage: compaction error", "path", path, "error", cerr)
			continue
		}
		if ok {
			filesCompacted++
			entriesRemoved += removed
		}
	}
	return filesCompacted, entriesRemoved, nil
}

// compactOne trims path in place to its last maxEntries lines. Returns
// (linesRemoved, didCompact, error).
func compactOne(path string, maxEntries int) (int, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, false, fmt.Errorf("open: %w", err)
	}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 8*1024*1024) // up to 8MB per line
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	closeErr := f.Close()
	if err := scanner.Err(); err != nil {
		return 0, false, fmt.Errorf("scan: %w", err)
	}
	if closeErr != nil {
		return 0, false, fmt.Errorf("close: %w", closeErr)
	}
	if len(lines) <= maxEntries {
		return 0, false, nil
	}

	removedCount := len(lines) - maxEntries
	kept := lines[removedCount:]

	// Write atomically via temp file + rename to avoid truncation on crash.
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".compact-*")
	if err != nil {
		return 0, false, fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	w := bufio.NewWriter(tmp)
	for _, line := range kept {
		if _, err := w.WriteString(line); err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
			return 0, false, fmt.Errorf("write: %w", err)
		}
		if err := w.WriteByte('\n'); err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
			return 0, false, fmt.Errorf("write newline: %w", err)
		}
	}
	if err := w.Flush(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return 0, false, fmt.Errorf("flush: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return 0, false, fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return 0, false, fmt.Errorf("rename: %w", err)
	}
	return removedCount, true, nil
}
