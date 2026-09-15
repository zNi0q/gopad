<div align="center">

```
  ██████╗  ██████╗ ██████╗  █████╗ ██████╗
 ██╔════╝ ██╔═══██╗██╔══██╗██╔══██╗██╔══██╗
 ██║  ███╗██║   ██║██████╔╝███████║██║  ██║
 ██║   ██║██║   ██║██╔═══╝ ██╔══██║██║  ██║
 ╚██████╔╝╚██████╔╝██║     ██║  ██║██████╔╝
  ╚═════╝  ╚═════╝ ╚═╝     ╚═╝  ╚═╝╚═════╝
```

**Crash-proof desktop notepad for Linux.**  
Built with Go + Wails (WebKitGTK). No Chromium. No Electron. No data loss.

![Go](https://img.shields.io/badge/Go-1.21+-00ACD7?style=flat-square&logo=go&logoColor=white)
![SQLite](https://img.shields.io/badge/SQLite-WAL-003B57?style=flat-square&logo=sqlite&logoColor=white)
![Platform](https://img.shields.io/badge/Linux-Arch-1793D1?style=flat-square&logo=arch-linux&logoColor=white)
![License](https://img.shields.io/badge/license-MIT-green?style=flat-square)

</div>

---

## The Problem

Every native notepad app on Linux has the same silent flaw: unsaved text lives only in RAM. Suspend, kernel panic, dead battery — it's gone. GoPad treats the database as the primary store and the filesystem as an optional export target. Your keystrokes are durable before you ever think to hit Save.

---

## How It Works

```
Keystroke
    │
    ▼
900 ms debounce timer (resets on each keystroke)
    │
    ▼
SQLite WAL write  ──►  ~/.local/share/gopad/notes.db
    │
    ▼  (optional, user-triggered)
Filesystem export via native Wails Save dialog
```

The UI is HTML/CSS/JS rendered by **WebKitGTK** (the same engine behind GNOME Web), wired up through [**Wails v2**](https://wails.io). Exported methods on the Go `App` struct are bound into the frontend as `window.go.main.App.<Method>()` — each call is a typed Go function invoked as an `async` JS Promise that resolves with the return value or rejects with the error. Native open/save dialogs come from the Wails runtime. There is no HTTP server, no IPC socket, no Electron — the UI and the backend share the same process.

---

## Features

| Category | Details |
|---|---|
| **Durability** | Every keystroke auto-saves to SQLite within 900 ms; WAL mode + 5 s busy-timeout prevents write contention |
| **Multi-note** | Unlimited notes; sidebar sorted by last-modified descending |
| **Archive** | Archive notes out of the main list without deleting them; restore them from the Archive view |
| **Locking** | Lock a note with a passphrase — its content is encrypted at rest (scrypt + AES-256-GCM) and stays unreadable until the passphrase is entered; the key is dropped as soon as you switch notes |
| **Search** | Live full-text sidebar search across title and content |
| **Find & Replace** | In-editor search with match counter (`3/17`), cycle prev/next, replace one or all |
| **File I/O** | Open any text/code file from disk; export/save back via the native Wails file dialog; drag a file or folder onto the window to import it or open it as a workspace |
| **Markdown preview** | Toggle button renders the active note/file as sanitized HTML (GFM tables, task lists, strikethrough); ` ```mermaid ` fences render as diagrams |
| **Safe Mode** | Header toggle — off, New Note creates a temporary note that lives in memory only and disappears when GoPad closes, instead of the default durable DB note |
| **Folder workspace** | Open a real directory as a lazy-loading file tree in its own sidebar tab; edit files in place, autosaving straight to disk |
| **Line numbers** | Rendered in a synced sibling div — scroll-locked to the textarea at all times |
| **Editor** | Monospace font stack, `Tab` → 4 spaces, configurable font size (10–36 px), word wrap toggle |
| **Themes** | Dark (default) and light; selection persisted to the `settings` table |
| **Status bar** | Real-time line/column, word count, character count, linked filename |
| **Undo/Redo** | Full undo history (Ctrl+Z) and redo (Ctrl+Shift+Z / Ctrl+Y); up to 100 states per note |
| **Durability heartbeat** | A live `Safe` indicator in the status bar pulses on every commit to the WAL — the crash-proof promise, made visible |
| **Auto-update** | Checks GitHub Releases hourly; an update pill in the top bar downloads the new build, verifies it against the release `SHA256SUMS`, swaps the binary in place and restarts |
| **No Chromium** | Rendering engine is WebKitGTK; binary depends only on system GTK/WebKit libraries |

---

## Requirements

### Runtime

| Package | Arch | Purpose |
|---|---|---|
| `webkit2gtk-4.1` | `sudo pacman -S webkit2gtk-4.1` | Rendering engine |
| `gtk3` | pulled as a dep | Window and widget toolkit |

### Build

| Tool | Platform | Installation | Purpose |
|---|---|---|---|
| `gcc` | Arch | `sudo pacman -S gcc` | CGO compilation |
| `pkg-config` | Arch | `sudo pacman -S pkgconf` | Library flag resolution |
| MinGW-w64 GCC | Windows | `winget install --id BrechtSanders.WinLibs.POSIX.UCRT -e` | CGO compilation for SQLite |
| Go ≥ 1.21 | All | [go.dev/dl](https://go.dev/dl/) | Compiler |
| Wails CLI *(optional)* | All | `go install github.com/wailsapp/wails/v2/cmd/wails@latest` | Only for `make dev` live-reload |

> **Note** — a plain `make build` does **not** need the Wails CLI. The frontend ships as static
> files under `frontend/dist/` and is embedded with `//go:embed`, so `go build` is enough.

---

## Installation

### From source (recommended)

```bash
git clone <repo-url> gopad
cd gopad

# Fetch Go module dependencies
make deps

# Build
make build

# Optional: install to ~/.local/bin
make install
```

### Quick one-liner

```bash
make deps && make install
```

The `install` target copies the binary to `~/.local/bin/gopad`. Make sure that directory is in your `$PATH`.

### Windows

Open PowerShell in the repository after installing Go and MinGW-w64 GCC, then run:

```powershell
.\build-windows.ps1
.\gopad.exe
```

The build script enables CGO for the SQLite driver, embeds the prebuilt frontend, stamps the current Git version, and creates a GUI executable without a console window. The Wails CLI is not required.

---

## Building Manually

If you prefer not to use Make:

```bash
CGO_ENABLED=1 go build -tags webkit2_41 -o gopad .
```

On Windows, the equivalent manual command is:

```powershell
$env:CGO_ENABLED = "1"
go build -tags "desktop,production" -ldflags "-H windowsgui" -o gopad.exe .
```

> **Why the `webkit2_41` build tag?**
> Wails targets `webkit2gtk-4.0` by default. Arch Linux (and Fedora 37+, Ubuntu 22.10+) ship `webkit2gtk-4.1`. The `webkit2_41` build tag tells Wails to link against the installed 4.1 library — no pkg-config shim required.

---

## Usage

```bash
./gopad
# or, after make install:
gopad
```

The app opens at 1200×800. All notes are loaded from the local database on startup; the most-recently-modified note is opened automatically.

---

## Keyboard Shortcuts

| Shortcut | Action |
|---|---|
| `Ctrl+N` | New note |
| `Ctrl+O` | Open file from disk |
| `Ctrl+S` | Flush pending save + export to file |
| `Ctrl+F` | Open Find & Replace bar |
| `Ctrl+B` | Collapse / expand the notes sidebar |
| `Ctrl+Z` | Undo last change |
| `Ctrl+Shift+Z` / `Ctrl+Y` | Redo previously undone change |
| `Tab` | Insert 4 spaces (does not move focus) |
| `Enter` (in find bar) | Next match |
| `Shift+Enter` (in find bar) | Previous match |
| `Esc` | Close find bar / dismiss modal |

---

## Data & Storage

### Database location

```
~/.local/share/gopad/notes.db
```

SQLite database created automatically on first launch. Two tables:

```sql
-- Every note, whether ever saved to disk or not
CREATE TABLE notes (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    title      TEXT NOT NULL DEFAULT 'Untitled',
    content    TEXT NOT NULL DEFAULT '',
    file_path  TEXT NOT NULL DEFAULT '',  -- empty if never exported
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    archived   INTEGER NOT NULL DEFAULT 0,
    locked     INTEGER NOT NULL DEFAULT 0
);

-- Persisted UI preferences
CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT ''
);
```

### Persisted settings keys

| Key | Values | Default |
|---|---|---|
| `theme` | `dark` \| `light` | `dark` |
| `fontSize` | `10`–`36` (px) | `14` |
| `wordWrap` | `1` \| `0` | `1` |
| `sidebar` | `1` (collapsed) \| `0` | `0` |

### Backup

The database is a single self-contained file. To back up all your notes:

```bash
cp ~/.local/share/gopad/notes.db ~/notes-backup-$(date +%Y%m%d).db
```

To inspect notes directly:

```bash
sqlite3 ~/.local/share/gopad/notes.db "SELECT title, updated_at FROM notes ORDER BY updated_at DESC;"
```

---

## Project Structure

```
gopad/
├── main.go              # Entry point — wails.Run(), embeds frontend/dist, binds App
├── app.go               # App struct: bound methods + native file dialogs (startup/shutdown)
├── db.go                # SQLite schema, typed query helpers
├── frontend/
│   └── dist/
│       └── index.html   # Complete frontend — HTML, CSS, JS; embedded at compile time
├── wails.json           # Wails project config
├── logo.svg             # Standalone 64 px SVG icon (for .desktop files / window managers)
├── Makefile
├── go.mod
└── go.sum
```

### Architecture

```
┌─────────────────────────────────────────────────┐
│                  gopad process                  │
│                                                 │
│  ┌──────────────┐   Wails bindings              │
│  │  Go backend  │ ◄──────────────────────────┐  │
│  │              │   window.go.main.App.*()    │  │
│  │  db.go       │   JSON over IPC bridge       │  │
│  │  app.go      │ ──────────────────────────►│  │
│  │  main.go     │                            │  │
│  └──────────────┘                            │  │
│                                              │  │
│  ┌──────────────────────────────────────┐    │  │
│  │         WebKitGTK renderer           │    │  │
│  │                                      │    │  │
│  │  frontend/dist/index.html            │────┘  │
│  │  (embedded via //go:embed)           │       │
│  │                                      │       │
│  └──────────────────────────────────────┘       │
└─────────────────────────────────────────────────┘
```

The entire frontend is embedded into the binary at compile time via `//go:embed all:frontend/dist`. There are no assets to ship alongside the executable.

---

## JS ↔ Go Bridge Reference

Each exported `App` method is bound by Wails as `window.go.main.App.<Method>()`, an `async` function that resolves with the Go return value or rejects (throws) with the Go error.

| JS Binding | Go Method | Description |
|---|---|---|
| `App.ListNotes()` | `dbListNotes` | All notes, ordered by `updated_at DESC` |
| `App.ListArchivedNotes()` | `dbListArchivedNotes` | Archived notes, ordered by `updated_at DESC` |
| `App.GetNote(id)` | `dbGetNote` | Single note by ID |
| `App.CreateNote()` | `dbCreateNote` | Insert new blank note, return it |
| `App.UpdateNote(id, title, content)` | `dbUpdateNote` | Persist title + content, bump `updated_at` |
| `App.DeleteNote(id)` | `dbDeleteNote` | Hard delete |
| `App.ArchiveNote(id)` | `dbSetArchived` | Move a note out of the main list |
| `App.RestoreNote(id)` | `dbSetArchived` | Restore an archived note to the main list |
| `App.LockNote(id, passphrase)` | `sealContent` → `dbSetLockedContent` | Encrypt the note's content under a passphrase |
| `App.RevealNote(id, passphrase)` | `openContent` | Decrypt and return content; holds the key for this note only |
| `App.RelockNote()` | — | Drop the in-memory key (called when leaving the note) |
| `App.RemoveLock(id)` | `dbSetLockedContent` | Decrypt permanently; requires the note to be revealed |
| `App.IsRevealed(id)` | — | Whether the note is currently readable |
| `App.CheckForUpdate()` | GitHub Releases API | Latest release + whether it's newer than this build |
| `App.DownloadAndInstall()` | `swapBinary` | Download, verify SHA256, replace the running binary |
| `App.RestartApp()` | `exec` + `runtime.Quit` | Relaunch into the new version |
| `App.Version()` | — | Version stamped at build time |
| `App.SaveToFile(id)` | Wails dialog → `os.WriteFile` | Export content to a user-chosen path |
| `App.OpenFile()` | Wails dialog → `os.ReadFile` | Import file into a new note |
| `App.ImportFile(path)` | `importFile` | Same as `OpenFile`, given a path directly — used by drag-and-drop |
| `App.ExportContent(name, content)` | Wails dialog → `os.WriteFile` | Save content that has no DB row (a temporary note) to a chosen path |
| `App.GetSetting(key)` | `dbGetSetting` | Read one setting value |
| `App.SetSetting(key, value)` | `dbSetSetting` | Upsert one setting value |
| `App.RenderMarkdown(content, basePath)` | goldmark → bluemonday | Sanitized HTML for the preview pane; `basePath` resolves relative image/link references when previewing a folder-workspace file |
| `App.ResolveWorkspacePath(basePath, dest)` | `resolveWithinBase` | Confirms a clicked preview link resolves inside `basePath` before the frontend opens it |
| `App.PickFolder()` | Wails directory dialog | Choose a folder to open as a workspace |
| `App.ListDir(path)` | `os.ReadDir` | One level of a folder-workspace tree |
| `App.ReadDiskFile(path)` / `App.WriteDiskFile(path, content)` | `os.ReadFile` / `os.WriteFile` | Read/save a folder-workspace file directly, no DB row |
| `App.IsDir(path)` | `os.Stat` | Routes a dropped path to folder-open vs file-import |

---

## Dependencies

| Module | Version | License | Role |
|---|---|---|---|
| [`github.com/wailsapp/wails/v2`](https://github.com/wailsapp/wails) | `v2.12.0` | MIT | WebKitGTK window, JS bindings, native dialogs |
| [`github.com/mattn/go-sqlite3`](https://github.com/mattn/go-sqlite3) | `v1.14.45` | MIT | SQLite driver (CGO) |
| [`github.com/yuin/goldmark`](https://github.com/yuin/goldmark) | `v1.8.6` | MIT | Markdown → HTML for the preview pane |
| [`github.com/microcosm-cc/bluemonday`](https://github.com/microcosm-cc/bluemonday) | `v1.0.27` | BSD-3-Clause | Sanitizes rendered markdown HTML before it reaches the WebView |
| [`mermaid`](https://github.com/mermaid-js/mermaid) | `11.4.1` | MIT | Vendored at `frontend/dist/assets/mermaid.min.js` (see its `mermaid.LICENSE.txt`) — renders ` ```mermaid ` fences in the preview. No CDN: the app has no network access at runtime, so the bundle ships inside the embedded frontend. |

System libraries (not vendored): `libwebkit2gtk-4.1`, `libgtk-3`, `libsqlite3`.

---

## Troubleshooting

**`Package 'webkit2gtk-4.0' not found` during build**

You are on a distro that ships `webkit2gtk-4.1` (Arch, Fedora 37+, Ubuntu 22.10+). Build with the `webkit2_41` tag — `make build` does this for you, or pass `-tags webkit2_41` to `go build` directly (see [Building Manually](#building-manually)).

**Downloaded `gopad-linux-amd64` fails with `error while loading shared libraries: libwebkit2gtk-4.0.so.37`**

The published Linux release binary is built against `webkit2gtk-4.1` (see the note above `runs-on` in `.github/workflows/release.yml`), matching what `make build` produces locally. On a distro that only ships `webkit2gtk-4.0`, install `webkit2gtk-4.1` if your package manager offers it, or build from source per [Building Manually](#building-manually).

**App fails to start with `failed to open database`**

GoPad cannot create `~/.local/share/gopad/`. Check that your home directory is writable. As a fallback it will attempt to write `notes.db` in the current working directory.

On Windows, also confirm that the executable was built with `CGO_ENABLED=1` and MinGW-w64 GCC available. The `go-sqlite3` driver cannot open the database in a CGO-disabled build.

**Notes from a previous session are missing**

The database persists at `~/.local/share/gopad/notes.db`. If you moved or deleted that file the notes are gone. Always back up with `cp` before doing filesystem operations in that directory.

---

## License

MIT — see [`LICENSE`](LICENSE).
