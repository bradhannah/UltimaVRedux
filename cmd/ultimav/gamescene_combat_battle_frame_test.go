package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

func TestCombatBattleFrameIsRedInBattleBlueOnceWonAndOptional(t *testing.T) {
	scene, screen := newCombatConcealmentTestScene(t)
	scene.gameConfig = scene.gameState.GameConfig
	board := scene.combatBattlefieldScreenRect()
	scene.drawCombatBattleFrame(screen)
	assert.Positive(t, countExactColor(screen, ucolor.CombatAllegianceEnemy), "undecided battles are framed in the enemy red")
	assert.Positive(t, countExactColor(screen, ucolor.EGAWhite))
	assert.Zero(t, countExactColor(screen, ucolor.EGABlue))
	squares, ok := screen.SubImage(board).(*ebiten.Image)
	require.True(t, ok)
	assert.Zero(t, opaquePixelCount(squares), "the frame never covers a square")

	scene.gameState.CombatScene.Victory = true
	screen.Clear()
	scene.drawCombatBattleFrame(screen)
	assert.Positive(t, countExactColor(screen, ucolor.EGABlue), "a won battle is framed in the panel blue")
	assert.Zero(t, countExactColor(screen, ucolor.CombatAllegianceEnemy))

	scene.gameConfig.GameplayConfiguration.ShowCombatBattleFrame = false
	screen.Clear()
	scene.drawCombatBattleFrame(screen)
	assert.Zero(t, opaquePixelCount(screen), "Battle Frame Off draws nothing")
}
