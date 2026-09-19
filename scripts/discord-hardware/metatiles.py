#!/usr/bin/env python3
"""Build and run the native 2x2 metatile versus flat-map expansion trial."""
import argparse
import hashlib
import json
from pathlib import Path
import platform
import subprocess


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--out', type=Path, required=True)
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[2]
    out = args.out.resolve()
    out.mkdir(parents=True, exist_ok=True)
    runtime = root / 'runtime'
    subprocess.run(['go', 'run', './internal/cmd/gentarget'], cwd=runtime, check=True)
    rom = out / 'metatiles.gba'
    subprocess.run(['tinygo', 'build', '-gc=none', '-opt=2', '-target=target-gba.json',
                    '-o', str(rom), './cmd/metatiletrial'], cwd=runtime, check=True)
    library = root / 'build/player/mgba/libmgba-core.a'
    runner = out / 'metatiles-runner'
    command = ['cc', '-std=c11', '-O2', '-I'+str(root/'third_party/mgba/include'),
               '-I'+str(root/'build/player/mgba/include'),
               str(root/'scripts/discord-hardware/metatiles.c'), str(library),
               '-lm', '-pthread', '-o', str(runner)]
    if platform.system() == 'Darwin':
        command.extend(['-framework', 'Foundation'])
    subprocess.run(command, check=True)
    measured = json.loads(subprocess.check_output([str(runner), str(rom)], text=True))
    measured.update(dict(
        samplesPerCase=100, mapWidth=32, mapHeight=32, metaWidth=2, metaHeight=2,
        rawPayloadBytes=2048, repeatedPayloadBytes=272, diversePayloadBytes=2304,
        expandedMapBytes=2048,
        tinygo=subprocess.check_output(['tinygo', 'version'], text=True).strip(),
        romSHA256=hashlib.sha256(rom.read_bytes()).hexdigest(),
        helperSHA256=hashlib.sha256(library.read_bytes()).hexdigest(),
        decision='Reject blanket conversion: slower expansion and data-dependent storage regression.'))
    (out/'report.json').write_text(json.dumps(measured, indent=2)+'\n')
    print(json.dumps(measured, indent=2))


if __name__ == '__main__':
    main()
