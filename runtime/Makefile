# TinyGo 0.41.1 recipes measured on this repo. cwd is runtime/.
#
# ld.lld is invoked directly (not via a cc driver).
#   -ldflags="-Map=smoke.map"  -> parsed by TinyGo, rejected
#   -Wl,-Map=smoke.map         -> not valid for ld.lld
#   -ldflags="-extldflags=-Map=smoke.map"  -> reaches lld

AUDIOREF_PSG_TARGETS := audioref-ch1-gba audioref-ch2-gba audioref-ch3-gba audioref-ch4-gba
AUDIOREF_RESEARCH_TARGETS := audioref-fifoa-gba audioref-fifob-gba audioref-all-gba
AUDIOREF_TARGETS := $(AUDIOREF_PSG_TARGETS) $(AUDIOREF_RESEARCH_TARGETS)

.PHONY: generate smoke-gba smoke-elf drawspike-gba drawspike-elf benchspike-gba benchspike-elf audiospike-gba audiospike-elf audioref-gba audioref-research-gba $(AUDIOREF_TARGETS) audiopsg-gba audiopsg-elf audiopsgcoexist-gba audiopsgcoexist-elf audiobench-gba audiobench-elf audiostereobench-gba audiostereobench-elf audiostream-gba audiostream-elf audiohardware-gba audiohardware-elf audiohardwarestandard-gba audiohardwarestandard-elf audiostereostream-gba audiostereostream-elf audiostereocapture-gba audiostereocapture-elf audiocoexist-gba audiocoexist-elf audiopcmcoexist-gba audiopcmcoexist-elf audiopcmthreecoexist-gba audiopcmthreecoexist-elf audiopcmtwocoexist-gba audiopcmtwocoexist-elf audiostereocoexist-gba audiostereocoexist-elf

generate:
	go run ./internal/cmd/gentarget

smoke-gba: generate
	tinygo build -gc=none -print-allocs=. -target=target-gba-smoke.json -o smoke.gba ./smoke 2>smoke.allocs
	go run ./internal/cmd/checkallocs smoke.allocs

smoke-elf: generate
	tinygo build -gc=none -ldflags="-extldflags=-Map=smoke.map" -print-allocs=. -target=target-gba-smoke.json -o smoke.elf ./smoke 2>smoke.allocs
	go run ./internal/cmd/checkallocs smoke.allocs

drawspike-gba: generate
	tinygo build -gc=none -print-allocs=. -target=target-gba.json -o game.gba ./cmd/drawspike 2>drawspike.allocs
	cp game.gba drawspike.gba
	go run ./internal/cmd/checkallocs drawspike.allocs

drawspike-elf: generate
	tinygo build -gc=none -ldflags="-extldflags=-Map=drawspike.map" -print-allocs=. -target=target-gba.json -o drawspike.elf ./cmd/drawspike 2>drawspike.allocs
	go run ./internal/cmd/checkallocs drawspike.allocs

# benchspike measures runtime hot paths in real ARM7TDMI cycles and prints
# a cycles/%-of-frame table on screen. See docs/performance.md.
benchspike-gba: generate
	tinygo build -gc=none -opt=2 -print-allocs=. -target=target-gba.json -o benchspike.gba ./cmd/benchspike 2>benchspike.allocs
	go run ./internal/cmd/checkallocs benchspike.allocs

benchspike-elf: generate
	tinygo build -gc=none -opt=2 -ldflags="-extldflags=-Map=benchspike.map" -print-allocs=. -target=target-gba.json -o benchspike.elf ./cmd/benchspike 2>benchspike.allocs
	go run ./internal/cmd/checkallocs benchspike.allocs

# audiospike proves FIFO A + Timer 0 + DMA 1 setup, repeat, silence,
# stop, and restart before the production refill architecture is selected.
audiospike-gba: generate
	tinygo build -gc=none -opt=2 -print-allocs=. -target=target-gba.json -o audiospike.gba ./cmd/audiospike 2>audiospike.allocs
	go run ./internal/cmd/checkallocs audiospike.allocs

