package main

import (
	"bytes"
	"fmt"
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

func newCombatArchTestScene(t *testing.T) (*GameScene, game_state.CombatTurnIdentity) {
	t.Helper()
	scene, actor := newCombatMarkerTestScene(t)
	state, combat := scene.gameState, scene.gameState.CombatScene
	for index := range state.PartyState.GetCharacterCount() {
		combat.SetPartyMemberPosition(index, references.Position{X: 10, Y: 10})
	}
	combat.SetPartyMemberPosition(actor.EntityIndex, references.Position{X: 5, Y: 5})
	state.PartyState.GetCharacter(actor.EntityIndex).SetStatus(party_state.Good)
	combat.PartySleeping[actor.EntityIndex] = false
	setCombatConcealmentTerrain(state, combat.GetCombatantPosition(actor), indexes.BrickWallArchway)
	state.ElapsedMs = 0

	return scene, actor
}

func TestCombatActiveArchComposesOnlyContourAndSquareOverTerrain(t *testing.T) {
	for _, size := range [][2]int{{400, 208}, {1366, 768}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			scene, actor := newCombatArchTestScene(t)
			setSquareVisualTestDisplay(t, size[0], size[1])
			state, combat := scene.gameState, scene.gameState.CombatScene
			position := combat.GetCombatantPosition(actor)
			// A fractional shake that keeps the body and its contour off a shared
			// texel boundary at the battlefield's column; on one they can differ by a pixel.
			scene.screenShakeOffsetX, scene.screenShakeOffsetY = 0.45, -0.65
			options := scene.combatPartyIdentityDrawOptions(position, actor)
			screen := combatLocalRaster(t, options)
			expected := combatLocalRaster(t, options)
			bodyMask := combatLocalRaster(t, options)
			markerMask := combatLocalRaster(t, options)
			rngBefore := state.GetRNG().Calls()
			for _, elapsed := range []int64{0, 450} {
				state.ElapsedMs = elapsed
				body := scene.spriteSheet.GetAnimatedSprite(state.PartyState.GetCharacter(actor.EntityIndex).GetKeySpriteIndex(), int32(actor.EntityIndex), elapsed)
				bodyMask.Clear()
				bodyMask.DrawImage(body, options)
				markerMask.Clear()
				drawCombatSelectionFrame(markerMask, options, elapsed, ucolor.CombatActivePCOutline)
				drawCombatActiveSelectionCorners(markerMask, options, elapsed, ucolor.CombatActivePCOutline)
				screen.Clear()
				scene.drawCombatPartyOverlay(screen)
				scene.drawCombatHealthFrames(screen)
				combat.ActiveTargetIdentity, combat.ActiveTargetIdentitySet = actor, true
				scene.drawCombatThreatenedSelectionUnderlays(screen)
				require.Zero(t, opaquePixelCount(screen), "body, HP and threatened-target gates remain closed")
				combat.ActiveTargetIdentitySet = false
				for _, destination := range []*ebiten.Image{screen, expected} {
					destination.Clear()
					destination.Fill(ucolor.EGACyan)
					destination.DrawImage(scene.spriteSheet.GetSprite(indexes.BrickWallArchway), scene.combatSquareMarkerDrawOptions(position))
				}
				terrain := signaturePixels(screen)
				contour := scene.spriteSheet.GetTileEffectProcessor().ApplyDynamicOutlineOnly(body, ucolor.CombatActivePCOutline.ToRGBA())
				require.NotNil(t, contour)
				expected.DrawImage(contour, options)
				drawCombatSelectionFrame(expected, options, elapsed, ucolor.CombatActivePCOutline)
				drawCombatActiveSelectionCorners(expected, options, elapsed, ucolor.CombatActivePCOutline)
				scene.drawAvatarOverlay(screen)
				assert.Equal(t, signaturePixels(expected), signaturePixels(screen), "production composition has exactly the hollow contour and existing square")
				assertArchBodyTransparent(t, terrain, signaturePixels(screen), signaturePixels(bodyMask), signaturePixels(markerMask))
				assertActiveFrameEdges(t, screen, options, elapsed, ucolor.CombatActivePCOutline)
				combat.AttackTargeting = &game_state.CombatAttackTargeting{Cursor: position}
				scene.drawAvatarOverlay(screen)
				assertOriginalReticlePixels(t, screen, scene.combatSquareMarkerDrawOptions(position), nil)
				combat.AttackTargeting = nil
			}
			assert.Equal(t, rngBefore, state.GetRNG().Calls(), "draw must not consume gameplay RNG")
			assert.Equal(t, position, combat.GetCombatantPosition(actor))
			assert.False(t, state.CombatTerrainAllowsObjectDisplay(position), "arch exception is presentation-local")
		})
	}
}

