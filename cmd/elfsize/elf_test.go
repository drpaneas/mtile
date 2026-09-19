package main

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// A small hand-built ARM ELF keeps the accounting oracle independent of the
// analyzer and needs neither a cross compiler nor a checked-in binary blob.
// ROM has a gap, an overlapping LOAD, and padding outside any section. RAM
// has overlapping NOBITS sections, a stack, and the separate debug mailbox.
func fixtureELF(t testing.TB, symbols bool) []byte {
	t.Helper()
	data := make([]byte, 0x800)
	put := func(off int, v any) {
		var b bytes.Buffer
		if err := binary.Write(&b, binary.LittleEndian, v); err != nil {
			t.Fatal(err)
		}
		copy(data[off:], b.Bytes())
	}
	var ident [16]byte
	copy(ident[:], "\x7fELF")
	ident[4], ident[5], ident[6] = 1, 1, 1
	put(0, elf.Header32{Ident: ident, Type: uint16(elf.ET_EXEC), Machine: uint16(elf.EM_ARM), Version: 1, Entry: 0x08000000, Phoff: 52, Shoff: 0x600, Ehsize: 52, Phentsize: 32, Phnum: 3, Shentsize: 40, Shnum: 12, Shstrndx: 11})
	put(52, elf.Prog32{Type: uint32(elf.PT_LOAD), Off: 0x100, Vaddr: 0x08000000, Paddr: 0x08000000, Filesz: 32, Memsz: 32, Flags: 5, Align: 4})
	load := elf.Prog32{Type: uint32(elf.PT_LOAD), Off: 0x200, Vaddr: 0x03002400, Paddr: 0x08000040, Filesz: 16, Memsz: 16, Flags: 5, Align: 4}
	put(84, load)
	put(116, load)
	sectionNames := "\x00.text\x00.data\x00.stack\x00.bss\x00.bss_overlap\x00.ewram\x00.debug_mailbox\x00.debug_info\x00.strtab\x00.symtab\x00.shstrtab\x00"
	copy(data[0x400:], sectionNames)
	name := func(s string) uint32 { return uint32(strings.Index(sectionNames, s+"\x00")) }
	sections := []elf.Section32{
		{},
		{Name: name(".text"), Type: uint32(elf.SHT_PROGBITS), Flags: uint32(elf.SHF_ALLOC | elf.SHF_EXECINSTR), Addr: 0x08000000, Off: 0x100, Size: 16, Addralign: 4},
		{Name: name(".data"), Type: uint32(elf.SHT_PROGBITS), Flags: uint32(elf.SHF_ALLOC | elf.SHF_EXECINSTR), Addr: 0x03002400, Off: 0x200, Size: 16, Addralign: 4},
		{Name: name(".stack"), Type: uint32(elf.SHT_NOBITS), Flags: uint32(elf.SHF_ALLOC), Addr: 0x03000000, Size: 0x2400, Addralign: 4},
		{Name: name(".bss"), Type: uint32(elf.SHT_NOBITS), Flags: uint32(elf.SHF_ALLOC | elf.SHF_WRITE), Addr: 0x03002410, Size: 8, Addralign: 4},
		{Name: name(".bss_overlap"), Type: uint32(elf.SHT_NOBITS), Flags: uint32(elf.SHF_ALLOC | elf.SHF_WRITE), Addr: 0x03002414, Size: 8, Addralign: 4},
		{Name: name(".ewram"), Type: uint32(elf.SHT_NOBITS), Flags: uint32(elf.SHF_ALLOC | elf.SHF_WRITE), Addr: 0x02000000, Size: 32, Addralign: 4},
		{Name: name(".debug_mailbox"), Type: uint32(elf.SHT_NOBITS), Flags: uint32(elf.SHF_ALLOC | elf.SHF_WRITE), Addr: 0x0203fe00, Size: 512, Addralign: 4},
		{Name: name(".debug_info"), Type: uint32(elf.SHT_PROGBITS), Off: 0x240, Size: 16, Addralign: 1},
		{Name: name(".strtab"), Type: uint32(elf.SHT_STRTAB), Off: 0x280, Addralign: 1},
		{Name: name(".symtab"), Type: uint32(elf.SHT_SYMTAB), Off: 0x300, Link: 9, Info: 1, Addralign: 4, Entsize: 16},
		{Name: name(".shstrtab"), Type: uint32(elf.SHT_STRTAB), Off: 0x400, Size: uint32(len(sectionNames)), Addralign: 1},
	}
	names := "\x00foo\x00foo_alias\x00thumb\x00overlap\x00scratch\x00undefined\x00absolute\x00$t\x00debug\x00"
	copy(data[0x280:], names)
	fn := func(s string, addr, size uint32, section uint16, kind elf.SymType) elf.Sym32 {
		return elf.Sym32{Name: uint32(strings.Index(names, s+"\x00")), Value: addr, Size: size, Info: byte(elf.STB_GLOBAL)<<4 | byte(kind), Shndx: section}
	}
	syms := []elf.Sym32{{}, fn("foo", 0x08000000, 8, 1, elf.STT_FUNC), fn("foo_alias", 0x08000000, 8, 1, elf.STT_FUNC), fn("thumb", 0x03002401, 8, 2, elf.STT_FUNC), fn("overlap", 0x03002404, 8, 2, elf.STT_FUNC), fn("scratch", 0x02000000, 16, 6, elf.STT_OBJECT), fn("undefined", 0, 4, 0, elf.STT_FUNC), fn("absolute", 0, 4, uint16(elf.SHN_ABS), elf.STT_NOTYPE), fn("$t", 0x03002401, 0, 2, elf.STT_NOTYPE), fn("debug", 0, 8, 8, elf.STT_OBJECT)}
	for i, s := range syms {
		put(0x300+i*16, s)
	}
	sections[9].Size = uint32(len(names))
	sections[10].Size = uint32(len(syms) * 16)
	if !symbols {
		sections[10].Type = uint32(elf.SHT_NULL)
	}
	for i, s := range sections {
		put(0x600+i*40, s)
	}
	return data
}

