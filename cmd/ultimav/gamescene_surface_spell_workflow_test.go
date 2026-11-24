package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestSurfaceDirectionalSpellProductionDispatchOwnsResourcesAndTime(t *testing.T) {
	tests := []struct {
		name     string
		spell    references.Spell
		target   indexes.SpriteIndex
		wantTile indexes.SpriteIndex
		wantMP   byte
	}{
		{"An Ylem", references.AnYlem, indexes.Plant, indexes.BrickFloor, 7},
		{"An Sanct", references.AnSanct, indexes.LockedDoor, indexes.RegularDoor, 6},
		{"In Ex Por", references.InExPor, indexes.MagicLockDoor, indexes.RegularDoor, 3},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, mock := gamefixture.NewIntegrationTestBuilder(t).
				WithLocation(references.Britain).
				WithPlayerAt(10, 10).
				WithSystemCallbacks().
				Build()
			caster := state.PartyState.GetCharacter(0)
			caster.Class = party_state.Wizard
			caster.Level = 8
			caster.SetMp(8)
			state.PartyState.Inventory.Spells.Set(test.spell, 1)
			target := references.Position{X: 11, Y: 10}
			state.GetLayeredMapByCurrentLocation().SetTileByLayer(map_state.MapLayer, &target, test.target)
			mock.Reset()

			scene := newProductionCastTestScene(state)
			castState := surfaceSpellFixtureState(state)
			request := game_state.CastRequest{Spell: test.spell, CasterIdx: 0, Direction: references.Right}
			result := state.ValidateAndConsumeSpellResources(request)
			require.True(t, result.Valid, result.ErrorMsg)

			workflow := state.EventQueueManager.Workflow("test_surface_spell_dispatch").
				Priority(event_queue.PriorityNormal).
				Sequential().
				First(func() {})
			scene.dispatchSpellEffect(workflow, request, castState)
			workflow.Finally(func() { scene.finishSpellCastingWorkflow(castState) }).Start()
			drainDismissingOverlays(t, scene, "test_surface_spell_dispatch")

			assert.Equal(t, uint16(0), state.PartyState.Inventory.Spells.Get(test.spell))
			assert.Equal(t, test.wantMP, caster.GetCurrentMp())
			assert.Equal(t, test.wantTile, state.GetLayeredMapByCurrentLocation().GetTileTopMapOnlyTile(&target).Index)
			assert.Equal(t, []int{1}, mock.TimeAdvanced())
		})
	}
}

