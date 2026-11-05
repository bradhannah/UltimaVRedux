package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/debugging"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

var errInvalidWorldPlacement = errors.New("invalid world placement")

func (g *GameScene) executeWorldNavigatorDebugCommand(request debugging.Request) (debugging.Result, bool) {
	if request.CommandID != debugging.CommandWorldPositionSet {
		return debugging.Result{}, false
	}

	return g.executeWorldPositionSet(request), true
}

func (g *GameScene) executeWorldPositionSet(request debugging.Request) debugging.Result {
	if result, blocked := g.worldSetupBlockedResult(request); blocked {
		return result
	}
	if !reviewedWorldPositionMatches(request, g.gameState) {
		return debugging.FailureResult(
			request,
			"stale_world_position",
			"world position changed after review",
		)
	}
	if mustDebugStringArgument(request, "expectedSourceFingerprint") != debugWorldSourceFingerprint(g.gameState) {
		return debugging.FailureResult(
			request,
			"stale_world_position",
			"world source state changed after review",
		)
	}

	placement, err := g.resolveWorldPositionPlacement(request)
	if err != nil {
		return debugging.FailureResult(request, "invalid_world_setup", err.Error())
	}
	beforeDungeon := currentDungeonAuthoritySnapshot(g.gameState)

	var (
		operation game_state.WorldSetupResult
		setupErr  error
	)
	switch {
	case placement.FreshDungeon:
		dungeonOperation, err := g.gameState.DebugSetupDungeonRoute(
			placement.Location,
			placement.Floor,
			placement.Position,
			placement.Facing,
			placement.LastDirection,
		)
		operation = dungeonOperation.WorldSetupResult
		setupErr = err
	case placement.Location.GetMapType() == references.DungeonMapType:
		current := g.gameState.MapState.PlayerLocation
		_, active := g.gameState.DungeonStateSnapshot()
		if active && current.Location == placement.Location {
			dungeonOperation, err := g.gameState.DebugSetDungeonPosition(placement.Floor, placement.Position)
			operation = dungeonOperation.WorldSetupResult
			setupErr = err
		} else {
			dungeonOperation, err := g.gameState.DebugSetupDungeonRoute(
				placement.Location,
				placement.Floor,
				placement.Position,
				references.DungeonFacingEast,
				references.DungeonLastEast,
			)
			operation = dungeonOperation.WorldSetupResult
			setupErr = err
		}
	default:
		operation, setupErr = g.gameState.DebugSetWorldPosition(
			placement.Location,
			placement.Floor,
			placement.Position,
		)
	}
	if setupErr != nil {
		return debugging.FailureResult(request, "invalid_world_setup", setupErr.Error())
	}

	afterDungeon := currentDungeonAuthoritySnapshot(g.gameState)
	status := newWorldPositionStatusData(g.gameState)
	result := debugSuccessResultWithMode(
		request,
		debugging.ExecutionSynchronized,
		worldPositionPlacementSummary(status, placement),
		status,
	)
	appendWorldSetupChanges(&result.Changes, operation)
	appendDungeonSetupChanges(&result.Changes, beforeDungeon, afterDungeon)

	return result
}

type debugWorldPositionPlacement struct {
	Placement     string
	Route         string
	Location      references.Location
	Floor         references.FloorNumber
	Position      references.Position
	Facing        references.DungeonFacing
	LastDirection references.DungeonLastDirection
	FreshDungeon  bool
}

