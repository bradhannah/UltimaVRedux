package main

import (
	"fmt"
	"strings"
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
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestTelescopeDaytimeUsesSourceBoundaries(t *testing.T) {
	for hour := range uint8(24) {
		assert.Equal(t, hour >= 6 && hour <= 17, isTelescopeDaytime(hour), "hour %d", hour)
	}
}

func TestAstronomyFeedbackHistoryExactlyOnceAtEveryPopupCap(t *testing.T) {
	cases := []struct {
		name     string
		hour     uint8
		spyglass bool
		owned    uint16
		floor    references.FloorNumber
		message  string
		modal    bool
	}{
		{name: "telescope sun", hour: 6, message: "Thou dost see the sun!"},
		{name: "telescope sky", hour: 18, message: "Thou dost see the night sky!", modal: true},
		{name: "spyglass sky", hour: 19, spyglass: true, owned: 1, message: "Thou dost see the night sky!", modal: true},
		{name: "spyglass day", hour: 18, spyglass: true, owned: 1, message: "No stars!"},
		{name: "spyglass missing", hour: 19, spyglass: true, message: "No spyglass!"},
		{name: "spyglass basement", hour: 19, spyglass: true, owned: 1, floor: -1, message: "Not here!"},
	}
	for limit := range 4 {
		for _, test := range cases {
			t.Run(fmt.Sprintf("%s/cap-%d", test.name, limit), func(t *testing.T) {
				state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.Britain).
					WithFloor(test.floor).WithRandomSeed(443).WithSystemCallbacks().Build()
				state.DateTime.Hour = test.hour
				state.PartyState.Inventory.QuestItems.Set(references.Spyglass, test.owned)
				scene := newSurfaceCompletionTestScene(state)
				held := make(map[ebiten.Key]bool)
				scene.keyboard = input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool { return held[key] })
				wireFeedbackOutputForTest(scene)
				setActionPopupLimitForTest(scene, limit)
				workflow := "telescope_workflow"
				completed := 0
				if test.spyglass {
					workflow = "spyglass_workflow"
					scene.ShowSpyglassInteraction(func() { completed++ })
				} else {
					scene.ShowTelescopeInteraction()
				}
				if test.modal {
					dialog := waitForTopModal[*widgets.AstronomyDialog](t, scene)
					assert.Len(t, scene.transientActionFeedback, min(limit, 1))
					assert.True(t, state.EventQueueManager.HasInputBlockingWorkflows(), "astronomy still owns acknowledgement at Off")
					dialog.Update()
					dialog.Update()
					held[ebiten.KeyEnter] = true
					dialog.Update()
					require.True(t, dialog.IsDismissed())
					clear(held)
				}
				drainNamedSurfaceUseWorkflow(t, scene, workflow)
				assert.Equal(t, 1, strings.Count(scene.output.JournalHistory(), test.message))
				assert.False(t, state.EventQueueManager.HasInputBlockingWorkflows())
				assert.False(t, scene.dialogStack.HasModalDialog())
				assert.Len(t, scene.transientActionFeedback, min(limit, 1))
				if test.spyglass {
					assert.Equal(t, 1, completed)
				}
				scene.clearTransientActionFeedback()
				state.EventQueueManager.Shutdown()
				assert.Zero(t, scene.dialogStack.Count())
				assert.Equal(t, 1, strings.Count(scene.output.JournalHistory(), test.message))
			})
		}
	}
}

func TestFindingsTelescopeFallbackSkipsSleepingAndCharmedMembers(t *testing.T) {
	for _, status := range []party_state.CharacterStatus{party_state.Sleep, party_state.Charmed} {
		state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.Britain).
			WithRandomSeed(443).WithSystemCallbacks().Build()
		state.DateTime.Hour = 6
		state.PartyState.GetCharacter(0).SetStatus(status)
		state.PartyState.GetCharacter(1).SetStatus(party_state.Good)
		hp0 := state.PartyState.GetCharacter(0).GetCurrentHp()
		hp1 := state.PartyState.GetCharacter(1).GetCurrentHp()
		scene := newSurfaceCompletionTestScene(state)
		scene.ShowTelescopeInteraction()
		drainNamedSurfaceUseWorkflow(t, scene, "telescope_workflow")
		assert.Equal(t, hp0, state.PartyState.GetCharacter(0).GetCurrentHp())
		assert.Equal(t, hp1-1, state.PartyState.GetCharacter(1).GetCurrentHp())
	}
}

