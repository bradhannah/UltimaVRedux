package main

import (
	"fmt"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func newCombatDoorSpellScene(t *testing.T, spell references.Spell) (*GameScene, *game_state.MockSystemCallbacks) {
	t.Helper()
	state, callbacks := newProductionCombatCastState(t)
	combat := state.CombatScene
	combat.TurnQueue.RemoveAllMonsters()
	combat.MonsterManager.Clear()
	for x := range references.XCombatMapTiles {
		for y := range references.YCombatMapTiles {
			position := references.Position{X: references.Coordinate(x), Y: references.Coordinate(y)}
			setProductionCombatPushTerrain(state, position, indexes.BrickFloor)
			state.GetCurrentLayeredMap().UnSetTileByLayer(map_state.MapOverrideLayer, &position)
		}
	}
	combat.SetPartyMemberPosition(combat.GetActiveCombatantIndex(), references.Position{X: 5, Y: 5})
	state.PartyState.Inventory.Spells.Set(spell, 2)
	state.ConfirmSpellKnowledge(spell, game_state.ExperimentationKnowledgeSource())
	callbacks.Reset()

	return newProductionCastTestScene(state), callbacks
}

func selectProductionCombatDoorSpell(t *testing.T, scene *GameScene, spell references.Spell) {
	t.Helper()
	scene.combatMapInputHandler(ebiten.KeyC)
	dialog := waitForTopModal[*widgets.SpellCastingDialog](t, scene)
	keys := []ebiten.Key{ebiten.KeyI, ebiten.KeyE, ebiten.KeyP}
	switch spell {
	case references.AnSanct:
		keys = []ebiten.Key{ebiten.KeyA, ebiten.KeyS}
	case references.AnExPor:
		keys = []ebiten.Key{ebiten.KeyA, ebiten.KeyE, ebiten.KeyP}
	}
	enterSpellSyllables(scene, dialog, keys)
	require.Equal(t, spell, dialog.GetSelectedSpell())
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	dialog.Update()
	for range 30 {
		scene.gameState.ElapsedMs += DialogPollingRateMs
		scene.gameState.EventQueueManager.Update()
		if scene.gameState.CombatScene.AdjacentSpellTargeting != nil &&
			!scene.gameState.EventQueueManager.HasWorkflow("spell_casting_workflow") {
			require.Equal(t, CombatAdjacentSpellTargetInput, scene.secondaryKeyState)
			require.Zero(t, scene.dialogStack.Count())
			require.False(t, scene.gameState.EventQueueManager.HasInputBlockingWorkflows())
			return
		}
	}
	t.Fatal("Cast did not hand off to adjacent targeting")
}

func TestCombatDoorSpellProductionSyllablesRespectInjectedControlAndRelease(t *testing.T) {
	for _, control := range []ebiten.Key{ebiten.KeyControlLeft, ebiten.KeyControlRight} {
		for _, test := range []struct {
			spell references.Spell
			keys  []ebiten.Key
		}{
			{references.AnSanct, []ebiten.Key{ebiten.KeyA, ebiten.KeyS}},
			{references.AnExPor, []ebiten.Key{ebiten.KeyA, ebiten.KeyE, ebiten.KeyP}},
			{references.InExPor, []ebiten.Key{ebiten.KeyI, ebiten.KeyE, ebiten.KeyP}},
		} {
			t.Run(fmt.Sprintf("%s/%s", control, references.GetSpellName(test.spell)), func(t *testing.T) {
				scene, callbacks := newCombatDoorSpellScene(t, test.spell)
				state := scene.gameState
				t.Cleanup(state.EventQueueManager.Shutdown)
				// Battle opens on what the actor can cast now. A second ready spell,
				// listed first, keeps the opening selection off the one under test.
				state.PartyState.Inventory.Spells.Set(references.Mani, 1)
				controlHeld := true
				scene.keyboard = input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool {
					return controlHeld && key == control
				})
				scene.combatMapInputHandler(ebiten.KeyC)
				dialog := waitForTopModal[*widgets.SpellCastingDialog](t, scene)
				initialSpell := dialog.GetSelectedSpell()
				require.Equal(t, references.Mani, initialSpell)
				commits := state.CastUseCommitSequence()
				enterSpellSyllables(scene, dialog, test.keys)
				assert.Equal(t, initialSpell, dialog.GetSelectedSpell(),
					"a simulated letter does not override a polled Ctrl modifier: the opening selection stays")
				assert.Nil(t, state.CombatScene.AdjacentSpellTargeting)
				assert.Equal(t, commits, state.CastUseCommitSequence())

				controlHeld = false
				enterSpellSyllables(scene, dialog, test.keys)
				require.Equal(t, test.spell, dialog.GetSelectedSpell(), "released Ctrl must permit the complete syllable sequence")
				scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
				dialog.Update()
				for range 30 {
					state.ElapsedMs += DialogPollingRateMs
					state.EventQueueManager.Update()
				}
				require.NotNil(t, state.CombatScene.AdjacentSpellTargeting)
				assert.Equal(t, test.spell, state.CombatScene.AdjacentSpellTargeting.Request.Spell)
				assert.Equal(t, CombatAdjacentSpellTargetInput, scene.secondaryKeyState)
				assert.Equal(t, commits, state.CastUseCommitSequence(), "aiming must still be uncommitted")
				assert.False(t, callbacks.InputBlocked)
				assert.False(t, state.EventQueueManager.HasWorkflow("spell_casting_workflow"))
				assert.Zero(t, scene.dialogStack.Count())
			})
		}
	}
}

