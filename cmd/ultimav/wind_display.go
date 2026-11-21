package main

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	ultimatext "github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

const (
	windTextMaxChars  = 40
	windTextPadding   = 8
	windCenterDivisor = 2
)

// WindDisplay renders the context label at the bottom center of the map.
// The frame-connected end caps are cached graphics, not font glyphs.
type WindDisplay struct {
	font             *ultimatext.UltimaFont
	output           *ultimatext.Output
	background       *ebiten.Image
	textBounds       image.Rectangle
	backgroundBounds image.Rectangle
	padding          int
	verticalPadding  int
	borderStroke     int
	borderBlue       color.RGBA
	lastText         string
	lastScreenWidth  int
	lastScreenHeight int
	visible          bool
}

// NewWindDisplay creates a new wind display overlay component
func NewWindDisplay() *WindDisplay {
	display := new(WindDisplay)
	display.visible = true
	display.ensureLayout()

	return display
}

// DrawLabel shares the wind chrome with ordinary small-map location names.
func (w *WindDisplay) DrawLabel(screen *ebiten.Image, label string) {
	if !w.visible {
		return
	}
	w.ensureLayout()
	w.ensureLabel(label)

	backgroundOptions := new(ebiten.DrawImageOptions)
	backgroundOptions.GeoM.Translate(
		float64(w.backgroundBounds.Min.X),
		float64(w.backgroundBounds.Min.Y),
	)
	screen.DrawImage(w.background, backgroundOptions)
	w.output.DrawPanelContent(screen, w.textBounds.Min, w.textBounds.Dx(), etext.AlignCenter)
}

func (w *WindDisplay) ensureLayout() {
	displayManager := display.GetManager()
	screenWidth, screenHeight := displayManager.GetCurrentSize()
	if screenWidth <= 0 || screenHeight <= 0 {
		return
	}
	if w.output != nil && screenWidth == w.lastScreenWidth && screenHeight == w.lastScreenHeight {
		return
	}

	w.lastScreenWidth = screenWidth
	w.lastScreenHeight = screenHeight
	w.font, w.output = widgets.GetScaledFontWithOutput(widgets.FontConfig{
		Size:                  widgets.FontSizeHUD,
		LineSpacingMultiplier: 0,
		MaxLines:              1,
		MaxChars:              windTextMaxChars,
	})
	outer, aperture := surfaceMapBorderGeometry()
	w.borderStroke, w.borderBlue = windBorderStyle(outer, aperture)
	w.padding = max(1, int(math.Round(ultimatext.GetScaledNumberToResolution(displayManager, windTextPadding))))
	textHeight := int(math.Ceil(w.output.GetFontHeight()))
	bandHeight := outer.Max.Y - aperture.Max.Y
	w.verticalPadding = min(w.padding, max(0, (bandHeight-textHeight)/windCenterDivisor))
	textY := aperture.Max.Y + (bandHeight-textHeight)/windCenterDivisor
	w.textBounds = image.Rect(outer.Min.X, textY, outer.Max.X, textY+textHeight)
	if w.background != nil {
		w.background.Deallocate()
		w.background = nil
	}
	w.lastText = ""
}

func (w *WindDisplay) ensureLabel(label string) {
	if label == w.lastText && w.background != nil {
		return
	}

	w.lastText = label
	w.output.Reset()
	w.output.SetMaxCharsPerLine(max(windTextMaxChars, len([]rune(label))))
	w.output.AppendLine().White(label).Done()
	textWidth, _ := w.font.MeasureString(label)
	backgroundHeight := w.textBounds.Dy() + w.verticalPadding + w.verticalPadding
	capWidth := max(1, backgroundHeight/windCenterDivisor)
	backgroundWidth := min(w.textBounds.Dx(), int(math.Ceil(textWidth))+2*(w.padding+capWidth))
	backgroundX := mapFrameCenteredX(w.textBounds, backgroundWidth)
	backgroundY := w.textBounds.Min.Y - w.verticalPadding
	w.backgroundBounds = image.Rect(
		backgroundX,
		backgroundY,
		backgroundX+backgroundWidth,
		backgroundY+backgroundHeight,
	)
	if w.background != nil {
		w.background.Deallocate()
	}
	w.background = ebiten.NewImage(backgroundWidth, backgroundHeight)
	w.background.Fill(ucolor.EGABlack)
	// The sky uses these identical bookends; keep both tied to the border raster.
	drawMapBorderBookends(w.background, w.borderStroke, w.borderBlue)
}

// Sample only when layout changes, from the same cached raster used by the frame.
func windBorderStyle(outer, aperture image.Rectangle) (int, color.RGBA) {
	borders := sprites.NewBorderSprites()
	frame, _ := borders.VeryPixelatedRoundedBlueWhite.CreateSizedAndScaledBorderSprite(
		borderWidthScaling, gameScreenPercents)

	return mapBorderBandStyle(frame, image.Rect(outer.Min.X, aperture.Max.Y, outer.Max.X, outer.Max.Y))
}

// Update is called each frame for resolution-dependent resource updates.
func (w *WindDisplay) Update() {
	w.ensureLayout()
}

func (g *GameScene) mapBorderLabel() string {
	if !g.usesSurfaceMapFrame() || g.gameState.ShrineScene != nil {
		return ""
	}
	location := g.gameState.MapState.PlayerLocation.Location
	if location.GetMapType() == references.LargeMapType {
		return g.gameState.GetCurrentWind().GetUIString()
	}

	return g.journalLocationLabel(location.String())
}
