//go:build tinygo

package main

import "github.com/drpaneas/mtile/runtime/internal/audioref"

func main() { audioref.Run(audioref.FIFOA) }