func TestCoreSurfaceSpellProductionDispatchOwnsResourcesAndTime(t *testing.T) {
	tests := []struct {
		name      string
		spell     references.Spell
		location  references.Location
		direction references.Direction
		prepare   func(*game_state.GameState)
		verify    func(*testing.T, *game_state.GameState, *game_state.MockSystemCallbacks)
	}{
		{
			name:     "In Xen Mani",
			spell:    references.InXenMani,
			location: references.Britain,
			prepare: func(state *game_state.GameState) {
				state.SetRandomSeed(42)
				state.PartyState.Inventory.Provisions.Food.Set(9998)
			},
			verify: func(t *testing.T, state *game_state.GameState, _ *game_state.MockSystemCallbacks) {
				t.Helper()
				assert.Equal(t, uint16(9999), state.PartyState.Inventory.Provisions.Food.Get())
			},
		},
		{
			name:     "In Wis",
			spell:    references.InWis,
			location: references.BritanniaUnderworld,
		},
		{
			name:     "In Mani Corp",
			spell:    references.InManiCorp,
			location: references.Britain,
			prepare: func(state *game_state.GameState) {
				target := state.PartyState.GetCharacter(1)
				target.SetHp(0)
				target.SetStatus(party_state.Dead)
			},
			verify: func(t *testing.T, state *game_state.GameState, _ *game_state.MockSystemCallbacks) {
				t.Helper()
				target := state.PartyState.GetCharacter(1)
				assert.True(t, target.IsGood())
				assert.Equal(t, uint16(1), target.GetCurrentHp())
			},
		},
		{
			name:      "In Por",
			spell:     references.InPor,
			location:  references.BritanniaUnderworld,
			direction: references.Right,
			prepare: func(state *game_state.GameState) {
				for x := references.Coordinate(11); x <= 31; x++ {
					position := references.Position{X: x, Y: 10}
					state.GetLayeredMapByCurrentLocation().SetTileByLayer(map_state.MapOverrideLayer, &position, indexes.BrickFloor)
				}
				target := references.Position{X: 11, Y: 10}
				state.GetLayeredMapByCurrentLocation().SetTileByLayer(map_state.MapOverrideLayer, &target, indexes.Grass)
			},
			verify: func(t *testing.T, state *game_state.GameState, _ *game_state.MockSystemCallbacks) {
				t.Helper()
				assert.Equal(t, references.Position{X: 11, Y: 10}, state.MapState.PlayerLocation.Position)
			},
		},
		{
			name:      "Rel Hur",
			spell:     references.RelHur,
			location:  references.BritanniaUnderworld,
			direction: references.Up,
			verify: func(t *testing.T, _ *game_state.GameState, mock *game_state.MockSystemCallbacks) {
				t.Helper()
				assert.Contains(t, mock.SoundEffectsPlayed, game_state.SoundMagic)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, mock := gamefixture.NewIntegrationTestBuilder(t).
				WithLocation(test.location).
				WithPlayerAt(10, 10).
				WithSystemCallbacks().
				Build()
			caster := state.PartyState.GetCharacter(0)
			caster.Class = party_state.Wizard
			caster.Level = 8
			caster.SetMp(8)
			state.PartyState.Inventory.Spells.Set(test.spell, 1)
			if test.prepare != nil {
				test.prepare(state)
			}
			mock.Reset()

			scene := newProductionCastTestScene(state)
			castState := surfaceSpellFixtureState(state)
			request := game_state.CastRequest{Spell: test.spell, CasterIdx: 0, TargetIdx: 1, Direction: test.direction}
			result := state.ValidateAndConsumeSpellResources(request)
			require.True(t, result.Valid, result.ErrorMsg)
			workflow := state.EventQueueManager.Workflow("test_surface_spell_dispatch").
				Priority(event_queue.PriorityNormal).
				Sequential().
				First(func() {})
			scene.dispatchSpellEffect(workflow, request, castState)
			workflow.Finally(func() { scene.finishSpellCastingWorkflow(castState) }).Start()
			drainDismissingOverlays(t, scene, "test_surface_spell_dispatch")

			assert.Equal(t, uint16(0), state.PartyState.Inventory.Spells.Get(test.spell))
			assert.Equal(t, byte(8-references.GetSpellCircle(test.spell)), caster.GetCurrentMp())
			minutes := game_state.DefaultSmallMapMinutesPerTurn
			if test.location.GetMapType() == references.LargeMapType {
				minutes = game_state.DefaultLargeMapMinutesPerTurn
			}
			assert.Equal(t, []int{minutes}, mock.TimeAdvanced())
			if test.verify != nil {
				test.verify(t, state, mock)
			}
		})
	}
}

func TestRelHurProductionBuilderHandlesRejectedDirection(t *testing.T) {
	state, mock := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(10, 10).
		WithSystemCallbacks().
		Build()
	mock.Reset()
	scene := &GameScene{gameState: state, keyboard: input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld)}
	workflow := state.EventQueueManager.Workflow("test_surface_spell_dispatch").
		Priority(event_queue.PriorityNormal).
		Sequential().
		First(func() {})
	workflow.ThenSequence(scene.thenCastRelHur(references.NoneDirection)).Finally(func() {}).Start()
	drainDismissingOverlays(t, scene, "test_surface_spell_dispatch")

	assert.Contains(t, mock.SoundEffectsPlayed, game_state.SoundSpellFailure)
	assert.NotContains(t, mock.SoundEffectsPlayed, game_state.SoundMagic)
}

