// Package fonts exposes the font assets used by the mtile editor and runtime
// tooling. Keeping the embed here lets the editor load the checked-in font
// without making the UI depend on a machine-installed font.
package fonts

import _ "embed"

// OpenDyslexic is the monospaced OpenDyslexic font used by the editor UI.
// The Mono variant keeps the source editor's character grid stable.
//
//go:embed OpenDyslexic/OpenDyslexicMNerdFontMono-Regular.otf
var OpenDyslexic []byte

// OpenDyslexicUIBold is the proportional bold face used for editor chrome.
// Code and logs keep the monospaced face above; labels and controls use the
// compact proportional face so they do not inherit terminal-like spacing.
//
//go:embed OpenDyslexic/OpenDyslexicNerdFont-Bold.otf
var OpenDyslexicUIBold []byte