func TestTownTelescopeLookCompletesCommandExactlyOnce(t *testing.T) {
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithPlayerAt(15, 15).
		WithRandomSeed(443).
		WithSystemCallbacks().
		Build()
	state.DateTime.Hour = 18
	scene := newSurfaceCompletionTestScene(state)
	state.SystemCallbacks.Flow.Observation.ShowTelescopeInteraction = scene.ShowTelescopeInteraction
	target := references.Right.GetNewPositionInDirection(&state.MapState.PlayerLocation.Position)
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, target, indexes.Telescope)
	callbacks.Reset()
	scene.secondaryKeyState = LookDirectionInput
	scene.keyboard.SimulateKeyPress(ebiten.KeyRight)

	scene.smallMapHandleSecondaryInput()
	waitForTopModal[*widgets.AstronomyDialog](t, scene)

	assert.Empty(t, callbacks.TimeAdvanced())
	require.NotPanics(t, state.EventQueueManager.Shutdown)
	assert.Equal(t, []int{game_state.DefaultSmallMapMinutesPerTurn}, callbacks.TimeAdvanced())
	state.EventQueueManager.Shutdown()
	assert.Len(t, callbacks.TimeAdvanced(), 1)
}

func TestTelescopeSunDamageTargetsActivePlayerOrLivingFallback(t *testing.T) {
	tests := []struct {
		name         string
		activePlayer int
		killAvatar   bool
		wantTarget   int
	}{
		{
			name: "active player", activePlayer: 1, wantTarget: 1,
		},
		{
			name: "first living fallback", activePlayer: -1, killAvatar: true, wantTarget: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, _ := gamefixture.NewIntegrationTestBuilder(t).
				WithLocation(references.Britain).
				WithRandomSeed(443).
				WithSystemCallbacks().
				Build()
			state.DateTime.Hour = 6
			if test.killAvatar {
				state.PartyState.GetCharacter(0).SetStatus(party_state.Dead)
			}
			if test.activePlayer >= 0 {
				require.True(t, state.SetActivePlayer(test.activePlayer))
			}
			hpBefore := make([]uint16, 2)
			for index := range hpBefore {
				hpBefore[index] = state.PartyState.GetCharacter(index).GetCurrentHp()
			}
			scene := newSurfaceCompletionTestScene(state)

			scene.StartTelescopeWorkflow()
			drainNamedSurfaceUseWorkflow(t, scene, "telescope_workflow")

			for index := range hpBefore {
				want := hpBefore[index]
				if index == test.wantTarget {
					want--
				}
				assert.Equal(t, want, state.PartyState.GetCharacter(index).GetCurrentHp(), "party slot %d", index)
			}
		})
	}
}

func TestSurfaceKeyURoutesSpyglassThroughCallbackAndCompletesOnce(t *testing.T) {
	scene, callbacks := newSurfaceUseOwnershipSceneAt(
		t,
		references.BritanniaUnderworld,
		references.FloorNumber(references.OVERWORLD),
		references.Position{X: 0x4e, Y: 0xab},
	)
	state := scene.gameState
	state.PartyState.Inventory.QuestItems.Set(references.Spyglass, 1)
	state.DateTime.Hour = 19
	expectedTime := state.DateTime
	expectedTime.Advance(game_state.DefaultLargeMapMinutesPerTurn)
	callbackRequests := 0
	state.SystemCallbacks.Flow.Observation.ShowSpyglassInteraction = func(complete func()) {
		callbackRequests++
		scene.ShowSpyglassInteraction(complete)
	}
	var feedback []string
	state.SystemCallbacks.UI.ShowActionFeedback = func(message string, _ bool) {
		feedback = append(feedback, message)
	}
	callbacks.Reset()

	scene.largeMapInputHandler(ebiten.KeyU)
	itemDialog := waitForTopModal[*widgets.ItemSelectionDialog](t, scene)
	itemDialog.HandleEnter()
	drainUseSelectionWorkflow(t, scene)
	astronomyDialog := waitForTopModal[*widgets.AstronomyDialog](t, scene)

	assert.Equal(t, 1, callbackRequests)
	assert.Contains(t, feedback, "Thou dost see the night sky!")
	assert.Empty(t, callbacks.TimeAdvanced())
	assert.True(t, callbacks.InputBlocked)
	assert.Equal(t, uint16(1), state.PartyState.Inventory.QuestItems.Get(references.Spyglass))

	require.NotNil(t, astronomyDialog)
	require.NotPanics(t, state.EventQueueManager.Shutdown)

	assert.Equal(t, []int{game_state.DefaultLargeMapMinutesPerTurn}, callbacks.TimeAdvanced())
	assert.Equal(t, expectedTime, state.DateTime)
	assert.False(t, callbacks.InputBlocked)
	assert.Equal(t, callbacks.BlockInputCalls, callbacks.UnblockInputCalls)
	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.Zero(t, scene.dialogStack.Count())

	require.NotPanics(t, state.EventQueueManager.Shutdown)
	assert.Len(t, callbacks.TimeAdvanced(), 1)
}

