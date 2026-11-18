package game_state

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	beradrng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

type observingWorldController struct {
	countingNPCController
	observe func()
}

func (c *observingWorldController) AdvanceNextTurnCalcAndMoveNPCs() {
	c.turns++
	c.observe()
}

func TestWorldOrdinaryTownOrder(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithLocation(references.Britain).
		WithPlayerAt(15, 15).WithSystemCallbacks().Build()
	state.PartyState.ForEachCharacter(func(index int, character *party_state.PlayerCharacter) {
		if index != 0 {
			character.PartyMap = party_state.PartyMapUnrecruited
		}
	})
	avatar := state.PartyState.GetAvatar()
	avatar.SetHp(50)
	avatar.SetStatus(party_state.Sleep)
	avatar.SetEquippedItem(references.SlotRing, references.RingRegeneration)
	position := state.MapState.PlayerLocation.Position
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, &position, indexes.Fireplace)
	door := references.Right.GetNewPositionInDirection(&position)
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, door, indexes.RegularDoor)
	require.Equal(t, map_state.OpenDoorOpened, state.MapState.OpenDoor(references.Right))
	require.False(t, state.MapState.SmallMapProcessTurnDoors(state.isSmallMapDoorOccupied))
	require.False(t, state.MapState.SmallMapProcessTurnDoors(state.isSmallMapDoorOccupied))
	state.DateTime = datetime.UltimaDate{Year: 1, Month: 1, Day: 1, Hour: 7, Minute: 59}
	state.ActiveDurationEffect = DurationEffect{EffectType: DurationEffectProtection, TurnsRemaining: 1}
	// Find a wake success followed by fire damage and successful regeneration.
	var seed uint64
	for ; seed < 10000; seed++ {
		rng := beradrng.New(seed)
		if rng.Intn(16) == 0 {
			_ = rng.Intn(8)
			if rng.Intn(8) == 7 {
				break
			}
		}
	}
	require.Less(t, seed, uint64(10000))
	wantRNG := beradrng.New(seed)
	_ = wantRNG.Intn(16)
	wantHP := uint16(50 - (wantRNG.Intn(8) + 1) + 1)
	state.SetRandomSeed(seed)
	controller := &observingWorldController{observe: func() {
		assert.Equal(t, byte(8), state.DateTime.Hour)
		assert.Zero(t, state.DateTime.Minute)
		assert.Equal(t, party_state.Good, avatar.GetStatus())
		assert.Equal(t, wantHP, avatar.GetCurrentHp(), "wake, fire, then regeneration")
		assert.False(t, state.HasActiveDurationEffect(DurationEffectProtection))
		assert.False(t, state.MapState.HasTransientDoor(), "door closes before actors")
		assert.Equal(t, uint64(3), state.GetRNG().Calls())
	}}
	state.CurrentNPCAIController = controller

	state.CompleteSurfaceCommandCycle(references.SmallMapType)

	assert.Equal(t, 1, controller.turns)
}

func TestWorldOutdoorActorsObserveCalendarAndTimeStopExpiry(t *testing.T) {
	for _, stopped := range []bool{false, true} {
		t.Run(fmt.Sprint(stopped), func(t *testing.T) {
			state, _ := NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).
				WithFloor(0).WithPlayerAt(100, 100).WithSystemCallbacks().Build()
			state.CurrentNPCAIController.RemoveAllEnemies()
			position := state.MapState.PlayerLocation.Position
			state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, &position, indexes.Water1)
			state.DateTime = datetime.UltimaDate{Year: 1, Month: 1, Day: 1, Hour: 4, Minute: 59}
			if stopped {
				state.DateTime.Hour = 8
				state.ActiveDurationEffect = DurationEffect{EffectType: DurationEffectTimeStop, TurnsRemaining: 1}
			}
			state.DebugOptions.MonsterGen = true
			state.SetRandomSeed(387)
			before := state.DateTime

			state.CompleteSurfaceCommandCycle(references.LargeMapType)

			if stopped {
				assert.Equal(t, before, state.DateTime)
			} else {
				assert.Equal(t, byte(5), state.DateTime.Hour)
				assert.Equal(t, byte(1), state.DateTime.Minute)
			}
			assert.False(t, state.HasActiveDurationEffect(DurationEffectTimeStop))
			assert.Equal(t, uint64(1), state.GetRNG().Calls(), "daylight generation threshold only; actors run after expiry")
		})
	}
}

