//go:build mtile_mode0 && mtile_mode0_obj && mtile_mode0_no_pspr

package runtime

// The compiler proved there are no pose draws. Ordinary sprites and text
// therefore need no pose-site or tile-window bookkeeping in their Context.
const (
	mode0PsprWindowLimit = 0
	mode0PsprSiteLimit   = 0
)
