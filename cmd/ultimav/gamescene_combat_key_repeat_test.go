package main

import (
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/text"
)

// newCombatKeyRepeatScene starts an all-party fight on open floor: every party
// member stands at the west edge of its own row, and a rat waits far away.
func newCombatKeyRepeatScene(t *testing.T, rate int, speed float64) (*GameScene, map[ebiten.Key]bool, func()) {
	t.Helper()
	scene, held, update, _ := newRunicReferenceTestScene(t, references.Britain)
	state := scene.gameState
	require.NoError(t, state.TransitionToCombatMap(0, references.BritanniaCombatTerritory, game_state.CombatEntryNorth))
	combat := state.CombatScene
	for x := range references.XCombatMapTiles {
		for y := range references.YCombatMapTiles {
			setProductionCombatPushTerrain(state, references.Position{
				X: references.Coordinate(x), Y: references.Coordinate(y),
			}, indexes.BrickFloor)
		}
	}
	for index, active := range combat.PartyActive {
		if active {
			state.PartyState.GetCharacter(index).Dexterity = 35
			combat.SetPartyMemberPosition(index, references.Position{X: 1, Y: references.Coordinate(1 + index)})
		}
	}
	enemies := *state.GameReferences.EnemyReferences
	combat.AddMonsterToQueue(game_state.NewCombatMonster(references.MonsterRat, &enemies[references.MonsterRat],
		references.Position{X: 10, Y: 10}, state.AsRandomProvider()))
	state.BeginCombatTurns(game_state.CombatTurnsPresented)
	setCombatTimingTestRate(scene, speed)
	scene.gameConfig.GameplayConfiguration.CombatKeyRepeat = rate
	// Production workflows wait for every held key through the scene's own check.
	state.SystemCallbacks.Visual.IsAnyKeyPressed = scene.IsAnyKeyPressed
	scene.keyboard.SetElapsedMillisecondsSource(scene.getElapsedMs)
	scene.output = text.NewOutput(nil, 1, 64, 128)
	t.Cleanup(func() {
		clear(held)
		scene.ResetTransientCommandInput()
		state.EventQueueManager.Shutdown()
	})

	return scene, held, update
}

type combatPartyMove struct {
	atMs  int64
	actor int
}

// holdCombatKey holds a key for durationMs of 16 ms frames and records each
// party move with the time it happened.
func holdCombatKey(scene *GameScene, held map[ebiten.Key]bool, update func(), key ebiten.Key, durationMs int64) []combatPartyMove {
	combat := scene.gameState.CombatScene
	positions := combat.PartyPositions
	moves := []combatPartyMove{}
	held[key] = true
	for start := scene.gameState.ElapsedMs; scene.gameState.ElapsedMs-start < durationMs; {
		update()
		for index, position := range combat.PartyPositions {
			if position != positions[index] {
				moves = append(moves, combatPartyMove{atMs: scene.gameState.ElapsedMs, actor: index})
			}
		}
		positions = combat.PartyPositions
	}

	return moves
}

func TestCombatKeyRepeatMovesEachPartyMemberAtTheCappedPace(t *testing.T) {
	scene, held, update := newCombatKeyRepeatScene(t, config.CombatKeyRepeatMeasured, config.CombatSpeedLudicrous)
	moves := holdCombatKey(scene, held, update, ebiten.KeyRight, 2000)
	require.Greater(t, len(moves), 6, "a held arrow keeps the party moving")
	actors := map[int]bool{}
	for index, move := range moves {
		actors[move.actor] = true
		if index == 0 {
			continue
		}
		gap := move.atMs - moves[index-1].atMs
		if index == 1 {
			assert.GreaterOrEqual(t, gap, passInitialDelayMs, "the first repeat waits for a held key")
		}
		assert.GreaterOrEqual(t, gap, int64(keyPressDelay), "move %d: the rate caps every party member together", index)
		assert.Less(t, gap, passInitialDelayMs+DialogPollingRateMs, "move %d: the repeat does not stall", index)
	}
	assert.Greater(t, len(actors), 1, "each party member steps when its turn comes")

	delete(held, ebiten.KeyRight)
	assert.Empty(t, holdCombatKey(scene, held, update, ebiten.KeyF1, 1000), "release stops the repeat")

	off, offHeld, offUpdate := newCombatKeyRepeatScene(t, 0, config.CombatSpeedLudicrous)
	assert.Len(t, holdCombatKey(off, offHeld, offUpdate, ebiten.KeyRight, 2000), 1, "Off: one step per press")
}

