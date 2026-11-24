package main

import (
	"fmt"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/environment"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestStartAutomaticSailingConsumesInputWhileWorkflowNameRemainsLive(t *testing.T) {
	var elapsedMs int64
	manager := event_queue.NewEventQueueManager(func() int64 { return elapsedMs }, nil)
	manager.Workflow(automaticSailingWorkflowName).
		Priority(event_queue.PriorityNormal).
		Sequential().
		First(func() {}).
		Wait(1000).
		Start()
	scene := &GameScene{
		gameState: &game_state.GameState{EventQueueManager: manager},
	}

	require.NotPanics(t, func() {
		require.True(t, scene.startAutomaticSailing(references.Up))
	})
	require.True(t, manager.HasWorkflow(automaticSailingWorkflowName))
}

func TestAutomaticSailingProductionWorkflowUsesSourceWaitCadence(t *testing.T) {
	tests := []struct {
		name      string
		wind      environment.WindDirection
		direction references.Direction
		phases    int
		wantTime  int
	}{
		{name: "n three zero waits", wind: environment.North, direction: references.Right, phases: 1, wantTime: 2},
		{name: "n one one wait", wind: environment.North, direction: references.Down, phases: 2, wantTime: 4},
		{name: "n two two waits", wind: environment.North, direction: references.Up, phases: 3, wantTime: 6},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAutomaticSailingWorkflowFixture(t, test.wind)
			fixture.state.ActiveDurationEffect = game_state.DurationEffect{
				EffectType: game_state.DurationEffectProtection, TurnsRemaining: 5,
			}
			before := fixture.state.MapState.PlayerLocation.Position

			fixture.scene.largeMapInputHandler(automaticSailingDirectionKey(test.direction))

			assert.Equal(t, test.direction, fixture.state.PartyVehicle.GetVehicleDetails().CurrentDirection())
			assert.Equal(t, before, fixture.state.MapState.PlayerLocation.Position)
			fixture.advancePhases(test.phases)
			assert.Contains(t, fixture.callbacks.CommandPrompts, "Sail "+test.direction.GetDirectionCompassName())
			assert.NotContains(t, fixture.callbacks.CommandPrompts, "Row "+test.direction.GetDirectionCompassName())

			wantPosition := test.direction.GetNewPositionInDirection(&before).
				GetWrapped(references.XLargeMapTiles, references.YLargeMapTiles)
			assert.Equal(t, *wantPosition, fixture.state.MapState.PlayerLocation.Position)
			assert.Equal(t, *wantPosition, fixture.state.PartyVehicle.Pos())
			assert.Equal(t, test.wantTime, fixture.elapsedMinutes())
			assert.Equal(t, 4, fixture.state.ActiveDurationEffect.TurnsRemaining,
				"wait phases run world actors but only movement owns ordinary player-turn effects")
		})
	}
}

func TestAutomaticSailingHasNoIndependentWhirlpoolGeneration(t *testing.T) {
	for _, floor := range []references.FloorNumber{0, -1} {
		t.Run(fmt.Sprint(floor), func(t *testing.T) {
			fixture := newAutomaticSailingWorkflowFixture(t, environment.North)
			state := fixture.state
			state.TransitionToLocation(references.BritanniaUnderworld, floor,
				state.MapState.PlayerLocation.Position, game_state.TransitionFullLocation, false)
			state.CurrentNPCAIController.RemoveAllEnemies()
			state.DateTime.Hour = 12
			state.DateTime.Minute = 0
			state.DebugOptions.MonsterGen = false
			position := state.MapState.PlayerLocation.Position
			for x := position.X - 16; x <= position.X+16; x++ {
				for y := position.Y - 16; y <= position.Y+16; y++ {
					pos := references.Position{X: x, Y: y}
					state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, &pos, indexes.Water1)
				}
			}
			state.SetRandomSeed(458)
			fixture.scene.largeMapInputHandler(ebiten.KeyRight)
			fixture.advancePhases(1)
			assert.Equal(t, position.X+1, state.MapState.PlayerLocation.Position.X)
			assert.False(t, state.HasAnyWhirlpool())
			wantCalls := uint64(0)
			if floor == -1 {
				wantCalls = 1 // ordinary Underworld earthquake eligibility only
			}
			assert.Equal(t, wantCalls, state.GetRNG().Calls())
		})
	}
}

