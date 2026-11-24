package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	beradrng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

func TestBedRestRunsOneRegenerationRingSweepPerIteration(t *testing.T) {
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithPlayerAt(15, 15).
		WithSystemCallbacks().
		Build()
	state.DateTime.Hour = 1
	state.DateTime.Minute = 0
	state.PartyState.Inventory.Provisions.Food.Set(100)
	character := state.PartyState.GetAvatar()
	character.SetStatus(party_state.Good)
	character.SetEquippedItem(references.SlotRing, references.RingRegeneration)
	character.SetHp(character.GetMaxHp() - 10)
	const seed = uint64(44)
	state.SetRandomSeed(seed)
	state.CurrentNPCAIController = &bedRestPassController{}
	expected := beradrng.New(seed)
	wantHealing := uint16(0)
	for range 6 {
		if expected.Intn(8) == 7 {
			wantHealing++
		}
	}
	scene := &GameScene{gameState: state}
	workflow := state.EventQueueManager.Workflow("bed_rest_regeneration_test").
		Priority(event_queue.PriorityNormal).
		Sequential().
		First(func() {})
	var restState *game_state.BedRestState
	workflow.ThenSequence(scene.thenExecuteBedRestSequence(1, &restState)).Finally(func() {
		state.FinishBedRest(restState)
	}).Start()

	for update := 0; update < 100 && state.EventQueueManager.HasActiveWorkflows(); update++ {
		state.ElapsedMs += 1000
		state.EventQueueManager.Update()
	}

	require.False(t, state.EventQueueManager.HasActiveWorkflows())
	assert.Equal(t, character.GetMaxHp()-10+wantHealing, character.GetCurrentHp())
	assert.Equal(t, uint64(6), state.GetRNG().Calls())
	assert.Empty(t, callbacks.TransientIconCalls, "Regeneration Ring ticks are excluded from explicit healing feedback")
}

func TestBedRestWorkflowRunsSixteenPreSleepPassesAndAbortsOnlyForAttack(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		interaction map_units.PendingNPCInteraction
		wantStarted bool
	}{
		{name: "talk does not abort", interaction: map_units.PendingNPCInteractionTalk, wantStarted: true},
		{name: "attack on final pass aborts", interaction: map_units.PendingNPCInteractionHostileCombat, wantStarted: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
				WithLocation(references.Britain).
				WithPlayerAt(15, 15).
				WithSystemCallbacks().
				Build()
			state.DateTime.Hour, state.DateTime.Minute = 1, 0
			state.PartyState.Inventory.Provisions.Food.Set(100)
			npc := newBedRestTestNPC(1)
			controller := &bedRestPassController{
				units:       map_units.MapUnits{npc},
				interaction: testCase.interaction,
			}
			state.CurrentNPCAIController = controller
			scene := &GameScene{gameState: state}
			var restState *game_state.BedRestState
			cleanupCalls := 0
			workflow := state.EventQueueManager.Workflow("bed_rest_pre_sleep_test").
				Priority(event_queue.PriorityNormal).Sequential().BlockInput().First(func() {})
			workflow.ThenSequence(scene.thenExecuteBedRestSequence(1, &restState)).Finally(func() {
				state.FinishBedRest(restState)
				cleanupCalls++
			}).Start()

			for update := 0; update < 200 && state.EventQueueManager.HasActiveWorkflows(); update++ {
				state.ElapsedMs += 1000
				state.EventQueueManager.Update()
			}

			require.False(t, state.EventQueueManager.HasActiveWorkflows())
			assert.Equal(t, 16, controller.passes)
			assert.Equal(t, 1, cleanupCalls)
			assert.False(t, callbacks.InputBlocked)
			assert.Equal(t, 1, callbacks.BlockInputCalls)
			assert.Equal(t, 1, callbacks.UnblockInputCalls)
			assert.Equal(t, testCase.wantStarted, restState.CurrentCycle > 0)
			wantDisplayUpdates := 16
			if testCase.wantStarted {
				wantDisplayUpdates++ // Normal wake-up moves the Avatar out of bed.
			}
			assert.Equal(t, wantDisplayUpdates, callbacks.DisplayUpdates)
			if !testCase.wantStarted {
				assert.Equal(t, byte(1), state.DateTime.Hour)
				assert.Equal(t, byte(0), state.DateTime.Minute)
			} else {
				assert.Equal(t, map_units.PendingNPCInteractionNone, npc.MapUnitDetails().PendingInteraction)
			}
		})
	}
}

