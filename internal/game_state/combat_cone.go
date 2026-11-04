package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

const (
	combatConeRayCount      = 21
	combatConeCandidateCap  = 63
	combatConeAdvanceRange  = 16
	combatConePixelMaximum  = 182
	combatConeLateralStep   = 10
	combatConeCenterRay     = 10
	combatConeMarkerEmpty   = indexes.SpriteIndex(0)
	combatConeMarkerVisited = indexes.BlackSquare
)

// See ref:SPELL-0003.
//
//nolint:gochecknoglobals // Immutable original spell projection rules/original game reference tables cone-ray data.
var combatConeRayDistances = [combatConeRayCount]int{
	10, 12, 14, 16, 20, 25, 35, 50, 80, 190, 2000, 190, 80, 50, 35, 25, 20, 16, 14, 12, 10,
}

// CombatConeSpec defines the terrain behavior shared by directional combat cones.
type CombatConeSpec struct {
	BlockedByTile bool
}

// CombatConeSpecForSpell returns the directional footprint configuration for a combat spell.
func CombatConeSpecForSpell(spell references.Spell) (CombatConeSpec, bool) {
	//nolint:exhaustive // Only implemented directional combat cones need a specification.
	switch spell {
	case references.InZu, references.InNoxHur, references.InFlamHur, references.InVasGravCorp:
		return CombatConeSpec{BlockedByTile: true}, true
	default:
		return CombatConeSpec{}, false
	}
}

type combatConeTileBuffer [references.XCombatMapTiles][references.YCombatMapTiles]indexes.SpriteIndex

type combatConeProjection struct {
	terrain combatConeTileBuffer
	display combatConeTileBuffer
}

type combatConeRay struct {
	x         int
	y         int
	distance  int
	rayIndex  int
	direction references.Direction
	tile      references.Position
	stopped   bool
}

func (g *GameState) tracePossibleCombatCone(
	origin references.Position,
	direction references.Direction,
	spec CombatConeSpec,
) []references.Position {
	return tracePossibleCombatCone(g.combatConeProjection().display, origin, direction, spec)
}

func tracePossibleCombatCone(
	buffer combatConeTileBuffer,
	origin references.Position,
	direction references.Direction,
	spec CombatConeSpec,
) []references.Position {
	if direction == references.NoneDirection {
		return nil
	}
	positions := make([]references.Position, 0, references.XCombatMapTiles*references.YCombatMapTiles)
	seen := make(map[references.Position]struct{}, cap(positions))
	for rayIndex := range combatConeRayCount {
		ray := newCombatConeRay(origin, direction, rayIndex)
		for !ray.stopped {
			position, plotted := ray.plot(buffer, spec.BlockedByTile)
			if !plotted {
				break
			}
			if isCombatMapPosition(position) {
				if _, found := seen[position]; !found {
					seen[position] = struct{}{}
					positions = append(positions, position)
				}
			}
			ray.advance()
		}
	}

	return positions
}

func (g *GameState) collectCommittedCombatCone(
	origin references.Position,
	direction references.Direction,
	spec CombatConeSpec,
) []references.Position {
	return collectCommittedCombatCone(
		g.combatConeProjection().display,
		origin,
		direction,
		spec,
		g.AsRandomProvider(),
	)
}

func collectCommittedCombatCone(
	buffer combatConeTileBuffer,
	origin references.Position,
	direction references.Direction,
	spec CombatConeSpec,
	rng references.RandomProvider,
) []references.Position {
	if direction == references.NoneDirection {
		return nil
	}
	rays := [combatConeRayCount]combatConeRay{}
	for rayIndex := range combatConeRayCount {
		rays[rayIndex] = newCombatConeRay(origin, direction, rayIndex)
	}
	positions := make([]references.Position, 0, combatConeCandidateCap)
	stopped := 0
	for stopped < combatConeRayCount {
		stopped = 0
		for rayIndex := range combatConeRayCount {
			ray := &rays[rayIndex]
			if ray.stopped {
				stopped++

				continue
			}
			advanceCount := rng.Intn(combatConeAdvanceRange)
			for range advanceCount {
				position, plotted := ray.plot(buffer, spec.BlockedByTile)
				if !plotted {
					break
				}
				if len(positions) < combatConeCandidateCap && isCombatMapPosition(position) &&
					buffer[position.X][position.Y] == combatConeMarkerEmpty {
					positions = append(positions, position)
					buffer[position.X][position.Y] = combatConeMarkerVisited
				}
				ray.advance()
			}
		}
	}

	return positions
}

func (g *GameState) combatConeProjection() combatConeProjection {
	if g.CombatScene == nil || g.CombatScene.CombatMapRef == nil {
		return combatConeProjection{}
	}
	projection := combatConeProjection{}
	for x := range references.XCombatMapTiles {
		for y := range references.YCombatMapTiles {
			position := references.Position{X: x, Y: y}
			projection.terrain[x][y] = g.CombatScene.CombatMapRef.GetTileNumber(position)
			projection.display[x][y] = projection.terrain[x][y]
		}
	}

	for objectSlot := sourceCombatQueueSlots - 1; objectSlot >= 0; objectSlot-- {
		g.projectCombatConeObjectSlot(&projection, encodeOriginalObjectSlot(objectSlot))
	}

	return projection
}

