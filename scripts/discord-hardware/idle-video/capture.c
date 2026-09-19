#include <mgba/core/core.h>
#include <mgba/core/config.h>
#include <mgba/gba/core.h>
#include <mgba/internal/gba/gba.h>
#include <mgba/internal/arm/arm.h>
#include <stdio.h>
#include <stdlib.h>
#include <inttypes.h>

static struct mTimingEvent sample;
static uint64_t samples, halted, dma;
static struct GBA *gba;
static void tick(struct mTiming *t,void *ctx,uint32_t late) {
 (void)ctx;
 ++samples;
 if(gba->cpu->halted)++halted;
 if(gba->cpuBlocked)++dma;
 mTimingSchedule(t,&sample,127-(int32_t)late);
}
int main(int argc,char **argv) {
 if(argc<3)return 2;
 struct mCore *core=GBACoreCreate();
 core->init(core);
 mCoreConfigInit(&core->config,NULL);
 mCoreConfigSetDefaultIntValue(&core->config,"idleOptimization",-1);
 core->loadConfig(core,&core->config);
 color_t *video=calloc(240*160,sizeof(color_t));
 core->setVideoBuffer(core,video,240);
 if(!mCoreLoadFile(core,argv[1]))return 3;
 core->reset(core);
 gba=core->board;
 gba->idleOptimization=IDLE_LOOP_IGNORE;
 sample=(struct mTimingEvent){.callback=tick,.name="halt sample",.priority=0x80};
 mTimingSchedule(&gba->timing,&sample,127);
 int frames=atoi(argv[2]);
 for(int i=0;i<frames;i++) {
  core->runFrame(core);
  uint64_t hash=1469598103934665603ULL;
  for(int p=0;p<240*160;p++){hash^=video[p];hash*=1099511628211ULL;}
  printf("F %d %016"PRIx64"\n",i,hash);
 }
 printf("PC %08x sp %08x\n",gba->cpu->gprs[15],gba->cpu->gprs[13]);
 printf("S samples=%"PRIu64" halted=%"PRIu64" dma=%"PRIu64"\n",samples,halted,dma);
 for(int n=3;n<argc;n++){
  uint32_t addr=(uint32_t)strtoul(argv[n],NULL,0);
  printf("W %08x %08x\n",addr,core->busRead32(core,addr));
 }
 return 0;
}