func TestBedRestPreSleepUsesFinalNPCCommand(t *testing.T) {
	for _, testCase := range []struct {
		name             string
		firstInteraction map_units.PendingNPCInteraction
		finalInteraction map_units.PendingNPCInteraction
		wantInterrupted  bool
	}{
		{
			name:             "later talk replaces attack",
			firstInteraction: map_units.PendingNPCInteractionHostileCombat,
			finalInteraction: map_units.PendingNPCInteractionTalk,
		},
		{
			name:             "later attack replaces talk",
			firstInteraction: map_units.PendingNPCInteractionTalk,
			finalInteraction: map_units.PendingNPCInteractionHostileCombat,
			wantInterrupted:  true,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			state, _ := gamefixture.NewIntegrationTestBuilder(t).
				WithLocation(references.Britain).
				WithPlayerAt(15, 15).
				WithSystemCallbacks().
				Build()
			first := newBedRestTestNPC(1)
			final := newBedRestTestNPC(2)
			controller := &bedRestFinalCommandController{
				units:            map_units.MapUnits{first, final},
				firstInteraction: testCase.firstInteraction,
				finalInteraction: testCase.finalInteraction,
			}
			state.CurrentNPCAIController = controller

			assert.Equal(t, testCase.wantInterrupted, state.ProcessBedRestPreSleepNPCPass())
			assert.Equal(t, 1, controller.passes)
			for _, unit := range controller.units {
				assert.Equal(t, map_units.PendingNPCInteractionNone, unit.MapUnitDetails().PendingInteraction)
			}
		})
	}
}

func TestBedRestPermanentTimeStopDoesNotDeadlockWorkflow(t *testing.T) {
	for _, turns := range []int{0, game_state.ToggleItemEffectDuration} {
		t.Run(fmt.Sprintf("duration_%d", turns), func(t *testing.T) {
			state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
				WithLocation(references.Britain).
				WithPlayerAt(15, 15).
				WithSystemCallbacks().
				Build()
			state.DateTime.Hour, state.DateTime.Minute = 1, 0
			state.ActiveDurationEffect = game_state.DurationEffect{
				EffectType:     game_state.DurationEffectTimeStop,
				TurnsRemaining: turns,
			}
			state.CurrentNPCAIController = &bedRestPassController{}
			scene := &GameScene{gameState: state}
			var restState *game_state.BedRestState
			workflow := state.EventQueueManager.Workflow("bed_rest_permanent_time_stop_test").
				Priority(event_queue.PriorityNormal).Sequential().First(func() {})
			workflow.ThenSequence(scene.thenExecuteBedRestSequence(1, &restState)).Finally(func() {
				state.FinishBedRest(restState)
			}).Start()

			for update := 0; update < 100 && state.EventQueueManager.HasActiveWorkflows(); update++ {
				state.ElapsedMs += 1000
				state.EventQueueManager.Update()
			}

			require.False(t, state.EventQueueManager.HasActiveWorkflows())
			require.NotNil(t, restState)
			assert.True(t, restState.TimeStopped)
			assert.Zero(t, restState.CurrentCycle)
			assert.Equal(t, byte(1), state.DateTime.Hour)
			assert.Equal(t, byte(0), state.DateTime.Minute)
			assert.Contains(t, callbacks.Messages, "Time is stopped!")
		})
	}
}

