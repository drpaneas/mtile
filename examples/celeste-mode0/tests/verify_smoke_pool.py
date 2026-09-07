#!/usr/bin/env python3
"""Prove that table recycling preserves smoke state, rendering and RNG order.

The baseline constructor below is the pre-pool implementation, using the same
readable constants. Only pooling is removed in a temporary project copy.
"""
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[3]
TESTS = Path(__file__).resolve().parent
OUT = Path(tempfile.mkdtemp(prefix='celeste-smoke-pool-'))
BASELINE = '''function effect_smoke(x, y)
  local velocity = 0.3 + rnd(0.2)
  add(smoke_particles, {
    x = x - 1 + rnd(2), y = y - 1 + rnd(2),
    vx = velocity, vy = -0.1, rx = 0, ry = 0,
    sprite = 29, flip_x = rnd() < 0.5, flip_y = rnd() < 0.5
  })
end
'''
env = dict(os.environ)
env.setdefault('GOCACHE', str(Path(tempfile.gettempdir()) / 'mtile-celeste-go-cache'))
for variant in ('baseline', 'pooled'):
    project = OUT / variant
    shutil.copytree(ROOT / 'examples/celeste', project,
                    ignore=shutil.ignore_patterns('tests', '*.gba', '*.elf', '*.sav', '__pycache__'))
    if variant == 'baseline':
        file = project / 'src/presentation.lua'
        text = file.read_text()
        start, end = text.index('function effect_smoke('), text.index('\nfunction update_smoke()')
        text = text[:start] + BASELINE + text[end:]
        start, end = text.index('function clear_smoke()'), text.index('\nfunction effect_smoke(')
        text = text[:start] + 'function clear_smoke()\n  smoke_particles = {}\nend\n' + text[end:]
        text = text.replace('      add(smoke_pool, p)\n', '')
        file.write_text(text)
    result = subprocess.run(['go', 'run', str(TESTS / 'gba_probe_build.go'), str(project), str(project / 'game.gba')],
                            cwd=ROOT, env=env, text=True, capture_output=True, check=True)
    print(result.stdout, end='', flush=True)
    work = Path(re.search(r'^workdir (.+)$', result.stdout, re.M)[1])
    shutil.copyfile(TESTS / 'physics_parity_test.go.tmpl', work / 'physics_parity_test.go')
    shutil.copyfile(TESTS / 'smoke_pool_test.go.tmpl', work / 'smoke_pool_test.go')
    output = OUT / (variant + '.json')
    subprocess.run(['go', 'test', '-run', '^TestSmokeTrace$', '-count=1', '.'], cwd=work,
                   env=dict(env, SMOKE_TRACE_OUT=str(output)), check=True)
left = json.loads((OUT / 'baseline.json').read_text())
right = json.loads((OUT / 'pooled.json').read_text())
assert len(left['Rows']) == len(right['Rows']) == 500
for a, b in zip(left['Rows'], right['Rows']):
    assert a == b, ('smoke state/pixel mismatch', a['Tick'], a, b)
assert left['NextRandom'] == right['NextRandom'], 'smoke pool changed RNG consumption'
print('Smoke pool: 500 exact active-state/order/count snapshots and framebuffer hashes; identical next RNG value.')
print('Includes 14 moving collections, respawn clearing, new-game clearing, and an uncapped burst (peak13 active particles).')
print('Smoke recycling evidence:', OUT)
