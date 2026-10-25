#!/usr/bin/env python3
"""Rebuild Lumen's original, editable Game Boy PSG score and five effects.

The score is 16 bars in 4/4 at 80 BPM: one 48-second loop. All timing is
integer 96-PPQN; no samples, external instruments, or random data are used.
Run from any directory with: python3 examples/lumen/generate_audio.py
"""

import json
import math
from pathlib import Path


DESTINATION = Path(__file__).resolve().parent / "assets" / "audio"
PPQN = 96
BAR = PPQN * 4
PHRASE = BAR * 4


def write_json(name, value):
    DESTINATION.mkdir(parents=True, exist_ok=True)
    (DESTINATION / name).write_text(json.dumps(value, indent=2) + "\n", encoding="utf-8")


def pulse(sound_id, name, duty, volume, pace):
    return {
        "id": sound_id, "name": name, "kind": "ch1", "route": "both",
        "pulse": {
            "duty": duty, "length": 0, "useLength": False,
            "envelope": {"initial": volume, "direction": "down", "pace": pace},
            "sweep": {"pace": 0, "direction": "down", "shift": 0},
        },
    }


def noise(sound_id, name, volume, divisor, shift):
    return {
        "id": sound_id, "name": name, "kind": "ch4", "route": "both",
        "noise": {
            "width": 15, "divisor": divisor, "shift": shift,
            "length": 0, "useLength": False,
            "envelope": {"initial": volume, "direction": "down", "pace": 1},
        },
    }


def note(tick, duration, pitch, sound, priority=96):
    return {
        "tick": tick, "durationTicks": duration,
        "note": pitch, "soundId": sound, "priority": priority,
    }


def clip(clip_id, name, kind, length, events, sfx=False):
    value = {
        "id": clip_id, "name": name, "kind": kind,
        "lengthTicks": length, "events": events,
    }
    if sfx:
        value["sfx"] = True
    return value