func TestBedRestPartyWipeDefersDeathUntilWorkflowCleanup(t *testing.T) {
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithPlayerAt(15, 15).
		WithSystemCallbacks().
		Build()
	for index := range state.PartyState.GetCharacterCount() {
		state.PartyState.GetCharacter(index).PartyMap = party_state.PartyMapUnrecruited
	}
	member := state.PartyState.GetCharacter(0)
	member.PartyMap = party_state.PartyMapActive
	member.SetStatus(party_state.Poisoned)
	member.SetHp(1)
	state.PartyState.Inventory.Provisions.Food.Set(100)
	state.DateTime.Hour, state.DateTime.Minute = 5, 40
	state.CurrentNPCAIController = &bedRestPassController{}
	scene := &GameScene{gameState: state}
	var restState *game_state.BedRestState
	deathCalls := 0
	state.SystemCallbacks.Flow.Consequences.TriggerDeathSequence = func() {
		deathCalls++
		assert.False(t, restState.Started)
	}
	workflow := state.EventQueueManager.Workflow("bed_rest_party_wipe_test").
		Priority(event_queue.PriorityNormal).Sequential().BlockInput().First(func() {})
	workflow.ThenSequence(scene.thenExecuteBedRestSequence(1, &restState)).Finally(func() {
		state.FinishBedRest(restState)
		state.SystemCallbacks.Visual.SetMapFadeLevel(0)
	}).Start()

	for update := 0; update < 200 && state.EventQueueManager.HasActiveWorkflows(); update++ {
		state.ElapsedMs += 1000
		state.EventQueueManager.Update()
	}

	require.False(t, state.EventQueueManager.HasActiveWorkflows())
	assert.Equal(t, 1, deathCalls)
	assert.True(t, state.PartyState.IsAllPartyDead())
	assert.Equal(t, byte(6), state.DateTime.Hour)
	require.NotEmpty(t, callbacks.MapFadeLevels)
	assert.Zero(t, callbacks.MapFadeLevels[len(callbacks.MapFadeLevels)-1])
	assert.False(t, callbacks.InputBlocked)
}

type bedRestPassController struct {
	units       map_units.MapUnits
	passes      int
	interaction map_units.PendingNPCInteraction
}

func (c *bedRestPassController) PopulateMapFirstLoad() {}

func (c *bedRestPassController) AdvanceNextTurnCalcAndMoveNPCs() {
	c.passes++
	for _, unit := range c.units {
		unit.MapUnitDetails().PendingInteraction = map_units.PendingNPCInteractionNone
	}
	if c.passes == 16 && len(c.units) > 0 {
		c.units[0].MapUnitDetails().PendingInteraction = c.interaction
	}
}

func (c *bedRestPassController) FreshenExistingNPCsOnMap()    {}
func (c *bedRestPassController) GetNpcs() *map_units.MapUnits { return &c.units }
func (c *bedRestPassController) RemoveAllEnemies()            {}
func (c *bedRestPassController) HardResetNPCs(_ bool)         {}

type bedRestFinalCommandController struct {
	units            map_units.MapUnits
	passes           int
	firstInteraction map_units.PendingNPCInteraction
	finalInteraction map_units.PendingNPCInteraction
}

func (c *bedRestFinalCommandController) PopulateMapFirstLoad() {}

func (c *bedRestFinalCommandController) AdvanceNextTurnCalcAndMoveNPCs() {
	c.passes++
	c.units[0].MapUnitDetails().PendingInteraction = c.firstInteraction
	c.units[1].MapUnitDetails().PendingInteraction = c.finalInteraction
}

func (c *bedRestFinalCommandController) FreshenExistingNPCsOnMap()    {}
func (c *bedRestFinalCommandController) GetNpcs() *map_units.MapUnits { return &c.units }
func (c *bedRestFinalCommandController) RemoveAllEnemies()            {}
func (c *bedRestFinalCommandController) HardResetNPCs(_ bool)         {}

func newBedRestTestNPC(npcNum int) *map_units.NPCFriendly {
	position := references.Position{X: 1, Y: 1}
	reference := references.NPCReference{
		Schedule: references.CreateNPCScheduledFixedOneLocation(position, 0),
	}
	reference.SetKeyIndex(indexes.BardKeyIndex)

	return map_units.NewNPCFriendly(reference, npcNum)
}
