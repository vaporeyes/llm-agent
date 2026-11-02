// ABOUTME: Persists chat history as append-only JSON Lines, one message per line.
// ABOUTME: Records conversation IDs, tool calls, and provider token usage.
package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"llm-agent/pkg/models"

	"github.com/google/uuid"
)

// ChatMessage represents a stored chat message
type ChatMessage struct {
	ID             string    `json:"id"`              // Unique message ID
	ConversationID string    `json:"conversation_id"` // ID linking related messages
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	Timestamp      time.Time `json:"timestamp"`
	Model          string    `json:"model"`
	// ToolCalls and ToolCallID record tool requests and the results that answer them
	ToolCalls  []models.ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
	Usage      struct {
		InputTokens  int64 `json:"input_tokens"`
		OutputTokens int64 `json:"output_tokens"`
	} `json:"usage"`
}

// ChatStorage handles saving chat history
type ChatStorage struct {
	filePath string
}

// NewChatStorage creates a new chat storage instance.
// History is stored as JSON Lines: one ChatMessage object per line, appended in order.
func NewChatStorage(filePath string) (*ChatStorage, error) {
	// Create file if it doesn't exist
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to create chat history file: %w", err)
	}
	defer f.Close()

	// Refuse to append lines to a file written in the older JSON array format
	first := make([]byte, 1)
	if n, _ := f.Read(first); n == 1 && first[0] == '[' {
		return nil, fmt.Errorf("%s uses the old JSON array format; convert it with: jq -c '.[]' %s > %s.jsonl", filePath, filePath, filePath)
	}
	return &ChatStorage{filePath: filePath}, nil
}

// SaveMessage appends a chat message to the storage file
func (s *ChatStorage) SaveMessage(msg models.Message, modelName string, usage models.Usage, conversationID string) error {
	chatMsg := ChatMessage{
		ID:             uuid.New().String(),
		ConversationID: conversationID,
		Role:           msg.Role,
		Content:        msg.Content,
		Timestamp:      time.Now(),
		Model:          modelName,
		ToolCalls:      msg.ToolCalls,
		ToolCallID:     msg.ToolCallID,
	}
	chatMsg.Usage.InputTokens = usage.InputTokens
	chatMsg.Usage.OutputTokens = usage.OutputTokens

	line, err := json.Marshal(chatMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal chat message: %w", err)
	}

	// A single O_APPEND write never rewrites earlier history, so an interrupted
	// save can lose at most the line being written
	f, err := os.OpenFile(s.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open chat history: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("failed to write chat history: %w", err)
	}
	return nil
}