func assertArchBodyTransparent(t *testing.T, terrain, actual, body, marker []byte) {
	t.Helper()
	bodyChecked, contourFace, contourBacking := 0, 0, 0
	for pixel := 0; pixel < len(actual); pixel += 4 {
		if marker[pixel+3] != 0 {
			continue
		}
		if body[pixel+3] != 0 {
			bodyChecked++
			if !bytes.Equal(terrain[pixel:pixel+4], actual[pixel:pixel+4]) {
				t.Fatalf("concealed body changed terrain at raster pixel %d: %v -> %v", pixel/4, terrain[pixel:pixel+4], actual[pixel:pixel+4])
			}
		} else if !bytes.Equal(terrain[pixel:pixel+4], actual[pixel:pixel+4]) {
			face := ucolor.CombatActivePCOutline
			if bytes.Equal(actual[pixel:pixel+4], []byte{face.R, face.G, face.B, face.A}) {
				contourFace++
			}
			if bytes.Equal(actual[pixel:pixel+4], []byte{0, 0, 0, 255}) {
				contourBacking++
			}
		}
	}
	require.Positive(t, bodyChecked, "body transparency needs actual opaque source pixels")
	require.Positive(t, contourFace, "the shape contour must add green pixels beyond the square")
	require.Positive(t, contourBacking, "the shape contour must add black backing beyond the square")
}

func assertCombatArchFrame(t *testing.T, scene *GameScene, identity game_state.CombatTurnIdentity, body *ebiten.Image, face ucolor.EGAColor) []byte {
	t.Helper()
	position, visible := scene.combatSelectionPresentation(identity, scene.gameState.CombatScene.GetCombatantPosition(identity))
	require.True(t, visible)
	options := scene.combatPartyIdentityDrawOptions(position, identity)
	screen := ebiten.NewImageWithOptions(combatFixtureRasterBounds(options), &ebiten.NewImageOptions{Unmanaged: true})
	expected := ebiten.NewImageWithOptions(screen.Bounds(), &ebiten.NewImageOptions{Unmanaged: true})
	defer screen.Dispose()
	defer expected.Dispose()
	contour := scene.spriteSheet.GetTileEffectProcessor().ApplyDynamicOutlineOnly(body, face.ToRGBA())
	require.NotNil(t, contour)
	expected.DrawImage(contour, options)
	drawCombatSelectionFrame(expected, options, scene.getElapsedMs(), face)
	drawCombatActiveSelectionCorners(expected, options, scene.getElapsedMs(), face)
	scene.drawCombatActiveSelection(screen)
	require.Positive(t, opaquePixelCount(screen))
	actual := signaturePixels(screen)
	assert.Equal(t, signaturePixels(expected), actual)

	return actual
}

