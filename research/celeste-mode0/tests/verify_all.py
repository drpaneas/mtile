#!/usr/bin/env python3
"""Verify this tile-only candidate, optionally including native GBA capture.

The copied original port's framebuffer presentation/audio experiments are
historical; they are not evidence for this candidate. This runner invokes
only checks that explicitly exercise research/celeste-mode0.
"""
import argparse
import os
from pathlib import Path
import subprocess
import sys

TESTS = Path(__file__).resolve().parent
ROOT = TESTS.parents[2]
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--gba', action='store_true', help='Also compare paired ROMs in native mGBA; requires the local player helper')
args = parser.parse_args()

def run(*command, env=None):
    print('\n>', ' '.join(map(str, command)), flush=True)
    subprocess.run([str(arg) for arg in command], cwd=ROOT, check=True, env=env)

for script in ('verify_sprite_effects.py', 'verify_tile_helpers.py',
               'verify_physics.py'):
    run(sys.executable, TESTS / script)
if args.gba:
    env = dict(os.environ, MTILE_CELESTE_MODE0_NATIVE='1')
    run('go', 'test', './cmd/renderbench', '-tags=runtimeperfdiagnostic',
        '-run', '^TestCelesteMode0NativeDiagnostic$', '-count=1', '-timeout=15m', '-v', env=env)
print('\nTile-only Celeste checks passed.')
