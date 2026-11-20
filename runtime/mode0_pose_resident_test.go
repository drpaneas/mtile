//go:build mtile_mode0 && mtile_mode0_obj && !mtile_mode0_no_pspr

package runtime

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
)

func TestMode0ResidentPosePixelsFlipsCameraAndNoUploads(t *testing.T) {
	for _, parts := range [][]PosePart{
		rectangularPose(8, 8),
		{{Tile: 1, X: -5, Y: -3}, {Tile: 2, X: -1, Y: 1}, {Tile: 3, X: 7, Y: 1, Flags: PoseFlipX | PoseFlipY}},
	} {
		for flip := 0; flip < 4; flip++ {
			for _, position := range [][2]int{{40, 30}, {-10, -10}, {230, 150}} {
				t.Run(fmt.Sprintf("parts%d-flip%d-%v", len(parts), flip, position), func(t *testing.T) {
					c := plannedMode0PoseFixture(t, parts)
					words := make([]uint32, len(c.cart.PoseOBJTiles)*16)
					for at, tile := range c.cart.PoseOBJTiles {
						pixels := c.poseBankCell(int(tile))
						for i := 0; i < 16; i++ {
							words[at*16+i] = binary.LittleEndian.Uint32(pixels[i*4:])
						}
					}
					c.cart.PoseOBJResident, c.cart.PoseOBJResidentBase = words, 17
					c.cart.PoseOBJStage = nil
					coldBefore := c.RendererUploadStats().ColdTileBytes
					c.FinalizeInitHardware()
					if got, want := c.RendererUploadStats().ColdTileBytes, coldBefore+uint32(len(words)*4); got != want {
						t.Fatalf("cold tile bytes = %d, want %d including the resident OBJ bank", got, want)
					}
					for frame := 0; frame < 3; frame++ {
						c.BeginRenderFrame()
						c.Cls(0)
						c.Camera(3, 5)
						x, y := position[0]+frame, position[1]
						c.PSpr(0, x, y, flip&1 != 0, flip&2 != 0)
						c.PrepareRender()
						if c.hasMode0PsprUploads() {
							t.Fatal("resident drawing scheduled a visible tile upload")
						}
						c.PresentRender()
						if c.RendererUploadStats().LastOBJTileBytes != 0 {
							t.Fatal("resident pose transferred pixels after initialization")
						}
						got, want := composeMode0Objects(t), authoredPosePixels(c, parts, x, y, flip&1 != 0, flip&2 != 0)
						if !bytes.Equal(got, want) {
							t.Fatal("resident pose changed authored pixels, flips, clip or camera semantics")
						}
					}
				})
			}
		}
	}
}

func TestMode0ResidentPoseInitializationValidatesBankAndOffsets(t *testing.T) {
	image := AssetImage{PoseIndex: []PoseRange{{Count: 1}}, PoseParts: []PosePart{{}},
		PoseOBJIndex: []PoseOBJRange{{Count: 1, TileCount: 1}},
		PoseOBJParts: []PoseOBJ{{Width: 8, Height: 8}}, PoseOBJResident: make([]uint32, 16), PoseOBJResidentBase: 479}
	if !poseIndexValid(&image) {
		t.Fatal("valid last resident tile rejected")
	}
	image.PoseOBJResidentBase = 480
	if poseIndexValid(&image) {
		t.Fatal("resident OBJ bank past VRAM accepted")
	}
	image.PoseOBJResidentBase = 0
	image.PoseOBJParts[0].TileOffset = 1
	if poseIndexValid(&image) {
		t.Fatal("resident object outside its bank accepted")
	}
	image.PoseOBJParts[0].TileOffset = 0
	image.PoseOBJResident = make([]uint32, 15)
	if poseIndexValid(&image) {
		t.Fatal("partial resident tile accepted")
	}
}
