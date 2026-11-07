package game_state

import (
	"errors"
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	rng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

// ErrInvalidWorldSetup indicates that a debug world transition is unsupported or invalid.
var ErrInvalidWorldSetup = errors.New("invalid world setup")

const (
	codexWorldX                = 233
	codexWorldY                = 233
	lordBritishSecretRoomFloor = 2
	lordBritishSecretRoomX     = 17
	lordBritishSecretRoomY     = 12
)

// WorldSetupResult reports exact state surrounding an immediate debug setup transition.
type WorldSetupResult struct {
	Before        references.PlayerLocation
	After         references.PlayerLocation
	SmashedBefore int
	SmashedAfter  int
}

func (g *GameState) currentWorldSetupResult() WorldSetupResult {
	location := g.MapState.PlayerLocation

	return WorldSetupResult{
		Before: location, After: location,
		SmashedBefore: g.smashed, SmashedAfter: g.smashed,
	}
}

// DungeonSetupSemanticState is the exact debug-visible dungeon authority around a setup.
type DungeonSetupSemanticState struct {
	Active bool
	DungeonStateSnapshot
	Lit bool
}

// DungeonSetupResult reports exact world and dungeon state around a debug setup.
type DungeonSetupResult struct {
	WorldSetupResult
	BeforeDungeon DungeonSetupSemanticState
	AfterDungeon  DungeonSetupSemanticState
}

// ValidateDebugMerchantLocationTransition verifies a remote Merchant Tester
// launch without changing world, session, callback, time, or RNG state.
func (g *GameState) ValidateDebugMerchantLocationTransition(id references.MerchantID) error {
	_, err := g.validateDebugMerchantLocationTransition(id)

	return err
}

func (g *GameState) validateDebugMerchantLocationTransition(
	id references.MerchantID,
) (references.MerchantDefinition, error) {
	if g == nil || g.GameReferences == nil || g.GameReferences.MerchantReferences == nil ||
		g.GameReferences.LocationReferences == nil {
		return references.MerchantDefinition{}, fmt.Errorf(
			"%w: merchant destination references are unavailable",
			ErrInvalidWorldSetup,
		)
	}
	if err := g.validateStableWorldSetupContext(); err != nil {
		return references.MerchantDefinition{}, err
	}
	merchant, ok := g.GameReferences.MerchantReferences.Definition(id)
	if !ok {
		return references.MerchantDefinition{}, fmt.Errorf(
			"%w: unknown merchant ID %q",
			ErrInvalidWorldSetup,
			id,
		)
	}
	if merchant.Location.GetMapType() != references.SmallMapType {
		return references.MerchantDefinition{}, fmt.Errorf(
			"%w: merchant %q destination is not a small map",
			ErrInvalidWorldSetup,
			id,
		)
	}
	locationReference := g.GameReferences.LocationReferences.GetLocationReference(merchant.Location)
	if locationReference == nil || locationReference.Location != merchant.Location {
		return references.MerchantDefinition{}, fmt.Errorf(
			"%w: merchant %q has no canonical destination location",
			ErrInvalidWorldSetup,
			id,
		)
	}
	minimumFloor, maximumFloor := locationReference.GetFloorMinMax()
	if smallMapStartingPositionFloor < minimumFloor || smallMapStartingPositionFloor > maximumFloor {
		return references.MerchantDefinition{}, fmt.Errorf(
			"%w: merchant %q destination has no target floor %d",
			ErrInvalidWorldSetup,
			id,
			smallMapStartingPositionFloor,
		)
	}
	npcReferences := locationReference.GetNPCReferences()
	if npcReferences == nil || merchant.NPCIndex < 0 || merchant.NPCIndex >= len(*npcReferences) {
		return references.MerchantDefinition{}, fmt.Errorf(
			"%w: merchant %q has no canonical NPC reference",
			ErrInvalidWorldSetup,
			id,
		)
	}
	npcReference := (*npcReferences)[merchant.NPCIndex]
	resolved, resolveErr := g.GameReferences.MerchantReferences.Resolve(
		references.DialogNumber(npcReference.DialogNumber),
		npcReference.Location,
		merchant.NPCIndex,
	)
	if resolveErr != nil || resolved != merchant {
		return references.MerchantDefinition{}, fmt.Errorf(
			"%w: canonical NPC reference does not match merchant %q",
			ErrInvalidWorldSetup,
			id,
		)
	}
	if npcReference.GetNPCType() == 0 ||
		(npcReference.Schedule.X[0] == 0 && npcReference.Schedule.Y[0] == 0) {
		return references.MerchantDefinition{}, fmt.Errorf(
			"%w: merchant %q canonical NPC reference is empty",
			ErrInvalidWorldSetup,
			id,
		)
	}
	if g.PartyState.IsNPCDead(merchant.Location, merchant.NPCIndex) {
		return references.MerchantDefinition{}, fmt.Errorf(
			"%w: merchant %q is dead",
			ErrInvalidWorldSetup,
			id,
		)
	}
	desiredSlot := npcReference.Schedule.DesiredSlot(g.DateTime)
	if floor := npcReference.Schedule.GetIndividualNPCBehaviourBySlot(desiredSlot).Floor; floor != smallMapStartingPositionFloor {
		return references.MerchantDefinition{}, fmt.Errorf(
			"%w: merchant %q is on floor %d rather than target floor %d at the current game time",
			ErrInvalidWorldSetup,
			id,
			floor,
			smallMapStartingPositionFloor,
		)
	}
	if buyer, _ := g.merchantPricingCharacter(); buyer == nil {
		return references.MerchantDefinition{}, fmt.Errorf(
			"%w: %w",
			ErrInvalidWorldSetup,
			ErrMerchantBuyerUnavailable,
		)
	}

	return merchant, nil
}

// DebugEnterMerchantLocation loads the canonical merchant town through normal
// transition/controller authority while preserving the gameplay RNG stream.
func (g *GameState) DebugEnterMerchantLocation(id references.MerchantID) (WorldSetupResult, error) {
	result := g.currentWorldSetupResult()
	merchant, err := g.validateDebugMerchantLocationTransition(id)
	if err != nil {
		return result, err
	}

	originalRNG := g.rng
	if originalRNG != nil {
		g.rng = rng.Restore(originalRNG.Seed(), originalRNG.Calls())
		defer func() { g.rng = originalRNG }()
	}
	g.TransitionToLocation(
		merchant.Location,
		smallMapStartingPositionFloor,
		references.Position{X: smallMapStartingPositionX, Y: smallMapStartingPositionY},
		TransitionFullLocation,
		true,
	)
	result.After = g.MapState.PlayerLocation
	result.SmashedAfter = g.smashed

	return result, nil
}

// DebugSetWorldPosition atomically selects one non-dungeon world location,
// exact floor, and coordinate without simulating travel time.
func (g *GameState) DebugSetWorldPosition(
	location references.Location,
	floor references.FloorNumber,
	position references.Position,
) (WorldSetupResult, error) {
	return g.debugSetWorldPosition(location, floor, position, location.GetMapType() == references.SmallMapType)
}

func (g *GameState) debugSetWorldPosition(
	location references.Location,
	floor references.FloorNumber,
	position references.Position,
	saveLastLargeMap bool,
) (WorldSetupResult, error) {
	result := g.currentWorldSetupResult()
	if err := g.validateWorldSetupContext(); err != nil {
		return result, err
	}
	minimumFloor, maximumFloor, err := g.debugFloorRangeForLocation(location)
	if err != nil {
		return result, err
	}
	if floor < minimumFloor || floor > maximumFloor {
		return result, fmt.Errorf(
			"%w: floor %d outside %d..%d for %s",
			ErrInvalidWorldSetup,
			floor,
			minimumFloor,
			maximumFloor,
			location.String(),
		)
	}
	width, height, err := debugWorldDimensions(location)
	if err != nil {
		return result, err
	}
	if position.X < 0 || position.X >= width || position.Y < 0 || position.Y >= height {
		return result, fmt.Errorf(
			"%w: position (%d,%d) outside 0..%d,0..%d for %s",
			ErrInvalidWorldSetup,
			position.X,
			position.Y,
			width-1,
			height-1,
			location.String(),
		)
	}

	originalRNG := g.rng
	if originalRNG != nil {
		g.rng = rng.Restore(originalRNG.Seed(), originalRNG.Calls())
		defer func() { g.rng = originalRNG }()
	}
	g.TransitionToLocation(location, floor, position, TransitionFullLocation, saveLastLargeMap)
	result.After = g.MapState.PlayerLocation
	result.SmashedAfter = g.smashed

	return result, nil
}

// DebugSetupDungeonRoute installs a fresh production dungeon visit with route
// orientation while preserving the party's carried torch and magic-light time.
func (g *GameState) DebugSetupDungeonRoute(
	location references.Location,
	floor references.FloorNumber,
	position references.Position,
	facing references.DungeonFacing,
	lastDirection references.DungeonLastDirection,
) (DungeonSetupResult, error) {
	return g.debugSetupDungeon(location, floor, position, facing, lastDirection, nil)
}

func (g *GameState) debugSetupDungeon(
	location references.Location,
	floor references.FloorNumber,
	position references.Position,
	facing references.DungeonFacing,
	lastDirection references.DungeonLastDirection,
	lit *bool,
) (DungeonSetupResult, error) {
	beforeDungeon := g.debugDungeonSetupSemanticState()
	result := DungeonSetupResult{
		WorldSetupResult: g.currentWorldSetupResult(),
		BeforeDungeon:    beforeDungeon,
		AfterDungeon:     beforeDungeon,
	}
	if err := g.validateWorldSetupContext(); err != nil {
		return result, err
	}
	if location < references.Deceit || location > references.Doom {
		return result, fmt.Errorf("%w: location %d is not a supported dungeon", ErrInvalidWorldSetup, location)
	}
	if !references.IsDungeonCoordinate(floor, position) {
		return result, fmt.Errorf(
			"%w: dungeon coordinate floor %d at (%d,%d) outside floor 0..%d and position 0..%d,0..%d",
			ErrInvalidWorldSetup,
			floor,
			position.X,
			position.Y,
			references.DungeonFloorCount-1,
			references.XDungeonMapTiles-1,
			references.YDungeonMapTiles-1,
		)
	}
	if !facing.IsValid() {
		return result, fmt.Errorf("%w: invalid dungeon facing %d", ErrInvalidWorldSetup, facing)
	}
	if !lastDirection.IsValid() {
		return result, fmt.Errorf("%w: invalid dungeon last direction %d", ErrInvalidWorldSetup, lastDirection)
	}
	prepared, err := g.PrepareDungeonState(location, facing, lastDirection)
	if err != nil {
		return result, fmt.Errorf("%w: preparing %s: %w", ErrInvalidWorldSetup, location.String(), err)
	}
	if err = validateDungeonDoorwaySetup(prepared, floor, position, facing); err != nil {
		return result, fmt.Errorf("%w: %w", ErrInvalidWorldSetup, err)
	}

	g.installPreparedDungeonState(prepared)
	g.TransitionToLocation(location, floor, position, TransitionFullLocation, false)
	if lit != nil {
		g.MapState.Lighting.ExtinguishTorch()
		g.MagicLightMinutesRemaining = 0
		if *lit {
			g.MagicLightMinutesRemaining = VasLorLightDuration
		}
		g.MapState.Lighting.SetMagicLight(*lit)
	}
	result.After = g.MapState.PlayerLocation
	result.SmashedAfter = g.smashed
	result.AfterDungeon = g.debugDungeonSetupSemanticState()

	return result, nil
}

func validateDungeonDoorwaySetup(
	dungeon *DungeonState,
	floor references.FloorNumber,
	position references.Position,
	facing references.DungeonFacing,
) error {
	cell, ok := dungeon.Cell(floor, position)
	if !ok {
		return errInvalidDungeonCell
	}
	if cell.Kind() != references.DungeonRevealedDoor {
		return nil
	}
	for _, direction := range []references.Direction{
		facing.Direction(),
		facing.Direction().GetOppositeDirection(),
	} {
		target := direction.GetNewPositionInDirection(&position).GetWrapped(
			references.XDungeonMapTiles,
			references.YDungeonMapTiles,
		)
		targetCell, valid := dungeon.Cell(floor, *target)
		if valid && targetCell.Raw() != 0x83 && dungeonCellAllowsMovement(targetCell.Kind(), DungeonAdvance) {
			return nil
		}
	}

	return errors.New("selected doorway facing has no traversable forward or reverse route")
}

// DebugSetDungeonPosition atomically changes floor and position within the
// active dungeon while preserving its semantic cells, facing, and lighting.
func (g *GameState) DebugSetDungeonPosition(
	floor references.FloorNumber,
	position references.Position,
) (DungeonSetupResult, error) {
	before := g.MapState.PlayerLocation
	beforeDungeon := g.debugDungeonSetupSemanticState()
	result := DungeonSetupResult{
		WorldSetupResult: g.currentWorldSetupResult(),
		BeforeDungeon:    beforeDungeon,
		AfterDungeon:     beforeDungeon,
	}
	if err := g.validateWorldSetupContext(); err != nil {
		return result, err
	}
	if before.Location.GetMapType() != references.DungeonMapType || !beforeDungeon.Active {
		return result, fmt.Errorf("%w: an active dungeon is required", ErrInvalidWorldSetup)
	}
	if !references.IsDungeonCoordinate(floor, position) {
		return result, fmt.Errorf(
			"%w: dungeon coordinate floor %d at (%d,%d) outside floor 0..%d and position 0..%d,0..%d",
			ErrInvalidWorldSetup,
			floor,
			position.X,
			position.Y,
			references.DungeonFloorCount-1,
			references.XDungeonMapTiles-1,
			references.YDungeonMapTiles-1,
		)
	}
	if floor == before.Floor && position == before.Position {
		return result, nil
	}

	transitionType := TransitionFullLocation
	if floor == before.Floor {
		transitionType = TransitionPositionOnly
	}
	g.TransitionToLocation(before.Location, floor, position, transitionType, false)
	result.After = g.MapState.PlayerLocation
	result.SmashedAfter = g.smashed
	result.AfterDungeon = g.debugDungeonSetupSemanticState()

	return result, nil
}

func (g *GameState) debugDungeonSetupSemanticState() DungeonSetupSemanticState {
	state := DungeonSetupSemanticState{Lit: g.MapState.Lighting.HasAnyLight()}
	if g.activeDungeon == nil || g.MapState.PlayerLocation.Location.GetMapType() != references.DungeonMapType ||
		g.activeDungeon.location != g.MapState.PlayerLocation.Location {
		return state
	}
	snapshot := g.activeDungeon.Snapshot()
	state.Active = true
	state.DungeonStateSnapshot = snapshot

	return state
}

// DebugTravelToLandmark transitions to a curated physical landmark
// without simulating travel time.
func (g *GameState) DebugTravelToLandmark(name string) (WorldSetupResult, error) {
	result := g.currentWorldSetupResult()
	if err := g.validateWorldSetupContext(); err != nil {
		return result, err
	}
	destination, err := debugLandmarkDestination(name)
	if err != nil {
		return result, err
	}

	return g.debugSetWorldPosition(
		destination.Location,
		destination.Floor,
		destination.Position,
		destination.Location.GetMapType() == references.SmallMapType,
	)
}

func (g *GameState) validateWorldSetupContext() error {
	if err := g.validateStableWorldSetupContext(); err != nil {
		return err
	}
	if g.PartyVehicle.GetVehicleDetails().VehicleType != references.NoPartyVehicle {
		return fmt.Errorf("%w: exit the current vehicle before changing world setup", ErrInvalidWorldSetup)
	}

	return nil
}

func (g *GameState) validateStableWorldSetupContext() error {
	if g.activeDungeonCommand != nil {
		return fmt.Errorf("%w: a dungeon command cycle is active", ErrInvalidWorldSetup)
	}
	if g.activeDungeon.hasPendingRoomContinuation() {
		return fmt.Errorf("%w: a dungeon room continuation is pending", ErrInvalidWorldSetup)
	}
	if g.IsInCombat() {
		return fmt.Errorf("%w: unavailable during combat", ErrInvalidWorldSetup)
	}
	switch g.MapState.PlayerLocation.Location.GetMapType() {
	case references.LargeMapType, references.SmallMapType:
		return nil
	case references.DungeonMapType:
		if g.activeDungeon == nil || g.activeDungeon.location != g.MapState.PlayerLocation.Location {
			return fmt.Errorf("%w: active dungeon state does not match the current location", ErrInvalidWorldSetup)
		}

		return nil
	case references.CombatMapType, references.CampingMapType:
		// These contexts cannot accept stable world setup changes.
	}

	return fmt.Errorf("%w: unavailable in the current map context", ErrInvalidWorldSetup)
}

func (g *GameState) debugFloorRangeForLocation(
	location references.Location,
) (references.FloorNumber, references.FloorNumber, error) {
	switch {
	case location == references.BritanniaUnderworld:
		return -1, 0, nil
	case location < references.Moonglow || location > references.SerpentsHold:
		return 0, 0, fmt.Errorf("%w: location %d is not a supported world map", ErrInvalidWorldSetup, location)
	case g.GameReferences == nil || g.GameReferences.LocationReferences == nil:
		return 0, 0, fmt.Errorf("%w: floor range is unavailable", ErrInvalidWorldSetup)
	}
	locationReference := g.GameReferences.LocationReferences.GetLocationReference(location)
	if locationReference == nil {
		return 0, 0, fmt.Errorf("%w: location %d is not loaded", ErrInvalidWorldSetup, location)
	}
	minimum, maximum := locationReference.GetFloorMinMax()

	return minimum, maximum, nil
}

func debugWorldDimensions(location references.Location) (references.Coordinate, references.Coordinate, error) {
	switch {
	case location == references.BritanniaUnderworld:
		return references.XLargeMapTiles, references.YLargeMapTiles, nil
	case location >= references.Moonglow && location <= references.SerpentsHold:
		return references.XSmallMapTiles, references.YSmallMapTiles, nil
	default:
		return 0, 0, fmt.Errorf("%w: location %d is not a supported world map", ErrInvalidWorldSetup, location)
	}
}

func debugLandmarkDestination(name string) (references.PlayerLocation, error) {
	switch name {
	case "lord_british_secret_room":
		return references.PlayerLocation{
			Location: references.LordBritishsCastle,
			Floor:    lordBritishSecretRoomFloor,
			Position: references.Position{X: lordBritishSecretRoomX, Y: lordBritishSecretRoomY},
		}, nil
	case "flame_truth":
		return debugFlameLandmarkDestination(references.FlameOfTruth), nil
	case "flame_love":
		return debugFlameLandmarkDestination(references.FlameOfLove), nil
	case "flame_courage":
		return debugFlameLandmarkDestination(references.FlameOfCourage), nil
	}
	if name == "codex" {
		return references.PlayerLocation{
			Location: references.BritanniaUnderworld,
			Floor:    0,
			Position: references.Position{X: codexWorldX, Y: codexWorldY},
		}, nil
	}
	shrine := GetShrineByName(name)
	if shrine == nil {
		return references.PlayerLocation{}, fmt.Errorf("%w: unknown landmark %q", ErrInvalidWorldSetup, name)
	}
	if shrine.Index == 6 {
		return references.PlayerLocation{}, fmt.Errorf(
			"%w: Shrine of Spirituality requires the midnight moongate",
			ErrInvalidWorldSetup,
		)
	}

	return references.PlayerLocation{
		Location: references.BritanniaUnderworld,
		Floor:    0,
		Position: references.Position{X: references.Coordinate(shrine.WorldX), Y: references.Coordinate(shrine.WorldY)},
	}, nil
}

func debugFlameLandmarkDestination(flame references.FlameOfPrinciple) references.PlayerLocation {
	location := references.FlameLocations[flame]

	return references.PlayerLocation{
		Location: location.Location,
		Floor:    references.FloorNumber(location.Floor),
		Position: references.Position{X: references.Coordinate(location.X), Y: references.Coordinate(location.Y)},
	}
}
