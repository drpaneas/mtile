#!/usr/bin/env python3
"""Run the target DMA/alignment regression and microbenchmark in native mGBA."""
import argparse
import hashlib
import json
import platform
from pathlib import Path
import shutil
import subprocess
import tempfile

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]


def run(args, out):
    runtime = out / 'runtime'
    shutil.copytree(args.runtime, runtime, ignore=shutil.ignore_patterns('*.elf', '*.gba', '*.map', '*.allocs', 'target-gba*.json'))
    fixture = runtime / 'cmd' / 'dmatrial'
    fixture.mkdir(parents=True, exist_ok=True)
    shutil.copy2(HERE / 'fixture.go', fixture / 'main.go')
    with (runtime / 'crt0/gba.s').open('a') as f:
        f.write('''\n.section .text.trial_fastset,"ax",%progbits
.arm
.align 2
.global trial_fastset
.type trial_fastset,%function
trial_fastset:
 swi 0x0c0000
 bx lr
''')
    subprocess.run(['go', 'run', './internal/cmd/gentarget'], cwd=runtime, check=True)
    elf, rom = out / 'trial.elf', out / 'trial.gba'
    subprocess.run(['tinygo', 'build', '-gc=none', '-opt=2', '-target=target-gba.json', '-o', str(elf), './cmd/dmatrial'], cwd=runtime, check=True)
    subprocess.run(['arm-none-eabi-objcopy', '-O', 'binary', str(elf), str(rom)], check=True)
    (out / 'trial.asm').write_bytes(subprocess.check_output(['arm-none-eabi-objdump', '-d', str(elf)]))
    executable = out / 'check'
    command = ['cc', '-std=c11', '-O2', '-I' + str(args.mgba_source / 'include'), '-I' + str(args.mgba_build / 'mgba/include'), str(HERE / 'check.c'), str(args.mgba_build / 'mgba/libmgba-core.a'), '-lm', '-pthread', '-o', str(executable)]
    if platform.system() == 'Darwin':
        command += ['-framework', 'Foundation']
    subprocess.run(command, check=True)
    output = subprocess.check_output([str(executable), str(rom)], text=True)
    (out / 'raw.txt').write_text(output)
    values = {int(fields[0]): int(fields[1]) for line in output.splitlines() if len(fields := line.split()) == 2 and all(x.isdigit() for x in fields)}
    if len(values) != 207 or values[0] != 0xD4A70001:
        raise ValueError('missing or incomplete target telemetry')
    report = {'environment': 'native mGBA, HLE BIOS, forced blank, EWRAM source; no physical hardware measurement', 'fill': [], 'copy': [], 'timer': [], 'counts': [], 'alignment': []}
    for offset in range(4):
        i = 1 + offset * 2
        report['fill'].append({'offset': offset, 'mismatches': values[i], 'cycles': values[i+1]})
    report['rejected_fill_mismatches'] = [values[i] for i in range(9, 13)]
    for kind, name in enumerate(['production_dma', 'explicit_4096_word_chunks', 'bios_cpufastset']):
        i = 13 + kind * 2
        report['copy'].append({'kind': name, 'cycles': values[i], 'mismatches': values[i+1]})
        for busy in range(2):
            for phase in range(8):
                i = 19 + busy * 48 + kind * 16 + phase * 2
                report['timer'].append({'kind': name, 'irq_body_cycles': busy * 16384, 'phase_cycles': phase * 8192, 'overflows': values[i], 'interrupts': values[i+1]})
    for kind, name in enumerate(['byte_slice', 'word_slice']):
        for j, count in enumerate([-1, 0, 65536, 9601, 1, 2, 7, 8, 31, 32, 4096, 4097, 9600]):
            i = 115 + kind * 26 + j * 2
            report['counts'].append({'kind': name, 'words': count, 'mismatches': values[i], 'cycles': values[i+1]})
    for source_offset in range(4):
        for dest_offset in range(4):
            i = 167 + (source_offset * 4 + dest_offset) * 2
            report['alignment'].append({'kind': 'byte_slice', 'source_offset': source_offset, 'dest_offset': dest_offset, 'mismatches': values[i], 'cycles': values[i+1]})
    for dest_offset in range(4):
        i = 199 + dest_offset * 2
        report['alignment'].append({'kind': 'word_slice', 'source_offset': 0, 'dest_offset': dest_offset, 'mismatches': values[i], 'cycles': values[i+1]})
    failures = sum(x['mismatches'] != 0 for group in ['fill', 'copy', 'counts', 'alignment'] for x in report[group])
    failures += sum(x != 0 for x in report['rejected_fill_mismatches'])
    failures += sum(x['overflows'] != x['interrupts'] for x in report['timer'] if x['kind'] == 'production_dma')
    report['failures'] = failures
    report['toolchain'] = {
        'tinygo': subprocess.check_output(['tinygo', 'version'], text=True).strip(),
        'assembler': subprocess.check_output(['arm-none-eabi-as', '--version'], text=True).splitlines()[0],
        'mgba_sha256': hashlib.sha256((args.mgba_build / 'mgba/libmgba-core.a').read_bytes()).hexdigest(),
        'input_sources': {name: hashlib.sha256((args.runtime / name).read_bytes()).hexdigest() for name in ['hw/fill_tinygo.go', 'hw/mmio_tinygo.go'] + (['hw/copy_bytes.go'] if (args.runtime / 'hw/copy_bytes.go').exists() else [])},
    }
    (out / 'report.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps({key: report[key] for key in ['environment', 'fill', 'copy', 'failures']}, indent=2))
    print('Artifacts:', out)
    return int(failures != 0)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--runtime', type=Path, default=ROOT / 'runtime')
    p.add_argument('--mgba-source', type=Path, default=ROOT / 'third_party/mgba')
    p.add_argument('--mgba-build', type=Path, default=ROOT / 'build/player')
    p.add_argument('--output-dir', type=Path)
    args = p.parse_args()
    for key in ['runtime', 'mgba_source', 'mgba_build']:
        setattr(args, key, getattr(args, key).resolve())
    if args.output_dir:
        args.output_dir = args.output_dir.resolve()
        args.output_dir.mkdir(parents=True, exist_ok=False)
        return run(args, args.output_dir)
    with tempfile.TemporaryDirectory(prefix='mtile-dma-check-') as temporary:
        return run(args, Path(temporary))


if __name__ == '__main__':
    raise SystemExit(main())