func TestWisAnYlemProductionWorkflowUsesFixedAuthenticDuration(t *testing.T) {
	state, mock := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithPlayerAt(10, 10).
		WithSystemCallbacks().
		Build()
	mock.Reset()
	scene := newProductionCastTestScene(state)
	castState := surfaceSpellFixtureState(state)
	request := game_state.CastRequest{Spell: references.WisAnYlem, CasterIdx: 0}
	caster := state.PartyState.GetCharacter(0)
	caster.Class, caster.Level = party_state.Wizard, 8
	caster.SetMp(8)
	state.PartyState.Inventory.Spells.Set(request.Spell, 1)
	result := state.ValidateAndConsumeSpellResources(request)
	require.True(t, result.Valid, result.ErrorMsg)

	workflow := state.EventQueueManager.Workflow("test_surface_spell_dispatch").
		Priority(event_queue.PriorityNormal).
		Sequential().
		BlockInput().
		First(func() {})
	scene.dispatchSpellEffect(workflow, request, castState)
	workflow.Finally(func() {
		scene.finishSpellCastingWorkflow(castState)
	}).Start()
	state.EventQueueManager.Update()

	require.True(t, scene.xRayVisionActive)
	assert.Empty(t, scene.dialogStack.Count(), "the fixed effect must not create a dismissible overlay")
	assert.Empty(t, mock.TimeAdvanced())
	state.ElapsedMs = wisAnYlemDurationMs - 1
	state.EventQueueManager.Update()
	assert.True(t, scene.xRayVisionActive)
	assert.Empty(t, mock.TimeAdvanced())

	state.ElapsedMs = wisAnYlemDurationMs
	state.EventQueueManager.Update()
	assert.False(t, scene.xRayVisionActive)
	assert.Equal(t, []int{1}, mock.TimeAdvanced())
	state.EventQueueManager.Update()
	assert.False(t, state.EventQueueManager.HasWorkflow("test_surface_spell_dispatch"))
}

// Dispatch fixtures use the same ownership/commit boundary as the production
// Cast workflow, rather than appending a separate time-advancement algorithm.
func surfaceSpellFixtureState(state *game_state.GameState) *spellCastingWorkflowState {
	return &spellCastingWorkflowState{
		ownsSurfaceCommand:    true,
		surfaceOriginMapType:  state.MapState.PlayerLocation.Location.GetMapType(),
		surfaceOriginLocation: state.MapState.PlayerLocation.Location,
		commitSequence:        state.CastUseCommitSequence(),
	}
}

func TestParseMoonPhaseInputRejectsCancellationAndInvalidInput(t *testing.T) {
	state, mock := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		Build()
	caster := state.PartyState.GetCharacter(0)
	caster.SetMp(8)
	state.PartyState.Inventory.Spells.Set(references.VasRelPor, 1)
	mock.Reset()

	for _, test := range []struct {
		text      string
		submitted bool
	}{
		{"", false},
		{"", true},
		{"0", true},
		{"9", true},
		{"x", true},
	} {
		phase, selected := parseMoonPhaseInput(test.text, test.submitted)
		assert.False(t, selected)
		assert.Equal(t, datetime.MoonPhaseNew, phase)
	}
	assert.Equal(t, byte(8), caster.GetCurrentMp())
	assert.Equal(t, uint16(1), state.PartyState.Inventory.Spells.Get(references.VasRelPor))
	assert.Empty(t, mock.TimeAdvanced())

	for inputPhase := 1; inputPhase <= 8; inputPhase++ {
		phase, selected := parseMoonPhaseInput(fmt.Sprintf("%d", inputPhase), true)
		assert.True(t, selected)
		assert.Equal(t, datetime.MoonPhase(inputPhase-1), phase)
	}
}

