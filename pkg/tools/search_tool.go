package tools

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// SearchFileTool implements the file searching tool
type SearchFileTool struct {
	BaseTool
}

func NewSearchFileTool() *SearchFileTool {
	return &SearchFileTool{
		BaseTool: BaseTool{
			Name:        "search_file",
			Description: "Search for a string or regex pattern within a file and return matching lines. Use regex:true for regex pattern matching.",
			InputSchema: generateSchema[SearchFileInput](),
			ExecuteFn:   searchFile,
		},
	}
}

type SearchFileInput struct {
	Path    string `json:"path" jsonschema_description:"The path of the file to search in"`
	Pattern string `json:"pattern" jsonschema_description:"The string or regex pattern to search for"`
	Regex   bool   `json:"regex" jsonschema_description:"Whether to treat the pattern as a regex (true) or plain string (false)"`
}

func searchFile(input json.RawMessage) (string, error) {
	var searchInput SearchFileInput
	if err := json.Unmarshal(input, &searchInput); err != nil {
		return "", err
	}

	// Validate input
	if searchInput.Path == "" || searchInput.Pattern == "" {
		return "", fmt.Errorf("path and pattern are required")
	}

	// Open the file
	file, err := os.Open(searchInput.Path)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Prepare pattern matching
	var matcher func(string) bool
	if searchInput.Regex {
		re, err := regexp.Compile(searchInput.Pattern)
		if err != nil {
			return "", fmt.Errorf("invalid regex pattern: %w", err)
		}
		matcher = re.MatchString
	} else {
		matcher = func(line string) bool {
			return strings.Contains(line, searchInput.Pattern)
		}
	}

	// Search through the file
	var matches []string
	scanner := bufio.NewScanner(file)
	lineNum := 1
	for scanner.Scan() {
		line := scanner.Text()
		if matcher(line) {
			matches = append(matches, fmt.Sprintf("Line %d: %s", lineNum, line))
		}
		lineNum++
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading file: %w", err)
	}

	// Format the results
	if len(matches) == 0 {
		return "No matches found", nil
	}

	result := fmt.Sprintf("Found %d matches:\n%s", len(matches), strings.Join(matches, "\n"))
	return result, nil
}
