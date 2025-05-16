package tools

import (
	"encoding/json"
)

// Tool defines the interface that all tools must implement
type Tool interface {
	// GetName returns the name of the tool
	GetName() string

	// GetDescription returns the description of the tool
	GetDescription() string

	// GetInputSchema returns the JSON schema for the tool's input
	GetInputSchema() json.RawMessage

	// Execute runs the tool with the given input
	Execute(input json.RawMessage) (string, error)
}

// BaseTool provides a basic implementation of the Tool interface
type BaseTool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	ExecuteFn   func(input json.RawMessage) (string, error)
}

func (t *BaseTool) GetName() string {
	return t.Name
}

func (t *BaseTool) GetDescription() string {
	return t.Description
}

func (t *BaseTool) GetInputSchema() json.RawMessage {
	return t.InputSchema
}

func (t *BaseTool) Execute(input json.RawMessage) (string, error) {
	return t.ExecuteFn(input)
}
