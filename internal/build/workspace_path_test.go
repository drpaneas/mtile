package build

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGeneratedWorkspaceAcceptsRuntimePathWithSpaces(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), "runtime with spaces")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeDir, "go.mod"), []byte("module github.com/drpaneas/mtile/runtime\n\ngo 1.27.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	if err := (Service{RuntimeDir: runtimeDir}).writeWorkspace(work, "package main\nfunc main() {}\n"); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "list", "-m", "-json", "github.com/drpaneas/mtile/runtime")
	cmd.Dir = work
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generated replacement path was not accepted: %v\n%s", err, output)
	}
	var module struct{ Dir string }
	if err := json.Unmarshal(output, &module); err != nil {
		t.Fatal(err)
	}
	if module.Dir != runtimeDir {
		t.Fatalf("replacement resolved to %q, want %q", module.Dir, runtimeDir)
	}
}
