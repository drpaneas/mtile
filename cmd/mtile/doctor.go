package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"go/version"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/drpaneas/mtile/internal/build"
)

const supportedTinyGo = "0.42.0"

var goVersionPattern = regexp.MustCompile(`^go version (go[0-9]+\.[0-9]+\.[0-9]+)(?:\s|$)`)

func supportedGoVersion(output string) bool {
	match := goVersionPattern.FindStringSubmatch(output)
	if len(match) != 2 {
		return false
	}
	return version.IsValid(match[1]) && version.Compare(match[1], "go1.27.1") >= 0
}

func doctorCommandOutput(env []string, command string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return "", fmt.Errorf("%s timed out after 10 seconds", filepath.Base(command))
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w: %s", filepath.Base(command), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func runDoctor(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	tinygo := flags.String("tinygo", "", "TinyGo executable (same selection as build)")
	help, err := parseFlags(flags, args, out)
	if err != nil || help {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("doctor does not take a project directory; run mtile doctor")
	}
	_, _ = fmt.Fprintln(out, "mTile setup check — ROM build tools")
	failed := false
	fail := func(name, detail, fix string) {
		failed = true
		_, _ = fmt.Fprintf(out, "FAIL %s: %s\n     Fix: %s\n", name, detail, fix)
	}
	service, setupErr := build.NewService(*tinygo, "")
	goBin, env := build.ToolchainEnvironment(service.TinyGoBin)
	goVersion, err := doctorCommandOutput(env, goBin, "version")
	if err != nil {
		fail("Go", err.Error(), "restore the complete desktop bundle, or install Go 1.27.1 or newer for a source checkout")
	} else if !supportedGoVersion(goVersion) {
		fail("Go", goVersion, "install a stable Go release, version 1.27.1 or newer")
	} else {
		_, _ = fmt.Fprintln(out, "OK   Go:", goVersion)
	}
	if setupErr != nil {
		fix := "keep the desktop bundle together, or install TinyGo 0.42.0; use -tinygo /path/to/tinygo or MTILE_TINYGO to select it"
		if errors.Is(setupErr, os.ErrPermission) {
			fix = "check access to the reported path; your account must be able to read the toolchain and write its mTile runtime cache"
		}
		fail("Build setup", setupErr.Error(), fix)
	} else {
		version, versionErr := doctorCommandOutput(env, service.TinyGoBin, "version")
		if versionErr != nil {
			fail("TinyGo", versionErr.Error(), "check the executable path and permissions; install the TinyGo 0.42.0 archive for your OS and CPU")
		} else if !strings.HasPrefix(version, "tinygo version "+supportedTinyGo+" ") {
			fail("TinyGo", version, "select TinyGo 0.42.0, the release tested with mTile")
		} else {
			_, _ = fmt.Fprintf(out, "OK   TinyGo: %s\n     %s\n", version, service.TinyGoBin)
			root, rootErr := doctorCommandOutput(env, service.TinyGoBin, "env", "TINYGOROOT")
			if rootErr != nil || root == "" {
				fail("GBA target", fmt.Sprintf("cannot locate TinyGo resources: %v", rootErr), "reinstall the complete toolchain; check TINYGOROOT if you set it")
			} else if info, statErr := os.Stat(filepath.Join(root, "targets", "gameboy-advance.json")); statErr != nil || info.IsDir() {
				fail("GBA target", "gameboy-advance.json is missing from "+root, "keep TinyGo's targets and libraries with its executable; check TINYGOROOT")
			} else {
				_, _ = fmt.Fprintln(out, "OK   GBA target and mTile runtime found")
			}
		}
	}
	_, _ = fmt.Fprintln(out, "\nThis checks tool availability, not a ROM build or desktop graphics/audio.")
	_, _ = fmt.Fprintln(out, "Source editor preview also needs CMake, a C compiler, and initialized submodules.")
	if failed {
		return errors.New("setup needs attention; apply the fixes above and run doctor again")
	}
	_, _ = fmt.Fprintln(out, "\nNext: mtile new -template coin-quest mygame")
	_, _ = fmt.Fprintln(out, "Then: mtile build mygame")
	return nil
}
