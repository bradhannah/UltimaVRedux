package main

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

func TestCombatHiddenHealthPreservesExactSignatureSpriteAndFleeing(t *testing.T) {
	scene := newCombatConcealmentTestState(t)
	setCombatHPBarsForTest(scene, false)
	index, monster := signatureMonster(t, scene, references.MonsterDevil, references.Position{X: 5, Y: 5})
	monster.SetPoisoned(true)
	monster.SetAsleep(true)
	monster.SetFleeing(true)
	scene.recordCombatEvent(game_state.CombatEvent{Kind: game_state.CombatEventTeleport,
		Attacker: game_state.CombatFeedbackEntity{Index: index, Position: monster.Position()},
		From:     monster.Position(), To: monster.Position()})
	require.Len(t, scene.combatSignatureEffects, 1)
	effect := scene.combatSignatureEffects[0]
	t.Cleanup(scene.clearCombatSignatureEffects)
	frame := scene.composeCombatSignatureFrame(effect)
	expected := ebiten.NewImage(18, 18)
	joined := ebiten.NewImage(18, 18)
	t.Cleanup(expected.Dispose)
	t.Cleanup(joined.Dispose)
	sprite := scene.combatMonsterSprite(monster, index, scene.gameState.ElapsedMs)
	outlined := scene.spriteSheet.GetTileEffectProcessor().ApplyDynamicSimple(sprite, sprites.BlackOutline)
	options := &ebiten.DrawImageOptions{}
	expected.DrawImage(outlined, options)
	scene.drawPoisonOverlay(expected, outlined, options, scene.gameState.ElapsedMs)
	scene.drawSleepOverlay(expected, sprite, options, scene.gameState.ElapsedMs)
	drawCombatFleeChevrons(expected, options)
	require.Equal(t, signaturePixels(expected), signaturePixels(frame), "only bar/attached outline disappear; every sprite/status pixel stays")
	require.Positive(t, opaquePixelCount(combatHealthFrameRegion(expected)), "fixture must exercise sprite feet in the health region")
	scene.prepareCombatSignatureLayers(effect)
	joined.DrawImage(effect.BodyFrame, options)
	joined.DrawImage(effect.HealthFrame, options)
	require.Equal(t, signaturePixels(expected), signaturePixels(joined), "foreground is still required when bars are hidden")
	for y, x := range []int{3, 2, 1, 0, 1, 2, 3} {
		assert.Equal(t, ucolor.EGAYellow.ToRGBA(), frame.At(x, y+5))
		assert.Equal(t, ucolor.EGAYellow.ToRGBA(), frame.At(17-x, y+5))
	}
	for _, showBars := range []bool{true, false} {
		setCombatHPBarsForTest(scene, showBars)
		monster.SetCurrentHP(1)
		monster.SetFleeing(false)
		joined.Clear()
		scene.drawCombatMonsterStatusMarks(joined, options, monster)
		assert.Zero(t, opaquePixelCount(joined), "critical HP cannot fabricate actual fleeing")
		monster.SetFleeing(true)
		joined.Clear()
		scene.drawCombatMonsterStatusMarks(joined, options, monster)
		expected.Clear()
		drawCombatFleeChevrons(expected, options)
		assert.Equal(t, signaturePixels(expected), signaturePixels(joined), "<> is independent in both modes")
	}
}

