package ai

import (
	"fmt"
	"image"
	"slices"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/pkg/helpers"
)

// WindProvider interface to avoid circular dependency with game_state
type WindProvider interface {
	IsWindCalm() bool
	GetPirateShipSpeedMultiplier(shipDirection references.Direction) int
}

const (
	coinFlipOdds                    = 2
	pathfindingRadius               = 15 // tiles around the player searched by A* for enemy movement
	stalledShipSpeed                = 4  // wind speed multiplier at which a pirate ship cannot move
	speedWithWind                   = 1
	speedAgainstWind                = 2
	speedCrosswind                  = 3
	randomMovementOdds              = 4
	directedPursuitWaitOdds         = 4
	maxCardinalDirectionIndex       = 3
	oneInThreeMaxRoll               = 2
	withWindMaxRoll                 = 3
	playerwardAxisCount             = 2
	viewportCenterDivisor           = 2
	spawnViewportBuffer             = 2
	monsterSpawnPositionTries       = 10
	minimumMonsterSpawnAxisDistance = 7
)

// NPCAIControllerLargeMap moves and spawns enemies and NPCs on the overworld or underworld map.
type NPCAIControllerLargeMap struct {
	World           references.World
	tileRefs        *references.Tiles
	mapState        *map_state.MapState
	debugOptions    *references.DebugOptions
	enemyReferences *references.EnemyReferences
	theOdds         *references.TheOdds
	dateTime        *datetime.UltimaDate
	windProvider    WindProvider
	rng             references.RandomProvider

	mapUnits map_units.MapUnits
}

// NewNPCAIControllerLargeMapInput carries the shared game state a large-map controller reads and mutates.
type NewNPCAIControllerLargeMapInput struct {
	World           references.World
	TileRefs        *references.Tiles
	MapState        *map_state.MapState
	DebugOptions    *references.DebugOptions
	TheOdds         *references.TheOdds
	EnemyReferences *references.EnemyReferences
	DateTime        *datetime.UltimaDate
	WindProvider    WindProvider
	RandomProvider  references.RandomProvider
}

// NewNPCAIControllerLargeMap builds a controller; it panics without a random provider.
func NewNPCAIControllerLargeMap(input NewNPCAIControllerLargeMapInput) *NPCAIControllerLargeMap {
	if input.RandomProvider == nil {
		panic("large-map NPC AI requires a random provider")
	}

	return &NPCAIControllerLargeMap{
		World:           input.World,
		tileRefs:        input.TileRefs,
		mapState:        input.MapState,
		debugOptions:    input.DebugOptions,
		enemyReferences: input.EnemyReferences,
		theOdds:         input.TheOdds,
		dateTime:        input.DateTime,
		windProvider:    input.WindProvider,
		rng:             input.RandomProvider,
		mapUnits:        make(map_units.MapUnits, 0, map_units.MaximumNpcsPerMap),
	}
}

func (m *NPCAIControllerLargeMap) oneInXOdds(odds int) bool {
	return m.rng.Intn(odds) == 0
}

func (m *NPCAIControllerLargeMap) randomIntInRange(minimum, maximum int) int {
	if minimum > maximum {
		panic("minimum cannot be greater than maximum")
	}

	return m.rng.Intn(maximum-minimum+1) + minimum
}

func (m *NPCAIControllerLargeMap) pickOneOf(first, second int) int {
	if m.oneInXOdds(coinFlipOdds) {
		return first
	}

	return second
}

// GetNpcs returns the live unit list, which callers may mutate.
func (m *NPCAIControllerLargeMap) GetNpcs() *map_units.MapUnits {
	return &m.mapUnits
}

// PopulateMapFirstLoad is a no-op: large-map units are populated by spawning and saved state.
func (m *NPCAIControllerLargeMap) PopulateMapFirstLoad() {
}