def generate():
    # The reduced master ratio leaves space around the score and its effects.
    write_json("settings.json", {
        "version": 2, "profile": "gameboy-psg",
        "leftLevel": 6, "rightLevel": 6, "psgRatio": 50,
    })

    melody = pulse(0, "Dewbell", 50, 4, 4)
    arpeggio = pulse(1, "Glass fern", 25, 3, 3)
    arpeggio["kind"] = "ch2"
    del arpeggio["pulse"]["sweep"]
    # A single gently rounded cycle with a small second harmonic. CH3's
    # quarter output level makes it a quiet supporting voice, not a lead.
    waveform = [round(8 + 5 * math.sin(i * math.tau / 32)
                      + math.sin(i * math.tau / 16)) for i in range(32)]
    bass = {
        "id": 2, "name": "Moonroot", "kind": "ch3", "route": "both",
        "wave": {
            "samples": waveform, "level": 25, "startBank": 0,
            "length": 0, "useLength": False,
        },
    }
    sounds = [
        melody, arpeggio, bass,
        noise(3, "Pollen in moonlight", 1, 3, 3),
        pulse(4, "Ember droplet", 12.5, 7, 1),
        pulse(5, "Lantern glass", 25, 7, 2),
        noise(6, "Moth wing", 3, 3, 2),
        pulse(7, "Soft landing", 50, 5, 2),
        pulse(8, "Dawn blossoms", 25, 7, 3),
    ]
    # A short descending frequency wobble gives the damage cue a rounded edge.
    sounds[7]["macro"] = {"steps": [
        {"periodDelta": 0}, {"periodDelta": -10},
        {"periodDelta": -22}, {"periodDelta": -10},
        {"periodDelta": -26}, {"periodDelta": -18},
    ]}
    write_json("sounds.json", {"version": 2, "sounds": sounds})

    # Standalone clip IDs are the game's stable sfx(0)..sfx(4) interface.
    # At 80 BPM these last 0.19, 0.66, 0.14, 0.28, and 1.50 seconds.
    clips = [
        clip(0, "Ember gathered", "ch1", 24, [
            note(0, 10, 86, 4, 160), note(12, 12, 93, 4, 160),
        ], True),
        clip(1, "Lantern restored", "ch1", 84, [
            note(0, 15, 74, 5, 176), note(18, 15, 81, 5, 176),
            note(36, 18, 86, 5, 176), note(57, 27, 90, 5, 176),
        ], True),
        clip(2, "Air dash", "ch4", 18, [note(0, 18, 60, 6, 160)], True),
        clip(3, "Bruised wing", "ch1", 36, [
            note(0, 16, 53, 7, 168), note(18, 18, 48, 7, 168),
        ], True),
        clip(4, "The garden remembers", "ch1", 192, [
            note(0, 24, 74, 8, 192), note(27, 24, 78, 8, 192),
            note(54, 24, 81, 8, 192), note(81, 27, 85, 8, 192),
            note(111, 27, 86, 8, 192), note(141, 51, 90, 8, 192),
        ], True),
    ]

    # D major with open ninths and suspensions: four small phrases rather than
    # a short ostinato. Each item is (bass note, four upper chord tones).
    harmony = [
        [(38, [62, 66, 69, 76]), (37, [61, 64, 69, 76]),
         (35, [59, 62, 66, 73]), (31, [59, 62, 66, 69])],
        [(30, [62, 66, 69, 76]), (31, [59, 62, 66, 69]),
         (28, [59, 62, 67, 71]), (33, [62, 64, 69, 74])],
        [(35, [61, 62, 66, 73]), (31, [59, 62, 66, 69]),
         (38, [62, 66, 69, 76]), (33, [61, 64, 69, 71])],
        [(28, [59, 62, 67, 71]), (31, [59, 62, 66, 69]),
         (33, [62, 64, 69, 74]), (38, [62, 66, 69, 76])],
    ]
    melody_phrases = [
        [(96, 84, 78), (288, 72, 76), (576, 144, 73),
         (816, 96, 74), (1008, 72, 78), (1296, 144, 81)],
        [(48, 120, 78), (336, 72, 76), (624, 96, 74),
         (864, 120, 71), (1152, 96, 73), (1368, 120, 76)],
        [(96, 144, 81), (384, 96, 78), (672, 72, 76),
         (864, 144, 78), (1152, 96, 81), (1392, 120, 83)],
        [(48, 120, 79), (288, 72, 78), (576, 144, 74),
         (864, 96, 73), (1104, 96, 76), (1344, 144, 74)],
    ]
    names = ["The sleeping gate", "Glass under leaves",
             "Fireflies remember", "A path toward morning"]
    parts = []
    for phrase_index, chords in enumerate(harmony):
        base = 16 + phrase_index * 4
        melody_events = [note(tick, duration, pitch, 0, 104)
                         for tick, duration, pitch in melody_phrases[phrase_index]]
        arpeggio_events, bass_events, shimmer_events = [], [], []
        for bar_index, (root, chord) in enumerate(chords):
            origin = bar_index * BAR
            # Asymmetric gaps let the glass notes breathe. Reverse the inner
            # notes every other bar so the repeated chords keep moving.
            pattern = [0, 2, 1, 3, 2, 1] if bar_index % 2 == 0 else [0, 1, 3, 2, 1, 2]
            for position, offset in enumerate([0, 72, 144, 192, 264, 336]):
                arpeggio_events.append(note(origin + offset, 30, chord[pattern[position]], 1, 88))
            bass_events.extend([
                note(origin, 276, root, 2, 80),
                note(origin + 288, 84, root + 7, 2, 80),
            ])
            if bar_index % 2 == 0:
                shimmer_events.append(note(origin + 240, 12, 60, 3, 64))
        clips.extend([
            clip(base, names[phrase_index] + " / dew", "ch1", PHRASE, melody_events),
            clip(base + 1, names[phrase_index] + " / glass", "ch2", PHRASE, arpeggio_events),
            clip(base + 2, names[phrase_index] + " / root", "ch3", PHRASE, bass_events),
            clip(base + 3, names[phrase_index] + " / pollen", "ch4", PHRASE, shimmer_events),
        ])
        parts.append({
            "id": phrase_index, "name": names[phrase_index], "lengthTicks": PHRASE,
            "bars": 4, "beatsPerBar": 4, "beatUnit": 4,
            "lanes": {"ch1": base, "ch2": base + 1, "ch3": base + 2, "ch4": base + 3},
        })
    write_json("music.json", {
        "version": 2, "ppqn": PPQN, "clips": clips, "parts": parts,
        "songs": [{"id": 0, "name": "Lumen - A Garden After Dark", "tempo": 80,
                   "sequence": [0, 1, 2, 3], "loopStart": 0}],
    })
    print(f"Generated 48-second Lumen score and five effects in {DESTINATION}")


if __name__ == "__main__":
    generate()
