package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

func TestCombatSignatureTeleportSelectionAndStatusFollowPresentedActor(t *testing.T) {
	scene, screen := newCombatConcealmentTestScene(t)
	state := scene.gameState
	state.CombatScene.MonsterManager.Clear()
	state.CombatScene.TurnQueue.RemoveAllMonsters()
	from, to := references.Position{X: 2, Y: 2}, references.Position{X: 8, Y: 7}
	setCombatConcealmentTerrain(state, from, indexes.BrickFloor)
	index, monster := signatureMonster(t, scene, references.MonsterShadowlord, to)
	monster.SetCurrentHP(monster.MaxHP() - 1) // The battlefield marks only the injured.
	monster.SetPoisoned(true)
	monster.SetAsleep(true)
	state.CombatScene.ActiveMonsterIndex = index
	state.SystemCallbacks.Combat = game_state.NewCombatCallbacks(scene.recordCombatEvent, scene.recordCombatConclusion)
	state.SystemCallbacks.Combat.PublishEvent(game_state.CombatEvent{Kind: game_state.CombatEventTeleport,
		Attacker: game_state.CombatFeedbackEntity{Index: index, Position: to}, From: from, To: to})
	require.Len(t, scene.combatSignatureEffects, 1)
	effect := scene.combatSignatureEffects[0]
	for _, phase := range []struct {
		ageMs                  int64
		fromVisible, toVisible bool
	}{
		{0, true, false}, {90, true, false}, {179, true, false},
		{180, false, false}, {259, false, false},
		{260, false, true}, {360, false, true}, {459, false, true},
	} {
		state.ElapsedMs = effect.StartedMs + phase.ageMs
		scene.updateCombatSignatureEffects()
		screen.Clear()
		scene.drawCombatThreatenedSelectionUnderlays(screen)
		scene.drawCombatMonsterOverlay(screen)
		scene.drawCombatSignatureEffects(screen)
		scene.drawCombatActiveSelection(screen)
		assert.Equal(t, phase.fromVisible, opaquePixelCount(combatSignatureRegion(scene, screen, from)) > 0, "age=%d departure", phase.ageMs)
		assert.Equal(t, phase.toVisible, opaquePixelCount(combatSignatureRegion(scene, screen, to)) > 0, "age=%d arrival", phase.ageMs)
		if phase.fromVisible || phase.toVisible {
			assert.Positive(t, countExactColor(screen, ucolor.CombatActiveMonsterOutline), "selection survives the blink")
			position := from
			if phase.toVisible {
				position = to
			}
			assertActiveCornerPixels(t, screen, scene.combatMonsterDrawOptions(position, index), state.ElapsedMs, ucolor.CombatActiveMonsterOutline, nil)
			assertSignatureHP(t, effect.Frame, monster.CurrentHP(), monster.MaxHP())
		}
		assert.False(t, scene.combatMovement.Active, "teleport must not add a walking/direction animation")
	}
	// Magical invisibility suppresses both the replacement frame and selection.
	monster.SetInvisible(true)
	screen.Clear()
	scene.drawCombatThreatenedSelectionUnderlays(screen)
	scene.drawCombatMonsterOverlay(screen)
	scene.drawCombatSignatureEffects(screen)
	scene.drawCombatActiveSelection(screen)
	assert.Zero(t, opaquePixelCount(screen))
	scene.updateCombatSignatureEffects()
	assert.Nil(t, effect.Frame)
}

func TestCombatSignatureGateAndTeleportComposePoisonSleepAndHP(t *testing.T) {
	for _, kind := range []game_state.CombatEventKind{game_state.CombatEventGating, game_state.CombatEventTeleport} {
		scene, screen := newCombatConcealmentTestScene(t)
		index, monster := signatureMonster(t, scene, references.MonsterDevil, references.Position{X: 5, Y: 5})
		monster.SetCurrentHP(monster.MaxHP() - 1) // The battlefield marks only the injured.
		event := signatureGateEvent(index, monster)
		if kind == game_state.CombatEventTeleport {
			event = game_state.CombatEvent{Kind: kind, From: references.Position{X: 4, Y: 5}, To: monster.Position(),
				Attacker: game_state.CombatFeedbackEntity{Index: index, Position: monster.Position()}}
		}
		scene.recordCombatEvent(event)
		require.Len(t, scene.combatSignatureEffects, 1)
		effect := scene.combatSignatureEffects[0]
		advanceSignatureForTest(scene, effect.StartedMs+effect.DurationMs-1)
		monster.SetPoisoned(true)
		monster.SetAsleep(true)
		scene.drawCombatSignatureEffects(screen)
		both := signaturePixels(effect.Frame)
		assertSignatureHP(t, effect.Frame, monster.CurrentHP(), monster.MaxHP())
		monster.SetPoisoned(false)
		scene.drawCombatSignatureEffects(screen)
		assert.NotEqual(t, both, signaturePixels(effect.Frame), "poison must survive combined sleep composition")
		monster.SetPoisoned(true)
		monster.SetAsleep(false)
		scene.drawCombatSignatureEffects(screen)
		assert.NotEqual(t, both, signaturePixels(effect.Frame), "sleep must survive combined poison composition")
		// Movement after admission invalidates the old-location frame immediately.
		monster.SetPosition(references.Position{X: 7, Y: 7})
		screen.Clear()
		scene.drawCombatSignatureEffects(screen)
		assert.Zero(t, opaquePixelCount(screen))
		assert.False(t, scene.combatSignatureHidesActor(game_state.CombatTurnIdentity{EntityIndex: index}))
		scene.updateCombatSignatureEffects()
		assert.Nil(t, effect.Frame)
		assert.False(t, effect.Dissolve.IsActive())
	}
}