func (m *NPCAIControllerLargeMap) placeNPCsOnLayeredMap() {
	layeredMap := m.mapState.GetLayeredMapByCurrentLocation()

	for _, mu := range m.mapUnits {
		if !mu.IsVisible() || m.mapState.PlayerLocation.Floor != mu.Floor() {
			continue
		}
		switch npc := mu.(type) {
		case *map_units.NPCEnemy:
			layeredMap.SetTileByLayer(map_state.MapUnitLayer, npc.PosPtr(), npc.GetKeySpriteIndex())
		case *map_units.NPCFriendly:
			layeredMap.SetTileByLayer(map_state.MapUnitLayer, npc.PosPtr(), npc.NPCReference.GetSpriteIndex())
		}
	}
}

// AdvanceNextTurnCalcAndMoveNPCs runs one NPC turn where no enemy attacks.
func (m *NPCAIControllerLargeMap) AdvanceNextTurnCalcAndMoveNPCs() {
	m.AdvanceNextTurnWithActorCheck(func(*map_units.NPCEnemy) OutdoorActorAction { return OutdoorActorNoAttack })
}

// OutdoorActorAction distinguishes a resolved attack from a workflow-owned
// mutation that must finish before evaluating the next descending source slot.
type OutdoorActorAction uint8

// Outdoor actor outcomes preserve the distinction between attack and suspension.
const (
	OutdoorActorNoAttack OutdoorActorAction = iota
	OutdoorActorAttacked
	OutdoorActorDeferred
)

// AdvanceNextTurnWithActorCheck preserves the original descending traversal (ref:ENEMY-0006) of
// object slots. Each attack check precedes that actor's movement; an attack
// suppresses all subsequent movement, but never subsequent attack checks.
func (m *NPCAIControllerLargeMap) AdvanceNextTurnWithActorCheck(attack func(*map_units.NPCEnemy) OutdoorActorAction) {
	if len(m.mapUnits) < map_units.MaximumNpcsPerMap && m.shouldGenerateTileBasedMonster() {
		m.generateTileBasedMonster()
	}
	m.advanceActorsBeforeSlot(map_units.MaximumNpcsPerMap, false, attack)
}

// ContinueTurnAfterAttack resumes an already-started phase after its deferred
// attack commits. The caller selects the current world's controller, so no
// actor pointers from a departed map survive a transport. No generation or
// cadence gates repeat, and the preceding attack still suppresses movement.
func (m *NPCAIControllerLargeMap) ContinueTurnAfterAttack(
	beforeSlot int,
	attack func(*map_units.NPCEnemy) OutdoorActorAction,
) {
	m.advanceActorsBeforeSlot(beforeSlot, true, attack)
}

func (m *NPCAIControllerLargeMap) advanceActorsBeforeSlot(
	beforeSlot int,
	attacked bool,
	attack func(*map_units.NPCEnemy) OutdoorActorAction,
) {
	actors := m.mapUnits.GetEnemiesMatching(func(enemy *map_units.NPCEnemy) bool {
		return enemy.MapUnitDetails().NPCNum < beforeSlot
	})
	slices.SortFunc(actors, func(left, right *map_units.NPCEnemy) int {
		return right.MapUnitDetails().NPCNum - left.MapUnitDetails().NPCNum
	})
	for _, npc := range actors {
		if !npc.IsVisible() || npc.Floor() != m.mapState.PlayerLocation.Floor {
			continue
		}
		m.FreshenExistingNPCsOnMap()
		switch attack(npc) {
		case OutdoorActorNoAttack:
		case OutdoorActorAttacked:
			attacked = true
		case OutdoorActorDeferred:
			return
		}
		if !attacked {
			m.calculateNextNPCPosition(npc)
		}
	}
	m.cleanupDistantEnemies()
	m.FreshenExistingNPCsOnMap()
}

// cleanupDistantEnemies removes transient hostile encounters outside the
// active area while preserving friendly vehicles wherever the player leaves them.
func (m *NPCAIControllerLargeMap) cleanupDistantEnemies() {
	m.mapUnits = helpers.FilterFromSlice(m.mapUnits, func(unit map_units.MapUnit) bool {
		if unit.IsEmptyMapUnit() {
			return false
		}

		if _, hostile := unit.(*map_units.NPCEnemy); !hostile {
			return true
		}

		dx, dy := unit.PosPtr().GetWrappedAxisDistances(m.mapState.PlayerLocation.Position)

		return dx+dy <= maxTileDistanceBeforeCleanup
	})
}

