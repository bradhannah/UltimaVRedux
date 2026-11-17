package main

import (
	"image"
	"math"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
)

const (
	combatViewportStartColumn          = 0
	combatViewportStartRow             = 1
	combatMapCenterCoordinate          = 5
	combatBattlefieldWidthSourcePixels = int(references.XCombatMapTiles) * sprites.TileSize
	combatViewportWidthSourcePixels    = xTilesVisibleOnGameScreen * sprites.TileSize
)

// combatLayout places the 11x11 battlefield and its panels. The panel column
// on the left is exactly as wide as the party column on the right, and the
// battlefield is centred on the screen between them. With panels Off the
// battlefield is centred in the map viewport instead.
type combatLayout struct {
	panels int
}

// With panels Off, aiming a spell at a party member still needs its readout:
// the Turn Order column is borrowed for as long as that aiming lasts.
func (g *GameScene) combatLayout() combatLayout {
	panels := g.gameConfig.GetCombatPanels()
	if panels == config.CombatPanelsOff && g.gameState != nil && g.gameState.IsCombatPartySpellTargeting() {
		panels = config.CombatPanelsTurnOrderOnly
	}

	return combatLayout{panels: panels}
}

// battlefieldLeft is the source-pixel distance from the viewport's left edge to
// the first battlefield column.
func (l combatLayout) battlefieldLeft() int {
	if l.panels == config.CombatPanelsOff {
		return (combatViewportWidthSourcePixels - combatBattlefieldWidthSourcePixels) / combatHUDCenterDivisor
	}
	// The viewport is only part of the screen: centre on the whole screen.
	screenWidth := float64(combatViewportWidthSourcePixels) /
		(gameScreenPercents.EndPercentX - gameScreenPercents.StartPercentX)

	return int(math.Round(screenWidth-float64(combatBattlefieldWidthSourcePixels))) / combatHUDCenterDivisor
}

func combatPositionToViewportTile(position references.Position) (int, int) {
	return combatViewportStartColumn + int(position.X),
		combatViewportStartRow + int(position.Y)
}

func (l combatLayout) positionToSourcePixels(position references.Position) (float64, float64) {
	tileX, tileY := combatPositionToViewportTile(position)
	sourceX, sourceY := sprites.ConvertPosition16to18(tileX, tileY)

	return sourceX + float64(l.battlefieldLeft()), sourceY
}

func combatViewportSourceX(screenBounds image.Rectangle, sourcePixel int) int {
	mapFraction := gameScreenPercents.StartPercentX +
		(gameScreenPercents.EndPercentX-gameScreenPercents.StartPercentX)*
			float64(sourcePixel)/float64(combatViewportWidthSourcePixels)

	return screenBounds.Min.X + int(math.Round(float64(screenBounds.Dx())*mapFraction))
}

func combatViewportRowY(screenBounds image.Rectangle, row int) int {
	mapFraction := gameScreenPercents.StartPercentY +
		(gameScreenPercents.EndPercentY-gameScreenPercents.StartPercentY)*
			float64(row)/float64(yTilesVisibleOnGameScreen)

	return screenBounds.Min.Y + int(math.Round(float64(screenBounds.Dy())*mapFraction))
}

func (l combatLayout) battlefieldRectangle(screenBounds image.Rectangle) image.Rectangle {
	return image.Rect(
		combatViewportSourceX(screenBounds, l.battlefieldLeft()),
		combatViewportRowY(screenBounds, combatViewportStartRow),
		combatViewportSourceX(screenBounds, l.battlefieldLeft()+combatBattlefieldWidthSourcePixels),
		combatViewportRowY(screenBounds, combatViewportStartRow+int(references.YCombatMapTiles)),
	)
}

// panelRectangle is the full-height column at the left edge, a mirror image of
// the party column at the right edge. It is empty when panels are Off.
func (l combatLayout) panelRectangle(screenBounds image.Rectangle) image.Rectangle {
	if l.panels == config.CombatPanelsOff {
		return image.Rectangle{}
	}
	partyColumn := calculateRightRailLayout(screenBounds).characterBorder

	return image.Rect(
		screenBounds.Min.X,
		combatViewportRowY(screenBounds, 0),
		screenBounds.Min.X+partyColumn.Dx(),
		combatViewportRowY(screenBounds, yTilesVisibleOnGameScreen),
	)
}

// turnAndReportRectangles splits the panel column between Turn Order and the
// Battle Report. Turn Order takes the whole column when it is the only panel.
func (l combatLayout) turnAndReportRectangles(screenBounds image.Rectangle) (image.Rectangle, image.Rectangle) {
	column := l.panelRectangle(screenBounds)
	if l.panels != config.CombatPanelsOrderAndReport {
		return column, image.Rectangle{}
	}

	return combatHUDStackedRectangles(screenBounds, column)
}
