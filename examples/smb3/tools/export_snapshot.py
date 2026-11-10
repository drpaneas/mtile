"""Extract authored tile and enemy data from local offline SMB3 oracle dumps."""
import argparse
import json
from pathlib import Path

SCREEN_COLUMNS = 16
SCREEN_ROWS = 27
BYTES_PER_SCREEN = SCREEN_COLUMNS * SCREEN_ROWS

OBJECT_NAMES = {
    0x41: 'goal_card', 0x6c: 'green_koopa', 0x6d: 'red_koopa',
    0x6e: 'green_paratroopa', 0x72: 'goomba', 0x73: 'para_goomba',
    0x74: 'micro_para_goomba', 0xa0: 'piranha', 0xa4: 'green_fire_plant',
    0xa6: 'venus_fire_trap', 0xc1: 'goomba_pipe_right',
}

REWARDS = {
    0x03: 'note_invisible', 0x2e: 'note', 0x2f: 'flower', 0x30: 'leaf', 0x31: 'star',
    0x44: 'coin', 0x45: 'one_up', 0x46: 'coin_heaven', 0x60: 'flower',
    0x61: 'leaf', 0x62: 'star', 0x63: 'coin', 0x64: 'coin_or_star',
    0x65: 'coin', 0x68: 'flower', 0x69: 'leaf', 0x6a: 'star', 0x6b: 'coin',
    0x6c: 'coin_or_star', 0x6d: 'ten_coins', 0x6e: 'one_up', 0x6f: 'vine',
    0x70: 'p_switch', 0x71: 'coin_heaven', 0x73: 'flower', 0x74: 'leaf', 0x75: 'star',
}
BUMPABLE = {0x03, *range(0x2e, 0x32), *range(0x44, 0x47), *range(0x60, 0x66), *range(0x67, 0x76)}


def export_level(ram, work, palette, banks):
    if (len(ram), len(work), len(palette)) != (2048, 8192, 32):
        raise ValueError('Expected complete 2 KiB RAM, 8 KiB cartridge RAM and 32 palette bytes')
    layout = int.from_bytes(work[0x1eb9:0x1ebb], 'little')
    if ram[0x728] != 3 or layout not in {0xbb82, 0xaf2a, 0xbc93, 0xba49}:
        raise ValueError(f'Expected a World 1-1 or 1-2 area, observed layout ${layout:04x}')
    screens = ram[0x22] + 1
    width = screens * SCREEN_COLUMNS
    tiles = [work[screen * BYTES_PER_SCREEN + row * SCREEN_COLUMNS + col]
             for row in range(SCREEN_ROWS) for screen in range(screens) for col in range(SCREEN_COLUMNS)]
    attributes = list(work[0x1e94:0x1e9c])
    flags = []
    names = {}
    for tile in range(256):
        group = tile // 64
        flag = int(tile >= attributes[group + 4])
        if tile >= attributes[group] and flag == 0:
            flag = 2
        if tile in BUMPABLE:
            flag |= 4
        if tile == 0x40:
            flag |= 8
        flags.append(flag)
        name = None
        if tile == 0x40:
            name = 'coin'
        elif 0x60 <= tile <= 0x65:
            name = 'question'
        elif 0x67 <= tile <= 0x70:
            name = 'brick'
        elif 0x03 == tile or 0x2e <= tile <= 0x31 or tile == 0x71:
            name = 'note'
        elif 0x44 <= tile <= 0x46:
            name = 'invisible'
        elif 0x72 <= tile <= 0x75:
            name = 'bouncing_wood'
        elif tile == 0x5f:
            name = 'spent_block'
        elif layout == 0xbb82 and 0xad <= tile <= 0xbb:
            name = 'pipe'
        if name and tile in tiles:
            names[tile] = name
    objects = []
    pos = 0x1b41  # The first byte is a list header. Then ID, column, row triples.
    while work[pos] != 255 and pos + 2 < 0x1bd0:
        kind, col, row = work[pos:pos + 3]
        objects.append({'id': kind, 'x': col * 16, 'y': row * 16, 'column': col, 'row': row,
                        'name': OBJECT_NAMES.get(kind, f'object_{kind:02x}')})
        pos += 3
    if pos + 2 >= 0x1bd0:
        raise ValueError('Unterminated level object list')
    return {'width': width, 'height': SCREEN_ROWS, 'tiles': tiles,
            'tileset': ram[0x70a], 'layout': layout, 'object_list_header': work[0x1b40],
            'spawn_x': ram[0x90] + 256 * ram[0x75], 'spawn_y': ram[0xa2] + 256 * ram[0x87],
            'initial_scroll_y': ram[0xfc], 'attributes': attributes, 'flags': flags,
            'tile_names': names, 'rewards': {k: v for k, v in REWARDS.items() if k in tiles},
            'junction_raw_y': list(work[0x1f54:0x1f64]), 'junction_raw_x': list(work[0x1f64:0x1f74]),
            'objects': objects, 'palette': list(palette), 'chr_1k': list(map(int, banks))}


def export(snapshot):
    log = (snapshot / 'raw.log').read_text().splitlines()
    banks = next(line for line in log if line.startswith('CHR_1K ')).split()[1:]
    return export_level((snapshot / 'iram.bin').read_bytes(), (snapshot / 'wram.bin').read_bytes(),
                        (snapshot / 'palette.bin').read_bytes(), banks)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('snapshot', type=Path)
    parser.add_argument('output', type=Path)
    args = parser.parse_args()
    args.output.write_text(json.dumps(export(args.snapshot), separators=(',', ':')) + '\n')


if __name__ == '__main__':
    main()
