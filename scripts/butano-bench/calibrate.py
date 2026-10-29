#!/usr/bin/env python3
"""Measure the F/1 stopwatch's zero-work control, including two wait/IRQ hooks."""
import json, os, pathlib, shutil
import run as suite

def main():
    work=pathlib.Path('/private/tmp/mtile-butano-bench');p=work/'butano-project';out=pathlib.Path('/private/tmp/mtile-butano-calibration');out.mkdir(exist_ok=True)
    suite.ENV=os.environ.copy();suite.ENV.update(DEVKITPRO='/opt/devkitpro',DEVKITARM='/opt/devkitpro/devkitARM');suite.ENV['PATH']='/opt/devkitpro/devkitARM/bin:/opt/devkitpro/tools/bin:'+suite.ENV['PATH']
    original=(p/'src/main.cpp').read_text()
    code='''#include "bn_core.h"
extern "C" void bench_start();
extern "C" unsigned bench_finish();
extern "C" void bench_pause();
extern "C" void bench_resume();
extern "C" void bench_irq_start();
extern "C" void bench_irq_end();
int main(){bn::core::init();bn::core::set_skip_frames(1);volatile unsigned* m=(volatile unsigned*)0x0203fe00;
for(unsigned tick=1;;++tick){unsigned best=0xffffffff;
for(int sample=0;sample<64;++sample){bench_start();
bench_pause();bench_irq_start();bench_irq_end();bench_resume();
bench_pause();bench_irq_start();bench_irq_end();bench_resume();
unsigned cost=bench_finish();if(cost<best)best=cost;}
bn::core::update();m[2]=tick*2+1;m[0]=0x44554c47;m[1]=0x02000001;m[3]=tick;m[5]=best;m[17]=0;m[2]=tick*2+2;}}
'''
    try:
        suite.write(p/'src/main.cpp',code);suite.run(['make','-j4'],cwd=p,log=out/'build.log');shutil.copyfile(p/'benchmark.gba',out/'calibration.gba')
        suite.run([work/'driver','capture',suite.ROOT/'build/player/mtile-player',out/'calibration.gba',out/'samples.json',out/'image.png',2,20,0])
        data=json.loads((out/'samples.json').read_text());suite.dump(out/'result.json',dict(stopwatchControlCycles=min(s['cycles'] for s in data['samples']),description='Zero useful work; two pause/resume boundaries and two empty interrupt hooks. No subtraction applied to engine results.'))
    finally:suite.write(p/'src/main.cpp',original)
if __name__=='__main__':main()
