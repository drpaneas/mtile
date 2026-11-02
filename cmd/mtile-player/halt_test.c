// GBA HALT is level-sensitive to IE&IF, independently of IME and CPSR.I.
// This reproduces the pending-request sleep bug in the pinned 0.10.3 core.
#include <stdio.h>
#include <string.h>
#include <mgba/internal/arm/arm.h>
#include <mgba/internal/gba/gba.h>
#include <mgba/internal/gba/io.h>

int main(void) {
    static struct GBA gba;
    static struct ARMCore cpu;
    const unsigned cases[][3] = {{1,1,0},{5,4,0},{1,2,1},{0,1,1},{5,0,1}};
    for (unsigned i=0;i<sizeof(cases)/sizeof(cases[0]);++i) {
        for (unsigned ime=0;ime<2;++ime) for (unsigned masked=0;masked<2;++masked) {
            memset(&gba,0,sizeof(gba));memset(&cpu,0,sizeof(cpu));gba.cpu=&cpu;
            cpu.cycles=17;cpu.nextEvent=123;cpu.cpsr.i=masked;
            gba.memory.io[REG_IE>>1]=cases[i][0];gba.memory.io[REG_IF>>1]=cases[i][1];gba.memory.io[REG_IME>>1]=ime;
            GBAHalt(&gba);
            if ((unsigned)cpu.halted!=cases[i][2]) {
                fprintf(stderr,"HALT: IE=%x IF=%x IME=%u CPSR.I=%u: halted=%d expected=%u\n",cases[i][0],cases[i][1],ime,masked,cpu.halted,cases[i][2]);return 1;
            }
        }
    }
    puts("HALT pending-request contract: 20 cases passed");return 0;
}
