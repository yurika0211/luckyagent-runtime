package session

import (
	"fmt"
	"testing"

	"github.com/yurika0211/luckyagent-runtime/provider"
)

func TestGetMessagesPageReturnsPersistedPrefixWhileTailIsUnsaved(t *testing.T) {
	dir := t.TempDir()
	s := NewSession("tail-page", dir)
	for i := 0; i < 3; i++ {
		if err := s.AddMessageWithError("user", fmt.Sprintf("saved-%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	if err := s.AddProviderMessageWithError(provider.Message{Role: "assistant", Content: "only-in-memory"}); err != nil {
		t.Fatal(err)
	}
	if s.MessageCount() != 4 {
		t.Fatalf("memory count = %d, want 4", s.MessageCount())
	}
	if s.persistedCount != 3 {
		t.Fatalf("persisted count = %d, want 3", s.persistedCount)
	}

	page, total, more, err := s.GetMessagesPage(10, 0)
	if err != nil {
		t.Fatalf("GetMessagesPage: %v", err)
	}
	if total != 3 || more || len(page) != 3 {
		t.Fatalf("page total=%d more=%v len=%d", total, more, len(page))
	}
	if page[2].Content != "saved-2" {
		t.Fatalf("last persisted content = %q", page[2].Content)
	}
	for _, msg := range page {
		if msg.Content == "only-in-memory" {
			t.Fatal("unsaved tail was returned as persisted history")
		}
	}
}

func TestLoadMessagesAcceptsUnsavedTailAndRejectsAShortFile(t *testing.T) {
	dir := t.TempDir()
	s := NewSession("tail-load", dir)
	for i := 0; i < 2; i++ {
		if err := s.AddMessageWithError("user", fmt.Sprintf("saved-%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	if err := s.AddMessageWithError("assistant", "only-in-memory"); err != nil {
		t.Fatal(err)
	}

	s.mu.Lock()
	s.messagesLoaded = false
	s.Messages = nil
	// Metadata still names the unsaved tail. The durable count is the prefix.
	s.messageCount = 3
	s.persistedCount = 2
	s.mu.Unlock()
	if err := s.loadMessagesLocked(); err != nil {
		t.Fatalf("load with unsaved tail: %v", err)
	}
	if got := len(s.GetMessages()); got != 2 {
		t.Fatalf("loaded %d messages, want persisted prefix 2", got)
	}

	short := NewSession("short-file", dir)
	if err := short.AddMessageWithError("user", "one"); err != nil {
		t.Fatal(err)
	}
	if err := short.Save(); err != nil {
		t.Fatal(err)
	}
	short.mu.Lock()
	short.messagesLoaded = false
	short.Messages = nil
	short.messageCount = 4
	short.persistedCount = 4
	short.mu.Unlock()
	if err := short.loadMessagesLocked(); err == nil {
		t.Fatal("truncated file was accepted")
	}
}