func TestVasRelPorProductionDispatchOwnsTransitionResourcesAndOneMinute(t *testing.T) {
	tests := []struct {
		name             string
		start            references.Location
		destinationMap   uint8
		destination      references.Location
		destinationFloor references.FloorNumber
	}{
		{"overworld to town", references.BritanniaUnderworld, uint8(references.Britain), references.Britain, 1},
		{"town to overworld", references.Britain, 0, references.BritanniaUnderworld, 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, mock := gamefixture.NewIntegrationTestBuilder(t).
				WithLocation(test.start).
				WithPlayerAt(10, 10).
				WithSystemCallbacks().
				Build()
			caster := state.PartyState.GetCharacter(0)
			caster.Class = party_state.Wizard
			caster.Level = 8
			caster.SetMp(8)
			state.PartyState.Inventory.Spells.Set(references.VasRelPor, 1)
			stone := state.GetMoonstoneSystem().GetMoonstoneByIndex(int(datetime.MoonPhaseFirstQuarter))
			stone.X, stone.Y = 14, 15
			stone.Map = test.destinationMap
			stone.Level = uint8(test.destinationFloor)
			stone.State = game_state.MoonstoneStateBuried
			request := game_state.CastRequest{
				Spell: references.VasRelPor, CasterIdx: 0, MoonPhase: datetime.MoonPhaseFirstQuarter,
			}
			destination, err := state.GetMoongateDestinationForPhase(request.MoonPhase)
			require.NoError(t, err)
			state.DateTime.Hour, state.DateTime.Minute = 5, 59
			state.PartyState.Inventory.QuestItems.Set(references.Sextant, 1)
			mock.Reset()

			validation := state.ValidateAndConsumeSpellResources(request)
			require.True(t, validation.Valid, validation.ErrorMsg)
			scene := &GameScene{gameState: state, keyboard: input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld)}
			castState := &spellCastingWorkflowState{moongateDestination: destination}
			workflow := state.EventQueueManager.Workflow("test_surface_spell_dispatch").
				Priority(event_queue.PriorityNormal).
				Sequential().
				First(func() {})
			scene.dispatchSpellEffect(workflow, request, castState)
			workflow.Finally(func() {}).Start()
			drainDismissingOverlays(t, scene, "test_surface_spell_dispatch")

			assert.Equal(t, uint16(0), state.PartyState.Inventory.Spells.Get(references.VasRelPor))
			assert.Equal(t, byte(0), caster.GetCurrentMp())
			assert.Equal(t, []int{1}, mock.TimeAdvanced())
			assert.Equal(t, test.destination, state.MapState.PlayerLocation.Location)
			assert.Equal(t, test.destinationFloor, state.MapState.PlayerLocation.Floor)
			assert.Equal(t, references.Position{X: 14, Y: 15}, state.MapState.PlayerLocation.Position)
			entry := state.CurrentQuestJournalDossier().Moonstones[datetime.MoonPhaseFirstQuarter]
			assert.True(t, entry.Known && entry.Placed)
			assert.Equal(t, "Reached through Vas Rel Por", entry.FirstSource.String())
			assert.Equal(t, 5, entry.ObservedAt.Time.Hour)
			assert.Equal(t, 59, entry.ObservedAt.Time.Minute, "observation precedes the existing surface casting minute")
			assert.Equal(t, test.destination == references.BritanniaUnderworld, entry.LastObservedSurvey.Measured)
		})
	}
}

func TestActualMoongateTeleportUsesDestinationWithoutOwningTimeOrSyntheticDaytimeGate(t *testing.T) {
	state, mock := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(10, 10).
		WithSystemCallbacks().
		Build()
	state.DateTime.Hour = 12
	mock.Reset()
	scene := &GameScene{
		gameState:            state,
		moongateVisualSystem: NewMoongateVisualSystem(state),
	}
	destination := game_state.MoongateDestination{
		X: 14, Y: 15, Map: uint8(references.Britain), Level: 1, Location: references.Britain,
	}
	state.SystemCallbacks.Visual.ResetTeleportationSourceRenderer = scene.resetTeleportationSourceRenderer
	state.SystemCallbacks.Visual.StabilizeTeleportationDestinationRenderer = scene.stabilizeTeleportationDestinationRenderer
	beforeTime := state.DateTime

	state.EventQueueManager.Workflow("visual_teleport_test").
		Priority(event_queue.PriorityHigh).
		Sequential().
		First(func() {}).
		ThenSequence(state.BuildTeleportationSequence(destination)).
		Finally(func() {}).
		Start()
	for state.EventQueueManager.HasActiveWorkflows() {
		state.ElapsedMs += 1000
		state.EventQueueManager.Update()
	}

	assert.Equal(t, references.Britain, state.MapState.PlayerLocation.Location)
	assert.Equal(t, references.FloorNumber(1), state.MapState.PlayerLocation.Floor)
	assert.Equal(t, references.Position{X: 14, Y: 15}, state.MapState.PlayerLocation.Position)
	assert.Equal(t, beforeTime, state.DateTime)
	assert.Empty(t, scene.moongateVisualSystem.GetActiveRenderers())
}