func TestCombatActiveArchFrameVariantsAndSettledTimeStopClock(t *testing.T) {
	for _, variant := range []string{"animated", "sleep", "rat", "shimmer", "possessed-time-stop", "auto-pass"} {
		t.Run(variant, func(t *testing.T) {
			scene, actor := newCombatArchTestScene(t)
			state, combat := scene.gameState, scene.gameState.CombatScene
			character := state.PartyState.GetCharacter(actor.EntityIndex)
			index := character.GetKeySpriteIndex()
			face := ucolor.CombatActivePCOutline
			switch variant {
			case "sleep":
				combat.PartySleeping[actor.EntityIndex] = true
				index = indexes.DeadBody
			case "rat":
				require.True(t, state.TransformActivePartyMemberIntoRat(actor.EntityIndex))
				index = indexes.RatKeyIndex
			case "shimmer":
				combat.PartyInvisible[actor.EntityIndex] = true
				index = indexes.InvisiblePlayer
			case "possessed-time-stop":
				combat.PartyPossessed[actor.EntityIndex] = true
				state.ActivateTimeStopEffect(20)
				face = ucolor.CombatActiveMonsterOutline
			case "auto-pass":
				combat.SetSoloMoveCharacter((actor.EntityIndex + 1) % state.PartyState.GetCharacterCount())
				face = ucolor.EGALightGray
			}
			scene.updateActorAnimationClocks()
			clockDistinguished := false
			for _, elapsed := range []int64{200, 450, 800, 1250} {
				state.ElapsedMs = elapsed
				scene.updateActorAnimationClocks()
				bodyTime := elapsed
				if variant == "possessed-time-stop" {
					bodyTime = elapsed / 20
					assert.Equal(t, bodyTime, scene.combatActorAnimationTime(actor, 0))
				}
				body := scene.spriteSheet.GetAnimatedSprite(index, int32(actor.EntityIndex), bodyTime)
				if variant == "sleep" || variant == "shimmer" {
					body = scene.spriteSheet.GetSprite(index)
				}
				clockDistinguished = clockDistinguished || body != scene.spriteSheet.GetAnimatedSprite(index, int32(actor.EntityIndex), elapsed)
				before := assertCombatArchFrame(t, scene, actor, body, face)
				assert.Equal(t, before, assertCombatArchFrame(t, scene, actor, body, face), "draw cannot advance the settled body clock")
			}
			if variant == "possessed-time-stop" {
				assert.True(t, clockDistinguished, "fixture must distinguish slow-body frames from the ordinary clock")
			}
		})
	}
}

