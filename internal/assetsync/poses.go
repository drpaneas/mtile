package assetsync

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func LoadPoses(projectDir string) (*PoseFile, error) {
	path := filepath.Join(projectDir, "assets", "poses.json")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return &PoseFile{Version: 1}, nil
	} else if err != nil {
		return nil, fmt.Errorf("stat poses.json: %w", err)
	}
	var file PoseFile
	if err := readJSON(path, &file); err != nil {
		return nil, fmt.Errorf("read poses.json: %w", err)
	}
	if file.Version != 1 {
		return nil, fmt.Errorf("poses.json: version %d is unsupported (want 1)", file.Version)
	}
	if len(file.Poses) > MaxPoseCount {
		return nil, fmt.Errorf("poses.json: %d poses exceeds %d", len(file.Poses), MaxPoseCount)
	}
	names := make(map[string]int, len(file.Poses))
	total := 0
	for i, pose := range file.Poses {
		if pose.Name != "" {
			if prev, exists := names[pose.Name]; exists {
				return nil, fmt.Errorf("poses.json: duplicate pose name %q (indices %d and %d)", pose.Name, prev, i)
			}
			names[pose.Name] = i
		}
		if len(pose.Parts) > MaxPoseParts {
			return nil, fmt.Errorf("poses.json: pose %d has %d parts (maximum %d)", i, len(pose.Parts), MaxPoseParts)
		}
		total += len(pose.Parts)
		if total > MaxPosePartTotal {
			return nil, fmt.Errorf("poses.json: more than %d parts", MaxPosePartTotal)
		}
	}
	return &file, nil
}
