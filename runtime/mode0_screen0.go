//go:build mtile_mode0 && mtile_mode0_layers0

package runtime

// The pointer keeps shared, statically unreachable map code type-checkable
// without reserving a 4 KiB BG screen shadow in zero-layer cartridges.
type mode0ScreenStorage [1]*[4096]byte

type mode0TileDirtyStorage [128]byte
