package main

import (
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// Equal-DEX PCs precede a single fast enemy. Selecting the last PC means that
// completion must offer PC 0 after a reset, rather than synchronously passing it.
func newCombatSoloResetUpdateScene(t *testing.T, claimed bool) (
	*GameScene, map[ebiten.Key]bool, *game_state.MockSystemCallbacks, game_state.CombatSoloResetBoundary,
) {
	t.Helper()
	scene, held, callbacks := newCombatDirectionUpdateScene(t, "first_actor")
	state := scene.gameState
	combat := state.CombatScene
	combat.MonsterManager.Clear()
	combat.TurnQueue.RemoveAllMonsters()
	selected := 0
	for index, active := range combat.PartyActive {
		if active {
			selected = index
			combat.SetPartyMemberPosition(index, references.Position{X: references.Coordinate(index + 3), Y: 9})
		}
	}
	require.Positive(t, selected)
	for x := range references.XCombatMapTiles {
		for y := range references.YCombatMapTiles {
			setProductionCombatPushTerrain(state, references.Position{X: references.Coordinate(x), Y: references.Coordinate(y)}, indexes.BrickFloor)
		}
	}
	reference := (*state.GameReferences.EnemyReferences)[references.MonsterRat]
	reference.Dexterity = 35
	monster := game_state.NewCombatMonster(references.MonsterRat, &reference,
		references.Position{X: 1, Y: 1}, state.AsRandomProvider())
	combat.AddMonsterToQueue(monster)
	require.True(t, state.SelectCombatPlayer(selected))
	state.SystemCallbacks.Visual.IsAnyKeyPressed = scene.IsAnyKeyPressed
	scene.keyboard.SetElapsedMillisecondsSource(scene.getElapsedMs)
	state.FinishCombatAction()
	boundary, eligible := state.ProjectCombatSoloReset()
	require.True(t, eligible)
	require.False(t, boundary.Claimed)
	require.Equal(t, game_state.CombatPresentationMonsterTurn, boundary.Step.Kind)
	if claimed {
		require.NoError(t, scene.Update(nil))
		require.True(t, scene.inputBlocked)
		boundary, eligible = state.ProjectCombatSoloReset()
		require.True(t, eligible)
		require.True(t, boundary.Claimed)
	}
	callbacks.Reset()
	t.Cleanup(func() {
		clear(held)
		scene.ResetTransientCommandInput()
		state.EventQueueManager.Shutdown()
	})

	return scene, held, callbacks, boundary
}

func finishCombatSoloResetWait(t *testing.T, scene *GameScene) {
	t.Helper()
	for range 300 {
		scene.gameState.ElapsedMs += 16
		require.NoError(t, scene.Update(nil))
		if !scene.inputBlocked && !scene.gameState.CombatScene.HasScheduledCombatStep() {
			return
		}
	}
	t.Fatal("enemy/skip owner did not release input")
}

func TestCombatSoloResetBriefPressAndHoldThroughUpdate(t *testing.T) {
	for _, hold := range []bool{false, true} {
		name := "brief press"
		if hold {
			name = "held through handoff"
		}
		t.Run(name, func(t *testing.T) {
			scene, held, callbacks, boundary := newCombatSoloResetUpdateScene(t, true)
			before := snapshotBlockedCombatState(t, scene.gameState)
			held[ebiten.Key0] = true
			require.NoError(t, scene.Update(nil))
			require.False(t, scene.gameState.CombatScene.IsSoloMoveActive())
			require.Equal(t, before, snapshotBlockedCombatState(t, scene.gameState))
			after, eligible := scene.gameState.ProjectCombatSoloReset()
			require.True(t, eligible)
			require.Equal(t, boundary, after)
			require.Equal(t, []string{"Set active player: None!"}, callbacks.Messages)
			require.Empty(t, callbacks.CommandPrompts)
			require.Zero(t, scene.dialogStack.Count())
			if !hold {
				delete(held, ebiten.Key0)
				require.NoError(t, scene.Update(nil))
				require.True(t, scene.inputBlocked, "a short tap is released before the enemy finishes")
			}
			finishCombatSoloResetWait(t, scene)
			require.Equal(t, game_state.CombatTurnIdentity{IsPartyMember: true, EntityIndex: 0}, scene.gameState.CombatScene.GetCurrentTurnIdentity())
			require.Equal(t, 1, strings.Count(strings.Join(callbacks.Messages, "\n"), "Set active player: None!"))
			require.Equal(t, PrimaryInput, scene.secondaryKeyState)
			require.ErrorIs(t, scene.gameState.CompleteScheduledCombatStep(boundary.Step.Token), game_state.ErrCombatPresentationDuplicate)
		})
	}
}

func TestCombatSoloResetSelectedConditionsDoNotRestrictRecovery(t *testing.T) {
	for _, condition := range []string{"sleep", "captive", "possessed"} {
		t.Run(condition, func(t *testing.T) {
			scene, held, _, boundary := newCombatSoloResetUpdateScene(t, true)
			combat := scene.gameState.CombatScene
			selected := combat.GetSoloMoveCharacter()
			switch condition {
			case "sleep":
				combat.PartySleeping[selected] = true
			case "captive":
				combat.PartyUnderground[selected] = true
			case "possessed":
				combat.PartyPossessed[selected] = true
			}
			_, eligible := scene.gameState.ProjectSoloCombatInput()
			require.False(t, eligible, "pending verbs intentionally reject this actor")
			sleeping, captive, possessed := combat.PartySleeping, combat.PartyUnderground, combat.PartyPossessed
			held[ebiten.Key0] = true
			require.NoError(t, scene.Update(nil))
			require.False(t, combat.IsSoloMoveActive())
			require.Equal(t, sleeping, combat.PartySleeping)
			require.Equal(t, captive, combat.PartyUnderground)
			require.Equal(t, possessed, combat.PartyPossessed)
			after, eligible := scene.gameState.ProjectCombatSoloReset()
			require.True(t, eligible)
			require.Equal(t, boundary, after)
		})
	}
}

func TestCombatSoloResetUnclaimedBoundaryThroughUpdate(t *testing.T) {
	scene, held, _, boundary := newCombatSoloResetUpdateScene(t, false)
	held[ebiten.Key0] = true
	require.NoError(t, scene.Update(nil))
	require.False(t, scene.gameState.CombatScene.IsSoloMoveActive())
	after, eligible := scene.gameState.ProjectCombatSoloReset()
	require.True(t, eligible)
	require.True(t, after.Claimed, "ordinary Update review installs the presentation owner")
	require.Equal(t, boundary.Step, after.Step)
	require.True(t, scene.gameState.EventQueueManager.HasOnlyWorkflow("monster_turn_delay"))
	finishCombatSoloResetWait(t, scene)
	require.Equal(t, 0, scene.gameState.CombatScene.GetCurrentTurnIdentity().EntityIndex)
}

func TestCombatSoloResetCompletionFramePrecedesSynchronousSoloSkips(t *testing.T) {
	scene, held, _, boundary := newCombatSoloResetUpdateScene(t, true)
	probe, _, _, _ := newCombatSoloResetUpdateScene(t, true)
	// Probe one ordinary Update ahead at identical elapsed times. Its real
	// workflow identifies the exact completion frame without editing the token.
	for range 300 {
		probe.gameState.ElapsedMs += 16
		require.NoError(t, probe.Update(nil))
		_, pending := probe.gameState.ProjectCombatSoloReset()
		scene.gameState.ElapsedMs += 16
		if !pending {
			held[ebiten.Key0] = true
			held[ebiten.KeyG], held[ebiten.KeyO], held[ebiten.KeyNumpad6] = true, true, true
			require.NoError(t, scene.Update(nil))
			require.False(t, scene.gameState.CombatScene.IsSoloMoveActive())
			require.False(t, scene.transientCommandInput.pending.active)
			require.Equal(t, PrimaryInput, scene.secondaryKeyState)
			for key := range held {
				require.True(t, scene.keyboard.IsSuppressedUntilRelease(key))
			}
			require.Equal(t, 0, scene.gameState.CombatScene.GetCurrentTurnIdentity().EntityIndex)
			require.Equal(t, probe.gameState.CombatScene.GetSoloMoveCharacter(), probe.gameState.CombatScene.GetCurrentTurnIdentity().EntityIndex)
			require.ErrorIs(t, scene.gameState.CompleteScheduledCombatStep(boundary.Step.Token), game_state.ErrCombatPresentationDuplicate)

			return
		}
		require.NoError(t, scene.Update(nil))
	}
	t.Fatal("completion frame was not reached")
}

func TestCombatSoloResetWinsVerbsAndPreservesEarlyDirectionSuppression(t *testing.T) {
	for _, direction := range []ebiten.Key{ebiten.KeyRight, ebiten.KeyNumpad6} {
		t.Run(direction.String(), func(t *testing.T) {
			scene, held, _, _ := newCombatSoloResetUpdateScene(t, true)
			held[ebiten.KeyG], held[direction] = true, true
			require.NoError(t, scene.Update(nil))
			require.True(t, scene.transientCommandInput.pending.active)
			require.True(t, scene.keyboard.IsSuppressedUntilRelease(direction))
			held[ebiten.Key0] = true
			for _, key := range []ebiten.Key{ebiten.KeyO, ebiten.KeyJ, ebiten.KeyP, ebiten.KeyS} {
				held[key] = true
			}
			require.NoError(t, scene.Update(nil))
			require.False(t, scene.transientCommandInput.pending.active)
			for key := range held {
				require.True(t, scene.keyboard.IsSuppressedUntilRelease(key))
			}
			finishCombatSoloResetWait(t, scene)
			require.Equal(t, PrimaryInput, scene.secondaryKeyState)
			require.Zero(t, scene.dialogStack.Count())
			before := snapshotBlockedCombatState(t, scene.gameState)
			for range 3 {
				require.NoError(t, scene.Update(nil))
			}
			require.Equal(t, before, snapshotBlockedCombatState(t, scene.gameState), "held losing input cannot become a command or movement")
			require.False(t, scene.keyboard.TryToRegisterRepeatableKeyPress(direction))
			clear(held)
			require.NoError(t, scene.Update(nil))
			require.True(t, scene.gameState.SelectCombatPlayer(0))
			held[ebiten.Key0] = true
			require.NoError(t, scene.Update(nil))
			require.False(t, scene.gameState.CombatScene.IsSoloMoveActive(), "release/new press retains ready-turn 0")
			require.False(t, scene.transientCommandInput.pending.active, "reselecting cannot revive an old verb")
		})
	}
}

func TestCombatSoloResetAlreadyAllPartyIsSilentAndReleasesHold(t *testing.T) {
	scene, held, callbacks, _ := newCombatSoloResetUpdateScene(t, true)
	scene.gameState.CombatScene.ClearSoloMoveMode()
	before := snapshotBlockedCombatState(t, scene.gameState)
	active := scene.gameState.GetActivePlayerIndex()
	held[ebiten.Key0] = true
	require.NoError(t, scene.Update(nil))
	require.Equal(t, before, snapshotBlockedCombatState(t, scene.gameState))
	require.Equal(t, active, scene.gameState.GetActivePlayerIndex())
	require.Empty(t, callbacks.CurrentRowContent)
	require.Empty(t, callbacks.Messages)
	require.Empty(t, callbacks.CombatEvents)
	finishCombatSoloResetWait(t, scene)
	require.NotContains(t, callbacks.Messages, "Set active player: None!")
}