func (g *GameScene) resolveWorldPositionPlacement(
	request debugging.Request,
) (debugWorldPositionPlacement, error) {
	placement, supplied, err := optionalDebugStringArgument(request, "placement")
	if err != nil {
		return debugWorldPositionPlacement{}, err
	}
	if !supplied {
		placement = debugging.WorldPlacementExact
	}
	location := mustDebugLocationArgument(request, "location")
	if placement == debugging.WorldPlacementExact {
		return debugWorldPositionPlacement{
			Placement: placement,
			Location:  location,
			Floor:     mustDebugFloorArgument(request, "floor", true),
			Position: references.Position{
				X: mustDebugCoordinateArgument(request, "x", int(references.XLargeMapTiles)-1),
				Y: mustDebugCoordinateArgument(request, "y", int(references.YLargeMapTiles)-1),
			},
		}, nil
	}
	if location.GetMapType() != references.DungeonMapType {
		return debugWorldPositionPlacement{}, fmt.Errorf(
			"%w: placement %q requires a dungeon destination",
			errInvalidWorldPlacement,
			placement,
		)
	}
	if placement == debugging.WorldPlacementDungeonOutsideRoom {
		return g.resolveDungeonRoomPlacement(request, location)
	}
	route, routeSupplied, routeErr := optionalDebugStringArgument(request, "route")
	if routeErr != nil {
		return debugWorldPositionPlacement{}, routeErr
	}
	if !routeSupplied {
		route = debugging.WorldRouteBritannia
	}
	for _, target := range debugDungeonPlacementTargets(g.gameState, location) {
		if target.Placement != placement || target.Route != route {
			continue
		}
		if !target.Available {
			return debugWorldPositionPlacement{}, fmt.Errorf(
				"%w: %s",
				errInvalidWorldPlacement,
				target.UnavailableReason,
			)
		}

		return debugWorldPositionPlacement{
			Placement:     placement,
			Route:         route,
			Location:      target.Location,
			Floor:         target.Floor,
			Position:      target.Position,
			Facing:        target.Facing,
			LastDirection: target.LastDirection,
			FreshDungeon:  target.Location.GetMapType() == references.DungeonMapType,
		}, nil
	}

	return debugWorldPositionPlacement{}, fmt.Errorf(
		"%w: placement %q is unavailable for %s via route %q",
		errInvalidWorldPlacement,
		placement,
		location.String(),
		route,
	)
}

func (g *GameScene) resolveDungeonRoomPlacement(
	request debugging.Request,
	location references.Location,
) (debugWorldPositionPlacement, error) {
	room, supplied, err := optionalDebugIntegerArgument(request, "room")
	if err != nil {
		return debugWorldPositionPlacement{}, err
	}
	if !supplied {
		return debugWorldPositionPlacement{}, fmt.Errorf(
			"%w: placement %q requires a room", errInvalidWorldPlacement, debugging.WorldPlacementDungeonOutsideRoom,
		)
	}
	for _, approach := range g.gameState.DungeonRoomApproaches(location) {
		if approach.Room != room {
			continue
		}
		if !approach.Available {
			return debugWorldPositionPlacement{}, fmt.Errorf("%w: %s", errInvalidWorldPlacement, approach.UnavailableReason)
		}

		return debugWorldPositionPlacement{
			Placement:     debugging.WorldPlacementDungeonOutsideRoom,
			Location:      location,
			Floor:         approach.Floor,
			Position:      approach.Approach,
			Facing:        approach.Facing,
			LastDirection: references.DungeonLastDirection(approach.Facing),
			FreshDungeon:  true,
		}, nil
	}

	return debugWorldPositionPlacement{}, fmt.Errorf(
		"%w: room %d is not placed in %s", errInvalidWorldPlacement, room, location.String(),
	)
}

func worldPositionPlacementSummary(
	status debugging.WorldPositionStatusData,
	placement debugWorldPositionPlacement,
) string {
	summary := fmt.Sprintf(
		"World position applied: %s floor %d at (%d,%d)",
		status.LocationName,
		status.Floor,
		status.X,
		status.Y,
	)
	if placement.Route != "" {
		summary += " via " + debugWorldRouteLabel(placement.Route)
	}

	return summary + "."
}

func reviewedWorldPositionMatches(request debugging.Request, state *game_state.GameState) bool {
	current := state.MapState.PlayerLocation

	return mustDebugIntegerArgument(request, "expectedLocationId") == int(current.Location) &&
		mustDebugIntegerArgument(request, "expectedFloor") == int(current.Floor) &&
		mustDebugIntegerArgument(request, "expectedX") == int(current.Position.X) &&
		mustDebugIntegerArgument(request, "expectedY") == int(current.Position.Y)
}

type dungeonAuthoritySnapshot struct {
	Active                     bool
	Snapshot                   game_state.DungeonStateSnapshot
	Lit                        bool
	TorchMinutesRemaining      int
	MagicLightMinutesRemaining int
}

