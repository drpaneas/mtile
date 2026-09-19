#!/usr/bin/env python3
"""Generate deterministic fixtures for the Discord hardware trials."""
import argparse
import json
from pathlib import Path


def write(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, indent=2) + '\n')


def fourbpp(root):
    directory = root / 'fourbpp'
    write(directory / 'project.json', {'activePalette': 'aurora', 'renderer': 'mode0'})
    sprites = []
    for tile in range(256):
        pixels = [[0 if (x+y+tile) % 5 == 0 else 1+(x+2*y+tile) % 15 for x in range(8)] for y in range(8)]
        if tile == 0:
            pixels = [[0]*8 for _ in range(8)]
        sprites.append(dict(id=tile, x=(tile % 16)*8, y=(tile//16)*8,
                            width=8, height=8, pixels=pixels, flags=0))
    write(directory / 'assets/spritesheet.json', dict(width=128, height=128, sprites=sprites))
    write(directory / 'assets/map.json', dict(width=30, height=20, cells=[1+i % 255 for i in range(600)]))
    (directory / 'game.lua').write_text('''local tick=0
function _update()
 tick=tick+1
 pal(1,0)
 pal(3,200,1)
end
function _draw()
 cls(21)
 map(0,0,0,0,0,30,20)
 spr(1,8,8,1,1,false,false)
 spr(2,24,8,1,1,true,false)
 spr(3,40,8,1,1,false,true)
 spr(4,56,8,1,1,true,true)
end
''')


def shared(root):
    for kind in ('sprites', 'shared', 'phases'):
        directory = root / ('animation-' + kind)
        write(directory / 'project.json', {'activePalette': 'aurora', 'renderer': 'mode0'})
        sprites = []
        for tile in range(3):
            pixels = [[0 if tile == 0 else (tile+1 if y < 2 else 1) for _ in range(8)] for y in range(8)]
            sprites.append(dict(id=tile, x=tile*8, y=0, width=8, height=8, pixels=pixels, flags=0))
        write(directory / 'assets/spritesheet.json', dict(width=24, height=8, sprites=sprites))
        cells = [0]*600
        for y in range(8):
            for x in range(16):
                cells[y*2*30+x] = 1
        write(directory / 'assets/map.json', dict(width=30, height=20, cells=cells))
        source = '''local tick=0
local id=1
local opposite=2
function _update()
 tick=tick+1
 id=1
 opposite=2
 if tick%2==1 then id=2 opposite=1 end
'''
        if kind == 'shared':
            source += ' local color=2\n if tick%2==1 then color=3 end\n'
            for y in range(2):
                for x in range(8):
                    source += f' sset({8+x},{y},color)\n'
        source += 'end\nfunction _draw()\n cls(21)\n'
        if kind == 'shared':
            source += ' map(0,0,0,0,0,30,20)\n'
        else:
            for y in range(8):
                for x in range(16):
                    tile = 'opposite' if kind == 'phases' and x >= 8 else 'id'
                    source += f' spr({tile},{x*8},{y*16})\n'
        (directory / 'game.lua').write_text(source+'end\n')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--out', type=Path, required=True)
    args = parser.parse_args()
    fourbpp(args.out)
    shared(args.out)
    print(args.out.resolve())
