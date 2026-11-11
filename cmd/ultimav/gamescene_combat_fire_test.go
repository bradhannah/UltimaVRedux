package main

import (
	"fmt"
	"image/color"
	"math"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	beradrng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

func newCombatFireMovementTest(t *testing.T, tile indexes.SpriteIndex) (
	*GameScene, game_state.CombatTurnIdentity, *[]game_state.CombatEvent,
) {
	t.Helper()
	scene, actor := setupCombatAutoAttackInputTest(t)
	state := scene.gameState
	character := state.PartyState.GetCharacter(actor.EntityIndex)
	character.SetMaxHp(100)
	character.SetHp(100)
	character.SetEquippedItem(references.SlotRing, references.NoEquipment)
	destination := references.Position{X: 6, Y: 5}
	state.CombatScene.CombatMapRef.SetTileNumber(destination, tile)
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &destination, tile)
	events := make([]game_state.CombatEvent, 0)
	state.SystemCallbacks.Combat.PublishEvent = func(event game_state.CombatEvent) {
		events = append(events, event)
		scene.recordCombatEvent(event)
	}
	state.ElapsedMs = 10000
	setCombatFireTestRoll(t, state, 7)

	return scene, actor, &events
}

func setCombatFireTestRoll(t *testing.T, state *game_state.GameState, want int) {
	t.Helper()
	for seed := uint64(1); seed <= 256; seed++ {
		state.SetRandomSeed(seed)
		if state.RandomIntInRange(0, 10) == want {
			state.SetRandomSeed(seed)

			return
		}
	}
	t.Fatal("bounded deterministic fire-roll fixture found no seed")
}

func combatFireEvents(events []game_state.CombatEvent) []game_state.CombatEvent {
	var fires []game_state.CombatEvent
	for _, event := range events {
		if event.FireInjury != nil {
			fires = append(fires, event)
		}
	}

	return fires
}

func TestCombatFireKeyboardMovementSurvivesContactAndArrival(t *testing.T) {
	for _, speed := range combatTimingTestRates() {
		t.Run(fmt.Sprint(speed), func(t *testing.T) {
			scene, actor, events := newCombatFireMovementTest(t, indexes.Fireplace)
			state := scene.gameState
			setCombatTimingTestRate(scene, speed)
			hpBefore := state.PartyState.GetCharacter(actor.EntityIndex).GetCurrentHp()
			callsBefore := state.GetRNG().Calls()
			scene.combatMapInputHandler(ebiten.KeyRight)
			fires := combatFireEvents(*events)
			require.Len(t, fires, 1)
			require.Len(t, scene.combatImpactEffects, 1)
			assert.Equal(t, 7, fires[0].HPLost)
			assert.Equal(t, hpBefore-7, state.PartyState.GetCharacter(actor.EntityIndex).GetCurrentHp())
			assert.Equal(t, callsBefore+1, state.GetRNG().Calls(), "presentation must not add a damage roll")
			assert.Equal(t, actor.EntityIndex, fires[0].Target.Index)
			assert.Equal(t, actor.Slot, fires[0].Target.Slot)
			assert.Equal(t, references.Position{X: 6, Y: 5}, fires[0].Target.Position)
			assert.Contains(t, scene.combatEventPlainText(fires[0]), "burned for 7 damage")
			assert.True(t, scene.combatMovement.Active)
			assert.True(t, scene.combatImpactEffects[0].fireMovement)
			assert.Zero(t, scene.combatFireImpactAlpha(true, actor.EntityIndex, actor.Slot), "no burn before contact")
			drainCombatPacingAt(scene, state.ElapsedMs)
			duration := int64(scene.captureCombatTiming().duration(combatPlayerMovementDurationMs))
			state.ElapsedMs += (duration + 1) / 2
			state.EventQueueManager.Update()
			scene.updateCombatImpactEffects(30) // A prior frame hitch is not the new injury's age.
			require.Len(t, scene.combatImpactEffects, 1)
			assert.Equal(t, combatFireTintMaximumAlpha, scene.combatFireImpactAlpha(true, actor.EntityIndex, actor.Slot))
			x, y := scene.combatImpactLabelPosition(scene.combatImpactEffects[0])
			destinationX, destinationY := combatPositionToSourcePixels(fires[0].Target.Position)
			offsetX, offsetY := scene.combatMovementOffset(true, actor.EntityIndex, actor.Slot)
			assert.Equal(t, destinationX+offsetX, x)
			assert.Equal(t, destinationY+offsetY, y)
			assert.False(t, math.IsNaN(x) || math.IsInf(y, 0))
			state.ElapsedMs += duration - (duration+1)/2
			for range 5 {
				state.EventQueueManager.Update()
			}
			scene.updateCombatImpactEffects(30)
			require.False(t, scene.combatMovement.Active)
			require.Len(t, scene.combatImpactEffects, 1)
			assert.Zero(t, scene.combatImpactEffects[0].Elapsed)
			assert.Equal(t, combatFireTintMaximumAlpha, scene.combatFireImpactAlpha(true, actor.EntityIndex, actor.Slot))
			assert.Len(t, combatFireEvents(*events), 1, "arrival must not resolve standing damage again")
			assert.NotEqual(t, actor, state.CombatScene.GetCurrentTurnIdentity())
			state.ElapsedMs += 225
			scene.updateCombatImpactEffects(0)
			assert.InDelta(t, combatFireTintMaximumAlpha/2, scene.combatFireImpactAlpha(true, actor.EntityIndex, actor.Slot), 0.001)
			state.ElapsedMs += 225
			scene.updateCombatImpactEffects(0)
			assert.Zero(t, scene.combatFireImpactAlpha(true, actor.EntityIndex, actor.Slot))
			require.Len(t, scene.combatImpactEffects, 1, "label outlives tint")
			state.ElapsedMs += 800
			scene.updateCombatImpactEffects(0)
			assert.Empty(t, scene.combatImpactEffects)
		})
	}
}

