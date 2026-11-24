package main

import (
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestSurfaceWaterfallCompletionCheckpoint(t *testing.T) {
	tests := []struct {
		name     string
		complete func(*GameScene)
		consumes bool
	}{
		{name: "standing Pass through keyboard routing", consumes: true, complete: func(scene *GameScene) {
			scene.largeMapInputHandler(ebiten.KeySpace)
		}},
		{name: "completed command", consumes: true, complete: func(scene *GameScene) {
			scene.completeSurfaceCommand(newSurfaceCommandResult(surfaceCommandCompleted, references.LargeMapType))
		}},
		{name: "rejected consumed command", consumes: true, complete: func(scene *GameScene) {
			scene.completeSurfaceCommand(newSurfaceCommandResult(surfaceCommandRejected, references.LargeMapType))
		}},
		{name: "cancelled consumed command", consumes: true, complete: func(scene *GameScene) {
			scene.completeSurfaceCommand(newSurfaceCommandResult(surfaceCommandCancelled, references.LargeMapType))
		}},
		{name: "completed movement", consumes: true, complete: func(scene *GameScene) {
			result := newSurfaceCommandResult(surfaceCommandCompleted, references.LargeMapType)
			result.MovementOccurred = true
			scene.completeSurfaceMovement(result)
		}},
		{name: "blocked movement", complete: func(scene *GameScene) {
			scene.completeSurfaceMovement(newSurfaceCommandResult(surfaceCommandRejected, references.LargeMapType))
		}},
		{name: "free cancellation", complete: func(scene *GameScene) {
			result := newSurfaceCommandResult(surfaceCommandCancelled, references.LargeMapType)
			result.ConsumesNormalCycle = false
			scene.completeSurfaceCommand(result)
		}},
		{name: "deferred command", complete: func(scene *GameScene) {
			scene.completeSurfaceCommand(newSurfaceCommandResult(surfaceCommandDeferred, references.LargeMapType))
		}},
		{name: "still active command", complete: func(scene *GameScene) {
			scene.completeSurfaceCommand(newSurfaceCommandResult(surfaceCommandStillActive, references.LargeMapType))
		}},
		{name: "transition owns completion", complete: func(scene *GameScene) {
			result := newSurfaceCommandResult(surfaceCommandCompleted, references.LargeMapType)
			result.MapTransitioned = true
			scene.completeSurfaceCommand(result)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
				WithLocation(references.BritanniaUnderworld).WithPlayerAt(50, 50).
				WithSystemCallbacks().WithRandomSeed(123).Build()
			state.DebugOptions.MonsterGen = false
			state.GetCurrentLargeMapNPCAIController().GetNpcs().RemoveAllEnemies()
			state.PartyVehicle = map_units.NewNPCFriendlyVehiceNoVehicle()
			state.PartyState.ForEachCharacter(func(_ int, member *party_state.PlayerCharacter) {
				member.Dexterity = 1
				member.SetMaxHp(1000)
				member.SetHp(1000)
				member.SetStatus(party_state.Good)
			})
			state.ActiveDurationEffect = game_state.DurationEffect{
				EffectType: game_state.DurationEffectProtection, TurnsRemaining: 8,
			}
			for y := 50; y <= 55; y++ {
				position := references.Position{X: 50, Y: references.Coordinate(y)}
				tile := indexes.SpriteIndex(indexes.Grass)
				if y == 51 || y == 53 {
					tile = indexes.Waterfall1
				}
				state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, &position, tile)
			}
			callbacks.Reset()
			scene := newSurfaceCompletionTestScene(state)
			before := state.DateTime
			test.complete(scene)
			assert.Equal(t, test.consumes, state.IsWaterfallHazardActive())
			if !test.consumes {
				assert.Equal(t, before, state.DateTime)
				assert.Zero(t, state.GetRNG().Calls())
				assert.Equal(t, 8, state.ActiveDurationEffect.TurnsRemaining)
				assert.False(t, callbacks.InputBlocked)

				return
			}
			charged := state.DateTime
			assert.Equal(t, byte(2), charged.Minute-before.Minute)
			assert.Equal(t, before.Turn+1, charged.Turn)
			for updates := 0; state.IsWaterfallHazardActive(); updates++ {
				require.Less(t, updates, 200)
				assert.True(t, callbacks.InputBlocked)
				state.EventQueueManager.Update()
				state.ElapsedMs += 100
			}
			assert.Equal(t, references.Position{X: 50, Y: 54}, state.MapState.PlayerLocation.Position)
			assert.Equal(t, charged, state.DateTime)
			assert.Equal(t, uint16(998), state.PartyState.GetAvatar().GetCurrentHp())
			assert.Equal(t, 7, state.ActiveDurationEffect.TurnsRemaining)
			assert.False(t, callbacks.InputBlocked)
			assert.Equal(t, 1, callbacks.BlockInputCalls)
			assert.Equal(t, 1, callbacks.UnblockInputCalls)
		})
	}
}

