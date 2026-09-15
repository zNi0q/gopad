package main

import (
	"os"
	"path/filepath"
	"testing"
)

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestListDirSortsDirsFirstAndSkipsDotfiles(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "b.txt"), "b")
	mustWriteFile(t, filepath.Join(dir, "a.txt"), "a")
	mustWriteFile(t, filepath.Join(dir, ".hidden"), "x")
	if err := os.Mkdir(filepath.Join(dir, "zsub"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	a := &App{}
	entries, err := a.ListDir(dir)
	if err != nil {
		t.Fatalf("ListDir: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries = %#v, want 3 (dotfile skipped)", entries)
	}
	if !entries[0].IsDir || entries[0].Name != "zsub" {
		t.Fatalf("entries[0] = %#v, want the directory listed first", entries[0])
	}
	if entries[1].Name != "a.txt" || entries[2].Name != "b.txt" {
		t.Fatalf("file order = %q, %q, want alphabetical", entries[1].Name, entries[2].Name)
	}
}

func TestReadWriteDiskFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	mustWriteFile(t, path, "before")

	a := &App{}
	if err := a.WriteDiskFile(path, "after"); err != nil {
		t.Fatalf("WriteDiskFile: %v", err)
	}
	got, err := a.ReadDiskFile(path)
	if err != nil {
		t.Fatalf("ReadDiskFile: %v", err)
	}
	if got != "after" {
		t.Fatalf("content = %q, want %q", got, "after")
	}
}

func TestReadDiskFileRejectsOversized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.txt")
	if err := os.WriteFile(path, make([]byte, maxDiskFileBytes+1), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	a := &App{}
	if _, err := a.ReadDiskFile(path); err == nil {
		t.Fatal("ReadDiskFile on an oversized file: want an error, got nil")
	}
}

func TestIsDir(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f.txt")
	mustWriteFile(t, file, "x")

	a := &App{}
	if isDir, err := a.IsDir(dir); err != nil || !isDir {
		t.Fatalf("IsDir(dir) = %v, %v, want true, nil", isDir, err)
	}
	if isDir, err := a.IsDir(file); err != nil || isDir {
		t.Fatalf("IsDir(file) = %v, %v, want false, nil", isDir, err)
	}
}

func TestImportFileCreatesNote(t *testing.T) {
	db, err := initDB(filepath.Join(t.TempDir(), "notes.db"))
	if err != nil {
		t.Fatalf("initDB: %v", err)
	}
	defer db.Close()

	src := filepath.Join(t.TempDir(), "imported.md")
	mustWriteFile(t, src, "# Hello")

	a := &App{db: db}
	note, err := a.importFile(src)
	if err != nil {
		t.Fatalf("importFile: %v", err)
	}
	if note.Title != "imported.md" || note.Content != "# Hello" || note.FilePath != src {
		t.Fatalf("note = %#v, want title/content/path matching the source file", note)
	}

	stored, err := dbGetNote(db, note.ID)
	if err != nil {
		t.Fatalf("dbGetNote: %v", err)
	}
	if stored.Content != "# Hello" {
		t.Fatalf("stored content = %q, want %q", stored.Content, "# Hello")
	}
}

func TestImportFileRejectsDirectory(t *testing.T) {
	db, err := initDB(filepath.Join(t.TempDir(), "notes.db"))
	if err != nil {
		t.Fatalf("initDB: %v", err)
	}
	defer db.Close()

	a := &App{db: db}
	if _, err := a.importFile(t.TempDir()); err == nil {
		t.Fatal("importFile on a directory: want an error, got nil")
	}
}
