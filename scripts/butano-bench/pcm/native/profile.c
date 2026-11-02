// Instruction accounting of unmodified ROMs. The emulator is observed, not timed
// on the host. Event advancement while HALTed is recorded separately from CPU.
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <mgba/core/core.h>
#include <mgba/core/config.h>
#include <mgba/core/log.h>
#include <mgba/core/timing.h>
#include <mgba/internal/arm/arm.h>
#include <mgba/internal/gba/gba.h>
static color_t video[240*160];
static void quiet(struct mLogger*l,int c,enum mLogLevel v,const char*f,va_list a){(void)l;(void)c;(void)v;(void)f;(void)a;}
struct Bin {uint64_t cycles,instructions;unsigned wait;};
static struct Bin rom[0x100000/2],iwram[32768/2];
static unsigned envaddr(const char*name){const char*s=getenv(name);return s?strtoul(s,NULL,16):0;}
static struct Bin* bin(uint32_t pc){
 if(pc>=0x08000000&&pc<0x08100000)return &rom[(pc-0x08000000)/2];
 if(pc>=0x03000000&&pc<0x03008000)return &iwram[(pc-0x03000000)/2];return NULL;
}
int main(int argc,char**argv){
 if(argc<5){fprintf(stderr,"profile ROM WARMUP TICKS PREFIX [WAIT_PCS]\n");return 2;}
 unsigned warm=atoi(argv[2]),ticks=atoi(argv[3]);
 if(argc==6){FILE*f=fopen(argv[5],"r");if(!f)return 8;unsigned pc;while(fscanf(f,"%x",&pc)==1){struct Bin*b=bin(pc);if(b)b->wait=1;}fclose(f);}
 struct mLogger logger={.log=quiet,.filter=NULL};mLogSetDefaultLogger(&logger);
 struct mCore*c=mCoreFind(argv[1]);if(!c)return 3;mCoreInitConfig(c,"mtile-player");
 struct mCoreOptions options={.skipBios=true,.audioBuffers=1024,.sampleRate=48000,.fpsTarget=60,.volume=0x100};
 mCoreConfigLoadDefaults(&c->config,&options);if(!c->init(c)||!mCoreLoadFile(c,argv[1]))return 4;
 mCoreConfigMap(&c->config,&c->opts);c->setAudioBufferSize(c,c->opts.audioBuffers);c->loadConfig(c,&c->config);c->setVideoBuffer(c,video,240);c->reset(c);
 struct GBA*gba=c->board;struct ARMCore*cpu=c->cpu;uint32_t*mb=gba->memory.wram+0x3fe00/4;
 unsigned steps=0;while((mb[0]!=0x44554c47||mb[3]<warm||mb[2]&1)&&steps++<20000)c->runFrame(c);if(steps>=20000)return 5;
 // Start exactly on a new, complete publication, then measure complete intervals.
 unsigned old=mb[3];while(mb[3]==old||mb[2]&1)c->step(c);
 unsigned first=mb[3],last=first;uint32_t begin=mTimingCurrentTime(&gba->timing),boundary=begin;
 unsigned ptraddr=envaddr("PCM_CHANNELS_POINTER"),endaddr=envaddr("PCM_CHANNELS_END_POINTER"),minvoices=1000,maxvoices=0,capacity=0;
 unsigned frameaddr=envaddr("PCM_FRAME_ADDRESS"),frameret=0,framebegin=0,framecalls=0,framemax=0,frameover=0;uint64_t framesum=0;
 uint64_t events=0,halt=0,other=0,instructions=0,exec=0,wait=0,te=0,th=0,tx=0,tw=0;
 unsigned spmin[32],spmax[32];for(int i=0;i<32;i++){spmin[i]=0xffffffff;spmax[i]=0;}
 char path[4096];snprintf(path,sizeof(path),"%s.ticks.csv",argv[4]);FILE*tf=fopen(path,"w");if(!tf)return 7;
 fprintf(tf,"tick,elapsed,instructions,wait,activeEvents,haltEvents,voices\n");
 while(mb[3]<first+ticks||mb[2]&1){
  while(cpu->cycles>=cpu->nextEvent){
   uint32_t b=mTimingCurrentTime(&gba->timing);int halted=cpu->halted;cpu->irqh.processEvents(cpu);
   uint32_t d=(uint32_t)mTimingCurrentTime(&gba->timing)-b;if(halted){halt+=d;th+=d;}else{events+=d;te+=d;}
  }
  uint32_t start=mTimingCurrentTime(&gba->timing);uint32_t pc=cpu->gprs[ARM_PC]-(cpu->cpsr.t?2:4);
  if(frameret&&pc==frameret){unsigned d=start-framebegin;framesum+=d;if(d>framemax)framemax=d;if(d>197120)frameover++;framecalls++;frameret=0;}
  if(frameaddr&&pc==frameaddr&&!frameret){framebegin=start;frameret=cpu->gprs[ARM_LR]&~1u;}
  unsigned mode=cpu->cpsr.priv,sp=cpu->gprs[ARM_SP];
  if(sp>=0x03000000&&sp<=0x03008000){if(sp<spmin[mode])spmin[mode]=sp;if(sp>spmax[mode])spmax[mode]=sp;}
  c->step(c);uint32_t d=(uint32_t)mTimingCurrentTime(&gba->timing)-start;struct Bin*b=bin(pc);
  if(b){b->cycles+=d;b->instructions++;if(b->wait){wait+=d;tw+=d;}}else other+=d;
  exec+=d;tx+=d;++instructions;
  if(mb[3]!=last&&!(mb[2]&1)){
   unsigned voices=0;
   if(ptraddr&&endaddr){unsigned p=c->busRead32(c,ptraddr),end=c->busRead32(c,endaddr);capacity=(end-p)/16;if(capacity>32)return 9;for(unsigned i=0;i<capacity;i++)if(!(c->busRead32(c,p+i*16)&0x80000000))voices++;}
   if(voices<minvoices)minvoices=voices;if(voices>maxvoices)maxvoices=voices;
   uint32_t now=mTimingCurrentTime(&gba->timing);fprintf(tf,"%u,%u,%llu,%llu,%llu,%llu,%u\n",mb[3],now-boundary,(unsigned long long)tx,(unsigned long long)tw,(unsigned long long)te,(unsigned long long)th,voices);
   last=mb[3];boundary=now;te=th=tx=tw=0;
  }
  if(instructions>2000000000ULL)return 6;
 }
 fclose(tf);snprintf(path,sizeof(path),"%s.pc.csv",argv[4]);FILE*pf=fopen(path,"w");fprintf(pf,"pc,cycles,instructions\n");
 for(unsigned i=0;i<0x100000/2;i++)if(rom[i].instructions)fprintf(pf,"%08x,%llu,%llu\n",0x08000000+i*2,(unsigned long long)rom[i].cycles,(unsigned long long)rom[i].instructions);
 for(unsigned i=0;i<32768/2;i++)if(iwram[i].instructions)fprintf(pf,"%08x,%llu,%llu\n",0x03000000+i*2,(unsigned long long)iwram[i].cycles,(unsigned long long)iwram[i].instructions);fclose(pf);
 snprintf(path,sizeof(path),"%s.meta.json",argv[4]);FILE*mf=fopen(path,"w");
 fprintf(mf,"{\"firstTick\":%u,\"lastTick\":%u,\"elapsedCycles\":%u,\"instructionCycles\":%llu,\"waitInstructionCycles\":%llu,\"activeEventCycles\":%llu,\"haltEventCycles\":%llu,\"otherAddressCycles\":%llu,\"instructions\":%llu,\"mailboxMemory\":[%u,%u,%u,%u],\"stacks\":[",first,mb[3],(uint32_t)mTimingCurrentTime(&gba->timing)-begin,(unsigned long long)exec,(unsigned long long)wait,(unsigned long long)events,(unsigned long long)halt,(unsigned long long)other,(unsigned long long)instructions,mb[114],mb[115],mb[116],mb[117]);
 int comma=0;for(int i=0;i<32;i++)if(spmax[i]){fprintf(mf,"%s{\"mode\":%d,\"minSP\":%u,\"maxSP\":%u,\"observedRange\":%u}",comma?",":"",i,spmin[i],spmax[i],spmax[i]-spmin[i]);comma=1;}
 fprintf(mf,"],\"mixerCapacity\":%u,\"minObservedVoices\":%u,\"maxObservedVoices\":%u,\"mmFrameCalls\":%u,\"mmFrameCycles\":%llu,\"mmFrameMaximum\":%u,\"mmFrameOver160Lines\":%u}\n",capacity,minvoices,maxvoices,framecalls,(unsigned long long)framesum,framemax,frameover);fclose(mf);mCoreConfigDeinit(&c->config);c->deinit(c);return 0;
}
