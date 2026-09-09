//go:build !tinygo || !mtile_preview_log

package runtime

// DebugLog is enabled only in editor preview ROMs.
func DebugLog(message string) {}
