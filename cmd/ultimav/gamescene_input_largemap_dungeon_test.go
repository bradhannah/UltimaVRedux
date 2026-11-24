package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestLargeMapEnterRoutingCompletesOutdoorTurnExactlyOnce(t *testing.T) {
	for _, open := range []bool{false, true} {
		name := "rejection"
		if open {
			name = "success"
		}
		t.Run(name, func(t *testing.T) {
			state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
				WithLocation(references.BritanniaUnderworld).
				WithFloor(0).
				WithSystemCallbacks().
				Build()
			state.MapState.PlayerLocation.Position = state.GetDungeonEntranceCoordinates()[references.Deceit]
			state.PartyVehicle = map_units.NewNPCFriendlyVehiceNoVehicle()
			state.SetDungeonState(references.Deceit, open)
			state.PartyState.Inventory.QuestItems.Set(references.Sextant, 1)
			state.DateTime.Hour, state.DateTime.Minute = 5, 59
			entrance := state.MapState.PlayerLocation
			beforeEntryTime := state.DateTime
			callbacks.Reset()
			scene := &GameScene{
				gameState: state,
				keyboard:  input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
			}

			scene.largeMapInputHandler(ebiten.KeyE)

			if !open {
				assert.Equal(t, []int{game_state.DefaultLargeMapMinutesPerTurn}, callbacks.TimeAdvanced())
				assert.False(t, state.EventQueueManager.HasWorkflow("enter_location"))
				assert.Empty(t, state.CurrentQuestJournalDossier().Dungeons)

				return
			}

			require.True(t, state.EventQueueManager.HasWorkflow("enter_location"))
			assert.Equal(t, beforeEntryTime, state.DateTime)
			assert.Empty(t, callbacks.TimeAdvanced())
			assert.Empty(t, state.CurrentQuestJournalDossier().Dungeons, "accepted Enter has not transitioned yet")
			drainDungeonInputWorkflow(t, state, "enter_location")
			drainDungeonInputWorkflow(t, state, "surface_command_completion")
			assert.Equal(t, references.Deceit, state.MapState.PlayerLocation.Location)
			assert.Equal(t, beforeEntryTime, state.DateTime)
			assert.Empty(t, callbacks.TimeAdvanced())
			entries := state.CurrentQuestJournalDossier().Dungeons
			require.Len(t, entries, 1)
			survey := entries[0].Sites[0].Survey
			require.True(t, survey.Measured)
			assert.Equal(t, int(entrance.Position.X), survey.Location.X)
			assert.Equal(t, int(entrance.Position.Y), survey.Location.Y)
			assert.Equal(t, entrance.Floor, survey.Location.Floor)
			assert.Equal(t, 5, survey.RecordedAt.Time.Hour)
			assert.Equal(t, 59, survey.RecordedAt.Time.Minute)

			token, start := state.BeginDungeonCommandCycle()
			state.CompleteDungeonCommandCycle(token, game_state.DungeonCommandOnly)
			assert.Equal(t, 1, start.ElapsedMinutes,
				"the first accepted dungeon command must own the normal precharge")
		})
	}
}

func TestLargeMapEnterRoutingFinishesNonDungeonTurn(t *testing.T) {
	shrine := game_state.GetShrineData(0)
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithFloor(0).
		WithPlayerAt(int(shrine.WorldX), int(shrine.WorldY)).
		WithSystemCallbacks().
		Build()
	state.SetShrineRuined(0, true)
	beforeEnter := state.DateTime
	callbacks.Reset()
	scene := &GameScene{
		gameState: state,
		keyboard:  input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
	}

	scene.largeMapInputHandler(ebiten.KeyE)

	assert.NotEqual(t, beforeEnter, state.DateTime)
	assert.False(t, state.EventQueueManager.HasWorkflow("enter_location"))
	assert.Empty(t, state.CurrentQuestJournalDossier().Dungeons, "shrine Enter is not dungeon discovery")
}
