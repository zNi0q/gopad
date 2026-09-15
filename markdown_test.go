package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderMarkdownKeepsMermaidFenceClass(t *testing.T) {
	a := &App{}
	html, err := a.RenderMarkdown("```mermaid\ngraph TD; A-->B;\n```", "")
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}
	if !strings.Contains(html, `class="language-mermaid"`) {
		t.Fatalf("html = %q, want the fenced code block's language-mermaid class preserved through sanitization", html)
	}
	if !strings.Contains(html, "graph TD") {
		t.Fatalf("html = %q, want the diagram source present so the frontend can hand it to mermaid.js", html)
	}
}

func TestRenderMarkdownDropsUnrecognizedClass(t *testing.T) {
	a := &App{}
	html, err := a.RenderMarkdown("<code class=\"evil\">x</code>", "")
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}
	if strings.Contains(html, `class="evil"`) {
		t.Fatalf("html = %q, want a class not matching language-* stripped", html)
	}
}

func TestRenderMarkdownEmbedsRelativeImage(t *testing.T) {
	dir := t.TempDir()
	// A 1x1 transparent PNG, just enough bytes to be sniffable as image/png.
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0}
	mustWriteFile(t, filepath.Join(dir, "pic.png"), string(png))

	a := &App{}
	html, err := a.RenderMarkdown("![alt](pic.png)", dir)
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}
	if !strings.Contains(html, "data:image/png;base64,") {
		t.Fatalf("html = %q, want an embedded data: URI for the relative image", html)
	}
}

// RenderMarkdown deliberately never rewrites link destinations (only image
// destinations) — see ResolveWorkspacePath's doc comment for why. Confirm
// that holds even with a folder root in play.
func TestRenderMarkdownNeverRewritesLinks(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "other.md"), "# Other")

	a := &App{}
	html, err := a.RenderMarkdown("[Other](other.md)", dir)
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}
	if !strings.Contains(html, `href="other.md"`) {
		t.Fatalf("html = %q, want the link left exactly as authored", html)
	}
}

func TestResolveWorkspacePathWithinRoot(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "other.md"), "# Other")

	a := &App{}
	got, err := a.ResolveWorkspacePath(dir, "other.md")
	if err != nil {
		t.Fatalf("ResolveWorkspacePath: %v", err)
	}
	if want := filepath.Join(dir, "other.md"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResolveWorkspacePathRejectsEscape(t *testing.T) {
	dir := t.TempDir()
	a := &App{}
	if _, err := a.ResolveWorkspacePath(dir, "../../etc/passwd"); err == nil {
		t.Fatal("ResolveWorkspacePath on a path escaping the root: want an error, got nil")
	}
}

func TestResolveWorkspacePathRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.md")
	mustWriteFile(t, secret, "# Secret")

	link := filepath.Join(root, "escape.md")
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	a := &App{}
	if _, err := a.ResolveWorkspacePath(root, "escape.md"); err == nil {
		t.Fatal("ResolveWorkspacePath on a symlink escaping the root: want an error, got nil")
	}
}

// A hand-typed link that merely looks relative but is really an absolute
// path outside any folder root must not resolve either — this is the
// preview's actual security boundary for links (see ResolveWorkspacePath),
// exercised the same way the click handler uses it.
func TestResolveWorkspacePathRejectsForgedAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	a := &App{}
	if _, err := a.ResolveWorkspacePath(dir, "/etc/passwd"); err == nil {
		t.Fatal("ResolveWorkspacePath on an absolute path outside the root: want an error, got nil")
	}
}

func TestRenderMarkdownLeavesExternalURLsAlone(t *testing.T) {
	dir := t.TempDir()
	a := &App{}
	html, err := a.RenderMarkdown("[Ext](https://example.com/x)", dir)
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}
	if !strings.Contains(html, `href="https://example.com/x"`) {
		t.Fatalf("html = %q, want the external URL preserved as-is", html)
	}
}

func TestRenderMarkdownNoBasePathLeavesReferencesAlone(t *testing.T) {
	a := &App{}
	html, err := a.RenderMarkdown("[Rel](other.md)", "")
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}
	if !strings.Contains(html, `href="other.md"`) {
		t.Fatalf("html = %q, want the relative link left untouched with no folder context", html)
	}
}
