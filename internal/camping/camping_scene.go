package camping

import (
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// Time cycle constants
const (
	MinutesPerCycle        = 5
	CyclesPerHour          = 12
	DungeonMinutesPerCycle = 5
	DungeonCyclesPerHour   = 12
)

// Hour message tracking
const (
	NoHourMessageYet = -1 // Initial value for LastHourMessage
)

// CampingScene manages the camping visual experience and state
//
//nolint:revive // Preserve the established domain name used by gameplay and presentation callers.
type CampingScene struct {
	// Current camping map
	CampingMap *Map

	// Camping state
	Hours        int
	GuardIndex   int
	CurrentCycle int
	TotalCycles  int
	CycleMinutes int

	// Scene state
	IsActive        bool
	TimeElapsed     int // Minutes elapsed
	LastHourMessage int // Last hour we showed a time message
	sleeping        bool
	accelerated     bool
	skipping        bool

	// Original player location (to return to after camping)
	OriginalPlayerLocation references.PlayerLocation

	// Wanderer visual state

	WandererPresent       bool    // Whether wanderer should be visible on camping map
	WandererDissolveLevel float64 // 0.0 = fully visible, 1.0 = fully dissolved
}

// NewCampingSceneWithCombatRef creates a new camping scene with authentic positions from combat map
func NewCampingSceneWithCombatRef(
	hours int,
	guardIndex int,
	originalPlayerLocation references.PlayerLocation,
	combatMapRef *references.CombatMapReference,
) *CampingScene {
	scene := &CampingScene{
		CampingMap:             nil,
		Hours:                  hours,
		GuardIndex:             guardIndex,
		TotalCycles:            hours * CyclesPerHour,
		CycleMinutes:           MinutesPerCycle,
		CurrentCycle:           0,
		IsActive:               true,
		TimeElapsed:            0,
		LastHourMessage:        NoHourMessageYet,
		OriginalPlayerLocation: originalPlayerLocation,
		sleeping:               false,
		accelerated:            false,
		skipping:               false,
		WandererPresent:        false,
		WandererDissolveLevel:  0,
	}

	// Create the camping map with combat map reference for authentic positions
	scene.CampingMap = NewCampingMapWithCombatRef(guardIndex, OutdoorCamping, combatMapRef)

	logging.Debugf(logging.CategoryGameState,
		"Created camping scene with authentic BRIT.CBT positions for %d hours with guard %d", hours, guardIndex)

	return scene
}

// NewDungeonCampingScene creates a dungeon camp using a generated dungeon map.
func NewDungeonCampingScene(
	hours int,
	guardIndex int,
	originalPlayerLocation references.PlayerLocation,
	combatMapRef *references.CombatMapReference,
) *CampingScene {
	return &CampingScene{
		CampingMap:             NewCampingMapWithCombatRef(guardIndex, DungeonCamping, combatMapRef),
		Hours:                  hours,
		GuardIndex:             guardIndex,
		CurrentCycle:           0,
		TotalCycles:            hours * DungeonCyclesPerHour,
		CycleMinutes:           DungeonMinutesPerCycle,
		IsActive:               true,
		TimeElapsed:            0,
		LastHourMessage:        NoHourMessageYet,
		OriginalPlayerLocation: originalPlayerLocation,
		WandererPresent:        false,
		WandererDissolveLevel:  0,
		sleeping:               false,
		accelerated:            false,
		skipping:               false,
	}
}

// StartCamping begins the camping scene
func (cs *CampingScene) StartCamping() {
	cs.EndSleeping()
	cs.IsActive = true
	cs.CurrentCycle = 0
	cs.TimeElapsed = 0

	logging.Debugf(logging.CategoryGameState, "Starting camping scene for %d cycles", cs.TotalCycles)
}

// AdvanceExactCycle advances and processes the final configured cycle.
func (cs *CampingScene) AdvanceExactCycle() bool {
	if !cs.IsActive || cs.CurrentCycle >= cs.TotalCycles {
		return false
	}
	cs.CurrentCycle++
	cs.TimeElapsed += cs.CycleMinutes
	if cs.CurrentCycle >= cs.TotalCycles {
		cs.CompleteCamping()
	}

	return true
}

// CompleteCamping ends the camping scene
func (cs *CampingScene) CompleteCamping() {
	cs.IsActive = false
	cs.EndSleeping()
	logging.Debugf(logging.CategoryGameState, "Camping scene completed after %d cycles", cs.CurrentCycle)
}

// InterruptCamping stops camping early (due to encounters or death)
func (cs *CampingScene) InterruptCamping() {
	cs.IsActive = false
	cs.EndSleeping()
	logging.Debugf(logging.CategoryGameState, "Camping scene interrupted at cycle %d/%d", cs.CurrentCycle, cs.TotalCycles)
}

// BeginSleeping opens the transient pacing window after setup and Bard music.
func (cs *CampingScene) BeginSleeping() {
	cs.sleeping = cs.IsActive && cs.CurrentCycle < cs.TotalCycles
	cs.accelerated, cs.skipping = false, false
}

// EndSleeping closes the window before results, ambush or Wanderer presentation.
func (cs *CampingScene) EndSleeping() {
	cs.sleeping = false
	cs.accelerated, cs.skipping = false, false
}

// IsSleeping distinguishes logical sleep from setup and post-rest presentation.
func (cs *CampingScene) IsSleeping() bool { return cs != nil && cs.sleeping }

// IsAccelerated reports the current camp's one-way pacing latch.
func (cs *CampingScene) IsAccelerated() bool { return cs != nil && cs.sleeping && cs.accelerated }

// RequestAcceleration changes presentation intent only; no cycle or random draw.
func (cs *CampingScene) RequestAcceleration() bool {
	if !cs.IsSleeping() {
		return false
	}
	cs.accelerated = true

	return true
}

// IsSkipping reports that the rest of this sleep runs with no wait between cycles.
func (cs *CampingScene) IsSkipping() bool { return cs != nil && cs.sleeping && cs.skipping }

// RequestSkip changes pacing only, as acceleration does: every cycle still runs in order.
func (cs *CampingScene) RequestSkip() bool {
	if !cs.IsSleeping() {
		return false
	}
	cs.skipping = true

	return true
}

// GetPartyMemberPosition returns the position for a specific party member
func (cs *CampingScene) GetPartyMemberPosition(memberIndex int) references.Position {
	return cs.CampingMap.GetPartyMemberPosition(memberIndex)
}

// GetOriginalPlayerLocation returns the player location to return to after camping
func (cs *CampingScene) GetOriginalPlayerLocation() references.PlayerLocation {
	return cs.OriginalPlayerLocation
}

// SetWandererPresent sets whether the wanderer should be visible
func (cs *CampingScene) SetWandererPresent(present bool) {
	cs.WandererPresent = present
	logging.Debugf(logging.CategoryGameState, "Wanderer presence set to: %t", present)
}

// SetWandererDissolveLevel sets the wanderer's dissolve level for animation
func (cs *CampingScene) SetWandererDissolveLevel(level float64) {
	cs.WandererDissolveLevel = level
	logging.Debugf(logging.CategoryGameState, "Wanderer dissolve level set to: %.2f", level)
}

// GetWandererPosition returns the wanderer's position on the camping map
func (cs *CampingScene) GetWandererPosition() references.Position {
	// Position wanderer on the opposite side of campfire from party
	// This puts the wanderer in a visually prominent position
	const wandererColumn, wandererRow = 5, 5

	return references.Position{X: wandererColumn, Y: wandererRow} // Top side of camping area
}

// UpdateGuardPosition updates the guard's visual position during camping
// This is called when the guard wanders around the campsite
// Guard movement updates the selected watch character's position; ref:CAMP-0003.
func (cs *CampingScene) UpdateGuardPosition(newPosition references.Position) {
	// Update the camping map's guard position for rendering
	// This tells the visual system where to draw the guard sprite
	if cs.CampingMap != nil {
		cs.CampingMap.GuardPosition = newPosition
		logging.Debugf(logging.CategoryGameState,
			"Updated guard position to (%d,%d) in camping scene",
			newPosition.X, newPosition.Y)
	}
}
