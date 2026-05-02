package downloader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestErrFileTooLarge(t *testing.T) {
	d := &Downloader{}
	_, err := d.Download(context.Background(), "any", maxFileBytes+1, 1, "file.txt")
	if err != ErrFileTooLarge {
		t.Errorf("expected ErrFileTooLarge, got %v", err)
	}
}

func TestDownload_Success(t *testing.T) {
	content := []byte("hello tgconn")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer srv.Close()

	dir := t.TempDir()

	localPath, err := downloadURL(context.Background(), srv.URL, filepath.Join(dir, "42", "out.txt"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("cannot read downloaded file: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", got, content)
	}
}

func TestDownload_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	dir := t.TempDir()
	_, err := downloadURL(context.Background(), srv.URL, filepath.Join(dir, "42", "out.txt"))
	if err == nil {
		t.Error("expected error for HTTP 403")
	}
}