audiospike-elf: generate
	tinygo build -gc=none -opt=2 -ldflags="-extldflags=-Map=audiospike.map" -print-allocs=. -target=target-gba.json -o audiospike.elf ./cmd/audiospike 2>audiospike.allocs
	go run ./internal/cmd/checkallocs audiospike.allocs

# Audio v2 is gated only by the four direct-register PSG truth cartridges.
# The FIFO cartridges remain separately buildable historical research.
audioref-gba: $(AUDIOREF_PSG_TARGETS)

audioref-research-gba: $(AUDIOREF_RESEARCH_TARGETS)

$(AUDIOREF_TARGETS): audioref-%-gba: generate
	tinygo build -gc=none -opt=2 -print-allocs=. -target=target-gba.json -o audioref-$*.gba ./cmd/audioref-$* 2>audioref-$*.allocs
	go run ./internal/cmd/checkallocs audioref-$*.allocs

# Production-shaped PSG-only link gate: CH1 phrase, 96 PPQN scheduler, exact
# 256 Hz Timer0 IRQ, no Direct Sound, FIFO, audio DMA, or target allocations.
audiopsg-gba: generate
	tinygo build -gc=none -opt=2 -print-allocs=. -target=target-gba.json -o audiopsg.gba ./cmd/audiopsg 2>audiopsg.allocs
	go run ./internal/cmd/checkallocs audiopsg.allocs

audiopsg-elf: generate
	tinygo build -gc=none -opt=2 -ldflags="-extldflags=-Map=audiopsg.map" -print-allocs=. -target=target-gba.json -o audiopsg.elf ./cmd/audiopsg 2>audiopsg.allocs
	go run ./internal/cmd/checkallocs audiopsg.allocs

# Full production gate: all four PSG channels under a dense Mode 4 graphics
# workload, with debugger-visible service-cycle and self-test telemetry.
audiopsgcoexist-gba: generate
	tinygo build -gc=none -opt=2 -print-allocs=. -target=target-gba.json -o audiopsgcoexist.gba ./cmd/audiopsgcoexist 2>audiopsgcoexist.allocs
	go run ./internal/cmd/checkallocs audiopsgcoexist.allocs

audiopsgcoexist-elf: generate
	tinygo build -gc=none -opt=2 -ldflags="-extldflags=-Map=audiopsgcoexist.map" -print-allocs=. -target=target-gba.json -o audiopsgcoexist.elf ./cmd/audiopsgcoexist 2>audiopsgcoexist.allocs
	go run ./internal/cmd/checkallocs audiopsgcoexist.allocs

# audiobench measures shared-mixer costs with TM0/TM1 while Direct Sound is
# stopped. Results are visible on screen and in debugger-visible EWRAM words.
audiobench-gba: generate
	tinygo build -gc=none -opt=2 -print-allocs=. -target=target-gba.json -o audiobench.gba ./cmd/audiobench 2>audiobench.allocs
	go run ./internal/cmd/checkallocs audiobench.allocs

audiobench-elf: generate
	tinygo build -gc=none -opt=2 -ldflags="-extldflags=-Map=audiobench.map" -print-allocs=. -target=target-gba.json -o audiobench.elf ./cmd/audiobench 2>audiobench.allocs
	go run ./internal/cmd/checkallocs audiobench.allocs

# audiostereobench isolates left/right and pan costs so linking provisional
# stereo code does not perturb the established mono/SFX benchmark ROM layout.
audiostereobench-gba: generate
	tinygo build -gc=none -opt=2 -print-allocs=. -target=target-gba.json -o audiostereobench.gba ./cmd/audiostereobench 2>audiostereobench.allocs
	go run ./internal/cmd/checkallocs audiostereobench.allocs

audiostereobench-elf: generate
	tinygo build -gc=none -opt=2 -ldflags="-extldflags=-Map=audiostereobench.map" -print-allocs=. -target=target-gba.json -o audiostereobench.elf ./cmd/audiostereobench 2>audiostereobench.allocs
	go run ./internal/cmd/checkallocs audiostereobench.allocs

