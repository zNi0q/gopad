package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// lockSession is the decryption key for the one locked note currently revealed.
// It lives only in memory and is dropped as soon as the user leaves the note, so
// a locked note is readable exactly while it is open.
type lockSession struct {
	id   int64
	key  []byte
	salt []byte
}

// App is the Wails-bound backend. Every exported method becomes callable from
// the frontend as window.go.main.App.<Method>(), returning a Promise that
// resolves with the return value or rejects with the returned error.
type App struct {
	ctx context.Context
	db  *sql.DB

	mu      sync.Mutex
	session *lockSession
}

func NewApp() *App { return &App{} }

// startup opens the database. Wails calls it once the runtime is ready, so the
// context is valid for native dialogs and events.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	db, err := initDB(getDBPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "gopad: failed to open database: %v\n", err)
		os.Exit(1)
	}
	a.db = db
	cleanupOldBinary()
	a.watchUpdates()
}

func (a *App) shutdown(ctx context.Context) {
	if a.db != nil {
		a.db.Close()
	}
}

func getDBPath() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".local", "share", "gopad")
	if err := os.MkdirAll(dir, 0755); err != nil {
		dir = "."
	}
	return filepath.Join(dir, "notes.db")
}

// ── Notes ──────────────────────────────────────────────────────────────────

func (a *App) ListNotes() ([]Note, error) {
	return hideLocked(dbListNotes(a.db))
}

func (a *App) ListArchivedNotes() ([]Note, error) {
	return hideLocked(dbListArchivedNotes(a.db))
}

// hideLocked strips ciphertext from list results. Locked notes still show their
// title and dates in the sidebar; only the body is withheld, which also keeps
// them out of content search until they are revealed.
func hideLocked(notes []Note, err error) ([]Note, error) {
	if err != nil {
		return nil, err
	}
	for i := range notes {
		if notes[i].Locked {
			notes[i].Content = ""
		}
	}
	return notes, nil
}

// GetNote returns plaintext for an unlocked note, or for a locked note that the
// current session has revealed. Otherwise the body comes back empty — the
// frontend must call RevealNote with the passphrase.
func (a *App) GetNote(id int64) (*Note, error) {
	n, err := dbGetNote(a.db, id)
	if err != nil {
		return nil, err
	}
	if !n.Locked {
		return n, nil
	}
	if plain, ok := a.decryptWithSession(n); ok {
		n.Content = plain
		return n, nil
	}
	n.Content = ""
	return n, nil
}

func (a *App) CreateNote() (*Note, error) {
	return dbCreateNote(a.db)
}

// UpdateNote autosaves. For a revealed locked note the content is re-encrypted
// under the live session key (fresh nonce, same salt) before it touches disk;
// for a locked note with no session the write is rejected.
func (a *App) UpdateNote(id int64, title, content string) error {
	a.mu.Lock()
	s := a.session
	a.mu.Unlock()

	if s != nil && s.id == id {
		sealed, err := sealContent(content, s.key, s.salt)
		if err != nil {
			return err
		}
		return dbUpdateLockedNote(a.db, id, title, sealed)
	}
	return dbUpdateNote(a.db, id, title, content)
}

func (a *App) DeleteNote(id int64) error {
	return dbDeleteNote(a.db, id)
}

func (a *App) ArchiveNote(id int64) error {
	return dbSetArchived(a.db, id, true)
}

func (a *App) RestoreNote(id int64) error {
	return dbSetArchived(a.db, id, false)
}

// ── Locking ────────────────────────────────────────────────────────────────

