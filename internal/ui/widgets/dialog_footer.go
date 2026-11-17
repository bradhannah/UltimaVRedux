package widgets

import (
	"image"
	"math"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/hajimehoshi/ebiten/v2"
)

// DrawKeyboardFooter draws bracketed guidance with the shared hint packer for
// scene-owned dialogs as well as reusable widgets.
func DrawKeyboardFooter(screen *ebiten.Image, content image.Rectangle, guidance string, output *text.Output) {
	drawModifierFooter(screen, content, guidance, output)
}

// FitDialogLine fits a status label without splitting UTF-8 or losing its prefix
// to a wrapped one-row Output. An ellipsis marks any omitted tail.
func FitDialogLine(value string, font *text.UltimaFont, width int) string {
	return fitJournalLine(value, font, width)
}

// CutDialogLine cuts base plainly, with no ellipsis, until base and suffix fit
// width. The suffix, such as the "?" of an uncertain turn, is never cut.
func CutDialogLine(font *text.UltimaFont, base, suffix string, width int) string {
	name := base + suffix
	if measured, _ := font.MeasureString(name); int(math.Ceil(measured)) <= width {
		return name
	}
	runes := []rune(base)
	for len(runes) > 1 {
		runes = runes[:len(runes)-1]
		name = strings.TrimRight(string(runes), " ,;:") + suffix // No dangling space or punctuation.
		if measured, _ := font.MeasureString(name); int(math.Ceil(measured)) <= width {
			break
		}
	}

	return name
}
