// Benchmark-only F/1 stopwatch. Core hooks bracket only intentional VBlank
// waiting; useful ISR work occurring inside a wait is added back separately.
#include "bn_hw_timer.h"
static bool armed,paused;
static unsigned start,total,irq_start;
static bool irq_paused;
extern "C" void bench_start(){total=0;armed=true;paused=false;start=bn::hw::timer::ticks();}
extern "C" unsigned bench_finish(){total+=bn::hw::timer::ticks()-start;armed=false;return total;}
extern "C" void bench_pause(){if(armed){total+=bn::hw::timer::ticks()-start;paused=true;}}
extern "C" void bench_resume(){if(armed){paused=false;start=bn::hw::timer::ticks();}}
extern "C" void bench_irq_start(){irq_paused=armed&&paused;if(irq_paused)irq_start=bn::hw::timer::ticks();}
extern "C" void bench_irq_end(){if(irq_paused)total+=bn::hw::timer::ticks()-irq_start;}