func TestCombatActiveArchCloneHandoffAndExactMotion(t *testing.T) {
	scene, source := newCombatArchTestScene(t)
	state, combat := scene.gameState, scene.gameState.CombatScene
	combat.PartyPossessed[source.EntityIndex] = true
	require.True(t, state.CastCombatInQuasXen(source.EntityIndex, source))
	var clone game_state.CombatTurnIdentity
	combat.ForEachPCClone(func(identity game_state.CombatTurnIdentity, _ references.Position) { clone = identity })
	require.Positive(t, clone.Slot)
	combat.PartyPossessed[source.EntityIndex] = false
	clonePosition := combat.GetCombatantPosition(clone)
	sourcePosition := references.Position{X: (clonePosition.X + 5) % 11, Y: (clonePosition.Y + 5) % 11}
	combat.SetPartyMemberPosition(source.EntityIndex, sourcePosition)
	setCombatConcealmentTerrain(state, sourcePosition, indexes.BrickWallArchway)
	setCombatConcealmentTerrain(state, clonePosition, indexes.BrickWallArchway)
	sourceScreen := combatLocalRaster(t, scene.combatSquareMarkerDrawOptions(sourcePosition))
	cloneScreen := combatLocalRaster(t, scene.combatSquareMarkerDrawOptions(clonePosition))
	for _, current := range []game_state.CombatTurnIdentity{source, clone, source} {
		combat.CurrentTurnIdentity = current
		sourceScreen.Clear()
		cloneScreen.Clear()
		scene.drawCombatActiveSelection(sourceScreen)
		scene.drawCombatActiveSelection(cloneScreen)
		assert.Equal(t, current == source, opaquePixelCount(sourceScreen) > 0)
		assert.Equal(t, current == clone, opaquePixelCount(cloneScreen) > 0)
	}
	// Co-location cannot transfer clone motion or possession to the source.
	combat.SetPartyMemberPosition(source.EntityIndex, clonePosition)
	scene.screenShakeOffsetX, scene.screenShakeOffsetY = 0.35, -0.65
	scene.combatMovement = CombatMovementAnimation{Active: true, IsPartyMember: true,
		EntityIndex: clone.EntityIndex, Slot: clone.Slot,
		From: references.Position{X: clonePosition.X - 1, Y: clonePosition.Y}, To: clonePosition, Progress: 0.5}
	for _, current := range []game_state.CombatTurnIdentity{clone, source} {
		combat.CurrentTurnIdentity = current
		face, offset := ucolor.CombatActivePCOutline, float64(0)
		if current == clone {
			face, offset = ucolor.CombatActiveMonsterOutline, -8
		}
		options, gotFace := scene.combatActiveSelectionOptions()
		require.NotNil(t, options)
		assert.Equal(t, face, gotFace)
		x, y := options.GeoM.Apply(0, 0)
		wantX, wantY := scene.combatSquareMarkerDrawOptions(clonePosition).GeoM.Apply(offset, 0)
		assert.InDelta(t, wantX, x, 1e-6)
		assert.InDelta(t, wantY, y, 1e-6)
		body := scene.spriteSheet.GetAnimatedSprite(state.PartyState.GetCharacter(current.EntityIndex).GetKeySpriteIndex(), int32(current.EntityIndex), state.ElapsedMs)
		assertCombatArchFrame(t, scene, current, body, face)
	}
	scene.combatMovement = CombatMovementAnimation{}
	combat.CurrentTurnIdentity = clone
	feedback := newTestCombatFeedback(game_state.CombatFeedbackDamage)
	feedback.Target = game_state.CombatFeedbackEntity{IsPartyMember: true, Index: clone.EntityIndex, Slot: clone.Slot, Position: clonePosition}
	feedback.Attacker.Position = references.Position{X: clonePosition.X - 1, Y: clonePosition.Y}
	scene.showCombatImpact(feedback)
	scene.combatImpactEffects[0].Elapsed = combatJoltDurationSeconds / 6
	x, _ := scene.combatEntityOffset(true, clone.EntityIndex, clone.Slot)
	require.NotZero(t, x)
	body := scene.spriteSheet.GetAnimatedSprite(state.PartyState.GetCharacter(clone.EntityIndex).GetKeySpriteIndex(), int32(clone.EntityIndex), state.ElapsedMs)
	assertCombatArchFrame(t, scene, clone, body, ucolor.CombatActiveMonsterOutline)
	feedback.Attacker, feedback.Target = feedback.Target, feedback.Attacker
	feedback.Result = game_state.CombatFeedbackMiss
	scene.combatImpactEffects = nil
	scene.showCombatImpact(feedback)
	scene.combatImpactEffects[0].Elapsed = combatLungeDurationSeconds / 2
	x, _ = scene.combatEntityOffset(true, clone.EntityIndex, clone.Slot)
	require.NotZero(t, x)
	assertCombatArchFrame(t, scene, clone, body, ucolor.CombatActiveMonsterOutline)
}

