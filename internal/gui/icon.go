package gui

import (
	"fyne.io/fyne/v2"

	"github.com/ushineko/enshroudedbonker/internal/gui/assets"
)

func appIcon() fyne.Resource {
	return fyne.NewStaticResource("enshroudedbonker.svg", assets.IconSVG())
}