# audiostream exercises the interrupt-assisted four-page production candidate.
audiostream-gba: generate
	tinygo build -gc=none -opt=2 -print-allocs=. -target=target-gba.json -o audiostream.gba ./cmd/audiostream 2>audiostream.allocs
	go run ./internal/cmd/checkallocs audiostream.allocs

audiostream-elf: generate
	tinygo build -gc=none -opt=2 -ldflags="-extldflags=-Map=audiostream.map" -print-allocs=. -target=target-gba.json -o audiostream.elf ./cmd/audiostream 2>audiostream.allocs
	go run ./internal/cmd/checkallocs audiostream.allocs

# audiohardware is the self-scoring ten-minute physical-GBA sign-off ROM for
# the documented two-synth-plus-looping-PCM mono profile. The standard-tagged
# sibling covers the separate four-synth production profile.
audiohardware-gba: generate
	tinygo build -gc=none -opt=2 -print-allocs=. -target=target-gba.json -o audiohardware.gba ./cmd/audiohardware 2>audiohardware.allocs
	go run ./internal/cmd/checkallocs audiohardware.allocs

audiohardware-elf: generate
	tinygo build -gc=none -opt=2 -ldflags="-extldflags=-Map=audiohardware.map" -print-allocs=. -target=target-gba.json -o audiohardware.elf ./cmd/audiohardware 2>audiohardware.allocs
	go run ./internal/cmd/checkallocs audiohardware.allocs

audiohardwarestandard-gba: generate
	tinygo build -gc=none -opt=2 -tags=standard -print-allocs=. -target=target-gba.json -o audiohardwarestandard.gba ./cmd/audiohardware 2>audiohardwarestandard.allocs
	go run ./internal/cmd/checkallocs audiohardwarestandard.allocs

audiohardwarestandard-elf: generate
	tinygo build -gc=none -opt=2 -tags=standard -ldflags="-extldflags=-Map=audiohardwarestandard.map" -print-allocs=. -target=target-gba.json -o audiohardwarestandard.elf ./cmd/audiohardware 2>audiohardwarestandard.allocs
	go run ./internal/cmd/checkallocs audiohardwarestandard.allocs

# audiostereostream proves synchronized FIFO A/DMA1 left and FIFO B/DMA2
# right page handoffs without enabling stereo in generated projects.
audiostereostream-gba: generate
	tinygo build -gc=none -opt=2 -print-allocs=. -target=target-gba.json -o audiostereostream.gba ./cmd/audiostereostream 2>audiostereostream.allocs
	go run ./internal/cmd/checkallocs audiostereostream.allocs

audiostereostream-elf: generate
	tinygo build -gc=none -opt=2 -ldflags="-extldflags=-Map=audiostereostream.map" -print-allocs=. -target=target-gba.json -o audiostereostream.elf ./cmd/audiostereostream 2>audiostereostream.allocs
	go run ./internal/cmd/checkallocs audiostereostream.allocs

# audiostereocapture runs two spectrally distinct hard-panned voices for
# internal mGBA callback capture and host-reference correlation.
audiostereocapture-gba: generate
	tinygo build -gc=none -opt=2 -print-allocs=. -target=target-gba.json -o audiostereocapture.gba ./cmd/audiostereocapture 2>audiostereocapture.allocs
	go run ./internal/cmd/checkallocs audiostereocapture.allocs

audiostereocapture-elf: generate
	tinygo build -gc=none -opt=2 -ldflags="-extldflags=-Map=audiostereocapture.map" -print-allocs=. -target=target-gba.json -o audiostereocapture.elf ./cmd/audiostereocapture 2>audiostereocapture.allocs
	go run ./internal/cmd/checkallocs audiostereocapture.allocs

# audiocoexist runs the graphics matrix with the interrupt-assisted stream,
# including a separate forced-starvation and recovery gate.
audiocoexist-gba: generate
	tinygo build -gc=none -opt=2 -print-allocs=. -target=target-gba.json -o audiocoexist.gba ./cmd/audiocoexist 2>audiocoexist.allocs
	go run ./internal/cmd/checkallocs audiocoexist.allocs