func TestAutomaticSailingHMSCapeChangesOnlyAutomaticWaitPhases(t *testing.T) {
	tests := []struct {
		name      string
		direction references.Direction
		phases    int
		wantTime  int
	}{
		{name: "crosswind zero waits retains base two move", direction: references.Right, phases: 1, wantTime: 2},
		{name: "tailwind one wait costs one plus base move", direction: references.Down, phases: 2, wantTime: 3},
		{name: "headwind two waits cost one each plus base move", direction: references.Up, phases: 3, wantTime: 4},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAutomaticSailingWorkflowFixture(t, environment.North)
			fixture.state.SetHMSCapeActivated(true)
			before := fixture.state.MapState.PlayerLocation.Position

			fixture.scene.largeMapInputHandler(automaticSailingDirectionKey(test.direction))
			fixture.advancePhases(test.phases)

			wantPosition := test.direction.GetNewPositionInDirection(&before).
				GetWrapped(references.XLargeMapTiles, references.YLargeMapTiles)
			assert.Equal(t, *wantPosition, fixture.state.MapState.PlayerLocation.Position)
			assert.Equal(t, test.wantTime, fixture.elapsedMinutes())
		})
	}
}

func TestAutomaticSailingDirectionInputLifecycle(t *testing.T) {
	t.Run("different heading resets waits while same heading continues", func(t *testing.T) {
		fixture := newAutomaticSailingWorkflowFixture(t, environment.North)
		before := fixture.state.MapState.PlayerLocation.Position
		fixture.scene.largeMapInputHandler(ebiten.KeyDown)

		fixture.advancePhases(1)
		assert.Equal(t, 2, fixture.elapsedMinutes())

		fixture.held[ebiten.KeyUp] = true
		fixture.advancePhases(1)
		delete(fixture.held, ebiten.KeyUp)
		assert.Equal(t, references.Up, fixture.state.PartyVehicle.GetVehicleDetails().CurrentDirection())
		assert.Equal(t, 2, fixture.elapsedMinutes(), "heading change is not a wait or move phase")

		fixture.advancePhases(2)
		assert.Equal(t, before, fixture.state.MapState.PlayerLocation.Position)
		assert.Equal(t, 6, fixture.elapsedMinutes())

		fixture.held[ebiten.KeyUp] = true
		fixture.advancePhases(1)
		assert.NotEqual(t, before, fixture.state.MapState.PlayerLocation.Position)
		assert.Equal(t, 8, fixture.elapsedMinutes())
	})

	t.Run("initiating arrow replaces prior heading", func(t *testing.T) {
		fixture := newAutomaticSailingWorkflowFixture(t, environment.North)
		assert.Equal(t, references.Left, fixture.state.PartyVehicle.GetVehicleDetails().CurrentDirection())

		fixture.scene.largeMapInputHandler(ebiten.KeyRight)

		assert.Equal(t, references.Right, fixture.state.PartyVehicle.GetVehicleDetails().CurrentDirection())
		assert.Empty(t, fixture.callbacks.TimeAdvanced())
	})
}

func TestAutomaticSailingCalmWaitsUntilWindOrInterruption(t *testing.T) {
	fixture := newAutomaticSailingWorkflowFixture(t, environment.Calm)
	before := fixture.state.MapState.PlayerLocation.Position
	fixture.scene.largeMapInputHandler(ebiten.KeyUp)

	fixture.advancePhases(3)

	assert.Equal(t, before, fixture.state.MapState.PlayerLocation.Position)
	assert.Equal(t, 6, fixture.elapsedMinutes())
	assert.True(t, fixture.state.IsSailing())

	fixture.state.SetWindDirection(environment.North, false)
	fixture.advancePhases(1)

	assert.NotEqual(t, before, fixture.state.MapState.PlayerLocation.Position,
		"accumulated calm waits satisfy the new non-calm threshold")
	assert.Equal(t, 8, fixture.elapsedMinutes())
}

