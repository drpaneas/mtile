//go:build !tinygo

package runtime

import "math"

// FromFloat is for the desktop editor and host tests. Game ROMs never need
// floating point support.
func FromFloat(f float64) Number {
	return Number(math.Round(f * float64(One)))
}