func (m *NPCAIControllerLargeMap) calculateNextNPCPosition(mapUnit map_units.MapUnit) {
	enemy, ok := mapUnit.(*map_units.NPCEnemy)
	if !ok {
		// Friendly units do not currently move in the large maps (ie. Frigates)
		return
	}

	// Respect per-enemy immobility flags (e.g., Reapers, Mimics, fields)
	if enemy.EnemyReference.DoNotMove {
		return
	}

	if mapUnit.PosPtr().IsNextTo(m.mapState.PlayerLocation.Position, mapUnit.Floor(), m.mapState.PlayerLocation.Floor) {
		// if the NPC is next to the player, we don't want to move them
		return
	}

	// Handle special whirlpool movement patterns BEFORE terrain checks
	// Whirlpools have their own movement rules (50% skip, 25% random, 25% seek)
	// and should bypass standard terrain-based movement probability
	if m.handleWhirlpoolMovement(mapUnit) {
		return
	}

	// Handle pirate ship movement with turn-then-move rules (like player frigate)
	// Pirates must rotate to face movement direction before actually moving
	if m.isPirateShip(mapUnit) {
		if m.shouldPirateShipMoveThisTick(mapUnit) {
			m.handlePirateShipMovement(mapUnit)
		}

		return
	}

	// Redux policy: Bat and Mongbat use ordinary pursuit. The original repulsion
	// branch says "mongbat" but tests shape 0xfc (Shadowlord), not Mongbat's 0xf0.
	// Do not treat that ambiguity as a fidelity rule; ref:ENEMY-0005.
	// REDUX DEVIATION: Original movement normally attempts pursuit every phase,
	// which chooses a playerward axis and uses random movement only when blocked.
	// Redux chooses random movement on one in four phases, then waits on one in
	// four directed opportunities so A* does not increase encounter pressure by
	// routing around obstacles.
	if m.oneInXOdds(randomMovementOdds) {
		m.moveRandomDirection(mapUnit)

		return
	}
	if m.oneInXOdds(directedPursuitWaitOdds) {
		return
	}
	m.setBestNextPositionToMoveTowardsWalkablePoint(enemy)
}

// setBestNextPositionToMoveTowardsWalkablePoint searches only a window around
// the player. The window uses unwrapped coordinates, so a route may cross the
// world's wrapping edge; positions wrap only to read the map.
func (m *NPCAIControllerLargeMap) setBestNextPositionToMoveTowardsWalkablePoint(enemy *map_units.NPCEnemy) {
	player := m.mapState.PlayerLocation.Position
	layeredMap := m.mapState.GetLayeredMapByCurrentLocation()
	occupied := m.enemyMovementOccupiedPositions(enemy)
	window := image.Rect(
		int(player.X)-pathfindingRadius, int(player.Y)-pathfindingRadius,
		int(player.X)+pathfindingRadius, int(player.Y)+pathfindingRadius)
	path := references.FindRoute(nearestLargeMapCopy(enemy.Pos(), player), player, window,
		func(position references.Position) int {
			position.GetWrapped(references.XLargeMapTiles, references.YLargeMapTiles)
			if slices.Contains(occupied, position) ||
				!enemy.EnemyReference.CanMoveToTile(layeredMap.GetTopTile(&position)) {
				return -1
			}

			return 1
		})
	if len(path) <= 1 {
		// without a route, at least try to get them closer to the avatar using basic pathing
		m.setBestNextPositionToMoveTowardsWalkablePointDumb(enemy)

		return
	}
	next := path[1]
	m.tryMoveEnemyTo(enemy, *next.GetWrapped(references.XLargeMapTiles, references.YLargeMapTiles))
}

// nearestLargeMapCopy moves position by whole map sizes to the copy closest to
// anchor, so a position just across the wrapping edge counts as nearby.
func nearestLargeMapCopy(position, anchor references.Position) references.Position {
	return references.Position{
		X: nearestWrappedCoordinate(position.X, anchor.X, references.XLargeMapTiles),
		Y: nearestWrappedCoordinate(position.Y, anchor.Y, references.YLargeMapTiles),
	}
}

