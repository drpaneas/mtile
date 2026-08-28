//go:build !darwin || !cgo

package midicontrol

import "errors"

func NewSystemBackend() (Backend, error) {
	return nil, errors.New("native MIDI is currently available on macOS with cgo")
}