func TestELFAccountingUnionsAliasesThumbAndPadding(t *testing.T) {
	got, err := analyzeELF(bytes.NewReader(fixtureELF(t, true)), "synthetic.elf")
	if err != nil {
		t.Fatal(err)
	}
	if want := (cost{ROM: 48, IWRAM: 9244, EWRAM: 32, Mailbox: 512}); got.Totals != want {
		t.Fatalf("totals=%+v want %+v", got.Totals, want)
	}
	if got.ROMImageBytes != 80 || got.ROMSegmentPadding != 16 {
		t.Fatalf("ROM span/padding=%d/%d want80/16", got.ROMImageBytes, got.ROMSegmentPadding)
	}
	if len(got.Sections) != 7 || len(got.Symbols) != 4 {
		t.Fatalf("allocated sections/symbol groups=%d/%d want7/4", len(got.Sections), len(got.Symbols))
	}
	for _, s := range got.Sections {
		if s.Name == ".data" && (s.LMA == nil || *s.LMA != 0x08000040 || s.Cost.ROM != 16 || s.Cost.IWRAM != 16 || s.Unattributed != 4) {
			t.Fatalf("IWRAM dual attribution/overlap=%+v", s)
		}
		if s.Type == "SHT_NOBITS" && (s.LMA != nil || s.Cost.ROM != 0) {
			t.Fatalf("NOBITS has ROM bytes: %+v", s)
		}
	}
	for _, s := range got.Symbols {
		if s.Names[0] == "foo" && !reflect.DeepEqual(s.Names, []string{"foo", "foo_alias"}) {
			t.Fatalf("aliases=%v", s.Names)
		}
		if s.Names[0] == "thumb" && (s.VMA != 0x03002400 || s.LMA == nil || *s.LMA != 0x08000040) {
			t.Fatalf("Thumb address not normalized: %+v", s)
		}
	}
}

func TestELFWithoutSymbolsStillReportsSections(t *testing.T) {
	got, err := analyzeELF(bytes.NewReader(fixtureELF(t, false)), "stripped.elf")
	if err != nil {
		t.Fatal(err)
	}
	if got.SymbolsAvailable || len(got.Symbols) != 0 || got.Totals.ROM != 48 {
		t.Fatalf("stripped result=%+v", got)
	}
	for _, s := range got.Sections {
		if s.Unattributed != s.Size {
			t.Fatalf("stripped section has attributed symbols: %+v", s)
		}
	}
}

