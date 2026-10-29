// Instruction-level profiling of unmodified ROMs with the pinned mGBA core.
// Pending hardware events are accounted separately from instruction cycles.
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
static void quiet(struct mLogger* l,int c,enum mLogLevel v,const char* f,va_list a){(void)l;(void)c;(void)v;(void)f;(void)a;}
struct Bin {uint64_t cycles, instructions;};
int main(int argc,char**argv){
 if(argc!=4){fprintf(stderr,"usage: profile ROM WARMUP TICKS\n");return 2;}
 unsigned warm=atoi(argv[2]),ticks=atoi(argv[3]);
 struct mLogger logger={.log=quiet,.filter=NULL};mLogSetDefaultLogger(&logger);
 struct mCore* c=mCoreFind(argv[1]);if(!c)return 3;
 mCoreInitConfig(c,"mtile-player");
 struct mCoreOptions options={.skipBios=true,.audioBuffers=1024,.sampleRate=48000,.fpsTarget=60,.volume=0x100};
 mCoreConfigLoadDefaults(&c->config,&options);
 if(!c->init(c)||!mCoreLoadFile(c,argv[1]))return 4;
 mCoreConfigMap(&c->config,&c->opts);c->setAudioBufferSize(c,c->opts.audioBuffers);c->loadConfig(c,&c->config);
 c->setVideoBuffer(c,video,240);c->reset(c);
 struct GBA* gba=c->board;struct ARMCore* cpu=c->cpu;
 uint32_t* mailbox=gba->memory.wram+(0x3fe00/4);
 unsigned steps=0;
 while((mailbox[0]!=0x44554c47 || mailbox[3]<warm || mailbox[2]&1) && steps++<20000)c->runFrame(c);
 if(steps>=20000)return 5;
 const unsigned rom_size=1<<20,iwram_size=32768;
 struct Bin* rom=calloc(rom_size/2,sizeof(*rom));struct Bin* iwram=calloc(iwram_size/2,sizeof(*iwram));
 uint32_t begin=(uint32_t)mTimingCurrentTime(&gba->timing);uint64_t events=0,other=0,instructions=0;
 unsigned first=mailbox[3];
 while(mailbox[3]<first+ticks || mailbox[2]&1){
  uint32_t before_events=(uint32_t)mTimingCurrentTime(&gba->timing);
  while(cpu->cycles>=cpu->nextEvent)cpu->irqh.processEvents(cpu);
  uint32_t start=(uint32_t)mTimingCurrentTime(&gba->timing);events+=(uint32_t)(start-before_events);
  // ARMCore's PC points one instruction past the current prefetch slot.
  uint32_t pc=cpu->gprs[ARM_PC]-(cpu->cpsr.t?2:4);
  c->step(c);uint64_t cost=(uint32_t)((uint32_t)mTimingCurrentTime(&gba->timing)-start);
  struct Bin* bin=NULL;
  if(pc>=0x08000000 && pc<0x08000000+rom_size)bin=&rom[(pc-0x08000000)/2];
  if(pc>=0x03000000 && pc<0x03000000+iwram_size)bin=&iwram[(pc-0x03000000)/2];
  if(bin){bin->cycles+=cost;++bin->instructions;}else other+=cost;
  ++instructions;
  if(instructions>2000000000ULL){fprintf(stderr,"instruction ceiling\n");return 6;}
 }
 printf("pc,cycles,instructions\n");
 for(unsigned i=0;i<rom_size/2;++i)if(rom[i].instructions)printf("%08x,%llu,%llu\n",0x08000000+i*2,(unsigned long long)rom[i].cycles,(unsigned long long)rom[i].instructions);
 for(unsigned i=0;i<iwram_size/2;++i)if(iwram[i].instructions)printf("%08x,%llu,%llu\n",0x03000000+i*2,(unsigned long long)iwram[i].cycles,(unsigned long long)iwram[i].instructions);
 fprintf(stderr,"{\"firstPublishedTick\":%u,\"lastPublishedTick\":%u,\"instructions\":%llu,\"elapsedCycles\":%llu,\"eventCycles\":%llu,\"otherAddressCycles\":%llu}\n",first,mailbox[3],(unsigned long long)instructions,(unsigned long long)(uint32_t)((uint32_t)mTimingCurrentTime(&gba->timing)-begin),(unsigned long long)events,(unsigned long long)other);
 free(rom);free(iwram);mCoreConfigDeinit(&c->config);c->deinit(c);mLogSetDefaultLogger(NULL);return 0;
}
