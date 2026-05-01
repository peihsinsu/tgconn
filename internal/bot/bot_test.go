package bot

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSplitMessage_Short(t *testing.T) {
	chunks := splitMessage("hello world")
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0] != "hello world" {
		t.Errorf("unexpected content: %q", chunks[0])
	}
}

func TestSplitMessage_Exact(t *testing.T) {
	text := strings.Repeat("a", maxMessageLen)
	chunks := splitMessage(text)
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk for exact-length text, got %d", len(chunks))
	}
}

func TestSplitMessage_Split(t *testing.T) {
	text := strings.Repeat("a", maxMessageLen+1)
	chunks := splitMessage(text)
	if len(chunks) != 2 {
		t.Errorf("expected 2 chunks, got %d", len(chunks))
	}
	total := 0
	for _, c := range chunks {
		l := utf8.RuneCountInString(c)
		if l > maxMessageLen {
			t.Errorf("chunk exceeds max length: %d", l)
		}
		total += l
	}
	if total != maxMessageLen+1 {
		t.Errorf("expected total runes %d, got %d", maxMessageLen+1, total)
	}
}

func TestSplitMessage_Unicode(t *testing.T) {
	// Each Chinese char is 1 rune but 3 bytes — split must count runes, not bytes
	text := strings.Repeat("中", maxMessageLen+10)
	chunks := splitMessage(text)
	for i, chunk := range chunks {
		l := utf8.RuneCountInString(chunk)
		if l > maxMessageLen {
			t.Errorf("chunk %d exceeds max rune length: %d", i, l)
		}
	}
}

func TestSplitMessage_Empty(t *testing.T) {
	chunks := splitMessage("")
	if len(chunks) != 1 || chunks[0] != "" {
		t.Errorf("expected single empty chunk, got %v", chunks)
	}
}
