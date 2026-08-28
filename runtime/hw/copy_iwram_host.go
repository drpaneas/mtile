//go:build !tinygo

package hw

func CopyToVRAM32IWRAM(src []byte, dest uintptr, words int) {
	CopyToVRAM32CPU(src, dest, words)
}
