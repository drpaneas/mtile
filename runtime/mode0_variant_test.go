//go:build mtile_mode0 && mtile_mode0_obj

package runtime

import (
	"encoding/binary"
	"testing"

	"github.com/drpaneas/mtile/runtime/hw"
)

func TestMode0PoseVariantsShare4bppPixelsAndKeepIndependentBanks(t *testing.T) {
	c := plannedMode0PoseFixture(t, []PosePart{{Tile: 0}})
	c.cart.PoseVariantBaseCount = 1
	c.cart.PoseVariantNames = []string{"fire"}
	c.cart.PoseIndex = append(c.cart.PoseIndex, c.cart.PoseIndex[0])
	c.cart.PoseOBJIndex = []PoseOBJRange{{Count: 1, TileCount: 2}, {Start: 1, Count: 1, TileCount: 2}}
	c.cart.PoseOBJParts = []PoseOBJ{{TileOffset: 1, Width: 8, Height: 8, Palette: 1}, {TileOffset: 1, Width: 8, Height: 8, Palette: 2}}
	packed := make([]byte, 64)
	packed[32+2*4] = 0x10   // source pixel (1,2), local color 1
	packed[32+5*4+3] = 0x02 // source pixel (6,5), local color 2
	c.cart.PoseOBJResident = make([]uint32, 16)
	for i := range c.cart.PoseOBJResident {
		c.cart.PoseOBJResident[i] = binary.LittleEndian.Uint32(packed[i*4:])
	}
	c.cart.PoseOBJResidentBase = 17
	c.cart.PoseOBJPalettes = make([]uint8, 32)
	c.cart.PoseOBJPalettes[1], c.cart.PoseOBJPalettes[2] = 7, 9
	c.cart.PoseOBJPalettes[17], c.cart.PoseOBJPalettes[18] = 10, 12
	c.cart.AnimationClips = []AnimationClip{{Name: "idle", Count: 1, FPS: 8, Loop: true, FlipOffsetX: 8, FlipOffsetY: 8}}
	c.cart.AnimationFrames = []uint16{0}
	c.paletteDirty = true
	c.FinalizeInitHardware()
	for frame := 0; frame < 3; frame++ {
		c.BeginRenderFrame()
		c.Cls(0)
		c.PVariantID(0)
		c.PSpr(0, 20, 30, false, false)
		c.PVariantStaticV(c.InternStatic("fire"))
		c.AnimV(c.InternStatic("idle"), NumVal(FromInt(40)), NumVal(FromInt(30)), BoolVal(true), BoolVal(true), NilVal())
		c.PrepareRender()
		c.PresentRender()
		if c.hasMode0PsprUploads() || c.RendererUploadStats().LastOBJTileBytes != 0 {
			t.Fatal("variant switch uploaded pixels")
		}
		found := 0
		for slot := 0; slot < 128; slot++ {
			base := hw.OAM + uintptr(slot*8)
			a0, a1, a2 := hw.GetReg16(base), hw.GetReg16(base+2), hw.GetReg16(base+4)
			if a0&mode0ObjectDisable != 0 {
				continue
			}
			found++
			if a0&0x2000 != 0 {
				t.Fatal("variant object uses 8bpp")
			}
			if a2&0x3ff != 35 {
				t.Fatalf("tile units=%d want 35", a2&0x3ff)
			}
			bank := int(a2 >> 12)
			if bank != 0 && bank != 1 {
				t.Fatalf("bank=%d", bank)
			}
			wantX := uint16(20)
			if bank == 1 {
				wantX = 40
			}
			if a1&511 != wantX || a0&255 != 30 {
				t.Fatalf("bank %d object position=(%d,%d)", bank, a1&511, a0&255)
			}
			flip := a1 & (3 << 12)
			if (bank == 0 && flip != 0) || (bank == 1 && flip != 3<<12) {
				t.Fatalf("bank=%d flip=%x", bank, flip)
			}
			want := uint16(c.palette[7])
			if bank == 1 {
				want = c.palette[10]
			}
			if got := hw.ReadPaletteRAM16(256 + bank*16 + 1); got != want {
				t.Fatalf("bank %d palette=%x want %x", bank, got, want)
			}
			// Independently decode the asymmetric source pixel and reflection.
			sx, sy := 1, 2
			if bank == 1 {
				sx, sy = 6, 5
			}
			if flip != 0 {
				sx, sy = 7-sx, 7-sy
			}
			address := hw.OBJVRAM + uintptr(int(a2&0x3ff)*32+sy*4+sx/2)
			value := byte(hw.ReadVRAM32(address&^3)>>((address&3)*8)) >> uint((sx&1)*4) & 15
			if value != 1 {
				t.Fatalf("bank %d decoded local index=%d", bank, value)
			}
		}
		if found != 2 {
			t.Fatalf("visible objects=%d", found)
		}
	}
}
