// Package assetsync loads the native GBA asset format and emits the static
// data consumed by generated TinyGo programs.
//
// Native assets are deliberately independent: the sprite sheet is an 8bpp
// pixel buffer, the map is a uint16 cell buffer, and flags are one byte per
// complete 8x8 sprite cell. Sprite-sheet pixels and map cells do not share a
// single address space.
package assetsync

import (
	"fmt"
	"math"
)

const (
	CanonicalPaletteName   = "aurora"
	DefaultRendererName    = "mode4"
	NewProjectRendererName = "auto"
	MaxPaletteNameLength   = 32
	MaxSpriteN             = 65536
	// OBJ graphics live in ROM. Tile IDs use uint16; they do not share the
	// map cache's EWRAM budget, and sheets can use any aspect ratio.
	MaxObjSheetCells = MaxSpriteN
	DefaultObjSheetW = 512
	DefaultObjSheetH = 320
	// MapStreamingThresholdCells must match runtime.MapStreamingThresholdCells.
	// Maps above this size use the fixed ROM-backed chunk cache instead of a
	// full EWRAM copy. Keep the value here so assetsync does not import runtime.
	MapStreamingThresholdCells = 16384
	MapSchemaVersion           = 2
	MapLayerCount              = 4
)

// Project is the small project-level asset configuration. Only one palette
// is active globally; runtime pal() remaps colors at draw/display time.
type Project struct {
	ActivePalette string `json:"activePalette"`
	// Entry is an optional project-relative Lua entry point. Empty preserves
	// the original root game.lua convention.
	Entry string `json:"entry,omitempty"`
	// Renderer is the build-time display policy: mode4, mode0, or auto. Empty
	// is normalized to mode4 so projects created before this field retain their
	// established framebuffer behavior.
	Renderer string `json:"renderer,omitempty"`
	// IWRAMFunctions optionally places named top-level Lua helpers in fast
	// internal RAM. Lifecycle callbacks remain under the generated shell.
	IWRAMFunctions []string `json:"iwramFunctions,omitempty"`
}

// RGB15 is a GBA-compatible color component triple. Components are five-bit
// values, not the 0..32 range sometimes used in informal GBA documentation.
type RGB15 struct {
	R int `json:"r"`
	G int `json:"g"`
	B int `json:"b"`
}

// Palette is the on-disk reusable palette format. Exactly 16 colors (a
// shorthand that expands the remaining entries from color 0) or 256 colors
// are accepted.
type Palette struct {
	Name   string  `json:"name"`
	Colors []RGB15 `json:"colors"`
}

// SpriteRecord describes authored pixels at a location in the native sheet.
// Entries may be sparse and may use rectangles larger than one 8x8 cell; the
// runtime still addresses the resulting flattened sheet in 8x8 cells.
type SpriteRecord struct {
	ID     int     `json:"id"`
	X      int     `json:"x"`
	Y      int     `json:"y"`
	Width  int     `json:"width"`
	Height int     `json:"height"`
	Pixels [][]int `json:"pixels"`
	Flags  int     `json:"flags"`
}

// Spritesheet is the native project sheet format.
type Spritesheet struct {
	Width   int            `json:"width"`
	Height  int            `json:"height"`
	Sprites []SpriteRecord `json:"sprites"`
}

const (
	// Lua fixed-point integers represent nonnegative pose IDs through 32767.
	MaxPoseCount = 32768
	MaxPoseParts = 65535
	// Each flattened part occupies 8 bytes in the 32 MiB cartridge address space.
	MaxPosePartTotal = (32 * 1024 * 1024) / 8
	MaxPoseNameBytes = 32
)

type PosePartRecord struct {
	Tile  int  `json:"tile"`
	X     int  `json:"x"`
	Y     int  `json:"y"`
	FlipX bool `json:"flipX,omitempty"`
	FlipY bool `json:"flipY,omitempty"`
}

type PoseRecord struct {
	Name  string           `json:"name,omitempty"`
	Parts []PosePartRecord `json:"parts"`
}

// PoseColorVariant stores replacements, never another copy of the artwork.
type PoseColorVariant struct {
	Name   string          `json:"name"`
	Colors []PoseColorSwap `json:"colors"`
}
type PoseColorSwap struct {
	From int `json:"from"`
	To   int `json:"to"`
}
type PoseFile struct {
	Version  int                `json:"version"`
	Poses    []PoseRecord       `json:"poses"`
	Variants []PoseColorVariant `json:"variants,omitempty"`
}

// FlatPosePart is one draw step within a flattened pose: which sprite tile
// to draw and the pixel offset and flip flags to draw it with. Flags packs
// flipX in bit 0 and flipY in bit 1; the remaining bits are reserved and
// always zero.
type FlatPosePart struct {
	Tile  uint16
	X, Y  int16
	Flags uint8
}

// FlatPose names a contiguous span within Assets.PoseParts: parts
// [Start, Start+Count) draw the pose, in the same order as poses.json.
type FlatPose struct {
	Start                    uint32
	Count                    uint16
	FlipOffsetX, FlipOffsetY int32
}

// FlatAnimationClip addresses immutable global pose IDs in AnimationFrames.
type FlatAnimationClip struct {
	Name                     string
	Start, Count             uint32
	FPS                      uint8
	Loop                     bool
	FlipOffsetX, FlipOffsetY int32
}

