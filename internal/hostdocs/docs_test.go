package hostdocs

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestForProjectUsesSourceCheckoutGuideAndPreservesAnchor(t *testing.T) {
	root := filepath.Join(t.TempDir(), "checkout with spaces")
	project := filepath.Join(root, "examples", "game")
	page := filepath.Join(root, "website", "_site", "guides", "lint.html")
	if err := os.MkdirAll(filepath.Dir(page), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module github.com/drpaneas/mtile\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(page, []byte("<html>guide</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw := ForProject(project, "docs/lint.md#mtl008-discarded-construction")
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "file" || u.Fragment != "mtl008-discarded-construction" || u.Path != filepath.ToSlash(page) {
		t.Fatalf("guide URL: %q (%v)", raw, err)
	}
	if err := os.Remove(page); err != nil {
		t.Fatal(err)
	}
	if got := ForProject(project, "docs/lint.md#mtl008-discarded-construction"); got != "https://github.com/drpaneas/mtile/blob/main/docs/lint.md#mtl008-discarded-construction" {
		t.Fatalf("missing local guide fallback: %q", got)
	}
}

func TestDocumentationLinksAndInvalidLaunches(t *testing.T) {
	if got := URL("docs/performance.md#reuse", "https://docs.example/mtile/"); got != "https://docs.example/mtile/guides/performance.html#reuse" {
		t.Fatalf("rendered site: %q", got)
	}
	if got := ForProject(t.TempDir(), "docs/performance.md#reuse"); !strings.HasPrefix(got, "https://github.com/drpaneas/mtile/") {
		t.Fatalf("external project fallback: %q", got)
	}
	for _, raw := range []string{"javascript:alert(1)", "file://remote/tmp/a.html", "file:relative.md", "https://user:secret@example.test/", "https:///missing-host"} {
		if err := Open(raw); err == nil {
			t.Fatalf("invalid launch accepted: %q", raw)
		}
	}
}
