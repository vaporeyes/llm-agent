package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ListDirTool struct {
	workspaceRoot string
}

type ListDirInput struct {
	Path string `json:"path"`
}

func NewListDirTool(workspaceRoot string) *ListDirTool {
	return &ListDirTool{
		workspaceRoot: workspaceRoot,
	}
}

func (t *ListDirTool) GetName() string {
	return "list_dir"
}

func (t *ListDirTool) GetDescription() string {
	return "Lists the contents of a directory, showing files and subdirectories. Use this to explore the workspace structure."
}

func (t *ListDirTool) GetInputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Path to the directory to list, relative to the workspace root"
			}
		},
		"required": ["path"]
	}`)
}

func (t *ListDirTool) Execute(input json.RawMessage) (string, error) {
	var params ListDirInput
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("failed to parse input: %w", err)
	}

	// Validate and sanitize the path
	absPath := filepath.Join(t.workspaceRoot, params.Path)
	if !strings.HasPrefix(absPath, t.workspaceRoot) {
		return "", fmt.Errorf("path must be within workspace root")
	}

	// Check if directory exists
	fileInfo, err := os.Stat(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to access directory: %w", err)
	}
	if !fileInfo.IsDir() {
		return "", fmt.Errorf("path is not a directory")
	}

	// Read directory contents
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to read directory: %w", err)
	}

	// Format directory contents
	var output strings.Builder
	output.WriteString(fmt.Sprintf("Contents of %s:\n\n", params.Path))

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue // Skip entries we can't get info for
		}

		// Format the entry
		var size string
		if !entry.IsDir() {
			size = formatSize(info.Size())
		} else {
			size = "<dir>"
		}

		// Add a prefix to indicate type
		prefix := "📄" // file
		if entry.IsDir() {
			prefix = "📁" // directory
		}

		output.WriteString(fmt.Sprintf("%s %s\t%s\n", prefix, entry.Name(), size))
	}

	return output.String(), nil
}

func formatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}