func pressCombatDoorSpellKey(scene *GameScene, key ebiten.Key) {
	scene.keyboard.SimulateKeyPress(key)
	scene.combatMapHandleSecondaryInput()
}

func waitForCombatDoorSpellPhase(t *testing.T, scene *GameScene, phase combatUtilityEffectPhase) {
	t.Helper()
	for range 100 {
		scene.gameState.EventQueueManager.Update()
		if scene.combatUtilityEffect != nil && scene.combatUtilityEffect.Phase == phase {
			return
		}
		scene.gameState.ElapsedMs += 10
	}
	t.Fatalf("door spell did not reach phase %v", phase)
}

func drainCombatDoorSpellEffect(t *testing.T, scene *GameScene) {
	t.Helper()
	for range 100 {
		scene.gameState.ElapsedMs += 25
		scene.gameState.EventQueueManager.Update()
		if !scene.gameState.EventQueueManager.HasWorkflow("combat_utility_spell_effect") {
			return
		}
	}
	t.Fatal("door spell effect did not complete")
}

func assertSceneCombatDoorTerrain(t *testing.T, state *game_state.GameState, target references.Position, want indexes.SpriteIndex) {
	t.Helper()
	assert.Equal(t, want, state.CombatScene.CombatMapRef.GetTileNumber(target))
	assert.Equal(t, want, state.GetCurrentLayeredMap().SpriteIndexByLayer(map_state.MapLayer, target))
	assert.Equal(t, want, state.GetCurrentLayeredMap().GetTileTopMapOnlyTile(&target).Index)
	assert.Equal(t, want, state.GetEffectiveTile(&target), "renderer and Open must see the committed door")
}

func combatDoorSpellEvents(callbacks *game_state.MockSystemCallbacks) []game_state.CombatEvent {
	var events []game_state.CombatEvent
	for _, event := range callbacks.CombatEvents {
		if event.Kind == game_state.CombatEventUtilitySpell {
			events = append(events, event)
		}
	}

	return events
}