func TestSurfaceKeyURoutesSextantOutputAndCompletesOnce(t *testing.T) {
	scene, callbacks := newSurfaceUseOwnershipSceneAt(
		t,
		references.BritanniaUnderworld,
		references.FloorNumber(references.OVERWORLD),
		references.Position{X: 0x4e, Y: 0xab},
	)
	state := scene.gameState
	state.PartyState.Inventory.QuestItems.Set(references.Sextant, 1)
	state.DateTime.Hour = 19
	expectedTime := state.DateTime
	expectedTime.Advance(game_state.DefaultLargeMapMinutesPerTurn)
	callbacks.Reset()

	scene.largeMapInputHandler(ebiten.KeyU)
	itemDialog := waitForTopModal[*widgets.ItemSelectionDialog](t, scene)
	itemDialog.HandleEnter()
	drainUseSelectionWorkflow(t, scene)

	assert.Contains(t, callbacks.Messages, "Position:")
	assert.Contains(t, callbacks.Messages, "K'L\", E'O\"")
	assert.Equal(t, uint16(1), state.PartyState.Inventory.QuestItems.Get(references.Sextant))
	assert.Equal(t, []int{game_state.DefaultLargeMapMinutesPerTurn}, callbacks.TimeAdvanced())
	assert.Equal(t, expectedTime, state.DateTime)
	assert.False(t, callbacks.InputBlocked)
	assert.Equal(t, callbacks.BlockInputCalls, callbacks.UnblockInputCalls)
	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.Zero(t, scene.dialogStack.Count())

	for range 3 {
		state.EventQueueManager.Update()
	}
	assert.Len(t, callbacks.TimeAdvanced(), 1)
}

func TestSpyglassAcceptsBritanniaAtSourceNightBoundaries(t *testing.T) {
	for _, hour := range []uint8{0, 5, 19, 23} {
		t.Run(fmt.Sprintf("hour %d", hour), func(t *testing.T) {
			state, _ := gamefixture.NewIntegrationTestBuilder(t).
				WithLocation(references.BritanniaUnderworld).
				WithFloor(references.FloorNumber(references.OVERWORLD)).
				WithSystemCallbacks().
				Build()
			state.PartyState.Inventory.QuestItems.Set(references.Spyglass, 1)
			state.DateTime.Hour = hour
			var feedback []string
			state.SystemCallbacks.UI.ShowActionFeedback = func(message string, _ bool) {
				feedback = append(feedback, message)
			}
			scene := newSurfaceCompletionTestScene(state)
			completed := 0

			scene.startSpyglassWorkflow(func() { completed++ })
			waitForTopModal[*widgets.AstronomyDialog](t, scene)
			state.EventQueueManager.Shutdown()

			assert.Equal(t, 1, completed)
			assert.Contains(t, feedback, "Thou dost see the night sky!")
			assert.Zero(t, scene.dialogStack.Count())
		})
	}
}

func TestSpyglassUsesSourceDayAndLocationFeedback(t *testing.T) {
	t.Run("18 has no stars", func(t *testing.T) {
		state, _ := gamefixture.NewIntegrationTestBuilder(t).
			WithLocation(references.BritanniaUnderworld).
			WithFloor(references.FloorNumber(references.OVERWORLD)).
			WithSystemCallbacks().
			Build()
		state.PartyState.Inventory.QuestItems.Set(references.Spyglass, 1)
		state.DateTime.Hour = 18
		var feedback []string
		state.SystemCallbacks.UI.ShowActionFeedback = func(message string, _ bool) {
			feedback = append(feedback, message)
		}
		scene := newSurfaceCompletionTestScene(state)
		completed := 0

		scene.startSpyglassWorkflow(func() { completed++ })
		drainNamedSurfaceUseWorkflow(t, scene, "spyglass_workflow")

		assert.Equal(t, 1, completed)
		assert.Contains(t, feedback, "No stars!")
	})

	t.Run("basement rejected", func(t *testing.T) {
		state, _ := gamefixture.NewIntegrationTestBuilder(t).
			WithLocation(references.Britain).
			WithFloor(-1).
			WithSystemCallbacks().
			Build()
		state.PartyState.Inventory.QuestItems.Set(references.Spyglass, 1)
		var feedback []string
		state.SystemCallbacks.UI.ShowActionFeedback = func(message string, _ bool) {
			feedback = append(feedback, message)
		}
		scene := newSurfaceCompletionTestScene(state)
		completed := 0

		scene.startSpyglassWorkflow(func() { completed++ })

		assert.Equal(t, 1, completed)
		assert.Contains(t, feedback, "Not here!")
		assert.False(t, state.EventQueueManager.HasWorkflow("spyglass_workflow"))
	})
}
