package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestActionFeedbackHarvestHistoryKeepsRepeatedEventsAtEveryCap(t *testing.T) {
	for limit := range 4 {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.Britain).
				WithPlayerAt(15, 15).WithSystemCallbacks().Build()
			scene := newOwnedSurfaceCompletionTestScene(t, state)
			wireFeedbackOutputForTest(scene)
			setActionPopupLimitForTest(scene, limit)
			state.PartyState.Inventory.Provisions.Food.Set(100)
			state.PartyState.Karma.Value = 50
			for _, direction := range []references.Direction{references.Up, references.Right} {
				target := direction.GetNewPositionInDirection(&state.MapState.PlayerLocation.Position)
				state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, target, indexes.WheatInField)
				require.True(t, state.ActionGetSmallMap(direction))
				assert.Equal(t, indexes.SpriteIndex(indexes.PlowedField), state.GetCurrentLayeredMap().GetTileTopMapOnlyTile(target).Index)
			}
			history := scene.output.JournalHistory()
			assert.Equal(t, 2, strings.Count(history, "Crops picked!"))
			assert.Equal(t, 2, strings.Count(history, "Those crops are not yours, Avatar."))
			assert.EqualValues(t, 102, state.PartyState.Inventory.Provisions.Food.Get())
			assert.Equal(t, 48, state.PartyState.Karma.Value)
			assert.Len(t, scene.transientActionFeedback, min(limit, 2))
			assert.False(t, scene.dialogStack.HasModalDialog())
			scene.clearTransientActionFeedback()
			assert.Equal(t, history, scene.output.JournalHistory())
		})
	}
}

func TestActionFeedbackSurfaceSearchAndLookHistoryHaveOneOwnerAtEveryCap(t *testing.T) {
	for limit := range 4 {
		for _, location := range []references.Location{references.Britain, references.BritanniaUnderworld} {
			t.Run(fmt.Sprintf("cap-%d/location-%d", limit, location), func(t *testing.T) {
				state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(location).
					WithPlayerAt(15, 15).WithSystemCallbacks().Build()
				scene := newOwnedSurfaceCompletionTestScene(t, state)
				wireFeedbackOutputForTest(scene)
				setActionPopupLimitForTest(scene, limit)
				target := references.Up.GetNewPositionInDirection(&state.MapState.PlayerLocation.Position)
				state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, target, indexes.Grass)
				state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, target, indexes.Grass)
				state.CurrentNPCAIController.GetNpcs().ReplaceWith(nil)
				if location == references.Britain {
					require.False(t, state.ActionSearchSmallMap(references.Up))
					require.True(t, state.ActionLookSmallMap(references.Up))
				} else {
					require.False(t, state.ActionSearchLargeMap(references.Up))
					require.True(t, state.ActionLookLargeMap(references.Up))
				}
				history := scene.output.JournalHistory()
				assert.Equal(t, 1, strings.Count(history, game_state.MessageNothingOfNote))
				look := "Thou dost see " + state.GameReferences.LookReferences.GetTileLookDescription(indexes.Grass)
				assert.Equal(t, 1, strings.Count(history, look))
				assert.Contains(t, history, "Look-"+look, "the caller retains the complete prefixed narration")
				assert.Len(t, scene.transientActionFeedback, min(limit, 2))
				assert.False(t, scene.dialogStack.HasModalDialog())
				scene.clearTransientActionFeedback()
			})
		}
	}
}

func TestActionFeedbackCombatSearchHistoryHasOneOwnerAtEveryCap(t *testing.T) {
	for limit := range 4 {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			scene := newCombatReportTestScene(t)
			state := scene.gameState
			wireFeedbackOutputForTest(scene)
			setActionPopupLimitForTest(scene, limit)
			actor := state.CombatScene.GetActiveCombatantIndex()
			state.CombatScene.SetPartyMemberPosition(actor, references.Position{X: 5, Y: 9})
			position := references.Position{X: 5, Y: 10}
			require.True(t, state.CombatScene.AddObject(game_state.NewCombatCorpse(position)))
			state.SetRandomSeed(42)
			require.True(t, state.ActionSearchCombatMap(references.Down))
			assert.Equal(t, 1, strings.Count(scene.output.JournalHistory(), "Thou dost find nothing!"))
			assert.Len(t, scene.transientActionFeedback, min(limit, 1))
			scene.clearTransientActionFeedback()
		})
	}
}

func TestActionFeedbackDungeonSearchAndLookRetainCallerHistoryAtEveryCap(t *testing.T) {
	for limit := range 4 {
		for _, command := range []string{"search", "look", "dark search", "dark look", "invalid search"} {
			t.Run(fmt.Sprintf("%s/cap-%d", command, limit), func(t *testing.T) {
				scene, state, _ := newDungeonInputTestScene(t)
				retirePresentationFixtureCaches(t)
				t.Cleanup(scene.Dispose)
				wireFeedbackOutputForTest(scene)
				setActionPopupLimitForTest(scene, limit)
				state.MapState.Lighting.RestoreTorchMinutes(0)
				state.MagicLightMinutesRemaining = 0
				state.MapState.Lighting.SetMagicLight(false)
				var message string
				switch command {
				case "dark search":
					require.False(t, state.CanSearchDungeonMap())
					message = "You find darkness."
				case "dark look":
					require.False(t, state.CanLookDungeonMap())
					message = "You see darkness."
				default:
					state.MapState.Lighting.RestoreTorchMinutes(10)
					token, _ := state.BeginDungeonCommandCycle()
					switch command {
					case "search":
						require.False(t, state.ActionSearchDungeonMapWithCharacter(references.Down, 0))
						message = game_state.MessageNothingOfNote
						assert.Contains(t, scene.output.JournalHistory(), "Search-"+message)
					case "invalid search":
						require.False(t, state.ActionSearchDungeonMapWithCharacter(references.NoneDirection, 0))
						message = game_state.MessageCantSearchThere
						assert.Contains(t, scene.output.JournalHistory(), "Search-"+message)
					case "look":
						_, found := state.ActionLookDungeonMap(references.Up, 0)
						require.True(t, found)
						message = "You see a passage."
						assert.Contains(t, scene.output.JournalHistory(), "Look-"+message)
					}
					state.CompleteDungeonCommandCycle(token, game_state.DungeonFullEndPlayer)
				}
				assert.Equal(t, 1, strings.Count(scene.output.JournalHistory(), message))
				assert.Len(t, scene.transientActionFeedback, min(limit, 1))
				scene.clearTransientActionFeedback()
			})
		}
	}
}
