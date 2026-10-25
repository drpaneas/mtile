//go:build (!mtile_mode0 && !mtile_tilecanvas) || (mtile_mode0 && mtile_mode0_obj)

package runtime

import (
	"bytes"
	"fmt"
	"testing"
)

// The same independent pixel expectations run against the software framebuffer
// and the composed hardware OAM, for both linked poses and clips.
func TestTileLinkMapFlagMaskPixelParity(t *testing.T) {
	for _, scan := range []bool{false, true} {
		for _, marker := range []uint16{tileLinkMarker, tileLinkClipMarker} {
			t.Run(fmt.Sprintf("scan=%t/marker=%d", scan, marker), func(t *testing.T) {
				c := tileLinkFixture(t)
				c.cart.Map[1*6+2] = marker
				if scan {
					c.tileLinkScanLayers = 1
				}
				for _, state := range [][2]int{{0, 1}, {1, 1}, {1, 3}, {3, 3}, {2, 1}, {0, 0}, {0, -1}, {0, 256}} {
					flags, mask := state[0], state[1]
					c.FsetByte(int(marker), flags)
					want := make([]byte, ScreenWidth*ScreenHeight)
					if mask <= 0 || mask > 255 || flags&mask == mask {
						for y := 8; y < 24; y++ {
							color := byte(tileLinkTopColor)
							if marker == tileLinkClipMarker {
								if y >= 16 {
									continue
								}
								color = tileLinkBottomColor
							} else if y >= 16 {
								color = tileLinkBottomColor
							}
							for x := 16; x < 24; x++ {
								want[y*ScreenWidth+x] = color
							}
						}
					}
					if got := tileLinkFrameForTest(t, c, 0, 0, 0, 0, mask); !bytes.Equal(got, want) {
						t.Fatalf("flags %d mask %d: linked pixels differ from filtered pose oracle", flags, mask)
					}
				}
			})
		}
	}
}

func TestTileLinkDefaultExtentReachPixelParity(t *testing.T) {
	for _, scan := range []bool{false, true} {
		for _, tc := range []struct {
			name                     string
			x, y, ox, oy, camX, camY int
			w, h                     int
			visible                  bool
		}{
			{"right", 30, 2, -8, 0, 0, 0, 0, 0, true},
			{"bottom", 2, 20, 0, -8, 0, 0, 0, 0, true},
			{"corner", 30, 20, -8, -8, 0, 0, 0, 0, true},
			{"subtile camera", 30, 20, -8, -8, 3, 5, 0, 0, true},
			{"distant camera", 80, 60, -8, -8, 400, 320, 0, 0, true},
			{"explicit width", 30, 2, -8, 0, 0, 0, 30, 0, false},
			{"explicit height", 2, 20, 0, -8, 0, 0, 0, 20, false},
			{"explicit rectangle", 30, 20, -8, -8, 0, 0, 31, 21, true},
		} {
			t.Run(fmt.Sprintf("scan=%t/%s", scan, tc.name), func(t *testing.T) {
				c := tileLinkFixture(t)
				c.cart.MapW, c.cart.MapH = 96, 80
				c.cart.Map = make([]uint16, c.cart.MapW*c.cart.MapH)
				c.cart.Map[tc.y*c.cart.MapW+tc.x] = tileLinkMarker
				c.cart.TileLinkInstances = []TileLinkInstance{{X: uint16(tc.x), Y: uint16(tc.y)}}
				c.cart.TileLinkRows = make([]uint32, c.cart.MapH+1)
				for row := tc.y + 1; row < len(c.cart.TileLinkRows); row++ {
					c.cart.TileLinkRows[row] = 1
				}
				c.cart.PoseIndex = []PoseRange{{Count: 1}}
				c.cart.PoseParts = []PosePart{{Tile: 2, X: int16(tc.ox), Y: int16(tc.oy)}}
				c.cart.TileLinkReach = TileLinkReach{Left: int16(-tc.ox), Top: int16(-tc.oy)}
				if scan {
					c.tileLinkScanLayers = 1
				}
				want := make([]byte, ScreenWidth*ScreenHeight)
				if tc.visible {
					for y := tc.y*8 + tc.oy - tc.camY; y < tc.y*8+tc.oy-tc.camY+8; y++ {
						for x := tc.x*8 + tc.ox - tc.camX; x < tc.x*8+tc.ox-tc.camX+8; x++ {
							if x >= 0 && x < ScreenWidth && y >= 0 && y < ScreenHeight {
								want[y*ScreenWidth+x] = tileLinkTopColor
							}
						}
					}
				}
				got := tileLinkFrameForTest(t, c, tc.camX, tc.camY, tc.w, tc.h, 0)
				if !bytes.Equal(got, want) {
					t.Fatal("linked pixels differ from the pose oracle at the requested extent")
				}
			})
		}
	}
}
