package build

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/transpiler"
)

func TestTileCanvasEditorExperimentIsProjectScoped(t *testing.T) {
	t.Setenv("MTILE_EXPERIMENTAL_TILECANVAS_OBJECTS", "")
	selected, other := t.TempDir(), t.TempDir()
	initial := Service{UseProjectRenderer: true}
	for _, scope := range []string{"", other, filepath.Join(other, "missing")} {
		t.Setenv("MTILE_EXPERIMENTAL_TILECANVAS_PROJECT", scope)
		service := initial
		service.configureTileCanvasExperiment(selected)
		if !reflect.DeepEqual(service, initial) {
			t.Fatalf("scope %q changed unrelated build policy: %+v", scope, service)
		}
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(selected, alias); err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{selected, alias} {
		t.Setenv("MTILE_EXPERIMENTAL_TILECANVAS_PROJECT", scope)
		service := initial
		service.configureTileCanvasExperiment(selected)
		renderer, err := service.resolveRenderer(assetsync.Project{Renderer: "auto"})
		if err != nil || renderer != transpiler.RendererMode4 || !service.DisableMapRasterCache || !service.tileCanvasExperiment {
			t.Fatalf("scope %q did not enable the private build: %+v renderer=%v error=%v", scope, service, renderer, err)
		}
		report := transpiler.RendererReport{Selected: renderer, HeapLinked: true, SheetReadOnly: true}
		tags := service.runtimeBuildTags(report)
		if tags != runtimeBuildTags(report)+",mtile_tilecanvas" || strings.Contains(tags, "mtile_mode0") {
			t.Fatalf("experiment lost ordinary specialization or enabled public Mode 0: %s", tags)
		}
		// Build configures a value receiver. Its next invocation/project uses
		// the original policy instead of retaining this local override.
		if initial.tileCanvasExperiment || initial.DisableMapRasterCache || !initial.UseProjectRenderer {
			t.Fatal("experiment mutated the reusable caller service")
		}
	}
}

func TestLegacyObjectsOptInDoesNotAddSceneSpecificRuntime(t *testing.T) {
	selected, other := t.TempDir(), t.TempDir()
	t.Setenv("MTILE_EXPERIMENTAL_TILECANVAS_PROJECT", selected)
	t.Setenv("MTILE_EXPERIMENTAL_TILECANVAS_OBJECTS", "1")
	report := transpiler.RendererReport{Selected: transpiler.RendererMode4, SheetReadOnly: true}
	for _, project := range []string{selected, other} {
		service := Service{UseProjectRenderer: true}
		service.configureTileCanvasExperiment(project)
		tags := service.runtimeBuildTags(report)
		if strings.Contains(tags, "mtile_tilecanvas_obj") || strings.Contains(tags, "mtile_tilecanvas") != (project == selected) {
			t.Fatalf("project %q has incorrect runtime tags %q", project, tags)
		}
	}
}

func TestPreviewRuntimeLoggingIsOptIn(t *testing.T) {
	for _, renderer := range []transpiler.Renderer{transpiler.RendererMode0, transpiler.RendererMode4} {
		report := transpiler.RendererReport{Selected: renderer}
		for _, debug := range []bool{false, true} {
			service := Service{Debug: debug}
			tags := service.runtimeBuildTags(report)
			if strings.Contains(tags, "mtile_preview_log") != debug || strings.Contains(tags, "mgbadebug") {
				t.Fatalf("debug=%v renderer=%v: unexpected tags %q", debug, renderer, tags)
			}
		}
	}
}
