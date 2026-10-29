package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
)

func TestFontBDemoCapture(t *testing.T) {
	out := os.Getenv("MTILE_FONT_DEMO_CAPTURE")
	if out == "" {
		t.Skip("set MTILE_FONT_DEMO_CAPTURE to capture the real demo ROM")
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	service, err := build.NewService("", "")
	if err != nil {
		t.Fatal(err)
	}
	service.Debug = true
	result, err := service.Build(filepath.Join(root, "examples/font-demo"), filepath.Join(out, "demo.gba"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := captureROM(filepath.Join(root, "build/player/mtile-player"), result.ROMPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = writePNG(filepath.Join(out, "demo.png"), c.Pixels[len(c.Pixels)-1]); err != nil {
		t.Fatal(err)
	}
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(out, "demo.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}
}
