#!/usr/bin/env python3
"""Independent integer state simulation and pixel oracle for the ROM runs.
Checks every captured tick in BOTH repetitions, not only final screenshots.
"""
import hashlib, json, pathlib, re, sys
import numpy as np
import run as suite

def pixels(w, frame, image=False):
    tiles=np.array(suite.TILES,dtype=np.uint8)
    if w.get('opaque'):tiles[1:]=np.where(tiles[1:]==0,2,tiles[1:])
    if w['kind']==7:
        for i in range(16):tiles[i+1 if w['spread'] else 1,0 if w['spread'] else i]=2+(frame%2)*3
    yy,xx=np.indices((160,240));canvas=np.ones((160,240),dtype=np.uint8)
    for layer in range(w['layers']):
        moving=w['kind'] in (4,5,14)
        cx=frame*(layer+1)*w.get('speed',1) if moving else 0
        cy=frame*w.get('verticalSpeed',0) if moving else 0
        wx,wy=xx+cx,yy+cy;mx,my=wx//8,wy//8
        ids=(1+(mx*3+my*5+layer*7)%31).astype(np.uint8)
        if layer:ids=np.where((mx+2*my+layer)%4,0,ids)
        if w['kind']==6:ids=np.where((my==0)&(mx<w['mutations']),2+frame%2,ids)
        ids=np.where((mx<w.get('logicalWidth',w['width']))&(my<w.get('logicalHeight',w['height'])),ids,0)
        values=tiles[ids,(wy%8)*8+wx%8];canvas=np.where(values,values,canvas)
    for i in range(w['objects']):
        size=w['size'];cols=12 if size==8 else 8;spacing=18 if size==8 else 28
        x=4+(i%cols)*spacing+frame%8;y=8+(i//cols)*18
        for dy in range(size):
            for dx in range(size):
                if x+dx>=240 or y+dy>=160:continue
                tile=(1+(frame//4)%4 if w['kind']==2 else 1)+(dy//8)*8+dx//8
                value=tiles[tile,(dy%8)*8+dx%8]
                if value:canvas[y+dy,x+dx]=value
    if w['kind']==13:
        for i in range(w['entities']):
            x=(i*13+frame)%220;y=(i*17)%140;canvas[y:y+10,x:x+10]=2+i%30
    if w['kind'] in (8,14,15):
        text=w['hud'] if w['kind']==8 else str(frame)
        raw=(suite.ROOT/'runtime/font_default.go').read_text().split('var builtinFont = Font{ASCII: [95][6]uint16{')[1]
        glyphs=[[int(x.strip()) for x in row.split(',') if x.strip()] for row in re.findall(r'\{([^{}]+)\}',raw)[:95]]
        for i,ch in enumerate(text):
            for x,col in enumerate(glyphs[ord(ch)-32]):
                for y in range(10):
                    if col>>y&1:canvas[8+y,8+i*6+x]=7
    palette=np.array(suite.COLORS,dtype=np.uint8)
    if w['kind']==9:
        for i in range(w['mutations']):palette[i+2]=suite.COLORS[2+(i+frame)%30]
    rgb=palette[canvas];rgb=(rgb<<3)|(rgb>>2)
    rgba=np.full((160,240,4),255,dtype=np.uint8);rgba[:,:,:3]=rgb
    return rgba if image else hashlib.sha256(rgba.tobytes()).hexdigest()

def checksums(w, end):
    values={};n=w['entities']
    if w['kind']==10:
        px=[(8+(i*13)%220)<<16 for i in range(n)];py=[(8+(i*17)%136)<<16 for i in range(n)]
        vx=[((i%7)-3)*8192 for i in range(n)];vy=[((i%5)-2)*8192 for i in range(n)]
        for tick in range(1,end+1):
            total=0
            for i in range(n):
                dx=vx[i];dy=vy[i]+4096
                if w.get('drag'):dx=dx*65280>>16;dy=dy*65280>>16
                x=px[i]+dx;y=py[i]+dy
                if x<4*65536:x=4*65536;dx=abs(dx)
                elif x>235*65536:x=235*65536;dx=-abs(dx)
                if y<4*65536:y=4*65536;dy=abs(dy)
                elif y>155*65536:y=155*65536;dy=-abs(dy)
                px[i],py[i],vx[i],vy[i]=x,y,dx,dy;total=(total+(x>>16)+(y>>16))%16001
            values[tick]=total
    else:
        for tick in range(1,end+1):
            f=tick%120;total=0
            if w['kind']==11:
                for i in range(n):total=(total+1+(((i*7+f)%32)*3+((i*11+f)%32)*5)%31)%16001
            elif w['kind']==14:
                for i in range(n):total=(total+1+(((i*7)%32)*3+((i+f)%32)*5)%31)%16001
            values[tick]=total
    return values

def main():
    root=pathlib.Path(sys.argv[1]);checks=[];failures=[]
    for path in sorted(root.glob('*/result.json')):
        result=json.loads(path.read_text());w=result['workload'];captures=list(path.parent.glob('*-samples-*.json'));end=max(json.loads(p.read_text())['samples'][-1]['tick'] for p in captures);expected_state=checksums(w,end);cache={}
        for engine,summary in result['engines'].items():
            if 'error' in summary:failures.append(f"{w['name']} {engine}: {summary['error']}");continue
            for capture in sorted(path.parent.glob(engine+'-samples-*.json')):
                ss=json.loads(capture.read_text())['samples'];bad_pixels=[];bad_states=[]
                for s in ss:
                    phase=s['tick']%120
                    if phase not in cache:cache[phase]=pixels(w,phase)
                    if s['sha256']!=cache[phase]:bad_pixels.append(s['tick'])
                    watches={x['Name']:x['Raw'] for x in s['watches']}
                    if watches.get('tick')!=s['tick']*65536 or watches.get('checksum')!=expected_state[s['tick']]*65536:bad_states.append(s['tick'])
                record=dict(workload=w['name'],engine=engine,capture=capture.name,samples=len(ss),goldenPixelMismatches=len(bad_pixels),goldenStateMismatches=len(bad_states),firstPixelMismatches=bad_pixels[:5],firstStateMismatches=bad_states[:5])
                checks.append(record)
                if bad_pixels or bad_states:failures.append(record)
    suite.dump(root/'verification.json',dict(checks=checks,failures=failures,passAll=not failures,totalTicks=sum(c['samples'] for c in checks)))
    print(json.dumps(dict(captures=len(checks),ticks=sum(c['samples'] for c in checks),failures=failures),indent=2))
    return bool(failures)
if __name__=='__main__':sys.exit(main())
