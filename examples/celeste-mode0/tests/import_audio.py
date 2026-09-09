#!/usr/bin/env python3
"""Reproducible Celeste Classic -> mtile PSG arrangement (not sample parity)."""
from __future__ import annotations
import argparse
from dataclasses import asdict
from fractions import Fraction
import hashlib
import json
import math
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(ROOT / 'examples/celeste-mode0/tests'))
from pico_source import load_cart, authored_music, sfx_length

PROJECT = ROOT / 'examples/celeste'
SOURCE = ROOT / 'celeste/smalleste.p8'
SFX_IDS = (0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 13, 14, 15, 16, 23, 35, 54, 55)
SONG_IDS = (0, 10, 20, 30, 40)
# Channel steal ladder. SFX must sit above song notes to take a music channel.
PRIORITY_MUSIC = 80
PRIORITY_SFX = 160
# All arrangements use 120 BPM, including when stopped: SFX never change speed.
TICKS_PER_SECOND = 192
UNIT_SECONDS = Fraction(183, 22050)
UNIT_TICKS = UNIT_SECONDS * TICKS_PER_SECOND


def nearest(v):
    return math.floor(v + Fraction(1, 2))


def tick(units):
    return nearest(units * UNIT_TICKS)


def waveform(shape, phase):
    triangle = lambda p: 1 - 4 * abs((p % 1) - .5)
    if shape == 0:
        return triangle(phase)
    if shape == 1:
        return phase / .875 * 2 - 1 if phase < .875 else 1 - (phase-.875) / .125 * 2
    if shape == 2:
        return phase * 2 - 1
    if shape == 3:
        return 1 if phase < .5 else -1
    if shape == 4:
        return 1 if phase < .25 else -1
    if shape == 5:
        return (triangle(phase) + .5 * triangle(phase * 2)) / 1.5
    if shape == 7:
        # Static bright approximation; PICO's moving phase cannot be a PSG wave.
        return (triangle(phase) + .5 * triangle(phase * 4 + .125)) / 1.5
    raise ValueError(f'unsupported tonal waveform {shape}')


