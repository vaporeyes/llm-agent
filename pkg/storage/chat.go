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
	Usage          struct {
		InputTokens  int64 `json:"input_tokens"`
		OutputTokens int64 `json:"output_tokens"`
	} `json:"usage"`
}

// ChatStorage handles saving chat history
type ChatStorage struct {
	filePath string
}

// NewChatStorage creates a new chat storage instance
func NewChatStorage(filePath string) (*ChatStorage, error) {
	// Create file if it doesn't exist
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		if err := os.WriteFile(filePath, []byte("[]"), 0644); err != nil {
			return nil, fmt.Errorf("failed to create chat history file: %w", err)
		}
	}
	return &ChatStorage{filePath: filePath}, nil
}

// SaveMessage saves a chat message to the storage file
func (s *ChatStorage) SaveMessage(msg models.Message, modelName string, usage models.Usage, conversationID string) error {
	chatMsg := ChatMessage{
		ID:             uuid.New().String(),
		ConversationID: conversationID,
		Role:           msg.Role,
		Content:        msg.Content,
		Timestamp:      time.Now(),
		Model:          modelName,
	}
	chatMsg.Usage.InputTokens = usage.InputTokens
	chatMsg.Usage.OutputTokens = usage.OutputTokens

	// Read existing messages
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return fmt.Errorf("failed to read chat history: %w", err)
	}

	var messages []ChatMessage
	if err := json.Unmarshal(data, &messages); err != nil {
		return fmt.Errorf("failed to parse chat history: %w", err)
	}

	// Append new message
	messages = append(messages, chatMsg)

	// Write back to file
	newData, err := json.MarshalIndent(messages, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal chat history: %w", err)
	}

	if err := os.WriteFile(s.filePath, newData, 0644); err != nil {
		return fmt.Errorf("failed to write chat history: %w", err)
	}

	return nil
}
