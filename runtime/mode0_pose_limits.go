//go:build mtile_mode0 && mtile_mode0_obj && !mtile_mode0_no_pspr

package runtime

const (
	mode0PsprWindowLimit = Mode0ObjectTileLimit
	mode0PsprSiteLimit   = 128
)
