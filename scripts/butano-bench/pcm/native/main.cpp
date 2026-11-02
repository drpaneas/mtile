// Audio-only fixture. WORKLOAD, MUSIC_ID and SAMPLE_ID are build constants.
// GLUD is a benchmark publication, not an engine timing hook.
#include "bn_core.h"
#include "bn_memory.h"
#include "bn_music_item.h"
#include "bn_sound_item.h"
#include <cstdint>
#ifndef PERIOD
#define PERIOD 30
#endif
static volatile uint32_t* const mb = reinterpret_cast<volatile uint32_t*>(0x0203fe00);
static void publish(unsigned tick) {
 mb[2]=tick*2+1;mb[0]=0x44554c47;mb[1]=0x02000001;
 mb[3]=tick;mb[4]=bn::core::last_missed_frames();mb[17]=0;
 mb[114]=bn::memory::used_static_ewram();mb[115]=bn::memory::used_static_iwram();
 mb[116]=bn::memory::used_stack_iwram();mb[117]=bn::memory::used_alloc_ewram();
 mb[2]=tick*2+2;
}
int main() {
 bn::core::init();bn::core::set_skip_frames(1);
 if constexpr(WORKLOAD>=3) bn::music_item(MUSIC_ID).play();
 for(unsigned tick=1;;++tick) {
  if constexpr(WORKLOAD==2 || WORKLOAD==4) {
   if(tick%PERIOD==1) {
    for(int i=0;i<EFFECTS;++i) bn::sound_item(SAMPLE_ID).play(bn::fixed(0.125));
   }
  }
  bn::core::update();publish(tick);
 }
}
