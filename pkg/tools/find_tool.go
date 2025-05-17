package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FindFileTool implements the file finding tool
type FindFileTool struct {
	BaseTool
}

func NewFindFileTool() *FindFileTool {
	return &FindFileTool{
		BaseTool: BaseTool{
			Name:        "find_file",
			Description: "Find files in a directory that match a name pattern. Supports glob patterns like *.txt or *test*.go",
			InputSchema: generateSchema[FindFileInput](),
			ExecuteFn:   findFile,
		},
	}
}

type FindFileInput struct {
	Dir     string `json:"dir" jsonschema_description:"The directory to search in (defaults to current directory if empty)"`
	Pattern string `json:"pattern" jsonschema_description:"The file name pattern to match (e.g., *.txt, *test*.go)"`
}

func findFile(input json.RawMessage) (string, error) {
	var findInput FindFileInput
	if err := json.Unmarshal(input, &findInput); err != nil {
		return "", err
	}

	// Validate input
	if findInput.Pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}

	// Set default directory to current directory if not specified
	dir := "."
	if findInput.Dir != "" {
		dir = findInput.Dir
	}

	// Verify directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return "", fmt.Errorf("directory does not exist: %s", dir)
	}

	// Find matching files
	var matches []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		// Check if file matches pattern
		matched, err := filepath.Match(findInput.Pattern, relPath)
		if err != nil {
			return err
		}

		if matched {
			matches = append(matches, relPath)
		}
		return nil
	})

	if err != nil {
		return "", fmt.Errorf("error searching directory: %w", err)
	}

	// Format the results
	if len(matches) == 0 {
		return fmt.Sprintf("No files found matching pattern '%s' in directory '%s'", findInput.Pattern, dir), nil
	}

	result := fmt.Sprintf("Found %d files matching pattern '%s':\n%s",
		len(matches),
		findInput.Pattern,
		strings.Join(matches, "\n"))
	return result, nil
}
