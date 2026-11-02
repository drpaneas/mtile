package main

import (
	"fmt"
	"strings"

	"github.com/drpaneas/mtile/internal/audioimport"
)

func pcmQualityName(q audioimport.Quality) string {
	switch q {
	case audioimport.QualitySmall:
		return "Smallest"
	case audioimport.QualitySource:
		return "Keep Source"
	default:
		return "Balanced"
	}
}
func pcmSavedPercent(before, after int) float64 {
	if before <= 0 {
		return 0
	}
	return 100 * float64(before-after) / float64(before)
}
func pcmProfileLabel(p audioimport.Prepared, q audioimport.Quality) string {
	bytes := p.Analysis.PreparedBytes(q, p.Trimmed)
	label := strings.ToUpper(pcmQualityName(q)) + " / " + pcmBytes(bytes)
	if bytes < p.OriginalBytes {
		label += fmt.Sprintf(" / %.0f%% smaller than original", pcmSavedPercent(p.OriginalBytes, bytes))
	}
	return label
}

// Recommendations are explicit authoring heuristics, never import failures or
// promises about audibility, fixed mixer RAM, or GBA CPU savings.
func pcmAudioAdvice(p audioimport.Prepared) []string {
	if p.Music {
		return nil
	}
	a := p.Analysis
	if a.Info.Rate <= 0 {
		return nil
	}
	var messages []string
	if a.AllQuiet {
		messages = append(messages, "Entire recording is at or below -60 dBFS. Check the source before importing; trimming is disabled.")
	} else {
		q := audioimport.QualitySmall
		if p.Quality == audioimport.QualitySource {
			q = audioimport.QualityBalanced
		}
		smaller := a.PreparedBytes(q, p.Trimmed)
		saved := len(p.Data) - smaller
		if saved >= 1024 && pcmSavedPercent(len(p.Data), smaller) >= 10 {
			messages = append(messages, fmt.Sprintf("Try %s: %s instead of %s; save %s (%.0f%%). Select it above and audition the difference.", pcmQualityName(q), pcmBytes(smaller), pcmBytes(len(p.Data)), pcmBytes(saved), pcmSavedPercent(len(p.Data), smaller)))
		}
		if a.LoopMarkers {
			messages = append(messages, "Authored WAV loop markers detected: trimming is disabled. Keep Source preserves those markers.")
		} else if a.CanTrim() {
			if p.Trimmed {
				messages = append(messages, fmt.Sprintf("Trim applied: %.2f s removed. This changes trigger timing and total duration; compare Original WAV before saving.", a.RemovedSeconds()))
			} else {
				messages = append(messages, fmt.Sprintf("Quiet edges: %.2f s before / %.2f s after. Optional trim removes %.2f s, retaining 20 ms at each cut. Pauses inside stay intact.", float64(a.LeadingFrames)/float64(a.Info.Rate), float64(a.TrailingFrames)/float64(a.Info.Rate), a.RemovedSeconds()))
			}
		}
	}
	if p.After.Seconds() >= audioimport.LongRecordingSeconds || len(p.Data) >= audioimport.LargeRecordingBytes {
		messages = append(messages, fmt.Sprintf("Long/large recording: %.1f s, %s prepared. Consider a shorter effect or tracker music. This is advice; final ROM usage is checked during build.", p.After.Seconds(), pcmBytes(len(p.Data))))
	}
	return messages
}