func TestCombatDoorSpellProductionCastThenOpen(t *testing.T) {
	for _, territory := range []references.CombatMapTerritory{references.BritanniaCombatTerritory, references.DungeonCombatTerritory} {
		for _, test := range []struct {
			spell references.Spell
			from  indexes.SpriteIndex
			to    indexes.SpriteIndex
		}{
			{references.InExPor, indexes.MagicLockDoor, indexes.RegularDoor},
			{references.InExPor, indexes.MagicLockDoorWithView, indexes.RegularDoorView},
			{references.AnSanct, indexes.LockedDoor, indexes.RegularDoor},
			{references.AnSanct, indexes.LockedDoorView, indexes.RegularDoorView},
			{references.AnExPor, indexes.RegularDoor, indexes.MagicLockDoor},
			{references.AnExPor, indexes.RegularDoorView, indexes.MagicLockDoorWithView},
			{references.AnExPor, indexes.LockedDoor, indexes.MagicLockDoor},
			{references.AnExPor, indexes.LockedDoorView, indexes.MagicLockDoorWithView},
		} {
			t.Run(fmt.Sprintf("%v/%v/%v", territory, test.spell, test.from), func(t *testing.T) {
				scene, callbacks := newCombatDoorSpellScene(t, test.spell)
				baseline, _ := newCombatDoorSpellScene(t, test.spell)
				state, expected := scene.gameState, baseline.gameState
				target := references.Position{X: 6, Y: 5}
				for _, candidate := range []*game_state.GameState{state, expected} {
					candidate.CombatScene.CombatMapRef.Territory = territory
					setProductionCombatPushTerrain(candidate, target, test.from)
					candidate.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, &target, test.from)
				}
				actor := state.CombatScene.GetCurrentTurnIdentity()
				caster := state.PartyState.GetCharacter(actor.EntityIndex)
				publish := state.SystemCallbacks.Combat.PublishEvent
				state.SystemCallbacks.Combat.PublishEvent = func(event game_state.CombatEvent) {
					if event.Kind == game_state.CombatEventUtilitySpell {
						assertSceneCombatDoorTerrain(t, state, target, test.to)
						assert.Equal(t, actor.EntityIndex, event.Attacker.Index)
						assert.Equal(t, actor.Slot, event.Attacker.Slot)
					}
					publish(event)
				}
				mp, rng, commits := caster.GetCurrentMp(), state.GetRNG().Calls(), state.CastUseCommitSequence()
				selectProductionCombatDoorSpell(t, scene, test.spell)
				assert.Equal(t, mp, caster.GetCurrentMp())
				assert.EqualValues(t, 2, state.PartyState.Inventory.Spells.Get(test.spell))
				assertCombatMatchesSingleFinish(t, state, expected)
				pressCombatDoorSpellKey(scene, ebiten.KeyRight)
				cursor, visible := scene.combatDoorSpellCursor()
				require.True(t, visible)
				assert.Equal(t, target, cursor)
				pressCombatDoorSpellKey(scene, ebiten.KeyEnter)
				assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
				assert.Equal(t, commits+1, state.CastUseCommitSequence())
				assert.Equal(t, mp-byte(references.GetSpellCircle(test.spell)), caster.GetCurrentMp())
				assert.EqualValues(t, 1, state.PartyState.Inventory.Spells.Get(test.spell))
				assert.Equal(t, game_state.CombatAdjacentSpellTargetIgnored, state.ConfirmCombatAdjacentSpellTarget().Outcome)
				_, visible = scene.combatDoorSpellCursor()
				assert.False(t, visible, "paid casts no longer expose an aiming cursor")
				waitForCombatDoorSpellPhase(t, scene, combatUtilityApproach)
				assertSceneCombatDoorTerrain(t, state, target, test.from)
				assert.Empty(t, combatDoorSpellEvents(callbacks))
				assertCombatMatchesSingleFinish(t, state, expected)
				waitForCombatDoorSpellPhase(t, scene, combatUtilityResult)
				assertSceneCombatDoorTerrain(t, state, target, test.to)
				require.Len(t, combatDoorSpellEvents(callbacks), 1)
				assert.Equal(t, rng, state.GetRNG().Calls())
				assertCombatMatchesSingleFinish(t, state, expected)
				drainCombatDoorSpellEffect(t, scene)
				expected.FinishCombatAction()
				assertCombatMatchesSingleFinish(t, state, expected)
				assert.Equal(t, expected.GetRNG().Calls(), state.GetRNG().Calls())
				assert.Nil(t, scene.combatUtilityEffect)
				assert.Nil(t, state.CombatScene.AdjacentSpellTargeting)
				assert.False(t, callbacks.InputBlocked)
				drainCombatDoorSpellEffect(t, scene)
				assertCombatMatchesSingleFinish(t, state, expected)
				require.Len(t, combatDoorSpellEvents(callbacks), 1)

				// Give the next scheduled PC access to the same door, then use
				// ordinary production Open routing after Cast's finalizer ran.
				next := state.CombatScene.GetCurrentTurnIdentity()
				require.True(t, next.IsPartyMember)
				state.CombatScene.SetPartyMemberPosition(next.EntityIndex, references.Position{X: 5, Y: 5})
				scene.combatMapInputHandler(ebiten.KeyO)
				require.Equal(t, OpenDirectionInput, scene.secondaryKeyState)
				pressCombatDoorSpellKey(scene, ebiten.KeyRight)
				if test.spell == references.AnExPor {
					assertSceneCombatDoorTerrain(t, state, target, test.to)
					assert.Contains(t, callbacks.Messages, "Locked!")
				} else {
					assertSceneCombatDoorTerrain(t, state, target, indexes.BrickFloor)
					assert.Contains(t, callbacks.Messages, "Opened!")
					assert.NotContains(t, callbacks.Messages, "Locked!")
				}
				assert.Equal(t, commits+1, state.CastUseCommitSequence())
				assert.EqualValues(t, 1, state.PartyState.Inventory.Spells.Get(test.spell))
			})
		}
	}
}

