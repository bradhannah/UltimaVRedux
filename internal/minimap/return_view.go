//nolint:mnd,gocyclo,funlen,lll,exhaustruct_v5,maintidx // Direct translation of verified RTTV opcodes and operands.
package minimap

import (
	"errors"
	"fmt"
	"os"

	"github.com/bradhannah/UltimaVRedux/internal/files"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

const (
	returnViewFileName      = "MISCMAPS.DAT"
	returnViewMapsOffset    = 704
	returnViewMapStoredRows = 4
	returnViewMapStoredCols = 32
	returnViewMapWidth      = 19
	returnViewMapHeight     = 4
	returnViewMapCount      = 4
	returnViewMapBytes      = returnViewMapStoredRows * returnViewMapStoredCols
	returnViewScriptOffset  = 1216
	returnViewScriptBytes   = 655
	returnViewObjectCount   = 32
	returnViewActorBank     = 256
	returnViewPlayerSprite  = 0x4c

	// ReturnViewRefreshMs is the deterministic Redux duration of one original logical refresh.
	ReturnViewRefreshMs = 56
)

var errReturnViewDataInvalid = errors.New("return to the View data is invalid")

// ReturnViewData contains validated original maps and a typed Script.
type ReturnViewData struct {
	Maps   map[FrameID]TileMap
	Script *Script
}

// LoadReturnViewData loads and decodes the original attract sequence.
func LoadReturnViewData(dataDir string) (*ReturnViewData, error) {
	path := files.Resolve(dataDir, returnViewFileName)
	data, err := os.ReadFile(path) //nolint:gosec // Configured original-data directory is user-selected.
	if err != nil {
		return nil, fmt.Errorf("reading Return to the View data %s: %w", path, err)
	}

	return DecodeReturnViewData(data)
}

// DecodeReturnViewData validates MISCMAPS.DAT and converts its original bytecode
// into the same typed script representation used by other mini-map scenes.
func DecodeReturnViewData(data []byte) (*ReturnViewData, error) {
	required := returnViewScriptOffset + returnViewScriptBytes
	if len(data) < required {
		return nil, fmt.Errorf("%w: MISCMAPS.DAT is %d bytes, need at least %d", errReturnViewDataInvalid, len(data), required)
	}
	maps := decodeReturnViewMaps(data[returnViewMapsOffset:returnViewScriptOffset])
	script, err := decodeReturnViewScript(data[returnViewScriptOffset:required])
	if err != nil {
		return nil, err
	}

	return &ReturnViewData{Maps: maps, Script: script}, nil
}

func decodeReturnViewMaps(data []byte) map[FrameID]TileMap {
	captions := [...]string{"The Summoning", "The Journey", "The Arrival", "The Welcoming"}
	maps := make(map[FrameID]TileMap, returnViewMapCount)
	for mapIndex := range returnViewMapCount {
		tiles := make([]indexes.SpriteIndex, 0, returnViewMapWidth*returnViewMapHeight)
		mapOffset := mapIndex * returnViewMapBytes
		for row := range returnViewMapHeight {
			rowOffset := mapOffset + row*returnViewMapStoredCols
			for column := range returnViewMapWidth {
				tiles = append(tiles, indexes.SpriteIndex(data[rowOffset+column]))
			}
		}
		id := returnViewMapID(mapIndex)
		maps[id] = TileMap{Width: returnViewMapWidth, Height: returnViewMapHeight, Tiles: tiles, Caption: captions[mapIndex]}
	}

	return maps
}

//nolint:cyclop,gocognit // Direct, bounds-checked translation of the original 16-opcode RTTV stream.
func decodeReturnViewScript(data []byte) (*Script, error) {
	builder := NewMiniMapScript("return_to_the_view")
	commands := make([]TileCommand, 0, len(data)/2)
	position := 0
	loopStart, loopCount := -1, 0
	loopMovements := make([]returnViewMovement, 0)
	actorPositions := make(map[byte]references.Position, returnViewObjectCount)
	gatePosition := references.Position{}
	hasGatePosition := false
	for position < len(data) {
		opcode := data[position]
		position++
		switch opcode {
		case 0x00: // ADD object, tile, x, y
			operands, next, err := returnViewOperands(data, position, 4)
			if err != nil {
				return nil, err
			}
			position = next
			if err := validateReturnViewActor(operands[0]); err != nil {
				return nil, err
			}
			if err := validateReturnViewPosition(operands[2], operands[3]); err != nil {
				return nil, err
			}
			actorPositions[operands[0]] = returnViewPosition(operands[2], operands[3])
			commands = append(commands, returnViewActorCommand(TileCommandAddActor, operands[0], operands[1], operands[2], operands[3]))
			if hasGatePosition && operands[1] == returnViewPlayerSprite && actorPositions[operands[0]] == gatePosition {
				// REDUX DEVIATION: The original adds the Avatar at the raised gate without a fade (ref:INTRO-0005).
				// Redux uses the requested deterministic pixel fizz to make arrival visibly emerge from the gate.
				commands = append(commands, returnViewEffectCommand(
					TileCommandFizzActor, references.Position{}, returnViewActorID(operands[0]), 12,
				))
			}
		case 0x01: // DEL object
			operands, next, err := returnViewOperands(data, position, 1)
			if err != nil {
				return nil, err
			}
			position = next
			if err := validateReturnViewActor(operands[0]); err != nil {
				return nil, err
			}
			actorID := returnViewActorID(operands[0])
			command := TileCommand{Type: TileCommandRemoveActor, ActorID: actorID}
			if actorPosition, ok := actorPositions[operands[0]]; ok && hasGatePosition && actorPosition == gatePosition {
				// REDUX DEVIATION: The original deletes the Avatar immediately after walking to the moongate (ref:INTRO-0004).
				// Redux uses the requested deterministic pixel fizz so gate travel has visible absorption feedback.
				command = returnViewEffectCommand(TileCommandFadeActor, references.Position{}, actorID, 12)
			}
			commands = append(commands, command)
			delete(actorPositions, operands[0])
		case 0x02, 0x0d: // MOV/WLK object, direction
			operands, next, err := returnViewOperands(data, position, 2)
			if err != nil {
				return nil, err
			}
			position = next
			if err := validateReturnViewActor(operands[0]); err != nil {
				return nil, err
			}
			direction, err := returnViewDirection(operands[1])
			if err != nil {
				return nil, err
			}
			commands = append(commands, TileCommand{Type: TileCommandMoveActor, ActorID: returnViewActorID(operands[0]), Direction: direction})
			if actorPosition, ok := actorPositions[operands[0]]; ok {
				actorPositions[operands[0]] = moveReturnViewPosition(actorPosition, direction)
			}
			if loopStart >= 0 {
				loopMovements = append(loopMovements, returnViewMovement{actor: operands[0], direction: direction})
			}
			if opcode == 0x0d {
				commands = append(commands, returnViewRefreshCommand(7))
			}
		case 0x03: // REF count
			operands, next, err := returnViewOperands(data, position, 1)
			if err != nil {
				return nil, err
			}
			position = next
			commands = append(commands, returnViewRefreshCommand(max(1, int(operands[0]))))
		case 0x04: // RAI x, y
			operands, next, err := returnViewOperands(data, position, 2)
			if err != nil {
				return nil, err
			}
			position = next
			if err := validateReturnViewPosition(operands[0], operands[1]); err != nil {
				return nil, err
			}
			gatePosition = returnViewPosition(operands[0], operands[1])
			hasGatePosition = true
			commands = append(commands, returnViewEffectCommand(TileCommandRaiseGate, gatePosition, "", 17))
		case 0x05: // DRO uses latest RAI coordinates
			if !hasGatePosition {
				return nil, fmt.Errorf("%w: DRO at byte %d precedes RAI", errReturnViewDataInvalid, position-1)
			}
			commands = append(commands, returnViewEffectCommand(TileCommandDropGate, gatePosition, "", 17))
		case 0x06: // MAP map
			operands, next, err := returnViewOperands(data, position, 1)
			if err != nil {
				return nil, err
			}
			position = next
			if operands[0] >= returnViewMapCount {
				return nil, fmt.Errorf("%w: map index %d is out of range", errReturnViewDataInvalid, operands[0])
			}
			hasGatePosition = false
			commands = append(commands, TileCommand{Type: TileCommandSetMap, MapID: returnViewMapID(int(operands[0]))})
		case 0x07, 0x08: // FIZ/FAD object
			operands, next, err := returnViewOperands(data, position, 1)
			if err != nil {
				return nil, err
			}
			position = next
			if err := validateReturnViewActor(operands[0]); err != nil {
				return nil, err
			}
			commandType := TileCommandFizzActor
			if opcode == 0x08 {
				commandType = TileCommandFadeActor
			}
			commands = append(commands, returnViewEffectCommand(commandType, references.Position{}, returnViewActorID(operands[0]), 12))
		case 0x09: // RST
			commands = append(commands, TileCommand{Type: TileCommandReset})
		case 0x0a: // STO tile, x, y
			operands, next, err := returnViewOperands(data, position, 3)
			if err != nil {
				return nil, err
			}
			position = next
			if err := validateReturnViewPosition(operands[1], operands[2]); err != nil {
				return nil, err
			}
			commands = append(commands, TileCommand{
				Type: TileCommandSetTile, Sprite: indexes.SpriteIndex(operands[0]), Position: returnViewPosition(operands[1], operands[2]),
			})
		case 0x0b: // NUK ignored-source, target
			operands, next, err := returnViewOperands(data, position, 2)
			if err != nil {
				return nil, err
			}
			position = next
			if err := validateReturnViewActor(operands[1]); err != nil {
				return nil, err
			}
			commands = append(commands, returnViewEffectCommand(TileCommandNukActor, references.Position{}, returnViewActorID(operands[1]), 8))
		case 0x0c: // NEW
			clear(actorPositions)
			commands = append(commands, TileCommand{Type: TileCommandClearActors})
		case 0x0e: // LUP count
			if loopStart >= 0 {
				return nil, fmt.Errorf("%w: nested LUP at byte %d", errReturnViewDataInvalid, position-1)
			}
			operands, next, err := returnViewOperands(data, position, 1)
			if err != nil {
				return nil, err
			}
			position = next
			loopStart, loopCount = len(commands), int(operands[0])
			loopMovements = loopMovements[:0]
		case 0x0f: // END loop
			if loopStart < 0 || loopCount <= 0 {
				return nil, fmt.Errorf("%w: END at byte %d has no active loop", errReturnViewDataInvalid, position-1)
			}
			body := append([]TileCommand(nil), commands[loopStart:]...)
			for range loopCount - 1 {
				commands = append(commands, body...)
			}
			for range loopCount - 1 {
				for _, movement := range loopMovements {
					if actorPosition, ok := actorPositions[movement.actor]; ok {
						actorPositions[movement.actor] = moveReturnViewPosition(actorPosition, movement.direction)
					}
				}
			}
			loopStart, loopCount = -1, 0
			loopMovements = loopMovements[:0]
		default:
			return nil, fmt.Errorf("%w: unknown opcode 0x%02x at byte %d", errReturnViewDataInvalid, opcode, position-1)
		}
	}
	if loopStart >= 0 {
		return nil, fmt.Errorf("%w: unterminated LUP", errReturnViewDataInvalid)
	}
	if len(commands) == 0 || commands[len(commands)-1].Type != TileCommandReset {
		return nil, fmt.Errorf("%w: script does not end in RST", errReturnViewDataInvalid)
	}
	for _, command := range commands {
		builder.TileCommand(command)
	}
	script := builder.Build()
	if err := script.Validate(); err != nil {
		return nil, fmt.Errorf("%w: decoded script: %w", errReturnViewDataInvalid, err)
	}

	return script, nil
}

type returnViewMovement struct {
	actor     byte
	direction references.Direction
}

func moveReturnViewPosition(position references.Position, direction references.Direction) references.Position {
	switch direction {
	case references.Up:
		position.Y--
	case references.Right:
		position.X++
	case references.Down:
		position.Y++
	case references.Left:
		position.X--
	case references.NoneDirection:
	}

	return position
}

func returnViewOperands(data []byte, position, count int) ([]byte, int, error) {
	if position+count > len(data) {
		return nil, position, fmt.Errorf("%w: opcode at byte %d has truncated operands", errReturnViewDataInvalid, position-1)
	}

	return data[position : position+count], position + count, nil
}

func returnViewActorCommand(commandType TileCommandType, actor, sprite, x, y byte) TileCommand {
	return TileCommand{
		Type: commandType, ActorID: returnViewActorID(actor),
		Sprite: indexes.SpriteIndex(sprite) + returnViewActorBank, Position: returnViewPosition(x, y),
	}
}

func returnViewRefreshCommand(cycles int) TileCommand {
	return TileCommand{Type: TileCommandRefresh, Cycles: cycles, CycleMs: ReturnViewRefreshMs}
}

func returnViewEffectCommand(
	commandType TileCommandType,
	position references.Position,
	actorID ActorID,
	cycles int,
) TileCommand {
	return TileCommand{
		Type: commandType, ActorID: actorID, Position: position,
		Cycles: cycles, CycleMs: ReturnViewRefreshMs, DurationMs: cycles * ReturnViewRefreshMs,
	}
}

func returnViewDirection(value byte) (references.Direction, error) {
	switch value {
	case 0:
		return references.Up, nil
	case 1:
		return references.Right, nil
	case 2:
		return references.Down, nil
	case 3:
		return references.Left, nil
	default:
		return references.NoneDirection, fmt.Errorf("%w: direction %d is out of range", errReturnViewDataInvalid, value)
	}
}

func returnViewMapID(index int) FrameID {
	return FrameID(fmt.Sprintf("return-view-%d", index))
}

func returnViewActorID(index byte) ActorID {
	return ActorID(fmt.Sprintf("object-%02d", index))
}

func returnViewPosition(x, y byte) references.Position {
	return references.Position{X: references.Coordinate(x), Y: references.Coordinate(y)}
}

func validateReturnViewActor(actor byte) error {
	if actor >= returnViewObjectCount {
		return fmt.Errorf("%w: object index %d is out of range", errReturnViewDataInvalid, actor)
	}

	return nil
}

func validateReturnViewPosition(x, y byte) error {
	if x >= returnViewMapWidth || y >= returnViewMapHeight {
		return fmt.Errorf("%w: position (%d,%d) is outside the 19x4 map", errReturnViewDataInvalid, x, y)
	}

	return nil
}
