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
	scene, held, update, _ := newCombatKeyRepeatSceneWith(t, rate, speed, nil)

	return scene, held, update
}

// newCombatKeyRepeatSceneWith lets a test arrange the battlefield and the keys
// already down before the first turn begins.
func newCombatKeyRepeatSceneWith(
	t *testing.T, rate int, speed float64, arrange func(*GameScene, map[ebiten.Key]bool),
) (*GameScene, map[ebiten.Key]bool, func(), *game_state.MockSystemCallbacks) {
	t.Helper()
	scene, held, update, callbacks := newRunicReferenceTestScene(t, references.Britain)
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
	if arrange != nil {
		arrange(scene, held)
	}
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

	return scene, held, update, callbacks
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

// combatKeyRepeatFight is an all-party fight with a troll beside every member.
// Everyone is healed each frame, so the fight lasts as long as a test needs.
type combatKeyRepeatFight struct {
	scene     *GameScene
	held      map[ebiten.Key]bool
	callbacks *game_state.MockSystemCallbacks
	step      func()
}

func newCombatKeyRepeatFight(t *testing.T, rate int, arrange func(*GameScene, map[ebiten.Key]bool)) *combatKeyRepeatFight {
	t.Helper()
	scene, held, update, callbacks := newCombatKeyRepeatSceneWith(t, rate, 1, func(scene *GameScene, held map[ebiten.Key]bool) {
		state := scene.gameState
		enemies := *state.GameReferences.EnemyReferences
		for index, active := range state.CombatScene.PartyActive {
			if active {
				// Slower members let the trolls take turns between the party's.
				state.PartyState.GetCharacter(index).Dexterity = 12
				state.CombatScene.AddMonsterToQueue(game_state.NewCombatMonster(references.MonsterTroll,
					&enemies[references.MonsterTroll], references.Position{X: 2, Y: references.Coordinate(1 + index)},
					state.AsRandomProvider()))
			}
		}
		if arrange != nil {
			arrange(scene, held)
		}
	})
	fight := &combatKeyRepeatFight{scene: scene, held: held, callbacks: callbacks, step: nil}
	fight.step = func() {
		update()
		state, combat := scene.gameState, scene.gameState.CombatScene
		for index, active := range combat.PartyActive {
			if active {
				member := state.PartyState.GetCharacter(index)
				member.SetHp(member.GetMaxHp())
			}
		}
		for index := range combat.MonsterManager.Count() {
			monster := combat.MonsterManager.GetMonster(index)
			monster.SetCurrentHP(monster.MaxHP())
		}
	}

	return fight
}

// turns counts the party turn prompts shown so far.
func (f *combatKeyRepeatFight) turns() int {
	count := 0
	for _, message := range f.callbacks.Messages {
		if strings.Contains(message, ", armed with") {
			count++
		}
	}

	return count
}

func (f *combatKeyRepeatFight) busy() bool {
	return f.scene.inputBlocked || f.scene.gameState.EventQueueManager.HasInputBlockingWorkflows()
}

func (f *combatKeyRepeatFight) ready() bool {
	return !f.busy() && f.scene.secondaryKeyState == PrimaryInput &&
		f.scene.gameState.CombatScene.GetCurrentTurnIdentity().IsPartyMember
}

func (f *combatKeyRepeatFight) until(t *testing.T, condition func() bool) {
	t.Helper()
	for range 2000 {
		if condition() {
			return
		}
		f.step()
	}
	t.Fatal("the fight never reached the awaited moment")
}

// tap presses a key for one frame. A longer arrow press would move an aim twice.
func (f *combatKeyRepeatFight) tap(key ebiten.Key) {
	f.held[key] = true
	f.step()
	delete(f.held, key)
	f.step()
	f.step()
}

// aim opens the attack aim by hand and puts it on the troll to the right.
func (f *combatKeyRepeatFight) aim(t *testing.T) {
	t.Helper()
	f.until(t, f.ready)
	f.tap(ebiten.KeyA)
	targeting := f.scene.gameState.CombatScene.AttackTargeting
	require.NotNil(t, targeting, "A opens the aim")
	if targeting.Cursor == targeting.Origin {
		f.tap(ebiten.KeyRight)
	}
}

// primed gives every member a remembered target by attacking once by hand.
func (f *combatKeyRepeatFight) primed(t *testing.T) *combatKeyRepeatFight {
	t.Helper()
	for range 18 {
		f.aim(t)
		f.tap(ebiten.KeyA)
	}
	f.until(t, f.ready)

	return f
}

// untilAnEnemyActs attacks by hand until an enemy's turn is playing with no key down.
func (f *combatKeyRepeatFight) untilAnEnemyActs(t *testing.T) {
	t.Helper()
	enemyActing := func() bool {
		return f.scene.gameState.EventQueueManager.HasWorkflow("monster_turn_delay")
	}
	for range 40 {
		if enemyActing() {
			return
		}
		f.aim(t)
		f.tap(ebiten.KeyA)
		f.until(t, func() bool { return enemyActing() || f.ready() })
	}
	t.Fatal("no enemy took a turn")
}

// hold keeps a key down for durationMs and returns the party turns that began.
func (f *combatKeyRepeatFight) hold(key ebiten.Key, durationMs int64) int {
	before := f.turns()
	f.held[key] = true
	for start := f.scene.gameState.ElapsedMs; f.scene.gameState.ElapsedMs-start < durationMs; {
		f.step()
	}

	return f.turns() - before
}

func TestCombatKeyRepeatPicksUpAKeyThatWentDownEarly(t *testing.T) {
	enemyTurn := func(t *testing.T, fight *combatKeyRepeatFight) { fight.untilAnEnemyActs(t) }
	ownAttack := func(t *testing.T, fight *combatKeyRepeatFight) {
		fight.aim(t)
		fight.held[ebiten.KeyEnter] = true
		fight.step()
		delete(fight.held, ebiten.KeyEnter)
		fight.step()
		require.True(t, fight.busy(), "the attack is still playing")
	}
	cases := []struct {
		name   string
		key    ebiten.Key
		arrive func(*testing.T, *combatKeyRepeatFight)
	}{
		{name: "A pressed while an enemy acts", key: ebiten.KeyA, arrive: enemyTurn},
		{name: "Tab pressed while an enemy acts", key: ebiten.KeyTab, arrive: enemyTurn},
		{name: "A pressed while the party's own attack plays", key: ebiten.KeyA, arrive: ownAttack},
		{name: "Tab pressed while the party's own attack plays", key: ebiten.KeyTab, arrive: ownAttack},
		{
			name: "A opens the aim, then a held A confirms it", key: ebiten.KeyA,
			arrive: func(t *testing.T, fight *combatKeyRepeatFight) { fight.aim(t) },
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fight := newCombatKeyRepeatFight(t, 10, nil).primed(t)
			test.arrive(t, fight)
			assert.Greater(t, fight.hold(test.key, 4000), 3, "the held key keeps the party acting")
			assert.True(t, fight.scene.transientCommandInput.combatRepeat.active)
		})
	}
}