// MapLayer is one fixed logical background plane. IDs are stable and range
// from BG0 (back) through BG3 (front); array order is not semantic.
type MapLayer struct {
	ID    int   `json:"id"`
	Cells []int `json:"cells"`
}

// Map is the native dense uint16-ID map format. Version 0 is the original
// single-plane shape and stores Cells directly. Version 2 stores four
// independent background planes in Layers. Readers support both shapes so
// opening an existing project never rewrites or expands it implicitly.
type Map struct {
	Version int        `json:"version,omitempty"`
	Width   int        `json:"width"`
	Height  int        `json:"height"`
	Cells   []int      `json:"cells,omitempty"`
	Layers  []MapLayer `json:"layers,omitempty"`
}

// Assets is the validated, flattened representation used by code generation.
type Assets struct {
	Project      Project
	AudioVersion int
	NativeAudio  *NativeAudioAssets

	Palette              Palette
	PaletteROM           [256]uint16
	SheetW               int
	SheetH               int
	SpriteCols           int
	SpriteRows           int
	SpriteN              int
	MapW                 int
	MapH                 int
	MapLayers            int
	MapStreaming         bool
	SheetROM             []byte
	MapROM               []uint16
	FlagROM              []byte
	ObjSheetW            int
	ObjSheetH            int
	ObjSpriteCols        int
	ObjSpriteRows        int
	ObjSpriteN           int
	ObjSheetROM          []byte
	PoseVariantBaseCount int
	PoseVariantNames     []string
	PoseIndex            []FlatPose
	PoseParts            []FlatPosePart
	// PoseNames are build/debugger labels from the same validated source as
	// PoseIndex; they are not added to the cartridge's runtime pose records.
	PoseNames       []string
	AnimationClips  []FlatAnimationClip
	AnimationFrames []uint16
	// TileLinks draw linked shared tiles as poses or clips; see tilelinks.go.
	TileLinks []FlatTileLink
}

// Valid reports whether each component is a 5-bit GBA channel (0..31).
func (p RGB15) Valid() error {
	if p.R < 0 || p.R > 31 || p.G < 0 || p.G > 31 || p.B < 0 || p.B > 31 {
		return fmt.Errorf("RGB15 component out of range: {%d,%d,%d} (want 0..31)", p.R, p.G, p.B)
	}
	return nil
}

// Packed returns the BGR555 hardware word: b<<10 | g<<5 | r.
func (p RGB15) Packed() uint16 {
	return uint16(p.R) | uint16(p.G)<<5 | uint16(p.B)<<10
}

func expandPalette(p Palette) ([256]uint16, error) {
	var out [256]uint16
	if p.Name == "" {
		return out, fmt.Errorf("palette name is required")
	}
	if err := validatePaletteName(p.Name); err != nil {
		return out, err
	}
	if len(p.Colors) != 16 && len(p.Colors) != 256 {
		return out, fmt.Errorf("palette %q has %d colors (want 16 or 256)", p.Name, len(p.Colors))
	}
	for i, color := range p.Colors {
		if err := color.Valid(); err != nil {
			return out, fmt.Errorf("palette %q color %d: %w", p.Name, i, err)
		}
		out[i] = color.Packed()
	}
	if len(p.Colors) == 16 {
		for i := 16; i < len(out); i++ {
			out[i] = out[0]
		}
	}
	return out, nil
}

// ExpandedPaletteColors returns the editor-facing RGB15 table after applying
// the same 16-color shorthand rule used by code generation. A 16-color file
// therefore has its entries 16..255 aliased to entry 0; a 256-color file is
// returned unchanged.
func ExpandedPaletteColors(p Palette) ([256]RGB15, error) {
	if _, err := expandPalette(p); err != nil {
		return [256]RGB15{}, err
	}
	var out [256]RGB15
	copy(out[:], p.Colors)
	if len(p.Colors) == 16 {
		for i := 16; i < len(out); i++ {
			out[i] = out[0]
		}
	}
	return out, nil
}

func validatePaletteName(name string) error {
	if name == "" || len(name) > MaxPaletteNameLength {
		return fmt.Errorf("palette name %q must be 1..%d characters", name, MaxPaletteNameLength)
	}
	for i, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || (r == '-' && i > 0 && i < len(name)-1) {
			continue
		}
		return fmt.Errorf("palette name %q must use lowercase letters, digits, and internal hyphens", name)
	}
	return nil
}

func checkedProduct(a, b int) (int, error) {
	if a < 0 || b < 0 {
		return 0, fmt.Errorf("negative dimensions %d×%d", a, b)
	}
	if a != 0 && b > math.MaxInt/a {
		return 0, fmt.Errorf("dimension product %d×%d overflows int", a, b)
	}
	product := a * b
	if uint64(product) > uint64(^uint32(0)) {
		return 0, fmt.Errorf("dimension product %d×%d exceeds uint32 asset limit", a, b)
	}
	return product, nil
}

func checkedSum(a, b int) (int, error) {
	if a < 0 || b < 0 || a > math.MaxInt-b {
		return 0, fmt.Errorf("coordinate sum overflows int")
	}
	return a + b, nil
}