type dungeonWandererSemanticData struct {
	Monster   int  `json:"monster"`
	Floor     int  `json:"floor"`
	X         int  `json:"x"`
	Y         int  `json:"y"`
	PreviousX int  `json:"previousX"`
	PreviousY int  `json:"previousY"`
	OnCeiling bool `json:"onCeiling"`
}

type worldSourceFingerprintData struct {
	Version                    int                           `json:"version"`
	LocationID                 int                           `json:"locationId"`
	Floor                      int                           `json:"floor"`
	X                          int                           `json:"x"`
	Y                          int                           `json:"y"`
	Dungeon                    *dungeonSourceFingerprintData `json:"dungeon,omitempty"`
	AtDungeonCommandBoundary   bool                          `json:"atDungeonCommandBoundary"`
	TorchMinutesRemaining      int                           `json:"torchMinutesRemaining"`
	MagicLightMinutesRemaining int                           `json:"magicLightMinutesRemaining"`
}

type dungeonSourceFingerprintData struct {
	LocationID        int                          `json:"locationId"`
	Revision          uint64                       `json:"revision"`
	FacingID          int                          `json:"facingId"`
	LastDirectionID   int                          `json:"lastDirectionId"`
	QuicknessSkipNext bool                         `json:"quicknessSkipNext"`
	Wanderer          *dungeonWandererSemanticData `json:"wanderer,omitempty"`
}

type debugDungeonPlacementTarget struct {
	Placement         string
	PlacementLabel    string
	Route             string
	RouteLabel        string
	Available         bool
	UnavailableReason string
	Location          references.Location
	Floor             references.FloorNumber
	Position          references.Position
	Facing            references.DungeonFacing
	LastDirection     references.DungeonLastDirection
}

func currentDungeonAuthoritySnapshot(state *game_state.GameState) dungeonAuthoritySnapshot {
	snapshot, active := state.DungeonStateSnapshot()

	return dungeonAuthoritySnapshot{
		Active: active, Snapshot: snapshot, Lit: state.MapState.Lighting.HasAnyLight(),
		TorchMinutesRemaining:      state.MapState.Lighting.TorchMinutesRemaining(),
		MagicLightMinutesRemaining: state.MagicLightMinutesRemaining,
	}
}

func debugWorldSourceFingerprint(state *game_state.GameState) string {
	current := state.MapState.PlayerLocation
	source := worldSourceFingerprintData{
		Version:                    1,
		LocationID:                 int(current.Location),
		Floor:                      int(current.Floor),
		X:                          int(current.Position.X),
		Y:                          int(current.Position.Y),
		Dungeon:                    nil,
		AtDungeonCommandBoundary:   state.IsAtDungeonCommandBoundary(),
		TorchMinutesRemaining:      state.MapState.Lighting.TorchMinutesRemaining(),
		MagicLightMinutesRemaining: state.MagicLightMinutesRemaining,
	}
	if snapshot, active := state.DungeonStateSnapshot(); active {
		source.Dungeon = &dungeonSourceFingerprintData{
			LocationID:        int(snapshot.Location),
			Revision:          snapshot.Revision,
			FacingID:          int(snapshot.Facing),
			LastDirectionID:   int(snapshot.LastDirection),
			QuicknessSkipNext: snapshot.QuicknessSkipNext,
			Wanderer:          dungeonWandererSemantic(snapshot.Wanderer),
		}
	}
	payload, err := json.Marshal(source)
	if err != nil {
		panic(fmt.Sprintf("encoding debug world source fingerprint: %v", err))
	}
	digest := sha256.Sum256(payload)

	return hex.EncodeToString(digest[:])
}

func dungeonWandererSemantic(wanderer *game_state.DungeonWanderer) *dungeonWandererSemanticData {
	if wanderer == nil {
		return nil
	}

	return &dungeonWandererSemanticData{
		Monster:   int(wanderer.Monster),
		Floor:     int(wanderer.Floor),
		X:         int(wanderer.Position.X),
		Y:         int(wanderer.Position.Y),
		PreviousX: int(wanderer.PreviousPosition.X),
		PreviousY: int(wanderer.PreviousPosition.Y),
		OnCeiling: wanderer.OnCeiling,
	}
}

