package build

import (
	"os"

	"github.com/drpaneas/mtile/internal/transpiler"
)

// The edit command scopes this opt-in to one project in its child process.
// Switching projects in that editor must not carry the experiment into an
// unrelated cartridge. This is not a project renderer value or AUTO profile.
func (s *Service) configureTileCanvasExperiment(projectDir string) {
	selected := os.Getenv("MTILE_EXPERIMENTAL_TILECANVAS_PROJECT")
	if selected == "" {
		return
	}
	wanted, wantErr := os.Stat(selected)
	actual, actualErr := os.Stat(projectDir)
	if wantErr != nil || actualErr != nil || !wanted.IsDir() || !os.SameFile(wanted, actual) {
		return
	}
	s.tileCanvasExperiment = true
	s.Renderer = transpiler.RendererMode4
	s.UseProjectRenderer = false
	s.DisableMapRasterCache = true
}

func (s Service) runtimeBuildTags(report transpiler.RendererReport) string {
	tags := runtimeBuildTags(report)
	if s.tileCanvasExperiment {
		if tags != "" {
			tags += ","
		}
		tags += "mtile_tilecanvas"
	}
	return tags
}