func nearestWrappedCoordinate(value, anchor, size references.Coordinate) references.Coordinate {
	switch {
	case value-anchor > size/2:
		return value - size
	case anchor-value > size/2:
		return value + size
	default:
		return value
	}
}

func (m *NPCAIControllerLargeMap) setBestNextPositionToMoveTowardsWalkablePointDumb(mapUnit map_units.MapUnit) {
	allDirections := mapUnit.PosPtr().GetFourDirectionsWrapped(references.XLargeMapTiles, references.YLargeMapTiles)
	// getting the current distance to the player will make sure they never move further away
	playerPos := &m.mapState.PlayerLocation.Position
	fCurrentShortestDistance := mapUnit.PosPtr().GetWrappedDistanceBetweenWrapped(
		playerPos, references.XLargeMapTiles, references.YLargeMapTiles)
	bestPos := *mapUnit.PosPtr()
	bFound := false
	for _, newPos := range allDirections {
		fNewDistance := newPos.GetWrappedDistanceBetweenWrapped(
			playerPos, references.XLargeMapTiles, references.YLargeMapTiles)

		if fNewDistance < fCurrentShortestDistance {
			if !m.canEnemyMoveTo(mapUnit, newPos) {
				continue
			}

			bestPos = newPos
			fCurrentShortestDistance = fNewDistance
			bFound = true
		}
	}

	if !bFound {
		// if we don't find a new position, we don't try to move
		return
	}
	m.tryMoveEnemyTo(mapUnit, bestPos)
}

// moveRandomDirection attempts bounded random cardinal movement; ref:ENEMY-0005.
func (m *NPCAIControllerLargeMap) moveRandomDirection(mapUnit map_units.MapUnit) {
	allDirections := mapUnit.PosPtr().GetFourDirectionsWrapped(references.XLargeMapTiles, references.YLargeMapTiles)

	// Try up to 3 random directions like the original
	for range 3 {
		direction := m.randomIntInRange(0, maxCardinalDirectionIndex)
		newPos := allDirections[direction]
		enemy, ok := mapUnit.(*map_units.NPCEnemy)
		if !ok || !m.canEnemyMoveTo(mapUnit, newPos) {
			continue
		}
		if m.shouldEnemyMoveTo(enemy, newPos) {
			mapUnit.SetPos(newPos)
		}

		return
	}
	// If no valid random direction found, don't move
}

// FreshenExistingNPCsOnMap redraws every visible unit on the current map layer.
func (m *NPCAIControllerLargeMap) FreshenExistingNPCsOnMap() {
	m.mapState.GetLayeredMapByCurrentLocation().ClearMapUnitTiles()
	m.placeNPCsOnLayeredMap()
}

func (m *NPCAIControllerLargeMap) generateTileBasedMonster() {
	var deltaX, deltaY references.Coordinate
	xDistanceAway := m.mapState.XTilesVisibleOnGameScreen/viewportCenterDivisor + spawnViewportBuffer
	yDistanceAway := m.mapState.YTilesVisibleOnGameScreen/viewportCenterDivisor + spawnViewportBuffer

	// Probability checking now handled in shouldGenerateTileBasedMonster()
	// This method only executes when a monster should actually spawn

	for range monsterSpawnPositionTries {
		if m.oneInXOdds(coinFlipOdds) { // do deltaY
			//nolint:gosec // G115: Viewport dimensions are bounded during initialization.
			deltaY = references.Coordinate(m.pickOneOf(yDistanceAway, -yDistanceAway))

			deltaX = m.randomSignedAxisDistance(minimumMonsterSpawnAxisDistance, xDistanceAway)
		} else { // do deltaX
			deltaY = m.randomSignedAxisDistance(minimumMonsterSpawnAxisDistance, yDistanceAway)
			//nolint:gosec // G115: Viewport dimensions are bounded during initialization.
			deltaX = references.Coordinate(m.pickOneOf(xDistanceAway, -xDistanceAway))
		}

		playerPos := m.mapState.PlayerLocation.Position
		pos := references.Position{X: playerPos.X + deltaX, Y: playerPos.Y + deltaY}
		pos = *pos.GetWrapped(references.XLargeMapTiles, references.YLargeMapTiles)
		if pos.X < 0 || pos.Y < 0 {
			panic(fmt.Sprintf("unexpected negative position X=%d Y=%d", pos.X, pos.Y))
		}

		tile := m.mapState.GetLayeredMapByCurrentLocation().GetTopTile(&pos)

		// Select a creature using terrain-specific candidate gates; ref:ENEMY-0004.
		monsterType := references.PickMonsterByTerrain(tile, m.mapState.IsUnderworld(), m.rng)
		if monsterType == references.MonsterNone {
			continue
		}

		var enemy *references.EnemyReference
		if monsterType == references.MonsterStocks && !m.mapState.IsUnderworld() {
			if !tile.IsBoatPassable() {
				continue
			}
			direction := m.calculatePirateSeekDirections(pos, playerPos)[0]
			pirate := references.NewPirateShipEnemyReference(
				m.tileRefs.GetTile(references.PirateShipSprite(direction)),
			)
			enemy = &pirate
		} else {
			enemy = m.lookupEnemyByMonsterType(monsterType)
			if enemy == nil {
				continue
			}
		}

		npc := map_units.NewEnemyNPC(*enemy, 0)

		npc.SetPos(pos)
		npc.SetFloor(m.mapState.PlayerLocation.Floor)
		m.mapUnits.AddEnemy(npc)

		return
	}
}

