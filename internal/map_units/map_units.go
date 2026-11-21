package map_units

import (
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/pkg/helpers"
)

// MapUnits is the set of NPCs, enemies and vehicles placed on one map.
type MapUnits []MapUnit

// ReplaceWith replaces a controller's units with the provided ordered instances.
func (m *MapUnits) ReplaceWith(units []MapUnit) {
	*m = append((*m)[:0], units...)
}

// MaximumNpcsPerMap is the original game's fixed limit on NPC slots per map.
const MaximumNpcsPerMap = 32

func (m *MapUnits) getNextAvailableNPCIndexNumber() int {
	if m == nil {
		return -1
	}
	if len(*m) >= MaximumNpcsPerMap {
		return -1
	}

	used := make(map[int]struct{}, len(*m))
	for _, mapUnit := range *m {
		used[mapUnit.MapUnitDetails().NPCNum] = struct{}{}
	}
	for candidate := range MaximumNpcsPerMap {
		if _, exists := used[candidate]; !exists {
			return candidate
		}
	}

	return -1
}

// HasCapacity reports whether another map unit can be inserted without
// changing the original fixed controller limit.
func (m *MapUnits) HasCapacity() bool {
	return m.getNextAvailableNPCIndexNumber() != -1
}

// RemoveNPCAtPosition hides and removes the first unit at position, reporting whether one was found.
func (m *MapUnits) RemoveNPCAtPosition(position references.Position) bool {
	// NOTE: the deletion could cause indexing issues - but shouldn't
	for i, mu := range *m {
		if mu.Pos() == position {
			mu.SetVisible(false)

			*m = helpers.Delete(*m, i)

			return true
		}
	}

	return false
}

// RemoveNPCAtPositionAndFloor removes the first map unit at the exact position and floor.
func (m *MapUnits) RemoveNPCAtPositionAndFloor(
	position references.Position,
	floor references.FloorNumber,
) bool {
	if m == nil {
		return false
	}

	for i, mu := range *m {
		if mu.Pos() == position && mu.Floor() == floor {
			mu.SetVisible(false)
			*m = helpers.Delete(*m, i)

			return true
		}
	}

	return false
}

// RemoveNPCByNumber removes the stable small-map NPC/object slot.
func (m *MapUnits) RemoveNPCByNumber(npcNumber int) bool {
	for index, mapUnit := range *m {
		if mapUnit.MapUnitDetails().NPCNum != npcNumber {
			continue
		}

		mapUnit.SetVisible(false)
		*m = helpers.Delete(*m, index)

		return true
	}

	return false
}

// GetMapUnitAtPositionOrNil returns the first unit at an XY position without
// considering floor. Use it only for flat map-unit collections.
func (m *MapUnits) GetMapUnitAtPositionOrNil(pos references.Position) *MapUnit {
	for index := range *m {
		if (*m)[index].Pos() == pos {
			matched := (*m)[index]

			return &matched
		}
	}

	return nil
}

// GetMapUnitAtPositionAndFloorOrNil returns a map unit at the specified position and floor,
// or nil if no unit exists at that exact location and floor level.
// This method is floor-aware, unlike GetMapUnitAtPositionOrNil which only checks X,Y coordinates.
func (m *MapUnits) GetMapUnitAtPositionAndFloorOrNil(pos references.Position, floor references.FloorNumber) *MapUnit {
	if m == nil {
		return nil
	}
	for index := range *m {
		if (*m)[index].Pos() == pos && (*m)[index].MapUnitDetails().Floor == floor {
			matched := (*m)[index]

			return &matched
		}
	}

	return nil
}

// GetVisibleEnemyAtPositionAndFloorOrNil returns the stable enemy instance at
// the exact position and floor. Hidden enemies are excluded from contact.
func (m *MapUnits) GetVisibleEnemyAtPositionAndFloorOrNil(
	pos references.Position,
	floor references.FloorNumber,
) *NPCEnemy {
	if m == nil {
		return nil
	}

	for _, mapUnit := range *m {
		enemy, ok := mapUnit.(*NPCEnemy)
		if !ok || !enemy.IsVisible() {
			continue
		}
		if enemy.Pos() == pos && enemy.Floor() == floor {
			return enemy
		}
	}

	return nil
}

// GetVehicleAtPositionAndFloorOrNil returns a vehicle at the exact position and floor.
func (m *MapUnits) GetVehicleAtPositionAndFloorOrNil(
	pos references.Position,
	floor references.FloorNumber,
) *NPCFriendly {
	mapUnit := m.GetMapUnitAtPositionAndFloorOrNil(pos, floor)

	if mapUnit == nil {
		return nil
	}

	if friendly, ok := (*mapUnit).(*NPCFriendly); ok {
		if friendly.NPCReference.GetNPCType() == references.Vehicle {
			return friendly
		}
	}

	return nil
}

