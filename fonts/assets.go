// Package fonts exposes the font assets used by the mtile editor and runtime
// tooling. Keeping the embed here lets the editor load the checked-in font
// without making the UI depend on a machine-installed font.
package fonts

import _ "embed"

// OpenDyslexic is the legacy monospaced OpenDyslexic asset. The source editor
// uses JetBrains Mono; this remains available to projects and tooling that explicitly
// need the accessibility-oriented face.
//
//go:embed OpenDyslexic/OpenDyslexicMNerdFontMono-Regular.otf
var OpenDyslexic []byte

// OpenDyslexicUIBold is the proportional bold face used outside the Develop
// workspace, where labels and controls keep MTile's distinctive visual identity.
//
//go:embed OpenDyslexic/OpenDyslexicNerdFont-Bold.otf
var OpenDyslexicUIBold []byte

// JetBrainsMonoRegular is the neutral sans-serif monospaced face used for code,
// logs, and the surrounding Develop workspace chrome.
//
//go:embed JetBrainsMono/JetBrainsMono-Regular.ttf
var JetBrainsMonoRegular []byte
