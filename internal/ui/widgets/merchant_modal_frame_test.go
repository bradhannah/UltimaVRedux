package widgets

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/sprites"
)

func TestMerchantModalFrameResponsiveGeometryAndDrawing(t *testing.T) {
	setMerchantShellTestResolution(t, 320, 180)
	frame := newMerchantModalFrame(merchantModalFrameConfig{
		placement: sprites.PercentBasedPlacement{
			StartPercentX: 0.25,
			EndPercentX:   0.75,
			StartPercentY: 0.12,
			EndPercentY:   0.88,
		},
		headerRatio:           0.13,
		footerRatio:           0.16,
		minimumInset:          5,
		lineSpacingMultiplier: 1.08,
		footerMinimumLines:    2,
	})

	require.True(t, frame.ensureLayout())
	require.Equal(t, image.Rect(80, 21, 240, 158), frame.bounds())
	require.False(t, frame.ensureLayout(), "unchanged dimensions must retain derived resources")
	requireMerchantModalFrameRegions(t, frame)

	screen := ebiten.NewImage(320, 180)
	frame.draw(screen)
	requireColorRGBA(t, MerchantDialogBackgroundColor, screen.At(frame.headerText.Min.X, frame.headerText.Min.Y))
	requireImageRegionContainsColor(t, screen, frame.headerBounds, DialogRuleColor)
	requireImageRegionContainsColor(t, screen, frame.bodyBounds, DialogRuleColor)
	requireImageRegionContainsColor(t, screen, frame.footerBounds, DialogRuleColor)
	require.Equal(t, frame.headerBounds.Max.Y, frame.bodyBounds.Min.Y)
	require.Equal(t, frame.bodyBounds.Max.Y, frame.footerBounds.Min.Y)
	outerFramePoint := image.Pt(frame.headerBounds.Min.X+frame.headerBounds.Dx()/2, frame.headerBounds.Min.Y)
	requireColorRGBA(t, DialogRuleColor, screen.At(outerFramePoint.X, outerFramePoint.Y))
	paneCorner := frame.headerBounds.Min
	requireColorNotRGBA(t, MerchantDialogBackgroundColor, screen.At(paneCorner.X, paneCorner.Y))
	legacyOuter := frame.bounds()
	legacyOuterPoint := image.Pt(legacyOuter.Min.X+legacyOuter.Dx()/2, legacyOuter.Min.Y)
	requireColorNotRGBA(t, DialogRuleColor, screen.At(legacyOuterPoint.X, legacyOuterPoint.Y))

	setMerchantShellTestResolution(t, 1280, 720)
	require.True(t, frame.ensureLayout())
	require.Equal(t, image.Rect(320, 86, 960, 633), frame.bounds())
	requireMerchantModalFrameRegions(t, frame)
}

func TestMerchantModalFrameRejectsInvalidGeometryConfiguration(t *testing.T) {
	require.Panics(t, func() {
		newMerchantModalFrame(merchantModalFrameConfig{})
	})
}

func requireMerchantModalFrameRegions(t *testing.T, frame *merchantModalFrame) {
	t.Helper()
	bounds := frame.bounds()
	require.False(t, frame.headerBounds.Empty())
	require.False(t, frame.bodyBounds.Empty())
	require.False(t, frame.footerBounds.Empty())
	require.True(t, frame.headerBounds.In(bounds))
	require.True(t, frame.bodyBounds.In(bounds))
	require.True(t, frame.footerBounds.In(bounds))
	require.Equal(t, frame.headerBounds.Max.Y, frame.bodyBounds.Min.Y)
	require.Equal(t, frame.bodyBounds.Max.Y, frame.footerBounds.Min.Y)
	require.True(t, frame.headerText.In(frame.headerBounds))
	require.True(t, frame.bodyText.In(frame.bodyBounds))
	require.True(t, frame.footerText.In(frame.footerBounds))
}
