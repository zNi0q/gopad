package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// maxDiskFileBytes bounds what the folder workspace will open as text. Large
// enough for any real note or source file, small enough that a stray binary
// or log dump can't freeze the UI trying to render it.
const maxDiskFileBytes = 5 << 20

// DirEntry is one row of a folder-workspace tree listing.
type DirEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
}

// IsDir reports whether path is a directory — used to route a dragged-and-
// dropped path to either "import as a note" or "open as a folder".
func (a *App) IsDir(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}

// PickFolder opens a native directory picker for the folder workspace.
// Returns "" if the user cancelled.
func (a *App) PickFolder() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Open Folder",
	})
}

// ListDir lists the immediate children of path for the folder tree —
// directories first, then both groups alphabetical. Dotfiles/dotdirs are
// skipped, the same convention most file explorers use to hide VCS/tooling
// clutter like .git.
func (a *App) ListDir(path string) ([]DirEntry, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	out := make([]DirEntry, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		out = append(out, DirEntry{Name: name, Path: filepath.Join(path, name), IsDir: e.IsDir()})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// ReadDiskFile reads a real file from disk for the folder-workspace editor.
// This is separate from note content, which always lives in SQLite.
func (a *App) ReadDiskFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%q is a folder, not a file", path)
	}
	if info.Size() > maxDiskFileBytes {
		return "", fmt.Errorf("file is too large to open (%d bytes)", info.Size())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WriteDiskFile saves the folder-workspace editor's content back to the exact
// file it was opened from, preserving that file's existing permissions.
func (a *App) WriteDiskFile(path, content string) error {
	mode := os.FileMode(0644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode()
	}
	return os.WriteFile(path, []byte(content), mode)
}