func (m *NPCAIControllerLargeMap) randomSignedAxisDistance(minimum, maximum int) references.Coordinate {
	distance := m.randomIntInRange(minimum, maximum)

	//nolint:gosec // Viewport-derived distances are bounded far below int16 limits.
	return references.Coordinate(m.pickOneOf(distance, -distance))
}

func (m *NPCAIControllerLargeMap) shouldGenerateTileBasedMonster() bool {
	if !m.debugOptions.MonsterGen {
		return false
	}

	// The original attempt rule reads map terrain (ref:ENEMY-0004), independent of a vehicle or
	// other object projected at the party's square. Preserve terrain overrides.
	playerTile := m.mapState.GetLayeredMapByCurrentLocation().GetTileTopMapOnlyTile(&m.mapState.PlayerLocation.Position)
	probability := references.CalculateEncounterProbability(
		playerTile,
		references.IsOriginalMonsterSpawnNightBonusHour(m.dateTime.Hour),
		m.mapState.IsUnderworld(),
	)
	roll := m.randomIntInRange(1, m.theOdds.GetMonsterGenerationRollMaximum())

	return probability > roll
}

func (m *NPCAIControllerLargeMap) tryMoveEnemyTo(mapUnit map_units.MapUnit, destination references.Position) {
	enemy, ok := mapUnit.(*map_units.NPCEnemy)
	if !ok {
		return
	}
	if !m.canEnemyMoveTo(mapUnit, destination) ||
		!m.shouldEnemyMoveTo(enemy, destination) {
		return
	}

	mapUnit.SetPos(destination)
}

// canEnemyMoveTo applies creature terrain admission (ref:ENEMY-0002) to the underlying
// terrain, while objects block independently even for ethereal creatures.
func (m *NPCAIControllerLargeMap) canEnemyMoveTo(
	mapUnit map_units.MapUnit,
	destination references.Position,
) bool {
	enemy, ok := mapUnit.(*map_units.NPCEnemy)
	if !ok {
		return false
	}
	layeredMap := m.mapState.GetLayeredMapByCurrentLocation()
	if !enemy.EnemyReference.CanMoveToTile(layeredMap.GetTileTopMapOnlyTile(&destination)) {
		return false
	}
	if destination == m.mapState.PlayerLocation.Position ||
		layeredMap.SpriteIndexByLayer(map_state.MapUnitLayer, destination) > 0 {
		return false
	}
	for _, unit := range m.mapUnits {
		if unit.IsVisible() && !unit.IsEmptyMapUnit() && unit.Floor() == mapUnit.Floor() &&
			unit.Pos() == destination {
			return false
		}
	}

	return true
}

func (m *NPCAIControllerLargeMap) enemyMovementOccupiedPositions(mapUnit map_units.MapUnit) []references.Position {
	positions := make([]references.Position, 0, len(m.mapUnits))
	for _, unit := range m.mapUnits {
		if unit.IsVisible() && !unit.IsEmptyMapUnit() && unit.Floor() == mapUnit.Floor() &&
			unit.Pos() != mapUnit.Pos() {
			positions = append(positions, unit.Pos())
		}
	}

	return positions
}

