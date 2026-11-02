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
	out, err := NewReadFileTool(root).Execute(mustInput(t, map[string]string{"path": "big.txt"}))
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
	out, err := NewListFilesTool(root).Execute(mustInput(t, map[string]string{}))
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
	out, err := NewSearchFileTool(root).Execute(mustInput(t, map[string]any{"path": "min.js", "pattern": "needle"}))
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
	edit := NewEditFileTool(root)

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

// escapeFixture builds root/ws as the workspace with a secret outside it, a sibling
// directory sharing the workspace name as a prefix, and a symlink leading out
func escapeFixture(t *testing.T) (ws string, secret string) {
	t.Helper()
	base := t.TempDir()
	ws = filepath.Join(base, "ws")
	writeFile(t, ws, "inside.txt", "inside content")
	writeFile(t, ws, "sub/nested.txt", "nested content")
	writeFile(t, base, "secret.txt", "SECRET")
	writeFile(t, base, "ws-secrets/key.txt", "SECRET")
	if err := os.Symlink(filepath.Join(base, "secret.txt"), filepath.Join(ws, "link.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(base, filepath.Join(ws, "linkdir")); err != nil {
		t.Fatal(err)
	}
	return ws, filepath.Join(base, "secret.txt")
}

func TestToolsRejectPathsOutsideWorkspace(t *testing.T) {
	ws, secret := escapeFixture(t)
	escapes := []string{
		"../secret.txt",
		"../ws-secrets/key.txt",
		"sub/../../secret.txt",
		secret,
		"link.txt",
		"linkdir/secret.txt",
		"/etc/passwd",
	}
	dirEscapes := []string{"..", "../ws-secrets", "linkdir", "/"}

	fileTools := map[string]func(p string) json.RawMessage{
		"read_file":      func(p string) json.RawMessage { return mustInput(t, map[string]string{"path": p}) },
		"search_file":    func(p string) json.RawMessage { return mustInput(t, map[string]string{"path": p, "pattern": "SECRET"}) },
		"summarize_file": func(p string) json.RawMessage { return mustInput(t, map[string]string{"path": p}) },
		"edit_file": func(p string) json.RawMessage {
			return mustInput(t, map[string]string{"path": p, "old_str": "SECRET", "new_str": "x"})
		},
	}
	dirTools := map[string]func(p string) json.RawMessage{
		"list_dir":   func(p string) json.RawMessage { return mustInput(t, map[string]string{"path": p}) },
		"list_files": func(p string) json.RawMessage { return mustInput(t, map[string]string{"path": p}) },
		"find_file":  func(p string) json.RawMessage { return mustInput(t, map[string]string{"dir": p, "pattern": "*"}) },
	}
	all := map[string]Tool{
		"read_file":      NewReadFileTool(ws),
		"search_file":    NewSearchFileTool(ws),
		"summarize_file": NewSummarizeFileTool(ws),
		"edit_file":      NewEditFileTool(ws),
		"list_dir":       NewListDirTool(ws),
		"list_files":     NewListFilesTool(ws),
		"find_file":      NewFindFileTool(ws),
	}

	check := func(name string, input json.RawMessage, p string) {
		out, err := all[name].Execute(input)
		if err == nil || !strings.Contains(err.Error(), "outside the workspace") {
			t.Errorf("%s(%q): err = %v, out = %.60q", name, p, err, out)
		}
	}
	for name, in := range fileTools {
		for _, p := range escapes {
			check(name, in(p), p)
		}
	}
	for name, in := range dirTools {
		for _, p := range dirEscapes {
			check(name, in(p), p)
		}
	}
	if data, _ := os.ReadFile(secret); string(data) != "SECRET" {
		t.Fatal("secret file was modified")
	}
}

func TestToolsAllowPathsInsideWorkspace(t *testing.T) {
	ws, _ := escapeFixture(t)
	for _, p := range []string{"inside.txt", "./sub/nested.txt", "sub/../inside.txt", filepath.Join(ws, "inside.txt")} {
		out, err := NewReadFileTool(ws).Execute(mustInput(t, map[string]string{"path": p}))
		if err != nil || !strings.HasSuffix(out, "content") {
			t.Errorf("read_file(%q) = %q, %v", p, out, err)
		}
	}
	if _, err := NewListDirTool(ws).Execute(mustInput(t, map[string]string{"path": "."})); err != nil {
		t.Errorf("list_dir(.) = %v", err)
	}
	// edit_file may create files in directories that do not exist yet
	if _, err := NewEditFileTool(ws).Execute(mustInput(t, map[string]string{"path": "new/dir/f.txt", "new_str": "hi"})); err != nil {
		t.Fatalf("create = %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(ws, "new/dir/f.txt")); string(data) != "hi" {
		t.Errorf("created content = %q", data)
	}
}