func TestCombatHiddenHealthSignatureForegroundKeepsFadedFeet(t *testing.T) {
	manager := display.GetManager()
	saved := *manager
	t.Cleanup(func() { *manager = saved })
	manager.PublishLogicalSize(400, 208)
	scene := newCombatConcealmentTestState(t)
	setCombatHPBarsForTest(scene, false)
	state := scene.gameState
	state.CombatScene.MonsterManager.Clear()
	state.CombatScene.TurnQueue.RemoveAllMonsters()
	for index := range state.CombatScene.PartyActive {
		state.CombatScene.PartyActive[index] = false
	}
	from, to := references.Position{X: 2, Y: 2}, references.Position{X: 8, Y: 7}
	setCombatConcealmentTerrain(state, from, indexes.BrickFloor)
	index, monster := signatureMonster(t, scene, references.MonsterDevil, to)
	monster.SetFleeing(true)
	scene.recordCombatEvent(game_state.CombatEvent{Kind: game_state.CombatEventTeleport, From: from, To: to,
		Attacker: game_state.CombatFeedbackEntity{Index: index, Position: to}})
	effect := scene.combatSignatureEffects[0]
	t.Cleanup(scene.clearCombatSignatureEffects)
	state.ElapsedMs = effect.StartedMs + combatTeleportDepartureMs/2
	scene.updateCombatSignatureEffects()
	sample := combatFixtureRasterBounds(scene.combatEntityDrawOptions(from, false, index, 0))
	actual := ebiten.NewImageWithOptions(sample, &ebiten.NewImageOptions{Unmanaged: true})
	expected := ebiten.NewImageWithOptions(sample, &ebiten.NewImageOptions{Unmanaged: true})
	t.Cleanup(actual.Dispose)
	t.Cleanup(expected.Dispose)
	scene.drawCombatSignatureEffects(actual)
	scene.drawCombatHealthFrames(actual)
	options := scene.combatEntityDrawOptions(from, false, index, 0)
	drawCombatCornerCue(expected, options, 2.5, ucolor.CombatMaterialization)
	options.ColorScale.ScaleAlpha(0.5)
	expected.DrawImage(effect.Frame, options)
	left, top := options.GeoM.Apply(1, 12)
	right, bottom := options.GeoM.Apply(16, 18)
	region := image.Rect(int(left), int(top), int(right), int(bottom))
	assert.Equal(t, signaturePixels(expected.SubImage(region).(*ebiten.Image)), signaturePixels(actual.SubImage(region).(*ebiten.Image)),
		"whole-frame reference includes every foreground foot pixel exactly once at the same opacity")
	require.Positive(t, opaquePixelCount(effect.HealthFrame))
}

func TestCombatHiddenHealthGateMaskAndIndependentIndicators(t *testing.T) {
	scene := newCombatConcealmentTestState(t)
	setCombatHPBarsForTest(scene, false)
	index, monster := signatureMonster(t, scene, references.MonsterDevil, references.Position{X: 5, Y: 5})
	monster.SetFleeing(true)
	scene.recordCombatEvent(signatureGateEvent(index, monster))
	effect := scene.combatSignatureEffects[0]
	t.Cleanup(scene.clearCombatSignatureEffects)
	for _, progress := range []float64{0, 0.5, 1} {
		effect.Dissolve.UpdateProgress(progress)
		frame := scene.composeCombatSignatureFrame(effect)
		pixels, mask := signaturePixels(frame), signaturePixels(effect.Dissolve.effect.GetMask())
		for pixel := 0; pixel < len(pixels); pixel += 4 {
			if mask[pixel+3] == 0 {
				assert.Equal(t, []byte{0, 0, 0, 0}, pixels[pixel:pixel+4], "no unmasked HP artwork may restore hidden body pixels")
			}
		}
		if progress == 1 {
			assert.Equal(t, ucolor.EGAYellow.ToRGBA(), frame.At(0, 8))
			assert.Equal(t, ucolor.EGAYellow.ToRGBA(), frame.At(17, 8))
			require.Positive(t, opaquePixelCount(combatHealthFrameRegion(frame)))
		}
	}
	actor, disclosed := scene.gameState.ProjectCombatActor(game_state.CombatTurnIdentity{EntityIndex: index}, monster.CombatantID())
	require.True(t, disclosed)
	assert.Equal(t, "Enemy", combatAllegianceText(actor))
	monster.SetLoyalty(game_state.CombatMonsterLoyaltyParty)
	actor, disclosed = scene.gameState.ProjectCombatActor(actor.Identity, monster.CombatantID())
	require.True(t, disclosed)
	assert.Equal(t, "Ally", combatAllegianceText(actor))
}

func TestCombatHiddenHealthKeepsOnlyInsectMemberAndControlPips(t *testing.T) {
	scene := newCombatConcealmentTestState(t)
	setCombatHPBarsForTest(scene, false)
	state := scene.gameState
	state.CombatScene.MonsterManager.Clear()
	state.CombatScene.TurnQueue.RemoveAllMonsters()
	position := references.Position{X: 5, Y: 5}
	var indices []int
	for range 4 {
		index, monster := signatureMonster(t, scene, references.MonsterInsect, position)
		monster.SetLoyalty(game_state.CombatMonsterLoyaltyParty)
		indices = append(indices, index)
	}
	state.CombatScene.ActiveMonsterIndex = indices[1]
	state.CombatScene.CurrentTurnIdentity = game_state.CombatTurnIdentity{EntityIndex: indices[1]}
	frame, expected := ebiten.NewImage(18, 18), ebiten.NewImage(18, 18)
	t.Cleanup(frame.Dispose)
	t.Cleanup(expected.Dispose)
	options := &ebiten.DrawImageOptions{}
	scene.drawCombatInsectStackIndicators(expected, options, indices)
	for slot, index := range indices {
		frame.Clear()
		actor, disclosed := state.ProjectCombatActor(game_state.CombatTurnIdentity{EntityIndex: index}, 0)
		require.True(t, disclosed)
		scene.drawCombatActorHealthFrame(frame, options, actor)
		if slot == 0 {
			assert.Equal(t, signaturePixels(expected), signaturePixels(frame))
			assert.Equal(t, 4, opaquePixelCount(frame))
			assert.Equal(t, 1, countExactColor(frame, ucolor.EGAWhite), "controlled member keeps its independent white pip")
			assert.Zero(t, opaquePixelCount(combatHealthFrameRegion(frame)))
		} else {
			assert.Zero(t, opaquePixelCount(frame))
		}
	}
	// The primitive is also used for world ships; the combat preference cannot
	// suppress these consumers or introduce a combat allegiance frame.
	frame.Clear()
	drawCombatHealthBar(frame, options, 50, 100)
	assert.Positive(t, opaquePixelCount(frame))
	assert.Zero(t, opaquePixelCount(frame.SubImage(image.Rect(0, 0, 18, 12)).(*ebiten.Image)))
}