// LockNote encrypts the note's content with a key derived from passphrase and
// clears the plaintext from the database. There is no recovery path: lose the
// passphrase and the content is unreadable.
func (a *App) LockNote(id int64, passphrase string) error {
	if passphrase == "" {
		return errEmptyPass
	}
	n, err := dbGetNote(a.db, id)
	if err != nil {
		return err
	}
	if n.Locked {
		return errors.New("note is already locked")
	}
	salt, err := newSalt()
	if err != nil {
		return err
	}
	key, err := deriveKey(passphrase, salt)
	if err != nil {
		return err
	}
	sealed, err := sealContent(n.Content, key, salt)
	if err != nil {
		return err
	}
	if err := dbSetLockedContent(a.db, id, sealed, true); err != nil {
		return err
	}
	a.clearSessionFor(id)
	return nil
}

// RevealNote decrypts a locked note and keeps the key in memory so the note can
// be read and edited. The key is dropped by RelockNote when the user navigates
// away, so the passphrase is required again next time.
func (a *App) RevealNote(id int64, passphrase string) (string, error) {
	n, err := dbGetNote(a.db, id)
	if err != nil {
		return "", err
	}
	if !n.Locked {
		return n.Content, nil
	}
	salt, err := saltOf(n.Content)
	if err != nil {
		return "", err
	}
	key, err := deriveKey(passphrase, salt)
	if err != nil {
		return "", err
	}
	plain, err := openContent(n.Content, key)
	if err != nil {
		return "", err // wrong passphrase
	}
	a.mu.Lock()
	a.session = &lockSession{id: id, key: key, salt: salt}
	a.mu.Unlock()
	return plain, nil
}

// RelockNote forgets the in-memory key. The frontend calls it whenever the
// revealed note stops being the active note.
func (a *App) RelockNote() {
	a.clearSession()
}

// RemoveLock decrypts the note permanently and clears the lock flag. It requires
// the note to be revealed first, so it cannot be used to strip a lock without
// the passphrase.
func (a *App) RemoveLock(id int64) error {
	n, err := dbGetNote(a.db, id)
	if err != nil {
		return err
	}
	if !n.Locked {
		return nil
	}
	plain, ok := a.decryptWithSession(n)
	if !ok {
		return errNoteLocked
	}
	if err := dbSetLockedContent(a.db, id, plain, false); err != nil {
		return err
	}
	a.clearSessionFor(id)
	return nil
}

// IsRevealed reports whether the note is currently readable in this session.
func (a *App) IsRevealed(id int64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.session != nil && a.session.id == id
}

func (a *App) clearSession() {
	a.mu.Lock()
	a.session = nil
	a.mu.Unlock()
}

// clearSessionFor drops the key only if it belongs to this note. Locking or
// unlocking one note must not revoke the reveal on a different one that is open
// and being edited.
func (a *App) clearSessionFor(id int64) {
	a.mu.Lock()
	if a.session != nil && a.session.id == id {
		a.session = nil
	}
	a.mu.Unlock()
}

// decryptWithSession returns the note's plaintext if the live session holds its
// key, reporting false when the note is locked and no key is held.
func (a *App) decryptWithSession(n *Note) (string, bool) {
	a.mu.Lock()
	s := a.session
	a.mu.Unlock()
	if s == nil || s.id != n.ID {
		return "", false
	}
	plain, err := openContent(n.Content, s.key)
	if err != nil {
		return "", false
	}
	return plain, true
}

// ── Highlights ─────────────────────────────────────────────────────────────

// GetHighlights withholds a locked note's highlights until it is revealed —
// their excerpts would otherwise quote the encrypted content back to the reader.
func (a *App) GetHighlights(noteId int64) ([]Highlight, error) {
	if locked, err := a.lockedAndHidden(noteId); err != nil {
		return nil, err
	} else if locked {
		return []Highlight{}, nil
	}
	return dbGetHighlights(a.db, noteId)
}

func (a *App) AddHighlight(noteId int64, selText, color, comment string, offStart int64) (*Highlight, error) {
	if locked, err := a.lockedAndHidden(noteId); err != nil {
		return nil, err
	} else if locked {
		return nil, errNoteLocked
	}
	return dbAddHighlight(a.db, noteId, selText, color, comment, offStart)
}

