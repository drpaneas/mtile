module github.com/drpaneas/mtile

go 1.27.1

toolchain go1.27.3

require (
	github.com/drpaneas/mtile/runtime v0.0.0
	github.com/ebitengine/oto/v3 v3.5.0
	github.com/hajimehoshi/ebiten/v2 v2.10.2
	github.com/rivo/uniseg v0.4.7
	gitlab.com/gomidi/midi/v2 v2.3.23
	golang.design/x/clipboard v0.9.0
	golang.org/x/image v0.45.0
)

replace github.com/drpaneas/mtile/runtime => ./runtime

require (
	github.com/ebitengine/gomobile v0.0.0-20260820040257-d11f821a26a6 // indirect
	github.com/ebitengine/hideconsole v1.0.0 // indirect
	github.com/ebitengine/purego v0.11.0 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	github.com/jezek/xgb v1.1.1 // indirect
	github.com/jfreymuth/pulse v0.1.3 // indirect
	github.com/srwiley/rasterx v0.0.0-20220730225603-2ab79fcdd4ef // indirect
	golang.design/x/x11 v0.2.0 // indirect
	golang.org/x/exp/shiny v0.0.0-20250606033433-dcc06ee1d476 // indirect
	golang.org/x/mobile v0.0.0-20250606033058-a2a15c67f36f // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
)