func TestCombatActiveArchCloneConditionsAndHiddenIntervalsAreEntryLocal(t *testing.T) {
	for _, variant := range []string{"sleep", "rat", "shimmer"} {
		t.Run(variant, func(t *testing.T) {
			scene, source := newCombatArchTestScene(t)
			state, combat := scene.gameState, scene.gameState.CombatScene
			index := indexes.SpriteIndex(indexes.DeadBody)
			switch variant {
			case "sleep":
				combat.PartySleeping[source.EntityIndex] = true
			case "rat":
				require.True(t, state.TransformActivePartyMemberIntoRat(source.EntityIndex))
				index = indexes.RatKeyIndex
			case "shimmer":
				combat.PartyInvisible[source.EntityIndex] = true
				index = indexes.InvisiblePlayer
			}
			require.True(t, state.CastCombatInQuasXen(source.EntityIndex, source))
			var clone game_state.CombatTurnIdentity
			combat.ForEachPCClone(func(identity game_state.CombatTurnIdentity, _ references.Position) { clone = identity })
			require.Positive(t, clone.Slot)
			combat.PartySleeping[source.EntityIndex] = variant == "rat"
			combat.PartyInvisible[source.EntityIndex] = false
			combat.CurrentTurnIdentity = clone
			position := combat.GetCombatantPosition(clone)
			setCombatConcealmentTerrain(state, position, indexes.BrickWallArchway)
			state.ElapsedMs = 450
			body := scene.spriteSheet.GetSprite(index)
			if variant == "rat" {
				body = scene.spriteSheet.GetAnimatedSprite(index, int32(clone.EntityIndex), state.ElapsedMs)
			}
			before := assertCombatArchFrame(t, scene, clone, body, ucolor.CombatActivePCOutline)
			combat.PartyUnderground[source.EntityIndex] = true
			scene.combatTeleportEffect = &combatTeleportSpellEffect{Caster: source, Phase: combatTeleportHidden}
			assert.Equal(t, before, assertCombatArchFrame(t, scene, clone, body, ucolor.CombatActivePCOutline), "source hiding cannot hide the clone")
			screen := combatLocalRaster(t, scene.combatSquareMarkerDrawOptions(position))
			scene.combatTeleportEffect.Caster = clone
			scene.drawCombatActiveSelection(screen)
			assert.Zero(t, opaquePixelCount(screen), "exact clone hidden phase suppresses both cues")
			scene.combatTeleportEffect = nil
			state.PartyState.GetCharacter(source.EntityIndex).MarkAsDead()
			screen.Clear()
			scene.drawCombatActiveSelection(screen)
			assert.Zero(t, opaquePixelCount(screen), "shared roster death suppresses the clone too")
		})
	}
}

func TestCombatActiveArchHiddenExceptionsAndRetirement(t *testing.T) {
	for _, hidden := range []string{"black", "underground", "teleport-hidden", "signature-hidden", "signature-underground", "gate-arrival", "offscreen", "dead", "departed", "retired", "exit"} {
		t.Run(hidden, func(t *testing.T) {
			scene, actor := newCombatArchTestScene(t)
			state, combat := scene.gameState, scene.gameState.CombatScene
			position := combat.GetCombatantPosition(actor)
			screen := combatLocalRaster(t, scene.combatSquareMarkerDrawOptions(position))
			scene.drawCombatActiveSelection(screen)
			require.Positive(t, opaquePixelCount(screen), "positive admission control")
			character := state.PartyState.GetCharacter(actor.EntityIndex)
			switch hidden {
			case "black":
				setCombatConcealmentTerrain(state, position, indexes.BlackSquare)
			case "underground":
				combat.PartyUnderground[actor.EntityIndex] = true
			case "teleport-hidden":
				scene.combatTeleportEffect = &combatTeleportSpellEffect{Caster: actor, Phase: combatTeleportHidden}
			case "signature-hidden", "signature-underground", "gate-arrival":
				effect := &combatSignatureEffect{Encounter: combat, Actor: actor, From: position, To: position,
					Kind: game_state.CombatEventTeleport, DurationMs: combatBlinkDurationMs}
				state.ElapsedMs = combatTeleportDepartureMs + 1
				if hidden == "signature-underground" {
					effect.Kind, effect.Condition = game_state.CombatEventCondition, game_state.CombatConditionDraggedUnder
					combat.PartyUnderground[actor.EntityIndex] = true
				} else if hidden == "gate-arrival" {
					effect.Kind, effect.GatePhase = game_state.CombatEventGating, combatGateArrival
				}
				scene.combatSignatureEffects = []*combatSignatureEffect{effect}
				require.True(t, scene.combatSignatureEffectCurrent(effect))
			case "offscreen":
				combat.SetPartyMemberPosition(actor.EntityIndex, references.Position{X: -2, Y: 5})
			case "dead":
				character.SetHp(0)
				character.MarkAsDead()
			case "departed":
				character.PartyMap = party_state.PartyMapAtInn(references.Britain)
			case "retired":
				combat.PartyActive[actor.EntityIndex] = false
			case "exit":
				state.CombatScene = nil
			}
			screen.Clear()
			scene.drawCombatActiveSelection(screen)
			assert.Zero(t, opaquePixelCount(screen), "neither cached contour nor square survives %s", hidden)
		})
	}
}

