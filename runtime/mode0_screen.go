//go:build mtile_mode0 && !mtile_mode0_layers0

package runtime

type mode0ScreenStorage [mode0LayerStorageCount][4096]byte

type mode0TileDirtyStorage [120]byte
