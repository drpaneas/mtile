#!/usr/bin/env python3
"""Replay the proven level route on GBA/mGBA and compare source-oracle state."""
from pathlib import Path
import argparse
import json
import os
import re
import shutil
import select
import statistics
import struct
import subprocess
import tempfile
import zlib

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--reference-dir', type=Path, default=os.environ.get('CELESTE_PARITY_DIR'), help='Native PICO-8 output from verify_physics.py')
parser.add_argument('--max-cycles', type=int, default=0, help='Optional route frame-cycle limit, excluding initial spawn and Start reload; 0 reports measurements')
parser.add_argument('--game-source', type=Path, help='Use an alternate game.lua with current modules/runtime for paired performance comparisons')
parser.add_argument('--project', type=Path, help='Copy an alternate complete project for paired engine comparisons')
args = parser.parse_args()
if not args.reference_dir:
    raise SystemExit('Run verify_physics.py --gba, or supply --reference-dir from its output.')
root = Path(__file__).resolve().parents[3]
out = Path(tempfile.mkdtemp(prefix='mtile-celeste-gba-'))
project = out / 'project'
shutil.copytree(args.project or root / 'examples/celeste', project, ignore=shutil.ignore_patterns('tests', '*.gba', '*.elf', '*.map', '__pycache__'))
if args.game_source:
    shutil.copyfile(args.game_source, project / 'game.lua')
route = json.loads((root / 'examples/celeste/tests/level_completion_route.json').read_text())['inputs']
reference = {r['tick']: r['state'] for r in json.loads((Path(args.reference_dir) / 'reference-traces.json').read_text()) if r['case'] == 'local_complete_level'}
assert len(reference) == len(route) and reference[len(route)]['exited'], 'missing successful native PICO-8 route reference'

# Deterministic input enters the original controller once per game update.
# This avoids a host LCD frame arriving before/after the GBA's input poll.
# The production example is untouched; only this temporary instrumented copy changes.
for file in [project / 'game.lua', *(project / 'src').glob('*.lua')]:
    text = file.read_text()
    text = re.sub(r'\bbtn\(', 'probe_btn(', text)
    if file.name == 'game.lua':
        text = text.replace('function _update()', 'function probe_game_update()')
    file.write_text(text)
probe = '''p_x=0
p_y=0
p_vx=0
p_vy=0
p_freeze=0
p_dead=false
p_spawn=true
p_done=false
p_step=0
p_tick=0
p_stage=0
p_hair=0
probe_keys=0
probe_route={ROUTE}

function probe_btn(button)
 return band(probe_keys, shl(1,button)) ~= 0
end
'''.replace('ROUTE', ','.join(map(str, route)))
wrappers = '''
function _update()
 p_tick=p_tick+1
 probe_keys=0
 if p_stage==0 and not spawning then p_stage=1 end
 if p_stage==1 then
  p_step=p_step+1
  probe_keys=probe_route[p_step] or 0
  if p_step==#probe_route+3 then
   probe_keys=64
   p_stage=2
  end
 end
 probe_game_update()
 p_x=player.x p_y=player.y p_vx=player.vx p_vy=player.vy
 p_freeze=freeze p_dead=player.dead p_spawn=spawning p_done=level_complete
 p_hair=#player.hair
end
'''
game = project / 'game.lua'
(project / 'src/probe.lua').write_text(probe)
game.write_text('require("probe")\n' + game.read_text() + wrappers)
env = dict(os.environ)
env.setdefault('GOCACHE', str(Path(tempfile.gettempdir()) / 'mtile-celeste-go-cache'))
rom = out / 'probe.gba'
subprocess.run(['go', 'run', str(Path(__file__).with_name('gba_probe_build.go')), str(project), str(rom)], cwd=root, env=env, check=True)
stderr_file = open(out / 'mgba-stderr.log', 'wb')
p = subprocess.Popen([str(root / 'build/player/mtile-player'), str(rom)], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=stderr_file)

def read(size):
    result = bytearray()
    while len(result) < size:
        if not select.select([p.stdout], [], [], 20)[0]:
            raise TimeoutError('mGBA stopped responding; see ' + str(out / 'mgba-stderr.log'))
        part = os.read(p.stdout.fileno(), size - len(result))
        if not part:
            raise RuntimeError('emulator exited; see ' + str(out / 'mgba-stderr.log'))
        result.extend(part)
    return bytes(result)

def step():
    p.stdin.write(struct.pack('<BBH', 1, 0, 0)); p.stdin.flush()
    header = read(20)
    assert header[:4] == b'FRAM'
    frame, audio_count, overflow, telemetry_size = struct.unpack('<4I', header[4:])
    assert not overflow, 'native audio overflow'
    rgba = read(240 * 160 * 4)
    audio = read(audio_count * 4)
    telemetry = read(telemetry_size)
    mailbox = telemetry[40:]
    if mailbox[:4] != b'GLUD':
        return rgba, audio, None
    sequence = struct.unpack_from('<I', mailbox, 8)[0]
    if sequence & 1:
        return rgba, audio, None
    watches = {}
    for i in range(mailbox[68]):
        at = 72 + i * 24
        name = mailbox[at:at+16].split(b'\0')[0].decode()
        kind = mailbox[at+16]
        raw = struct.unpack_from('<i', mailbox, at+20)[0]
        watches[name] = bool(raw) if kind == 1 else raw
    stats = dict(zip('frames missed frame max_frame update max_update draw max_draw gc last_gc max_gc blit'.split(), struct.unpack_from('<12I', mailbox, 12)))
    stats['heap_error'] = struct.unpack_from('<I', mailbox, 480)[0]
    return rgba, audio, dict(sequence=sequence, watches=watches, stats=stats)