func TestCombatDoorSpellProductionInvalidCancelAndCardinalCursor(t *testing.T) {
	for _, origin := range []references.Position{{}, {X: 10}, {Y: 10}, {X: 10, Y: 10}, {X: 5, Y: 5}} {
		t.Run(fmt.Sprint(origin), func(t *testing.T) {
			scene, callbacks := newCombatDoorSpellScene(t, references.InExPor)
			state := scene.gameState
			actor := state.CombatScene.GetCurrentTurnIdentity()
			state.CombatScene.SetPartyMemberPosition(actor.EntityIndex, origin)
			before := snapshotBlockedCombatState(t, state)
			mp := state.PartyState.GetCharacter(actor.EntityIndex).GetCurrentMp()
			selectProductionCombatDoorSpell(t, scene, references.InExPor)
			initial, visible := scene.combatDoorSpellCursor()
			require.True(t, visible)
			wantInitial := references.Position{X: origin.X, Y: origin.Y - 1}
			if origin.Y == 0 {
				wantInitial = references.Position{X: origin.X + 1, Y: origin.Y}
				if origin.X == references.XCombatMapTiles-1 {
					wantInitial = references.Position{X: origin.X, Y: origin.Y + 1}
				}
			}
			assert.Equal(t, wantInitial, initial, "initial neighbour is geometric, not eligibility-driven")
			for _, key := range []ebiten.Key{
				ebiten.KeyUp, ebiten.KeyDown, ebiten.KeyLeft, ebiten.KeyRight,
				ebiten.KeyNumpad8, ebiten.KeyNumpad2, ebiten.KeyNumpad4, ebiten.KeyNumpad6,
			} {
				pressCombatDoorSpellKey(scene, key)
				cursor, visible := scene.combatDoorSpellCursor()
				require.True(t, visible)
				assert.GreaterOrEqual(t, int(cursor.X), 0)
				assert.GreaterOrEqual(t, int(cursor.Y), 0)
				assert.Less(t, int(cursor.X), int(references.XCombatMapTiles))
				assert.Less(t, int(cursor.Y), int(references.YCombatMapTiles))
				dx, dy := int(cursor.X-origin.X), int(cursor.Y-origin.Y)
				assert.Equal(t, 1, dx*dx+dy*dy)
				pressCombatDoorSpellKey(scene, key)
				repeated, _ := scene.combatDoorSpellCursor()
				assert.Equal(t, cursor, repeated, "repeated movement cannot extend range")
				for _, diagonal := range []ebiten.Key{ebiten.KeyNumpad7, ebiten.KeyNumpad9, ebiten.KeyNumpad1, ebiten.KeyNumpad3} {
					pressCombatDoorSpellKey(scene, diagonal)
					unchanged, _ := scene.combatDoorSpellCursor()
					assert.Equal(t, cursor, unchanged)
				}
				for _, confirm := range []ebiten.Key{ebiten.KeyEnter, ebiten.KeyNumpadEnter, ebiten.KeyC, ebiten.KeySpace} {
					pressCombatDoorSpellKey(scene, confirm)
					assert.Equal(t, CombatAdjacentSpellTargetInput, scene.secondaryKeyState)
				}
			}
			pressCombatDoorSpellKey(scene, ebiten.KeyEscape)
			before.ElapsedMs = state.ElapsedMs
			assert.Equal(t, before, snapshotBlockedCombatState(t, state))
			assert.Equal(t, mp, state.PartyState.GetCharacter(actor.EntityIndex).GetCurrentMp())
			assert.Zero(t, state.CastUseCommitSequence())
			assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
			assert.Nil(t, state.CombatScene.AdjacentSpellTargeting)
			assert.Nil(t, scene.combatUtilityEffect)
			assert.Empty(t, combatDoorSpellEvents(callbacks))
		})
	}
}