func TestCombatActiveArchSignatureTeleportTracksFramePositionAndHiddenPhase(t *testing.T) {
	scene, actor := newCombatArchTestScene(t)
	state, combat := scene.gameState, scene.gameState.CombatScene
	from, to := combat.GetCombatantPosition(actor), references.Position{X: 8, Y: 5}
	combat.SetPartyMemberPosition(actor.EntityIndex, to)
	setCombatConcealmentTerrain(state, to, indexes.BrickWallArchway)
	scene.showCombatSignatureEffect(game_state.CombatEvent{Kind: game_state.CombatEventTeleport,
		Attacker: game_state.CombatFeedbackEntity{IsPartyMember: true, Index: actor.EntityIndex, Position: from}, From: from, To: to})
	t.Cleanup(scene.clearCombatSignatureEffects)
	require.Len(t, scene.combatSignatureEffects, 1)
	body := scene.spriteSheet.GetSprite(state.PartyState.GetCharacter(actor.EntityIndex).GetKeySpriteIndex())
	contour := scene.spriteSheet.GetTileEffectProcessor().ApplyDynamicOutlineOnly(body, ucolor.CombatActivePCOutline.ToRGBA())
	require.NotNil(t, contour)
	for _, sample := range []struct {
		elapsed  int64
		position references.Position
		opacity  float32
		visible  bool
	}{
		{90, from, 0.5, true},
		{180, to, 0, false},
		{260, to, 0, true},
		{360, to, 0.5, true},
	} {
		state.ElapsedMs = sample.elapsed
		for _, position := range []references.Position{from, to} {
			options := scene.combatPartyIdentityDrawOptions(position, actor)
			screen := combatLocalRaster(t, options)
			expected := combatLocalRaster(t, options)
			if sample.visible && position == sample.position {
				contourOptions := *options
				contourOptions.ColorScale.ScaleAlpha(sample.opacity)
				expected.DrawImage(contour, &contourOptions)
				drawCombatSelectionFrame(expected, options, sample.elapsed, ucolor.CombatActivePCOutline)
				drawCombatActiveSelectionCorners(expected, options, sample.elapsed, ucolor.CombatActivePCOutline)
			}
			scene.drawCombatActiveSelection(screen)
			assert.Equal(t, signaturePixels(expected), signaturePixels(screen), "signature phase %d at %v", sample.elapsed, position)
		}
	}
	setCombatConcealmentTerrain(state, to, indexes.BlackSquare)
	screen := combatLocalRaster(t, scene.combatSquareMarkerDrawOptions(to))
	scene.drawCombatActiveSelection(screen)
	assert.Zero(t, opaquePixelCount(screen), "arrival cannot disclose black terrain")
}

