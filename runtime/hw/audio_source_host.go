//go:build !tinygo

package hw

func validFIFOSourceRange(samples []uint32) bool {
	return len(samples) > 0
}
