package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// mdPolicy sanitizes the rendered HTML before it reaches the WebView. Goldmark
// already drops raw HTML and dangerous URLs by default, but unlike a browser
// tab this WebView has the Go bridge bound onto window.go.main.App, so the
// output still gets an independent allow-list pass. data-URI images are
// needed for embedded local images, since the WebView has no filesystem
// access of its own. Relative markdown links are deliberately left as plain,
// unrewritten hrefs — see ResolveWorkspacePath for why and how the preview's
// click handler follows them. The narrow class allow-list on <code> is how
// the frontend recognizes a ```mermaid fence to hand off to mermaid.js —
// goldmark already emits "language-mermaid" there by default, bluemonday
// would otherwise strip it (class isn't in UGCPolicy's baseline).
var mdPolicy = func() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowDataURIImages()
	p.AllowAttrs("class").Matching(regexp.MustCompile(`^language-[a-zA-Z0-9_-]+$`)).OnElements("code")
	return p
}()

// RenderMarkdown renders note content to sanitized HTML for the preview pane.
// basePath is the root of the open folder workspace (empty for a DB or
// temporary note with no folder context) — relative image/link references in
// the markdown resolve against that root, not the file's own subdirectory.
func (a *App) RenderMarkdown(content, basePath string) (string, error) {
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRendererOptions(html.WithHardWraps()),
		goldmark.WithParserOptions(parser.WithASTTransformers(
			util.Prioritized(&relativeRefResolver{base: basePath}, 100),
		)),
	)
	var buf bytes.Buffer
	if err := md.Convert([]byte(content), &buf); err != nil {
		return "", err
	}
	return mdPolicy.Sanitize(buf.String()), nil
}

// relativeRefResolver inlines relative image references as data URIs so they
// actually display (the WebView has no filesystem access of its own). Link
// destinations are deliberately left untouched here — see
// ResolveWorkspacePath.
type relativeRefResolver struct{ base string }

func (r *relativeRefResolver) Transform(doc *ast.Document, _ text.Reader, _ parser.Context) {
	if r.base == "" {
		return
	}
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if node, ok := n.(*ast.Image); ok {
			if abs, ok := resolveWithinBase(r.base, string(node.Destination)); ok {
				if dataURI, ok := embedImageDataURI(abs); ok {
					node.Destination = []byte(dataURI)
				}
			}
		}
		return ast.WalkContinue, nil
	})
}

// ResolveWorkspacePath resolves a markdown link's raw href against basePath
// and confirms it doesn't escape it. Unlike images, link destinations are
// never rewritten at render time (see relativeRefResolver) — rewriting them
// to a custom URL scheme would mean the sanitizer could no longer tell a
// legitimately-relative link from one an attacker hand-typed to look the
// same (e.g. a crafted link straight to a sensitive file), since by the time
// bluemonday sees the HTML both look identical. Re-resolving fresh here, at
// the moment the preview's click handler actually follows the link, closes
// that gap: only a destination that genuinely resolves inside basePath ever
// reaches openFolderFile / ReadDiskFile.
func (a *App) ResolveWorkspacePath(basePath, dest string) (string, error) {
	abs, ok := resolveWithinBase(basePath, dest)
	if !ok {
		return "", fmt.Errorf("%q does not resolve to a file inside the open folder", dest)
	}
	return abs, nil
}

// resolveWithinBase resolves a markdown-relative destination against base and
// confirms the result doesn't escape it (e.g. via "../../..").
func resolveWithinBase(base, dest string) (string, bool) {
	if dest == "" || strings.HasPrefix(dest, "#") {
		return "", false
	}
	if u, err := url.Parse(dest); err == nil && u.Scheme != "" {
		return "", false // has its own scheme (http, mailto, data, …) — not relative
	}
	abs := dest
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(base, dest)
	}
	abs = filepath.Clean(abs)
	baseClean := filepath.Clean(base)
	if abs != baseClean && !strings.HasPrefix(abs, baseClean+string(filepath.Separator)) {
		return "", false
	}
	// The lexical check above doesn't see through symlinks — a symlink inside
	// the folder root could otherwise point outside it and still pass. Resolve
	// both sides and re-check whenever the target actually exists; a target
	// that doesn't exist can't leak anything (the caller's Stat/ReadFile will
	// just fail on it), so that case is left to the lexical result above.
	if baseReal, err := filepath.EvalSymlinks(baseClean); err == nil {
		if absReal, err := filepath.EvalSymlinks(abs); err == nil {
			if absReal != baseReal && !strings.HasPrefix(absReal, baseReal+string(filepath.Separator)) {
				return "", false
			}
		}
	}
	return abs, true
}

func embedImageDataURI(absPath string) (string, bool) {
	info, err := os.Stat(absPath)
	if err != nil || info.IsDir() || info.Size() > maxDiskFileBytes {
		return "", false
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", false
	}
	mimeType := mime.TypeByExtension(filepath.Ext(absPath))
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	if i := strings.IndexByte(mimeType, ';'); i >= 0 {
		mimeType = mimeType[:i]
	}
	if !strings.HasPrefix(mimeType, "image/") {
		return "", false
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data), true
}
