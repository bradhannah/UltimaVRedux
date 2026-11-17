package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func TestCombatPartySpellHUDMeasuredReadout(t *testing.T) {
	manager := display.GetManager()
	saved := *manager
	t.Cleanup(func() { *manager = saved })
	// This helper composes Output and draws in one call. A clipped target avoids
	// allocating a full HUD frame for assertions solely about measured text.
	target := ebiten.NewImage(1, 1)
	t.Cleanup(target.Deallocate)
	for _, size := range []image.Point{{1280, 720}, {1920, 1080}} {
		state, _ := newProductionCombatCastState(t)
		scene := newProductionCastTestScene(state)
		state.PartyState.Inventory.Spells.Set(references.Mani, 1)
		caster := state.CombatScene.GetCurrentTurnIdentity()
		require.True(t, state.BeginCombatPartySpellTargeting(game_state.CastRequest{Spell: references.Mani, CasterIdx: caster.EntityIndex}).Valid)
		manager.PublishLogicalSize(size.X, size.Y)
		scene.combatHUD = newCombatHUD()
		font := text.NewUltimaFont(float64(combatHUDFontSize) * float64(size.X) / 1920)
		scene.combatHUD.turnOutput = text.NewOutput(font, font.TextFace.Size, combatHUDOutputLines, 80)
		scene.combatHUD.eventOutput = text.NewOutput(font, font.TextFace.Size, combatHUDOutputLines, 80)
		bounds := image.Rectangle{Max: size}
		turn, event := combatLayout{}.turnAndReportRectangles(bounds)
		turn = combatHUDContentBounds(bounds, turn)
		event = combatHUDContentBounds(bounds, event)
		scene.drawCombatPartySpellHUD(target, turn, event)
		assert.LessOrEqual(t, scene.combatHUD.turnOutput.GetPanelContentLineCount(), scene.combatHUD.turnOutput.GetLineCapacity(turn.Dy()))
		assert.LessOrEqual(t, scene.combatHUD.eventOutput.GetPanelContentLineCount(), scene.combatHUD.eventOutput.GetLineCapacity(event.Dy()))
		assert.Contains(t, scene.combatHUD.turnOutput.GetOutputStr(false), "HP:")
		assert.Contains(t, scene.combatHUD.eventOutput.GetOutputStr(false), "Cancel")
	}
}

func TestCombatLootAndSavePromptReviewFrames(t *testing.T) {
	manager := display.GetManager()
	saved := *manager
	t.Cleanup(func() { *manager = saved })
	manager.PublishLogicalSize(1280, 720)
	sheet := sharedPresentationTestSpriteSheet()
	entries := []game_state.CombatLootReveal{
		{Sprite: indexes.ItemMoney, Description: "a sack of gold!"},
		{Sprite: indexes.ItemGem, Description: "a gem!"},
		{Sprite: indexes.ItemPotion, Description: "a potion!"},
		{Sprite: indexes.ItemWeapon, Description: "a weapon!"},
	}
	dialog := widgets.NewCombatLootDialog(entries, input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld), sheet)
	dialog.Reveal(len(entries))
	dialog.FinishReveal()
	screen := newPresentationRaster(1280, 720)
	t.Cleanup(screen.Deallocate)
	t.Cleanup(screen.Deallocate)
	dialog.Draw(screen)
	assert.True(t, dialog.GetBounds().In(image.Rect(0, 0, 960, 720)))
	state, _ := newProductionCombatCastState(t)
	scene := newProductionCastTestScene(state)
	scene.showSaveQuestion()
	assert.Contains(t, scene.saveQuestionDialog.GetContentText(), "Save game?")
	assert.Contains(t, scene.saveQuestionDialog.GetContentText(), "[Y]")
	assert.Contains(t, scene.saveQuestionDialog.GetContentText(), "[N/Esc]")
	scene.clearSaveQuestion()
}

func exportPresentationReview(t *testing.T, screen *ebiten.Image, name string) {
	t.Helper()
	directory := os.Getenv("ULTIMA5_REVIEW_IMAGES")
	if directory == "" {
		return
	}
	file, err := os.Create(filepath.Join(directory, name))
	require.NoError(t, err)
	require.NoError(t, png.Encode(file, screen))
	require.NoError(t, file.Close())
}