func TestCombatDoorSpellProductionShutdownSettlesPaidCastExactlyOnce(t *testing.T) {
	for _, phase := range []string{"before first update", "approach", "result"} {
		t.Run(phase, func(t *testing.T) {
			scene, callbacks := newCombatDoorSpellScene(t, references.InExPor)
			baseline, _ := newCombatDoorSpellScene(t, references.InExPor)
			state := scene.gameState
			target := references.Position{X: 6, Y: 5}
			setProductionCombatPushTerrain(state, target, indexes.MagicLockDoor)
			actor := state.CombatScene.GetCurrentTurnIdentity()
			mp := state.PartyState.GetCharacter(actor.EntityIndex).GetCurrentMp()
			selectProductionCombatDoorSpell(t, scene, references.InExPor)
			pressCombatDoorSpellKey(scene, ebiten.KeyRight)
			pressCombatDoorSpellKey(scene, ebiten.KeySpace)
			if phase == "approach" {
				waitForCombatDoorSpellPhase(t, scene, combatUtilityApproach)
			} else if phase == "result" {
				waitForCombatDoorSpellPhase(t, scene, combatUtilityResult)
			}
			state.EventQueueManager.Shutdown()
			state.EventQueueManager.Shutdown()
			baseline.gameState.FinishCombatAction()
			assertCombatMatchesSingleFinish(t, state, baseline.gameState)
			assertSceneCombatDoorTerrain(t, state, target, indexes.RegularDoor)
			assert.Equal(t, mp-byte(references.GetSpellCircle(references.InExPor)), state.PartyState.GetCharacter(actor.EntityIndex).GetCurrentMp())
			assert.EqualValues(t, 1, state.PartyState.Inventory.Spells.Get(references.InExPor))
			assert.EqualValues(t, 1, state.CastUseCommitSequence())
			require.Len(t, combatDoorSpellEvents(callbacks), 1)
			assert.Nil(t, scene.combatUtilityEffect)
			assert.Nil(t, state.CombatScene.AdjacentSpellTargeting)
			assert.False(t, state.EventQueueManager.HasInputBlockingWorkflows())
			assert.False(t, callbacks.InputBlocked)
		})
	}
}

func TestCombatDoorSpellProductionStaleOwnerCannotMutateOrFinishReplacement(t *testing.T) {
	for _, phase := range []string{"selection", "approach", "result"} {
		for _, change := range []string{"actor", "clone", "slot provider", "later dispatch", "encounter", "moved", "terrain map", "layered map"} {
			t.Run(phase+"/"+change, func(t *testing.T) {
				scene, callbacks := newCombatDoorSpellScene(t, references.InExPor)
				state := scene.gameState
				target := references.Position{X: 6, Y: 5}
				setProductionCombatPushTerrain(state, target, indexes.MagicLockDoor)
				selectProductionCombatDoorSpell(t, scene, references.InExPor)
				pressCombatDoorSpellKey(scene, ebiten.KeyRight)
				if phase != "selection" {
					pressCombatDoorSpellKey(scene, ebiten.KeyC)
					waitForCombatDoorSpellPhase(t, scene, combatUtilityApproach)
					if phase == "result" {
						waitForCombatDoorSpellPhase(t, scene, combatUtilityResult)
					}
				}
				changeCombatKeyWatchScene(t, scene, references.SkullKeys, change)
				before := snapshotBlockedCombatState(t, state)
				commits, events := state.CastUseCommitSequence(), len(combatDoorSpellEvents(callbacks))
				if phase == "selection" {
					pressCombatDoorSpellKey(scene, ebiten.KeyEnter)
				} else {
					drainCombatDoorSpellEffect(t, scene)
				}
				before.ElapsedMs = state.ElapsedMs
				assert.Equal(t, before, snapshotBlockedCombatState(t, state))
				assert.Equal(t, commits, state.CastUseCommitSequence())
				assert.Len(t, combatDoorSpellEvents(callbacks), events)
				assert.Nil(t, scene.combatUtilityEffect)
				assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
				assert.False(t, state.EventQueueManager.HasInputBlockingWorkflows())
			})
		}
	}
}

