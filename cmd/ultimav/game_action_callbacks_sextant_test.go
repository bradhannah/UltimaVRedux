package main

import (
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/conversation"
	"github.com/bradhannah/UltimaVRedux/internal/conversation/semantic"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	"github.com/stretchr/testify/require"
)

func TestProductionSemanticItem72GrantRecordsGreyhavenOnce(t *testing.T) {
	for _, location := range []references.Location{references.Greyhaven, references.Fogsbane} {
		state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).WithSystemCallbacks().Build()
		state.PartyState.Inventory.QuestItems.Set(references.Sextant, 0)
		state.GameConfig.GameplayConfiguration.FlexibleSextantUse = false
		state.DateTime.Hour = 12
		position, _ := state.GameReferences.LocationReferences.WorldLocations.GetWorldLocation(location)
		state.MapState.PlayerLocation.Position = position
		require.Equal(t, game_state.EnterLargeMapTransition, state.ResolveEnterLargeMap())
		for range 200 {
			state.EventQueueManager.Update()
			state.ElapsedMs += 1000
			if !state.EventQueueManager.HasWorkflow("enter_location") {
				break
			}
		}
		require.Equal(t, location, state.MapState.PlayerLocation.Location)
		var changes []journal.AtlasChange
		state.SystemCallbacks.UI.AtlasUpdated = func(change journal.AtlasChange) { changes = append(changes, change) }
		callbacks := NewGameActionCallbacks(&GameScene{gameState: state}, references.NPCReference{Location: location}, nil)
		dialogOutput := text.NewOutput(nil, 1, 64, 80)
		callbacks.SetDialogOutput(dialogOutput)
		definition := boundaryTalkDefinition(semantic.Body{&semantic.GiveItemAction{Item: "sextant", Quantity: 3}, &semantic.EndAction{}})
		item, valid := semantic.OriginalItemID("sextant")
		require.True(t, valid)
		require.EqualValues(t, 72, item)
		engine, err := conversation.NewSemanticConversationEngine(definition, callbacks, false)
		require.NoError(t, err)
		response := engine.Start(1)
		require.NoError(t, response.Error)
		require.EqualValues(t, 3, state.PartyState.Inventory.QuestItems.Get(references.Sextant))
		require.Contains(t, dialogOutput.GetOutputStr(false), "Received: Sextant")
		if location == references.Greyhaven {
			require.Len(t, changes, 1)
			require.Equal(t, "chart-16", changes[0].EntryKey)
			require.Equal(t, "Greyhaven", changes[0].Name)
			require.True(t, changes[0].Surveyed)
			var receipt journal.AtlasEntry
			for _, entry := range state.CurrentAtlasDossier().Entries {
				if entry.Key == "chart-16" {
					receipt = entry
				}
			}
			require.True(t, receipt.Survey.Measured)
			require.False(t, receipt.Survey.Flexible)
			require.Equal(t, string(journal.AtlasSourceSextantReceipt), receipt.SurveySource)
			require.Equal(t, journal.SiteLocation{Map: references.BritanniaUnderworld, X: 104, Y: 216}, receipt.Survey.Location)
		} else {
			require.Empty(t, changes)
			for _, entry := range state.CurrentAtlasDossier().Entries {
				require.False(t, entry.Survey.Measured)
			}
		}
	}
}
