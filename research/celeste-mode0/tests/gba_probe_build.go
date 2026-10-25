//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/drpaneas/mtile/internal/build"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: go run gba_probe_build.go PROJECT OUTPUT.gba")
		os.Exit(2)
	}

	// The verifier supplies a temporary project copy. Override only that copy,
	// then use the same validated project configuration as a production build.
	if selected, ok := os.LookupEnv("CELESTE_PROBE_IWRAM"); ok {
		path := filepath.Join(os.Args[1], "project.json")
		data, err := os.ReadFile(path)
		if err != nil {
			panic(err)
		}
		var config map[string]any
		if err := json.Unmarshal(data, &config); err != nil {
			panic(err)
		}
		var names []string
		if selected != "" {
			names = strings.Split(selected, ",")
		}
		config["iwramFunctions"] = names
		data, err = json.MarshalIndent(config, "", "  ")
		if err != nil {
			panic(err)
		}
		if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
			panic(err)
		}
	}
	service, err := build.NewService("", "")
	if err != nil {
		panic(err)
	}
	service.Debug = true
	service.KeepWork = true
	service.UseProjectRenderer = true
	service.DisableStableWriteSpecialization = os.Getenv("CELESTE_DISABLE_STABLE_WRITE_SPECIALIZATION") == "1"
	service.DisableConditionSpecialization = os.Getenv("CELESTE_DISABLE_CONDITION_SPECIALIZATION") == "1"
	service.DisableConstantDivision = os.Getenv("CELESTE_DISABLE_CONSTANT_DIVISION") == "1"
	result, err := service.Build(os.Args[1], os.Args[2])
	if err != nil {
		panic(err)
	}

	fmt.Printf("IWRAM %d/%d bytes\n", result.IWRAMUsed, build.IWRAMCapacity)
	fmt.Println("workdir", result.WorkDir)
	fmt.Printf("probe ROM %s (%d bytes), EWRAM free %d\n", result.ROMPath, result.ROMSize, result.EWRAMFree)
}
