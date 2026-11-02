// ABOUTME: Tests for the file tools exposed to the model.
// ABOUTME: Each test works inside a temporary workspace directory.
package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustInput(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestSummarizeGoFileCountsDeclarations(t *testing.T) {
	root := t.TempDir()
	// Comments and identifiers mention func, type, and import, which substring counting miscounted
	writeFile(t, root, "x.go", `package x

import (
	"fmt"
	"os"
)

// This function has a typeName and imports nothing new
type A struct{}
type (
	B int
	C interface{}
)

func funcName() { fmt.Println(os.Args) }
func (A) method() {}
`)
	out, err := NewSummarizeFileTool(root).Execute(mustInput(t, map[string]string{"path": "x.go"}))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"package x", "- 2 imports", "- 2 functions", "- 3 types"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q:\n%s", want, out)
		}
	}
}

func TestReadFileTruncatesLargeFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "big.txt", strings.Repeat("a", maxReadBytes+10))
	out, err := NewReadFileTool().Execute(mustInput(t, map[string]string{"path": filepath.Join(root, "big.txt")}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, strings.Repeat("a", maxReadBytes)+"\n[truncated") {
		t.Fatalf("output not truncated at limit, len=%d", len(out))
	}
}

func TestListFilesStopsAtEntryLimit(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < maxListEntries+50; i++ {
		writeFile(t, root, fmt.Sprintf("f%04d", i), "")
	}
	out, err := NewListFilesTool().Execute(mustInput(t, map[string]string{"path": root}))
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	if err := json.Unmarshal([]byte(out), &files); err != nil {
		t.Fatal(err)
	}
	if len(files) != maxListEntries+1 || !strings.Contains(files[maxListEntries], "truncated") {
		t.Fatalf("got %d entries, last %q", len(files), files[len(files)-1])
	}
}

func TestSearchFileHandlesLongLines(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "min.js", "short\n"+strings.Repeat("z", 100*1024)+"needle\n")
	out, err := NewSearchFileTool().Execute(mustInput(t, map[string]any{"path": filepath.Join(root, "min.js"), "pattern": "needle"}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "Found 1 matches:\nLine 2:") {
		t.Fatalf("out = %.80q", out)
	}
}

func TestEditFileRequiresExactlyOneMatch(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "f.txt")
	writeFile(t, root, "f.txt", "a b a")
	edit := NewEditFileTool()

	for _, tc := range []struct{ old, wantErr string }{
		{"a", "matches 2 times"},
		{"zzz", "not found"},
		{"", "old_str is required"},
	} {
		_, err := edit.Execute(mustInput(t, map[string]string{"path": path, "old_str": tc.old, "new_str": "X"}))
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("old_str %q: err = %v, want %q", tc.old, err, tc.wantErr)
		}
	}
	if data, _ := os.ReadFile(path); string(data) != "a b a" {
		t.Fatalf("file changed after rejected edits: %q", data)
	}

	if _, err := edit.Execute(mustInput(t, map[string]string{"path": path, "old_str": "a b", "new_str": "c"})); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != "c a" {
		t.Fatalf("content = %q", data)
	}
}