func TestAutomaticSailingStopInputDoesNotFurl(t *testing.T) {
	t.Run("space stops without an extra phase", func(t *testing.T) {
		fixture := newAutomaticSailingWorkflowFixture(t, environment.North)
		fixture.state.ActiveDurationEffect = game_state.DurationEffect{
			EffectType: game_state.DurationEffectProtection, TurnsRemaining: 5,
		}
		before := fixture.state.MapState.PlayerLocation.Position
		fixture.scene.largeMapInputHandler(ebiten.KeyRight)
		fixture.held[ebiten.KeySpace] = true

		fixture.advancePhases(1)

		assert.False(t, fixture.state.IsSailing())
		assert.Equal(t, before, fixture.state.MapState.PlayerLocation.Position)
		assert.Empty(t, fixture.callbacks.TimeAdvanced())
		assert.Contains(t, fixture.callbacks.Messages, "Sheets in irons!")
		assert.Equal(t, map_units.SailRaised, fixture.state.PartyVehicle.GetVehicleDetails().SailState)
		assert.True(t, fixture.scene.keyboard.IsWaitingForRelease(), "Space is consumed by sailing")
		assert.Equal(t, 5, fixture.state.ActiveDurationEffect.TurnsRemaining,
			"input interruption returns to dispatch without a player-update phase")
	})

	t.Run("other command interrupts and remains processable", func(t *testing.T) {
		fixture := newAutomaticSailingWorkflowFixture(t, environment.North)
		before := fixture.state.MapState.PlayerLocation.Position
		fixture.scene.largeMapInputHandler(ebiten.KeyRight)
		fixture.held[ebiten.KeyL] = true

		fixture.advancePhases(1)
		fixture.scene.largeMapInputHandler(ebiten.KeyL)

		assert.False(t, fixture.state.IsSailing())
		assert.Equal(t, LookDirectionInput, fixture.scene.secondaryKeyState)
		assert.Equal(t, before, fixture.state.MapState.PlayerLocation.Position)
		assert.Empty(t, fixture.callbacks.TimeAdvanced())
		assert.Equal(t, map_units.SailRaised, fixture.state.PartyVehicle.GetVehicleDetails().SailState)
		assert.False(t, fixture.scene.keyboard.IsWaitingForRelease())
	})
}

