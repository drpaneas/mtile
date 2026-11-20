package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
)

func TestDevelopROMExportMatchesCLI(t *testing.T) {
	if os.Getenv("MTILE_VERIFY_ROM_EXPORT") == "" {
		t.Skip("set MTILE_VERIFY_ROM_EXPORT to exercise TinyGo and mGBA")
	}
	project := filepath.Join(t.TempDir(), "coin-quest")
	if err := build.CreateProject(project, build.NewProjectOptions{Template: "coin-quest"}); err != nil {
		t.Fatal(err)
	}
	release := filepath.Join(project, "game.gba")
	cli := filepath.Join(project, "cli.gba")
	if output, err := exec.Command("go", "run", "../mtile", "build", "-o", cli, project).CombinedOutput(); err != nil {
		t.Fatalf("CLI build: %v\n%s", err, output)
	}
	if _, err := buildDevelopReleaseROM(project, release); err != nil {
		t.Fatal(err)
	}
	read := func(path string) []byte {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	if !bytes.Equal(read(release), read(cli)) {
		t.Fatal("export differs from the CLI release build")
	}
	preview, err := developPreviewROMPath(project)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(preview)) })
	service, err := build.NewService("", "")
	if err != nil {
		t.Fatal(err)
	}
	service.Debug, service.UseProjectRenderer = true, true
	if _, err := service.Build(project, preview); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(read(release), read(cli)) || bytes.Equal(read(release), read(preview)) {
		t.Fatal("preview build replaced the export or failed to produce a distinct debug ROM")
	}
	helper, err := emulator.ResolveHelper()
	if err != nil {
		t.Fatal(err)
	}
	play := func(path string) emulator.Frame {
		t.Helper()
		session, err := emulator.Start(helper, path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := session.Close(); err != nil {
				t.Error(err)
			}
		}()
		var frame emulator.Frame
		for range 12 {
			frame, err = session.Step(0, false)
			if err != nil {
				t.Fatal(err)
			}
		}
		return frame
	}
	debugFrame, releaseFrame := play(preview), play(release)
	if !debugFrame.Telemetry.DebugValid || releaseFrame.Telemetry.DebugValid || !bytes.Equal(debugFrame.RGBA, releaseFrame.RGBA) {
		t.Fatal("expected the same pixels with debug telemetry only in the preview")
	}
}