// These holds must keep waiting for release, with or without Combat Key Repeat.
func TestCombatKeyRepeatLeavesOtherHeldKeysWaitingForRelease(t *testing.T) {
	cases := map[string]struct {
		rate    int
		before  func(*GameScene, map[ebiten.Key]bool)
		arrive  func(t *testing.T, fight *combatKeyRepeatFight)
		holding ebiten.Key
		steps   int // party steps the hold may cause without any repeat
	}{
		"Off: a key pressed while an enemy acts": {
			rate: 0, before: nil, holding: ebiten.KeyA, steps: 0,
			arrive: func(t *testing.T, fight *combatKeyRepeatFight) { fight.primed(t).untilAnEnemyActs(t) },
		},
		"Measured never repeats A, early or not": {
			rate: config.CombatKeyRepeatMeasured, before: nil, holding: ebiten.KeyA, steps: 0,
			arrive: func(t *testing.T, fight *combatKeyRepeatFight) { fight.primed(t).untilAnEnemyActs(t) },
		},
		"an arrow held from moving the aim, still down after the shot": {
			rate: 10, before: nil, holding: ebiten.KeyLeft, steps: 0,
			arrive: func(t *testing.T, fight *combatKeyRepeatFight) {
				fight.primed(t).aim(t)
				fight.held[ebiten.KeyLeft] = true
				fight.step()
				fight.held[ebiten.KeyEnter] = true
				fight.step()
				delete(fight.held, ebiten.KeyEnter)
			},
		},
		// Its one ordinary step is today's behaviour; it must never become a repeat.
		"a key down since before the fight began": {
			rate: 10, holding: ebiten.KeyLeft, steps: 1,
			before: func(scene *GameScene, held map[ebiten.Key]bool) {
				held[ebiten.KeyLeft] = true
				scene.keyboard.CaptureKeySample()
			},
			arrive: func(*testing.T, *combatKeyRepeatFight) {},
		},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			fight := newCombatKeyRepeatFight(t, test.rate, test.before)
			test.arrive(t, fight)
			positions, turns := fight.scene.gameState.CombatScene.PartyPositions, fight.turns()
			fight.held[test.holding] = true
			for start := fight.scene.gameState.ElapsedMs; fight.scene.gameState.ElapsedMs-start < 3000; {
				fight.step()
				require.False(t, fight.scene.transientCommandInput.combatRepeat.active, "the held key is never taken up")
			}
			moved := 0
			for index, position := range fight.scene.gameState.CombatScene.PartyPositions {
				if position != positions[index] {
					moved++
				}
			}
			assert.LessOrEqual(t, moved, test.steps, "the hold moves nobody further")
			assert.LessOrEqual(t, fight.turns()-turns, 1, "no further party turn begins while the key is down")
		})
	}
}
