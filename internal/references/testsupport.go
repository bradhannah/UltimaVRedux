package references

import "github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"

// Test support: fixtures and seams that tests in other packages use. The
// dead-code check treats this file as test support (workshop deadcode config).

// GetListOfAllSmallMaps names every small-map location, Moonglow to Serpent's Hold.
func GetListOfAllSmallMaps() []string {
	names := make([]string, 0)

	for loc := Moonglow; loc <= SerpentsHold; loc++ {
		names = append(names, loc.String())
	}

	return names
}

// MonsterSpawnPositions are 16 predefined positions for placing monsters on the 11x11 combat map.
// Original encounter-placement coordinates; ref:COMBAT-0001.
// Encounter generation uses these positions independently of creature identities.
//
//nolint:gochecknoglobals // Immutable encounter-placement coordinates; ref:COMBAT-0001.
var MonsterSpawnPositions = [16]Position{
	{X: 2, Y: 0},
	{X: 5, Y: 0},
	{X: 8, Y: 0},
	{X: 4, Y: 1},
	{X: 6, Y: 1},
	{X: 1, Y: 2},
	{X: 5, Y: 2},
	{X: 9, Y: 2},
	{X: 3, Y: 3},
	{X: 7, Y: 3},
	{X: 0, Y: 4},
	{X: 4, Y: 4},
	{X: 6, Y: 4},
	{X: 10, Y: 4},
	{X: 2, Y: 5},
	{X: 8, Y: 5},
}

// GetAllMonsterSpawnPositions returns a copy of all spawn positions.
// Useful for shuffling positions for random monster placement.
func GetAllMonsterSpawnPositions() []Position {
	positions := make([]Position, len(MonsterSpawnPositions))
	copy(positions, MonsterSpawnPositions[:])

	return positions
}

// ShuffleMonsterSpawnPositions returns shuffled spawn positions.
// Randomizes the placement order without changing the coordinate set.
func ShuffleMonsterSpawnPositions(rng RandomProvider) []Position {
	positions := GetAllMonsterSpawnPositions()

	// Fisher-Yates shuffle
	for i := len(positions) - 1; i > 0; i-- {
		j := rng.Intn(i + 1)
		positions[i], positions[j] = positions[j], positions[i]
	}

	return positions
}

// IsNormalChar returns true if the byte represents a normal (IBM) character (high bit set).
func IsNormalChar(b byte) bool {
	return b&SignHighBitMask != 0
}

// GetSpellsByContext returns all spells valid in the given context.
func GetSpellsByContext(context SpellContext) []Spell {
	var spells []Spell
	for i := range TotalSpellCount {
		if spellMetadata[i].Context.HasContext(context) {
			spells = append(spells, Spell(i))
		}
	}

	return spells
}

// IsSpellValidInContext returns true if the spell can be cast in the given context.
func IsSpellValidInContext(spell Spell, context SpellContext) bool {
	info := GetSpellInfo(spell)
	if info == nil {
		return false
	}

	return info.Context.HasContext(context)
}

// GetStringName returns the missile type's display name.
func (m MissileType) GetStringName() string {
	switch m {
	case MissileNone:
		return "None"
	case MissileArrow:
		return "Arrow"
	case MissileCannonBall:
		return "Cannon Ball"
	case MissileAxe:
		return "Axe"
	case MissileRed:
		return "Red"
	case MissileBlue:
		return "Blue"
	case MissileGreen:
		return "Green"
	case MissileViolet:
		return "Violet"
	case MissileRock:
		return "Rock"
	default:
		return "Unknown"
	}
}

// Reset restores all Shadowlords to alive and clears all presence states.
func (s *ShadowlordState) Reset() {
	s.Alive = [ShadowlordCount]bool{true, true, true}
	s.TownAssignments = [ShadowlordCount]Location{}
	s.LastCalled = NoShadowlord
	s.Summoned = false
	s.AutomaticPresence = NoShadowlord
}

// GetSignsAtLocation returns all signs at the specified location.
func (sr *SignReferences) GetSignsAtLocation(location Location) []*SignRecord {
	return sr.SignsByLocation[location]
}

// GetClosestLadder returns the closest authored directional connection, or
// (-1,-1) when none exists. Runtime route selection must additionally check
// persistent terrain overrides, occupancy and reachability.
func (s *SmallLocationReference) GetClosestLadder(npcCurrentPosition Position, nCurrentFloor, nTargetFloor FloorNumber) Position {
	ladderOrStairType := GetLadderOfStairsType(nCurrentFloor, nTargetFloor)
	ls := s.getListOfAllLaddersAndStairs(nCurrentFloor, ladderOrStairType)

	if len(ls) == 0 {
		// Runtime routes must handle missing connections as pending travel.
		return Position{X: -1, Y: -1}
	}
	bestPosition := ls[0]
	for _, pos := range ls[1:] {
		if npcCurrentPosition.HeuristicTileDistance(pos) < npcCurrentPosition.HeuristicTileDistance(bestPosition) {
			bestPosition = pos
		}
	}

	return bestPosition
}

func (s *SmallLocationReference) getListOfAllLaddersAndStairs(nFloor FloorNumber, ladderOrStairType LadderOrStairType) []Position {
	positions := make([]Position, 0)
	if s.rawData[int(nFloor)] == nil {
		return positions
	}

	for x := Coordinate(0); x <= s.GetMaxX(); x++ {
		for y := Coordinate(0); y <= s.GetMaxY(); y++ {
			pos := Position{X: x, Y: y}
			isLadderOrStair := IsSpecificLadderOrStairs(s.GetTileNumber(pos, nFloor), ladderOrStairType)
			if isLadderOrStair {
				positions = append(positions, pos)
			}
		}
	}

	return positions
}

// IsWall reports whether the tile is a rock or stone-brick wall, including a
// secret one.
func (t *Tile) IsWall() bool {
	return t.Is(indexes.LargeRockWall) || t.Is(indexes.StoneBrickWall) || t.Is(indexes.StoneBrickWallSecret)
}

// GetSpeedFactor returns the speed factor for this tile
func (t *Tile) GetSpeedFactor() int {
	return t.SpeedFactor
}
