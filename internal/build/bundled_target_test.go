package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestBundledTargetKeepsEveryExtraFileInsideSDK(t *testing.T) {
	sdk := t.TempDir()
	project := t.TempDir()
	work := t.TempDir()
	for _, name := range []string{filepath.Join(sdk, "targets"), filepath.Join(sdk, "src", "runtime"), filepath.Join(project, "crt0")} {
		if err := os.MkdirAll(name, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for path, data := range map[string]string{
		filepath.Join(sdk, "targets", "gameboy-advance.json"): "{}",
		filepath.Join(sdk, "src", "runtime", "asm_arm.S"):     "",
		filepath.Join(project, "crt0", "gba.s"):               "custom assembly",
		filepath.Join(project, "crt0", "gba.ld"):              "",
	} {
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.CopyFS(filepath.Join(sdk, "targets", "mtile"), os.DirFS("../../scripts/release/tinygo-extra")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(work, "preview.json")
	if err := GenerateTarget(sdk, project, path); err != nil {
		t.Fatal(err)
	}
	runtimeDir, err := filepath.Abs("../../runtime")
	if err != nil {
		t.Fatal(err)
	}
	if err := addDirectAudioTarget(sdk, runtimeDir, path, 0x18); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var target struct {
		Extra []string `json:"extra-files"`
		Flags []string `json:"cflags"`
	}
	if err := json.Unmarshal(raw, &target); err != nil {
		t.Fatal(err)
	}
	startupDir := filepath.Join(work, ".mtile-crt0", "preview.json")
	if !slices.Contains(target.Flags, "-I"+startupDir) || !slices.Contains(target.Flags, "-I"+runtimeDir) {
		t.Fatalf("missing include directories: %v", target.Flags)
	}
	if len(target.Extra) != 9 {
		t.Fatalf("unexpected bundled inputs: %v", target.Extra)
	}
	for _, source := range target.Extra {
		if !filepath.IsLocal(source) {
			t.Fatalf("TinyGo cannot open an extra-file on another drive: %s", source)
		}
		wrapper, err := os.ReadFile(filepath.Join(sdk, source))
		if err != nil {
			t.Fatal(err)
		}
		if source == "src/runtime/asm_arm.S" {
			continue
		}
		include := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(string(wrapper)), "#include <"), ">")
		if include == "mtile-gba-startup.s" {
			data, err := os.ReadFile(filepath.Join(startupDir, include))
			if err != nil || string(data) != "custom assembly" {
				t.Fatalf("selected startup = %q, %v", data, err)
			}
		} else if _, err := os.Stat(filepath.Join(runtimeDir, include)); err != nil {
			t.Fatalf("wrapper %s: %v", source, err)
		}
	}
}
