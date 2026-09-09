# TinyGo 0.42.0 build recipes. cwd is runtime/.
#
# ld.lld is invoked directly (not via a cc driver).
#   -ldflags="-Map=smoke.map"  -> parsed by TinyGo, rejected
#   -Wl,-Map=smoke.map         -> not valid for ld.lld
#   -ldflags="-extldflags=-Map=smoke.map"  -> reaches lld

AUDIOREF_PSG_TARGETS := audioref-ch1-gba audioref-ch2-gba audioref-ch3-gba audioref-ch4-gba
AUDIOREF_RESEARCH_TARGETS := audioref-fifoa-gba audioref-fifob-gba audioref-all-gba
AUDIOREF_TARGETS := $(AUDIOREF_PSG_TARGETS) $(AUDIOREF_RESEARCH_TARGETS)

.PHONY: generate smoke-gba smoke-elf drawspike-gba drawspike-elf benchspike-gba benchspike-elf audioref-gba audioref-research-gba $(AUDIOREF_TARGETS) audiopsg-gba audiopsg-elf audiopsgcoexist-gba audiopsgcoexist-elf

generate:
	go run ./internal/cmd/gentarget

smoke-gba: generate
	tinygo build -gc=none -print-allocs-cover=. -target=target-gba-smoke.json -o smoke.gba ./smoke 2>smoke.allocs
	go run ./internal/cmd/checkallocs smoke.allocs

smoke-elf: generate
	tinygo build -gc=none -ldflags="-extldflags=-Map=smoke.map" -print-allocs-cover=. -target=target-gba-smoke.json -o smoke.elf ./smoke 2>smoke.allocs
	go run ./internal/cmd/checkallocs smoke.allocs

drawspike-gba: generate
	tinygo build -gc=none -print-allocs-cover=. -target=target-gba.json -o game.gba ./cmd/drawspike 2>drawspike.allocs
	cp game.gba drawspike.gba
	go run ./internal/cmd/checkallocs drawspike.allocs

drawspike-elf: generate
	tinygo build -gc=none -ldflags="-extldflags=-Map=drawspike.map" -print-allocs-cover=. -target=target-gba.json -o drawspike.elf ./cmd/drawspike 2>drawspike.allocs
	go run ./internal/cmd/checkallocs drawspike.allocs

# benchspike measures runtime hot paths in real ARM7TDMI cycles and prints
# a cycles/%-of-frame table on screen. See docs/performance.md.
benchspike-gba: generate
	tinygo build -gc=none -opt=2 -print-allocs-cover=. -target=target-gba.json -o benchspike.gba ./cmd/benchspike 2>benchspike.allocs
	go run ./internal/cmd/checkallocs benchspike.allocs

benchspike-elf: generate
	tinygo build -gc=none -opt=2 -ldflags="-extldflags=-Map=benchspike.map" -print-allocs-cover=. -target=target-gba.json -o benchspike.elf ./cmd/benchspike 2>benchspike.allocs
	go run ./internal/cmd/checkallocs benchspike.allocs

# Audio v2 is gated only by the four direct-register PSG truth cartridges.
# The FIFO cartridges remain separately buildable historical research.
audioref-gba: $(AUDIOREF_PSG_TARGETS)

audioref-research-gba: $(AUDIOREF_RESEARCH_TARGETS)

$(AUDIOREF_TARGETS): audioref-%-gba: generate
	tinygo build -gc=none -opt=2 -print-allocs-cover=. -target=target-gba.json -o audioref-$*.gba ./cmd/audioref-$* 2>audioref-$*.allocs
	go run ./internal/cmd/checkallocs audioref-$*.allocs

# Production-shaped PSG-only link gate: CH1 phrase, 96 PPQN scheduler, exact
# 256 Hz Timer0 IRQ, no Direct Sound, FIFO, audio DMA, or target allocations.
audiopsg-gba: generate
	tinygo build -gc=none -opt=2 -print-allocs-cover=. -target=target-gba.json -o audiopsg.gba ./cmd/audiopsg 2>audiopsg.allocs
	go run ./internal/cmd/checkallocs audiopsg.allocs

audiopsg-elf: generate
	tinygo build -gc=none -opt=2 -ldflags="-extldflags=-Map=audiopsg.map" -print-allocs-cover=. -target=target-gba.json -o audiopsg.elf ./cmd/audiopsg 2>audiopsg.allocs
	go run ./internal/cmd/checkallocs audiopsg.allocs

# Full production gate: all four PSG channels under a dense Mode 4 graphics
# workload, with debugger-visible service-cycle and self-test telemetry.
audiopsgcoexist-gba: generate
	tinygo build -gc=none -opt=2 -print-allocs-cover=. -target=target-gba.json -o audiopsgcoexist.gba ./cmd/audiopsgcoexist 2>audiopsgcoexist.allocs
	go run ./internal/cmd/checkallocs audiopsgcoexist.allocs

audiopsgcoexist-elf: generate
	tinygo build -gc=none -opt=2 -ldflags="-extldflags=-Map=audiopsgcoexist.map" -print-allocs-cover=. -target=target-gba.json -o audiopsgcoexist.elf ./cmd/audiopsgcoexist 2>audiopsgcoexist.allocs
	go run ./internal/cmd/checkallocs audiopsgcoexist.allocs