func TestELFRejectsMalformedAndUnsupportedInput(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func([]byte) []byte
		want   string
	}{
		{"header", func(b []byte) []byte { return b[:12] }, "read ELF"},
		{"machine", func(b []byte) []byte { binary.LittleEndian.PutUint16(b[18:], uint16(elf.EM_386)); return b }, "ELF32 ARM"},
		{"relocatable", func(b []byte) []byte { binary.LittleEndian.PutUint16(b[16:], uint16(elf.ET_REL)); return b }, "executable"},
		{"truncated-load-backing", func(b []byte) []byte { binary.LittleEndian.PutUint32(b[56:], 0x900); return b }, "truncated PT_LOAD"},
		{"file-exceeds-memory", func(b []byte) []byte { binary.LittleEndian.PutUint32(b[72:], 16); return b }, "file size exceeds"},
		{"outside-ROM", func(b []byte) []byte { binary.LittleEndian.PutUint32(b[64:], 0x02000000); return b }, "outside GBA ROM"},
		{"missing-load", func(b []byte) []byte { binary.LittleEndian.PutUint32(b[0x600+2*40+16:], 0x220); return b }, "no complete"},
		{"conflicting-load", func(b []byte) []byte { binary.LittleEndian.PutUint32(b[128:], 0x08000044); return b }, "conflicting"},
		{"symbol-outside-section", func(b []byte) []byte { binary.LittleEndian.PutUint32(b[0x300+3*16+8:], 20); return b }, "extends outside"},
		{"symbol-missing-section", func(b []byte) []byte { binary.LittleEndian.PutUint16(b[0x300+3*16+14:], 15); return b }, "missing section"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := analyzeELF(bytes.NewReader(tc.mutate(fixtureELF(t, true))), tc.name)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v want %q", err, tc.want)
			}
		})
	}
	if _, err := checkedSpan(^uint64(0)-1, 3); err == nil {
		t.Fatal("accepted overflowing address range")
	}
}

func TestSymbolDeltasIgnoreAddressShiftAndGroupAliases(t *testing.T) {
	before, err := analyzeELF(bytes.NewReader(fixtureELF(t, true)), "before")
	if err != nil {
		t.Fatal(err)
	}
	after, err := analyzeELF(bytes.NewReader(fixtureELF(t, true)), "after")
	if err != nil {
		t.Fatal(err)
	}
	for i := range after.Symbols {
		after.Symbols[i].VMA += 100
		if after.Symbols[i].Names[0] == "foo" {
			after.Symbols[i].Size += 4
			after.Symbols[i].Cost.ROM += 4
		}
	}
	delta := compareImages(before, after)
	if len(delta.Symbols) != 1 || delta.Symbols[0].Name != "foo = foo_alias" || delta.Symbols[0].Delta != 4 {
		t.Fatalf("alias/relocation delta=%+v", delta.Symbols)
	}
	after.SymbolsAvailable = false
	if got := compareImages(before, after); got.SymbolsComparable || len(got.Symbols) != 0 {
		t.Fatal("invented symbol deltas for stripped ELF")
	}
}

func TestCLIJSONAndErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture.elf")
	if err := os.WriteFile(path, fixtureELF(t, true), 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if err := run([]string{"-before", path, "-after", path, "-json"}, &out, &stderr); err != nil {
		t.Fatal(err)
	}
	var decoded report
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Comparison == nil || decoded.Comparison.Totals != (signedCost{}) {
		t.Fatalf("CLI comparison=%+v", decoded)
	}
	first := out.String()
	out.Reset()
	if err := run([]string{"-before", path, "-after", path, "-json"}, &out, &stderr); err != nil {
		t.Fatal(err)
	}
	if out.String() != first {
		t.Fatal("JSON output is nondeterministic")
	}
	for _, args := range [][]string{nil, {"-top", "-1", path}, {"-after", path, path}, {"missing.elf"}} {
		if err := run(args, &out, &stderr); err == nil {
			t.Fatalf("accepted arguments %v", args)
		}
	}
	if err := run([]string{"-h"}, &out, &stderr); err != nil {
		t.Fatalf("help should succeed: %v", err)
	}
}

