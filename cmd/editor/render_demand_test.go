package main

import "testing"

func TestDynamicSurfaceActivity(t *testing.T) {
	tests := []struct {
		name      string
		game      *Game
		active    bool
		everyTick bool
	}{
		{name: "idle", game: &Game{}},
		{name: "theme pulse", game: &Game{theme: themeState{buildPulse: 1}}, active: true},
		{name: "running emulator", game: &Game{mode: modeCode, playStop: make(chan struct{})}, active: true, everyTick: true},
		{name: "paused emulator", game: &Game{mode: modeCode, playStop: make(chan struct{}), playPaused: true}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			active, everyTick := test.game.dynamicSurfaceActivity()
			if active != test.active || everyTick != test.everyTick {
				t.Fatalf("dynamicSurfaceActivity() = (%v, %v), want (%v, %v)", active, everyTick, test.active, test.everyTick)
			}
		})
	}
}
