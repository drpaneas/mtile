// Package hostdocs resolves the maintained learning guides for host tools.
package hostdocs

import (
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var routes = map[string]string{
	"docs/performance.md": "guides/performance.html",
	"docs/lint.md":        "guides/lint.html",
}

// URL links to repository Markdown, or the same guide on a rendered site.
func URL(guide, base string) string {
	path, fragment, _ := strings.Cut(guide, "#")
	remote := url.URL{Scheme: "https", Host: "github.com", Path: "/drpaneas/mtile/blob/main/" + path, Fragment: fragment}
	route, ok := routes[path]
	u, err := url.Parse(base)
	if base == "" || !ok || err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return remote.String()
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + route
	u.Fragment = fragment
	return u.String()
}

// ForProject prefers the built guide in a source checkout. Installed editors
// and external projects fall back to the public repository documentation.
func ForProject(project, guide string) string {
	path, fragment, _ := strings.Cut(guide, "#")
	route, ok := routes[path]
	if !ok {
		return URL(guide, "")
	}
	parent, err := filepath.Abs(project)
	if err != nil {
		return URL(guide, "")
	}
	for {
		module, err := os.ReadFile(filepath.Join(parent, "go.mod"))
		if err == nil && strings.Contains(strings.ReplaceAll(string(module), "\r\n", "\n"), "module github.com/drpaneas/mtile\n") {
			page := filepath.Join(parent, "website", "_site", filepath.FromSlash(route))
			if info, err := os.Stat(page); err == nil && info.Mode().IsRegular() {
				filePath := filepath.ToSlash(page)
				if !strings.HasPrefix(filePath, "/") {
					filePath = "/" + filePath
				}
				return (&url.URL{Scheme: "file", Path: filePath, Fragment: fragment}).String()
			}
			break
		}
		next := filepath.Dir(parent)
		if next == parent {
			break
		}
		parent = next
	}
	return URL(guide, "")
}

// Open asks the OS browser to show a documentation URL without blocking the UI.
func Open(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil {
		return errors.New("invalid documentation URL")
	}
	switch u.Scheme {
	case "http", "https":
		if u.Host == "" {
			return errors.New("documentation URL has no host")
		}
	case "file":
		if u.Host != "" || !strings.HasPrefix(u.Path, "/") {
			return errors.New("documentation file must be local and absolute")
		}
	default:
		return errors.New("unsupported documentation URL scheme")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", raw)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", raw)
	default:
		cmd = exec.Command("xdg-open", raw)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
