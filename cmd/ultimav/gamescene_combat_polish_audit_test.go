package main

import (
	"fmt"
	"image"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/visual"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	beradrng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

func TestCombatAllegianceExactCloneConcealmentAndReplacement(t *testing.T) {
	scene, mock := newCombatUseTestScene(t)
	state, combat := scene.gameState, scene.gameState.CombatScene
	source := combat.GetCurrentTurnIdentity()
	combat.PartyPossessed[source.EntityIndex] = true
	require.True(t, state.CastCombatInQuasXen(source.EntityIndex, source))
	var clone game_state.CombatTurnIdentity
	var position references.Position
	combat.ForEachPCClone(func(identity game_state.CombatTurnIdentity, pos references.Position) { clone, position = identity, pos })
	combat.PartyPossessed[source.EntityIndex] = false
	setCombatConcealmentTerrain(state, position, indexes.BrickFloor)
	setCombatConcealmentTerrain(state, combat.GetCombatantPosition(source), indexes.BrickFloor)
	scene.updateCombatAllegiance()
	mock.Messages = nil
	assert.Equal(t, ucolor.CombatHealthFriend, scene.combatDisclosedHealthBorder(source))
	assert.Equal(t, ucolor.CombatAllegianceEnemy, scene.combatDisclosedHealthBorder(clone))
	setCombatConcealmentTerrain(state, position, indexes.BlackSquare)
	scene.updateCombatAllegiance()
	assert.Equal(t, ucolor.EGABlack, scene.combatDisclosedHealthBorder(clone), "a hidden actor discloses no side")
	assert.NotContains(t, scene.combatAllegiance.actors, combatAllegianceKey{identity: clone})
	assert.Contains(t, scene.combatAllegiance.actors, combatAllegianceKey{identity: source})
	setCombatConcealmentTerrain(state, position, indexes.BrickFloor)
	scene.updateCombatAllegiance()
	assert.Empty(t, mock.Messages)
	replacement := game_state.NewCombatMonster(references.MonsterRat, &(*state.GameReferences.EnemyReferences)[references.MonsterRat], position, state.AsRandomProvider())
	require.GreaterOrEqual(t, combat.ReplaceCombatantWithMonster(clone, replacement), 0)
	require.True(t, state.CastCombatInQuasXen(source.EntityIndex, source))
	var nextClone game_state.CombatTurnIdentity
	combat.ForEachPCClone(func(identity game_state.CombatTurnIdentity, pos references.Position) {
		nextClone = identity
		setCombatConcealmentTerrain(state, pos, indexes.BrickFloor)
	})
	require.Greater(t, nextClone.Slot, clone.Slot, "encounter clone slots are never recycled")
	scene.updateCombatAllegiance()
	assert.NotContains(t, scene.combatAllegiance.actors, combatAllegianceKey{identity: clone})
	assert.Equal(t, game_state.CombatMonsterLoyaltyParty, scene.combatAllegiance.actors[combatAllegianceKey{identity: nextClone}].loyalty)
	assert.Empty(t, mock.Messages, "a replacement is not an allegiance transition of the retired clone")
	assert.Equal(t, ucolor.EGABlack, scene.combatDisclosedHealthBorder(clone),
		"stale identity cannot borrow its source's or successor's health border")
}

func TestCombatGateDrawAfterHitchUsesUpdateOwnedPhases(t *testing.T) {
	scene, screen := newCombatConcealmentTestScene(t)
	state := scene.gameState
	state.CombatScene.MonsterManager.Clear()
	state.CombatScene.TurnQueue.RemoveAllMonsters()
	index, monster := signatureMonster(t, scene, references.MonsterDevil, references.Position{X: 5, Y: 5})
	scene.recordCombatEvent(signatureGateEvent(index, monster))
	effect := scene.combatSignatureEffects[0]
	rng := state.GetRNG().Calls()
	state.ElapsedMs += 5000 // The first draw arrives long after event admission.
	assert.True(t, scene.hasCombatSignatureEffects())
	scene.drawCombatMonsterOverlay(screen)
	assert.Zero(t, opaquePixelCount(screen))
	scene.drawCombatSignatureEffects(screen)
	assert.Zero(t, opaquePixelCount(effect.Frame), "prelude still has no creature frame")
	assert.Equal(t, combatGateArrival, effect.GatePhase, "drawing cannot advance phases")
	scene.updateCombatSignatureEffects()
	assert.Equal(t, combatGateMaterialize, effect.GatePhase)
	assert.Zero(t, effect.Dissolve.GetLevel(), "late admission gives materialization its full lifetime")
	state.ElapsedMs += 60
	scene.updateCombatSignatureEffects()
	scene.drawCombatSignatureEffects(screen)
	assert.Positive(t, opaquePixelCount(effect.Highlight))
	mask := dissolveMaskPixels(effect.Dissolve.effect.GetMask())
	state.ElapsedMs += 10000 // Also hitch mid-reveal before the next effect update.
	for range 3 {
		assert.True(t, scene.hasCombatSignatureEffects())
		screen.Clear()
		scene.drawCombatMonsterOverlay(screen)
		assert.Zero(t, opaquePixelCount(screen))
		scene.drawCombatSignatureEffects(screen)
		assert.Equal(t, mask, dissolveMaskPixels(effect.Dissolve.effect.GetMask()))
		assert.Equal(t, combatGateMaterialize, effect.GatePhase)
	}
	scene.updateCombatSignatureEffects()
	assert.Equal(t, combatGateSettle, effect.GatePhase)
	assert.Equal(t, float64(1), effect.Dissolve.GetLevel())
	state.ElapsedMs += combatGateSettleMs - 1
	scene.updateCombatSignatureEffects()
	assert.True(t, scene.hasCombatSignatureEffects())
	scene.drawCombatSignatureEffects(screen)
	assert.Zero(t, opaquePixelCount(effect.Highlight))
	state.ElapsedMs++
	assert.True(t, scene.hasCombatSignatureEffects(), "workflow poll before effects cannot retire the replacement")
	scene.updateCombatSignatureEffects()
	assert.False(t, scene.hasCombatSignatureEffects())
	assert.Nil(t, effect.Frame)
	assert.Nil(t, effect.Highlight)
	assert.Nil(t, effect.Dissolve.effect)
	assert.Nil(t, effect.Dissolve.buffer)
	assert.Equal(t, rng, state.GetRNG().Calls(), "draw/effect updates never draw from gameplay RNG")
}

func TestDissolveSharedCleanupPreservesRevealHideAndSurvivingTile(t *testing.T) {
	scene := &GameScene{spriteSheet: sharedPresentationTestSpriteSheet()}
	sprite := ebiten.NewImage(8, 8)
	sprite.Fill(ucolor.EGAWhite)
	t.Cleanup(sprite.Dispose)
	frame := ebiten.NewImage(8, 8)
	t.Cleanup(frame.Dispose)
	for _, mode := range []visual.DissolveMode{visual.DissolveModeReveal, visual.DissolveModeHide} {
		var dissolve DissolveEffectState
		for range 2 { // The same consumer may initialize again after cleanup.
			dissolve.Initialize(8, 8, mode, beradrng.New(42))
			for _, progress := range []float64{0, 0.25, 1} {
				dissolve.UpdateProgress(progress)
				frame.Clear()
				scene.drawSpriteWithDissolveEffect(sprite, &dissolve, &ebiten.DrawImageOptions{}, frame)
				want := int(64 * progress)
				if mode == visual.DissolveModeHide {
					want = 64 - want
				}
				assert.Equal(t, want, opaquePixelCount(frame))
			}
			composed := signaturePixels(frame)
			dissolve.Cleanup()
			dissolve.Cleanup()
			assert.Nil(t, dissolve.effect)
			assert.Nil(t, dissolve.buffer)
			assert.Zero(t, dissolve.GetLevel())
			assert.Equal(t, composed, signaturePixels(frame), "releasing source resources does not erase already-composed output")
			frame.Clear()
			scene.drawSpriteWithDissolveEffect(sprite, &dissolve, &ebiten.DrawImageOptions{}, frame)
			assert.Equal(t, 64, opaquePixelCount(frame), "inactive consumers still use ordinary rendering")
		}
	}
	options := TileDissolveOptions{Position: references.Position{X: 5, Y: 5}, SpriteIndex: indexes.Apparation1,
		Mode: visual.DissolveModeReveal, OutlineColor: sprites.WhiteOutline, PositionMode: PositionModeFixedGrid, RenderLayer: RenderLayerOverFade}
	first, second := scene.StartTileDissolveWithOptions(options), scene.StartTileDissolveWithOptions(options)
	scene.UpdateTileDissolveProgress(second, 0.5)
	mask := dissolveMaskPixels(scene.activeTileDissolves[1].Effect.effect.GetMask())
	scene.StopTileDissolve(first)
	require.Len(t, scene.activeTileDissolves, 1)
	assert.Equal(t, second, scene.activeTileDissolves[0].ID)
	assert.Equal(t, mask, dissolveMaskPixels(scene.activeTileDissolves[0].Effect.effect.GetMask()))
	scene.StopAllTileDissolves()
	scene.StopAllTileDissolves()
	assert.Empty(t, scene.activeTileDissolves)
}

func TestCombatFullHUDCombinedComposition(t *testing.T) {
	manager := display.GetManager()
	logicalWidth, logicalHeight := manager.LogicalSize()
	t.Cleanup(func() {
		width, height := manager.LogicalSize()
		assert.Equal(t, image.Pt(logicalWidth, logicalHeight), image.Pt(width, height),
			"the fixture must restore published/unpublished logical-size state, not only resolved dimensions")
	})
	// Preserve the unpublished logical-size state too. Republishing the resolved
	// fallback freezes later window-driven tests at this test's initial size.
	savedManager := *manager
	t.Cleanup(func() { *manager = savedManager })
	for _, size := range []image.Point{{1280, 720}, {1366, 768}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			manager.PublishLogicalSize(size.X, size.Y)
			scene, actor := setupCombatAutoAttackInputTest(t)
			state := scene.gameState
			scene.gameConfig = state.GameConfig
			scene.spriteSheet = sharedPresentationTestSpriteSheet()
			scene.mapImage = ebiten.NewImage(304, 208)
			t.Cleanup(scene.mapImage.Dispose)
			scene.output = nil // Replace the input-only fixture's oversized buffer with the production transcript.
			scene.initializeResizeableVisualElements()
			state.SystemCallbacks.Message.ShowCommandPrompt = scene.showCommandPrompt
			scene.activeEffectsPanel = NewActiveEffectsPanel(state, scene.spriteSheet, scene.rightRail.activeBorder)
			scene.syncRightRailLayout(image.Rectangle{Max: size})
			t.Cleanup(scene.outputClipImage.Dispose)
			state.CombatScene.MonsterManager.Clear()
			state.CombatScene.TurnQueue.RemoveAllMonsters()
			state.CombatScene.SetPartyMemberPosition(actor.EntityIndex, references.Position{X: 1, Y: 1})
			character := state.PartyState.GetCharacter(actor.EntityIndex)
			character.SetHp(character.GetMaxHp() - 1) // The battlefield marks only the injured.
			clear(character.Name[:])
			copy(character.Name[:], "Shamino")
			character.Intelligence = 255
			character.SetEquippedItem(references.SlotHelm, references.SpikedHelm)
			character.SetEquippedItem(references.SlotWeapon, references.MorningStar)
			character.SetEquippedItem(references.SlotShield, references.NoEquipment)
			for i := 8; i >= 0; i-- { // Deliberately reverse spatial draw order for neighbor overlap.
				position := references.Position{X: references.Coordinate(3 + i%3), Y: references.Coordinate(3 + i/3)}
				index, monster := signatureMonster(t, scene, references.MonsterBard, position)
				monster.SetPoisoned(true)
				if i%2 == 1 {
					state.SetRandomSeed(42)
					require.True(t, state.CastCombatAnXenEx(actor.EntityIndex, game_state.CombatTurnIdentity{EntityIndex: index}))
				}
			}
			_, target := signatureMonster(t, scene, references.MonsterLordBritish, references.Position{X: 8, Y: 2})
			require.True(t, state.CombatScene.AddField(game_state.NewCombatField(game_state.CombatFieldPoison, references.Position{X: 4, Y: 4})))
			scene.updateCombatAllegiance()
			scene.combatMapInputHandler(ebiten.KeyA)
			firstPromptRows := scene.output.GetPanelContentLineCount()
			scene.finishCombatAttackTargetAction(state.CancelCombatAttackSource())
			state.CombatScene.AttackTargeting.Cursor = target.Position()
			scene.showCombatAttackTargetContext()
			assert.LessOrEqual(t, scene.output.GetPanelContentLineCount()-firstPromptRows,
				scene.output.GetLineCapacity(scene.rightRail.outputContent.Dy()), "the complete current-source prompt and target must remain visible together")
			scene.combatHUD.detail = true
			screen := newPresentationRaster(size.X, size.Y)
			t.Cleanup(screen.Dispose)
			for y := range references.YCombatMapTiles {
				for x := range references.XCombatMapTiles {
					screen.DrawImage(scene.spriteSheet.GetSprite(indexes.Grass), scene.combatEntityDrawOptions(references.Position{X: references.Coordinate(x), Y: references.Coordinate(y)}, false, -1, 0))
				}
			}
			scene.drawAvatarOverlay(screen)
			scene.drawCombatHUD(screen)
			scene.drawBorders(screen)
			scene.activeEffectsPanel.Draw(screen)
			scene.drawMainOutput(screen)
			scene.characterSummary.Draw(&state.PartyState, screen, state.GetActivePlayerIndex(), false, -1)
			scene.provisionSummary.Draw(state, screen)
			// HP survives poisoned neighbors and the full HUD except for the exact
			// approved foreground indicator pixels. Include the aimed actor too:
			// selected aim owns overlapping pixels, not an exemption for its whole cell.
			var sample image.Rectangle
			var healthOptions []*ebiten.DrawImageOptions
			state.ForEachDisclosedCombatActor(func(view game_state.CombatActorPresentation) {
				options := scene.combatEntityDrawOptions(view.Position, view.Identity.IsPartyMember, view.Identity.EntityIndex, view.Identity.Slot)
				healthOptions = append(healthOptions, options)
				sample = sample.Union(combatFixtureRasterBounds(options))
			})
			sample = sample.Intersect(screen.Bounds())
			reference := ebiten.NewImageWithOptions(sample, &ebiten.NewImageOptions{Unmanaged: true})
			foreground := ebiten.NewImageWithOptions(sample, &ebiten.NewImageOptions{Unmanaged: true})
			t.Cleanup(reference.Dispose)
			t.Cleanup(foreground.Dispose)
			state.ForEachDisclosedCombatActor(func(view game_state.CombatActorPresentation) {
				scene.drawCombatActorHealthFrame(reference, scene.combatEntityDrawOptions(view.Position, view.Identity.IsPartyMember, view.Identity.EntityIndex, view.Identity.Slot), view)
			})
			require.Equal(t, actor, state.CombatScene.GetCurrentTurnIdentity())
			activeOptions := scene.combatPartyIdentityDrawOptions(state.CombatScene.GetCombatantPosition(actor), actor)
			drawActiveSelectionReference(foreground, activeOptions, state.ElapsedMs, ucolor.CombatActivePCOutline)
			drawOriginalReticleReference(foreground, scene.combatSquareMarkerDrawOptions(target.Position()))
			assertCombatHealthSelectionComposition(t, screen.SubImage(sample).(*ebiten.Image), reference, foreground, healthOptions)
			turn, _ := combatLayout{}.turnAndReportRectangles(screen.Bounds())
			bounds := combatHUDContentBounds(screen.Bounds(), turn)
			output := scene.combatHUD.turnOutput
			readout := appendCombatAttackReadoutRows(nil, scene.combatAttackPreview())
			readoutRows := len(readout)
			require.Equal(t, 3, readoutRows, "weapon, target and verdict take one row each")
			assert.Equal(t, combatReadoutRow{text: "Morning Star", value: "R2"}, readout[0])
			assert.Equal(t, combatReadoutRow{text: "Lord British", value: "D7"}, readout[1])
			line := int(math.Ceil(output.GetLineSpacing()))
			bottom := scene.drawCombatCurrentTurn(screen, bounds, line, line/3)
			_, spriteHeight := scene.combatHUDSpriteSize(screen, scene.combatTurnSprite(actor))
			assert.LessOrEqual(t, bottom+line/3+int(math.Ceil(output.GetTotalHeightForLines(readoutRows)))+2*line+spriteHeight+line, bounds.Max.Y)
		})
	}
}