def png(path, rgba):
    def chunk(kind, data):
        return struct.pack('>I', len(data)) + kind + data + struct.pack('>I', zlib.crc32(kind + data))
    pixels = b''.join(b'\0' + rgba[y*240*4:(y+1)*240*4] for y in range(160))
    path.write_bytes(b'\x89PNG\r\n\x1a\n' + chunk(b'IHDR', struct.pack('>2I5B',240,160,8,6,0,0,0)) + chunk(b'IDAT',zlib.compress(pixels)) + chunk(b'IEND',b''))

rows = []
velocity_drift = {"p_vx":0,"p_vy":0}
checked = set()
last_sequence = 0
last_tick = 0
complete = False
restarted = False
audible = False
try:
    assert read(16)[:4] == b'GLUP'
    for lcd_frame in range(6000):
        rgba, audio, snapshot = step()
        audible = audible or any(audio)
        if not snapshot or snapshot['sequence'] == last_sequence:
            continue
        last_sequence = snapshot['sequence']
        w, stats = snapshot['watches'], snapshot['stats']
        assert 'p_tick' in w and 'p_hair' in w, f'probe watches missing: {w}'
        tick = w['p_tick'] // 65536
        if last_tick:
            assert tick == last_tick + 1, (last_tick, tick, 'target publication skipped')
        last_tick = tick
        assert not stats['heap_error'], ('GBA heap error', stats)
        assert w['p_hair'] == 5*65536, ('GBA hair lifecycle corruption', w)
        rows.append(snapshot)
        if w['p_stage'] == 65536:
            index = w['p_step'] // 65536
            if index in reference:
                expected = reference[index]
                for probe_name, source_name in [('p_x','x'),('p_y','y'),('p_vx','vx'),('p_vy','vy'),('p_freeze','freeze'),('p_dead','dead'),('p_done','exited')]:
                    limit = {'p_vx': 1, 'p_vy': 15}.get(probe_name, 0)
                    delta = abs(w[probe_name] - expected[source_name])
                    assert delta <= limit, (index, probe_name, w[probe_name], expected[source_name], 'limit',limit)
                    if probe_name in velocity_drift: velocity_drift[probe_name] = max(velocity_drift[probe_name],delta)
                assert not w['p_spawn'], ('route unexpectedly respawned',index)
                checked.add(index)
            if w['p_done'] and not complete:
                complete = True
                png(out / 'level-complete.png', rgba)
        elif w['p_stage'] == 2*65536:
            assert complete and not w['p_done'] and w['p_spawn'], ('Start failed to restart',w)
            restarted = True
            png(out / 'restarted.png', rgba)
            break
    assert len(checked) == len(route), (len(checked),len(route),'route samples missing')
    assert complete and restarted, 'native GBA completion/restart not observed'
    assert audible, 'GBA emitted no audible samples during spawn/dash/jump/completion'
    worst = max(row['stats']['frame'] for row in rows[5:])
    gameplay = [row['stats'] for row in rows if row['watches']['p_stage'] == 65536 and row['watches']['p_step'] <= len(route)*65536]
    (out / 'target-traces.json').write_text(json.dumps(rows,indent=2))
    print(f'Native mGBA: {len(checked)} route updates retain exact PICO-8 positions/discrete states, successful completion and Start restart; hair stable, sound samples active.')
    print('Maximum raw16.16 velocity drift (explicit limits vx1/vy15):',velocity_drift)
    print(f'Target worst frame after warmup: {worst} cycles ({worst/561792*100:.1f}% of 30 Hz budget). Probe overhead is included.')
    print('Route cycles, median/max: ' + ', '.join(f'{key} {statistics.median(s[key] for s in gameplay):.0f}/{max(s[key] for s in gameplay)}' for key in ('frame','update','draw')))
    over_budget = sum(s['frame'] > 561792 for s in gameplay)
    print(f'Route ticks above the 30 Hz budget: {over_budget}/{len(gameplay)}. Startup/reload peaks are reported separately above.')
    print('Native GBA traces and screenshots:',out)
    if args.max_cycles:
        route_worst = max(s['frame'] for s in gameplay)
        assert route_worst <= args.max_cycles, (route_worst,args.max_cycles,'target route frame budget exceeded')
finally:
    if p.poll() is None:
        p.stdin.write(bytes([3,0,0,0])); p.stdin.flush()
        try:
            p.wait(timeout=3)
        except subprocess.TimeoutExpired:
            p.kill()
            p.wait()
    stderr_file.close()
