#!/usr/bin/env python3
"""Replay the native mTile SMB3 cartridge through World 1-1 and into 1-2."""

import argparse
import hashlib
import json
from pathlib import Path
import re
import struct
import subprocess
import zlib


WIDTH, HEIGHT = 240, 160
WATCHES = (
    'stage', 'course', 'tick', 'player_x', 'player_y', 'player_vx',
    'player_vy', 'lives', 'card', 'cam_x', 'cam_y', 'coins',
)


def read_game_layout(elf, nm, dwarfdump):
    symbols = subprocess.check_output([nm, '-S', str(elf)], text=True)
    symbol = re.search(r'^([0-9a-f]+) [0-9a-f]+ \w main\.game$', symbols, re.M)
    if symbol is None:
        raise ValueError('main.game was not found in the matching TinyGo ELF')
    names = [flag for name in WATCHES for flag in ('--name', name)]
    debug = subprocess.check_output(
        [dwarfdump, *names, str(elf)], text=True, stderr=subprocess.DEVNULL,
    )
    members = {}
    for entry in re.split(r'\n0x[0-9a-f]+: ', debug):
        if not entry.startswith('DW_TAG_member'):
            continue
        name = re.search(r'DW_AT_name\s+\("([a-z_]+)"\)', entry)
        offset = re.search(r'DW_AT_data_member_location\s+\((0x[0-9a-f]+)\)', entry)
        if name and offset and name[1] in WATCHES and '/runtime.Number' in entry:
            members[name[1]] = int(offset[1], 16)
    if set(members) != set(WATCHES):
        raise ValueError(f'missing Game fields in ELF: {sorted(set(WATCHES) - set(members))}')
    return int(symbol[1], 16), members


def png_chunk(kind, data):
    return struct.pack('>I', len(data)) + kind + data + struct.pack('>I', zlib.crc32(kind + data))


def write_png(path, rgba):
    stride = WIDTH * 4
    rows = b''.join(b'\0' + rgba[start:start + stride] for start in range(0, len(rgba), stride))
    header = struct.pack('>2I5B', WIDTH, HEIGHT, 8, 6, 0, 0, 0)
    path.write_bytes(b'\x89PNG\r\n\x1a\n' + png_chunk(b'IHDR', header)
                     + png_chunk(b'IDAT', zlib.compress(rows)) + png_chunk(b'IEND', b''))


def mario_sprite_pixels(rgba, state, powered=False):
    """Count Mario's dark red pixels in the expected small or super sprite."""
    left = state['player_x'] - state['cam_x']
    top = state['player_y'] - state['cam_y'] + (0 if powered else 16)
    red = bytes((148, 33, 33))  # Imported Mario palette after GBA RGB conversion.
    count = 0
    for y in range(max(0, top), min(HEIGHT, top + (32 if powered else 16))):
        for x in range(max(0, left), min(WIDTH, left + 16)):
            pixel = (y * WIDTH + x) * 4
            count += rgba[pixel:pixel + 3] == red
    if count < 8:
        raise ValueError(f"Mario's {'super' if powered else 'small'} sprite is missing: {state}")
    return count


class Preview:
    def __init__(self, process, address, members):
        self.process = process
        self.address = address
        self.members = members
        self.read_length = max(members.values()) + 4
        if self.read(16)[:4] != b'GLUP':
            raise ValueError('invalid mTile emulator handshake')

    def read(self, size):
        data = self.process.stdout.read(size)
        if len(data) != size:
            raise EOFError(f'preview closed during a {size}-byte reply')
        return data

    def write(self, packet):
        self.process.stdin.write(packet)
        self.process.stdin.flush()

    def state(self):
        self.write(struct.pack('<BBHII', 6, 0, 0, self.address, self.read_length))
        signature, status, size = struct.unpack('<4sII', self.read(12))
        data = self.read(size)
        if signature != b'DATA' or status or size != self.read_length:
            raise RuntimeError(f'mTile preview memory read failed: {data.decode(errors="replace")}')
        return {name: struct.unpack_from('<i', data, offset)[0] // 65536
                for name, offset in self.members.items()}

    def step(self, mask):
        self.write(struct.pack('<BBH', 1, 0, mask))
        signature, frame, audio, overflow, telemetry = struct.unpack('<4sIIII', self.read(20))
        if signature != b'FRAM' or overflow:
            raise ValueError('invalid frame or overflowing audio from mTile emulator')
        rgba = self.read(WIDTH * HEIGHT * 4)
        audible = any(self.read(audio * 4))
        self.read(telemetry)
        return frame, rgba, audible, self.state()


