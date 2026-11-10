#!/usr/bin/env python3
"""Convert SMB3 ROM graphics and extracted course tiles into native mTile assets."""

import argparse
import hashlib
import json
from pathlib import Path

ROM_HASH = "4377a7f5e6eb50bdd2ac6f249bf1a7085500aca8eb41f38545c3a2731c51a579"
PRG_SIZE = 8192
CHR_START = 16 + 32 * PRG_SIZE
# RGB display rendition of the NES DAC indices. CHR and palette selectors are
# cartridge data. The NES itself does not store RGB triplets.
NES_RGB = bytes.fromhex("""
    545454 001e74 081090 300088 440064 5c0030 540400 3c1800
    202a00 083a00 004000 003c00 00323c 000000 000000 000000
    989698 084cc4 3032ec 5c1ee4 8814b0 a01464 982220 783c00
    545a00 287200 087c00 007628 006678 000000 000000 000000
    eceeec 4c9aec 787cec b062ec e454ec ec58b4 ec6a64 d48820
    a0aa00 74c400 4cd020 38cc6c 38b4cc 3c3c3c 000000 000000
    eceeec a8ccec bcbcec d4b2ec ecaeec ecaed4 ecb4b0 e4c490
    ccd278 b4de78 a8e290 98e2b4 a0d6e4 a0a2a0 000000 000000
""")
# PRG15/16 store four columns of 256 pattern numbers: TL, BL, TR, BR.
TILESETS = {1: (15, 0x0C92), 3: (16, 0x0E12)}
# PRG30 Level_BG_Pages1, indexed by the low five bits of header byte seven.
BG_PAGES = {1: 8, 3: 28, 19: 108}
# Animation advances through these four 2 KiB background pages (PRG30).
ANIM_BG = (96, 98, 100, 102)
QUADRANTS = (0, 2, 1, 3)
EMPTY = (0,) * 64
SPENT_TILE = 0x5F
COIN_REMOVED_TILE = 0x41
PLANT_POSES = {
    "piranha": "piranha",
    "green_fire": "green_fire_plant",
    "venus": "venus_fire_trap",
}
PLAIN_POSES = {
    "idle": 14, "walk_1": 12, "walk_2": 13, "jump": 79,
    "fall": 6, "skid": 48, "duck": 15, "kick": 45,
}
SMALL_POSES = {
    "idle": 62, "walk": 63, "jump": 64, "fall": 64,
    "skid": 65, "kick": 66, "death": 75,
}
RACCOON_POSES = {
    **PLAIN_POSES, "tail_1": 3, "tail_2": 4, "tail_3": 5,
    "fly_1": 54, "fly_2": 55, "fly_3": 56,
    "flutter_1": 8, "flutter_2": 9, "flutter_3": 10,
}


def read_rom(path):
    rom = path.read_bytes()
    if hashlib.sha256(rom).hexdigest() != ROM_HASH:
        raise ValueError("expected the supported NTSC Super Mario Bros. 3 ROM revision")
    return rom


def bank(rom, index):
    start = 16 + index * PRG_SIZE
    return rom[start:start + PRG_SIZE]


def chr_tile(rom, page, index, palette, flip_x=False):
    """Decode one NES 2bpp CHR tile to row-major mTile palette indices."""
    offset = CHR_START + page * 1024 + index * 16
    pattern = rom[offset:offset + 16]
    if len(pattern) != 16:
        raise ValueError(f"invalid CHR tile {page}:{index}")
    bit_order = range(8) if flip_x else range(7, -1, -1)
    return tuple(
        palette[((pattern[y] >> bit) & 1) + 2 * ((pattern[y + 8] >> bit) & 1)]
        for y in range(8) for bit in bit_order
    )


def rgb15(nes_code):
    base = (nes_code & 63) * 3
    return {
        channel: (value * 31 + 127) // 255
        for channel, value in zip("rgb", NES_RGB[base:base + 3])
    }


