package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/ai"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

func TestJimmyPrisonerFirstSuccessAppliesOneTimeRewardAndOneCommandTurn(t *testing.T) {
	fixture := newJimmyPrisonerFixture(t)
	fixture.prisoner.MapUnitDetails().SetCurrentPath([]references.Position{{X: 7, Y: 7}})
	fixture.prisoner.MapUnitDetails().PendingInteraction = map_units.PendingNPCInteractionTalk
	fixture.state.ActiveDurationEffect = DurationEffect{EffectType: DurationEffectProtection, TurnsRemaining: 2}

	result := fixture.state.ActionJimmySmallMap(references.Up)

	require.True(t, result)
	assert.Equal(t, uint16(3), fixture.state.PartyState.Inventory.Provisions.Keys.Get(),
		"successful physical Jimmy retains the key")
	assert.Equal(t, 52, fixture.state.PartyState.Karma.Value)
	assert.Equal(t, byte(references.NoResponse), fixture.prisoner.NPCReference.DialogNumber)
	assert.Equal(t, references.AiType(references.FollowAroundAndBeAnnoyingThenNeverSeeAgain),
		fixture.prisoner.MapUnitDetails().GetOverriddenAiType())
	assert.Empty(t, fixture.prisoner.MapUnitDetails().CurrentPath)
	assert.Equal(t, map_units.PendingNPCInteractionNone, fixture.prisoner.MapUnitDetails().PendingInteraction)
	assert.True(t, fixture.state.PartyState.IsNPCDead(references.Britain, fixture.prisonerSlot))
	assert.True(t, mapUnitsContain(fixture.controller.GetNpcs(), fixture.prisoner),
		"the freed prisoner follows until the map is rebuilt")
	assert.Equal(t, []string{"\"I thank thee!\""}, fixture.callbacks.Messages)
	assert.Equal(t, byte(1), fixture.state.DateTime.Minute)
	assert.Equal(t, 1, fixture.state.ActiveDurationEffect.TurnsRemaining,
		"Jimmy must process the complete town turn, not only advance the clock")
	assert.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestJimmyPrisonerRewardIsCappedAndNeverRepeated(t *testing.T) {
	fixture := newJimmyPrisonerFixture(t)
	fixture.state.PartyState.Karma.Value = 98
	dexChecks := 0
	fixture.state.jimmyPrisonerSuccessForTesting = func(*party_state.PlayerCharacter) bool {
		dexChecks++

		return true
	}

	require.True(t, fixture.state.ActionJimmySmallMap(references.Up))
	fixture.callbacks.Messages = nil
	require.True(t, fixture.state.ActionJimmySmallMap(references.Up))

	assert.Equal(t, 99, fixture.state.PartyState.Karma.Value)
	assert.Equal(t, uint16(3), fixture.state.PartyState.Inventory.Provisions.Keys.Get())
	assert.Empty(t, fixture.callbacks.Messages)
	assert.Equal(t, 2, dexChecks,
		"a repeated loaded attempt still performs the original physical unlock check")
	assert.Equal(t, byte(2), fixture.state.DateTime.Minute)
	assert.Equal(t, []int{1, 1}, fixture.callbacks.TimeAdvanced())
	assert.Equal(t, byte(references.NoResponse), fixture.prisoner.NPCReference.DialogNumber)
}

func TestJimmyPrisonerFailureConsumesOneKeyWithoutMutation(t *testing.T) {
	fixture := newJimmyPrisonerFixture(t)
	fixture.state.jimmyPrisonerSuccessForTesting = func(*party_state.PlayerCharacter) bool { return false }

	result := fixture.state.ActionJimmySmallMap(references.Up)

	assert.False(t, result)
	assert.Equal(t, uint16(2), fixture.state.PartyState.Inventory.Provisions.Keys.Get())
	assert.Equal(t, 50, fixture.state.PartyState.Karma.Value)
	assert.Equal(t, byte(7), fixture.prisoner.NPCReference.DialogNumber)
	assert.Equal(t, references.AiType(references.Unset),
		fixture.prisoner.MapUnitDetails().GetOverriddenAiType())
	assert.False(t, fixture.state.PartyState.IsNPCDead(references.Britain, fixture.prisonerSlot))
	assert.Equal(t, []string{"Key broke!"}, fixture.callbacks.Messages)
	assert.Equal(t, byte(1), fixture.state.DateTime.Minute)
	assert.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestJimmyPrisonerRequiresKeyBeforeDexterityCheck(t *testing.T) {
	fixture := newJimmyPrisonerFixture(t)
	fixture.state.PartyState.Inventory.Provisions.Keys.Set(0)
	dexChecks := 0
	fixture.state.jimmyPrisonerSuccessForTesting = func(*party_state.PlayerCharacter) bool {
		dexChecks++

		return true
	}

	result := fixture.state.ActionJimmySmallMap(references.Up)

	assert.False(t, result)
	assert.Zero(t, dexChecks)
	assert.Equal(t, 50, fixture.state.PartyState.Karma.Value)
	assert.Equal(t, byte(7), fixture.prisoner.NPCReference.DialogNumber)
	assert.False(t, fixture.state.PartyState.IsNPCDead(references.Britain, fixture.prisonerSlot))
	assert.Equal(t, []string{"No Keys!"}, fixture.callbacks.Messages)
	assert.Equal(t, byte(1), fixture.state.DateTime.Minute)
	assert.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestJimmyPrisonerWithNoEligibleMemberSpendsNoKey(t *testing.T) {
	fixture := newJimmyPrisonerFixture(t)

	result := fixture.state.ActionJimmySmallMapWithCharacter(references.Up, nil)

	assert.False(t, result)
	assert.Equal(t, uint16(3), fixture.state.PartyState.Inventory.Provisions.Keys.Get())
	assert.False(t, fixture.state.PartyState.IsNPCDead(references.Britain, fixture.prisonerSlot))
	assert.Equal(t, []string{"No characters available!"}, fixture.callbacks.Messages)
}

func TestJimmyPrisonerLookupIsFloorAware(t *testing.T) {
	fixture := newJimmyPrisonerFixture(t)
	decoySlot := prisonerTestAlternateSlot(fixture.prisonerSlot)
	decoy := prisonerTestNPC(decoySlot, fixture.target, 1, 9)
	fixture.controller.GetNpcs().ReplaceWith([]map_units.MapUnit{decoy, fixture.prisoner})

	require.True(t, fixture.state.ActionJimmySmallMap(references.Up))

	assert.True(t, fixture.state.PartyState.IsNPCDead(references.Britain, fixture.prisonerSlot))
	assert.False(t, fixture.state.PartyState.IsNPCDead(references.Britain, decoySlot))
	assert.Equal(t, byte(references.NoResponse), fixture.prisoner.NPCReference.DialogNumber)
	assert.Equal(t, byte(9), decoy.NPCReference.DialogNumber)
}

func TestJimmyPrisonerRejectsNonFriendlyOccupantBeforeResources(t *testing.T) {
	fixture := newJimmyPrisonerFixture(t)
	enemyReference := enemyReference(fixture.state.GameReferences.EnemyReferences, references.MonsterRat)
	require.NotNil(t, enemyReference)
	enemy := map_units.NewEnemyNPC(*enemyReference, fixture.prisonerSlot)
	enemy.SetPos(fixture.target)
	enemy.SetFloor(0)
	enemy.SetVisible(true)
	fixture.controller.GetNpcs().ReplaceWith([]map_units.MapUnit{&enemy})
	dexChecks := 0
	fixture.state.jimmyPrisonerSuccessForTesting = func(*party_state.PlayerCharacter) bool {
		dexChecks++

		return true
	}

	result := fixture.state.ActionJimmySmallMap(references.Up)

	assert.False(t, result)
	assert.Zero(t, dexChecks)
	assert.Equal(t, uint16(3), fixture.state.PartyState.Inventory.Provisions.Keys.Get())
	assert.Equal(t, 50, fixture.state.PartyState.Karma.Value)
	assert.Equal(t, []string{"No one is there!"}, fixture.callbacks.Messages)
}

func TestJimmyPrisonerDisappearsAfterMapRebuild(t *testing.T) {
	fixture := newJimmyPrisonerFixture(t)
	require.True(t, fixture.state.ActionJimmySmallMap(references.Up))

	fixture.state.TransitionToLocation(
		references.Yew,
		0,
		references.Position{X: 10, Y: 10},
		TransitionFullLocation,
		false,
	)
	fixture.state.TransitionToLocation(
		references.Britain,
		0,
		references.Position{X: 5, Y: 5},
		TransitionFullLocation,
		false,
	)

	assert.False(t, mapUnitsContainSlot(fixture.state.CurrentNPCAIController.GetNpcs(), fixture.prisonerSlot))
}

func TestJimmyPrisonerImmediateNativeCaptureOmitsPersistentlyAbsentUnit(t *testing.T) {
	fixture := newJimmyPrisonerFixture(t)
	require.True(t, fixture.state.ActionJimmySmallMap(references.Up))

	snapshot, err := fixture.state.CaptureSaveSnapshot()
	require.NoError(t, err)
	for _, unit := range snapshot.MapUnits {
		if unit.Kind == "npc" && unit.Position.Location == int(references.Britain) {
			assert.NotEqual(t, fixture.prisonerSlot, unit.NPCNum)
		}
	}

	hydrated, err := NewGameStateFromSaveSnapshot(
		snapshot,
		fixture.state.GameConfig,
		fixture.state.GameReferences,
		xTilesVisibleOnGameScreen,
		yTilesVisibleOnGameScreen,
	)
	require.NoError(t, err)
	assert.True(t, hydrated.PartyState.IsNPCDead(references.Britain, fixture.prisonerSlot))
	assert.False(t, mapUnitsContainSlot(hydrated.CurrentNPCAIController.GetNpcs(), fixture.prisonerSlot))
}

type jimmyPrisonerFixture struct {
	state        *GameState
	callbacks    *MockSystemCallbacks
	controller   *ai.NPCAIControllerSmallMap
	prisoner     *map_units.NPCFriendly
	target       references.Position
	prisonerSlot int
}

func newJimmyPrisonerFixture(t *testing.T) *jimmyPrisonerFixture {
	t.Helper()

	state, callbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithPlayerAt(5, 5).
		WithSystemCallbacks().
		WithRandomSeed(1).
		Build()
	controller, ok := state.CurrentNPCAIController.(*ai.NPCAIControllerSmallMap)
	require.True(t, ok)
	target := references.Position{X: 5, Y: 4}
	currentMap := state.GetLayeredMapByCurrentLocation()
	currentMap.SetTileByLayer(map_state.MapLayer, &target, indexes.Stocks)
	currentMap.UnSetTileByLayer(map_state.MapOverrideLayer, &target)

	prisonerSlot := -1
	for _, unit := range *controller.GetNpcs() {
		if _, friendly := unit.(*map_units.NPCFriendly); friendly && unit.MapUnitDetails().NPCNum != 0 {
			prisonerSlot = unit.MapUnitDetails().NPCNum

			break
		}
	}
	require.NotEqual(t, -1, prisonerSlot, "Britain fixture requires a persistent nonzero NPC slot")
	prisoner := prisonerTestNPC(prisonerSlot, target, 0, 7)
	controller.GetNpcs().ReplaceWith([]map_units.MapUnit{prisoner})
	state.PartyState.Inventory.Provisions.Keys.Set(3)
	state.PartyState.Karma.Value = 50
	state.DateTime.Hour = 12
	state.DateTime.Minute = 0
	state.PartyState.GetCharacter(0).SetStatus(party_state.Good)
	state.PartyState.GetCharacter(0).Dexterity = 30
	state.jimmyPrisonerSuccessForTesting = func(*party_state.PlayerCharacter) bool { return true }

	return &jimmyPrisonerFixture{
		state: state, callbacks: callbacks, controller: controller,
		prisoner: prisoner, target: target, prisonerSlot: prisonerSlot,
	}
}

func prisonerTestNPC(
	slot int,
	position references.Position,
	floor references.FloorNumber,
	dialogNumber byte,
) *map_units.NPCFriendly {
	reference := references.NPCReference{
		DialogNumber: dialogNumber,
		Location:     references.Britain,
		Schedule:     references.CreateNPCScheduledFixedOneLocation(position, floor),
	}
	reference.SetKeyIndex(indexes.TownsPersonKeyIndex)
	npc := map_units.NewNPCFriendly(reference, slot)
	npc.PlaceAtScheduleSlot(0)
	npc.SetVisible(true)

	return npc
}

func prisonerTestAlternateSlot(slot int) int {
	if slot != party_state.NPcsPerLocation-1 {
		return party_state.NPcsPerLocation - 1
	}

	return party_state.NPcsPerLocation - 2
}

func mapUnitsContainSlot(units *map_units.MapUnits, slot int) bool {
	for _, unit := range *units {
		if unit.MapUnitDetails().NPCNum == slot {
			return true
		}
	}

	return false
}
