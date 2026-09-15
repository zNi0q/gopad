package main

import (
	"bytes"
	"encoding/base64"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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
// output still gets an independent allow-list pass. gopadfile is our own
// scheme for relative links to another file in an open folder (see
// relativeRefResolver below); data-URI images are needed for embedded local
// images, since the WebView has no filesystem access of its own.
var mdPolicy = func() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowDataURIImages()
	p.AllowURLSchemes("gopadfile")
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

// relativeRefResolver rewrites relative image/link destinations so they still
// work inside the preview: images are inlined as data URIs, and links to
// another file under base get a gopadfile:// destination that the preview's
// click handler resolves back to a real path and opens in the folder
// workspace. Anything outside base, or with a scheme of its own (http,
// mailto, data, …), is left untouched.
type relativeRefResolver struct{ base string }

func (r *relativeRefResolver) Transform(doc *ast.Document, _ text.Reader, _ parser.Context) {
	if r.base == "" {
		return
	}
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *ast.Image:
			if abs, ok := resolveWithinBase(r.base, string(node.Destination)); ok {
				if dataURI, ok := embedImageDataURI(abs); ok {
					node.Destination = []byte(dataURI)
				}
			}
		case *ast.Link:
			if abs, ok := resolveWithinBase(r.base, string(node.Destination)); ok {
				node.Destination = []byte((&url.URL{Scheme: "gopadfile", Path: abs}).String())
			}
		}
		return ast.WalkContinue, nil
	})
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