class Sheet:
    def __init__(self, columns, reserve_text_tiles=False):
        self.columns = columns
        self.reserve_text_tiles = reserve_text_tiles
        self.ids = {EMPTY: 0}

    def add(self, pixels):
        tile = len(self.ids)
        # Mode 0 print() uploads its glyphs over shared sheet slots 480–511.
        if self.reserve_text_tiles and tile >= 480:
            tile += 32
        return self.ids.setdefault(pixels, tile)

    def data(self):
        rows = max(self.ids.values()) // self.columns + 1
        return {
            "width": self.columns * 8,
            "height": rows * 8,
            "sprites": [
                {
                    "id": tile, "x": tile % self.columns * 8, "y": tile // self.columns * 8,
                    "width": 8, "height": 8,
                    "pixels": [pixels[start:start + 8] for start in range(0, 64, 8)],
                    "flags": 0,
                }
                for pixels, tile in self.ids.items()
            ],
        }


def level_art(rom, course, sheet):
    metatile_bank, palette_offset = TILESETS[course["tileset"]]
    level_header = bank(rom, metatile_bank)[course["layout"] - 0xA000:]
    bg_page = BG_PAGES[level_header[7] & 31]
    bg_palette_selector = level_header[5] & 7
    obj_palette_selector = 8 + ((level_header[5] >> 3) & 3)
    palettes = bank(rom, 27)[palette_offset:palette_offset + 192]
    bg_colors = palettes[bg_palette_selector * 16:][:16]
    obj_colors = palettes[obj_palette_selector * 16:][:16]
    # PPU color-zero entries mirror the backdrop rather than the ROM's
    # unprogrammed object-palette placeholders.
    if course.get("palette"):
        measured = course["palette"]
        colors = bg_colors + obj_colors
        if any(colors[i] != measured[i] for i in range(32) if i % 4):
            raise ValueError("ROM level palettes differ from the measured course palettes")
    layout = bank(rom, metatile_bank)[:1024]

    def metatile_pieces(metatile):
        palette_offset = (metatile >> 6) * 4
        colors = [0] + [code + 1 for code in bg_colors[palette_offset + 1:palette_offset + 4]]
        pieces = []
        for quadrant in QUADRANTS:
            pattern = layout[quadrant * 256 + metatile]
            page = bg_page if pattern < 128 else ANIM_BG[0]
            page += (pattern // 64) & 1
            pieces.append(sheet.add(chr_tile(rom, page, pattern % 64, colors)))
        return pieces

    # Mode 0 keeps the mutable shared sheet in EWRAM. Allocate tiles for the
    # placed metatiles and the possible runtime replacements only.
    used_metatiles = set(course["tiles"]) | {SPENT_TILE, COIN_REMOVED_TILE, 0x80}
    if any(not 0 <= tile < 256 for tile in used_metatiles):
        raise ValueError("invalid original metatile ID")
    tile_ids = [metatile_pieces(tile) if tile in used_metatiles else [0] * 4
                for tile in range(256)]
    w, h = course["width"], course["height"]
    if len(course["tiles"]) != w * h:
        raise ValueError("course width and height do not match its metatile count")
    cells = [0] * (w * h * 4)
    for i, metatile in enumerate(course["tiles"]):
        if not 0 <= metatile < 256:
            raise ValueError(f"invalid original metatile ID {metatile}")
        tl, tr, bl, br = tile_ids[metatile]
        start = ((i // w) * w * 2 + i % w) * 2
        cells[start:start + 2] = (tl, tr)
        cells[start + w * 2:start + w * 2 + 2] = (bl, br)
    return cells, tile_ids, bg_colors, obj_colors


def new_pose(name):
    return {"name": name, "parts": []}


def add_chr_piece(rom, sheet, pose, page, pattern, colors, x, y, flip_x=False):
    """Build one 8×16 NES sprite from two native 8×8 mTile pieces."""
    for dy in (0, 8):
        tile = chr_tile(rom, page, (pattern & 62) + dy // 8, colors, flip_x)
        if tile != EMPTY:
            pose["parts"].append({"tile": sheet.add(tile), "x": x, "y": y + dy})


def include_width(pose, width):
    # mTile mirrors around the authored tile envelope. Add transparent anchors
    # when CHR pixels in the edge columns are blank in this particular frame.
    positions = {part["x"] for part in pose["parts"]}
    for x in (0, width - 8):
        if x not in positions:
            pose["parts"].append({"tile": 0, "x": x, "y": 0})


def mario_art(rom, sheet):
    pose_bank = bank(rom, 29)
    palettes = bank(rom, 8)[0x529:0x549]
    forms = (
        ("small", 0, 0, 16, SMALL_POSES),
        ("super", 1, 0, 16, PLAIN_POSES),
        ("fire", 2, 2, 16, {**PLAIN_POSES, "throw_1": 16, "throw_2": 17}),
        ("raccoon", 3, 0, 24, RACCOON_POSES),
    )
    poses = []
    for form, suit, color_set, width, frames in forms:
        colors = [0] + [n + 1 for n in palettes[color_set * 4 + 1:color_set * 4 + 4]]
        for name, frame in frames.items():
            page = pose_bank[0xE98 + suit] + pose_bank[0xE37 + frame]
            address = 0xC4D + pose_bank[0xC00 + frame] * 2
            patterns = pose_bank[address:address + 6]
            pose = new_pose(f"mario_{form}_{name}")
            for i, pattern in enumerate(patterns):
                if pattern >= 64:
                    continue
                x = i % 3 * 8
                y = i // 3 * 16
                if form == "small":
                    y -= 16
                if frame == 45 and i == 2:
                    x, y, pattern = -8, 16, 27
                add_chr_piece(rom, sheet, pose, page, pattern, colors, x, y,
                              patterns[3] == patterns[4] and i % 3 == 1)
            include_width(pose, width)
            poses.append(pose)
    return poses


def enemy_art(rom, sheet, obj_colors):
    def palette(index):
        return [0] + [code + 1 for code in obj_colors[index * 4 + 1:index * 4 + 4]]

    def piece(pattern, x, y, color, flip=False, page=None):
        if page is None:
            page = (0x53, 0x04, 0x4E, 0x4F)[pattern // 64]
        return page, pattern, palette(color), x, y, flip

    def make(name, pieces, width=16):
        pose = new_pose(name)
        for page, pattern, colors, x, y, flip in pieces:
            add_chr_piece(rom, sheet, pose, page, pattern, colors, x, y, flip)
        include_width(pose, width)
        poses.append(pose)

    poses = []
    for i, (left, right) in enumerate(((0xD9, 0xDB), (0xDB, 0xD9)), 1):
        make(f"goomba_{i}", [piece(left, 0, 0, 3), piece(right, 8, 0, 3, True)])
        wing = 0xCD if i == 1 else 0xCF
        make(f"para_goomba_{i}", [
            piece(left, 0, 0, 1), piece(right, 8, 0, 1, True),
            piece(wing, -2, -10, 2, True), piece(wing, 10, -10, 2),
        ])
    make("goomba_flat", [piece(0xEB, 0, 0, 3), piece(0xEB, 8, 0, 3, True)])

    for enemy, color in (("green_koopa", 2), ("red_koopa", 1), ("green_paratroopa", 2)):
        for i, (body, feet, body_y) in enumerate(((0xCB, (0xC7, 0xC9), 10),
                                                    (0xC3, (0xF9, 0xFB), 11)), 1):
            pieces = [
                piece(0xC1, 0, body_y - 16, 3),
                piece(body, 0, body_y, 3), piece(0xC5, 8, body_y, color),
                piece(feet[0], 0, body_y + 16, 3), piece(feet[1], 8, body_y + 16, 3),
            ]
            if enemy == "green_paratroopa":
                pieces.append(piece(0xCD if i == 1 else 0xCF, 8, body_y - 8, 1))
            make(f"{enemy}_{i}", pieces)
        make(f"{enemy}_shell", [piece(0xD1, 0, 0, color), piece(0xD1, 8, 0, color, True)])

    for name, pattern, color in (("mushroom", 0x51, 1), ("one_up", 0x51, 2),
                                 ("goal_mushroom", 0x51, 1), ("goal_flower", 0x53, 2),
                                 ("goal_star", 0x55, 1)):
        make(name, [piece(pattern, 0, 0, color), piece(pattern, 8, 0, color, True)])
    make("leaf", [piece(0x5D, 0, 0, 1), piece(0x5F, 8, 0, 1)])
    for i, pattern in enumerate((0xE1, 0xE5), 1):
        make(f"piranha_{i}", [
            piece(pattern, 0, 0, 2), piece(pattern, 8, 0, 2, True),
            piece(0xE3, 0, 16, 2), piece(0xE3, 8, 16, 2, True),
        ])
    for plant, color in (("green_fire_plant", 2), ("venus_fire_trap", 1)):
        for i, (left, right) in enumerate(((0xF1, 0xF3), (0xF5, 0xF7)), 1):
            make(f"{plant}_{i}", [
                piece(left, 0, 0, color), piece(right, 8, 0, color),
                piece(0xE3, 0, 16, 2), piece(0xE3, 8, 16, 2, True),
            ])
    return poses


def fireball_art(rom, sheet, obj_colors):
    # PRG07 Fireball_Patterns/Attributes use these two sprites in four
    # orientations. CHR bank 4 is the shared gameplay spritesheet.
    poses = []
    colors = [0] + [code + 1 for code in obj_colors[5:8]]
    for i, pattern in enumerate((0x65, 0x67), 1):
        pose = new_pose(f"enemy_fireball_{i}")
        add_chr_piece(rom, sheet, pose, 4, pattern, colors, 0, 0)
        if not any(part["y"] == 8 for part in pose["parts"]):
            pose["parts"].append({"tile": 0, "x": 0, "y": 8})
        poses.append(pose)
    return poses


def height_variants(sheet, poses):
    """Mask the last partial tile row of plants and emerging powerups."""
    source = {pose["name"]: pose for pose in poses}
    pixels_by_id = {tile: pixels for pixels, tile in sheet.ids.items()}
    targets = {name: (name, 16) for name in ("mushroom", "one_up", "leaf")}
    for alias, plant in PLANT_POSES.items():
        targets[alias] = (f"{plant}_1", 32)
        targets[alias + "_open"] = (f"{plant}_2", 32)
    variants = []
    heights = {}
    for alias, (name, full_height) in targets.items():
        full = source[name]
        names = []
        for height in range(4, full_height, 4):
            pose = new_pose(f"{name}_h{height:02}")
            for part in full["parts"]:
                rows = min(8, height - part["y"])
                if rows <= 0:
                    continue
                clipped = part.copy()
                if rows < 8:
                    pixels = pixels_by_id[part["tile"]]
                    visible = pixels[:rows * 8] + (0,) * ((8 - rows) * 8)
                    if visible == EMPTY:
                        continue
                    clipped["tile"] = sheet.add(visible)
                pose["parts"].append(clipped)
            include_width(pose, 16)
            variants.append(pose)
            names.append(pose["name"])
        heights[alias] = names + [name]
    return variants, heights


def animation_workspace(poses, sheet, previous=None):
    """Seed named editor clips while keeping existing page and clip edits."""
    clip_specs = [
        ("mario_small_stride", 10, True, ("mario_small_idle", "mario_small_walk")),
        ("mario_super_stride", 10, True, ("mario_super_walk_1", "mario_super_walk_2",
                                         "mario_super_idle", "mario_super_walk_2")),
        ("mario_raccoon_stride", 10, True, ("mario_raccoon_walk_1", "mario_raccoon_walk_2",
                                           "mario_raccoon_idle", "mario_raccoon_walk_2")),
        ("mario_raccoon_tail", 15, False, ("mario_raccoon_tail_1", "mario_raccoon_tail_3",
                                          "mario_raccoon_tail_1", "mario_raccoon_tail_2",
                                          "mario_raccoon_tail_1")),
        ("mario_raccoon_flight", 20, True, tuple(f"mario_raccoon_fly_{i}" for i in (1, 2, 3))),
        ("mario_raccoon_flutter", 20, True, tuple(f"mario_raccoon_flutter_{i}" for i in (1, 2, 3))),
        ("mario_fire_throw", 15, False, ("mario_fire_throw_1", "mario_fire_throw_2", "mario_fire_idle")),
        ("goal_card_cycle", 15, True, ("goal_mushroom", "goal_flower", "goal_star")),
        ("goomba_cycle", 15, True, ("goomba_1", "goomba_1", "goomba_2", "goomba_2")),
    ]
    for enemy in ("para_goomba", "green_koopa", "red_koopa",
                  "green_paratroopa", *PLANT_POSES.values()):
        clip_specs.append((enemy + "_cycle", 15, True, (enemy + "_1", enemy + "_2")))
    if "map_mario_1" in poses:
        # The NES alternates every eight 60 Hz ticks. A 15 FPS authored clip
        # with two copies of each pose reproduces that interval.
        clip_specs.append(("world_map_mario", 15, True,
                           ("map_mario_1", "map_mario_1", "map_mario_2", "map_mario_2")))

    sheet["height"] = max(sheet["height"], 320)
    default_page = {"name": "OBJ 1", "x": 0, "y": 0, "width": sheet["width"], "height": 320}
    workspace = dict(previous or {"version": 1, "pages": [default_page]})
    if workspace.get("version") != 1:
        raise ValueError("unsupported existing animation workspace version")
    for page in workspace.get("pages") or []:
        if page["x"] + page["width"] > sheet["width"]:
            raise ValueError(f"animation page {page['name']!r} exceeds the SMB3 OBJ sheet width")
        sheet["height"] = max(sheet["height"], page["y"] + page["height"])
    clips = list(workspace.get("clips") or [])
    names = {clip["name"] for clip in clips}
    for name, fps, loop, frames in clip_specs:
        if name not in names:
            clips.append({"name": name, "frames": [poses[frame] for frame in frames],
                          "fps": fps, "loop": loop})
    workspace["clips"] = clips
    return workspace


def title_art(rom, sheet):
    # Title background updates and fade/menu writes from PRG25; render the
    # settled state using the title's CHR banks configured in PRG30.
    program = bank(rom, 25)
    ppu = bytearray(0x4000)
    ppu[0x2000:0x23C0] = bytes([0xFC] * 960)
    for index in (1, *range(8, 35), 6, 7):
        pointer = 0x05DA + 2 * index
        cursor = (program[pointer] | program[pointer + 1] << 8) - 0xC000
        while program[cursor]:
            address = program[cursor] << 8 | program[cursor + 1]
            mode = program[cursor + 2]
            count = mode & 63
            stride = 32 if mode & 0x80 else 1
            cursor += 3
            if mode & 0x40:
                colors = [program[cursor]] * count
                cursor += 1
            else:
                colors = program[cursor:cursor + count]
                cursor += count
            for i, value in enumerate(colors):
                ppu[address + i * stride] = value

    backdrop = ppu[0x3F00] + 1
    rows = []
    for y in range(4, 24):
        row = []
        for x in range(1, 31):
            pattern = ppu[0x2000 + y * 32 + x]
            attr = ppu[0x23C0 + (y // 4) * 8 + x // 4]
            shift = (y & 2) * 2 + (x & 2)
            start = 0x3F00 + ((attr >> shift) & 3) * 4
            colors = [backdrop] + [code + 1 for code in ppu[start + 1:start + 4]]
            row.append(sheet.add(chr_tile(rom, 120 + pattern // 64, pattern % 64, colors)))
        rows.append(row)
    return rows


def world_map_art(rom, shared, objects):
    # PRG12 Tile_Layout_TS0 and the initial screen of W1_Map_Layout. A map
    # screen stores 16×9 metatiles. Use the measured World 1 CHR animation
    # phase for the static scene, then crop one 8px column per side.
    program = bank(rom, 12)
    layout = program[:1024]
    board = program[0x05AA:0x05AA + 16 * 9]
    palettes = bank(rom, 27)[0x0BD2:0x0BD2 + 192]
    pages = (116, 117, 22, 23)

    def tile_art(metatile):
        start = (metatile >> 6) * 4
        colors = [palettes[0] + 1] + [code + 1 for code in palettes[start + 1:start + 4]]
        patterns = [layout[i * 256 + metatile] for i in QUADRANTS]
        return [shared.add(chr_tile(rom, pages[n // 64], n % 64, colors)) for n in patterns]

    def expand(metatiles):
        art = [tile_art(metatile) for metatile in metatiles]
        top = [cell for quad in art for cell in quad[:2]]
        bottom = [cell for quad in art for cell in quad[2:]]
        return top[1:31], bottom[1:31]

    rows = [expand([0x4E] * 16)[1]]
    for start in range(0, len(board), 16):
        rows.extend(expand(board[start:start + 16]))
    rows.append(expand([0x4F] * 16)[0])

    mario_colors = [0] + [code + 1 for code in palettes[129:132]]
    poses = []
    for frame, (left, right, flip) in enumerate(((0x2D, 0x2F, False),
                                                  (0x2F, 0x2D, True)), 1):
        pose = new_pose(f"map_mario_{frame}")
        add_chr_piece(rom, objects, pose, 32, left, mario_colors, 0, 0, flip)
        add_chr_piece(rom, objects, pose, 32, right, mario_colors, 8, 0, flip)
        include_width(pose, 16)
        poses.append(pose)
    return rows, poses, tile_art(0)


def status_bar_art(rom, sheet, backdrop):
    # PRG30 Video_DoStatusBar writes the nametable at $2B00 using a series
    # of literal/repeated PPU update commands. At the original scan split,
    # the game's HUD mapper pages the four 1 KiB CHR banks starting at 92.
    commands = bank(rom, 30)[0x011B:]
    patterns = [0xFC] * (32 * 4)
    cursor = 0
    while commands[cursor]:
        offset = (commands[cursor] << 8 | commands[cursor + 1]) - 0x2B00
        mode = commands[cursor + 2]
        length = mode & 0x3F
        cursor += 3
        if offset >= len(patterns):
            break
        if mode & 0x40:
            patterns[offset:offset + length] = [commands[cursor]] * length
            cursor += 1
        else:
            patterns[offset:offset + length] = commands[cursor:cursor + length]
            cursor += length

    # Initial dynamic fields from the game's digit/initial tables (PRG26).
    patterns[32 + 6] = 0xF1
    patterns[32 + 18:32 + 20] = (0xFE, 0xF0)
    patterns[64 + 2:64 + 4] = (0x74, 0x75)
    patterns[64 + 6] = 0xF4
    patterns[64 + 18:64 + 21] = (0xF3, 0xF0, 0xF0)

    # Map 0 is transparent; use an opaque backdrop index in this overlay.
    colors = [backdrop + 1, 16, 49, 61]

    def tile_id(pattern):
        return sheet.add(chr_tile(rom, 92 + pattern // 64, pattern % 64, colors))

    rows = [[tile_id(pattern) for pattern in patterns[start + 1:start + 31]]
            for start in range(0, len(patterns), 32)]
    return rows, {
        "layer": 1, "width": 30, "height": 4,
        "digit_art": [tile_id(pattern) for pattern in range(0xF0, 0xFA)],
        "blank_art": tile_id(0xFE),
        "world": [5, 1], "lives": [4, 2, 2], "score": [7, 2, 7],
        "coins": [17, 1, 2], "timer": [16, 2, 3],
    }


def write_json(path, data):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data, separators=(",", ":")) + "\n", encoding="utf-8")


def extract(rom, courses, output, include_status_bar=False, include_title=False,
            include_world_map=False, existing_animation=None):
    shared = Sheet(30, reserve_text_tiles=True)
    objects = Sheet(64)
    width = courses[0]["width"] * 2
    height = 0
    course_cells = []
    art = []
    graphics = {"courses": art}
    if include_status_bar:
        status_rows, graphics["status_bar"] = status_bar_art(rom, shared, courses[0]["palette"][0])
    backdrop = None
    object_palette = None
    for course in courses:
        if course["width"] * 2 != width:
            raise ValueError("stacked courses must have the same width")
        cells, metatiles, bg_colors, obj_colors = level_art(rom, course, shared)
        if backdrop is None:
            backdrop, object_palette = bg_colors[0], obj_colors
        if backdrop != bg_colors[0] or object_palette != obj_colors:
            raise ValueError("stacked courses must share the backdrop and object palette")
        art.append({
            "map_row": height, "metatile_art": metatiles,
            "layout": course["layout"],
        })
        course_cells.extend(cells)
        height += course["height"] * 2

    poses = mario_art(rom, objects) + enemy_art(rom, objects, object_palette)

    def append_scene(rows):
        nonlocal height
        scene_row = height
        for row in rows:
            course_cells.extend(row)
            course_cells.extend([0] * (width - len(row)))
        height += len(rows)
        return scene_row

    if include_title:
        title = title_art(rom, shared)
        graphics["title_row"] = append_scene(title)
    if include_world_map:
        world_map, map_poses, completed_tile = world_map_art(rom, shared, objects)
        graphics["world_map_row"] = append_scene(world_map)
        graphics["world_map"] = {
            "start": [24, 40], "junction": [56, 40],
            "level_1": [56, 8], "level_2": [120, 8],
            "completed_tile": completed_tile,
        }
        poses.extend(map_poses)
    poses.extend(fireball_art(rom, objects, object_palette))
    variants, pose_heights = height_variants(objects, poses)
    poses.extend(variants)
    graphics["poses"] = {pose["name"]: i for i, pose in enumerate(poses)}
    graphics["pose_heights"] = {
        name: [graphics["poses"][frame] for frame in frames]
        for name, frames in pose_heights.items()
    }
    map_data = {"width": width, "height": height}
    if include_status_bar:
        status_cells = [0] * (width * height)
        for y, row in enumerate(status_rows):
            status_cells[y * width:y * width + len(row)] = row
        empty = [0] * (width * height)
        map_data.update(version=2, layers=[
            {"id": 0, "cells": course_cells}, {"id": 1, "cells": status_cells},
            {"id": 2, "cells": empty}, {"id": 3, "cells": empty},
        ])
    else:
        map_data["cells"] = course_cells
    write_json(output / "spritesheet.json", shared.data())
    object_sheet = objects.data()
    workspace = animation_workspace(graphics["poses"], object_sheet, existing_animation)
    write_json(output / "objsheet.json", object_sheet)
    write_json(output / "poses.json", {"version": 1, "poses": poses})
    write_json(output / "animation.json", workspace)
    write_json(output / "map.json", map_data)
    write_json(output / "tile-ids.json", graphics)
    palette = [rgb15(backdrop)] + [rgb15(code) for code in range(64)]
    palette += [palette[0]] * (256 - len(palette))
    write_json(output / "palettes" / "smb3.json", {"name": "smb3", "colors": palette})


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("rom", type=Path)
    parser.add_argument("course", type=Path, help="JSON from the native course extractor")
    parser.add_argument("output", type=Path, help="mTile project assets directory")
    parser.add_argument("--next-course", type=Path, help="optional course stacked below the first")
    parser.add_argument("--status-bar", action="store_true", help="include the original HUD on BG1")
    parser.add_argument("--title", action="store_true", help="append the cropped original title on BG0")
    parser.add_argument("--world-map", action="store_true", help="append the initial World 1 map on BG0")
    args = parser.parse_args()
    try:
        course_paths = [args.course]
        if args.next_course:
            course_paths.append(args.next_course)
        courses = [json.loads(path.read_text(encoding="utf-8")) for path in course_paths]
        extract(read_rom(args.rom), courses, args.output, args.status_bar, args.title, args.world_map)
    except (OSError, ValueError, KeyError) as error:
        parser.exit(1, f"extract assets: {error}\n")


if __name__ == "__main__":
    main()
