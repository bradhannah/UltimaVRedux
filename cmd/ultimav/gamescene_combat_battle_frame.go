package main

import (
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

// The battlefield sits in the classic rounded frame the panels use: two lines
// of colour around one of white, a few units clear of the outermost squares.
const (
	combatBattleFrameWhiteUnits = 3
	combatBattleFrameInnerUnits = 4
	combatBattleFrameOuterUnits = 5
	// An expanded sprite frame starts one source pixel outside its square.
	combatBattleFrameSpriteBorder = (sprites.ExpandedTileSize - sprites.OriginalTileSize) / 2
)

func (g *GameScene) showCombatBattleFrame() bool {
	return g.gameConfig == nil || g.gameConfig.GetShowCombatBattleFrame()
}

// combatBattleFrameColor is the enemy red while the battle is undecided and the
// panel blue once it is won.
func combatBattleFrameColor(won bool) ucolor.EGAColor {
	if won {
		return ucolor.EGABlue
	}

	return ucolor.CombatAllegianceEnemy
}

// combatBattlefieldScreenRect is the eleven by eleven squares in whole screen
// pixels, following the battlefield when it shakes or slides.
func (g *GameScene) combatBattlefieldScreenRect() image.Rectangle {
	transform := ebiten.GeoM{}
	transform.Translate(combatBattleFrameSpriteBorder, combatBattleFrameSpriteBorder)
	transform.Concat(g.combatSquareMarkerDrawOptions(references.Position{}).GeoM)
	left, top := transform.Apply(0, 0)
	side := float64(combatBattlefieldWidthSourcePixels)
	right, bottom := transform.Apply(side, side)

	return image.Rect(int(math.Round(left)), int(math.Round(top)), int(math.Round(right)), int(math.Round(bottom)))
}

func (g *GameScene) drawCombatBattleFrame(target *ebiten.Image) {
	if !g.gameState.IsInCombat() || g.gameState.CombatScene == nil || !g.showCombatBattleFrame() {
		return
	}
	bounds := target.Bounds()
	unit := widgets.ClassicUIPixelUnit(bounds.Dx(), bounds.Dy())
	board := g.combatBattlefieldScreenRect()
	side := combatBattleFrameColor(g.gameState.CombatScene.Victory)
	widgets.DrawClassicPixelRoundedFrame(target, board.Inset(-combatBattleFrameOuterUnits*unit), unit, side)
	widgets.DrawClassicPixelRoundedFrame(target, board.Inset(-combatBattleFrameInnerUnits*unit), unit, side)
	widgets.DrawClassicPixelRoundedFrame(target, board.Inset(-combatBattleFrameWhiteUnits*unit), unit, ucolor.EGAWhite)
}
