package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestMigrateLegacy_NoLegacy(t *testing.T) {
	tmp := t.TempDir()
	legacy := filepath.Join(tmp, "src", ".tgconn") // does not exist
	target := filepath.Join(tmp, "dst", "project")

	moved, err := MigrateLegacy(legacy, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if moved {
		t.Error("moved=true but no legacy directory existed")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("target dir created unexpectedly: %v", err)
	}
}

func TestMigrateLegacy_HappyPath(t *testing.T) {
	tmp := t.TempDir()
	legacy := filepath.Join(tmp, "src", ".tgconn")
	target := filepath.Join(tmp, "dst", "project")

	writeFile(t, filepath.Join(legacy, "2026-05-01.jsonl"), "log entry\n")
	writeFile(t, filepath.Join(legacy, "history_42.jsonl"), "{}\n")
	writeFile(t, filepath.Join(legacy, "cron", "abc123.json"), "{}\n")
	writeFile(t, filepath.Join(legacy, "tmp", "42", "photo.jpg"), "binary")

	moved, err := MigrateLegacy(legacy, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !moved {
		t.Fatal("expected moved=true")
	}

	// Target should now contain everything.
	for _, rel := range []string{
		"2026-05-01.jsonl",
		"history_42.jsonl",
		"cron/abc123.json",
		"tmp/42/photo.jpg",
	} {
		if _, err := os.Stat(filepath.Join(target, rel)); err != nil {
			t.Errorf("expected %s in target, got: %v", rel, err)
		}
	}

	// Legacy dir should be empty except for the breadcrumb.
	entries, err := os.ReadDir(legacy)
	if err != nil {
		t.Fatalf("read legacy: %v", err)
	}
	if len(entries) != 1 || !strings.HasPrefix(entries[0].Name(), breadcrumbPrefix) {
		t.Errorf("expected single breadcrumb file in legacy, got: %v", entriesNames(entries))
	}
	data, err := os.ReadFile(filepath.Join(legacy, entries[0].Name()))
	if err != nil {
		t.Fatalf("read breadcrumb: %v", err)
	}
	if !strings.Contains(string(data), target) {
		t.Errorf("breadcrumb missing target path: %s", data)
	}
}

func TestMigrateLegacy_Reentrant(t *testing.T) {
	tmp := t.TempDir()
	legacy := filepath.Join(tmp, "src", ".tgconn")
	target := filepath.Join(tmp, "dst", "project")

	writeFile(t, filepath.Join(legacy, "2026-05-01.jsonl"), "log\n")

	if _, err := MigrateLegacy(legacy, target); err != nil {
		t.Fatalf("first migration: %v", err)
	}

	// Run again — now legacy contains only a breadcrumb. Should noop.
	moved, err := MigrateLegacy(legacy, target)
	if err != nil {
		t.Fatalf("second migration: %v", err)
	}
	if moved {
		t.Error("re-run should not move anything")
	}
}

func TestMigrateLegacy_ConflictBothPopulated(t *testing.T) {
	tmp := t.TempDir()
	legacy := filepath.Join(tmp, "src", ".tgconn")
	target := filepath.Join(tmp, "dst", "project")

	writeFile(t, filepath.Join(legacy, "old.jsonl"), "legacy\n")
	writeFile(t, filepath.Join(target, "new.jsonl"), "centralized\n")

	moved, err := MigrateLegacy(legacy, target)
	if err == nil {
		t.Fatal("expected error when both dirs populated")
	}
	if moved {
		t.Error("moved should be false on conflict")
	}
	if !strings.Contains(err.Error(), "refusing to migrate") {
		t.Errorf("error should explain conflict, got: %v", err)
	}
	// Both originals must remain untouched.
	if _, err := os.Stat(filepath.Join(legacy, "old.jsonl")); err != nil {
		t.Errorf("legacy file disappeared: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "new.jsonl")); err != nil {
		t.Errorf("target file disappeared: %v", err)
	}
}

func TestMigrateLegacy_EmptyTargetTolerated(t *testing.T) {
	tmp := t.TempDir()
	legacy := filepath.Join(tmp, "src", ".tgconn")
	target := filepath.Join(tmp, "dst", "project")

	writeFile(t, filepath.Join(legacy, "data.jsonl"), "x")
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}

	moved, err := MigrateLegacy(legacy, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !moved {
		t.Fatal("expected migration to proceed when target is empty")
	}
	if _, err := os.Stat(filepath.Join(target, "data.jsonl")); err != nil {
		t.Errorf("data did not arrive at target: %v", err)
	}
}

func TestCopyTreeFallback(t *testing.T) {
	// Exercise the copy path directly to give us coverage on systems where
	// os.Rename always succeeds.
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dst := filepath.Join(tmp, "dst")

	writeFile(t, filepath.Join(src, "a.txt"), "a")
	writeFile(t, filepath.Join(src, "sub", "b.txt"), "b")

	if err := copyTree(src, dst); err != nil {
		t.Fatalf("copyTree: %v", err)
	}
	for _, rel := range []string{"a.txt", "sub/b.txt"} {
		got, err := os.ReadFile(filepath.Join(dst, rel))
		if err != nil {
			t.Errorf("missing %s: %v", rel, err)
			continue
		}
		want := filepath.Base(rel)[:1]
		if string(got) != want {
			t.Errorf("%s: got %q want %q", rel, got, want)
		}
	}
}

func entriesNames(entries []os.DirEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}
