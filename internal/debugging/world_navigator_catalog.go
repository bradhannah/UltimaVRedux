package debugging

import (
	"encoding/json"
	"strconv"

	"github.com/bradhannah/UltimaVRedux/internal/references"
)

const (
	// CommandWorldPositionSet atomically applies an exact position or authoritative route target.
	CommandWorldPositionSet = "world.position.set"

	parameterWorldPlacement     = "placement"
	parameterWorldRoute         = "route"
	parameterWorldLocation      = "location"
	parameterWorldFloor         = "floor"
	parameterWorldRoom          = "room"
	parameterExpectedLocationID = "expectedLocationId"
	parameterExpectedFloor      = "expectedFloor"
	parameterExpectedSource     = "expectedSourceFingerprint"
	floorName                   = "Floor"
	worldRouteBritanniaLabel    = "Britannia"
	worldRouteUnderworldLabel   = "Underworld"

	// WorldPlacementExact preserves arbitrary location, floor, and coordinate setup.
	WorldPlacementExact = "exact"
	// WorldPlacementDungeonOutside resolves a dungeon entrance on a selected world plane.
	WorldPlacementDungeonOutside = "dungeon_outside_entrance"
	// WorldPlacementDungeonInsideStart resolves the selected route's fresh dungeon entry.
	WorldPlacementDungeonInsideStart = "dungeon_inside_start"
	// WorldPlacementDungeonOutsideRoom stands the party beside a fixed room, facing it, one step from entering.
	WorldPlacementDungeonOutsideRoom = "dungeon_outside_room"
	// WorldPlacementDungeonUnderworldExit resolves the floor-seven down-ladder endpoint.
	WorldPlacementDungeonUnderworldExit = "dungeon_underworld_exit"
	// WorldRouteBritannia selects the surface route.
	WorldRouteBritannia = "britannia"
	// WorldRouteUnderworld selects the Underworld route.
	WorldRouteUnderworld = "underworld"
)

