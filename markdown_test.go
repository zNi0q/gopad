package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestRenderMarkdownRewritesRelativeLinkWithinRoot(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mustWriteFile(t, filepath.Join(dir, "other.md"), "# Other")

	a := &App{}
	html, err := a.RenderMarkdown("[Other](other.md)", dir)
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}
	want := "gopadfile://" + filepath.Join(dir, "other.md")
	if !strings.Contains(html, want) {
		t.Fatalf("html = %q, want it to contain %q", html, want)
	}
}

func TestRenderMarkdownLeavesEscapingLinkAlone(t *testing.T) {
	dir := t.TempDir()
	a := &App{}
	html, err := a.RenderMarkdown("[Escape](../../etc/passwd)", dir)
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}
	if strings.Contains(html, "gopadfile://") {
		t.Fatalf("html = %q, a path escaping the folder root must not be rewritten to gopadfile://", html)
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