func TestWorldMountedCadenceContextMatrix(t *testing.T) {
	for _, location := range []references.Location{references.Britain, references.BritanniaUnderworld} {
		for _, vehicle := range []references.VehicleType{references.NoPartyVehicle, references.HorseVehicle, references.CarpetVehicle} {
			for _, pass := range []bool{false, true} {
				for _, quick := range []bool{false, true} {
					t.Run(fmt.Sprintf("%v/%v/pass=%v/quick=%v", location, vehicle, pass, quick), func(t *testing.T) {
						state := &GameState{}
						state.MapState.PlayerLocation.Location = location
						state.PartyVehicle = *map_units.NewNPCFriendlyVehiceNewRef(vehicle, references.Position{}, 0)
						if quick {
							state.ActiveDurationEffect = DurationEffect{EffectType: DurationEffectQuickness, TurnsRemaining: 20}
						}
						stride := 1
						if vehicle != references.NoPartyVehicle && !(location == references.Britain && pass) {
							stride *= 2
						}
						if quick {
							stride *= 2
						}
						for phase := 1; phase <= 8; phase++ {
							assert.Equal(t, phase%stride == 0, state.shouldProcessNonCombatWorldActorsForCommand(pass), "phase %d", phase)
						}
					})
				}
			}
		}
	}
}

func TestWorldCadenceGateOrderAndVehicleChanges(t *testing.T) {
	state := &GameState{}
	state.MapState.PlayerLocation.Location = references.Britain
	state.PartyVehicle = *map_units.NewNPCFriendlyVehiceNewRef(references.HorseVehicle, references.Position{}, 0)
	state.ActiveDurationEffect = DurationEffect{EffectType: DurationEffectTimeStop, TurnsRemaining: 3}
	assert.False(t, state.shouldProcessNonCombatWorldActors())
	assert.True(t, state.townMountedActorSkip, "town riding toggles even while time stopped")
	state.MapState.PlayerLocation.Location = references.BritanniaUnderworld
	assert.False(t, state.shouldProcessNonCombatWorldActors())
	assert.False(t, state.outdoorMountedActorSkip, "outdoor Time Stop precedes riding")
	state.ActiveDurationEffect = DurationEffect{EffectType: DurationEffectQuickness, TurnsRemaining: 3}
	assert.False(t, state.shouldProcessNonCombatWorldActors())
	assert.True(t, state.quicknessWorldActorSkip)
	assert.False(t, state.outdoorMountedActorSkip, "outdoor Quickness precedes riding")
	state.ActiveDurationEffect = DurationEffect{}
	assert.False(t, state.shouldProcessNonCombatWorldActors())
	state.PartyVehicle = *map_units.NewNPCFriendlyVehiceNewRef(references.NoPartyVehicle, references.Position{}, 0)
	assert.True(t, state.shouldProcessNonCombatWorldActors())
	state.PartyVehicle = *map_units.NewNPCFriendlyVehiceNewRef(references.CarpetVehicle, references.Position{}, 0)
	assert.True(t, state.shouldProcessNonCombatWorldActors(), "horse and carpet share phase, foot does not reset it")
	state.MapState.PlayerLocation.Location = references.Britain
	assert.True(t, state.shouldProcessNonCombatWorldActors(), "town phase survived outdoor activity")
	assert.False(t, state.townQuicknessActorSkip, "outdoor Quickness did not toggle town phase")
	state.MapState.PlayerLocation.Location = references.CombatLocation
	state.ActiveDurationEffect = DurationEffect{EffectType: DurationEffectQuickness, TurnsRemaining: 3}
	assert.False(t, state.shouldProcessNonCombatWorldActors())
	assert.True(t, state.quicknessWorldActorSkip, "combat completion cannot consume outdoor cadence")
}

func TestWorldBlockedMovementAndDoorsRetainContextRules(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithLocation(references.Britain).
		WithPlayerAt(15, 15).WithSystemCallbacks().Build()
	state.PartyVehicle = *map_units.NewNPCFriendlyVehiceNewRef(references.HorseVehicle, references.Position{}, 0)
	controller := &countingNPCController{}
	state.CurrentNPCAIController = controller
	door := references.Right.GetNewPositionInDirection(&state.MapState.PlayerLocation.Position)
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, door, indexes.RegularDoor)
	require.Equal(t, map_state.OpenDoorOpened, state.MapState.OpenDoor(references.Right))
	state.ActiveDurationEffect = DurationEffect{EffectType: DurationEffectTimeStop, TurnsRemaining: 5}
	for range 3 {
		state.CompleteSurfaceMovementCycle(references.SmallMapType, false)
	}
	assert.False(t, state.MapState.HasTransientDoor(), "doors advance even when mounted and time stopped")
	assert.Equal(t, 2, state.ActiveDurationEffect.TurnsRemaining)
	assert.Zero(t, controller.turns)
	before := state.DateTime
	state.CompleteSurfaceMovementCycle(references.LargeMapType, false)
	assert.Equal(t, before, state.DateTime)
	assert.Equal(t, 2, state.ActiveDurationEffect.TurnsRemaining)
	assert.False(t, state.outdoorMountedActorSkip)
}