func TestCombatFireStationaryActionAndAdmissionClock(t *testing.T) {
	scene, actor, events := newCombatFireMovementTest(t, indexes.Fireplace)
	state := scene.gameState
	state.CombatScene.SetPartyMemberPosition(actor.EntityIndex, references.Position{X: 6, Y: 5})
	hpBefore := state.PartyState.GetCharacter(actor.EntityIndex).GetCurrentHp()
	callsBefore := state.GetRNG().Calls()
	scene.combatMapInputHandler(ebiten.KeySpace)
	require.Len(t, combatFireEvents(*events), 1)
	require.Len(t, scene.combatImpactEffects, 1)
	assert.Equal(t, hpBefore-7, state.PartyState.GetCharacter(actor.EntityIndex).GetCurrentHp())
	assert.Equal(t, callsBefore+1, state.GetRNG().Calls())
	scene.updateCombatImpactEffects(60)
	require.Len(t, scene.combatImpactEffects, 1)
	assert.Zero(t, scene.combatImpactEffects[0].Elapsed, "admission must not inherit the preceding delta")
	assert.Equal(t, combatFireTintMaximumAlpha, scene.combatFireImpactAlpha(true, actor.EntityIndex, 0))
	assert.False(t, state.EventQueueManager.HasWorkflow("player_combat_movement"))
	assert.False(t, scene.inputBlocked, "stationary burn feedback has no input ownership")
	assert.NotEqual(t, actor, state.CombatScene.GetCurrentTurnIdentity(), "the existing action boundary advances once")
}

func TestCombatFireSharedTerrainAndFieldMovementHaveOneOwner(t *testing.T) {
	for _, test := range []struct {
		name  string
		tile  indexes.SpriteIndex
		field bool
	}{
		{name: "fireplace", tile: indexes.Fireplace},
		{name: "lava", tile: indexes.Lava},
		{name: "magical fire", tile: indexes.BrickFloor, field: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			scene, actor, events := newCombatFireMovementTest(t, test.tile)
			state := scene.gameState
			wantCalls := uint64(1)
			if test.field {
				require.True(t, state.CombatScene.AddField(game_state.NewCombatField(
					game_state.CombatFieldFire, references.Position{X: 6, Y: 5})))
				state.SetRandomSeed(sceneAreaSpellSeed(t, func(rng *beradrng.Source) bool {
					return rng.Intn(256) >= 16 && rng.Intn(11) == 7
				}))
				wantCalls++ // The existing field-expiration roll precedes standing injury.
			}
			calls := state.GetRNG().Calls()
			scene.combatMapInputHandler(ebiten.KeyRight)
			fires := combatFireEvents(*events)
			require.Len(t, fires, 1)
			require.Len(t, scene.combatImpactEffects, 1)
			assert.Equal(t, 7, fires[0].HPLost)
			assert.Equal(t, calls+wantCalls, state.GetRNG().Calls())
			assert.Equal(t, uint16(93), state.PartyState.GetCharacter(actor.EntityIndex).GetCurrentHp())
			assert.True(t, scene.combatImpactEffects[0].fireMovement)
		})
	}
}