func (g *GameState) projectCombatConeObjectSlot(projection *combatConeProjection, objectSlot uint8) {
	for _, entry := range g.CombatScene.TurnQueue.entries {
		if entry.originalObjectSlot != objectSlot || entry.status != turnQueueActive || !entry.Combatant.IsAlive() {
			continue
		}
		identity := turnQueueIdentity(entry)
		position := g.CombatScene.GetCombatantPosition(identity)
		if !isCombatMapPosition(position) {
			return
		}
		if !identity.IsPartyMember {
			monster := g.CombatScene.MonsterManager.GetMonster(identity.EntityIndex)
			if monster == nil || monster.IsInvisible() {
				return
			}
		}
		projection.projectActor(position, identity.IsPartyMember)

		return
	}
	for _, field := range g.CombatScene.fields {
		if field.originalObjectSlot == objectSlot {
			projection.projectField(field.Position())

			return
		}
	}
	for _, object := range g.CombatScene.objects {
		if object.originalObjectSlot == objectSlot {
			projection.projectObject(object.Position())

			return
		}
	}
}

func (projection *combatConeProjection) projectActor(position references.Position, partyMember bool) {
	current := projection.display[position.X][position.Y]
	if current == indexes.BlackSquare || current == indexes.BrickWallArchway {
		return
	}
	terrain := projection.terrain[position.X][position.Y]
	//nolint:exhaustive // Only source-special terrain markers alter actor projection.
	switch terrain {
	case indexes.Forest4, indexes.SnakeSign1:
		return
	case indexes.CastleParapet8:
		projection.display[position.X][position.Y] = indexes.CastleGargoyle

		return
	case indexes.TrollBridgeHoriz, indexes.TrollBridgeVert:
		if !partyMember {
			return
		}
	}
	if partyMember && position.Y > 0 && projection.terrain[position.X][position.Y-1] == indexes.Mirror {
		projection.display[position.X][position.Y-1] = indexes.MirrorAvatar
	}
	projection.display[position.X][position.Y] = combatConeMarkerEmpty
}

func (projection *combatConeProjection) projectObject(position references.Position) {
	if !isCombatMapPosition(position) {
		return
	}
	current := projection.display[position.X][position.Y]
	if current == indexes.BlackSquare || current == indexes.BrickWallArchway {
		return
	}
	projection.display[position.X][position.Y] = combatConeMarkerEmpty
}

func (projection *combatConeProjection) projectField(position references.Position) {
	if !isCombatMapPosition(position) {
		return
	}
	current := projection.display[position.X][position.Y]
	if current == indexes.BlackSquare || current == indexes.BrickWallArchway || current <= combatConeMarkerEmpty {
		return
	}
	projection.display[position.X][position.Y] = combatConeMarkerEmpty
}

func newCombatConeRay(origin references.Position, direction references.Direction, rayIndex int) combatConeRay {
	x := int(origin.X)*originalCombatTilePixels + originalCombatWindowInset
	y := int(origin.Y)*originalCombatTilePixels + originalCombatWindowInset
	tile := origin
	//nolint:exhaustive // Callers admit only cardinal directions.
	switch direction {
	case references.Up:
		x += originalCombatWindowInset
		tile.Y--
	case references.Right:
		x += originalCombatTilePixels
		y += originalCombatWindowInset
		tile.X++
	case references.Down:
		x += originalCombatWindowInset
		y += originalCombatTilePixels
		tile.Y++
	case references.Left:
		y += originalCombatWindowInset
		tile.X--
	}

	return combatConeRay{
		x: x, y: y, distance: combatConeRayDistances[rayIndex], rayIndex: rayIndex,
		direction: direction, tile: tile, stopped: false,
	}
}

func (ray *combatConeRay) plot(buffer combatConeTileBuffer, blockedByTile bool) (references.Position, bool) {
	if ray.x < originalCombatPixelMinimum || ray.x > combatConePixelMaximum ||
		ray.y < originalCombatPixelMinimum || ray.y > combatConePixelMaximum {
		ray.stopped = true

		return references.Position{}, false
	}
	if ray.y&1 != 0 {
		ray.tile = references.Position{
			X: references.Coordinate((ray.x - originalCombatWindowInset) / originalCombatTilePixels),
			Y: references.Coordinate((ray.y - originalCombatWindowInset) / originalCombatTilePixels),
		}
		if blockedByTile && !isCombatMissilePassable(buffer[ray.tile.X][ray.tile.Y]) {
			ray.stopped = true

			return references.Position{}, false
		}
	}

	return ray.tile, true
}

func (ray *combatConeRay) advance() {
	ray.distance -= combatConeLateralStep
	//nolint:exhaustive // Rays are initialized only for cardinal directions.
	switch ray.direction {
	case references.Up:
		ray.y--
	case references.Down:
		ray.y++
	case references.Right:
		ray.x++
	case references.Left:
		ray.x--
	}
	if ray.distance >= 1 {
		return
	}
	lateral := 1
	if ray.rayIndex < combatConeCenterRay {
		lateral = -1
	}
	//nolint:exhaustive // Rays are initialized only for cardinal directions.
	switch ray.direction {
	case references.Up:
		ray.x += lateral
	case references.Down:
		ray.x -= lateral
	case references.Right:
		ray.y += lateral
	case references.Left:
		ray.y -= lateral
	}
	ray.distance += combatConeRayDistances[ray.rayIndex]
}

func (combatState *CombatState) combatConeVictimAt(
	position references.Position,
	processed map[CombatTurnIdentity]struct{},
) (CombatTurnIdentity, bool) {
	for sourceSlot := sourceCombatQueueSlots - 1; sourceSlot >= 0; sourceSlot-- {
		entry := combatState.TurnQueue.getActiveEntryAtSourceSlot(sourceSlot)
		if entry == nil {
			continue
		}
		identity := turnQueueIdentity(entry)
		if _, found := processed[identity]; found || combatState.GetCombatantPosition(identity) != position {
			continue
		}
		processed[identity] = struct{}{}

		return identity, true
	}

	return CombatTurnIdentity{}, false
}
