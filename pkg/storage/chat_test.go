// ABOUTME: Tests for JSON Lines chat history storage.
// ABOUTME: Covers append behavior, file permissions, and legacy format detection.
package storage

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"llm-agent/pkg/models"
)

func TestSaveMessageAppendsJSONLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	s, err := NewChatStorage(path)
	if err != nil {
		t.Fatal(err)
	}
	call := models.ToolCall{ID: "c1", Name: "read_file", Input: json.RawMessage(`{"path":"a"}`)}
	msgs := []models.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", ToolCalls: []models.ToolCall{call}},
		{Role: "tool", Content: "body", ToolCallID: "c1"},
	}
	for _, m := range msgs {
		if err := s.SaveMessage(m, "model", models.Usage{OutputTokens: 2}, "conv"); err != nil {
			t.Fatal(err)
		}
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var got []ChatMessage
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var m ChatMessage
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("line %d: %v", len(got)+1, err)
		}
		got = append(got, m)
	}
	if len(got) != 3 {
		t.Fatalf("got %d lines, want 3", len(got))
	}
	if got[1].ToolCalls[0].Name != "read_file" || got[2].ToolCallID != "c1" || got[0].ConversationID != "conv" {
		t.Errorf("unexpected records: %+v", got)
	}
}

func TestHistoryFileIsOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	if _, err := NewChatStorage(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("perm = %o, want 600", perm)
	}
}

func TestRejectsLegacyArrayFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	if err := os.WriteFile(path, []byte("[]"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := NewChatStorage(path)
	if err == nil || !strings.Contains(err.Error(), "old JSON array format") {
		t.Fatalf("err = %v", err)
	}
}