func TestCombatDoorSpellProductionChangedTargetFinishesWithoutSuccess(t *testing.T) {
	for _, layerOnly := range []bool{false, true} {
		t.Run(fmt.Sprint(layerOnly), func(t *testing.T) {
			scene, callbacks := newCombatDoorSpellScene(t, references.InExPor)
			baseline, _ := newCombatDoorSpellScene(t, references.InExPor)
			state := scene.gameState
			target := references.Position{X: 6, Y: 5}
			setProductionCombatPushTerrain(state, target, indexes.MagicLockDoor)
			selectProductionCombatDoorSpell(t, scene, references.InExPor)
			pressCombatDoorSpellKey(scene, ebiten.KeyRight)
			pressCombatDoorSpellKey(scene, ebiten.KeyNumpadEnter)
			waitForCombatDoorSpellPhase(t, scene, combatUtilityApproach)
			if layerOnly {
				state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, &target, indexes.BrickFloor)
			} else {
				setProductionCombatPushTerrain(state, target, indexes.BrickFloor)
			}
			drainCombatDoorSpellEffect(t, scene)
			baseline.gameState.FinishCombatAction()
			assertCombatMatchesSingleFinish(t, state, baseline.gameState)
			assert.Empty(t, combatDoorSpellEvents(callbacks))
			assert.EqualValues(t, indexes.BrickFloor, state.GetEffectiveTile(&target))
			assert.EqualValues(t, 1, state.PartyState.Inventory.Spells.Get(references.InExPor))
			assert.EqualValues(t, 1, state.CastUseCommitSequence())
			assert.Nil(t, scene.combatUtilityEffect)
		})
	}
}

func TestCombatDoorSpellProductionRevalidatesResourcesAtConfirmation(t *testing.T) {
	for _, resource := range []string{"MP", "prepared spell"} {
		t.Run(resource, func(t *testing.T) {
			scene, callbacks := newCombatDoorSpellScene(t, references.InExPor)
			state := scene.gameState
			target := references.Position{X: 6, Y: 5}
			setProductionCombatPushTerrain(state, target, indexes.MagicLockDoor)
			selectProductionCombatDoorSpell(t, scene, references.InExPor)
			pressCombatDoorSpellKey(scene, ebiten.KeyRight)
			caster := state.PartyState.GetCharacter(state.CombatScene.GetActiveCombatantIndex())
			if resource == "MP" {
				caster.SetMp(0)
			} else {
				state.PartyState.Inventory.Spells.Set(references.InExPor, 0)
			}
			before, mp := snapshotBlockedCombatState(t, state), caster.GetCurrentMp()
			pressCombatDoorSpellKey(scene, ebiten.KeyEnter)
			assert.Equal(t, before, snapshotBlockedCombatState(t, state))
			assert.Equal(t, mp, caster.GetCurrentMp())
			assert.Zero(t, state.CastUseCommitSequence())
			assertSceneCombatDoorTerrain(t, state, target, indexes.MagicLockDoor)
			assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
			assert.Nil(t, state.CombatScene.AdjacentSpellTargeting)
			assert.False(t, state.EventQueueManager.HasActiveWorkflows())
			assert.Empty(t, combatDoorSpellEvents(callbacks))
			require.NotEmpty(t, callbacks.CommandPrompts)
			assert.Empty(t, callbacks.CommandPrompts[len(callbacks.CommandPrompts)-1])
		})
	}
}

func TestCombatDoorSpellProductionAimDoesNotDiscloseEligibilityOrTrap(t *testing.T) {
	for _, spell := range []references.Spell{references.InExPor, references.AnExPor, references.AnSanct} {
		var neutral string
		for _, kind := range []string{"invalid", "eligible door", "concealed", "trapped chest", "safe chest"} {
			t.Run(fmt.Sprintf("%v/%s", spell, kind), func(t *testing.T) {
				scene, callbacks := newCombatDoorSpellScene(t, spell)
				state := scene.gameState
				target := references.Position{X: 6, Y: 5}
				switch kind {
				case "eligible door":
					tile := indexes.SpriteIndex(indexes.MagicLockDoor)
					if spell == references.AnExPor {
						tile = indexes.RegularDoor
					} else if spell == references.AnSanct {
						tile = indexes.LockedDoor
					}
					setProductionCombatPushTerrain(state, target, tile)
				case "concealed":
					setProductionCombatPushTerrain(state, target, indexes.BlackSquare)
				case "trapped chest", "safe chest":
					require.True(t, state.CombatScene.AddObject(game_state.NewCombatChest(target, 10, kind == "trapped chest")))
				}
				selectProductionCombatDoorSpell(t, scene, spell)
				pressCombatDoorSpellKey(scene, ebiten.KeyRight)
				cursor, visible := scene.combatDoorSpellCursor()
				require.True(t, visible)
				assert.Equal(t, target, cursor)
				require.NotEmpty(t, callbacks.CommandPrompts)
				prompt := callbacks.CommandPrompts[len(callbacks.CommandPrompts)-1]
				if neutral == "" {
					neutral = prompt
				}
				assert.Equal(t, neutral, prompt)
				assert.Contains(t, prompt, "Enter/C/Space: cast")
				assert.NotContains(t, prompt, "invalid")
				assert.NotContains(t, prompt, "trapped")
				pressCombatDoorSpellKey(scene, ebiten.KeyEscape)
				assert.Zero(t, state.CastUseCommitSequence())
			})
		}
	}
}

