//go:build tinygo && !mtile_pcm

package audio

func (p *PCMController) initPCM([]uint32) bool   { return false }
func (p *PCMController) startPCM()               {}
func (p *PCMController) musicPCM(int, bool)      {}
func (p *PCMController) samplePCM(int, int, int) {}