func TestSurfaceWaterfallResumesAfterTacticalReturnBeforeQueuedPass(t *testing.T) {
	scene, state, callbacks := newSmashedInputTestScene(t, 0, 123)
	scene.spriteSheet = sharedPresentationTestSpriteSheet()
	now := time.Unix(100, 0)
	scene.startTime, scene.now = now, func() time.Time { return now }
	scene.lastEffectsUpdate = now
	block := state.SystemCallbacks.Visual.BlockAllInput
	state.SystemCallbacks.Visual.BlockAllInput = func(blocked bool) {
		block(blocked)
		scene.blockAllInput(blocked)
	}
	state.TransitionToLocation(references.BritanniaUnderworld, references.MainFloor,
		references.Position{X: 50, Y: 50}, game_state.TransitionFullLocation, false)
	state.GetCurrentLargeMapNPCAIController().GetNpcs().RemoveAllEnemies()
	state.DebugOptions.MonsterGen = false
	state.PartyVehicle = map_units.NewNPCFriendlyVehiceNoVehicle()
	state.PartyState.ForEachCharacter(func(_ int, member *party_state.PlayerCharacter) {
		member.PartyMap = party_state.PartyMapAtInn(references.Britain)
		member.SetStatus(party_state.Good)
		member.SetMaxHp(1000)
		member.SetHp(1000)
		member.SetEquippedItem(references.SlotRing, references.NoEquipment)
	})
	avatar := state.PartyState.GetAvatar()
	avatar.PartyMap, avatar.Dexterity = party_state.PartyMapActive, 30
	for y := references.Coordinate(49); y <= 55; y++ {
		for x := references.Coordinate(47); x <= 53; x++ {
			position := references.Position{X: x, Y: y}
			tile := indexes.SpriteIndex(indexes.Grass)
			if x == 50 && (y == 51 || y == 53) {
				tile = indexes.Waterfall1
			}
			state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &position, tile)
		}
	}
	state.ActiveDurationEffect = game_state.DurationEffect{
		EffectType: game_state.DurationEffectProtection, TurnsRemaining: 8,
	}
	addOutdoorRetreatSceneEnemy(t, state, references.MonsterSeaSerpent, references.Position{X: 51, Y: 52}, 31)
	state.CompleteSurfacePassCycle(references.LargeMapType)
	charged := state.DateTime
	drainSurfaceCompletionWorkflow(t, state, "waterfall_hazard")
	require.True(t, state.IsWaterfallHazardActive())
	require.True(t, state.HasPendingOverworldContact())
	assert.False(t, scene.inputBlocked)
	assert.False(t, callbacks.InputBlocked)
	now = scene.startTime.Add(time.Duration(state.ElapsedMs+1) * time.Millisecond)
	require.NoError(t, scene.Update(nil)) // Existing pending-contact drain admits combat.
	require.NotNil(t, state.CombatScene)
	now = now.Add(time.Millisecond)
	require.NoError(t, scene.Update(nil)) // Tactical scheduler is not blocked by the cascade.
	require.True(t, state.CombatScene.CombatTurnsStarted())
	assert.False(t, scene.inputBlocked)
	state.EndCombat(game_state.CombatVictory)
	require.Nil(t, state.CombatScene)
	require.True(t, state.IsWaterfallHazardActive())
	assert.Equal(t, references.Position{X: 50, Y: 52}, state.MapState.PlayerLocation.Position)
	assert.ErrorIs(t, state.ValidateSaveSnapshotEligibility(), game_state.ErrSaveSnapshotNotRepresentable)
	scene.keyboard.SimulateKeyPress(ebiten.KeySpace)
	now = now.Add(time.Millisecond)
	require.NoError(t, scene.Update(nil))
	assert.True(t, state.EventQueueManager.HasWorkflow("waterfall_hazard"), "production pre-input hook resumes without another command")
	assert.True(t, scene.inputBlocked)
	assert.Equal(t, charged, state.DateTime, "the queued Pass cannot consume a fresh ordinary cycle")
	assert.Equal(t, 7, state.ActiveDurationEffect.TurnsRemaining)
	now = now.Add(time.Millisecond)
	require.NoError(t, scene.Update(nil))
	assert.Equal(t, charged, state.DateTime)
	drainSurfaceCompletionWorkflow(t, state, "waterfall_hazard")
	assert.Equal(t, references.Position{X: 50, Y: 54}, state.MapState.PlayerLocation.Position)
	assert.Equal(t, charged, state.DateTime)
	assert.Equal(t, 7, state.ActiveDurationEffect.TurnsRemaining)
	assert.False(t, state.IsWaterfallHazardActive())
	assert.False(t, scene.inputBlocked)
}