func TestCombatDoorSpellProductionDrawUsesOriginalWhiteCursor(t *testing.T) {
	for _, spell := range []references.Spell{references.AnSanct, references.AnExPor, references.InExPor} {
		t.Run(references.GetSpellName(spell), func(t *testing.T) {
			scene, actor := newCombatMarkerTestScene(t)
			state := scene.gameState
			caster := state.PartyState.GetCharacter(actor.EntityIndex)
			caster.Class, caster.Level = party_state.Wizard, 8
			caster.SetMp(30)
			state.PartyState.Inventory.Spells.Set(spell, 2)
			state.ConfirmSpellKnowledge(spell, game_state.ExperimentationKnowledgeSource())
			selectProductionCombatDoorSpell(t, scene, spell)
			pressCombatDoorSpellKey(scene, ebiten.KeyUp)
			cursor, visible := scene.combatDoorSpellCursor()
			require.True(t, visible)
			options := scene.combatSquareMarkerDrawOptions(cursor)
			bounds := combatFixtureRasterBounds(options)
			screen := ebiten.NewImageWithOptions(bounds, &ebiten.NewImageOptions{Unmanaged: true})
			defer screen.Dispose()
			before := snapshotBlockedCombatState(t, state)
			scene.drawCombatDirectionIndicators(screen)
			scene.drawCombatAdjacentSpellTargetPreview(screen)
			assert.Equal(t, make([]byte, bounds.Dx()*bounds.Dy()*4), signaturePixels(screen),
				"door spells omit directional arrows and the old coloured underlay")
			// This calls the shared production aim layer, not the door helper or
			// reticle renderer directly. It requires the parent-owned draw hook.
			scene.drawCombatAttackCrosshair(screen)
			assertOriginalReticlePixels(t, screen, options, nil)
			assert.Equal(t, before, snapshotBlockedCombatState(t, state))
			pressCombatDoorSpellKey(scene, ebiten.KeyEscape)
			screen.Clear()
			scene.drawCombatAttackCrosshair(screen)
			assert.Equal(t, make([]byte, bounds.Dx()*bounds.Dy()*4), signaturePixels(screen))
		})
	}
}

func TestCombatDoorSpellProductionCombatExitRetiresPaidEffect(t *testing.T) {
	scene, callbacks := newCombatDoorSpellScene(t, references.InExPor)
	state := scene.gameState
	combat := state.CombatScene
	target := references.Position{X: 6, Y: 5}
	setProductionCombatPushTerrain(state, target, indexes.MagicLockDoor)
	selectProductionCombatDoorSpell(t, scene, references.InExPor)
	pressCombatDoorSpellKey(scene, ebiten.KeyRight)
	pressCombatDoorSpellKey(scene, ebiten.KeyEnter)
	waitForCombatDoorSpellPhase(t, scene, combatUtilityApproach)
	state.CombatScene = nil
	state.MapState.PlayerLocation.Location = references.Britain
	beforeRNG, beforeTime := state.GetRNG().Calls(), state.DateTime
	drainCombatDoorSpellEffect(t, scene)
	assert.Equal(t, beforeRNG, state.GetRNG().Calls())
	assert.Equal(t, beforeTime, state.DateTime)
	assert.EqualValues(t, indexes.MagicLockDoor, combat.CombatMapRef.GetTileNumber(target))
	assert.Nil(t, combat.AdjacentSpellTargeting)
	assert.Nil(t, scene.combatUtilityEffect)
	assert.Empty(t, combatDoorSpellEvents(callbacks))
	assert.EqualValues(t, 1, state.CastUseCommitSequence())
	assert.EqualValues(t, 1, state.PartyState.Inventory.Spells.Get(references.InExPor))
	assert.False(t, callbacks.InputBlocked)
}
