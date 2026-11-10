"""Compile original SMB3 sound-driver events to native mTile PSG assets."""
import argparse
from dataclasses import dataclass
from itertools import groupby
import json
import math
from pathlib import Path

from read_audio import QUEUE_LEVEL, QUEUE_MAP, QUEUE_MUSIC1, QUEUE_MUSIC2, QUEUE_PLAYER, read_track
from rom_layout import check_rom

NES_CPU_CLOCK = 1_789_773
NES_FPS = 60.0988
TEMPO = 120
TICKS_PER_FRAME = TEMPO * 96 / (60 * NES_FPS)
LENGTH = (10, 254, 20, 2, 40, 4, 80, 6, 160, 8, 60, 10, 14, 12, 26, 14,
          12, 16, 24, 18, 48, 20, 96, 22, 192, 24, 72, 26, 16, 28, 32, 30)
NES_NOISE = (4, 8, 16, 32, 64, 96, 128, 160, 202, 254, 380, 508, 762, 1016, 2034, 4068)
GB_NOISE = tuple((524288 / ((divisor or 0.5) * 2 ** (shift + 1)), divisor, shift)
                 for divisor in range(8) for shift in range(16))
TRIANGLE = list(range(15, -1, -1)) + list(range(16))

# IDs are the Lua API contract. Loop starts and boundaries come from the ROM
# driver's advancing Music_NextIndex, not from wall-clock capture.
SONGS = (
    ('Plains', QUEUE_MUSIC2, 0x10, (0, 192, 1728), 1),
    ('World 1 map', QUEUE_MUSIC2, 0x01, (0, 864), 0),
    ('Hills', QUEUE_MUSIC2, 0x90, (0, 192, 2040), 1),
    ('Course clear', QUEUE_MUSIC1, 0x20, (0, 193), None),
    ('Lost life', QUEUE_MUSIC1, 0x01, (0, 193), None),
)
EFFECTS = (
    ('Jump', QUEUE_PLAYER, 0x01, 0), ('Coin', QUEUE_LEVEL, 0x01, 1),
    ('Bump', QUEUE_PLAYER, 0x02, 0), ('Squish', QUEUE_PLAYER, 0x04, 0),
    ('Kick', QUEUE_PLAYER, 0x08, 0), ('Pipe', QUEUE_PLAYER, 0x10, 0),
    ('Power up', QUEUE_LEVEL, 0x20, 1), ('Extra life', QUEUE_LEVEL, 0x40, 1),
    ('Map step', QUEUE_MAP, 0x02, 0),
)


def note_for(period, channel):
    divider = 32 if channel == 2 else 16
    hz = NES_CPU_CLOCK / (divider * (period + 1))
    return max(24 if channel == 2 else 36, min(127, round(69 + 12 * math.log2(hz / 440))))


@dataclass
class Channel:
    control: int = 0
    timer: int = 0
    sweep: int = 0
    sweep_clock: int = 0
    envelope: int = 0
    envelope_clock: int = 0
    length: int = 0
    linear: int = 0
    running: bool = False

    def restart(self, value):
        self.running = True
        self.length = LENGTH[value >> 3]
        self.envelope = 15
        self.envelope_clock = (self.control & 15) + 1
        self.linear = self.control & 127

    def state(self, index):
        if not self.running or not self.length:
            return None
        if index == 2:
            return (note_for(self.timer, index), 0) if self.linear else None
        volume = (self.control & 15) if self.control & 16 else self.envelope
        if volume == 0:
            return None
        if index == 3:
            period = NES_NOISE[self.timer & 15]
            width = 7 if self.timer & 128 else 15
            return 36, volume, width, period
        if self.timer < 8:
            return None
        return note_for(self.timer, index), volume, (12.5, 25, 50, 75)[self.control >> 6]

    def advance(self, index):
        if not self.running:
            return
        if index == 2:
            if not self.control & 128:
                self.linear = max(0, self.linear - 4)
                self.length = max(0, self.length - 2)
            return
        for _ in range(4):
            self.envelope_clock -= 1
            if self.envelope_clock <= 0:
                self.envelope_clock = (self.control & 15) + 1
                if self.envelope or self.control & 32:
                    self.envelope = (self.envelope - 1) & 15
        if not self.control & 32:
            self.length = max(0, self.length - 2)
        if index >= 2 or not self.sweep & 128 or not self.sweep & 7:
            return
        for _ in range(2):
            self.sweep_clock -= 1
            if self.sweep_clock > 0:
                continue
            self.sweep_clock = ((self.sweep >> 4) & 7) + 1
            delta = self.timer >> (self.sweep & 7)
            if self.sweep & 8:
                delta = -delta - int(index == 0)
            if 8 <= self.timer + delta < 2048:
                self.timer += delta