func TestCombatFireFlameWindFatalTypedOutcomeKeepsOneLabel(t *testing.T) {
	scene, callbacks, monster := newAreaSpellOutcomeScene(t, references.InFlamHur, references.MonsterSnake)
	state := scene.gameState
	monster.SetPosition(references.Position{X: 5, Y: 3})
	monster.SetCurrentHP(1)
	caster := state.CombatScene.GetActiveCombatantIndex()
	require.True(t, state.BeginCombatConeTargeting(game_state.CastRequest{
		Spell: references.InFlamHur, CasterIdx: caster,
	}).Valid)
	state.SetCombatConeDirection(references.Up)
	committed := state.ConfirmCombatConeTarget()
	state.SetRandomSeed(sceneAreaSpellSeed(t, func(rng *beradrng.Source) bool { return rng.Intn(31) == 1 }))
	result := state.ResolveCommittedCombatCone(committed)
	require.Len(t, result.Targets, 1)
	require.True(t, result.Targets[0].Fatal)
	assert.Equal(t, 1, result.Targets[0].HPLost)
	require.Len(t, scene.combatImpactEffects, 1)
	assert.Empty(t, callbacks.CombatImpactCalls)
	assert.Len(t, combatAreaResultEvents(callbacks.CombatEvents), 1)
	effect := scene.combatImpactEffects[0]
	assert.Equal(t, game_state.CombatSpellDamaged, effect.SpellOutcome)
	assert.True(t, effect.Fire)
	assert.False(t, effect.fireMovement)
	label, _ := combatImpactText(effect)
	assert.Equal(t, "SLAIN", label)
	calls := state.GetRNG().Calls()
	scene.updateCombatImpactEffects(30)
	require.Len(t, scene.combatImpactEffects, 1, "fatal typed result survives the death object's replacement of the sprite")
	assert.Zero(t, scene.combatImpactEffects[0].Elapsed)
	assert.Equal(t, calls, state.GetRNG().Calls())
	monster.SetInvisible(true)
	scene.updateCombatImpactEffects(0)
	assert.Empty(t, scene.combatImpactEffects, "fatality does not override disclosure")
}

func TestCombatFireKeyboardZeroFatalAndBlocked(t *testing.T) {
	for _, test := range []struct {
		name    string
		roll    int
		hp      uint16
		blocked bool
	}{
		{name: "zero", roll: 0, hp: 100},
		{name: "fatal", roll: 7, hp: 3},
		{name: "blocked", roll: 7, hp: 100, blocked: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			scene, actor, events := newCombatFireMovementTest(t, indexes.Fireplace)
			state := scene.gameState
			character := state.PartyState.GetCharacter(actor.EntityIndex)
			character.SetHp(test.hp)
			setCombatFireTestRoll(t, state, test.roll)
			if test.blocked {
				require.True(t, state.CombatScene.AddField(game_state.NewCombatField(
					game_state.CombatFieldProtection, references.Position{X: 6, Y: 5})))
			}
			before := state.GetRNG().Calls()
			scene.combatMapInputHandler(ebiten.KeyRight)
			fires := combatFireEvents(*events)
			if test.blocked {
				assert.Empty(t, fires)
				assert.Equal(t, before, state.GetRNG().Calls())
				assert.Equal(t, test.hp, character.GetCurrentHp())
				assert.False(t, scene.combatMovement.Active)
			} else {
				require.Len(t, fires, 1)
				assert.Equal(t, min(int(test.hp), test.roll), fires[0].HPLost)
			}
			if test.name != "fatal" {
				assert.Empty(t, scene.combatImpactEffects)
				return
			}
			require.Len(t, scene.combatImpactEffects, 1)
			assert.True(t, character.IsDead())
			scene.updateCombatMovementProgress(1)
			scene.clearCombatMovement()
			scene.updateCombatImpactEffects(0)
			require.Len(t, scene.combatImpactEffects, 1, "fatal label survives sprite retirement")
			label, _ := combatImpactText(scene.combatImpactEffects[0])
			assert.Equal(t, "FALLEN", label)
			position, disclosed := state.ProjectCombatFireInjury(*fires[0].FireInjury, scene.combatImpactEffects[0].fireOwner)
			assert.True(t, disclosed)
			assert.Equal(t, fires[0].Target.Position, position)
			state.CombatScene.EncounterID++
			scene.updateCombatImpactEffects(0)
			assert.Empty(t, scene.combatImpactEffects)
		})
	}
}