func mustDebugEnumIntegerArgument(request debugging.Request, argumentID string) int {
	value, err := strconv.Atoi(mustDebugStringArgument(request, argumentID))
	if err != nil {
		panic(fmt.Sprintf("validated debug command %q argument %q is not an integer", request.CommandID, argumentID))
	}

	return value
}

func mustDebugLocationArgument(request debugging.Request, argumentID string) references.Location {
	value := mustDebugEnumIntegerArgument(request, argumentID)
	if value < int(references.BritanniaUnderworld) || value > int(references.Doom) {
		panic(fmt.Sprintf("validated debug command %q argument %q is outside world locations", request.CommandID, argumentID))
	}

	return references.Location(value)
}

func mustDebugFloorArgument(
	request debugging.Request,
	argumentID string,
	allowUnderworld bool,
) references.FloorNumber {
	var value int
	if allowUnderworld {
		value = mustDebugEnumIntegerArgument(request, argumentID)
	} else {
		value = mustDebugIntegerArgument(request, argumentID)
	}
	minimum := 0
	if allowUnderworld {
		minimum = -1
	}
	if value < minimum || value >= references.DungeonFloorCount {
		panic(fmt.Sprintf("validated debug command %q argument %q is outside world floors", request.CommandID, argumentID))
	}

	return references.FloorNumber(value) //nolint:gosec // G115: Checked against the complete -1..7 floor range above.
}

func mustDebugCoordinateArgument(
	request debugging.Request,
	argumentID string,
	maximum int,
) references.Coordinate {
	value := mustDebugIntegerArgument(request, argumentID)
	if value < 0 || value > maximum {
		panic(fmt.Sprintf("validated debug command %q argument %q is outside map bounds", request.CommandID, argumentID))
	}

	return references.Coordinate(value) //nolint:gosec // G115: Checked against the catalog's maximum map coordinate above.
}

func newWorldPositionStatusData(state *game_state.GameState) debugging.WorldPositionStatusData {
	current := state.MapState.PlayerLocation
	result := debugging.WorldPositionStatusData{
		SourceFingerprint: debugWorldSourceFingerprint(state),
		LocationID:        int(current.Location),
		LocationName:      current.Location.String(),
		MapType:           debugMapTypeName(current.Location.GetMapType()),
		Floor:             int(current.Floor),
		X:                 int(current.Position.X),
		Y:                 int(current.Position.Y),
		Bounds:            debugWorldPositionBounds(current.Location.GetMapType()),
		Destinations:      make([]debugging.WorldPositionDestinationData, 0, int(references.Doom)+1),
	}
	for location := references.BritanniaUnderworld; location <= references.Doom; location++ {
		result.Destinations = append(result.Destinations, debugWorldPositionDestination(state, location))
	}

	return result
}

func debugWorldPositionDestination(
	state *game_state.GameState,
	location references.Location,
) debugging.WorldPositionDestinationData {
	mapType := location.GetMapType()
	name := location.String()
	setup := debugging.WorldPositionSetupData{Floor: 0, X: 15, Y: 30}
	if location == references.BritanniaUnderworld {
		name = "Britannia / Underworld"
		setup = debugging.WorldPositionSetupData{Floor: 0, X: 80, Y: 100}
	} else if mapType == references.DungeonMapType {
		targets := debugDungeonPlacementTargets(state, location)
		for _, target := range targets {
			if target.Placement == debugging.WorldPlacementDungeonInsideStart && target.Available {
				setup = debugWorldPositionSetupData(target.Floor, target.Position)

				break
			}
		}
	}

	destination := debugging.WorldPositionDestinationData{
		LocationID:   int(location),
		LocationName: name,
		MapType:      debugMapTypeName(mapType),
		Floors:       debugWorldPositionFloors(state, location),
		Bounds:       debugWorldPositionBounds(mapType),
		DefaultSetup: setup,
	}
	if mapType == references.DungeonMapType {
		targets := debugDungeonPlacementTargets(state, location)
		destination.PlacementTargets = make([]debugging.WorldPositionPlacementTargetData, 0, len(targets))
		for _, target := range targets {
			destination.PlacementTargets = append(destination.PlacementTargets, debugWorldPlacementTargetData(target))
		}
		destination.Rooms = debugWorldPositionRooms(state, location)
	} else if location != references.BritanniaUnderworld && state.GameReferences != nil &&
		state.GameReferences.LocationReferences != nil &&
		state.GameReferences.LocationReferences.WorldLocations != nil {
		world, exists := state.GameReferences.LocationReferences.WorldLocations.LargeMapLocationPositions[location]
		if exists {
			entrance := debugging.WorldPositionSetupData{
				Floor: int(world.Floor),
				X:     int(world.Position.X),
				Y:     int(world.Position.Y),
			}
			destination.WorldEntrance = &entrance
		}
	}

	return destination
}