func TestSavedELFReferences(t *testing.T) {
	root := os.Getenv("MTILE_ELFSIZE_FIXTURES")
	if root == "" {
		t.Skip("set MTILE_ELFSIZE_FIXTURES to the saved experiment directory")
	}
	for _, tc := range []struct {
		name, elf, rom                  string
		romBytes, iwram, ewram, mailbox uint64
	}{
		{"testio-mode0", "native-baseline/mode0-game.elf", "native-baseline/mode0.gba", 24376, 10200, 19956, 0},
		{"testio-mode4", "native-baseline/mode4-game.elf", "native-baseline/mode4.gba", 26356, 13220, 75936, 0},
		{"hash-baseline", "hash-fingerprint/native/collision-mode0/baseline/game.elf", "hash-fingerprint/native/collision-mode0/baseline/mode0.gba", 42940, 11660, 153928, 512},
		{"hash-candidate", "hash-fingerprint/native/collision-mode0/candidate/game.elf", "hash-fingerprint/native/collision-mode0/candidate/mode0.gba", 43228, 11764, 153928, 512},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := readImage(filepath.Join(root, tc.elf))
			if err != nil {
				t.Fatal(err)
			}
			if want := (cost{ROM: tc.romBytes, IWRAM: tc.iwram, EWRAM: tc.ewram, Mailbox: tc.mailbox}); got.Totals != want {
				t.Fatalf("totals=%+v want %+v", got.Totals, want)
			}
			rom, err := os.Stat(filepath.Join(root, tc.rom))
			if err != nil {
				t.Fatal(err)
			}
			if got.ROMImageBytes != uint64(rom.Size()) || got.ROMSegmentPadding != 0 {
				t.Fatalf("ROM span/padding=%d/%d file=%d", got.ROMImageBytes, got.ROMSegmentPadding, rom.Size())
			}
		})
	}
	before, err := readImage(filepath.Join(root, "hash-fingerprint/native/collision-mode0/baseline/game.elf"))
	if err != nil {
		t.Fatal(err)
	}
	after, err := readImage(filepath.Join(root, "hash-fingerprint/native/collision-mode0/candidate/game.elf"))
	if err != nil {
		t.Fatal(err)
	}
	delta := compareImages(before, after)
	want := map[string]int64{"(*github.com/drpaneas/mtile/runtime.Context).TableSet": 104, "(*github.com/drpaneas/mtile/runtime.Context).hashFind": 104, "(*github.com/drpaneas/mtile/runtime.Context).growTableWithRoots": 80}
	if len(delta.Symbols) != len(want) {
		t.Fatalf("changed symbols=%+v", delta.Symbols)
	}
	for _, s := range delta.Symbols {
		if s.Delta != want[s.Name] {
			t.Fatalf("unexpected symbol delta=%+v", s)
		}
		if s.Name == "(*github.com/drpaneas/mtile/runtime.Context).hashFind" && (s.Cost.ROM != 104 || s.Cost.IWRAM != 104) {
			t.Fatalf("hashFind dual cost=%+v", s)
		}
	}
	if delta.Totals.ROM != 288 || delta.Totals.IWRAM != 104 {
		t.Fatalf("hash totals=%+v", delta.Totals)
	}
}

var elfSizeBenchmarkSink imageSize

func BenchmarkELFParse(b *testing.B) {
	root := os.Getenv("MTILE_ELFSIZE_FIXTURES")
	if root == "" {
		b.Skip("set MTILE_ELFSIZE_FIXTURES for real ELF measurements")
	}
	for _, name := range []string{"baseline", "candidate"} {
		b.Run(name, func(b *testing.B) {
			data, err := os.ReadFile(filepath.Join(root, "hash-fingerprint/native/collision-mode0", name, "game.elf"))
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			b.ResetTimer()
			for range b.N {
				elfSizeBenchmarkSink, err = analyzeELF(bytes.NewReader(data), name)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