// shouldEnemyMoveTo applies original slowdown (ref:ENEMY-0005) from the destination
// terrain. Map units occupy the top layer while movement is calculated, so the
// terrain-only lookup is required here.
func (m *NPCAIControllerLargeMap) shouldEnemyMoveTo(
	enemy *map_units.NPCEnemy,
	destination references.Position,
) bool {
	switch enemy.EnemyReference.MonsterType { //nolint:exhaustive // Only source-defined slowdown exemptions differ.
	case references.MonsterDragon, references.MonsterBat, references.MonsterDevil, references.MonsterMongbat:
		return true
	default:
	}

	tile := m.mapState.GetLayeredMapByCurrentLocation().GetTileTopMapOnlyTile(&destination)
	if tile == nil {
		return false
	}
	switch tile.Index { //nolint:exhaustive // Only source-defined slowdown terrain needs cases.
	case indexes.Swamp, indexes.Forest1, indexes.Desert1, indexes.Forest2,
		indexes.LeftDesert2, indexes.RightDesert2:
		return m.randomIntInRange(0, 1) == 0
	case indexes.Forest3, indexes.Forest4, indexes.Hills1, indexes.SmallMountains,
		indexes.TallMountains, indexes.LeftHills2, indexes.RightHills2:
		return m.randomIntInRange(0, oneInThreeMaxRoll) == oneInThreeMaxRoll
	default:
		return true
	}
}

// RemoveAllEnemies satisfies NPCAIController by clearing hostile units.
func (m *NPCAIControllerLargeMap) RemoveAllEnemies() {
	m.ClearEnemies()
}

// ClearEnemies removes hostile units while preserving friendly NPCs and vehicles.
func (m *NPCAIControllerLargeMap) ClearEnemies() int {
	removed := m.mapUnits.RemoveAllEnemies()
	m.FreshenExistingNPCsOnMap()

	return removed
}

// lookupEnemyByMonsterType finds the EnemyReference for a given MonsterType.
// Returns nil if no matching enemy exists in the loaded references.
func (m *NPCAIControllerLargeMap) lookupEnemyByMonsterType(mt references.MonsterType) *references.EnemyReference {
	for i := range *m.enemyReferences {
		if (*m.enemyReferences)[i].MonsterType == mt {
			return &(*m.enemyReferences)[i]
		}
	}

	return nil
}

// isPirateShip checks if the given map unit is a pirate ship
func (m *NPCAIControllerLargeMap) isPirateShip(mapUnit map_units.MapUnit) bool {
	// Check if this is an NPCEnemy that might represent a pirate ship
	if enemy, ok := mapUnit.(*map_units.NPCEnemy); ok {
		// Get the tile at the enemy's position to check for pirate ship sprites
		currentTile := m.mapState.GetLayeredMapByCurrentLocation().GetTopTile(enemy.PosPtr())

		// Check if the tile shows a pirate ship sprite
		return references.IsPirateShipSprite(currentTile.Index)
	}

	return false
}

// shouldPirateShipMoveThisTick applies wind-based pirate sailing cadence; ref:SHIP-0001.
func (m *NPCAIControllerLargeMap) shouldPirateShipMoveThisTick(mapUnit map_units.MapUnit) bool {
	// If no wind provider available, fall back to normal movement
	if m.windProvider == nil {
		return true
	}

	// Calm winds stop all pirate ship movement
	if m.windProvider.IsWindCalm() {
		return false
	}

	// Get ship direction from sprite
	shipDirection := m.getPirateShipDirection(mapUnit)
	if shipDirection == references.NoneDirection {
		return false // Invalid ship direction
	}

	// Get wind speed multiplier from wind provider
	speedMultiplier := m.windProvider.GetPirateShipSpeedMultiplier(shipDirection)

	// Speed multiplier 4 = stalled (don't move)
	if speedMultiplier >= stalledShipSpeed {
		return false
	}

	// The original speed matrix makes crosswind fastest, with-wind next, and
	// against-wind slowest. GetPirateShipSpeedMultiplier returns 1, 2, or 3
	// for those relative wind states rather than the matrix's 3, 2, or 4.
	return m.shouldPirateShipMoveAtSpeed(speedMultiplier)
}