func debugWorldPositionRooms(
	state *game_state.GameState,
	location references.Location,
) []debugging.WorldPositionDungeonRoomData {
	if state.GameReferences == nil || state.GameReferences.LocationReferences == nil {
		return nil
	}
	approaches := state.DungeonRoomApproaches(location)
	if len(approaches) == 0 {
		return nil
	}
	dungeonIndex, _ := references.DungeonLocationToCombatIndex(location)
	rooms := make([]debugging.WorldPositionDungeonRoomData, 0, len(approaches))
	for _, approach := range approaches {
		room := debugging.WorldPositionDungeonRoomData{
			Room:              approach.Room,
			Label:             "Room " + strconv.Itoa(approach.Room),
			Floor:             int(approach.Floor),
			RoomX:             int(approach.RoomCell.X),
			RoomY:             int(approach.RoomCell.Y),
			Available:         approach.Available,
			UnavailableReason: approach.UnavailableReason,
			Summary:           debugDungeonRoomSummary(state, dungeonIndex, approach.Room),
		}
		if approach.Available {
			setup := debugWorldPositionSetupData(approach.Floor, approach.Approach)
			room.Setup = &setup
			room.FacingLabel = debugDungeonFacingLabel(approach.Facing)
		}
		rooms = append(rooms, room)
	}

	return rooms
}

// debugDungeonRoomSummary names the fixed monsters a room preview shows, such as "Ghost ×3, Insects".
func debugDungeonRoomSummary(
	state *game_state.GameState,
	dungeon references.DungeonCombatIndex,
	room int,
) string {
	preview, err := state.CombatEncounterPreview(game_state.CombatEncounterMap{
		Territory: references.DungeonCombatTerritory, Dungeon: dungeon, DungeonRoom: room,
	})
	if err != nil {
		return ""
	}
	names := make([]string, 0, len(preview.Entities))
	counts := map[string]int{}
	for _, entity := range preview.Entities {
		if entity.Kind != game_state.CombatEncounterEntityMonster &&
			entity.Kind != game_state.CombatEncounterEntityRandomMonster {
			continue
		}
		if counts[entity.Name] == 0 {
			names = append(names, entity.Name)
		}
		counts[entity.Name]++
	}
	parts := make([]string, 0, len(names))
	for _, name := range names {
		if counts[name] > 1 {
			name += " ×" + strconv.Itoa(counts[name])
		}
		parts = append(parts, name)
	}

	return strings.Join(parts, ", ")
}