func TestAutomaticSailingMovementOutcomesRemainCallerOwned(t *testing.T) {
	t.Run("terrain collision stops and damages", func(t *testing.T) {
		fixture := newAutomaticSailingWorkflowFixture(t, environment.North)
		fixture.state.DebugOptions.FreeMove = false
		before := fixture.state.MapState.PlayerLocation.Position
		target := references.Right.GetNewPositionInDirection(&before).
			GetWrapped(references.XLargeMapTiles, references.YLargeMapTiles)
		fixture.state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, target, indexes.TallMountains)
		fixture.scene.largeMapInputHandler(ebiten.KeyRight)

		fixture.advancePhases(1)

		assert.False(t, fixture.state.IsSailing())
		assert.Equal(t, before, fixture.state.MapState.PlayerLocation.Position)
		assert.Empty(t, fixture.callbacks.TimeAdvanced(), "blocked sailing does not complete a movement cycle")
		assert.Less(t, fixture.state.PartyVehicle.GetVehicleDetails().GetHullHP(), 99)
		assert.Equal(t, map_units.SailRaised, fixture.state.PartyVehicle.GetVehicleDetails().SailState)
	})

	t.Run("later sailing wait replaces earlier provision crossing", func(t *testing.T) {
		fixture := newAutomaticSailingWorkflowFixture(t, environment.North)
		fixture.state.DebugOptions.FreeMove = false
		fixture.state.DateTime.Hour, fixture.state.DateTime.Minute = 5, 59
		fixture.state.PartyState.Inventory.Provisions.Food.Set(100)
		foodBefore := fixture.state.PartyState.Inventory.Provisions.Food.Get()
		before := fixture.state.MapState.PlayerLocation.Position
		target := references.Up.GetNewPositionInDirection(&before).
			GetWrapped(references.XLargeMapTiles, references.YLargeMapTiles)
		fixture.state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, target, indexes.TallMountains)
		fixture.scene.largeMapInputHandler(ebiten.KeyUp)

		fixture.advancePhases(3)
		for fixture.state.EventQueueManager.HasActiveWorkflows() {
			fixture.state.ElapsedMs += 1000
			fixture.state.EventQueueManager.Update()
		}

		assert.False(t, fixture.state.IsSailing())
		assert.Equal(t, before, fixture.state.MapState.PlayerLocation.Position)
		assert.Equal(t, foodBefore, fixture.state.PartyState.Inventory.Provisions.Food.Get())
		require.NoError(t, fixture.state.ValidateSaveSnapshotEligibility())
	})

	t.Run("pier docks without hull damage", func(t *testing.T) {
		fixture := newAutomaticSailingWorkflowFixture(t, environment.North)
		fixture.state.DebugOptions.FreeMove = false
		before := fixture.state.MapState.PlayerLocation.Position
		target := references.Right.GetNewPositionInDirection(&before).
			GetWrapped(references.XLargeMapTiles, references.YLargeMapTiles)
		fixture.state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, target, indexes.Dock)
		fixture.scene.largeMapInputHandler(ebiten.KeyRight)

		fixture.advancePhases(1)

		assert.False(t, fixture.state.IsSailing())
		assert.Equal(t, before, fixture.state.MapState.PlayerLocation.Position)
		assert.Equal(t, 99, fixture.state.PartyVehicle.GetVehicleDetails().GetHullHP())
		assert.Equal(t, map_units.SailFurled, fixture.state.PartyVehicle.GetVehicleDetails().SailState)
	})

	t.Run("shoal collision uses the shared hit without an extra minute", func(t *testing.T) {
		fixture := newAutomaticSailingWorkflowFixture(t, environment.North)
		fixture.state.DebugOptions.FreeMove = false
		before := fixture.state.MapState.PlayerLocation.Position
		target := references.Right.GetNewPositionInDirection(&before).
			GetWrapped(references.XLargeMapTiles, references.YLargeMapTiles)
		fixture.state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, target, indexes.WaterCoast)
		fixture.scene.largeMapInputHandler(ebiten.KeyRight)

		fixture.advancePhases(1)

		assert.False(t, fixture.state.IsSailing())
		assert.Equal(t, before, fixture.state.MapState.PlayerLocation.Position)
		assert.Empty(t, fixture.callbacks.TimeAdvanced())
		assert.Less(t, fixture.state.PartyVehicle.GetVehicleDetails().GetHullHP(), 99)
	})

	t.Run("lethal collision removes the frigate without creating a skiff and starts death once", func(t *testing.T) {
		fixture := newAutomaticSailingWorkflowFixture(t, environment.North)
		fixture.state.DebugOptions.FreeMove = false
		for index := range party_state.MaxActiveParty {
			member := fixture.state.PartyState.GetCharacter(index)
			member.PartyMap = party_state.PartyMapAtInn(references.Britain)
		}
		member := fixture.state.PartyState.GetCharacter(0)
		member.PartyMap = party_state.PartyMapActive
		member.SetStatus(party_state.Good)
		member.SetMaxHp(1)
		member.SetHp(1)
		details := fixture.state.PartyVehicle.GetVehicleDetails()
		details.SetHullHP(1)
		details.SetSkiffQuantity(0)
		fixture.state.PartyState.Inventory.QuestItems.Set(references.Carpet, 0)
		deathSequences := 0
		fixture.state.SystemCallbacks.Flow.Consequences.TriggerDeathSequence = func() { deathSequences++ }
		before := fixture.state.MapState.PlayerLocation.Position
		target := references.Right.GetNewPositionInDirection(&before).
			GetWrapped(references.XLargeMapTiles, references.YLargeMapTiles)
		fixture.state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, target, indexes.TallMountains)
		fixture.scene.largeMapInputHandler(ebiten.KeyRight)

		fixture.advancePhases(1)
		for fixture.state.EventQueueManager.HasActiveWorkflows() {
			fixture.state.ElapsedMs += 1000
			fixture.state.EventQueueManager.Update()
		}

		assert.Equal(t, before, fixture.state.MapState.PlayerLocation.Position)
		assert.Equal(t, references.NoPartyVehicle, fixture.state.PartyVehicle.GetVehicleDetails().VehicleType)
		assert.True(t, fixture.state.PartyState.IsAllPartyDead())
		assert.Equal(t, 1, deathSequences)
		assert.Empty(t, fixture.callbacks.TimeAdvanced())
	})

	t.Run("actor contact interrupts before movement", func(t *testing.T) {
		fixture := newAutomaticSailingWorkflowFixture(t, environment.North)
		enableAutomaticSailingCamera(fixture.scene, true)
		before := fixture.state.MapState.PlayerLocation.Position
		target := references.Right.GetNewPositionInDirection(&before).
			GetWrapped(references.XLargeMapTiles, references.YLargeMapTiles)
		enemyRef := (*fixture.state.GameReferences.EnemyReferences)[int(references.MonsterDragon)]
		enemy := map_units.NewEnemyNPC(enemyRef, 1)
		enemy.SetPos(*target)
		enemy.SetFloor(fixture.state.MapState.PlayerLocation.Floor)
		enemy.SetVisible(true)
		require.True(t, fixture.state.GetCurrentLargeMapNPCAIController().GetNpcs().AddEnemy(enemy))
		require.NotNil(t, fixture.state.GetCurrentLargeMapNPCAIController().GetNpcs().
			GetVisibleEnemyAtPositionAndFloorOrNil(*target, fixture.state.MapState.PlayerLocation.Floor))
		fixture.scene.largeMapInputHandler(ebiten.KeyDown)

		fixture.advancePhases(1)

		assert.False(t, fixture.state.IsSailing())
		assert.True(t, fixture.state.HasPendingOverworldContact())
		assert.Equal(t, before, fixture.state.MapState.PlayerLocation.Position)
		assert.Equal(t, 2, fixture.elapsedMinutes(), "the source wait completes before its actor phase")
		assert.Equal(t, 99, fixture.state.PartyVehicle.GetVehicleDetails().GetHullHP())
		assert.Equal(t, map_units.SailFurled, fixture.state.PartyVehicle.GetVehicleDetails().SailState)
		assert.Zero(t, fixture.scene.surfaceCamera, "contact-only phases do not scroll")
	})
}

