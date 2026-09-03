//go:build !mtile_map_compact && !mtile_map_streaming

package runtime

// Host tests and hand-written runtime programs retain the dynamic path. Every
// generated cartridge selects exactly one fixed storage tag at link time.
const (
	mapStorageFixed     = false
	mapStorageStreaming = false
)
