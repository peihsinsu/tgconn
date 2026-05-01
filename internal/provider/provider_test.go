package provider

import (
	"testing"
)

func TestNew_Claude(t *testing.T) {
	p, err := New("claude")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Error("expected non-nil provider")
	}
}

func TestNew_Unsupported(t *testing.T) {
	_, err := New("unknown")
	if err == nil {
		t.Error("expected error for unsupported provider")
	}
}

func TestNew_Codex(t *testing.T) {
	p, err := New("codex")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Error("expected non-nil provider")
	}
}

func TestNew_GeminiNotImplemented(t *testing.T) {
	_, err := New("gemini")
	if err == nil {
		t.Error("expected error: gemini not yet implemented")
	}
}