func TestWorldSlowTerrainExtraPhasesPrecedeTimeAndPlayerUpdate(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).
		WithFloor(0).WithPlayerAt(100, 100).WithSystemCallbacks().Build()
	state.CurrentNPCAIController.RemoveAllEnemies()
	state.PartyVehicle = *map_units.NewNPCFriendlyVehiceNewRef(references.HorseVehicle, references.Position{}, 0)
	position := state.MapState.PlayerLocation.Position
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, &position, indexes.Swamp)
	require.Equal(t, 4, state.calculateMovementTimeCost(state.GetCurrentAvatarGroundTile()))
	state.DateTime = datetime.UltimaDate{Year: 1, Month: 1, Day: 1, Hour: 8}
	state.DebugOptions.MonsterGen = true
	state.ActiveDurationEffect = DurationEffect{EffectType: DurationEffectQuickness, TurnsRemaining: 1}
	state.SetRandomSeed(123)

	state.CompleteSurfaceMovementCycle(references.LargeMapType, true)

	assert.True(t, state.quicknessWorldActorSkip, "slow phase reached Quickness before player-update expiry")
	assert.True(t, state.outdoorMountedActorSkip, "ordinary phase reached riding after Quickness expired")
	assert.False(t, state.HasActiveDurationEffect(DurationEffectQuickness))
	assert.Equal(t, byte(2), state.DateTime.Minute, "extra addtime and ordinary addtime each use active Quickness")
	assert.Equal(t, uint32(2), state.DateTime.Turn, "two distinct addtime owners")
	assert.Zero(t, state.GetRNG().Calls(), "both actor gates skip; mounted swamp takes no exposure draws")
}

func TestWorldTownVisitResetsOnlyTownCadence(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithLocation(references.Britain).
		WithPlayerAt(15, 15).WithSystemCallbacks().Build()
	state.townMountedActorSkip = true
	state.townQuicknessActorSkip = true
	state.outdoorMountedActorSkip = true
	state.quicknessWorldActorSkip = true
	state.TransitionToLocation(references.Britain, 1, references.Position{X: 15, Y: 15}, TransitionFloorOnly, false)
	assert.True(t, state.townMountedActorSkip, "town stairs do not restart townloop")
	assert.True(t, state.townQuicknessActorSkip)
	state.TransitionToLocation(references.BritanniaUnderworld, 0, references.Position{X: 100, Y: 100}, TransitionFullLocation, false)
	state.TransitionToLocation(references.Britain, 0, references.Position{X: 15, Y: 15}, TransitionFullLocation, false)
	assert.False(t, state.townMountedActorSkip)
	assert.False(t, state.townQuicknessActorSkip)
	assert.True(t, state.outdoorMountedActorSkip)
	assert.True(t, state.quicknessWorldActorSkip)
}

func TestWorldSparseSweepsUseOccupiedEligibleOrder(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithSystemCallbacks().Build()
	var roster [party_state.NRosterSlots]*party_state.PlayerCharacter
	for _, index := range []int{0, 3, 5, 10} {
		character := *state.PartyState.GetAvatar()
		character.PartyMap = party_state.PartyMapActive
		character.SetStatus(party_state.Sleep)
		character.SetHp(50)
		character.SetEquippedItem(references.SlotRing, references.RingRegeneration)
		roster[index] = &character
	}
	roster[5].SetStatus(party_state.Dead)
	roster[10].PartyMap = party_state.PartyMapUnrecruited
	state.PartyState = *party_state.NewPartyStateFromRoster(roster)
	state.SetRandomSeed(123)
	wantRNG := beradrng.New(123)
	state.processAutoWake()
	for _, index := range []int{0, 3} {
		assert.Equal(t, wantRNG.Intn(16) == 0, !state.PartyState.GetCharacter(index).IsAsleep())
	}
	assert.Equal(t, uint64(2), state.GetRNG().Calls())
	rings := state.ProcessRegenerationRingSweep()
	require.Len(t, rings, 2)
	assert.Equal(t, []int{0, 3}, []int{rings[0].CharacterIndex, rings[1].CharacterIndex})
	for _, result := range rings {
		assert.Equal(t, wantRNG.Intn(8) == 7, result.RollSucceeded)
	}
	assert.Equal(t, uint64(4), state.GetRNG().Calls())
	result := state.earthquakeSystem.TriggerEarthquakeManually(&state.PartyState)
	require.Len(t, result.Damage, 2)
	assert.Equal(t, []int{0, 3}, []int{result.Damage[0].CharacterIndex, result.Damage[1].CharacterIndex})
	for _, damage := range result.Damage {
		assert.Equal(t, wantRNG.Intn(8)+1, damage.Damage)
	}
	assert.Equal(t, uint64(6), state.GetRNG().Calls())
}

