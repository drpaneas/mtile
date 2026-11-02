//go:build !tinygo

package audio

// Host unit tests exercise the API and bank ownership; actual playback is
// verified with captured GBA ROM audio, not an unrelated desktop synthesizer.
func (p *PCMController) initPCM([]uint32) bool   { return true }
func (p *PCMController) startPCM()               {}
func (p *PCMController) musicPCM(int, bool)      {}
func (p *PCMController) samplePCM(int, int, int) {}