def build_assets():
    cart = load_cart(SOURCE)
    patterns = {p['id']: p for p in authored_music(cart)['patterns']}
    sounds, clips, parts, songs, provenance = [], [], [], [], []
    sound_ids = {}
    next_clip = 64
    split_sfx = {}

    def sound(kind, shape, volume, pitch):
        # Noise is a polynomial clock, not a pitched oscillator. Preserve the
        # rising/falling contour with the closest available LFSR clock.
        noise = None
        if kind == 'ch4':
            hz = 16384 * 2 ** ((pitch - 48) / 12)
            noise = min(((d, s) for d in range(8) for s in range(12)),
                        key=lambda ds: abs(math.log2((524288 / ((ds[0] or .5) * 2 ** (ds[1]+1))) / hz)))
        key = (kind, shape if kind == 'ch3' else (25 if shape in (1, 4, 7) else 50), volume, noise)
        if key in sound_ids:
            return sound_ids[key]
        sid = len(sounds)
        obj = dict(id=sid, name=f'{kind} wave{shape} vol{volume}' + (f' n{noise[0]}/{noise[1]}' if noise else ''), kind=kind, route='both')
        env = dict(initial=max(1, nearest(volume * Fraction(15, 7))), direction='down', pace=0)
        if kind in ('ch1', 'ch2'):
            obj['pulse'] = dict(duty=key[1], length=0, useLength=False, envelope=env)
            if kind == 'ch1':
                obj['pulse']['sweep'] = dict(pace=0, direction='down', shift=0)
        elif kind == 'ch3':
            obj['wave'] = dict(samples=[max(0, min(15, nearest(7.5 + waveform(shape, i/32) * 7.5 * volume / 7))) for i in range(32)], level=100, startBank=0, length=0, useLength=False)
        else:
            obj['noise'] = dict(width=15, divisor=noise[0], shift=noise[1], length=0, useLength=False, envelope=env)
        sounds.append(obj)
        sound_ids[key] = sid
        return sid

    def note_at(effect, units):
        step = int(units // effect.speed)
        if step >= effect.loop_end and effect.loop_start < effect.loop_end:
            step = effect.loop_start + (step - effect.loop_start) % (effect.loop_end-effect.loop_start)
        if step >= sfx_length(effect):
            return None, None
        return step, effect.notes[step]

    def events(sfx_id, duration, offset, kind, tonal, base_units=0, clip_id=0):
        effect = cart.sfx[sfx_id]
        result = []
        unit = Fraction(0)
        while unit < duration:
            absolute = unit + offset
            step, note = note_at(effect, absolute)
            remain = effect.speed - absolute % effect.speed
            end = min(duration, unit + remain)
            if note is None:
                break
            target = 'ch4' if note.waveform == 6 else tonal
            # Four control points for pitch/volume effects. These are explicit
            # PSG approximations of PICO's continuous effects.
            slices = (2 if note.waveform == 6 else 4) if note.effect else 1
            for sub in range(slices):
                start_u = unit + (end-unit) * Fraction(sub, slices)
                end_u = unit + (end-unit) * Fraction(sub+1, slices)
                if target != kind or not note.volume:
                    continue
                phase = float((start_u + offset) % effect.speed / effect.speed)
                pitch, volume = note.pitch, note.volume
                previous = effect.notes[max(0, step-1)]
                if note.effect == 1:
                    pitch = nearest(previous.pitch + (pitch-previous.pitch) * phase)
                elif note.effect == 2:
                    pitch += nearest(.5 * math.sin(phase * 4 * math.pi))
                elif note.effect == 3:
                    pitch = max(0, nearest(pitch - 12 * phase))
                elif note.effect == 4:
                    volume = nearest(volume * phase)
                elif note.effect == 5:
                    volume = nearest(volume * (1-phase))
                elif note.effect in (6, 7):
                    group = step // 4 * 4
                    cadence = (2 if effect.speed <= 8 else 4) * (2 if note.effect == 7 else 1)
                    pitch = effect.notes[group + int((start_u+offset) / cadence) % 4].pitch
                start_t = tick(base_units+start_u) - tick(base_units)
                end_t = tick(base_units+end_u) - tick(base_units)
                if end_t <= start_t or volume == 0:
                    continue
                sid = sound(kind, note.waveform, volume, pitch)
                event = dict(tick=start_t, durationTicks=end_t-start_t, note=pitch+36, soundId=sid, priority=PRIORITY_MUSIC if clip_id >= 64 else PRIORITY_SFX)
                # Legato would preserve envelope but native wave edits can
                # change timbre/volume; keep explicit hardware retriggers.
                result.append(event)
                provenance.append(dict(clip=clip_id, event=len(result)-1, sfx=sfx_id, step=step, startUnits=str(start_u), baseUnits=str(base_units), sourcePitch=note.pitch, sourceVolume=note.volume, sourceWaveform=note.waveform, sourceEffect=note.effect, renderedPitch=pitch, renderedVolume=volume))
            unit = end
        return result

    def add_clip(cid, name, kind, duration, ev, sfx=False, base=0):
        obj = dict(id=cid, name=name, kind=kind, lengthTicks=max(1, tick(base+duration)-tick(base)), events=ev)
        if sfx:
            obj['sfx'] = True
        clips.append(obj)

    for sid in SFX_IDS:
        effect = cart.sfx[sid]
        duration = Fraction(effect.speed * sfx_length(effect))
        tonal_shapes = {n.waveform for n in effect.notes if n.volume and n.waveform != 6}
        tonal = 'ch1' if tonal_shapes and tonal_shapes <= {3, 4} else 'ch3'
        channels = ([tonal] if tonal_shapes else []) + (['ch4'] if any(n.waveform == 6 and n.volume for n in effect.notes) else [])
        ids = []
        for index, kind in enumerate(channels):
            cid = sid if index == 0 else next_clip
            if index:
                next_clip += 1
            ev = events(sid, duration, 0, kind, tonal, clip_id=cid)
            # Gameplay priority is explicit, including a second split clip.
            for event in ev:
                event['priority'] = PRIORITY_SFX
            add_clip(cid, f'Celeste SFX {sid} {kind}', kind, duration, ev, True)
            ids.append(cid)
        split_sfx[sid] = ids

    for entry in SONG_IDS:
        sequence = []
        pos = entry
        while pos >= 0 and pos not in sequence:
            sequence.append(pos)
            pos = patterns[pos]['next']
        loop_start = sequence.index(pos) if pos in sequence else None
        songs.append(dict(id=entry, name=f'Celeste music {entry}', tempo=120, sequence=sequence, loopStart=loop_start))
        elapsed = Fraction(0)
        previous_ids = [-1] * 4
        offsets = [Fraction(0)] * 4
        for pid in sequence:
            pat = patterns[pid]
            duration = Fraction(pat['durationUnits'])
            tone_channels = [i for i, sid in enumerate(pat['channels']) if sid >= 0 and any(n.volume and n.waveform != 6 for n in cart.sfx[sid].notes)]
            # Give scarce CH3 to the voice with most non-square authored notes.
            wave_voice = max(tone_channels, key=lambda i: sum(n.volume > 0 and n.waveform in (0, 1, 2, 5) for n in cart.sfx[pat['channels'][i]].notes), default=-1)
            tonal = {wave_voice: 'ch3'}
            for i, ch in enumerate(c for c in tone_channels if c != wave_voice):
                tonal[ch] = ('ch1', 'ch2')[i]
            merged = {f'ch{i}': [] for i in range(1, 5)}
            clip_ids = {kind: next_clip+i for i, kind in enumerate(merged)}
            next_clip += 4
            for source_ch, sid in enumerate(pat['channels']):
                if sid < 0:
                    offsets[source_ch] = Fraction(0)
                    continue
                effect = cart.sfx[sid]
                if previous_ids[source_ch] != sid or not effect.loop_start < effect.loop_end:
                    offsets[source_ch] = Fraction(0)
                for kind in merged:
                    merged[kind].extend(events(sid, duration, offsets[source_ch], kind, tonal.get(source_ch, 'ch1'), elapsed, clip_ids[kind]))
                offsets[source_ch] += duration
            previous_ids = pat['channels']
            lanes = dict.fromkeys(merged)
            for kind, ev in merged.items():
                if not ev:
                    continue
                ev.sort(key=lambda e: e['tick'])
                for a, b in zip(ev, ev[1:]):
                    if a['tick'] + a['durationTicks'] > b['tick']:
                        raise ValueError(f'overlapping source voices in music{pid}/{kind}; explicit arrangement needed')
                cid = clip_ids[kind]
                add_clip(cid, f'Celeste pattern {pid} {kind}', kind, duration, ev, base=elapsed)
                lanes[kind] = cid
            parts.append(dict(id=pid, name=f'Celeste pattern {pid}', lengthTicks=tick(elapsed+duration)-tick(elapsed), lanes=lanes))
            elapsed += duration

    if len(sounds) > 256 or next_clip > 256 or sum(len(c['events']) for c in clips) > 8192:
        raise ValueError(f'PSG limits: sounds={len(sounds)}, next clip={next_clip}, events={sum(len(c["events"]) for c in clips)}')
    raw = SOURCE.read_text()
    source_audio = raw[raw.index('__sfx__'):]
    manifest = dict(source='celeste/smalleste.p8', sourceAudioSHA256=hashlib.sha256(source_audio.encode()).hexdigest(), sfx=[asdict(s) for s in cart.sfx], music=[asdict(m) for m in cart.music], sourceSFXToClips=split_sfx, songEntryIDs=SONG_IDS, ticksPerSecond=192, speedUnitSeconds='183/22050', events=provenance, fidelity='PSG arrangement: authored source retained exactly; native waveforms, effects, channel assignment and event times are quantized approximations.')
    return {'assets/audio/sounds.json': dict(version=2, sounds=sounds), 'assets/audio/music.json': dict(version=2, ppqn=96, clips=clips, parts=parts, songs=songs), 'assets/audio/source-pico8.json': manifest}


def canonical_json(content):
    """Ignore JSON formatting/key order, preserving every value and array order."""
    def unique_object(items):
        result = {}
        for key, value in items:
            if key in result:
                raise ValueError(f'duplicate JSON key: {key}')
            result[key] = value
        return result
    model = json.loads(content, object_pairs_hook=unique_object)
    # Unlike Python dict equality, this also distinguishes true from 1 and
    # integer schema values from floats. Unknown/missing fields remain visible.
    return json.dumps(model, sort_keys=True, separators=(',', ':'), ensure_ascii=False, allow_nan=False)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check', action='store_true', help='Compare committed local assets without writing')
    args = parser.parse_args()
    for relative, obj in build_assets().items():
        content = obj if isinstance(obj, str) else json.dumps(obj, indent=2, ensure_ascii=False) + '\n'
        path = PROJECT / relative
        if args.check:
            if not path.exists():
                raise SystemExit(f'Audio import is missing: {path}.')
            actual = path.read_text()
            if isinstance(obj, str):
                matches = actual == content
            else:
                try:
                    matches = canonical_json(actual) == canonical_json(content)
                except ValueError as error:
                    raise SystemExit(f'Invalid audio JSON in {path}: {error}') from error
            if not matches:
                raise SystemExit(f'Audio data differs: {path}. Assets were not changed; inspect authored differences before regenerating.')
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(content)
        print(('Verified ' if args.check else 'Wrote ') + relative)

if __name__ == '__main__':
    main()
