#!/usr/bin/env python3
"""Verify source import, PSG event playback, and optionally native ROM audio."""
from __future__ import annotations
import argparse
import array
import json
import math
import os
from pathlib import Path
import re
import shutil
import select
import struct
import subprocess
import sys
import tempfile
import wave

from import_audio import PROJECT, ROOT, SOURCE, SFX_IDS, SONG_IDS, UNIT_SECONDS, load_cart


def check_source():
    subprocess.run([sys.executable, str(PROJECT/'tests/test_import_audio.py')], cwd=ROOT, check=True)
    subprocess.run([sys.executable, str(PROJECT/'tests/import_audio.py'), '--check'], cwd=ROOT, check=True)
    archive = json.loads((PROJECT/'assets/audio/source-pico8.json').read_text())
    # Independent raw-hex decoding, not the shared cart parser used by import.
    text = SOURCE.read_text()
    rows = text.split('__sfx__\n')[1].split('__')[0].strip().splitlines()
    for sid, row in enumerate(rows):
        s = archive['sfx'][sid]
        assert (s['filters'], s['speed'], s['loop_start'], s['loop_end']) == (int(row[:2],16)&254, int(row[2:4],16), int(row[4:6],16), int(row[6:8],16))
        for i,n in enumerate(s['notes']):
            cell=row[8+i*5:13+i*5]
            assert (n['pitch'],n['waveform'],n['volume'],n['effect']) == (int(cell[:2],16),int(cell[2],16),int(cell[3],16),int(cell[4],16))
    music_rows = text.split('__music__\n')[1].split('__')[0].strip().splitlines()
    assert len(music_rows) == len(archive['music'])
    for index, row in enumerate(music_rows):
        flags, cells = row.split()
        channels = [int(cells[i:i+2],16) for i in range(0,8,2)]
        expected = [None if c & 64 else c & 63 for c in channels]
        assert archive['music'][index] == dict(flags=int(flags,16),channels=expected)
    from fractions import Fraction
    music=json.loads((PROJECT/'assets/audio/music.json').read_text())
    clips={c['id']:c for c in music['clips']}
    max_error=0
    for p in archive['events']:
        event=clips[p['clip']]['events'][p['event']]
        # Native event pitch is exactly the importer's documented quantized
        # result. Original pitches/effects remain available alongside it.
        assert event['note']==p['renderedPitch']+36
        base=Fraction(p['baseUnits']); start=Fraction(p['startUnits'])
        from import_audio import tick
        ideal=(base+start)*UNIT_SECONDS
        actual=Fraction(tick(base)+event['tick'],192)
        error=abs(ideal-actual)
        assert error<=Fraction(1,384),(p,float(error))
        max_error=max(max_error,error)
    print(f'PASS: {len(rows)*32} source note cells and {len(archive["music"])} pattern records retained; {len(archive["events"])} rendered events, onset rounding ≤{float(max_error)*1000:.3f} ms.')
    subprocess.run(['go','run','examples/celeste/tests/audio_verify.go'],cwd=ROOT,check=True)


def project_lua(mode0):
    parts=[(mode0/'game.lua').read_text()]
    src_dir=mode0/'src'
    if src_dir.is_dir():
        parts.extend(path.read_text() for path in sorted(src_dir.glob('*.lua')))
    return '\n'.join(parts)


def check_wrappers():
    mode0=Path(__file__).resolve().parents[1]
    assert not (mode0/'src/sound.lua').exists()
    src=project_lua(mode0)
    assert 'sfx_timer' not in src and 'music_timer' not in src
    assert 'function play_' not in src and 'sound_init' not in src
    archive=json.loads((mode0/'assets/audio/source-pico8.json').read_text())
    want=set()
    for sid in ('0','1','2','3','4','5','9','54','55'):
        want.update(archive['sourceSFXToClips'][sid])
    names={name:int(value) for name,value in re.findall(r'local (sfx_\w+) = (\d+)', src)}
    called=set()
    for arg in re.findall(r'sfx\(([^)]+)\)', src):
        arg=arg.strip()
        if arg.isdigit():
            called.add(int(arg))
        elif arg in names:
            called.add(names[arg])
    assert called, 'gameplay must call sfx() at events'
    assert want.issubset(called), (want, called)
    assert 'music(0)' in src
    print('PASS: gameplay calls native clips at the event; no sound.lua wrappers or sfx_timer.')