func TestDeferredSurfaceFireShutdownAtWaterfallBeforeFirstUpdate(t *testing.T) {
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).WithPlayerAt(50, 50).
		WithSystemCallbacks().WithRandomSeed(123).Build()
	state.CurrentNPCAIController = state.GetCurrentLargeMapNPCAIController()
	state.CurrentNPCAIController.GetNpcs().RemoveAllEnemies()
	state.DebugOptions.MonsterGen = false
	state.PartyState.ForEachCharacter(func(_ int, member *party_state.PlayerCharacter) {
		member.SetStatus(party_state.Good)
		member.SetEquippedItem(references.SlotRing, references.NoEquipment)
	})
	position := state.MapState.PlayerLocation.Position
	state.PartyVehicle = *map_units.NewNPCFriendlyVehiceNewRef(references.FrigateVehicle, position, references.MainFloor)
	state.PartyVehicle.GetVehicleDetails().SetPartyVehicleDirection(references.Up)
	state.PartyVehicle.GetVehicleDetails().SetSailState(map_units.SailFurled)
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, &position, indexes.Grass)
	south := position.GetPositionDown()
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, south, indexes.Waterfall1)
	state.ActiveDurationEffect = game_state.DurationEffect{
		EffectType: game_state.DurationEffectProtection, TurnsRemaining: 3,
	}
	callbacks.Reset()
	scene := newSurfaceCompletionTestScene(state)
	before, calls, hp := state.DateTime, state.GetRNG().Calls(), state.PartyState.GetAvatar().GetCurrentHp()
	scene.largeMapInputHandler(ebiten.KeyF)
	require.Equal(t, FireDirectionInput, scene.secondaryKeyState)
	scene.keyboard.SimulateKeyPress(ebiten.KeyRight)
	scene.largeMapHandleSecondaryInput()
	require.Equal(t, PrimaryInput, scene.secondaryKeyState)
	require.True(t, state.EventQueueManager.HasWorkflow("ship_cannon_fire"), "real broadside was accepted")
	require.True(t, state.EventQueueManager.HasWorkflow("surface_command_completion"))
	assert.False(t, state.IsWaterfallHazardActive())
	assert.Equal(t, before, state.DateTime, "accepted Fire is still awaiting its deferred completion")

	// No Update: the accepted command finalizer runs AFTER GameState's shutdown
	// action, exactly the point where an unguarded hazard admission orphaned state.
	require.NotPanics(t, state.EventQueueManager.Shutdown)
	want := before
	want.Advance(game_state.DefaultLargeMapMinutesPerTurn)
	assert.Equal(t, want, state.DateTime)
	assert.Equal(t, []int{game_state.DefaultLargeMapMinutesPerTurn}, callbacks.TimeAdvanced())
	assert.Equal(t, 2, state.ActiveDurationEffect.TurnsRemaining, "accepted player completion still runs once")
	assert.Equal(t, position, state.MapState.PlayerLocation.Position)
	assert.Equal(t, hp, state.PartyState.GetAvatar().GetCurrentHp())
	assert.Equal(t, calls, state.GetRNG().Calls())
	assert.False(t, state.IsWaterfallHazardActive())
	assert.False(t, state.EventQueueManager.HasWorkflow("waterfall_hazard"))
	assert.False(t, state.EventQueueManager.HasActiveWorkflows())
	assert.False(t, callbacks.InputBlocked)
	assert.NoError(t, state.ValidateSaveSnapshotEligibility(), "shutdown completion must not leave a retained cascade")
	require.NotPanics(t, func() { assert.False(t, state.ResumeWaterfallContinuation()) })
	state.EventQueueManager.Shutdown()
	state.EventQueueManager.Update()
	assert.Equal(t, want, state.DateTime)
	assert.Equal(t, 2, state.ActiveDurationEffect.TurnsRemaining)
	assert.Equal(t, []int{game_state.DefaultLargeMapMinutesPerTurn}, callbacks.TimeAdvanced())
	assert.False(t, state.ProcessWaterfallHazard(&position))
	assert.NoError(t, state.ValidateSaveSnapshotEligibility())
}
