#!/usr/bin/env python3
"""Bound stock mtile sine error against every native PICO-8 16.16 phase."""
import argparse
import json
import os
from pathlib import Path
import re
import shutil
import struct
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[3]
p = argparse.ArgumentParser(description=__doc__)
p.add_argument('--build-dir', type=Path, required=True)
args = p.parse_args()
out = Path(tempfile.mkdtemp(prefix='celeste-source-math-'))
cart = out / 'sine.p8'
cart.write_text('pico-8 cartridge // http://www.pico-8.com\nversion 42\n__lua__\n'
                'for h=0,1 do for i=0,32767 do '
                'printh(tostr(sin(i*0.0000152587890625+h*0.5),true)) end end\n')
pico = os.environ.get('CELESTE_PICO8_BIN') or shutil.which('pico8') or '/Applications/PICO-8.app/Contents/MacOS/pico8'
run = subprocess.run([pico, '-home', str(out), '-x', str(cart)], check=True,
                     env=dict(os.environ, SDL_VIDEODRIVER='dummy', SDL_AUDIODRIVER='dummy'),
                     text=True, capture_output=True, timeout=20)
values = [int(line[2:].replace('.', ''), 16) for line in run.stdout.splitlines() if line.startswith('0x')]
assert len(values) == 65536, 'native oracle incomplete'
(out / 'sine.bin').write_bytes(struct.pack('<65536I', *values))
shutil.copyfile(Path(__file__).with_name('physics_parity_test.go.tmpl'), args.build_dir / 'physics_parity_test.go')
(args.build_dir / 'source_math_test.go').write_text(r'''package main
import (
 "encoding/binary"
 "os"
 "testing"
 rt "github.com/drpaneas/mtile/runtime"
)
func TestSourceMathFullPhase(t *testing.T) {
 data,err:=os.ReadFile(os.Getenv("CELESTE_SINE_ORACLE")); if err!=nil {t.Fatal(err)}
 if len(data)!=65536*4 {t.Fatal("incomplete oracle")}
 mismatches,maxError:=0,int32(0)
 for phase:=0; phase<65536; phase++ {
  want:=int32(binary.LittleEndian.Uint32(data[phase*4:]))
  for _,cycle:=range []int{0,-65536,65536} {
   got:=rt.Number(phase+cycle).Sin()
   delta:=int32(got)-want;if delta<0{delta=-delta}
   if delta>15 {t.Fatalf("phase%d cycle%d error%d exceeds15 raw units",phase,cycle,delta)}
   if phase%8192==0 && delta!=0 {t.Fatalf("principal direction phase%d must remain exact",phase)}
   if cycle==0 {if delta!=0 {mismatches++};if delta>maxError{maxError=delta}}
  }
  wantCos:=int32(binary.LittleEndian.Uint32(data[((phase-16384)&65535)*4:]))
  got:=rt.Number(phase).Cos();delta:=int32(got)-wantCos;if delta<0{delta=-delta}
  if delta>15 || (phase%8192==0 && delta!=0) {t.Fatalf("cos phase%d got%d want%d",phase,got,wantCos)}
 }
 t.Logf("262144 native-PICO8 comparisons: principal directions exact; maximum error %d/65536, %d/65536 sine phases nonexact",maxError,mismatches)
}
''')
subprocess.run(['go', 'test', '-run', '^TestSourceMath', '-v', '.'], cwd=args.build_dir,
               env=dict(os.environ, CELESTE_SINE_ORACLE=str(out / 'sine.bin')), check=True)
print('Native math oracle:', out)
