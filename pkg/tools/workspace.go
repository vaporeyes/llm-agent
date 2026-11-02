// ABOUTME: Confines tool file access to the configured workspace directory.
// ABOUTME: Resolves symlinks so links pointing outside the workspace are rejected.
package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// resolveInWorkspace returns the real absolute path for p, which may be relative to
// root or absolute, and fails if it lies outside root once symlinks are resolved.
// Paths that do not exist yet, such as a file edit_file will create, are checked
// through their nearest existing parent directory.
func resolveInWorkspace(root, p string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("invalid workspace root: %w", err)
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", fmt.Errorf("invalid workspace root: %w", err)
	}

	target := p
	if !filepath.IsAbs(target) {
		target = filepath.Join(rootReal, target)
	}
	real, err := evalExistingSymlinks(filepath.Clean(target))
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(rootReal, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q is outside the workspace", p)
	}
	return real, nil
}

// evalExistingSymlinks resolves symlinks in the longest existing prefix of path
// and appends the remaining, not yet existing, components unchanged
func evalExistingSymlinks(path string) (string, error) {
	real, err := filepath.EvalSymlinks(path)
	if err == nil {
		return real, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return "", err
	}
	realParent, err := evalExistingSymlinks(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(realParent, filepath.Base(path)), nil
}
