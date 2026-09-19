//go:build mtile_editor_vm

// editorvmcheck drives the real editor through Ebitengine's opt-in VM protocol.
// It edits only a temporary copy of the supplied project and does not exercise
// audio or MIDI. Run with -tags mtile_editor_vm after building a guest
// editor with -tags ebitenginevmguest at the same Ebitengine version.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/png"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/exp/vmhost"
)

type smoke struct {
	guest      *vmhost.GuestSession
	screen     *ebiten.Image
	out, entry string
	original   []byte
	hashes     map[string]string
	closed     bool
	closeErr   error
}

func (s *smoke) ticks(n int) error {
	s.guest.AdvanceTicks(n)
	if !s.guest.WaitTicks() {
		return fmt.Errorf("guest ticks failed: %v", s.guest.Err())
	}
	return nil
}

func (s *smoke) chord(keys ...ebiten.Key) error {
	for _, key := range keys {
		s.guest.PressKey(key)
	}
	if err := s.ticks(1); err != nil {
		return err
	}
	for _, key := range keys {
		s.guest.ReleaseKey(key)
	}
	return s.ticks(2)
}

func (s *smoke) click(x, y float64) error {
	s.guest.MoveCursor(x, y)
	s.guest.PressMouseButton(ebiten.MouseButtonLeft)
	if err := s.ticks(1); err != nil {
		return err
	}
	s.guest.ReleaseMouseButton(ebiten.MouseButtonLeft)
	return s.ticks(2)
}

func (s *smoke) capture(name string) error {
	s.guest.AdvanceFrame()
	if !s.guest.WaitFrame() || !s.guest.CompositeFrame() {
		return fmt.Errorf("capture %s failed: %v", name, s.guest.Err())
	}
	img := image.NewRGBA(s.screen.Bounds())
	s.screen.ReadPixels(img.Pix)
	sum := sha256.Sum256(img.Pix)
	s.hashes[name] = hex.EncodeToString(sum[:])
	if bytes.Equal(img.Pix, make([]byte, len(img.Pix))) {
		return fmt.Errorf("capture %s is empty", name)
	}
	f, err := os.Create(filepath.Join(s.out, name+".png"))
	if err != nil {
		return err
	}
	err = png.Encode(f, img)
	return errors.Join(err, f.Close())
}

func (s *smoke) Update() error {
	defer func() { s.closeErr = s.guest.Close(); s.closed = true }()
	scale := ebiten.Monitor().DeviceScaleFactor()
	s.screen = ebiten.NewImage(int(1200*scale), int(1000*scale))
	if err := s.guest.SetOutsideScreen(s.screen); err != nil {
		return err
	}
	if err := s.ticks(30); err != nil {
		return err
	}
	if err := s.capture("sprites"); err != nil {
		return err
	}
	if err := s.chord(ebiten.Key2); err != nil {
		return err
	}
	if err := s.capture("map"); err != nil {
		return err
	}
	// The first tab's origin is fixed across font sizes. Returning there avoids
	// Map mode's number keys, which select background layers rather than tabs.
	s.guest.MoveCursor(160, 18)
	s.guest.PressMouseButton(ebiten.MouseButtonLeft)
	if err := s.ticks(1); err != nil {
		return err
	}
	s.guest.ReleaseMouseButton(ebiten.MouseButtonLeft)
	if err := s.ticks(2); err != nil {
		return err
	}
	if err := s.chord(ebiten.Key4); err != nil {
		return err
	}
	// Develop starts a preview build on entry. Stop it through the actual UI;
	// this smoke exercises editing independently of an installed TinyGo toolchain.
	if err := s.click(1160, 70); err != nil {
		return err
	}
	if err := s.capture("code"); err != nil {
		return err
	}
	if s.hashes["sprites"] == s.hashes["map"] || s.hashes["map"] == s.hashes["code"] {
		return errors.New("workspace navigation did not change rendered output")
	}
	if err := s.chord(ebiten.KeyControlLeft, ebiten.KeyA); err != nil {
		return err
	}
	const replacement = "-- editor VM smoke"
	for _, r := range replacement {
		s.guest.TypeRune(r)
	}
	if err := s.ticks(1); err != nil {
		return err
	}
	if err := s.chord(ebiten.KeyControlLeft, ebiten.KeyS); err != nil {
		return err
	}
	got, err := os.ReadFile(s.entry)
	if err != nil {
		return err
	}
	if string(got) != replacement {
		return fmt.Errorf("injected edit/save mismatch: got %q", got)
	}
	if err := s.capture("edited"); err != nil {
		return err
	}
	if err := s.chord(ebiten.KeyControlLeft, ebiten.KeyZ); err != nil {
		return err
	}
	if err := s.chord(ebiten.KeyControlLeft, ebiten.KeyS); err != nil {
		return err
	}
	got, err = os.ReadFile(s.entry)
	if err != nil {
		return err
	}
	if !bytes.Equal(got, s.original) {
		return errors.New("undo/save did not restore original source bytes")
	}
	if err := s.capture("restored"); err != nil {
		return err
	}
	report := struct {
		Ticks          int               `json:"ticks"`
		SourceRestored bool              `json:"source_restored"`
		FrameHashes    map[string]string `json:"frame_sha256"`
	}{s.guest.ProcessedTicks(), true, s.hashes}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(s.out, "result.json"), append(data, '\n'), 0o644); err != nil {
		return err
	}
	return ebiten.Termination
}