audiocoexist-elf: generate
	tinygo build -gc=none -opt=2 -ldflags="-extldflags=-Map=audiocoexist.map" -print-allocs=. -target=target-gba.json -o audiocoexist.elf ./cmd/audiocoexist 2>audiocoexist.allocs
	go run ./internal/cmd/checkallocs audiocoexist.allocs

# The PCM build runs the identical matrix with four synthesized music voices
# plus one looping native sample in the production six-voice mono profile.
audiopcmcoexist-gba: generate
	tinygo build -gc=none -opt=2 -tags=pcm -print-allocs=. -target=target-gba.json -o audiopcmcoexist.gba ./cmd/audiocoexist 2>audiopcmcoexist.allocs
	go run ./internal/cmd/checkallocs audiopcmcoexist.allocs

audiopcmcoexist-elf: generate
	tinygo build -gc=none -opt=2 -tags=pcm -ldflags="-extldflags=-Map=audiopcmcoexist.map" -print-allocs=. -target=target-gba.json -o audiopcmcoexist.elf ./cmd/audiocoexist 2>audiopcmcoexist.allocs
	go run ./internal/cmd/checkallocs audiopcmcoexist.allocs

# Intermediate active-source profile used to identify the measured boundary
# between the full and two-synth native-sample workloads.
audiopcmthreecoexist-gba: generate
	tinygo build -gc=none -opt=2 -tags=pcm,pcmthree -print-allocs=. -target=target-gba.json -o audiopcmthreecoexist.gba ./cmd/audiocoexist 2>audiopcmthreecoexist.allocs
	go run ./internal/cmd/checkallocs audiopcmthreecoexist.allocs

audiopcmthreecoexist-elf: generate
	tinygo build -gc=none -opt=2 -tags=pcm,pcmthree -ldflags="-extldflags=-Map=audiopcmthreecoexist.map" -print-allocs=. -target=target-gba.json -o audiopcmthreecoexist.elf ./cmd/audiocoexist 2>audiopcmthreecoexist.allocs
	go run ./internal/cmd/checkallocs audiopcmthreecoexist.allocs

# The two-synth PCM candidate retains the generated six-voice pool while
# limiting the active workload to two music channels plus one native sample.
audiopcmtwocoexist-gba: generate
	tinygo build -gc=none -opt=2 -tags=pcm,pcmtwo -print-allocs=. -target=target-gba.json -o audiopcmtwocoexist.gba ./cmd/audiocoexist 2>audiopcmtwocoexist.allocs
	go run ./internal/cmd/checkallocs audiopcmtwocoexist.allocs

audiopcmtwocoexist-elf: generate
	tinygo build -gc=none -opt=2 -tags=pcm,pcmtwo -ldflags="-extldflags=-Map=audiopcmtwocoexist.map" -print-allocs=. -target=target-gba.json -o audiopcmtwocoexist.elf ./cmd/audiocoexist 2>audiopcmtwocoexist.allocs
	go run ./internal/cmd/checkallocs audiopcmtwocoexist.allocs

# The stereo build uses the identical eight-case graphics/recovery matrix with
# two hard-panned music channels and the dual-FIFO StereoStream.
audiostereocoexist-gba: generate
	tinygo build -gc=none -opt=2 -tags=stereo -print-allocs=. -target=target-gba.json -o audiostereocoexist.gba ./cmd/audiocoexist 2>audiostereocoexist.allocs
	go run ./internal/cmd/checkallocs audiostereocoexist.allocs

audiostereocoexist-elf: generate
	tinygo build -gc=none -opt=2 -tags=stereo -ldflags="-extldflags=-Map=audiostereocoexist.map" -print-allocs=. -target=target-gba.json -o audiostereocoexist.elf ./cmd/audiocoexist 2>audiostereocoexist.allocs
	go run ./internal/cmd/checkallocs audiostereocoexist.allocs