func debugDungeonPlacementTargets(
	state *game_state.GameState,
	location references.Location,
) []debugDungeonPlacementTarget {
	entrance, exists := state.GetDungeonEntranceCoordinates()[location]
	if !exists {
		return nil
	}
	routes := []struct {
		ID    string
		Label string
		Floor references.FloorNumber
	}{
		{ID: debugging.WorldRouteBritannia, Label: debugWorldRouteLabel(debugging.WorldRouteBritannia), Floor: references.FloorNumber(references.OVERWORLD)},
		{ID: debugging.WorldRouteUnderworld, Label: debugWorldRouteLabel(debugging.WorldRouteUnderworld), Floor: references.FloorNumber(references.UNDERWORLD)},
	}
	targets := make([]debugDungeonPlacementTarget, 0, 5)
	for _, route := range routes {
		if !game_state.DungeonEntranceExistsOnFloor(location, route.Floor) {
			continue
		}
		targets = append(targets, debugDungeonPlacementTarget{
			Placement:      debugging.WorldPlacementDungeonOutside,
			PlacementLabel: debugWorldPlacementLabel(debugging.WorldPlacementDungeonOutside),
			Route:          route.ID,
			RouteLabel:     route.Label,
			Available:      true,
			Location:       references.BritanniaUnderworld,
			Floor:          route.Floor,
			Position:       entrance,
		})
		floor, position, facing, lastDirection := game_state.DungeonEntrySetup(location, route.Floor)
		targets = append(targets, debugDungeonPlacementTarget{
			Placement:      debugging.WorldPlacementDungeonInsideStart,
			PlacementLabel: debugWorldPlacementLabel(debugging.WorldPlacementDungeonInsideStart),
			Route:          route.ID,
			RouteLabel:     route.Label,
			Available:      true,
			Location:       location,
			Floor:          floor,
			Position:       position,
			Facing:         facing,
			LastDirection:  lastDirection,
		})
		if route.ID != debugging.WorldRouteUnderworld {
			continue
		}
		exitFloor, exitPosition, exitFacing, exitLastDirection, available :=
			game_state.DungeonUnderworldExitSetup(location)
		target := debugDungeonPlacementTarget{
			Placement:      debugging.WorldPlacementDungeonUnderworldExit,
			PlacementLabel: debugWorldPlacementLabel(debugging.WorldPlacementDungeonUnderworldExit),
			Route:          route.ID,
			RouteLabel:     route.Label,
			Available:      available,
			Location:       location,
			Floor:          exitFloor,
			Position:       exitPosition,
			Facing:         exitFacing,
			LastDirection:  exitLastDirection,
		}
		if !available {
			target.UnavailableReason = "Doom has no down-ladder Underworld endpoint; its floor-zero Up route exits to Britannia"
		}
		targets = append(targets, target)
	}

	return targets
}

func debugWorldPlacementTargetData(
	target debugDungeonPlacementTarget,
) debugging.WorldPositionPlacementTargetData {
	data := debugging.WorldPositionPlacementTargetData{
		Placement:         target.Placement,
		PlacementLabel:    target.PlacementLabel,
		Route:             target.Route,
		RouteLabel:        target.RouteLabel,
		Available:         target.Available,
		UnavailableReason: target.UnavailableReason,
		LocationID:        int(target.Location),
	}
	if !target.Available {
		return data
	}
	setup := debugWorldPositionSetupData(target.Floor, target.Position)
	data.Setup = &setup
	if target.Location.GetMapType() == references.DungeonMapType {
		data.FacingLabel = debugDungeonFacingLabel(target.Facing)
		data.LastDirectionLabel = debugDungeonLastDirectionLabel(target.LastDirection)
	}

	return data
}

func debugWorldPositionSetupData(
	floor references.FloorNumber,
	position references.Position,
) debugging.WorldPositionSetupData {
	return debugging.WorldPositionSetupData{Floor: int(floor), X: int(position.X), Y: int(position.Y)}
}

func debugWorldPlacementLabel(placement string) string {
	switch placement {
	case debugging.WorldPlacementDungeonOutside:
		return "Outside dungeon entrance"
	case debugging.WorldPlacementDungeonInsideStart:
		return "Inside dungeon start"
	case debugging.WorldPlacementDungeonOutsideRoom:
		return "Outside a room"
	case debugging.WorldPlacementDungeonUnderworldExit:
		return "Underworld exit ladder"
	default:
		panic("invalid world placement")
	}
}

func debugWorldRouteLabel(route string) string {
	switch route {
	case debugging.WorldRouteBritannia:
		return "Britannia"
	case debugging.WorldRouteUnderworld:
		return "Underworld"
	default:
		panic("invalid world route")
	}
}

func debugWorldPositionFloors(state *game_state.GameState, location references.Location) []int {
	switch location.GetMapType() {
	case references.LargeMapType:
		return []int{-1, 0}
	case references.DungeonMapType:
		floors := make([]int, references.DungeonFloorCount)
		for floor := range floors {
			floors[floor] = floor
		}

		return floors
	case references.SmallMapType:
		if state.GameReferences == nil || state.GameReferences.LocationReferences == nil {
			return []int{0}
		}
		locationReference := state.GameReferences.LocationReferences.GetLocationReference(location)
		if locationReference == nil {
			return []int{0}
		}
		minimum, maximum := locationReference.GetFloorMinMax()
		floors := make([]int, 0, int(maximum-minimum)+1)
		for floor := minimum; floor <= maximum; floor++ {
			floors = append(floors, int(floor))
		}

		return floors
	case references.CombatMapType, references.CampingMapType:
		return nil
	default:
		return nil
	}
}

