package runtime

import "testing"

func rectangularPose(width, height int) []PosePart {
	parts := make([]PosePart, 0, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			parts = append(parts, PosePart{Tile: uint16(len(parts)), X: int16(x * 8), Y: int16(y * 8)})
		}
	}
	return parts
}

func TestPoseOBJPlanPacksLargeFighter(t *testing.T) {
	objects, tiles := BuildPoseOBJPlan(rectangularPose(8, 8))
	if len(objects) != 1 || objects[0].Width != 64 || objects[0].Height != 64 || len(tiles) != 64 {
		t.Fatalf("64x64 fighter plan: %d objects, %d tiles, objects=%+v", len(objects), len(tiles), objects)
	}
	for i, tile := range tiles {
		if int(tile) != i {
			t.Fatalf("tile %d = %d, want row-major %d", i, tile, i)
		}
	}
}

func TestPoseOBJPlanPreservesOverlappingPainterOrder(t *testing.T) {
	parts := []PosePart{{Tile: 5}, {Tile: 2, X: 4}, {Tile: 9, X: 8}}
	objects, tiles := BuildPoseOBJPlan(parts)
	if len(objects) != len(parts) {
		t.Fatalf("overlapping pose packed into %d objects", len(objects))
	}
	for i, object := range objects {
		if object.Width != 8 || object.Height != 8 || object.X != parts[i].X || tiles[object.TileOffset] != parts[i].Tile {
			t.Fatalf("object %d lost painter order: %+v", i, object)
		}
	}
}

func TestPoseOBJPlanKeepsAuthoredFlips(t *testing.T) {
	parts := rectangularPose(2, 2)
	parts[0].Flags = PoseFlipX | PoseFlipY
	objects, tiles := BuildPoseOBJPlan(parts)
	if objects[0].Width != 8 || objects[0].Height != 8 || objects[0].Flags != parts[0].Flags || tiles[objects[0].TileOffset] != 0 {
		t.Fatalf("flipped cell was repacked: %+v", objects[0])
	}
}

func TestPoseOBJPlanLargeImport(t *testing.T) {
	parts := rectangularPose(256, 256)[:65535]
	objects, tiles := BuildPoseOBJPlan(parts)
	if len(tiles) != len(parts) || len(objects) >= 1100 {
		t.Fatalf("large import plan has %d objects and %d tiles", len(objects), len(tiles))
	}
}

func BenchmarkPoseOBJPlanLargeImport(b *testing.B) {
	parts := rectangularPose(256, 256)[:65535]
	for b.Loop() {
		BuildPoseOBJPlan(parts)
	}
}