type automaticSailingWorkflowFixture struct {
	state     *game_state.GameState
	callbacks *game_state.MockSystemCallbacks
	scene     *GameScene
	held      map[ebiten.Key]bool
}

func newAutomaticSailingWorkflowFixture(
	t *testing.T,
	wind environment.WindDirection,
) automaticSailingWorkflowFixture {
	t.Helper()
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(50, 50).
		WithRandomSeed(458).
		WithSystemCallbacks().
		Build()
	state.DebugOptions.FreeMove = true
	state.DebugOptions.MonsterGen = false
	frigate := map_units.NewNPCFriendlyVehiceNewRef(
		references.FrigateVehicle,
		state.MapState.PlayerLocation.Position,
		state.MapState.PlayerLocation.Floor,
	)
	frigate.GetVehicleDetails().SetSailState(map_units.SailRaised)
	frigate.GetVehicleDetails().SetPartyVehicleDirection(references.Left)
	state.PartyVehicle = *frigate
	state.SetWindDirection(wind, false)
	scene := newSurfaceCompletionTestScene(state)
	held := make(map[ebiten.Key]bool)
	scene.keyboard = input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool {
		return held[key]
	})
	state.SystemCallbacks.Visual.IsAnyKeyPressed = scene.IsAnyKeyPressed
	state.SystemCallbacks.Visual.GetPressedKey = scene.GetPressedKey
	callbacks.Reset()

	return automaticSailingWorkflowFixture{state: state, callbacks: callbacks, scene: scene, held: held}
}

func (f automaticSailingWorkflowFixture) advancePhases(count int) {
	for range count {
		f.state.ElapsedMs += automaticSailingPhaseIntervalMs
		f.state.EventQueueManager.Update()
	}
}

func (f automaticSailingWorkflowFixture) elapsedMinutes() int {
	elapsed := 0
	for _, minutes := range f.callbacks.TimeAdvanced() {
		elapsed += minutes
	}

	return elapsed
}