func TestCombatFireProductionUpdateAdmissionAfterHitch(t *testing.T) {
	scene, actor, events := newCombatFireMovementTest(t, indexes.Fireplace)
	state := scene.gameState
	scene.windDisplay = &WindDisplay{}
	scene.moongateVisualSystem = NewMoongateVisualSystem(state)
	now := time.Unix(100, 0)
	scene.startTime, scene.lastEffectsUpdate, scene.now = now, now, func() time.Time { return now }
	state.ElapsedMs = 0
	var callsAtAdmission uint64
	state.EventQueueManager.Workflow("fire_movement_admission_test").Priority(event_queue.PriorityNormal).Sequential().First(func() {
		// Wind runs first in the production update. Seed the subsequent command,
		// not the independent wind subsystem, to select this injury deterministically.
		setCombatFireTestRoll(t, state, 7)
		callsAtAdmission = state.GetRNG().Calls()
		scene.combatMapInputHandler(ebiten.KeyRight)
	}).Start()
	now = now.Add(3 * time.Second)
	scene.updateWindAndEffects()
	require.Len(t, combatFireEvents(*events), 1)
	require.Len(t, scene.combatImpactEffects, 1)
	assert.Equal(t, callsAtAdmission+1, state.GetRNG().Calls())
	assert.Equal(t, int64(3000), scene.combatImpactEffects[0].fireAdmittedAt)
	assert.Zero(t, scene.combatImpactEffects[0].Elapsed, "the preceding hitch cannot age a newly admitted effect")
	for range 16 {
		scene.updateWindAndEffects()
	}
	now = now.Add(167 * time.Millisecond)
	scene.updateWindAndEffects()
	assert.Equal(t, combatFireTintMaximumAlpha, scene.combatFireImpactAlpha(true, actor.EntityIndex, actor.Slot))
	now = now.Add(166 * time.Millisecond)
	for range 16 {
		scene.updateWindAndEffects()
	}
	require.False(t, scene.combatMovement.Active)
	require.Len(t, scene.combatImpactEffects, 1)
	assert.Zero(t, scene.combatImpactEffects[0].Elapsed)
	assert.False(t, scene.inputBlocked, "the burn lifetime must not extend movement's input ownership")
	assert.Len(t, combatFireEvents(*events), 1)
	now = now.Add(1250 * time.Millisecond)
	scene.updateWindAndEffects()
	assert.Empty(t, scene.combatImpactEffects)
}

func TestCombatFireMovementEventArrivesBeforeInterpolationAndIsDetached(t *testing.T) {
	scene, actor, events := newCombatFireMovementTest(t, indexes.Fireplace)
	state := scene.gameState
	publish := state.SystemCallbacks.Combat.PublishEvent
	state.SystemCallbacks.Combat.PublishEvent = func(event game_state.CombatEvent) {
		if event.FireInjury != nil {
			assert.False(t, scene.combatMovement.Active, "authoritative injury precedes the animation")
			assert.Equal(t, uint16(93), state.PartyState.GetCharacter(actor.EntityIndex).GetCurrentHp())
		}
		publish(event)
	}
	scene.combatMapInputHandler(ebiten.KeyRight)
	fires := combatFireEvents(*events)
	require.Len(t, fires, 1)
	require.Len(t, scene.combatImpactEffects, 1)
	injury := *fires[0].FireInjury
	fires[0].FireInjury.HPLost = 99
	fires[0].FireInjury.Target.Slot++
	assert.Equal(t, injury, *scene.combatImpactEffects[0].FireInjury)
	found := false
	for _, event := range scene.combatHUD.history {
		if event.FireInjury != nil {
			assert.Equal(t, injury, *event.FireInjury, "history also owns its admitted value")
			found = true
		}
	}
	assert.True(t, found)
}

