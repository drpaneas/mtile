package assetsync

import (
	"errors"
	"fmt"
	"math"
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
	if err := validatePoseFile(file); err != nil {
		return nil, err
	}
	return &file, nil
}

func SavePoses(projectDir string, file *PoseFile) error {
	if file == nil {
		return fmt.Errorf("poses file is required")
	}
	normalized := *file
	if normalized.Version == 0 {
		normalized.Version = 1
	}
	if err := validatePoseFile(normalized); err != nil {
		return err
	}
	path := filepath.Join(projectDir, "assets", "poses.json")
	if len(normalized.Poses) == 0 {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return nil
		} else if err != nil {
			return fmt.Errorf("stat poses.json: %w", err)
		}
	}
	return writeJSON(path, normalized)
}

func validatePoseFile(file PoseFile) error {
	if file.Version != 1 {
		return fmt.Errorf("poses.json: version %d is unsupported (want 1)", file.Version)
	}
	if len(file.Poses) > MaxPoseCount {
		return fmt.Errorf("poses.json: %d poses exceeds %d", len(file.Poses), MaxPoseCount)
	}
	names := make(map[string]int, len(file.Poses))
	total := 0
	for i, pose := range file.Poses {
		if pose.Name != "" {
			if len(pose.Name) > MaxPoseNameBytes {
				return fmt.Errorf("poses.json: pose %d name is %d bytes (maximum %d)", i, len(pose.Name), MaxPoseNameBytes)
			}
			if prev, exists := names[pose.Name]; exists {
				return fmt.Errorf("poses.json: duplicate pose name %q (indices %d and %d)", pose.Name, prev, i)
			}
			names[pose.Name] = i
		}
		if len(pose.Parts) > MaxPoseParts {
			return fmt.Errorf("poses.json: pose %d has %d parts (maximum %d)", i, len(pose.Parts), MaxPoseParts)
		}
		// Editor loading consumes the raw records before build flattening.
		// Reject unrepresentable fields here so malformed coordinates cannot
		// reach preview bounds or be saved after silent integer truncation.
		for j, part := range pose.Parts {
			if part.Tile < 0 || part.Tile > math.MaxUint16 {
				return fmt.Errorf("poses.json: pose %d part %d tile %d is outside 0..%d", i, j, part.Tile, math.MaxUint16)
			}
			if part.X < math.MinInt16 || part.X > math.MaxInt16 {
				return fmt.Errorf("poses.json: pose %d part %d x %d is outside %d..%d", i, j, part.X, math.MinInt16, math.MaxInt16)
			}
			if part.Y < math.MinInt16 || part.Y > math.MaxInt16 {
				return fmt.Errorf("poses.json: pose %d part %d y %d is outside %d..%d", i, j, part.Y, math.MinInt16, math.MaxInt16)
			}
		}
		total += len(pose.Parts)
		if total > MaxPosePartTotal {
			return fmt.Errorf("poses.json: more than %d parts", MaxPosePartTotal)
		}
	}
	return nil
}
