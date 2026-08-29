package assetsync

import (
	"go/format"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func testPalette(name string, n int) Palette {
	colors := make([]RGB15, n)
	for i := range colors {
		colors[i] = RGB15{R: i & 31, G: (i + 1) & 31, B: (i + 2) & 31}
	}
	return Palette{Name: name, Colors: colors}
}

func testSprite(id, x, y, width, height, pixel, flags int) SpriteRecord {
	pixels := make([][]int, height)
	for row := range pixels {
		pixels[row] = make([]int, width)
		for col := range pixels[row] {
			pixels[row][col] = pixel
		}
	}
	return SpriteRecord{ID: id, X: x, Y: y, Width: width, Height: height, Pixels: pixels, Flags: flags}
}

func writeTestProject(t *testing.T, dir string, palette Palette, sheet Spritesheet, mapData Map) {
	t.Helper()
	if palette.Name != CanonicalPaletteName {
		if err := SavePalette(dir, &palette); err != nil {
			t.Fatal(err)
		}
	}
	if err := SaveSpritesheet(dir, &sheet); err != nil {
		t.Fatal(err)
	}
	if err := SaveMap(dir, &mapData); err != nil {
		t.Fatal(err)
	}
}

func nativeFixture() (Spritesheet, Map) {
	return Spritesheet{
		Width: 9, Height: 9,
		Sprites: []SpriteRecord{testSprite(0, 0, 0, 1, 1, 15, 0xa5)},
	}, Map{Width: 2, Height: 1, Cells: []int{0, 0}}
}

func TestLoadFlattensNativeAssets(t *testing.T) {
	dir := t.TempDir()
	sheet, mapData := nativeFixture()
	writeTestProject(t, dir, canonicalPalette(), sheet, mapData)

	assets, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if assets.SheetW != 9 || assets.SheetH != 9 || assets.SpriteCols != 1 || assets.SpriteRows != 1 || assets.SpriteN != 1 {
		t.Fatalf("unexpected sheet metadata: %+v", assets)
	}
	if assets.MapW != 2 || assets.MapH != 1 || !slices.Equal(assets.MapROM, []uint16{0, 0}) {
		t.Fatalf("unexpected map metadata/data: %dx%d %#v", assets.MapW, assets.MapH, assets.MapROM)
	}
	if len(assets.SheetROM) != 81 || assets.SheetROM[0] != 15 || assets.FlagROM[0] != 0xa5 {
		t.Fatalf("unexpected flattened sheet/flags: len=%d first=%d flags=%#x", len(assets.SheetROM), assets.SheetROM[0], assets.FlagROM[0])
	}
	if assets.Palette.Name != CanonicalPaletteName || len(assets.Palette.Colors) != 256 || assets.PaletteROM[16] == assets.PaletteROM[0] {
		t.Fatalf("unexpected canonical palette: name=%q colors=%d rom16=%#x rom0=%#x", assets.Palette.Name, len(assets.Palette.Colors), assets.PaletteROM[16], assets.PaletteROM[0])
	}
}

func TestProjectRendererDefaultsPersistsAndValidates(t *testing.T) {
	dir := t.TempDir()
	project, err := LoadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if project.Renderer != DefaultRendererName {
		t.Fatalf("missing project renderer = %q, want %q", project.Renderer, DefaultRendererName)
	}
	project.Renderer = "auto"
	if err := SaveProject(dir, project); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Renderer != "auto" {
		t.Fatalf("reloaded renderer = %q", reloaded.Renderer)
	}
	project.Renderer = "adaptive"
	if err := SaveProject(dir, project); err == nil || !strings.Contains(err.Error(), "mode4, mode0, or auto") {
		t.Fatalf("invalid renderer error = %v", err)
	}
}

func Test128x64MapKeepsFullMutableRAMPath(t *testing.T) {
	sheet := Spritesheet{Width: 16, Height: 8}
	mapData := Map{Width: 128, Height: 64, Cells: make([]int, 128*64)}
	assets, err := Flatten(Project{ActivePalette: CanonicalPaletteName}, sheet, mapData, canonicalPalette())
	if err != nil {
		t.Fatal(err)
	}
	if assets.MapStreaming {
		t.Fatalf("128x64 map unexpectedly selected streaming: %dx%d", assets.MapW, assets.MapH)
	}
	if len(assets.MapROM) != 8192 {
		t.Fatalf("map cells = %d, want 8192", len(assets.MapROM))
	}
}

func TestPaletteExpansion(t *testing.T) {
	p := testPalette("sixteen", 16)
	rom, err := expandPalette(p)
	if err != nil {
		t.Fatal(err)
	}
	if rom[15] != p.Colors[15].Packed() || rom[16] != p.Colors[0].Packed() || rom[255] != p.Colors[0].Packed() {
		t.Fatalf("16-color palette did not expand from color zero")
	}

	p = testPalette("full", 256)
	rom, err = expandPalette(p)
	if err != nil {
		t.Fatal(err)
	}
	if rom[255] != p.Colors[255].Packed() || rom[255] == rom[0] {
		t.Fatalf("256-color palette was not preserved")
	}

	canonical, err := LoadPalette(t.TempDir(), CanonicalPaletteName)
	if err != nil {
		t.Fatal(err)
	}
	if len(canonical.Colors) != 256 || canonical.Colors[1] != (RGB15{R: 2, G: 2, B: 2}) {
		t.Fatalf("unexpected canonical aurora palette: len=%d color1=%#v", len(canonical.Colors), canonical.Colors[1])
	}
}

func TestPaletteCRUD(t *testing.T) {
	dir := t.TempDir()
	created := testPalette("forest", 16)
	if err := CreatePalette(dir, created); err != nil {
		t.Fatal(err)
	}
	if err := DuplicatePalette(dir, "forest", "bright"); err != nil {
		t.Fatal(err)
	}
	if err := SetActivePalette(dir, "forest"); err != nil {
		t.Fatal(err)
	}
	if err := RenamePalette(dir, "forest", "forest-night"); err != nil {
		t.Fatal(err)
	}
	project, err := LoadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if project.ActivePalette != "forest-night" {
		t.Fatalf("renaming active palette did not update project: %#v", project)
	}
	if err := DeletePalette(dir, "forest-night"); err == nil {
		t.Fatal("deleting active palette unexpectedly succeeded")
	}
	if err := SetActivePalette(dir, CanonicalPaletteName); err != nil {
		t.Fatal(err)
	}
	if err := DeletePalette(dir, "forest-night"); err != nil {
		t.Fatal(err)
	}
	if err := DeletePalette(dir, CanonicalPaletteName); err == nil {
		t.Fatal("deleting built-in palette unexpectedly succeeded")
	}

	names, err := ListPalettes(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(names, []string{CanonicalPaletteName, "bright"}) {
		t.Fatalf("unexpected palette list: %#v", names)
	}
}

func TestPaletteCreationAndRenameReserveCanonicalName(t *testing.T) {
	dir := t.TempDir()
	if err := CreatePalette(dir, testPalette("../escape", 16)); err == nil {
		t.Fatal("CreatePalette accepted a path-like palette name")
	}
	if err := CreatePalette(dir, testPalette(CanonicalPaletteName, 16)); err == nil {
		t.Fatal("CreatePalette accepted the implicit canonical palette name")
	}

	if err := CreatePalette(dir, testPalette("dawn", 16)); err != nil {
		t.Fatal(err)
	}
	if err := RenamePalette(dir, "dawn", CanonicalPaletteName); err == nil {
		t.Fatal("RenamePalette accepted the canonical palette as a destination")
	}
	if _, err := LoadPalette(dir, "dawn"); err != nil {
		t.Fatalf("failed rename removed the source palette: %v", err)
	}
}

// TestDeletePaletteRejectsCanonicalBeforeCheckingActive guards precedence
// between DeletePalette's two rejection reasons: even when the canonical
// palette happens to be the active one (the common case, since it is the
// default), the error must name it as the built-in palette rather than the
// active one, since "select another palette first" is misleading advice for
// a palette that can never be deleted regardless of selection.
func TestDeletePaletteRejectsCanonicalBeforeCheckingActive(t *testing.T) {
	dir := t.TempDir() // fresh project: canonical palette is active by default
	err := DeletePalette(dir, CanonicalPaletteName)
	if err == nil {
		t.Fatal("deleting the canonical palette unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "built-in") {
		t.Fatalf("expected a built-in-palette error, got: %v", err)
	}
}

// TestDeletePaletteUncheckedIgnoresDiskActiveSelection guards the variant
// editor.Model uses: unlike DeletePalette, it must not consult project.json
// at all, even when project.json still names the palette being deleted as
// active (e.g. because a caller's own in-memory selection has moved on but
// has not yet been saved).
func TestDeletePaletteUncheckedIgnoresDiskActiveSelection(t *testing.T) {
	dir := t.TempDir()
	p := testPalette("dawn", 16)
	if err := SavePalette(dir, &p); err != nil {
		t.Fatal(err)
	}
	if err := SetActivePalette(dir, "dawn"); err != nil {
		t.Fatal(err)
	}
	if err := DeletePaletteUnchecked(dir, "dawn"); err != nil {
		t.Fatalf("DeletePaletteUnchecked rejected a disk-active palette: %v", err)
	}
	if err := DeletePaletteUnchecked(dir, CanonicalPaletteName); err == nil {
		t.Fatal("DeletePaletteUnchecked accepted the canonical palette")
	}
}

func TestMissingActivePaletteListsAvailablePalettes(t *testing.T) {
	dir := t.TempDir()
	p := testPalette("dawn", 16)
	if err := SavePalette(dir, &p); err != nil {
		t.Fatal(err)
	}
	if err := SaveProject(dir, Project{ActivePalette: "missing"}); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPalette(dir, "missing"); err == nil || !strings.Contains(err.Error(), "available palettes: aurora, dawn") {
		t.Fatalf("missing palette error did not list available palettes: %v", err)
	}
}

func TestValidationRejectsOverflowAndOutOfRangeData(t *testing.T) {
	if _, _, _, _, err := validateSheet(Spritesheet{Width: math.MaxInt, Height: 2}, 16); err == nil {
		t.Fatal("overflowing sheet dimensions unexpectedly passed")
	}
	if _, _, _, _, err := validateSheet(Spritesheet{Width: 7, Height: 613566757}, 16); err == nil {
		t.Fatal("sheet product above uint32 unexpectedly passed")
	}
	if _, err := flatten(Project{}, Spritesheet{Width: 65536, Height: 65536}, Map{Width: 0, Height: 0}, testPalette("sixteen", 16)); err == nil {
		t.Fatal("sprite-grid product above uint32 unexpectedly passed")
	}
	if _, _, _, _, err := validateSheet(Spritesheet{Width: 2048, Height: 2056}, 16); err == nil {
		t.Fatal("sheet with more than 65536 sprites unexpectedly passed")
	}
	_, _, _, spriteN, err := validateSheet(Spritesheet{Width: 2048, Height: 2048}, 16)
	if err != nil || spriteN != MaxSpriteN {
		t.Fatalf("maximum uint16 sprite-id space was rejected: spriteN=%d err=%v", spriteN, err)
	}
	if err := validateMap(Map{Width: 1, Height: 1, Cells: []int{65535}}, MaxSpriteN); err != nil {
		t.Fatalf("maximum uint16 map cell was rejected: %v", err)
	}

	badPaletteSheet := Spritesheet{Width: 8, Height: 8, Sprites: []SpriteRecord{testSprite(0, 0, 0, 1, 1, 16, 0)}}
	if _, err := flatten(Project{}, badPaletteSheet, Map{Width: 1, Height: 1, Cells: []int{0}}, testPalette("sixteen", 16)); err == nil {
		t.Fatal("16-color palette accepted pixel 16")
	}
	if _, err := flatten(Project{}, badPaletteSheet, Map{Width: 1, Height: 1, Cells: []int{0}}, testPalette("full", 256)); err != nil {
		t.Fatalf("256-color palette rejected pixel 16: %v", err)
	}

	if err := validateMap(Map{Width: -1, Height: 1}, 1); err == nil {
		t.Fatal("negative map dimension unexpectedly passed")
	}
	if err := validateMap(Map{Width: 1, Height: 1, Cells: []int{1}}, 1); err == nil {
		t.Fatal("out-of-range map cell unexpectedly passed")
	}
	if err := validateMap(Map{Width: 1, Height: 1, Cells: nil}, 1); err == nil {
		t.Fatal("sparse map unexpectedly passed")
	}
	if err := validateMap(Map{Width: 1, Height: 1, Cells: []int{0}}, 0); err == nil {
		t.Fatal("map cell accepted when SpriteN is zero")
	}
}

func TestValidationRejectsDuplicateOverlapAndMalformedSprites(t *testing.T) {
	duplicate := Spritesheet{Width: 16, Height: 8, Sprites: []SpriteRecord{
		testSprite(0, 0, 0, 1, 1, 0, 0), testSprite(0, 2, 0, 1, 1, 0, 0),
	}}
	if _, _, _, _, err := validateSheet(duplicate, 16); err == nil {
		t.Fatal("duplicate sprite ID unexpectedly passed")
	}

	overlap := Spritesheet{Width: 16, Height: 8, Sprites: []SpriteRecord{
		testSprite(0, 0, 0, 8, 1, 0, 0), testSprite(1, 4, 0, 8, 1, 0, 0),
	}}
	if _, _, _, _, err := validateSheet(overlap, 16); err == nil {
		t.Fatal("validateSheet accepted overlapping sprite records")
	}
	if _, err := flatten(Project{}, overlap, Map{Width: 0, Height: 0}, testPalette("sixteen", 16)); err == nil {
		t.Fatal("overlapping sprites unexpectedly passed")
	}

	adjacent := Spritesheet{Width: 16, Height: 8, Sprites: []SpriteRecord{
		testSprite(0, 0, 0, 8, 1, 0, 0), testSprite(1, 8, 0, 8, 1, 0, 0),
	}}
	if _, err := flatten(Project{}, adjacent, Map{Width: 0, Height: 0}, testPalette("sixteen", 16)); err != nil {
		t.Fatalf("edge-adjacent sprites were treated as overlapping: %v", err)
	}
	stacked := Spritesheet{Width: 8, Height: 16, Sprites: []SpriteRecord{
		testSprite(0, 0, 0, 8, 8, 0, 0), testSprite(1, 0, 8, 8, 8, 0, 0),
	}}
	if _, err := flatten(Project{}, stacked, Map{Width: 0, Height: 0}, testPalette("sixteen", 16)); err != nil {
		t.Fatalf("vertically adjacent sprites were treated as overlapping: %v", err)
	}

	badRows := testSprite(0, 0, 0, 2, 2, 0, 0)
	badRows.Pixels[1] = []int{0}
	if _, _, _, _, err := validateSheet(Spritesheet{Width: 8, Height: 8, Sprites: []SpriteRecord{badRows}}, 16); err == nil {
		t.Fatal("malformed pixel row unexpectedly passed")
	}

	outOfBounds := testSprite(0, 7, 0, 2, 1, 0, 0)
	if _, _, _, _, err := validateSheet(Spritesheet{Width: 8, Height: 8, Sprites: []SpriteRecord{outOfBounds}}, 16); err == nil {
		t.Fatal("out-of-bounds sprite unexpectedly passed")
	}

	badFlags := testSprite(0, 0, 0, 1, 1, 0, 256)
	if _, _, _, _, err := validateSheet(Spritesheet{Width: 8, Height: 8, Sprites: []SpriteRecord{badFlags}}, 16); err == nil {
		t.Fatal("out-of-range flags unexpectedly passed")
	}
}

func TestSpriteOverlapValidationScalesToMaximumSpriteCount(t *testing.T) {
	sprites := make([]SpriteRecord, MaxSpriteN)
	for i := range sprites {
		// Adjacent one-pixel rectangles share an edge but do not overlap.
		sprites[i] = SpriteRecord{ID: i, X: i * 2, Y: 0, Width: 1, Height: 1}
	}
	if err := validateSpriteOverlaps(Spritesheet{Sprites: sprites}); err != nil {
		t.Fatalf("non-overlapping maximum-size sprite set was rejected: %v", err)
	}
}

func TestNativeSheetCanHaveNoCompleteSpriteCells(t *testing.T) {
	sheet := Spritesheet{Width: 7, Height: 8}
	assets, err := flatten(Project{}, sheet, Map{Width: 0, Height: 0}, testPalette("sixteen", 16))
	if err != nil {
		t.Fatal(err)
	}
	if assets.SpriteN != 0 || len(assets.FlagROM) != 0 || len(assets.SheetROM) != 56 {
		t.Fatalf("unexpected sub-8x8 sheet result: spriteN=%d flags=%d sheet=%d", assets.SpriteN, len(assets.FlagROM), len(assets.SheetROM))
	}
}

func TestGenerateGo(t *testing.T) {
	dir := t.TempDir()
	sheet, mapData := nativeFixture()
	writeTestProject(t, dir, canonicalPalette(), sheet, mapData)

	source, err := GenerateGo(dir, "gameassets")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := format.Source(source); err != nil {
		t.Fatalf("generated source is not gofmt-able: %v", err)
	}
	text := string(source)
	for _, want := range []string{
		"var sheetROM",
		"var mapROM",
		"var flagROM",
		"//go:section .ewram",
		"var cartData rt.AssetMemory",
		"func InitAssets(ctx *rt.Context) bool",
		"ctx.LoadPalette(&paletteROM)",
		`_ "unsafe"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("generated source missing %q", want)
		}
	}

	output := filepath.Join(dir, "generated", "assets.go")
	if err := GenerateFile(dir, output, "gameassets"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatal(err)
	}
}

// TestGenerateGoHandlesEmptySheetAndMapArrays guards the most common startup
// state - a brand-new project before anything is painted - which every
// other GenerateGo test skips by using a fixture with at least one sprite
// and one map cell. A sheet with no complete 8x8 cell (SpriteN=0) and an
// empty map produce zero-length flagROM/mapROM, and writeByteArray /
// writeUint16Array's loops never execute for a zero-length slice; this
// confirms the resulting `[0]byte{}` / `[0]uint16{}` literals are still
// syntactically valid, gofmt-able Go rather than malformed output that only
// a downstream TinyGo build would catch.
func TestGenerateGoHandlesEmptySheetAndMapArrays(t *testing.T) {
	dir := t.TempDir()
	sheet := Spritesheet{Width: 7, Height: 8}
	mapData := Map{Width: 0, Height: 0}
	if err := SaveSpritesheet(dir, &sheet); err != nil {
		t.Fatal(err)
	}
	if err := SaveMap(dir, &mapData); err != nil {
		t.Fatal(err)
	}
	source, err := GenerateGo(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := format.Source(source); err != nil {
		t.Fatalf("generated source with empty flagROM/mapROM is not gofmt-able: %v\n%s", err, source)
	}
	text := string(source)
	for _, want := range []string{"var mapROM = [0]uint16{}", "var flagROM = [0]byte{}"} {
		if !strings.Contains(text, want) {
			t.Errorf("generated source missing %q:\n%s", want, text)
		}
	}
}

func TestSaveMapUsesActivePaletteLength(t *testing.T) {
	dir := t.TempDir()
	p := testPalette("sixteen", 16)
	if err := SavePalette(dir, &p); err != nil {
		t.Fatal(err)
	}
	if err := SetActivePalette(dir, p.Name); err != nil {
		t.Fatal(err)
	}
	// Write an intentionally invalid sheet directly so SaveMap must validate it
	// against the selected 16-color palette rather than silently using 256.
	sheet := Spritesheet{Width: 8, Height: 8, Sprites: []SpriteRecord{testSprite(0, 0, 0, 1, 1, 16, 0)}}
	if err := writeJSON(filepath.Join(dir, "assets", "spritesheet.json"), sheet); err != nil {
		t.Fatal(err)
	}
	if err := SaveMap(dir, &Map{Width: 0, Height: 0}); err == nil {
		t.Fatal("SaveMap accepted sheet pixel outside active palette")
	}
}

func TestSaveSpritesheetRejectsOverlappingRecords(t *testing.T) {
	dir := t.TempDir()
	overlap := Spritesheet{Width: 16, Height: 8, Sprites: []SpriteRecord{
		testSprite(0, 0, 0, 8, 1, 0, 0), testSprite(1, 4, 0, 8, 1, 0, 0),
	}}
	if err := SaveSpritesheet(dir, &overlap); err == nil {
		t.Fatal("SaveSpritesheet accepted overlapping sprite records")
	}
}

func TestPaletteValidation(t *testing.T) {
	for _, palette := range []Palette{
		{Name: "", Colors: make([]RGB15, 16)},
		{Name: "UPPER", Colors: make([]RGB15, 16)},
		{Name: "bad-", Colors: make([]RGB15, 16)},
		{Name: "wrong-size", Colors: make([]RGB15, 15)},
		{Name: "bad-color", Colors: append(make([]RGB15, 15), RGB15{R: 32})},
	} {
		if _, err := expandPalette(palette); err == nil {
			t.Errorf("invalid palette %#v unexpectedly passed", palette)
		}
	}

	if err := SavePalette(t.TempDir(), &Palette{Name: "a", Colors: make([]RGB15, 16)}); err != nil {
		t.Fatal(err)
	}
	if err := SavePalette(t.TempDir(), &Palette{Name: "a/b", Colors: make([]RGB15, 16)}); err == nil {
		t.Fatal("palette path traversal/name unexpectedly passed")
	}
}