def replay(preview, route, output):
    evidence = {'nonzero_pcm_frames': 0, 'checkpoints': {}, 'screens': {},
                'mario_sprite_pixels': {}}
    names = {(1, 0): 'title', (1, 1): 'world-map', (1, 2): 'world1-1',
             (1, 4): 'course-clear', (2, 2): 'world1-2'}
    pending = {}
    captured = set()
    previous = None

    def step(mask):
        nonlocal previous
        frame, rgba, audible, state = preview.step(mask)
        state['display_frame'] = frame
        evidence['nonzero_pcm_frames'] += audible
        if (previous is not None and previous['stage'] == state['stage'] == 2
                and previous['course'] == state['course']):
            delta = state['cam_y'] - previous['cam_y']
            elapsed = state['tick'] - previous['tick']
            # The emulator can sample between the tick counter and the camera write.
            if not -3 * (elapsed + 1) <= delta <= 4 * (elapsed + 1):
                raise ValueError(f'vertical camera snapped {delta} pixels in {elapsed} ticks: {state}')
        previous = state
        label = names.get((state['course'], state['stage']))
        if label and label not in captured:
            pending.setdefault(label, frame + 5)
        if state['stage'] == 2 and state['course'] == 1 and state['player_y'] < 350:
            if 'jump' not in captured:
                pending.setdefault('jump', frame)
        ready = [name for name, due in pending.items() if frame >= due]
        for name in ready:
            if name in ('world1-1', 'world1-2'):
                evidence['mario_sprite_pixels'][name] = mario_sprite_pixels(rgba, state)
            evidence['screens'][name] = hashlib.sha256(rgba).hexdigest()
            if output is not None:
                write_png(output / f'{name}.png', rgba)
            del pending[name]
            captured.add(name)
        return state

    def apply(segments):
        for duration, mask in segments:
            for _ in range(duration):
                state = step(mask)
        return state

    entered = apply(route['boot'])
    if entered['stage'] != 2 or entered['course'] != 1 or entered['player_x'] != 24:
        raise ValueError(f'failed to enter World 1-1: {entered}')
    evidence['checkpoints']['entered_1_1'] = entered
    if 'course_ticks' in route:
        # Hardware can miss a presentation without taking another input sample.
        # Change inputs at the same physics tick across builds.
        schedule = route['course_ticks']
        cursor = 0
        clear = entered
        for _ in range(1600):
            while cursor + 1 < len(schedule) and clear['tick'] >= schedule[cursor + 1][0]:
                cursor += 1
            clear = step(schedule[cursor][1])
            if clear['stage'] != 2:
                break
    else:
        clear = apply(route['course'])
    if clear['stage'] != 4 or clear['course'] != 1 or not 1 <= clear['card'] <= 3:
        raise ValueError(f'World 1-1 did not award a card: {clear}')
    evidence['checkpoints']['cleared_1_1'] = clear
    after_clear = apply(((114, 0), (8, 8), (16, 0)))
    if after_clear['stage'] != 1 or after_clear['course'] != 1:
        raise ValueError(f'did not return to World 1 map after the clear: {after_clear}')
    next_course = apply(((8, 16), (48, 0), (8, 16), (48, 0), (8, 1), (10, 0), (50, 18)))
    if next_course['stage'] != 2 or next_course['course'] != 2 or next_course['player_x'] <= 24:
        raise ValueError(f'World 1-2 entry or movement failed: {next_course}')
    evidence['checkpoints']['moved_in_1_2'] = next_course
    if evidence['nonzero_pcm_frames'] == 0:
        raise ValueError('the replay contained no nonzero PCM frames')
    return evidence


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('rom', type=Path, help='generated native .gba cartridge')
    parser.add_argument('elf', type=Path, help='game.elf from the same mtile build -keep-work')
    parser.add_argument('helper', type=Path, help='mtile-player with memory read support')
    parser.add_argument('--route', type=Path, default=Path(__file__).with_name('world1-clear-route.json'))
    parser.add_argument('--output', type=Path, help='write the JSON report and checkpoint PNGs here')
    parser.add_argument('--nm', default='arm-none-eabi-nm')
    parser.add_argument('--dwarfdump', default='llvm-dwarfdump')
    args = parser.parse_args()
    route = json.loads(args.route.read_text())
    address, members = read_game_layout(args.elf, args.nm, args.dwarfdump)
    if args.output is not None:
        args.output.mkdir(parents=True, exist_ok=True)
    with subprocess.Popen([str(args.helper.resolve()), str(args.rom.resolve())],
                          stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                          stderr=subprocess.DEVNULL) as process:
        try:
            preview = Preview(process, address, members)
            evidence = replay(preview, route, args.output)
        finally:
            try:
                process.stdin.write(struct.pack('<BBH', 3, 0, 0))
                process.stdin.flush()
            except BrokenPipeError:
                pass
            try:
                process.wait(timeout=3)
            except subprocess.TimeoutExpired:
                process.kill()
    evidence['rom_sha256'] = hashlib.sha256(args.rom.read_bytes()).hexdigest()
    report = json.dumps(evidence, indent=2) + '\n'
    if args.output is not None:
        (args.output / 'verification.json').write_text(report)
    print(report, end='')


if __name__ == '__main__':
    main()
