package widgets

import (
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	tavernNoticePaddingUnits = 4
	tavernNoticeSides        = 2
	tavernNoticeVerticalGaps = 3
)

// TavernAdviceNotice is an acknowledged response centered in the merchant's
// right-hand workspace. Its owning Talk workflow removes the exact instance.
type TavernAdviceNotice struct {
	keyboard                 *input.Keyboard
	pane                     func() image.Rectangle
	message                  string
	isKeyPressed             func(ebiten.Key) bool
	keysReady                bool
	dismissed                bool
	viewport                 image.Rectangle
	width, height            int
	bounds, body, footer     image.Rectangle
	bodyOutput, footerOutput *text.Output
}

// NewTavernAdviceNotice retains the pane provider so the same modal reflows on resize.
func NewTavernAdviceNotice(keyboard *input.Keyboard, pane func() image.Rectangle, message string) *TavernAdviceNotice {
	if keyboard == nil || pane == nil || message == "" {
		panic("TavernAdviceNotice: keyboard, pane and message are required")
	}
	notice := &TavernAdviceNotice{ //nolint:exhaustruct_v5 // Measured presentation initializes below.
		keyboard: keyboard, pane: pane, message: message, isKeyPressed: ebiten.IsKeyPressed,
		keysReady: tavernResultControlsReleased(ebiten.IsKeyPressed),
	}
	notice.ensureLayout()

	return notice
}

// Draw presents the unchanged merchant response and standard dismissal guidance.
func (n *TavernAdviceNotice) Draw(screen *ebiten.Image) {
	if screen == nil || n.dismissed {
		return
	}
	n.ensureLayout()
	unit := getClassicUIPixelUnit(n.width, n.height)
	FillClassicPixelRoundedRect(screen, n.bounds, unit, MerchantDialogBackgroundColor)
	DrawClassicPixelRoundedFrame(screen, n.bounds, unit, DialogRuleColor)
	n.bodyOutput.DrawPanelContent(screen, n.body.Min, n.body.Dx(), etext.AlignCenter)
	n.footerOutput.DrawPanelContent(screen, n.footer.Min, n.footer.Dx(), etext.AlignCenter)
}

// Update requires release of the submitting key before Enter or Escape can dismiss.
func (n *TavernAdviceNotice) Update() {
	if n.dismissed {
		return
	}
	if !n.keysReady {
		if !tavernResultControlsReleased(n.isKeyPressed) {
			return
		}
		n.keysReady = true
	}
	if n.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) ||
		n.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter) ||
		n.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		n.Dismiss()
	}
}

// Dismiss leaves stack removal and merchant-session cleanup to Talk.
func (n *TavernAdviceNotice) Dismiss() {
	if !n.dismissed {
		n.keyboard.PrepareForTransition(0)
		n.dismissed = true
	}
}

// IsDismissed reports whether the workflow may remove this notice.
func (n *TavernAdviceNotice) IsDismissed() bool { return n.dismissed }

// GetBounds returns the current pane-centered measured bounds.
func (n *TavernAdviceNotice) GetBounds() image.Rectangle {
	n.ensureLayout()

	return n.bounds
}

func (n *TavernAdviceNotice) ensureLayout() {
	width, height := display.GetManager().GetCurrentSize()
	pane := n.pane()
	if n.bodyOutput != nil && pane == n.viewport && width == n.width && height == n.height {
		return
	}
	n.width, n.height, n.viewport = width, height, pane
	padding := getClassicUIPixelUnit(width, height) * tavernNoticePaddingUnits
	textWidth := max(1, pane.Dx()-padding*tavernNoticeSides*tavernNoticeSides)
	font, metrics := GetScaledFontWithOutput(FontConfig{
		Size: FontSizeBody, LineSpacing: 0, LineSpacingMultiplier: tavernShopLineSpacing,
		MaxLines: 1, MaxChars: 1,
	})
	chars := max(1, font.CharsForWidth(textWidth))
	lines := text.MeasureText(n.message, font, metrics.GetLineSpacing(), chars).Lines
	n.bodyOutput = text.NewOutput(font, metrics.GetLineSpacing(), max(1, lines), chars)
	n.bodyOutput.AddTextWithColor(n.message, DialogValueColor)
	footerLines := text.MeasureText(DialogCloseHint, font, metrics.GetLineSpacing(), chars).Lines
	n.footerOutput = text.NewOutput(font, metrics.GetLineSpacing(), max(1, footerLines), chars)
	AppendControlHints(n.footerOutput.AppendLine(), DialogCloseHint).Done()
	bodyHeight := int(math.Ceil(n.bodyOutput.GetTotalHeightForLines(max(1, lines))))
	footerHeight := int(math.Ceil(n.footerOutput.GetTotalHeightForLines(max(1, footerLines))))
	boxWidth, boxHeight := textWidth+padding*tavernNoticeSides, bodyHeight+footerHeight+padding*tavernNoticeVerticalGaps
	left := pane.Min.X + (pane.Dx()-boxWidth)/tavernNoticeSides
	top := pane.Min.Y + (pane.Dy()-boxHeight)/tavernNoticeSides
	n.bounds = image.Rect(left, top, left+boxWidth, top+boxHeight)
	n.body = image.Rect(left+padding, top+padding, left+padding+textWidth, top+padding+bodyHeight)
	n.footer = image.Rect(n.body.Min.X, n.body.Max.Y+padding, n.body.Max.X, n.body.Max.Y+padding+footerHeight)
}
