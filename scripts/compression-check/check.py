#!/usr/bin/env python3
"""Measure bounded BIOS LZ77 decoding versus raw asset copies in native mGBA."""
import argparse
import hashlib
import json
from pathlib import Path
import platform
import shutil
import struct
import subprocess
import tempfile

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]


def compress(raw):
    result = bytearray([0x10, len(raw) & 255, (len(raw) >> 8) & 255, (len(raw) >> 16) & 255])
    position = 0
    while position < len(raw):
        flag_index = len(result)
        result.append(0)
        for bit in range(7, -1, -1):
            if position == len(raw):
                break
            match, length = -1, 0
            for count in range(min(18, len(raw) - position), 2, -1):
                # The BIOS supports overlapping backreferences. Searching up to
                # position+count-1 admits these while excluding position itself.
                match = raw.rfind(raw[position:position+count], max(0, position-4096), position+count-1)
                if match >= 0:
                    length = count
                    break
            if match >= 0:
                result[flag_index] |= 1 << bit
                distance = position - match - 1
                result.extend((((length-3) << 4) | (distance >> 8), distance & 255))
                position += length
            else:
                result.append(raw[position])
                position += 1
    return bytes(result)


def make_data(destination):
    arrays = []

    def emit(name, data):
        padded = data + bytes((-len(data)) % 4)
        words = struct.unpack('<' + 'I' * (len(padded) // 4), padded)
        arrays.append('//go:section .rodata\nvar ' + name + ' = [...]uint32{' + ','.join(hex(w) for w in words) + '}\n')
        return 'bytesOf(' + name + '[:])[:' + str(len(data)) + ']'

    random = []
    seed = 0xDEADBEEF
    for _ in range(16384):
        seed ^= (seed << 13) & 0xFFFFFFFF
        seed ^= seed >> 17
        seed ^= (seed << 5) & 0xFFFFFFFF
        random.append(seed & 255)
    redundant = bytes(((i % 64) // 8 + (i % 8) + ((i // 256) % 4)) % 16 for i in range(16384))
    fixtures, metadata = [], []
    for name, raw in [('redundant', redundant), ('incompressible', bytes(random))]:
        packed = compress(raw)
        frames = [compress(raw[i:i+256]) for i in range(0, len(raw), 256)]
        fixtures.append('{raw:' + emit(name+'Raw', raw) + ',packed:' + emit(name+'Packed', packed) + ',frames:[64][]byte{' + ','.join(emit(name+'Frame'+str(i), frame) for i, frame in enumerate(frames)) + '}}')
        metadata.append({'name': name, 'expanded_bytes': len(raw), 'whole_bytes': len(packed), 'whole_padded_bytes': (len(packed)+3)&~3, 'frame_bytes': sum(map(len, frames)), 'frame_padded_bytes': sum((len(frame)+3)&~3 for frame in frames), 'largest_frame_bytes': max(map(len, frames))})
    malformed = [
        ('signature', bytes([0x11, 1, 0, 0, 0, 1])),
        ('zero_size', bytes([0x10, 0, 0, 0])),
        ('truncated_header', bytes([0x10, 1, 0])),
        ('truncated_literal', bytes([0x10, 1, 0, 0, 0])),
        ('truncated_token', bytes([0x10, 4, 0, 0, 0x80, 0])),
        ('negative_backreference', bytes([0x10, 3, 0, 0, 0x80, 0, 0])),
        ('overrun_token', bytes([0x10, 4, 0, 0, 0x40, 65, 0xF0, 0])),
    ]
    bad = [emit('bad'+str(i), data) for i, (_, data) in enumerate(malformed)]
    destination.write_text('//go:build tinygo\npackage main\n' + ''.join(arrays) + '\nvar fixtureData = [2]fixture{' + ','.join(fixtures) + '}\nvar malformedData = [7][]byte{' + ','.join(bad) + '}\n')
    return metadata, [name for name, _ in malformed]


def run(args, out):
    runtime = out / 'runtime'
    shutil.copytree(args.runtime, runtime, ignore=shutil.ignore_patterns('*.elf', '*.gba', '*.map', '*.allocs', 'target-gba*.json'))
    fixture = runtime / 'cmd/lztrial'
    fixture.mkdir(parents=True)
    shutil.copy2(HERE / 'fixture.go', fixture / 'main.go')
    sizes, malformed = make_data(fixture / 'data.go')
    with (runtime / 'crt0/gba.s').open('a') as f:
        f.write('''\n.section .text.trial_lz77,"ax",%progbits
.arm
.align 2
.global trial_lz77
.type trial_lz77,%function
trial_lz77:
 swi 0x110000
 bx lr
''')
    subprocess.run(['go', 'run', './internal/cmd/gentarget'], cwd=runtime, check=True)
    elf, rom = out / 'trial.elf', out / 'trial.gba'
    subprocess.run(['tinygo', 'build', '-gc=none', '-opt=2', '-target=target-gba.json', '-o', str(elf), './cmd/lztrial'], cwd=runtime, check=True)
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
    if len(values) != 38 or values[0] != 0xD4A70001:
        raise ValueError('missing or incomplete target telemetry')
    failures = 0
    for index, data in enumerate(sizes):
        i = 1 + index * 15
        data['raw_copy_cycles'] = values[i]
        data['validation_cycles'] = values[i+2]
        data['validated_expanded_bytes'] = values[i+3]
        data['bios_decode_cycles'] = values[i+4]
        data['validated_decode_cycles'] = values[i+6]
        data['frame_min_cycles'] = values[i+8]
        data['frame_max_cycles'] = values[i+9]
        data['all_frames_cycles'] = values[i+10]
        data['all_raw_frames_cycles'] = values[i+12]
        data['parity_guard_mismatches'] = sum(values[i+x] for x in [1, 5, 7, 11])
        data['undersized_destination_failures'] = values[i+13] + values[i+14]
        failures += int(data['validated_expanded_bytes'] != data['expanded_bytes']) + data['parity_guard_mismatches'] + data['undersized_destination_failures']
    malformed_results = {name: values[31+i] for i, name in enumerate(malformed)}
    failures += sum(malformed_results.values())
    report = {'environment': 'native mGBA HLE BIOS; forced blank; ROM compressed source, EWRAM expanded destination; no physical hardware measurements', 'sizes_and_cycles': sizes, 'malformed_acceptance_or_write_failures': malformed_results, 'failures': failures, 'tinygo': subprocess.check_output(['tinygo', 'version'], text=True).strip(), 'mgba_sha256': hashlib.sha256((args.mgba_build / 'mgba/libmgba-core.a').read_bytes()).hexdigest()}
    (out / 'report.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report, indent=2))
    print('Artifacts:', out)
    return int(failures != 0)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--runtime', type=Path, default=ROOT / 'runtime')
    parser.add_argument('--mgba-source', type=Path, default=ROOT / 'third_party/mgba')
    parser.add_argument('--mgba-build', type=Path, default=ROOT / 'build/player')
    parser.add_argument('--output-dir', type=Path)
    args = parser.parse_args()
    for key in ['runtime', 'mgba_source', 'mgba_build']:
        setattr(args, key, getattr(args, key).resolve())
    if args.output_dir:
        args.output_dir = args.output_dir.resolve()
        args.output_dir.mkdir(parents=True, exist_ok=False)
        return run(args, args.output_dir)
    with tempfile.TemporaryDirectory(prefix='mtile-compression-check-') as temporary:
        return run(args, Path(temporary))


if __name__ == '__main__':
    raise SystemExit(main())