func TestCombatFireRepeatedMovesAndIndependentImpacts(t *testing.T) {
	scene, actor, events := newCombatFireMovementTest(t, indexes.Fireplace)
	state := scene.gameState
	scene.combatMapInputHandler(ebiten.KeyRight)
	drainCombatPacingAt(scene, state.ElapsedMs)
	drainCombatPacingAt(scene, state.ElapsedMs+333)
	require.Len(t, scene.combatImpactEffects, 1)
	first := scene.combatImpactEffects[0]
	// A second real keyboard move by the next actor reaches another fireplace
	// while the first actor's label remains independently live.
	next := state.CombatScene.GetCurrentTurnIdentity()
	require.True(t, next.IsPartyMember)
	require.NotEqual(t, actor, next)
	from, to := references.Position{X: 7, Y: 5}, references.Position{X: 8, Y: 5}
	state.CombatScene.SetPartyMemberPosition(next.EntityIndex, from)
	state.CombatScene.CombatMapRef.SetTileNumber(to, indexes.Fireplace)
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &to, indexes.Fireplace)
	state.PartyState.GetCharacter(next.EntityIndex).SetHp(100)
	state.PartyState.GetCharacter(next.EntityIndex).SetEquippedItem(references.SlotRing, references.NoEquipment)
	setCombatFireTestRoll(t, state, 7)
	scene.combatMapInputHandler(ebiten.KeyRight)
	require.Len(t, scene.combatImpactEffects, 2)
	assert.Equal(t, first, scene.combatImpactEffects[0], "new movement cannot restart or hide the earlier burn")
	assert.False(t, scene.combatImpactEffects[0].fireMovement)
	assert.True(t, scene.combatImpactEffects[1].fireMovement)
	assert.Equal(t, actor.EntityIndex, scene.combatImpactEffects[0].Feedback.Target.Index)
	assert.Equal(t, next.EntityIndex, scene.combatImpactEffects[1].Feedback.Target.Index)
	assert.Len(t, combatFireEvents(*events), 2)
	state.EventQueueManager.Shutdown()
	require.Len(t, scene.combatImpactEffects, 1)
	assert.Equal(t, first, scene.combatImpactEffects[0], "cancel only the current movement injury")
	state.ElapsedMs += combatImpactDurationMs
	scene.updateCombatImpactEffects(0)
	assert.Empty(t, scene.combatImpactEffects)
}

func TestCombatFireMoveBoundaryDoesNotBindEarlierSameTickInjury(t *testing.T) {
	scene, _, events := newCombatFireMovementTest(t, indexes.Fireplace)
	state := scene.gameState
	// Obtain a real detached injury, then replay its already-resolved presentation
	// at the next move's admission time. No new gameplay damage is inferred.
	scene.combatMapInputHandler(ebiten.KeyRight)
	drainCombatPacingAt(scene, state.ElapsedMs)
	drainCombatPacingAt(scene, state.ElapsedMs+333)
	fires := combatFireEvents(*events)
	require.Len(t, fires, 1)
	scene.recordCombatEvent(fires[0])
	require.Len(t, scene.combatImpactEffects, 2)
	require.False(t, scene.combatImpactEffects[1].fireMovementBound)
	scene.recordCombatEvent(game_state.CombatEvent{Kind: game_state.CombatEventMove, To: fires[0].Target.Position})
	target := fires[0].Target
	scene.startCombatMovement(target.IsPartyMember, target.Index, target.Slot,
		references.Position{X: 5, Y: 5}, target.Position)
	assert.False(t, scene.combatImpactEffects[1].fireMovement, "an older injury is not this move's result")
	scene.clearCombatMovement()
	assert.Len(t, scene.combatImpactEffects, 2)
}

