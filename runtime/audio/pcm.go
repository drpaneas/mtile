package audio

// PCMController owns an imported Maxmod bank. The bank and ID map are immutable
// ROM arrays for the complete cartridge lifetime. It shares no Lua heap state.
type PCMController struct {
	ready     bool
	modules   []uint16
	samples   []uint16
	frames    uint32
	maxCycles uint32
}

type PCMStats struct {
	Ready                bool
	Frames, MaxMixCycles uint32
}

func (p *PCMController) Init(bank []uint32, music, samples []uint16) bool {
	if p == nil || p.ready || len(bank) < 3 || bank[1] != 0x78616d2a || bank[2] != 0x2a646f6d {
		return false
	}
	count := int(bank[0] & 0xffff)
	modules := int(bank[0] >> 16)
	if modules > 256 || len(music) > 256 || len(samples) > 256 || len(bank) < 3+count+modules {
		return false
	}
	for _, id := range samples {
		if id != 65535 && int(id) >= count {
			return false
		}
	}
	activeMusic := 0
	for _, id := range music {
		if id == 65535 {
			continue
		}
		activeMusic++
		if int(id) >= modules {
			return false
		}
	}
	if activeMusic != modules {
		return false
	}
	if !p.initPCM(bank) {
		return false
	}
	p.modules, p.samples, p.ready = music, samples, true
	p.startPCM()
	return true
}

func (p *PCMController) Music(id int, loop bool) {
	if p != nil && p.ready && id >= -1 && id < len(p.modules) {
		if id >= 0 {
			id = int(p.modules[id])
			if id == 65535 {
				return
			}
		}
		p.musicPCM(id, loop)
	}
}

// Sample uses the public WAV ID map, excluding instruments embedded in songs.
func (p *PCMController) Sample(id, volume, priority int) {
	if p == nil || !p.ready || id < -1 || id >= len(p.samples) || volume < 0 || volume > 255 || priority < 0 || priority > 255 {
		return
	}
	if id >= 0 {
		id = int(p.samples[id])
		if id == 65535 {
			return
		}
	}
	p.samplePCM(id, volume, priority)
}

func (p *PCMController) PCMStats() PCMStats {
	if p == nil {
		return PCMStats{}
	}
	state := disableAudioInterrupts()
	stats := PCMStats{p.ready, p.frames, p.maxCycles}
	restoreAudioInterrupts(state)
	return stats
}