func TestCombatHealthPreferenceDefaultsAndOrdinaryFrames(t *testing.T) {
	scene := newCombatConcealmentTestState(t)
	scene.gameConfig = nil
	assert.True(t, scene.showCombatHPBars(), "nil fixture/configuration keeps legacy HP artwork")
	state := scene.gameState
	pc := state.CombatScene.GetCurrentTurnIdentity()
	position := state.CombatScene.GetCombatantPosition(pc)
	setCombatConcealmentTerrain(state, position, indexes.BrickFloor)
	index, monster := signatureMonster(t, scene, references.MonsterOrc, references.Position{X: 5, Y: 3})
	monster.SetCurrentHP(monster.MaxHP() - 1) // The battlefield marks only the injured.
	character := state.PartyState.GetCharacter(pc.EntityIndex)
	character.SetHp(character.GetMaxHp() - 1)
	frame := ebiten.NewImage(18, 18)
	t.Cleanup(frame.Dispose)
	for _, identity := range []game_state.CombatTurnIdentity{pc, {EntityIndex: index}} {
		actor, disclosed := state.ProjectCombatActor(identity, 0)
		require.True(t, disclosed)
		for _, visible := range []bool{true, false, true} {
			setCombatHPBarsForTest(scene, visible)
			frame.Clear()
			scene.drawCombatActorHealthFrame(frame, &ebiten.DrawImageOptions{}, actor)
			assert.Equal(t, visible, opaquePixelCount(frame) > 0)
		}
	}
}

func TestCombatHiddenHealthPreservesActiveSelectionAndTargetForeground(t *testing.T) {
	scene := newCombatConcealmentTestState(t)
	state := scene.gameState
	actor := state.CombatScene.GetCurrentTurnIdentity()
	character := state.PartyState.GetCharacter(actor.EntityIndex)
	character.SetEquippedItem(references.SlotWeapon, references.MagicAxe)
	character.SetEquippedItem(references.SlotShield, references.NoEquipment)
	index, monster := signatureMonster(t, scene, references.MonsterDevil, references.Position{X: 5, Y: 5})
	monster.SetPoisoned(true)
	monster.SetAsleep(true)
	monster.SetFleeing(true)
	require.True(t, state.BeginCombatAttack())
	state.CombatScene.AttackTargeting.Cursor = monster.Position()
	state.CombatScene.ActiveMonsterIndex = index
	scene.recordCombatEvent(signatureGateEvent(index, monster))
	advanceSignatureForTest(scene, state.ElapsedMs+combatGateDurationMs/2)
	t.Cleanup(scene.clearCombatSignatureEffects)
	sample := combatFixtureRasterBounds(scene.combatMonsterDrawOptions(monster.Position(), index))
	actual := ebiten.NewImageWithOptions(sample, &ebiten.NewImageOptions{Unmanaged: true})
	t.Cleanup(actual.Dispose)
	var selection []byte
	for _, visible := range []bool{true, false} {
		setCombatHPBarsForTest(scene, visible)
		actual.Clear()
		scene.drawCombatActiveSelection(actual)
		require.Positive(t, opaquePixelCount(actual))
		if visible {
			selection = signaturePixels(actual)
		} else {
			assert.Equal(t, selection, signaturePixels(actual), "active cue pixels are independent of HP artwork")
		}
		actual.Clear()
		scene.drawAvatarOverlay(actual)
		assertOriginalReticlePixels(t, actual, scene.combatSquareMarkerDrawOptions(monster.Position()), nil)
	}
}