// lockedAndHidden reports whether the note is locked without a live session.
func (a *App) lockedAndHidden(id int64) (bool, error) {
	n, err := dbGetNote(a.db, id)
	if err != nil {
		return false, err
	}
	return n.Locked && !a.IsRevealed(id), nil
}

func (a *App) DeleteHighlight(id int64) error {
	return dbDeleteHighlight(a.db, id)
}

// ── Settings ───────────────────────────────────────────────────────────────

func (a *App) GetSetting(key string) (string, error) {
	return dbGetSetting(a.db, key)
}

func (a *App) SetSetting(key, value string) error {
	return dbSetSetting(a.db, key, value)
}

// ── File import / export (native Wails dialogs) ────────────────────────────

var fileDialogFilters = []runtime.FileFilter{
	{DisplayName: "Text & Code", Pattern: "*.txt;*.md;*.log;*.csv;*.json;*.yaml;*.toml;*.go;*.py;*.js;*.ts;*.html;*.css;*.sh;*.rs;*.c;*.cpp;*.h"},
	{DisplayName: "All files", Pattern: "*"},
}

// SaveToFile exports the note's content to a user-chosen path and remembers it.
// Returns the chosen path, or "" if the user cancelled.
func (a *App) SaveToFile(id int64) (string, error) {
	note, err := dbGetNote(a.db, id)
	if err != nil {
		return "", err
	}
	if note.Locked {
		plain, ok := a.decryptWithSession(note)
		if !ok {
			return "", errNoteLocked // never write ciphertext out to a file
		}
		note.Content = plain
	}

	startName := note.FilePath
	if startName == "" {
		startName = note.Title
		if !strings.HasSuffix(startName, ".txt") && !strings.HasSuffix(startName, ".md") {
			startName += ".txt"
		}
	}

	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:                "Save File",
		DefaultFilename:      filepath.Base(startName),
		CanCreateDirectories: true,
		Filters:              fileDialogFilters,
	})
	if err != nil || path == "" {
		return "", err // empty path == cancelled
	}

	if err := os.WriteFile(path, []byte(note.Content), 0644); err != nil {
		return "", err
	}
	if err := dbSetFilePath(a.db, id, path); err != nil {
		return "", err
	}
	return path, nil
}

// ExportContent saves content that has no database row of its own — used to
// export a temporary note (Safe Mode off), which was never written to SQLite.
func (a *App) ExportContent(defaultName, content string) (string, error) {
	if !strings.HasSuffix(defaultName, ".txt") && !strings.HasSuffix(defaultName, ".md") {
		defaultName += ".txt"
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:                "Save File",
		DefaultFilename:      defaultName,
		CanCreateDirectories: true,
		Filters:              fileDialogFilters,
	})
	if err != nil || path == "" {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}
	return path, nil
}

// OpenFile imports a file from disk as a new note. Returns the created note, or
// nil if the user cancelled.
func (a *App) OpenFile() (*Note, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "Open File",
		Filters: fileDialogFilters,
	})
	if err != nil || path == "" {
		return nil, err
	}
	return a.importFile(path)
}

// ImportFile imports a file from disk as a new note, the same as OpenFile but
// given the path directly instead of through the native picker — used for
// drag-and-drop, where Wails already resolves the dropped item's real path.
func (a *App) ImportFile(path string) (*Note, error) {
	return a.importFile(path)
}

func (a *App) importFile(path string) (*Note, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%q is a folder, not a file", path)
	}
	if info.Size() > maxDiskFileBytes {
		return nil, fmt.Errorf("file is too large to import (%d bytes)", info.Size())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	note, err := dbCreateNote(a.db)
	if err != nil {
		return nil, err
	}

	title := filepath.Base(path)
	content := string(data)
	if err := dbUpdateNote(a.db, note.ID, title, content); err != nil {
		return nil, err
	}
	if err := dbSetFilePath(a.db, note.ID, path); err != nil {
		return nil, err
	}

	note.Title = title
	note.Content = content
	note.FilePath = path
	return note, nil
}
