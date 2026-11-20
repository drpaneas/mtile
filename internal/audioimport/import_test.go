package audioimport

import (
	"encoding/binary"
	"fmt"
	"github.com/drpaneas/mtile/internal/assetsync"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The subprocess fixture checks real argv/cwd execution and atomic publication.
// Native integration separately runs the genuine mmutil and Maxmod backend.
func TestMain(m *testing.M) {
	if os.Getenv("MTILE_IMPORT_HELPER") == "1" {
		fakeConverter()
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func fakeConverter() {
	if path := os.Getenv("MTILE_IMPORT_COUNT"); path != "" {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			panic(err)
		}
		if _, err := f.WriteString("convert\n"); err != nil {
			panic(err)
		}
		if err := f.Close(); err != nil {
			panic(err)
		}
	}
	var songs, samples []string
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		raw, _ := os.ReadFile(arg)
		if string(raw) == "FAIL" {
			os.Exit(2)
		}
		if filepath.Ext(arg) == ".wav" {
			samples = append(samples, arg)
		} else {
			songs = append(songs, arg)
		}
	}
	n := len(songs) + len(samples)
	bank := make([]byte, 12+4*n+8*n)
	binary.LittleEndian.PutUint16(bank, uint16(len(samples)))
	binary.LittleEndian.PutUint16(bank[2:], uint16(len(songs)))
	copy(bank[4:], "*maxmod*")
	header := ""
	for i, file := range append(samples, songs...) {
		off := 12 + 4*n + 8*i
		binary.LittleEndian.PutUint32(bank[12+4*i:], uint32(off))
		binary.LittleEndian.PutUint32(bank[off:], 8)
		kind := byte(1)
		prefix, id := "SFX", i
		if i >= len(samples) {
			kind = 0
			prefix, id = "MOD", i-len(samples)
		}
		bank[off+4], bank[off+5] = kind, 0x19
		name := strings.ToUpper(strings.TrimSuffix(file, filepath.Ext(file)))
		header += fmt.Sprintf("#define %s_%s %d\n", prefix, name, id)
	}
	if err := os.WriteFile("bank.bin", bank, 0o644); err != nil {
		panic(err)
	}
	if err := os.WriteFile("bank.h", []byte(header), 0o644); err != nil {
		panic(err)
	}
}

func TestImportPreservesIDsAndPlayableBankOnFailure(t *testing.T) {
	t.Setenv("MTILE_IMPORT_HELPER", "1")
	tool, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	project, input := t.TempDir(), t.TempDir()
	write := func(name, text string) string {
		path := filepath.Join(input, name)
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	x := write("zeta.mod", "old")
	bell := write("bell.wav", "old")
	first, err := Import(project, []string{x, bell}, tool)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Import(project, []string{write("alpha.xm", "new")}, tool)
	if err != nil {
		t.Fatal(err)
	}
	if second.Manifest.Music[0].Name != "zeta" || second.Manifest.Music[1].Name != "alpha" || second.Manifest.Samples[0].Name != "bell" {
		t.Fatal("public IDs reordered")
	}
	if second.Manifest.SHA256 == first.Manifest.SHA256 {
		t.Fatal("new item missing from bank")
	}
	manifest := filepath.Join(project, "assets/audio/direct/manifest.json")
	before, _ := os.ReadFile(manifest)
	if _, err = Import(project, []string{write("zeta.mod", "FAIL")}, tool); err == nil {
		t.Fatal("converter failure ignored")
	}
	after, _ := os.ReadFile(manifest)
	if string(before) != string(after) {
		t.Fatal("failure published new manifest")
	}
	if _, err = assetsync.LoadDirectAudio(project); err != nil {
		t.Fatal("old bank no longer playable", err)
	}
	if _, err = Import(project, []string{write("bad-name.wav", "new")}, tool); err == nil {
		t.Fatal("invalid name accepted")
	}
}
