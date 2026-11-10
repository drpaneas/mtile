#!/usr/bin/env python3
"""Generate compact native Lua course data from local ROM extraction."""

import argparse
import json
from pathlib import Path

from extract_sound import song_game_updates

from extract_assets import COIN_REMOVED_TILE, SPENT_TILE


def lua(value, indent=""):
    if isinstance(value, int):
        return str(value)
    if isinstance(value, str):
        return json.dumps(value)
    if isinstance(value, list):
        return "{" + ", ".join(lua(item, indent) for item in value) + "}"
    raise TypeError(type(value))


def sparse(values):
    return "{" + ", ".join(f"[{key}] = {lua(value)}" for key, value in values.items()) + "}"


def write_courses(levels, graphics, output):
    art_tables = [entry["metatile_art"] for entry in graphics["courses"]]
    poses = graphics["poses"]
    lines = ["-- Generated from the local NES cartridge.", "courses = {"]
    row = 0
    for level, art in zip(levels, art_tables, strict=True):
        width = level["width"]
        height = level["height"]
        tiles = level["tiles"]
        if len(tiles) != width * height or len(level["flags"]) != 256:
            raise ValueError("unexpected course dimensions or attribute table")
        art = dict(enumerate(art))
        replacements = {}
        actions = {}
        for key, name in level["tile_names"].items():
            tile = int(key)
            if tile not in tiles:
                continue
            reward = level["rewards"].get(key)
            if name == "coin":
                replacements[tile + 1] = COIN_REMOVED_TILE
            if name == "brick" and reward is None:
                replacements[tile + 1] = 0x80
                actions[tile + 1] = 1
            if reward == "coin":
                replacements[tile + 1] = SPENT_TILE
                actions[tile + 1] = 2
            if reward == "leaf":
                replacements[tile + 1] = SPENT_TILE
                actions[tile + 1] = 3
            if reward == "one_up":
                replacements[tile + 1] = SPENT_TILE
                actions[tile + 1] = 4
        art.setdefault(0, [0] * 4)
        edited_tiles = {key - 1 for key in replacements} | set(replacements.values())
        art = {key: value for key, value in art.items() if key in edited_tiles}
        # TILEA_BLOCKEMPTY is a solid metal plate in Plains and Hills.
        if SPENT_TILE in replacements.values() and (
            not any(art.get(SPENT_TILE, ())) or level["flags"][SPENT_TILE] & 1 == 0
        ):
            raise ValueError("spent block requires visible art and a solid floor flag")
        objects = [[obj["id"], obj["x"], obj["y"]] for obj in level["objects"]]
        slopes = {
            int(key) + 1: level["slope_heights"][shape]
            for key, shape in level.get("slopes", {}).items() if shape in (1, 2, 12, 13, 14, 15)
        }
        lines.extend([
            "  {",
            f"    columns = {width}, height = {height}, map_row = {row},",
            f"    spawn_x = {level['spawn_x']}, spawn_y = {level['spawn_y']}, timer = {level.get('timer', 300)},",
            f"    flags = {lua(level['flags'])},",
            f"    objects = {lua(objects)},",
            f"    art = {sparse({key + 1: value for key, value in art.items()})},",
            f"    actions = {sparse(actions)}, replacements = {sparse(replacements)},",
            f"    slopes = {sparse(slopes)},",
            "    rows = {",
        ])
        for start in range(0, len(tiles), width):
            lines.append(f'      "{bytes(tiles[start:start + width]).hex()}",')
        lines.extend(["    },", "  },"])
        row += height * 2
    lines.append("}")
    lines.append(f"art_scene_fade = {lua(graphics['fade_maps'])}")
    lines.append(f"art_death_jingle_updates = {song_game_updates(4)}")
    lines.append(f"art_title_row = {graphics['title_row']}")
    lines.append(f"art_world_map_row = {graphics['world_map_row']}")
    board = graphics["world_map"]
    positions = [board["start"], board["junction"], board["level_1"],
                 [88, board["level_1"][1]], board["level_2"]]
    lines.append(f"art_map_positions = {lua(positions)}")
    lines.append(f"art_map_clear = {lua(board['completed_tile'])}")
    lines.append(f"art_map_mario = {lua([poses['map_mario_1'], poses['map_mario_2']])}")
    status = graphics["status_bar"]
    for field in ("digit_art", "lives", "score", "coins", "timer"):
        lines.append(f"art_hud_{field} = {lua(status[field])}")
    families = {
        "small": ["idle", "idle", "walk", "walk", "jump", "fall", "skid", "idle", "death"],
        "super": ["idle", "walk_1", "walk_2", "walk_2", "jump", "fall", "skid", "duck", "idle"],
        "raccoon": ["idle", "walk_1", "walk_2", "walk_2", "jump", "fall", "skid", "duck", "idle"],
    }
    for family, frames in families.items():
        ids = [poses[f"mario_{family}_{frame}"] for frame in frames]
        lines.append(f"art_{family} = {lua(ids)}")
    for action in ("tail", "fly", "flutter"):
        ids = [poses[f"mario_raccoon_{action}_{frame}"] for frame in (1, 2, 3)]
        lines.append(f"art_{action} = {lua(ids)}")
    names = {
        "goomba_1": "goomba_1",
        "goomba_2": "goomba_2",
        "goomba_flat": "goomba_flat",
        "para_goomba_1": "para_goomba_1",
        "para_goomba_2": "para_goomba_2",
        "koopa_1": "green_koopa_1",
        "koopa_2": "green_koopa_2",
        "koopa_shell": "green_koopa_shell",
        "red_koopa_1": "red_koopa_1",
        "red_koopa_2": "red_koopa_2",
        "red_koopa_shell": "red_koopa_shell",
        "para_koopa_1": "green_paratroopa_1",
        "para_koopa_2": "green_paratroopa_2",
        "goal": "goal_mushroom",
        "goal_flower": "goal_flower",
        "goal_star": "goal_star",
        "mushroom": "mushroom",
        "one_up": "one_up",
        "leaf": "leaf",
        "piranha": "piranha_1",
        "piranha_open": "piranha_2",
        "green_fire": "green_fire_plant_1",
        "green_fire_open": "green_fire_plant_2",
        "venus": "venus_fire_trap_1",
        "venus_open": "venus_fire_trap_2",
        "enemy_fireball": "enemy_fireball_1",
        "enemy_fireball_alt": "enemy_fireball_2",
    }
    for variable, pose in names.items():
        lines.append(f"art_{variable} = {poses[pose]}")
    for actor, heights in graphics["pose_heights"].items():
        lines.append(f"art_{actor}_heights = {lua(heights)}")
    output.write_text("\n".join(lines) + "\n", encoding="ascii")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("graphics", type=Path, help="metatile and pose indexes from the graphics exporter")
    parser.add_argument("output", type=Path)
    parser.add_argument("levels", type=Path, nargs="+")
    args = parser.parse_args()
    levels = [json.loads(path.read_text()) for path in args.levels]
    graphics = json.loads(args.graphics.read_text())
    write_courses(levels, graphics, args.output)


if __name__ == "__main__":
    main()