func (m *NPCAIControllerLargeMap) shouldPirateShipMoveAtSpeed(speedMultiplier int) bool {
	switch speedMultiplier {
	case speedWithWind:
		return m.randomIntInRange(0, withWindMaxRoll) != 0 // 75% with the wind
	case speedAgainstWind:
		return m.randomIntInRange(0, 1) == 0 // 50% against the wind
	case speedCrosswind:
		return true // Crosswind is unrestricted
	default:
		return false // Stalled or unknown
	}
}

// getPirateShipDirection determines the direction a pirate ship is facing based on its sprite
func (m *NPCAIControllerLargeMap) getPirateShipDirection(mapUnit map_units.MapUnit) references.Direction {
	if enemy, ok := mapUnit.(*map_units.NPCEnemy); ok && enemy.Variant() == map_units.EnemyVariantPirate {
		return enemy.Facing()
	}

	// Default direction if not a pirate ship or unable to determine
	return references.Up
}

// rotatePirateShip updates the pirate ship's sprite to face the given direction
// This is used when a pirate ship needs to turn before moving
func (m *NPCAIControllerLargeMap) rotatePirateShip(mapUnit map_units.MapUnit, newDirection references.Direction) {
	enemy, ok := mapUnit.(*map_units.NPCEnemy)
	if !ok {
		return
	}

	newSprite := references.PirateShipSprite(newDirection)
	newTile := m.tileRefs.GetTile(newSprite)
	enemy.SetFacing(newDirection, newTile)

	logging.Debugf(logging.CategoryAI, "Pirate ship at %v rotated to face %s",
		enemy.Pos(), newDirection.GetDirectionCompassName())
}

// calculatePirateSeekDirections returns the two playerward axes in randomized
// order, matching original pursuit before its random fallback; ref:ENEMY-0005.
func (m *NPCAIControllerLargeMap) calculatePirateSeekDirections(
	piratePos, playerPos references.Position,
) []references.Direction {
	dx := wrappedLargeMapDelta(piratePos.X, playerPos.X)
	dy := wrappedLargeMapDelta(piratePos.Y, playerPos.Y)
	xDirection := directionForDelta(dx, references.Right, references.Left)
	yDirection := directionForDelta(dy, references.Down, references.Up)

	if !m.oneInXOdds(coinFlipOdds) {
		xDirection, yDirection = yDirection, xDirection
	}

	return nonzeroDirections(xDirection, yDirection)
}

func wrappedLargeMapDelta(from, to references.Coordinate) int {
	delta := int(to) - int(from)
	mapWidth := int(references.XLargeMapTiles)
	halfMap := mapWidth / playerwardAxisCount
	if delta > halfMap {
		delta -= mapWidth
	} else if delta < -halfMap {
		delta += mapWidth
	}

	return delta
}

func directionForDelta(
	delta int,
	positiveDirection, negativeDirection references.Direction,
) references.Direction {
	if delta > 0 {
		return positiveDirection
	}
	if delta < 0 {
		return negativeDirection
	}

	return references.NoneDirection
}

func nonzeroDirections(candidates ...references.Direction) []references.Direction {
	directions := make([]references.Direction, 0, playerwardAxisCount)
	for _, direction := range candidates {
		if direction != references.NoneDirection {
			directions = append(directions, direction)
		}
	}

	return directions
}

// handlePirateShipMovement tries both playerward axes (ref:ENEMY-0005),
// then fall back to random legal movement when coast or another object blocks them.
func (m *NPCAIControllerLargeMap) handlePirateShipMovement(mapUnit map_units.MapUnit) {
	piratePos := *mapUnit.PosPtr()
	playerPos := m.mapState.PlayerLocation.Position

	for _, direction := range m.calculatePirateSeekDirections(piratePos, playerPos) {
		if m.movePirateShipInDirection(mapUnit, direction) {
			return
		}
	}

	for range 3 {
		if m.movePirateShipInDirection(mapUnit, m.getRandomPirateDirection()) {
			return
		}
	}
}

