"""Export both World 1 courses and their pipe areas from a local SMB3 ROM."""
import argparse
import json
from pathlib import Path

from export_snapshot import export_level
from rom_layout import COURSES, check_rom, decode_layout, prg_bank

PALETTE_BASE = {1: 0x0c92, 3: 0x0e12}
BG_PAGES = (0x1762, 0x1779)
SLOPE_LUT = 0x0087
SLOPE_SHAPES = 21


def palette_and_chr(rom, name):
    bank, layout, _, tileset, _ = COURSES[name]
    header = prg_bank(rom, bank)[layout - 0xa000:layout - 0xa000 + 9]
    palette = prg_bank(rom, 27)
    base = PALETTE_BASE[tileset]
    background = base + (header[5] & 7) * 16
    sprites = base + (8 + ((header[5] >> 3) & 3)) * 16
    colors = list(palette[background:background + 16] + palette[sprites:sprites + 16])
    for i in range(16, 32, 4):
        colors[i] = colors[0]
    bank30 = prg_bank(rom, 30)
    chr_index = header[7] & 31
    chr1, chr2 = (bank30[offset + chr_index] for offset in BG_PAGES)
    return colors, [chr1, chr1 + 1, chr2, chr2 + 1], header


def add_slopes(data, rom):
    bank = prg_bank(rom, 0)
    pointers = [int.from_bytes(bank[2 * i:2 * i + 2], 'little') - 0xc000 for i in range(4)]
    slopes = {}
    for tile in set(data['tiles']):
        quadrant = tile // 64
        offset = tile - data['attributes'][quadrant]
        if offset >= 0:
            shape = bank[pointers[quadrant] + offset]
            if shape >= SLOPE_SHAPES:
                raise ValueError(f'Unexpected slope shape {shape} for tile {tile}')
            slopes[tile] = shape
    slope_tiles = bank[SLOPE_LUT:SLOPE_LUT + 16 * SLOPE_SHAPES]
    data['slopes'] = slopes
    data['slope_heights'] = [[v & 15 for v in slope_tiles[i:i + 16]] for i in range(0, len(slope_tiles), 16)]
    data['slope_ceilings'] = [[v >> 4 for v in slope_tiles[i:i + 16]] for i in range(0, len(slope_tiles), 16)]
    speed_start = SLOPE_LUT + SLOPE_SHAPES * 16
    data['slope_speed_effect'] = [v if v < 128 else v - 256 for v in bank[speed_start:speed_start + SLOPE_SHAPES]]


def extract_course(rom, name):
    ram, work = decode_layout(rom, name)
    palette, chr_banks, header = palette_and_chr(rom, name)
    data = export_level(ram, work, palette, chr_banks)
    data['header'] = list(header)
    data['timer'] = (300, 400, 200, 0)[header[8] >> 6]
    data['alternate_layout'] = int.from_bytes(header[:2], 'little')
    data['alternate_objects'] = int.from_bytes(header[2:4], 'little')
    data['alternate_tileset'] = header[6] & 15
    if data['tileset'] == 3:
        add_slopes(data, rom)
    return data


def extract_courses(rom):
    check_rom(rom)
    return {name: extract_course(rom, name) for name in COURSES}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('rom', type=Path)
    parser.add_argument('directory', type=Path)
    args = parser.parse_args()
    try:
        data = extract_courses(args.rom.read_bytes())
        args.directory.mkdir(parents=True, exist_ok=True)
        for name, course in data.items():
            (args.directory / f'{name}.json').write_text(json.dumps(course, separators=(',', ':')) + '\n')
    except (OSError, ValueError) as exc:
        parser.exit(1, f'{exc}\n')


if __name__ == '__main__':
    main()