func TestCombatKeyRepeatStopsAtBlockedUntilRelease(t *testing.T) {
	scene, held, update := newCombatKeyRepeatScene(t, config.CombatKeyRepeatMeasured, 1)
	combat := scene.gameState.CombatScene
	actor := combat.GetCurrentTurnIdentity()
	setProductionCombatPushTerrain(scene.gameState, combat.GetCombatantPosition(actor).Right(), indexes.StoneBrickWall)
	blocked := func() int { return strings.Count(scene.output.GetOutputStr(true), "Blocked!") }

	assert.Empty(t, holdCombatKey(scene, held, update, ebiten.KeyRight, 1000))
	assert.Equal(t, 1, blocked(), "Blocked! prints once, then the held key stops")
	assert.Equal(t, actor, combat.GetCurrentTurnIdentity(), "a blocked step costs no turn")
	assert.True(t, scene.keyboard.IsSuppressedUntilRelease(ebiten.KeyRight))

	delete(held, ebiten.KeyRight)
	update()
	assert.Empty(t, holdCombatKey(scene, held, update, ebiten.KeyRight, 100))
	assert.Equal(t, 2, blocked(), "a fresh press tries again")
}

func TestCombatKeyRepeatKeysAndStops(t *testing.T) {
	assert.True(t, combatRepeatableKey(ebiten.KeyNumpad6, config.CombatKeyRepeatMeasured))
	assert.True(t, combatRepeatableKey(ebiten.KeyTab, config.CombatKeyRepeatMeasured))
	assert.False(t, combatRepeatableKey(ebiten.KeyA, config.CombatKeyRepeatMeasured), "Measured never repeats A")
	assert.True(t, combatRepeatableKey(ebiten.KeyA, 10), "Fast repeats A")
	assert.False(t, combatRepeatableKey(ebiten.KeySpace, 30), "Pass never repeats in combat")
	assert.False(t, combatRepeatableKey(ebiten.KeyRight, 0))
	assert.Equal(t, int64(keyPressDelay), combatKeyRepeatIntervalMs(config.CombatKeyRepeatMeasured))
	assert.Equal(t, int64(33), combatKeyRepeatIntervalMs(30))

	scene, held, update := newCombatKeyRepeatScene(t, 30, 1)
	held[ebiten.KeyRight] = true
	scene.transientCommandInput.combatRepeat = combatKeyRepeat{
		active: true, key: ebiten.KeyRight, encounter: &game_state.CombatState{},
	}
	update()
	assert.False(t, scene.transientCommandInput.combatRepeat.active, "a repeat never outlives its encounter")
	assert.True(t, scene.keyboard.IsSuppressedUntilRelease(ebiten.KeyRight), "the key cannot act again after combat")

	pressed, pressedHeld, pressedUpdate := newCombatKeyRepeatScene(t, config.CombatKeyRepeatMeasured, 1)
	require.Len(t, holdCombatKey(pressed, pressedHeld, pressedUpdate, ebiten.KeyRight, 50), 1)
	require.True(t, pressed.transientCommandInput.combatRepeat.active)
	pressedHeld[ebiten.KeyEscape] = true
	pressedUpdate()
	assert.False(t, pressed.transientCommandInput.combatRepeat.active, "another fresh key stops the repeat")
	assert.True(t, pressed.keyboard.IsSuppressedUntilRelease(ebiten.KeyRight))

	held[ebiten.KeyA] = true
	scene.transientCommandInput.combatRepeat = combatKeyRepeat{
		active: true, key: ebiten.KeyA, encounter: scene.gameState.CombatScene,
	}
	origin := references.Position{X: 1, Y: 1}
	assert.False(t, scene.repeatCombatAttackConfirmation(&game_state.CombatAttackTargeting{Origin: origin, Cursor: origin}))
	assert.False(t, scene.transientCommandInput.combatRepeat.active, "an aim with no target stops a held A")
	assert.True(t, scene.keyboard.IsSuppressedUntilRelease(ebiten.KeyA))
}
