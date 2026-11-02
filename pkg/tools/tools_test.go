// ABOUTME: Tests for the file tools exposed to the model.
// ABOUTME: Each test works inside a temporary workspace directory.
package tools

import (
	"encoding/json"
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