func (*smoke) Draw(*ebiten.Image)         {}
func (*smoke) Layout(w, h int) (int, int) { return w, h }

func run() error {
	guestPath := flag.String("guest", "", "editor executable built with ebitenginevmguest")
	project := flag.String("project", "examples/hello", "project to copy for the smoke test (game.lua entry)")
	out := flag.String("out", "", "directory for project copy, PNGs, and result.json")
	flag.Parse()
	if *guestPath == "" || *out == "" {
		return errors.New("-guest and -out are required")
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	dir, err := os.MkdirTemp(*out, "project-")
	if err != nil {
		return err
	}
	if err := os.CopyFS(dir, os.DirFS(*project)); err != nil {
		return err
	}
	unavailableToolchain := filepath.Join(dir, "toolchain-disabled")
	if _, err := os.Lstat(unavailableToolchain); !os.IsNotExist(err) {
		return errors.New("smoke fixture must not contain toolchain-disabled")
	}
	entry := filepath.Join(dir, "game.lua")
	original, err := os.ReadFile(entry)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	if err := listener.(*net.TCPListener).SetDeadline(time.Now().Add(20 * time.Second)); err != nil {
		return err
	}
	endpoint, err := vmhost.EndpointURLFromAddr(listener.Addr())
	if err != nil {
		return err
	}
	cmd := exec.Command(*guestPath, "-project", dir)
	// An explicit unavailable toolchain keeps Develop's automatic build from
	// launching compilers or a cartridge while this checks only the host UI.
	cmd.Env = append(os.Environ(), "EBITENGINE_VM_ENDPOINT="+endpoint, "MTILE_TINYGO="+unavailableToolchain)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	reaped := false
	defer func() {
		if !reaped {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	conn, err := listener.Accept()
	if err != nil {
		return fmt.Errorf("accept editor VM: %w", err)
	}
	_ = listener.Close()
	guest, err := vmhost.NewGuestSession(conn, &vmhost.NewGuestSessionOptions{IdleTimeout: 20 * time.Second})
	if err != nil {
		_ = conn.Close()
		return err
	}
	s := &smoke{guest: guest, out: *out, entry: entry, original: original, hashes: make(map[string]string)}
	ebiten.SetWindowVisible(false)
	runErr := ebiten.RunGame(s)
	if !s.closed {
		return fmt.Errorf("host did not run: %w", runErr)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
	case <-time.After(20 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		err = errors.New("editor did not exit after VM closure")
	}
	reaped = true
	if err := errors.Join(runErr, s.closeErr, err); err != nil {
		return err
	}
	if err := guest.Err(); err != nil && !errors.Is(err, ebiten.Termination) {
		return err
	}
	fmt.Println("editor VM smoke passed: navigation, rendering, edit/save, undo/save, and clean exit")
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