// getRandomPirateDirection returns a random cardinal direction for pirate random movement
func (m *NPCAIControllerLargeMap) getRandomPirateDirection() references.Direction {
	directions := []references.Direction{references.Up, references.Right, references.Down, references.Left}

	return directions[m.randomIntInRange(0, maxCardinalDirectionIndex)]
}

// movePirateShipInDirection moves and turns a pirate in one action when the
// destination is valid frigate water, as in the original movement rule; ref:ENEMY-0002.
func (m *NPCAIControllerLargeMap) movePirateShipInDirection(
	mapUnit map_units.MapUnit,
	direction references.Direction,
) bool {
	currentPos := *mapUnit.PosPtr()
	newPos := direction.GetNewPositionInDirection(&currentPos)
	wrappedPos := *newPos.GetWrapped(references.XLargeMapTiles, references.YLargeMapTiles)

	terrain := m.mapState.GetLayeredMapByCurrentLocation().GetTileTopMapOnlyTile(&wrappedPos)
	if !pirateShipCanMoveToTile(terrain) || !m.canEnemyMoveTo(mapUnit, wrappedPos) {
		logging.Debugf(logging.CategoryAI, "Pirate ship at %v blocked moving %s (tile not passable)",
			currentPos, direction.GetDirectionCompassName())

		return false
	}

	m.rotatePirateShip(mapUnit, direction)
	mapUnit.SetPos(wrappedPos)
	logging.Debugf(logging.CategoryAI, "Pirate ship moved from %v to %v (direction: %s)",
		currentPos, wrappedPos, direction.GetDirectionCompassName())

	return true
}

func pirateShipCanMoveToTile(tile *references.Tile) bool {
	return tile != nil && tile.IsBoatPassable()
}

// handleWhirlpoolMovement applies alternating whirlpool movement; ref:SHIP-0003.
// Whirlpools alternate moving and skipping; this is persistent cadence, not a random gate.
// When moving: 50% random movement, 50% seek toward player
func (m *NPCAIControllerLargeMap) handleWhirlpoolMovement(mapUnit map_units.MapUnit) bool {
	// Get the enemy to check for whirlpool movement flag
	enemy, ok := mapUnit.(*map_units.NPCEnemy)
	if !ok {
		return false // Only enemies can have whirlpool movement
	}

	// Check if this enemy has whirlpool movement behavior enabled
	if !enemy.EnemyReference.HasWhirlpoolMovement {
		return false // Not a whirlpool or whirlpool movement disabled
	}

	// Toggle the persistent skip state each turn.
	details := enemy.MapUnitDetails()
	details.SkipNextTurn = !details.SkipNextTurn

	pos := enemy.Pos()

	// Use the toggled state to determine if whirlpool should move (50% chance per turn)
	if details.SkipNextTurn {
		logging.Debugf(logging.CategoryAI, "Whirlpool at %v skipping turn (XOR toggle=skip)", pos)

		return true // Whirlpool doesn't move this turn
	}

	oldPos := pos
	// When whirlpool moves, 50% chance for random movement, 50% for seek behavior
	if m.oneInXOdds(coinFlipOdds) {
		logging.Debugf(logging.CategoryAI, "Whirlpool at %v attempting random movement", oldPos)
		// Attempt bounded random cardinal movement.
		m.moveRandomDirection(mapUnit)
	} else {
		logging.Debugf(logging.CategoryAI, "Whirlpool at %v attempting seek movement", oldPos)
		// Seek toward player
		m.setBestNextPositionToMoveTowardsWalkablePoint(enemy)
	}

	newPos := enemy.Pos()
	logging.Debugf(logging.CategoryAI, "Whirlpool moved from %v to %v (moved=%v)", oldPos, newPos, oldPos != newPos)

	return true // Handled whirlpool movement
}

// HardResetNPCs is not applicable for large maps since there are no floor transitions.
// Large maps maintain NPC continuity across movement and don't have stairs/ladders.
func (m *NPCAIControllerLargeMap) HardResetNPCs(_ bool) {
	// No-op for large maps - NPCs maintain positional continuity
}