func TestCombatActiveArchSignatureMaskDoesNotRevealOrMutateCachedContour(t *testing.T) {
	scene, actor := newCombatArchTestScene(t)
	state, combat := scene.gameState, scene.gameState.CombatScene
	position := combat.GetCombatantPosition(actor)
	scene.showCombatSignatureEffect(game_state.CombatEvent{Kind: game_state.CombatEventCondition,
		Condition: game_state.CombatConditionRegurgitated,
		Target:    game_state.CombatFeedbackEntity{IsPartyMember: true, Index: actor.EntityIndex, Position: position}})
	t.Cleanup(scene.clearCombatSignatureEffects)
	require.Len(t, scene.combatSignatureEffects, 1)
	effect := scene.combatSignatureEffects[0]
	body := scene.spriteSheet.GetSprite(state.PartyState.GetCharacter(actor.EntityIndex).GetKeySpriteIndex())
	contour := scene.spriteSheet.GetTileEffectProcessor().ApplyDynamicOutlineOnly(body, ucolor.CombatActivePCOutline.ToRGBA())
	require.NotNil(t, contour)
	before := signaturePixels(contour)
	options := scene.combatPartyIdentityDrawOptions(position, actor)
	screen, expected := combatLocalRaster(t, options), combatLocalRaster(t, options)
	masked := ebiten.NewImage(contour.Bounds().Dx(), contour.Bounds().Dy())
	t.Cleanup(masked.Dispose)
	for _, elapsed := range []int64{0, combatCorpserDurationMs / 2, combatCorpserDurationMs - 1} {
		state.ElapsedMs = elapsed
		scene.updateCombatSignatureEffects()
		masked.Clear()
		masked.DrawImage(contour, nil)
		masked.DrawImage(effect.Dissolve.effect.GetMask(), &ebiten.DrawImageOptions{Blend: ebiten.BlendDestinationIn})
		expected.Clear()
		expected.DrawImage(masked, options)
		drawCombatSelectionFrame(expected, options, elapsed, ucolor.CombatActivePCOutline)
		drawCombatActiveSelectionCorners(expected, options, elapsed, ucolor.CombatActivePCOutline)
		screen.Clear()
		scene.drawCombatActiveSelection(screen)
		assert.Equal(t, signaturePixels(expected), signaturePixels(screen), "contour obeys signature mask at %d", elapsed)
		assert.Equal(t, before, signaturePixels(contour), "mutable mask results never enter the shared contour cache")
	}
}

func TestCombatOrdinaryFloorRetainsBodyHPAndSingleSelection(t *testing.T) {
	scene, actor := newCombatArchTestScene(t)
	state, combat := scene.gameState, scene.gameState.CombatScene
	position := combat.GetCombatantPosition(actor)
	setCombatConcealmentTerrain(state, position, indexes.BrickFloor)
	options := scene.combatPartyIdentityDrawOptions(position, actor)
	screen, expected := combatLocalRaster(t, options), combatLocalRaster(t, options)
	for _, elapsed := range []int64{0, 450} {
		state.ElapsedMs = elapsed
		screen.Clear()
		expected.Clear()
		scene.drawCombatPartyOverlay(expected)
		scene.drawCombatHealthFrames(expected)
		drawCombatSelectionFrame(expected, options, elapsed, ucolor.CombatActivePCOutline)
		drawCombatActiveSelectionCorners(expected, options, elapsed, ucolor.CombatActivePCOutline)
		scene.drawAvatarOverlay(screen)
		assert.Equal(t, signaturePixels(expected), signaturePixels(screen), "ordinary floor has no extra coloured contour or duplicate selection")
		screen.Clear()
		scene.drawCombatHealthFrames(screen)
		require.Positive(t, opaquePixelCount(screen))
		before := signaturePixels(screen)
		scene.drawCombatActiveSelection(screen)
		after := signaturePixels(screen)
		// Existing selection may cover exterior HP framing; the interior fill is unchanged.
		for y := 14; y < 16; y++ {
			for x := 3; x < 14; x++ {
				px, py := options.GeoM.Apply(float64(x)+0.5, float64(y)+0.5)
				pixel := ((int(py)-screen.Bounds().Min.Y)*screen.Bounds().Dx() + int(px) - screen.Bounds().Min.X) * 4
				assert.Equal(t, before[pixel:pixel+4], after[pixel:pixel+4])
			}
		}
	}
}
