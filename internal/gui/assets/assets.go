// Package assets embeds the window icon.
package assets

import _ "embed"

//go:embed enshroudedbonker.svg
var icon []byte

// IconSVG is the window and launcher icon. packaging/enshroudedbonker.svg is
// the same drawing, on disk for the installer.
func IconSVG() []byte { return icon }