// InsertVehicle inserts a copy and returns the authoritative stored instance.
// It returns nil when the controller has no free slot.
func (m *MapUnits) InsertVehicle(vehicle NPCFriendly) *NPCFriendly {
	index := m.getNextAvailableNPCIndexNumber()
	if index == -1 {
		return nil
	}

	vehicle.mapUnitDetails.NPCNum = index
	vehicle.SetPos(references.Position{
		X: references.Coordinate(vehicle.NPCReference.Schedule.X[0]),
		Y: references.Coordinate(vehicle.NPCReference.Schedule.Y[0]),
	})

	vehicle.SetVisible(true)

	stored := &vehicle
	*m = append(*m, stored)

	return stored
}

// AddVehicle preserves the existing boolean insertion contract.
func (m *MapUnits) AddVehicle(vehicle NPCFriendly) bool {
	return m.InsertVehicle(vehicle) != nil
}

// ActivatePendingVehicleDelivery clears the pending marker on the stored
// delivery and returns that same authoritative instance.
func (m *MapUnits) ActivatePendingVehicleDelivery() *NPCFriendly {
	if m == nil {
		return nil
	}

	for _, mapUnit := range *m {
		friendly, ok := mapUnit.(*NPCFriendly)
		if !ok || friendly.NPCReference.GetNPCType() != references.Vehicle ||
			!friendly.GetVehicleDetails().IsPendingDelivery() {
			continue
		}

		friendly.GetVehicleDetails().ClearPendingDelivery()

		return friendly
	}

	return nil
}

// AddEnemy adds an enemy NPC to the map units collection
// Returns false if maximum NPCs already spawned
func (m *MapUnits) AddEnemy(enemy NPCEnemy) bool {
	index := m.getNextAvailableNPCIndexNumber()
	if index == -1 {
		return false
	}

	enemy.mapUnitDetails.NPCNum = index
	enemy.SetVisible(true)

	*m = append(*m, &enemy)

	return true
}

// GetEnemies returns all enemy NPCs from the map units collection
func (m *MapUnits) GetEnemies() []NPCEnemy {
	enemies := make([]NPCEnemy, 0)

	for _, mu := range *m {
		if enemy, ok := mu.(*NPCEnemy); ok {
			enemies = append(enemies, *enemy)
		}
	}

	return enemies
}

// EnemyMatchFunc is a predicate function for filtering enemies
type EnemyMatchFunc func(*NPCEnemy) bool

// GetEnemiesMatching returns all enemies that match the predicate function
func (m *MapUnits) GetEnemiesMatching(match EnemyMatchFunc) []*NPCEnemy {
	var result []*NPCEnemy

	for _, mu := range *m {
		if enemy, ok := mu.(*NPCEnemy); ok {
			if match(enemy) {
				result = append(result, enemy)
			}
		}
	}

	return result
}

// GetFirstEnemyMatching returns the first enemy matching the predicate, or nil if none found
func (m *MapUnits) GetFirstEnemyMatching(match EnemyMatchFunc) *NPCEnemy {
	for _, mu := range *m {
		if enemy, ok := mu.(*NPCEnemy); ok {
			if match(enemy) {
				return enemy
			}
		}
	}

	return nil
}

// HasEnemyMatching returns true if any enemy matches the predicate function
func (m *MapUnits) HasEnemyMatching(match EnemyMatchFunc) bool {
	return m.GetFirstEnemyMatching(match) != nil
}

// FriendlyMatchFunc is a predicate function for filtering friendly NPCs
type FriendlyMatchFunc func(*NPCFriendly) bool

// GetFriendliesMatching returns all friendly NPCs that match the predicate function
func (m *MapUnits) GetFriendliesMatching(match FriendlyMatchFunc) []*NPCFriendly {
	var result []*NPCFriendly
	for _, mu := range *m {
		if friendly, ok := mu.(*NPCFriendly); ok {
			if match(friendly) {
				result = append(result, friendly)
			}
		}
	}

	return result
}

// RemoveEnemy removes an enemy NPC from the map units collection
func (m *MapUnits) RemoveEnemy(enemy *NPCEnemy) bool {
	for i, mu := range *m {
		if e, ok := mu.(*NPCEnemy); ok && e == enemy {
			e.SetVisible(false)
			*m = helpers.Delete(*m, i)

			return true
		}
	}

	return false
}

// RemoveAllEnemies removes only hostile units and returns the number removed.
func (m *MapUnits) RemoveAllEnemies() int {
	kept := (*m)[:0]
	removed := 0
	for _, mapUnit := range *m {
		if enemy, ok := mapUnit.(*NPCEnemy); ok {
			enemy.SetVisible(false)
			removed++

			continue
		}
		kept = append(kept, mapUnit)
	}
	*m = kept

	return removed
}

// ClearAllMapUnits removes all map units from the collection.
// Used for Stonegate volcano trap - more definitive than hiding.
func (m *MapUnits) ClearAllMapUnits() {
	*m = (*m)[:0]
}
