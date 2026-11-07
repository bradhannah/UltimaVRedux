package game_state

// MoonstoneState represents the current state of a moonstone
type MoonstoneState uint8

const (
	// MoonstoneStateBuried means the moonstone remains hidden at its location.
	MoonstoneStateBuried MoonstoneState = 0

	// MoonstoneStateFoundNotCollected means moonstone has been discovered but not picked up.
	MoonstoneStateFoundNotCollected MoonstoneState = 1

	// MoonstoneStateInInventory means moonstone is in player's inventory (0xFF in save data)
	MoonstoneStateInInventory MoonstoneState = 0xFF
)

// MoonstoneLocation represents the location and state of a single moonstone
// Based on original Ultima V SAVED.GAM format with stones_x, stones_y, stones_m, stones_l arrays
type MoonstoneLocation struct {
	X       uint8          // X coordinate (0-255)
	Y       uint8          // Y coordinate (0-255)
	Map     uint8          // Map number (0=overworld, 1-32=towns, 0xFF=inventory)
	Level   uint8          // Z level/dungeon level (0=surface)
	State   MoonstoneState // Current state (buried, exposed, or in inventory)
	IsFound bool           // Whether moonstone has been discovered by player (from obj_found array)
}

// IsInInventory returns true if the moonstone is in the player's inventory
func (ml *MoonstoneLocation) IsInInventory() bool {
	return ml.State == MoonstoneStateInInventory
}

// IsBuried returns true if the moonstone is buried at its location
func (ml *MoonstoneLocation) IsBuried() bool {
	return ml.State == MoonstoneStateBuried
}

// IsFoundNotCollected returns true if the moonstone has been discovered but not picked up
func (ml *MoonstoneLocation) IsFoundNotCollected() bool {
	return ml.State == MoonstoneStateFoundNotCollected
}

// CanCreateMoongate returns true while the moonstone remains placed in the world.
func (ml *MoonstoneLocation) CanCreateMoongate() bool {
	// Original map inspection rules check placement, not discovery; original Search/Get/Open/Jimmy rules exposes
	// a loose object without changing that placement. Pickup sets map to 0xFF.
	// See ref:MOON-0004.
	return ml.Map != 0xFF && (ml.IsBuried() || ml.IsFoundNotCollected())
}
