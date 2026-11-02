// ABOUTME: File tools for reading, listing, and editing files inside the workspace.
// ABOUTME: Also defines shared size limits and JSON schema generation for tool inputs.
package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/invopop/jsonschema"
)

// ReadFileTool implements the file reading tool
type ReadFileTool struct {
	BaseTool
}

func NewReadFileTool(workspaceRoot string) *ReadFileTool {
	return &ReadFileTool{
		BaseTool: BaseTool{
			Name:        "read_file",
			Description: "Read the contents of a given relative file path. Use this when you want to see what's inside a file. Do not use this with directory names.",
			InputSchema: generateSchema[ReadFileInput](),
			ExecuteFn:   func(input json.RawMessage) (string, error) { return readFile(workspaceRoot, input) },
		},
	}
}

type ReadFileInput struct {
	Path string `json:"path" jsonschema_description:"The relative path of a file in the working directory."`
}

func readFile(workspaceRoot string, input json.RawMessage) (string, error) {
	var readFileInput ReadFileInput
	if err := json.Unmarshal(input, &readFileInput); err != nil {
		return "", err
	}
	path, err := resolveInWorkspace(workspaceRoot, readFileInput.Path)
	if err != nil {
		return "", err
	}
	return readLimited(path)
}

// Limits that keep a single tool result from exhausting memory or the model's context
const (
	maxReadBytes   = 256 * 1024
	maxListEntries = 1000
)

// readLimited reads at most maxReadBytes from path and notes when the file was cut short
func readLimited(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, maxReadBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxReadBytes {
		return string(data[:maxReadBytes]) + fmt.Sprintf("\n[truncated: showing the first %d bytes; use search_file to find specific content]", maxReadBytes), nil
	}
	return string(data), nil
}

// ListFilesTool implements the directory listing tool
type ListFilesTool struct {
	BaseTool
}

func NewListFilesTool(workspaceRoot string) *ListFilesTool {
	return &ListFilesTool{
		BaseTool: BaseTool{
			Name:        "list_files",
			Description: "List files and directories at a given path. If no path is provided, lists files in the current directory.",
			InputSchema: generateSchema[ListFilesInput](),
			ExecuteFn:   func(input json.RawMessage) (string, error) { return listFiles(workspaceRoot, input) },
		},
	}
}

type ListFilesInput struct {
	Path string `json:"path,omitempty" jsonschema_description:"Optional relative path to list files from. Defaults to current directory if not provided."`
}

func listFiles(workspaceRoot string, input json.RawMessage) (string, error) {
	var listFilesInput ListFilesInput
	if err := json.Unmarshal(input, &listFilesInput); err != nil {
		return "", err
	}

	dir, err := resolveInWorkspace(workspaceRoot, listFilesInput.Path)
	if err != nil {
		return "", err
	}

	var files []string
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		if len(files) >= maxListEntries {
			files = append(files, fmt.Sprintf("[truncated after %d entries]", maxListEntries))
			return filepath.SkipAll
		}

		if relPath != "." {
			if info.IsDir() {
				files = append(files, relPath+"/")
			} else {
				files = append(files, relPath)
			}
		}
		return nil
	})

	if err != nil {
		return "", err
	}

	result, err := json.Marshal(files)
	if err != nil {
		return "", err
	}

	return string(result), nil
}

// EditFileTool implements the file editing tool
type EditFileTool struct {
	BaseTool
}

func NewEditFileTool(workspaceRoot string) *EditFileTool {
	return &EditFileTool{
		BaseTool: BaseTool{
			Name: "edit_file",
			Description: `Make edits to a text file.

Replaces 'old_str' with 'new_str' in the given file. 'old_str' and 'new_str' MUST be different from each other.

If the file specified with path doesn't exist, it will be created.`,
			InputSchema: generateSchema[EditFileInput](),
			ExecuteFn:   func(input json.RawMessage) (string, error) { return editFile(workspaceRoot, input) },
		},
	}
}

type EditFileInput struct {
	Path   string `json:"path" jsonschema_description:"The path to the file"`
	OldStr string `json:"old_str" jsonschema_description:"Text to search for - must match exactly and must only have one match exactly"`
	NewStr string `json:"new_str" jsonschema_description:"Text to replace old_str with"`
}

func editFile(workspaceRoot string, input json.RawMessage) (string, error) {
	var editFileInput EditFileInput
	if err := json.Unmarshal(input, &editFileInput); err != nil {
		return "", err
	}

	if editFileInput.Path == "" || editFileInput.OldStr == editFileInput.NewStr {
		return "", fmt.Errorf("invalid input parameters")
	}

	path, err := resolveInWorkspace(workspaceRoot, editFileInput.Path)
	if err != nil {
		return "", err
	}

	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && editFileInput.OldStr == "" {
			return createNewFile(path, editFileInput.NewStr)
		}
		return "", err
	}

	oldContent := string(content)

	// The description promises exactly one match, so refuse ambiguous or empty patterns
	if editFileInput.OldStr == "" {
		return "", fmt.Errorf("old_str is required when editing an existing file")
	}
	switch n := strings.Count(oldContent, editFileInput.OldStr); n {
	case 0:
		return "", fmt.Errorf("old_str not found in file")
	case 1:
	default:
		return "", fmt.Errorf("old_str matches %d times; include more surrounding text so it matches exactly once", n)
	}
	newContent := strings.Replace(oldContent, editFileInput.OldStr, editFileInput.NewStr, 1)

	err = os.WriteFile(path, []byte(newContent), 0644)
	if err != nil {
		return "", err
	}

	return "OK", nil
}

func createNewFile(filePath, content string) (string, error) {
	dir := path.Dir(filePath)
	if dir != "." {
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			return "", fmt.Errorf("failed to create directory: %w", err)
		}
	}

	err := os.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}

	return fmt.Sprintf("Successfully created file %s", filePath), nil
}

func generateSchema[T any]() json.RawMessage {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	var v T
	schema := reflector.Reflect(v)
	schemaBytes, err := json.Marshal(schema)
	if err != nil {
		// Schemas are reflected from static types at startup, so this is a programming error
		panic(fmt.Sprintf("failed to marshal schema for %T: %v", v, err))
	}
	return schemaBytes
}
