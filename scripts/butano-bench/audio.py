#!/usr/bin/env python3
"""PSG characterization. Event clocks differ, so these are NOT ranked as
sample-identical music benchmarks. The common four-channel register program
and measured differences are retained explicitly.
"""
import json, os, pathlib, re, shutil, sys
import run as bench

def main():
    work=pathlib.Path('/private/tmp/mtile-butano-audio-work')
    out=pathlib.Path(sys.argv[1] if len(sys.argv)>1 else '/private/tmp/mtile-butano-audio')
    butano=pathlib.Path('/private/tmp/mtile-butano-20261029')
    bench.ENV=os.environ.copy();bench.ENV.update(DEVKITPRO='/opt/devkitpro',DEVKITARM='/opt/devkitpro/devkitARM',GOCACHE=str(work/'go-cache'))
    bench.ENV['PATH']='/opt/devkitpro/devkitARM/bin:/opt/devkitpro/tools/bin:'+bench.ENV['PATH']
    work.mkdir(exist_ok=True);out.mkdir(exist_ok=True);bench.setup(work,butano)
    p=work/'butano-project/Makefile';bench.write(p,p.read_text().replace('DMGAUDIOBACKEND\t:=  null','DMGAUDIOBACKEND\t:=  default'))
    driver=work/'driver';bench.run(['go','build','-o',driver,bench.HERE/'driver.go'],cwd=bench.ROOT)
    periods=[int(x) for x in re.findall(r'\d+', (bench.ROOT/'runtime/audio/native_pitch.go').read_text().split('var pulsePeriods = [152]uint16{')[1].split('}')[0])]
    sounds=json.loads((bench.ROOT/'examples/audio-gb-melody/assets/audio/sounds.json').read_text())['sounds'][:4]
    for s in sounds:
        if 'pulse' in s:s['pulse']['envelope']=dict(initial=8,direction='down',pace=0)
        if 'noise' in s:s['noise']['envelope']=dict(initial=8,direction='down',pace=0)
    waveform=sounds[2]['wave']['samples'];results=[]
    for interval in (8,1):
        name=f'psg_events_every_{interval}_ticks';print(name,flush=True)
        w=dict(name=name,kind=12,objects=0,size=8,layers=0,entities=0,mutations=0,spread=0,width=32,height=32,hud='',mode0=True,drag=False,speed=1,verticalSpeed=0)
        project=bench.generate(w,work);assets=project/'assets/audio'
        bench.dump(assets/'sounds.json',dict(version=2,sounds=sounds));bench.dump(assets/'settings.json',dict(version=2,profile='gameboy-psg',leftLevel=7,rightLevel=7,psgRatio=100))
        bench.dump(assets/'music.json',dict(version=2,ppqn=96,clips=[],parts=[],songs=[]))
        lua=f'''tick=0
checksum=0
audio_mean=0
audio_services=0
function _update()
 tick=tick+1
 if (tick-1)%{interval}==0 then
  local phase=flr((tick-1)/{interval})%4
  gba_note(1,60+phase*2,0,0)
  gba_note(2,48+phase*2,1,0)
  gba_note(3,36+phase*2,2,0)
  gba_note(4,36,3,0)
 end
 audio_mean=stat(91)
 audio_services=stat(92)
end
function _draw() cls(1) end
'''
        bench.write(project/'game.lua',lua)
        # advgm's documented converted stream: register,value; 1=one VBlank;
        # 0 followed by uint32 offset=loop. Use Butano's public VGM player.
        music=[]
        def reg(a,v):music.extend((a,v))
        reg(0x80,0x77);reg(0x81,0xff);reg(0x60,8)
        reg(0x62,0x80);reg(0x63,0x80);reg(0x68,0x40);reg(0x69,0x80)
        reg(0x70,0);reg(0x72,0);reg(0x73,{0:0,25:0x60,50:0x40,75:0x80,100:0x20}[sounds[2]['wave']['level']])
        for i in range(16):reg(0x90+i,(waveform[i*2]<<4)|waveform[i*2+1])
        reg(0x70,0x80);reg(0x78,0);reg(0x79,0x80)
        start=len(music)
        for phase in range(4):
            for lo,hi,n in ((0x64,0x65,60+phase*2),(0x6c,0x6d,48+phase*2),(0x74,0x75,48+phase*2)):
                value=periods[n];reg(lo,value&255);reg(hi,0x80|(value>>8))
            noise=sounds[3]['noise'];reg(0x7c,noise['divisor']|(noise['shift']<<4));reg(0x7d,0x80)
            music += [1]*(interval*2)
        music += [0]+list(start.to_bytes(4,'little'))
        h=work/'butano-project/include/bench_assets.h';bench.write(h,h.read_text().replace('const uint8_t music_data[]={1,0,0,0,0,0};','const uint8_t music_data[]={'+','.join(map(str,music))+'};'))
        d=out/name;d.mkdir(exist_ok=True);bench.dump(d/'program.json',dict(interval=interval,sounds=sounds,advgmBytes=music));shutil.copyfile(project/'game.lua',d/'game.lua')
        row=dict(name=name,strictAudioParity=False,reason='mTile schedules at 256 Hz; Butano advgm executes at VBlank. Trigger/sample phase equality is not established.',engines={})
        for engine in ('butano','mode0','mode4'):
            print(' ',engine,flush=True);rom=d/(engine+'.gba')
            if engine=='butano':
                bench.run(['make','-j4'],cwd=work/'butano-project',log=d/'build.log');shutil.copyfile(work/'butano-project/benchmark.gba',rom)
            else:bench.run([driver,'build',project,engine,rom],log=d/(engine+'-build.log'))
            bench.run([driver,'capture',bench.ROOT/'build/player/mtile-player',rom,d/(engine+'.json'),d/(engine+'.png'),60,600,0])
            data=json.loads((d/(engine+'.json')).read_text());summary=bench.summarize(data)
            if engine!='butano':
                ss=data['samples'];vals=[]
                for s in ss:
                    values={x['Name']:x['Raw']/65536 for x in s['watches']};vals.append(values)
                # Mean is integer-rounded by the runtime; report a bound.
                cycles=(vals[-1]['audio_mean']*vals[-1]['audio_services']-vals[0]['audio_mean']*vals[0]['audio_services'])
                elapsed=(ss[-1]['videoFrame']-ss[0]['videoFrame'])*280896
                summary.update(serviceCpuPercentEstimate=cycles/elapsed*100,serviceMeanCycles=vals[-1]['audio_mean'],measuredServices=vals[-1]['audio_services'])
            row['engines'][engine]=summary
        a=json.loads((d/'butano.json').read_text())['samples']
        keys=['CH1Sweep','CH1Duty','CH1Frequency','CH2Duty','CH2Frequency','CH3Length','CH3Frequency','CH4Envelope','CH4Frequency','MixPSG','MixDMA','Status']
        for e in ('mode0','mode4'):
            b=json.loads((d/(e+'.json')).read_text())['samples'];row['engines'][e]['registerMismatchTicks']=sum(any(x['psg'][k]!=y['psg'][k] for k in keys) for x,y in zip(a,b))
            row['engines'][e]['finalPSG']=b[-1]['psg']
        row['engines']['butano']['finalPSG']=a[-1]['psg'];results.append(row);bench.dump(d/'result.json',row)
    bench.dump(out/'results.json',results)
if __name__=='__main__':main()