def sound_states(rom, queue, value, frames):
    channels = [Channel() for _ in range(4)]
    drum_address = 0
    drum_tail = 0
    enabled = 15
    for writes, _, _ in read_track(rom, queue, value, frames):
        for address, v in writes:
            if address == 0x4012:
                drum_address = v
            elif address == 0x4015:
                for i, channel in enumerate(channels):
                    if not v & (1 << i):
                        channel.running = False
                if v & 16 and not enabled & 16:
                    drum_tail = 4
                enabled = v
            elif 0x4000 <= address < 0x4010:
                index, reg = divmod(address - 0x4000, 4)
                channel = channels[index]
                if reg == 0:
                    channel.control = v
                    if index == 2:
                        channel.linear = v & 127
                elif reg == 1:
                    channel.sweep = v
                elif reg == 2:
                    channel.timer = (channel.timer & 0x700) | v
                elif enabled & (1 << index):
                    channel.timer = (channel.timer & 255) | ((v & 7) << 8)
                    channel.restart(v)
        states = [channel.state(i) for i, channel in enumerate(channels)]
        # GBA PSG lacks a DMC lane. Convert drum triggers to short noise
        # voices only where the actual NES noise channel has no active voice.
        if drum_tail and states[3] is None:
            states[3] = (36, drum_tail + 1, 15, 380 if drum_address & 1 else 96)
        yield states
        drum_tail = max(0, drum_tail - 1)
        for i, channel in enumerate(channels):
            channel.advance(i)


class PSGAssets:
    def __init__(self):
        self.sound_ids = {}
        self.sounds = []
        self.clips = []
        self.parts = []
        self.songs = []

    def instrument(self, channel, state):
        key = channel, state[1:]
        if key in self.sound_ids:
            return self.sound_ids[key]
        id = len(self.sounds)
        self.sound_ids[key] = id
        name = f'ch{channel + 1} voice {id}'
        sound = {'id': id, 'name': name, 'kind': f'ch{channel + 1}', 'route': 'both'}
        envelope = {'initial': state[1], 'direction': 'down', 'pace': 0}
        if channel == 2:
            sound['wave'] = {'samples': TRIANGLE, 'level': 50, 'startBank': 0, 'length': 0, 'useLength': False}
        elif channel == 3:
            frequency = NES_CPU_CLOCK / state[3]
            _, divisor, shift = min(GB_NOISE, key=lambda item: abs(math.log(item[0] / frequency)))
            sound['noise'] = {'width': state[2], 'divisor': divisor, 'shift': shift,
                              'length': 0, 'useLength': False, 'envelope': envelope}
        else:
            sound['pulse'] = {'duty': state[2], 'length': 0, 'useLength': False, 'envelope': envelope}
            if channel == 0:
                sound['pulse']['sweep'] = {'pace': 0, 'direction': 'down', 'shift': 0}
        self.sounds.append(sound)
        return id

    def clip(self, name, channel, states, sfx=False):
        id = len(self.clips)
        events = []
        frame = 0
        for state, values in groupby(row[channel] for row in states):
            next_frame = frame + sum(1 for _ in values)
            if state is not None:
                tick = round(frame * TICKS_PER_FRAME)
                end = round(next_frame * TICKS_PER_FRAME)
                events.append({'tick': tick, 'durationTicks': end - tick, 'note': state[0],
                               'soundId': self.instrument(channel, state), 'priority': 128})
            frame = next_frame
        length = round(len(states) * TICKS_PER_FRAME)
        clip = {'id': id, 'name': name, 'kind': f'ch{channel + 1}', 'lengthTicks': length, 'events': events}
        if sfx:
            clip['sfx'] = True
        self.clips.append(clip)
        return id

    def song(self, name, parts, loop):
        sequence = []
        for index, states in enumerate(parts):
            id = len(self.parts)
            lanes = {f'ch{i + 1}': self.clip(f'{name} {index + 1} ch{i + 1}', i, states) for i in range(4)}
            self.parts.append({'id': id, 'name': f'{name} {index + 1}',
                               'lengthTicks': round(len(states) * TICKS_PER_FRAME), 'lanes': lanes})
            sequence.append(id)
        song = {'id': len(self.songs), 'name': name, 'tempo': TEMPO, 'sequence': sequence}
        if loop is not None:
            song['loopStart'] = loop
        self.songs.append(song)

    def write(self, output):
        output.mkdir(parents=True, exist_ok=True)
        settings = {'version': 2, 'profile': 'gameboy-psg', 'leftLevel': 7, 'rightLevel': 7, 'psgRatio': 100}
        music = {'version': 2, 'ppqn': 96, 'clips': self.clips, 'parts': self.parts, 'songs': self.songs}
        for name, content in [('settings', settings), ('sounds', {'version': 2, 'sounds': self.sounds}), ('music', music)]:
            (output / f'{name}.json').write_text(json.dumps(content, separators=(',', ':')) + '\n')


def extract_sound(rom, output):
    check_rom(rom)
    assets = PSGAssets()
    for name, queue, value, channel in EFFECTS:
        states = list(sound_states(rom, queue, value, 120))
        length = max((i + 1 for i, row in enumerate(states) if row[channel] is not None), default=0)
        if not length:
            raise ValueError(f'No active PSG data for effect {name}')
        assets.clip(name, channel, states[:length], sfx=True)
    for name, queue, value, boundaries, loop in SONGS:
        states = list(sound_states(rom, queue, value, boundaries[-1]))
        assets.song(name, [states[start:end] for start, end in zip(boundaries, boundaries[1:])], loop)
    assets.write(output)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('rom', type=Path)
    parser.add_argument('audio_directory', type=Path)
    args = parser.parse_args()
    extract_sound(args.rom.read_bytes(), args.audio_directory)


if __name__ == '__main__':
    main()
