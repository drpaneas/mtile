#!/usr/bin/env python3
"""Explore equal-output Lua authoring/placement variants, not engine speedups.

Uses the unchanged shipping compiler/runtime and checks every captured tick.
The ordinary global-table source and Butano baseline remain separate records.
"""
import argparse
import json
import os
import pathlib
import re
import shutil
import statistics
import subprocess

import run as suite
import verify

CASES = [
    ('physics_64', 'local'),
    ('physics_64', 'local_iwram'),
    ('physics_64', 'global_iwram'),
    ('physics_128', 'local_iwram'),
    ('physics_drag_64', 'local_iwram'),
    ('physics_32', 'local_iwram'),
    ('physics_128', 'packed_iwram'),
    ('physics_128', 'interleaved_iwram'),
    ('physics_128', 'blocked_iwram'),
    ('scroll_chunk_cross', 'windowed'),
]


def variant_source(source, variant, entities):
    if variant == 'windowed':
        old = 'flr((frame*1*9)/8),flr((frame*3)/8),0,0,31,21'
        new = 'flr((frame*1*9)/256)*32,flr((frame*3)/64)*8,0,0,64,32'
        assert old in source
        return source.replace(old, new)
    if variant != 'global_iwram':
        for name in ('px', 'py', 'vx', 'vy'):
            source = source.replace('\n'+name+'={', '\nlocal '+name+'={')
    if variant.endswith('_iwram'):
        source = source.replace('function _update()', 'function simulate()')
        source += '\nfunction _update() simulate() end\n'
    if variant not in ('packed_iwram', 'interleaved_iwram', 'blocked_iwram'):
        return source
    assert entities == 128
    if variant != 'blocked_iwram':
        source = re.sub(r'^local (px|py|vx|vy)=\{[^\n]+\}\n', '', source, flags=re.M)
        header = ''.join('local '+name+'={'+','.join('0' for _ in range(256))+'}\n'
                         for name in ('positions', 'velocities'))
        if variant == 'interleaved_iwram':
            source = source.replace('for i=1,128 do', 'for i=1,128 do\nlocal k=i*2-1')
            slots = ('positions[k]', 'positions[k+1]', 'velocities[k]', 'velocities[k+1]')
        else:
            slots = ('positions[i]', 'positions[i+128]', 'velocities[i]', 'velocities[i+128]')
        for old, new in zip(('px', 'py', 'vx', 'vy'), slots):
            source = source.replace(old+'[i]', new)
        return header+source
    init = source.split('function _init()\n', 1)[1].split('\nend\nfunction simulate()', 1)[0]
    update = source.split('function simulate()\n', 1)[1].split('\nend\nfunction _draw()', 1)[0]
    init_body = init.split('for i=1,128 do\n', 1)[1].removesuffix('\nend')
    update_prefix, update_body = update.split('for i=1,128 do\n', 1)
    update_body = update_body.removesuffix('\nend')

    def block(body, array, offset, initializing=False):
        for index, name in enumerate(('px', 'py', 'vx', 'vy')):
            slot = array+'[k'+('+'+str(index) if index else '')+']'
            body = body.replace(name+'[i]', slot)
        prefix = f'local i=(k-1)/4+{offset+1}\n' if initializing else ''
        return 'for k=1,253,4 do\n'+prefix+body+'\nend\n'

    header = ''.join('local '+name+'={'+','.join('0' for _ in range(256))+'}\n'
                     for name in ('state_a', 'state_b'))+'tick=0\nchecksum=0\n'
    return (header+'function _init()\n'+block(init_body, 'state_a', 0, True)
            +block(init_body, 'state_b', 64, True)+'end\nfunction simulate()\n'
            +update_prefix+block(update_body, 'state_a', 0)+block(update_body, 'state_b', 64)
            +'end\nfunction _draw()'+source.split('function _draw()', 1)[1])


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('--work', type=pathlib.Path, default=pathlib.Path('/private/tmp/mtile-butano-parity'))
    ap.add_argument('--only', default='')
    ap.add_argument('--verify-only', action='store_true', help='Recheck existing captures and source hashes without rebuilding.')
    args = ap.parse_args()
    args.work.mkdir(parents=True, exist_ok=True)
    suite.ENV = os.environ.copy()
    suite.ENV['GOCACHE'] = str(args.work/'go-cache')
    driver = args.work/'driver'
    if not args.verify_only:
        suite.run(['go', 'build', '-o', driver, suite.HERE/'driver.go'], cwd=suite.ROOT)
    path = args.work/'results.json'
    results = json.loads(path.read_text()) if path.exists() else {}
    workloads = {w['name']: w for w in suite.workloads()}
    checks = []
    for name, variant in CASES:
        key = name+'-'+variant
        if args.only and not re.search(args.only, key):
            continue
        w = workloads[name]
        project = args.work/'projects'/key
        out = args.work/key
        out.mkdir(exist_ok=True)
        if not args.verify_only:
            original = suite.generate(w, args.work/'inputs')
            shutil.copytree(original, project, dirs_exist_ok=True)
            text = variant_source((original/'game.lua').read_text(), variant, w['entities'])
            (project/'game.lua').write_text(text)
            if variant.endswith('_iwram'):
                config = json.loads((project/'project.json').read_text())
                config['iwramFunctions'] = ['simulate']
                (project/'project.json').write_text(json.dumps(config)+'\n')
        rom = out/'mode0.gba'
        print(key, flush=True)
        if not args.verify_only:
            suite.run([driver, 'build', project, 'mode0', rom], log=out/'build.log')
        else:
            assert suite.sha(rom) == results[key]['romSHA256']
            assert suite.sha(project/'game.lua') == results[key]['sourceSHA256']
        expected_state = verify.checksums(w, 660)
        golden = {frame: verify.pixels(w, frame) for frame in range(120)}
        repetitions = []
        for rep in range(2):
            capture = out/f'samples-{rep}.json'
            if not args.verify_only:
                suite.run([driver, 'capture', suite.ROOT/'build/player/mtile-player', rom,
                           capture, out/'final.png', 60, 600, 0])
            data = json.loads(capture.read_text())
            assert len(data['samples']) == 600
            for sample in data['samples']:
                watches = {x['Name']: x['Raw'] for x in sample['watches']}
                assert sample['sha256'] == golden[sample['tick'] % 120], (key, rep, sample['tick'], 'pixels')
                assert watches['tick'] == sample['tick']*65536
                assert watches['checksum'] == expected_state[sample['tick']]*65536, (key, rep, sample['tick'], 'state')
            summary = suite.summarize(data)
            summary['mean'] = statistics.mean(s['cycles'] for s in data['samples'])
            repetitions.append(summary)
            checks.append(dict(case=key, repetition=rep, ticks=600, pixelMismatches=0, stateChecksumMismatches=0))
        meta = json.loads(pathlib.Path(str(rom)+'.build.json').read_text())
        result = dict(summary=repetitions[0], repeatMedians=[r['median'] for r in repetitions],
                      iwramUsed=meta['IWRAMUsed'], ewramUsed=meta['EWRAMUsed'], romBytes=rom.stat().st_size,
                      romSHA256=suite.sha(rom), sourceSHA256=suite.sha(project/'game.lua'), verifiedTicks=1200)
        assert len(set(result['repeatMedians'])) == 1
        if variant.endswith('_iwram'):
            elf = pathlib.Path(str(rom)+'.game.elf')
            symbols = subprocess.check_output(['/opt/devkitpro/devkitARM/bin/arm-none-eabi-nm', '-n', elf], text=True)
            placed = [parts for line in symbols.splitlines() if len(parts := line.split()) == 3
                      and parts[2] == '(*main.Game).simulate']
            assert len(placed) == 1 and 0x03000000 <= int(placed[0][0], 16) < 0x03007fa0
            result['helperAddress'] = placed[0][0]
        results[key] = result
        suite.dump(path, results)
        print(json.dumps(result), flush=True)
    suite.dump(args.work/'verification.json', dict(checks=checks, totalTicks=sum(c['ticks'] for c in checks), passAll=True))


if __name__ == '__main__':
    main()
