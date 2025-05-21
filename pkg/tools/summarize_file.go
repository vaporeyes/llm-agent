package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type SummarizeFileTool struct {
	workspaceRoot string
}

type SummarizeFileInput struct {
	Path string `json:"path"`
}

func NewSummarizeFileTool(workspaceRoot string) *SummarizeFileTool {
	return &SummarizeFileTool{
		workspaceRoot: workspaceRoot,
	}
}

func (t *SummarizeFileTool) GetName() string {
	return "summarize_file"
}

func (t *SummarizeFileTool) GetDescription() string {
	return "Summarizes the contents of a file, providing a brief overview of its structure and purpose. Use this when you want to understand what a file does without reading its entire contents."
}

func (t *SummarizeFileTool) GetInputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Path to the file to summarize, relative to the workspace root"
			}
		},
		"required": ["path"]
	}`)
}

func (t *SummarizeFileTool) Execute(input json.RawMessage) (string, error) {
	var params SummarizeFileInput
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("failed to parse input: %w", err)
	}

	// Validate and sanitize the path
	absPath, err := filepath.Abs(filepath.Join(t.workspaceRoot, params.Path))
	if err != nil {
		return "", fmt.Errorf("error getting filepath")
	}

	// Check if file exists
	fileInfo, err := os.Stat(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to access file: %w", err)
	}
	if fileInfo.IsDir() {
		return "", fmt.Errorf("path is a directory, not a file")
	}

	// Read the file
	file, err := os.Open(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Read first 1000 bytes for analysis
	buffer := make([]byte, 1000)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Analyze file content
	content := string(buffer[:n])
	fileType := filepath.Ext(absPath)

	// Generate summary based on file type
	var summary string
	switch fileType {
	case ".go":
		summary = analyzeGoFile(content)
	case ".py":
		summary = analyzePythonFile(content)
	case ".js", ".ts", ".jsx", ".tsx":
		summary = analyzeJavaScriptFile(content)
	case ".md":
		summary = analyzeMarkdownFile(content)
	default:
		summary = analyzeGenericFile(content)
	}

	// Add file metadata
	summary = fmt.Sprintf("File: %s\nSize: %d bytes\nType: %s\n\n%s",
		params.Path,
		fileInfo.Size(),
		fileType,
		summary,
	)

	return summary, nil
}

func analyzeGoFile(content string) string {
	// Count imports, functions, and structs
	imports := strings.Count(content, "import")
	functions := strings.Count(content, "func")
	structs := strings.Count(content, "type") - strings.Count(content, "type interface")

	return fmt.Sprintf("Go file analysis:\n- %d imports\n- %d functions\n- %d structs/interfaces\n\nFirst few lines:\n%s",
		imports, functions, structs, getFirstLines(content, 5))
}

func analyzePythonFile(content string) string {
	// Count imports, functions, and classes
	imports := strings.Count(content, "import") + strings.Count(content, "from")
	functions := strings.Count(content, "def")
	classes := strings.Count(content, "class")

	return fmt.Sprintf("Python file analysis:\n- %d imports\n- %d functions\n- %d classes\n\nFirst few lines:\n%s",
		imports, functions, classes, getFirstLines(content, 5))
}

func analyzeJavaScriptFile(content string) string {
	// Count imports, functions, and classes
	imports := strings.Count(content, "import") + strings.Count(content, "require")
	functions := strings.Count(content, "function") + strings.Count(content, "=>")
	classes := strings.Count(content, "class")

	return fmt.Sprintf("JavaScript/TypeScript file analysis:\n- %d imports\n- %d functions\n- %d classes\n\nFirst few lines:\n%s",
		imports, functions, classes, getFirstLines(content, 5))
}

func analyzeMarkdownFile(content string) string {
	// Count headers and list items
	headers := strings.Count(content, "#")
	listItems := strings.Count(content, "- ") + strings.Count(content, "* ")

	return fmt.Sprintf("Markdown file analysis:\n- %d headers\n- %d list items\n\nFirst few lines:\n%s",
		headers, listItems, getFirstLines(content, 5))
}

func analyzeGenericFile(content string) string {
	// Basic analysis for unknown file types
	lines := strings.Count(content, "\n")
	words := len(strings.Fields(content))

	return fmt.Sprintf("File analysis:\n- %d lines\n- %d words\n\nFirst few lines:\n%s",
		lines, words, getFirstLines(content, 5))
}

func getFirstLines(content string, n int) string {
	lines := strings.Split(content, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