func TestCombatFireEffectsBoundedAndClearOnExitOrReplacement(t *testing.T) {
	for _, transition := range []string{"exit", "replacement", "missing encounter"} {
		t.Run(transition, func(t *testing.T) {
			scene, actor, events := newCombatFireMovementTest(t, indexes.Fireplace)
			state := scene.gameState
			state.CombatScene.SetPartyMemberPosition(actor.EntityIndex, references.Position{X: 6, Y: 5})
			scene.combatMapInputHandler(ebiten.KeySpace)
			fires := combatFireEvents(*events)
			require.Len(t, fires, 1)
			scene.showCombatImpact(newTestCombatFeedback(game_state.CombatFeedbackMiss))
			for range combatFireImpactCapacity + 5 {
				scene.recordCombatEvent(fires[0])
			}
			require.Len(t, scene.combatImpactEffects, combatFireImpactCapacity+1)
			assert.False(t, scene.combatImpactEffects[0].Fire, "capacity must preserve unrelated impacts")
			unrelated := scene.combatImpactEffects[0]
			switch transition {
			case "replacement":
				combat := *state.CombatScene
				state.CombatScene = &combat
				require.True(t, state.IsInCombat())
			case "missing encounter":
				state.CombatScene = nil
				require.True(t, state.IsInCombat(), "a missing encounter alone is not a combat-location exit")
			case "exit":
				state.TransitionFromCombatMap()
				require.Nil(t, state.CombatScene)
				require.False(t, state.IsInCombat(), "real exit restores the pre-combat location")
			}
			scene.updateCombatImpactEffects(0)
			if transition != "exit" {
				require.Len(t, scene.combatImpactEffects, 1)
				assert.Equal(t, unrelated, scene.combatImpactEffects[0],
					"fire lifetime invalidation must discard every fire effect and preserve the unrelated impact unchanged")
			} else {
				assert.Empty(t, scene.combatImpactEffects, "actual combat exit clears every impact family")
			}
		})
	}
}

func TestCombatFireCancellationPreservesUnrelatedImpacts(t *testing.T) {
	scene, _, _ := newCombatFireMovementTest(t, indexes.Fireplace)
	scene.showCombatImpact(newTestCombatFeedback(game_state.CombatFeedbackMiss))
	scene.combatMapInputHandler(ebiten.KeyRight)
	require.Len(t, scene.combatImpactEffects, 2)
	scene.gameState.EventQueueManager.Shutdown()
	require.Len(t, scene.combatImpactEffects, 1)
	assert.False(t, scene.combatImpactEffects[0].Fire)
	assert.False(t, scene.combatMovement.Active)
}

func TestCombatFireMaskOverlayIsBoundedAndExpires(t *testing.T) {
	// A finite mask probe exercises the actual overlay renderer without a game
	// loop: transparent pixels must not acquire a full-tile red rectangle.
	scene := &GameScene{combatImpactEffects: []CombatImpactEffect{{
		Feedback: newTestCombatFeedback(game_state.CombatFeedbackDamage), Fire: true,
	}}}
	sprite := ebiten.NewImage(4, 4)
	t.Cleanup(sprite.Deallocate)
	sprite.Set(1, 1, color.White)
	screen := ebiten.NewImage(16, 16)
	t.Cleanup(screen.Deallocate)
	options := &ebiten.DrawImageOptions{}
	options.GeoM.Scale(2, 2)
	options.GeoM.Translate(4, 4)
	screen.Fill(ucolor.EGABlue)
	scene.drawCombatFireImpactOverlay(screen, sprite, options, true, 2, 0)
	pixel := color.RGBAModel.Convert(screen.At(6, 6)).(color.RGBA)
	assert.Greater(t, pixel.R, uint8(150), "opaque sprite pixels receive a strong burn cue")
	assert.Equal(t, ucolor.EGABlue.ToRGBA(), color.RGBAModel.Convert(screen.At(4, 4)))
	assert.Equal(t, ucolor.EGABlue.ToRGBA(), color.RGBAModel.Convert(screen.At(0, 0)))
	scene.combatImpactEffects[0].Elapsed = combatFireTintDurationSeconds
	screen.Fill(ucolor.EGABlue)
	scene.drawCombatFireImpactOverlay(screen, sprite, options, true, 2, 0)
	assert.Equal(t, ucolor.EGABlue.ToRGBA(), color.RGBAModel.Convert(screen.At(6, 6)))
}