def check_gba():
    output=Path(tempfile.mkdtemp(prefix='mtile-celeste-audio-gba-'))
    project=output/'fixture'
    shutil.copytree(PROJECT/'assets',project/'assets')
    (project/'src').mkdir()
    shutil.copy(PROJECT/'src/sound.lua',project/'src/sound.lua')
    cart=load_cart(SOURCE)
    archive=json.loads((PROJECT/'assets/audio/source-pico8.json').read_text())
    segments=[]
    for sid in SFX_IDS:
        s=cart.sfx[sid]
        last=max(i for i,n in enumerate(s.notes) if n.volume)+1
        length=math.ceil(float(last*s.speed*UNIT_SECONDS)*30)+12
        call='\n'.join(f'sfx({cid}, 160)' for cid in archive['sourceSFXToClips'][str(sid)])
        segments.append(dict(label=f'sfx-{sid}',ticks=length,call=call))
    for sid in SONG_IDS:
        segments.append(dict(label=f'music-{sid}',ticks=90,call=f'music({sid})'))
    code='require("sound")\nframe=0\nsegment=0\nfunction _init() music(-1) end\nfunction _update()\nframe=frame+1\n'
    when=15
    for i,segment in enumerate(segments):
        segment['startUpdate']=when
        code+=f'if frame=={when} then\nmusic(-1)\nsfx(-1)\nsegment={i+1}\n{segment["call"]}\nend\n'
        when+=segment['ticks']
    code+=f'if frame=={when} then segment=0 music(-1) sfx(-1) end\nend\n'
    # Diagnostic pixels encode the segment with palette indices in two nibbles.
    code+='function _draw() cls(0) rectfill(0,0,7,7,segment%16) rectfill(8,0,15,7,flr(segment/16)) end\n'
    (project/'game.lua').write_text(code)
    run=subprocess.run(['go','run','./cmd/mtile','build',str(project)],cwd=ROOT,text=True,capture_output=True)
    (output/'build.txt').write_text(run.stdout+run.stderr)
    if run.returncode:
        raise SystemExit(run.stdout+run.stderr)
    print(run.stdout.strip())
    player=ROOT/'build/player/mtile-player'
    if not player.is_file(): raise SystemExit('Build native mtile-player before --gba verification.')
    diagnostics=(output/'mgba.log').open('wb')
    p=subprocess.Popen([str(player),str(project/'game.gba')],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=diagnostics,bufsize=0)
    def read(n):
        data=bytearray()
        while len(data)<n:
            if not select.select([p.stdout],[],[],10)[0]:
                raise TimeoutError(f'mGBA did not return a frame; see {output}/mgba.log')
            part=p.stdout.read(n-len(data))
            if not part:raise RuntimeError(f'mGBA exited; see {output}/mgba.log')
            data.extend(part)
        return bytes(data)
    captures={s['label']:bytearray() for s in segments}
    colours=json.loads((project/'assets/palettes/aurora.json').read_text())['colors'][:16]
    marker_rgb={tuple((c[k]<<3)|(c[k]>>2) for k in ('r','g','b')):i for i,c in enumerate(colours)}
    # Visible marker follows actual emulation, avoiding an assumed boot delay.
    all_audio=bytearray()
    try:
        assert read(16)[:4]==b'GLUP'
        for display in range((when+10)*2+300):
            p.stdin.write(struct.pack('<BBH',1,0,0));p.stdin.flush()
            header=read(20);assert header[:4]==b'FRAM'
            _,samples,overflow,telemetry=struct.unpack('<4I',header[4:]);assert overflow==0
            rgba=read(240*160*4);audio=read(samples*4);read(telemetry)
            all_audio.extend(audio)
            low=marker_rgb.get(tuple(rgba[:3]),-1)
            high=marker_rgb.get(tuple(rgba[8*4:8*4+3]),-1)
            marker=low+16*high if low>=0 and high>=0 else 0
            if 1<=marker<=len(segments):
                captures[segments[marker-1]['label']].extend(audio)
    finally:
        if p.poll() is None:
            p.stdin.write(bytes([3,0,0,0]));p.stdin.flush()
            try:p.wait(timeout=3)
            except subprocess.TimeoutExpired:
                p.kill();p.wait()
        diagnostics.close()
    results=[]
    for s in segments:
        data=captures[s['label']]
        pcm=array.array('h');pcm.frombytes(data)
        assert pcm, f'{s["label"]}: no visible capture marker'
        peak=max(abs(x) for x in pcm)
        rms=math.sqrt(sum(x*x for x in pcm)/len(pcm))
        assert peak>100 and rms>5,(s['label'],peak,rms)
        results.append(dict(label=s['label'],peak=peak,rms=round(rms,2),samples=len(pcm)//2))
    with wave.open(str(output/'native-all-sounds.wav'),'wb') as wav:
        wav.setparams((2,2,48000,0,'NONE','not compressed'));wav.writeframes(all_audio)
    (output/'capture-results.json').write_text(json.dumps(results,indent=2)+'\n')
    print(f'PASS: native mGBA produced non-silent PCM for all18 SFX +5 songs, no emulator audio overflow. Artifacts: {output}')


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--gba',action='store_true',help='Build an isolated fixture from current assets and capture native mGBA PCM')
    args=parser.parse_args()
    check_source()
    check_wrappers()
    if args.gba: check_gba()

if __name__=='__main__': main()