func debugWorldPositionBounds(mapType references.GeneralMapType) debugging.WorldPositionBoundsData {
	maximumX, maximumY := debugMapMaxima(mapType)

	return debugging.WorldPositionBoundsData{
		MinimumX: 0,
		MaximumX: maximumX,
		MinimumY: 0,
		MaximumY: maximumY,
	}
}

func appendDungeonSetupChanges(changes *[]debugging.StateChange, before, after dungeonAuthoritySnapshot) {
	appendSemanticChange(changes, "dungeon.active", before.Active, after.Active)
	appendSemanticChange(changes, "dungeon.locationId", int(before.Snapshot.Location), int(after.Snapshot.Location))
	appendSemanticChange(changes, "dungeon.cells", flattenedDungeonCells(before), flattenedDungeonCells(after))
	appendSemanticChange(changes, "dungeon.facing", int(before.Snapshot.Facing), int(after.Snapshot.Facing))
	appendSemanticChange(
		changes,
		"dungeon.lastDirection",
		int(before.Snapshot.LastDirection),
		int(after.Snapshot.LastDirection),
	)
	appendSemanticChange(
		changes,
		"dungeon.quicknessSkipNext",
		before.Snapshot.QuicknessSkipNext,
		after.Snapshot.QuicknessSkipNext,
	)
	appendSemanticChange(
		changes,
		"dungeon.wanderer",
		dungeonWandererSemantic(before.Snapshot.Wanderer),
		dungeonWandererSemantic(after.Snapshot.Wanderer),
	)
	appendSemanticChange(changes, "dungeon.revision", before.Snapshot.Revision, after.Snapshot.Revision)
	appendSemanticChange(changes, "lighting.lit", before.Lit, after.Lit)
	appendSemanticChange(
		changes,
		"lighting.torchMinutesRemaining",
		before.TorchMinutesRemaining,
		after.TorchMinutesRemaining,
	)
	appendSemanticChange(
		changes,
		"lighting.magicLightMinutesRemaining",
		before.MagicLightMinutesRemaining,
		after.MagicLightMinutesRemaining,
	)
}

func flattenedDungeonCells(authority dungeonAuthoritySnapshot) []int {
	if !authority.Active {
		return []int{}
	}
	cells := make([]int, 0, references.DungeonCellCount)
	for floor := references.FloorNumber(0); floor < references.DungeonFloorCount; floor++ {
		for y := references.Coordinate(0); y < references.YDungeonMapTiles; y++ {
			for x := references.Coordinate(0); x < references.XDungeonMapTiles; x++ {
				cell, ok := authority.Snapshot.Cells.Cell(floor, references.Position{X: x, Y: y})
				if !ok {
					panic("active dungeon snapshot has an invalid cell coordinate")
				}
				cells = append(cells, int(cell.Raw()))
			}
		}
	}

	return cells
}

func debugDungeonFacingLabel(facing references.DungeonFacing) string {
	switch facing {
	case references.DungeonFacingNorth:
		return "North"
	case references.DungeonFacingEast:
		return "East"
	case references.DungeonFacingSouth:
		return "South"
	case references.DungeonFacingWest:
		return "West"
	default:
		panic("invalid dungeon facing")
	}
}

func debugDungeonLastDirectionLabel(direction references.DungeonLastDirection) string {
	switch direction {
	case references.DungeonLastNorth:
		return "North"
	case references.DungeonLastEast:
		return "East"
	case references.DungeonLastSouth:
		return "South"
	case references.DungeonLastWest:
		return "West"
	case references.DungeonArrivedFromBelow:
		return "Arrived from below"
	case references.DungeonArrivedFromAbove:
		return "Arrived from above"
	default:
		panic("invalid dungeon last direction")
	}
}
