#!/usr/bin/env python3
"""Compare authored effect pixels against the real Mode 4 rasterizer."""

import os
from pathlib import Path
import subprocess
import tempfile


PROJECT = Path(__file__).resolve().parents[1]
ROOT = PROJECT.parents[1]
GO = r'''
package main

import (
    "encoding/json"
    "fmt"
    "os"
    "reflect"

    mt "github.com/drpaneas/mtile/runtime"
)

type sprite struct {
    ID int `json:"id"`
    X int `json:"x"`
    Y int `json:"y"`
    Width int `json:"width"`
    Height int `json:"height"`
    Pixels [][]byte `json:"pixels"`
    Flags int `json:"flags"`
}

func sheet(path string) map[int]sprite {
    data, err := os.ReadFile(path)
    if err != nil { panic(err) }
    var raw struct { Sprites []sprite `json:"sprites"` }
    if err = json.Unmarshal(data, &raw); err != nil { panic(err) }
    out := make(map[int]sprite)
    for _, s := range raw.Sprites { out[s.ID] = s }
    return out
}

func compare(c *mt.Context, actual sprite) {
    for y := 0; y < 8; y++ {
        for x := 0; x < 8; x++ {
            want := c.Pget(x, y)
            if actual.Pixels[y][x] != want {
                panic(fmt.Sprintf("sprite %d (%d,%d): got %d want Mode4 %d",
                    actual.ID, x, y, actual.Pixels[y][x], want))
            }
        }
    }
}

func main() {
    actual, original := sheet(os.Args[1]), sheet(os.Args[2])
    originalAt := map[int]int{
        1: 1, 2: 2, 3: 3, 4: 4, 5: 5, 6: 6, 7: 7,
        31: 22, 41: 23, 43: 19, 44: 20, 45: 21,
        60: 26, 61: 27, 62: 28, 63: 29, 64: 30, 65: 31, 66: 32, 67: 33,
        71: 24, 91: 34, 92: 35, 93: 36, 94: 37, 95: 38, 96: 39,
        101: 25, 103: 40, 104: 41, 105: 42, 122: 43, 128: 44,
    }
    keptOriginal := 0
    for oldID, newID := range originalAt {
        src, ok := original[oldID]
        if !ok {
            panic(fmt.Sprintf("original sprite %d missing from Celeste sheet", oldID))
        }
        got, ok := actual[newID]
        if !ok {
            panic(fmt.Sprintf("packed sprite %d missing", newID))
        }
        if !reflect.DeepEqual(got.Pixels, src.Pixels) || got.Flags != src.Flags {
            panic(fmt.Sprintf("original sprite %d pixels/flags changed at packed id %d", oldID, newID))
        }
        keptOriginal++
    }
    for _, id := range []int{240, 241, 242, 243, 244, 245, 246, 247, 248, 249, 250, 251, 252, 253, 263, 264, 265, 266} {
        if _, exists := actual[id]; exists {
            panic(fmt.Sprintf("unused baked sprite %d still present", id))
        }
    }
    for _, pair := range []struct{ id, color, radius int }{
        {15, 158, 1}, {16, 158, 2}, {17, 214, 1}, {18, 214, 2},
    } {
        var c mt.Context
        c.Cls(0)
        c.Circfill(2, 2, pair.radius, pair.color)
        compare(&c, actual[pair.id])
    }
    for frame := 1; frame <= 7; frame++ {
        baked := actual[7+frame]
        for y, row := range original[frame].Pixels {
            for x, source := range row {
                want := source
                if source == 158 {
                    want = 214
                }
                if baked.Pixels[y][x] != want {
                    panic("baked player palette differs")
                }
            }
        }
    }
    for variant, color := range []int{11, 14} {
        for size := 1; size <= 2; size++ {
            var c mt.Context
            c.Cls(0)
            c.Rectfill(0, 0, size-1, size-1, color)
            compare(&c, actual[46 + variant * 2 + size - 1])
        }
        for shape, dimensions := range [][2]int{{1, 2}, {2, 1}} {
            var c mt.Context
            c.Cls(0)
            c.Rectfill(0, 0, dimensions[0]-1, dimensions[1]-1, color)
            compare(&c, actual[50 + variant * 2 + shape])
        }
    }
    for variant, color := range []int{249, 62} {
        for width := 1; width <= 5; width++ {
            for height := 1; height <= 5; height++ {
                var c mt.Context
                c.Cls(0)
                c.Rectfill(0, 0, width-1, height-1, color)
                compare(&c, actual[54 + variant * 25 + (width - 1) * 5 + height - 1])
            }
        }
    }
    for id := 323; id < 416; id++ {
        if _, exists := actual[id]; exists { panic(fmt.Sprintf("retired UI sprite %d still present", id)) }
    }
    fmt.Printf("PASS: %d original cells unchanged; authored effect cells match Mode4 masks.\n", keptOriginal)
}
'''


def main():
    with tempfile.TemporaryDirectory(prefix="mtile-mode0-art-") as directory:
        directory = Path(directory)
        (directory / "go.mod").write_text(
            "module verify-mode0-art\n\ngo 1.27.1\n\n"
            "require github.com/drpaneas/mtile/runtime v0.0.0\n"
            f"replace github.com/drpaneas/mtile/runtime => {ROOT / 'runtime'}\n")
        (directory / "main.go").write_text(GO)
        env = os.environ.copy()
        env.setdefault("GOCACHE", "/private/tmp/mtile-celeste-go-cache")
        subprocess.run(["go", "run", ".", str(PROJECT / "assets" / "spritesheet.json"),
                        str(ROOT / "examples" / "celeste" / "assets" / "spritesheet.json")],
                       cwd=directory, env=env, check=True)


if __name__ == "__main__":
    main()