func worldNavigatorCommands() []CommandDefinition {
	minimumWorldLocation, maximumWorldLocation := 0, int(references.Doom)
	minimumWorldFloor, maximumWorldFloor := -1, references.DungeonFloorCount-1
	minimumWorldCoordinate, maximumWorldCoordinate := 0, int(references.XLargeMapTiles)-1
	reviewedWorldParameters := []ParameterDefinition{
		{ID: parameterExpectedLocationID, Name: "Reviewed Location", Kind: ParameterInteger, Required: true, Minimum: &minimumWorldLocation, Maximum: &maximumWorldLocation, Availability: []string{contextGameplay, contextOutsideCombat}},
		{ID: parameterExpectedFloor, Name: "Reviewed Floor", Kind: ParameterInteger, Required: true, Minimum: &minimumWorldFloor, Maximum: &maximumWorldFloor, Availability: []string{contextGameplay, contextOutsideCombat}},
		{ID: "expectedX", Name: "Reviewed X", Kind: ParameterInteger, Required: true, Minimum: &minimumWorldCoordinate, Maximum: &maximumWorldCoordinate, Availability: []string{contextGameplay, contextOutsideCombat}},
		{ID: "expectedY", Name: "Reviewed Y", Kind: ParameterInteger, Required: true, Minimum: &minimumWorldCoordinate, Maximum: &maximumWorldCoordinate, Availability: []string{contextGameplay, contextOutsideCombat}},
		{ID: parameterExpectedSource, Name: "Reviewed Source Fingerprint", Kind: ParameterString, Required: true, Availability: []string{contextGameplay, contextOutsideCombat}},
	}

	minimumWorldRoom, maximumWorldRoom := 0, references.MaxDungeonRooms-1
	worldParameters := make([]ParameterDefinition, 0, 7+len(reviewedWorldParameters))
	worldParameters = append(worldParameters, []ParameterDefinition{
		{ID: parameterWorldPlacement, Name: "Placement", Description: "Choose exact coordinates or one authoritative dungeon route target.", Kind: ParameterEnum, Required: false, Default: json.RawMessage(`"exact"`), Choices: worldPlacementChoices(), ControlHint: controlHintSelect, Availability: []string{contextGameplay, contextOutsideCombat}},
		{ID: parameterWorldRoute, Name: "Dungeon Route", Description: "Select the Britannia or Underworld dungeon route when the placement uses one.", Kind: ParameterEnum, Required: false, Default: json.RawMessage(`"britannia"`), Choices: worldRouteChoices(), ControlHint: controlHintSelect, Availability: []string{contextGameplay, contextOutsideCombat}},
		{ID: parameterWorldLocation, Name: "Destination", Description: "Choose Britannia/Underworld, a small location, or a dungeon.", Kind: ParameterEnum, Required: true, Choices: worldLocationChoices(), ControlHint: controlHintSelect, Availability: []string{contextGameplay, contextOutsideCombat}},
		{ID: parameterWorldFloor, Name: floorName, Description: "Choose a floor valid for an exact destination.", Kind: ParameterEnum, Required: false, Default: json.RawMessage(`"0"`), Choices: worldFloorChoices(), ControlHint: controlHintSelect, Availability: []string{contextGameplay, contextOutsideCombat}},
		{ID: parameterWorldRoom, Name: "Room", Description: "Fixed room to stand beside; required for the outside-a-room placement.", Kind: ParameterInteger, Required: false, Minimum: &minimumWorldRoom, Maximum: &maximumWorldRoom, ControlHint: controlHintNumber, Availability: []string{contextGameplay, contextOutsideCombat}},
		{ID: "x", Name: "X", Kind: ParameterInteger, Required: false, Default: json.RawMessage(`0`), Minimum: &minimumWorldCoordinate, Maximum: &maximumWorldCoordinate, ControlHint: controlHintNumber, Availability: []string{contextGameplay, contextOutsideCombat}},
		{ID: "y", Name: "Y", Kind: ParameterInteger, Required: false, Default: json.RawMessage(`0`), Minimum: &minimumWorldCoordinate, Maximum: &maximumWorldCoordinate, ControlHint: controlHintNumber, Availability: []string{contextGameplay, contextOutsideCombat}},
	}...)
	worldParameters = append(worldParameters, reviewedWorldParameters...)

	return []CommandDefinition{
		{
			ID: CommandWorldPositionSet, Category: "World", Name: "Set World Position",
			Description: "Atomically apply one reviewed exact position or authoritative dungeon route target.",
			Aliases:     []string{"world navigator set", "teleport location floor"}, Parameters: worldParameters,
			ExecutionModes: []ExecutionMode{ExecutionSynchronized},
		},
	}
}

func worldPlacementChoices() []Choice {
	return []Choice{
		{Value: WorldPlacementExact, Label: "Arbitrary exact position"},
		{Value: WorldPlacementDungeonOutside, Label: "Outside dungeon entrance"},
		{Value: WorldPlacementDungeonInsideStart, Label: "Inside dungeon start"},
		{Value: WorldPlacementDungeonOutsideRoom, Label: "Outside a room"},
		{Value: WorldPlacementDungeonUnderworldExit, Label: "Dungeon Underworld-exit ladder"},
	}
}

func worldRouteChoices() []Choice {
	return []Choice{
		{Value: WorldRouteBritannia, Label: worldRouteBritanniaLabel},
		{Value: WorldRouteUnderworld, Label: worldRouteUnderworldLabel},
	}
}

func worldLocationChoices() []Choice {
	choices := make([]Choice, 0, int(references.Doom)+1)
	for location := references.BritanniaUnderworld; location <= references.Doom; location++ {
		label := location.String()
		if location == references.BritanniaUnderworld {
			label = "Britannia / Underworld"
		}
		choices = append(choices, Choice{Value: strconv.Itoa(int(location)), Label: label})
	}

	return choices
}

func worldFloorChoices() []Choice {
	choices := make([]Choice, 0, 1+references.DungeonFloorCount)
	choices = append(choices, Choice{Value: "-1", Label: "Underworld / Basement (-1)"})
	for floor := range references.DungeonFloorCount {
		choices = append(choices, Choice{
			Value: strconv.Itoa(floor),
			Label: "Floor " + strconv.Itoa(floor) + " / Dungeon Level " + strconv.Itoa(floor+1),
		})
	}

	return choices
}
