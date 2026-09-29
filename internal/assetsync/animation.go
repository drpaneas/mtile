package assetsync

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// AnimationWorkspace is editor metadata. Frames retain their global pose IDs;
// pages describe where artwork is stored, independently of playback order.
type AnimationWorkspace struct {
	Version int             `json:"version"`
	Pages   []AnimationPage `json:"pages"`
	Clips   []AnimationClip `json:"clips"`
}

type AnimationPage struct {
	Name   string `json:"name"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type AnimationClip struct {
	Name   string `json:"name"`
	Frames []int  `json:"frames"`
	FPS    int    `json:"fps"`
	Loop   *bool  `json:"loop,omitempty"`
}

func LoadAnimationWorkspace(projectDir string) (*AnimationWorkspace, error) {
	path := filepath.Join(projectDir, "assets", "animation.json")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return &AnimationWorkspace{Version: 1}, nil
	} else if err != nil {
		return nil, fmt.Errorf("stat animation.json: %w", err)
	}
	var workspace AnimationWorkspace
	if err := readJSON(path, &workspace); err != nil {
		return nil, fmt.Errorf("read animation.json: %w", err)
	}
	if err := validateAnimationFile(workspace); err != nil {
		return nil, err
	}
	return &workspace, nil
}

func SaveAnimationWorkspace(projectDir string, workspace *AnimationWorkspace) error {
	if workspace == nil {
		return fmt.Errorf("animation workspace is required")
	}
	normalized := *workspace
	if normalized.Version == 0 {
		normalized.Version = 1
	}
	if err := validateAnimationFile(normalized); err != nil {
		return err
	}
	path := filepath.Join(projectDir, "assets", "animation.json")
	if len(normalized.Pages) == 0 && len(normalized.Clips) == 0 {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return nil
		} else if err != nil {
			return fmt.Errorf("stat animation.json: %w", err)
		}
	}
	return writeJSON(path, normalized)
}

func animationNameValid(name string) bool {
	if name == "" || strings.TrimSpace(name) != name || len(name) > MaxPoseNameBytes || !utf8.ValidString(name) {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func validateAnimationFile(workspace AnimationWorkspace) error {
	width, height := 0, 0
	for _, page := range workspace.Pages {
		// Check each operand before adding untrusted JSON coordinates.
		if page.X < 0 || page.Y < 0 || page.Width < 8 || page.Height < 8 || page.X > MaxObjSheetCells*8-page.Width || page.Y > MaxObjSheetCells*8-page.Height {
			return fmt.Errorf("animation.json: page %q has invalid coordinates or dimensions", page.Name)
		}
		width, height = max(width, page.X+page.Width), max(height, page.Y+page.Height)
	}
	return ValidateAnimationWorkspace(workspace, width, height, MaxPoseCount)
}

func ValidateAnimationWorkspace(workspace AnimationWorkspace, sheetW, sheetH, poseCount int) error {
	if workspace.Version != 1 {
		return fmt.Errorf("animation.json: version %d is unsupported (want 1)", workspace.Version)
	}
	if poseCount < 0 || poseCount > MaxPoseCount {
		return fmt.Errorf("animation.json: invalid pose count %d", poseCount)
	}
	var occupied []bool
	if len(workspace.Pages) > 0 {
		if err := ValidateObjSheetDimensions(sheetW, sheetH); err != nil {
			return fmt.Errorf("animation.json: pages require a valid animation sheet: %w", err)
		}
		occupied = make([]bool, (sheetW/8)*(sheetH/8))
	}
	names := make(map[string]bool)
	for _, page := range workspace.Pages {
		if !animationNameValid(page.Name) || names[page.Name] {
			return fmt.Errorf("animation.json: page names must be unique, nonempty, and at most %d bytes: %q", MaxPoseNameBytes, page.Name)
		}
		names[page.Name] = true
		if page.X < 0 || page.Y < 0 || page.Width < 8 || page.Height < 8 || page.X%8 != 0 || page.Y%8 != 0 || page.Width%8 != 0 || page.Height%8 != 0 || page.Width > sheetW || page.Height > sheetH || page.X > sheetW-page.Width || page.Y > sheetH-page.Height {
			return fmt.Errorf("animation.json: page %q must be tile-aligned and inside the animation sheet", page.Name)
		}
		for y := page.Y / 8; y < (page.Y+page.Height)/8; y++ {
			for x := page.X / 8; x < (page.X+page.Width)/8; x++ {
				cell := y*(sheetW/8) + x
				if occupied[cell] {
					return fmt.Errorf("animation.json: page %q overlaps another page", page.Name)
				}
				occupied[cell] = true
			}
		}
	}
	names = make(map[string]bool)
	for _, clip := range workspace.Clips {
		if !animationNameValid(clip.Name) || names[clip.Name] {
			return fmt.Errorf("animation.json: clip names must be unique, nonempty, and at most %d bytes: %q", MaxPoseNameBytes, clip.Name)
		}
		names[clip.Name] = true
		if clip.FPS < 1 || clip.FPS > 60 {
			return fmt.Errorf("animation.json: clip %q speed must be 1..60 FPS", clip.Name)
		}
		for index, id := range clip.Frames {
			if id < 0 || id >= poseCount {
				return fmt.Errorf("animation.json: clip %q frame %d refers to missing pose %d", clip.Name, index, id)
			}
		}
	}
	return nil
}