func TestCombatSignatureCorpserComposesExactCloneConditionsWithoutSourceGhost(t *testing.T) {
	for _, condition := range []game_state.CombatConditionResult{game_state.CombatConditionDraggedUnder, game_state.CombatConditionRegurgitated} {
		scene, screen := newCombatConcealmentTestScene(t)
		state := scene.gameState
		combat := state.CombatScene
		source := combat.GetCurrentTurnIdentity()
		character := state.PartyState.GetCharacter(source.EntityIndex)
		character.SetStatus(party_state.Poisoned)
		combat.PartyPossessed[source.EntityIndex] = true
		combat.PartySleeping[source.EntityIndex] = true
		combat.PartyUnderground[source.EntityIndex] = condition == game_state.CombatConditionDraggedUnder
		require.True(t, state.CastCombatInQuasXen(source.EntityIndex, source))
		var clone game_state.CombatTurnIdentity
		var position references.Position
		combat.ForEachPCClone(func(identity game_state.CombatTurnIdentity, pos references.Position) { clone, position = identity, pos })
		combat.PartyPossessed[source.EntityIndex], combat.PartySleeping[source.EntityIndex], combat.PartyUnderground[source.EntityIndex] = false, false, false
		setCombatConcealmentTerrain(state, position, indexes.BrickFloor)
		scene.recordCombatEvent(game_state.CombatEvent{Kind: game_state.CombatEventCondition, Condition: condition,
			Target: game_state.CombatFeedbackEntity{IsPartyMember: true, Index: clone.EntityIndex, Slot: clone.Slot, Position: position}})
		require.Len(t, scene.combatSignatureEffects, 1)
		effect := scene.combatSignatureEffects[0]
		state.ElapsedMs = effect.StartedMs + effect.DurationMs/2
		scene.updateCombatSignatureEffects()
		scene.drawCombatSignatureEffects(screen)
		assert.Positive(t, countExactColor(effect.Frame, ucolor.CombatPossessed), "exact possessed clone retains its cap")
		assertSignatureHP(t, effect.Frame, int(character.GetCurrentHp()), int(character.GetMaxHp()))
		assert.True(t, scene.combatSignatureHidesActor(clone))
		assert.False(t, scene.combatSignatureHidesActor(source))
		withPoison := signaturePixels(effect.Frame)
		character.SetStatus(party_state.Good)
		scene.drawCombatSignatureEffects(screen)
		assert.NotEqual(t, withPoison, signaturePixels(effect.Frame), "poison remains composed with clone-local sleep and possession")
		character.SetHp(0)
		character.MarkAsDead()
		screen.Clear()
		scene.drawCombatSignatureEffects(screen)
		assert.Zero(t, opaquePixelCount(screen), "shared death must not leave the captured clone or source standing")
		scene.updateCombatSignatureEffects()
		assert.Nil(t, effect.Frame)
		assert.False(t, effect.Dissolve.IsActive())
	}
}

func assertSignatureHP(t *testing.T, frame *ebiten.Image, hp, maxHP int) {
	t.Helper()
	expected := ebiten.NewImage(frame.Bounds().Dx(), frame.Bounds().Dy())
	defer expected.Dispose()
	drawCombatHealthMark(expected, &ebiten.DrawImageOptions{}, ucolor.EGABlack, hp, maxHP)
	interior := newCombatHealthMark(&ebiten.DrawImageOptions{}).inner
	actualBar := frame.SubImage(interior).(*ebiten.Image)
	expectedBar := expected.SubImage(interior).(*ebiten.Image)
	assert.Equal(t, signaturePixels(expectedBar), signaturePixels(actualBar), "status tint must not recolor HP")
}

func TestCombatSignatureProductionCompositionKeepsTargetReticleOnTop(t *testing.T) {
	scene := newCombatConcealmentTestState(t)
	state := scene.gameState
	actor := state.CombatScene.GetCurrentTurnIdentity()
	character := state.PartyState.GetCharacter(actor.EntityIndex)
	character.SetEquippedItem(references.SlotWeapon, references.MagicAxe)
	character.SetEquippedItem(references.SlotShield, references.NoEquipment)
	index, monster := signatureMonster(t, scene, references.MonsterDevil, references.Position{X: 5, Y: 5})
	monster.SetPoisoned(true)
	monster.SetAsleep(true)
	require.True(t, state.BeginCombatAttack())
	state.CombatScene.AttackTargeting.Cursor = monster.Position()
	// Aim and active-turn ownership can coincide. The raised active corners and
	// the signature actor still yield to the selected square's white frame.
	state.CombatScene.ActiveMonsterIndex = index
	scene.recordCombatEvent(signatureGateEvent(index, monster))
	advanceSignatureForTest(scene, state.ElapsedMs+combatGateDurationMs/2)
	sample := combatFixtureRasterBounds(scene.combatMonsterDrawOptions(monster.Position(), index))
	screen := ebiten.NewImageWithOptions(sample, &ebiten.NewImageOptions{Unmanaged: true})
	defer screen.Dispose()
	scene.drawAvatarOverlay(screen)
	assertOriginalReticlePixels(t, screen, scene.combatSquareMarkerDrawOptions(monster.Position()), nil)
	scene.clearCombatSignatureEffects()
}
