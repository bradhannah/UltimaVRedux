package main

import (
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMoonstoneJournalSceneMoongateUsesReviewedPhaseAndArrivalClock(t *testing.T) {
	for _, test := range []struct {
		name     string
		hour     uint8
		measured bool
	}{
		{"dawn", 5, false}, {"dusk", 18, true}, {"noon phase changes", 11, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			state, callbacks := game_state.NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).
				WithPlayerAt(187, 167).WithSystemCallbacks().Build()
			state.DateTime = datetime.UltimaDate{Year: 139, Month: 4, Day: 2, Hour: 4}
			state.PartyState.Inventory.QuestItems.Set(references.Sextant, 1)
			stabilizeMoongateStage(t, state)
			state.DateTime.Hour, state.DateTime.Minute = test.hour, 59
			route := state.ReviewMoongateTravel()
			require.True(t, route.Success)
			require.Equal(t, game_state.MoongateTravelToMoonstone, route.Route.Kind)
			phase := route.Route.Destination.Phase
			before := state.CurrentQuestJournalDossier().Moonstones
			assert.False(t, before[phase].Known)
			callbacks.Reset()
			state.AdvanceGameplayMinutes(1) // Existing movement cost, after route review.
			arrival := state.DateTime
			if test.hour == 11 {
				assert.NotEqual(t, phase, state.DateTime.GetMoongateMoonPhase(), "fixture crosses Trammel/Felucca choice")
			}
			scene := newProductionCastTestScene(state)
			scene.queueReviewedMoongateTravel(route)
			drainSceneWorkflow(t, scene, game_state.MoongateTeleportWorkflow, 1000)
			after := state.CurrentQuestJournalDossier().Moonstones
			for index, entry := range after {
				if index != int(phase) {
					assert.Equal(t, before[index], entry, "departure and colocated phases are not identified")
					continue
				}
				assert.True(t, entry.Known && entry.Placed)
				assert.Equal(t, journal.MoonstoneSourceGate, entry.FirstSource)
				assert.Equal(t, int(arrival.Hour), entry.ObservedAt.Time.Hour)
				assert.Equal(t, int(arrival.Minute), entry.ObservedAt.Time.Minute)
				assert.Equal(t, test.measured, entry.LastObservedSurvey.Measured)
			}
			assert.Equal(t, arrival, state.DateTime)
			assert.Equal(t, []int{1}, callbacks.TimeAdvanced())
		})
	}
}

func TestMoonstoneJournalSceneBurialCommitsBeforeFeedbackAndSurvivesDismissal(t *testing.T) {
	scene, callbacks := newSurfaceUseOwnershipSceneAt(t, references.BritanniaUnderworld, 0, references.Position{X: 225, Y: 133})
	state := scene.gameState
	state.DateTime = datetime.UltimaDate{Year: 139, Month: 4, Day: 12, Hour: 5, Minute: 59}
	state.PartyState.Inventory.Moonstones.Set(references.NewMoon, 1)
	state.PartyState.Inventory.QuestItems.Set(references.Sextant, 1)
	position := state.MapState.PlayerLocation.Position
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &position, indexes.Grass)
	callbacks.Reset()
	scene.StartMoonstoneBurialWorkflow(references.NewMoon)
	var overlay *widgets.OverlayDialog
	for range 20 {
		state.ElapsedMs += DialogPollingRateMs
		state.EventQueueManager.Update()
		if candidate, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.OverlayDialog); ok {
			overlay = candidate
			break
		}
	}
	require.NotNil(t, overlay)
	assert.Equal(t, "Moonstone buried!", overlay.GetMessage())
	entry := state.CurrentQuestJournalDossier().Moonstones[0]
	require.True(t, entry.Placed && entry.LastObservedSurvey.Measured)
	assert.False(t, entry.Carried)
	assert.Equal(t, journal.MoonstoneSourceBurial, entry.FirstSource)
	assert.Equal(t, 5, entry.ObservedAt.Time.Hour)
	assert.Equal(t, 59, entry.LastObservedSurvey.RecordedAt.Time.Minute)
	assert.Equal(t, byte(6), state.DateTime.Hour, "the existing minute follows commitment")
	assert.Equal(t, int(position.X), entry.LastObservedSurvey.Location.X)
	assert.Equal(t, uint16(0), state.PartyState.Inventory.Moonstones.Get(references.NewMoon))
	assert.Equal(t, []int{1}, callbacks.TimeAdvanced())
	overlay.Dismiss()
	drainNamedSurfaceUseWorkflow(t, scene, "moonstone_burial")
	assert.Equal(t, entry, state.CurrentQuestJournalDossier().Moonstones[0])
	assert.Equal(t, []int{1}, callbacks.TimeAdvanced())
	assert.Zero(t, scene.dialogStack.Count())
}