func TestWorldMountedSwampGateDoesNotSuppressLavaOrExistingPoison(t *testing.T) {
	for _, location := range []references.Location{references.Britain, references.BritanniaUnderworld} {
		for _, vehicle := range []references.VehicleType{references.NoPartyVehicle, references.HorseVehicle, references.CarpetVehicle} {
			t.Run(fmt.Sprintf("%v/%v", location, vehicle), func(t *testing.T) {
				state, _ := NewIntegrationTestBuilder(t).WithLocation(location).WithPlayerAt(15, 15).
					WithSystemCallbacks().Build()
				state.PartyVehicle = *map_units.NewNPCFriendlyVehiceNewRef(vehicle, references.Position{}, 0)
				state.PartyState.ForEachCharacter(func(_ int, character *party_state.PlayerCharacter) {
					character.PartyMap = party_state.PartyMapUnrecruited
				})
				avatar := state.PartyState.GetAvatar()
				avatar.PartyMap = party_state.PartyMapActive
				avatar.SetStatus(party_state.Good)
				avatar.Dexterity = 0
				avatar.SetHp(50)
				position := state.MapState.PlayerLocation.Position
				state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, &position, indexes.Swamp)
				state.SetRandomSeed(123)
				state.ProcessEnvironmentalHazardsOnTurnAdvancement()
				wantCalls := uint64(0)
				if vehicle == references.NoPartyVehicle {
					wantCalls = 1
				}
				assert.Equal(t, wantCalls, state.GetRNG().Calls())
				avatar.SetStatus(party_state.Poisoned)
				state.processPoisonedCharactersForPlayerUpdate(state.cachePlayerUpdateStatuses())
				assert.Equal(t, uint16(49), avatar.GetCurrentHp())
				state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, &position, indexes.Lava)
				state.ProcessEnvironmentalHazardsOnTurnAdvancement()
				assert.Less(t, avatar.GetCurrentHp(), uint16(49))
				assert.Equal(t, wantCalls+1, state.GetRNG().Calls())
			})
		}
	}
}

func TestWorldWhirlpoolContactScansAllAndRunsOnlyBeforeMovement(t *testing.T) {
	state := newSourceHitFixture(t, references.NoPartyVehicle, 100)
	state.DebugOptions.MonsterGen = false
	state.CurrentNPCAIController.RemoveAllEnemies()
	position := state.MapState.PlayerLocation.Position
	far := references.Position{X: position.X + 5, Y: position.Y}
	near := references.Position{X: position.X + 2, Y: position.Y}
	adjacent := references.Position{X: position.X + 1, Y: position.Y}
	for _, pos := range []references.Position{far, near, adjacent} {
		state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, &pos, indexes.Water1)
	}
	require.True(t, state.spawnWhirlpoolAt(far))
	require.True(t, state.spawnWhirlpoolAt(near))
	// Enable movement on the near whirlpool and force its seek roll.
	for _, unit := range *state.CurrentNPCAIController.GetNpcs() {
		if unit.Pos() == near {
			unit.MapUnitDetails().SkipNextTurn = true
		}
	}
	var seed uint64
	for beradrng.New(seed).Intn(2) != 1 {
		seed++
	}
	state.SetRandomSeed(seed)
	state.largeMapProcessTurnEffects()
	require.NotNil(t, state.GetWhirlpoolAt(adjacent), "near whirlpool must move adjacent")
	name := outdoorWhirlpoolWorkflowName(state.GetWhirlpoolAt(adjacent).MapUnitDetails().NPCNum)
	assert.False(t, state.EventQueueManager.HasWorkflow(name))
	assert.Equal(t, uint64(1), state.GetRNG().Calls())
	state.largeMapProcessTurnEffects()
	require.True(t, state.EventQueueManager.HasWorkflow(name), "distant first whirlpool must not mask contact")
	require.NotPanics(t, state.largeMapProcessTurnEffects)
	drainSourceHitWorkflows(t, state)
	assert.Equal(t, uint64(2), state.GetRNG().Calls(), "one movement draw then exactly one hit, no duplicate contact")
	assert.Less(t, state.PartyState.GetAvatar().GetCurrentHp(), uint16(100))
}
