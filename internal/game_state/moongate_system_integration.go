package game_state

import (
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// IsPlayerOnActiveMoongate checks if the player is standing on an active moongate
// A placed moonstone on the current map/floor is active while the shared stage is nonzero.
func (g *GameState) IsPlayerOnActiveMoongate() bool {
	playerPos := g.MapState.PlayerLocation.Position

	location := g.MapState.PlayerLocation

	return g.moongateStage > MoongateStageHidden &&
		g.moonstoneSystem.CanCreateMoongateAt(location.Location, location.Floor, playerPos)
}

// IsMoongateActiveAt checks if there's an active moongate at the given position
// A moongate is active when the shared stage is nonzero and a placed stone matches this position.
// Any buried or exposed stone can source a gate, regardless of the current destination phase.
func (g *GameState) IsMoongateActiveAt(pos references.Position) bool {
	location := g.MapState.PlayerLocation

	return g.moongateStage > MoongateStageHidden &&
		g.moonstoneSystem.CanCreateMoongateAt(location.Location, location.Floor, pos)
}

// LookAtMoongate handles looking at a moongate and returns true if there was one
func (g *GameState) LookAtMoongate(pos references.Position) bool {
	if g.IsOutOfBounds(pos) || !g.IsMoongateActiveAt(pos) || g.moongateLookOccluded(pos) {
		return false
	}

	if phase, identified := g.moonstoneSourceAt(pos); identified {
		g.observeMoongateLook(phase, pos)
		g.SystemCallbacks.Message.AddRowStr(fmt.Sprintf("Moongate of %s", datetime.MoonPhase(phase)))
	}
	g.SystemCallbacks.Message.AddRowStr("A shimmering portal!")

	return true
}

// These objects are presented above the gate. Check live occupancy as well as
// the Look tile layer so a renderer refresh cannot reveal an obscured source.
// Cosmetic visibility masks are not observation authority for adjacent Look.
func (g *GameState) moongateLookOccluded(pos references.Position) bool {
	if g.GetLayeredMapByCurrentLocation().GetTileByLayer(map_state.MapUnitLayer, &pos).Index > 0 ||
		g.ItemStacksMap.HasItemStackAtPosition(&pos) {
		return true
	}
	if _, corpse := g.surfaceCorpseSpriteAt(pos); corpse {
		return true
	}
	unit := g.CurrentNPCAIController.GetNpcs().GetMapUnitAtPositionAndFloorOrNil(pos, g.MapState.PlayerLocation.Floor)
	if unit != nil && (*unit).IsVisible() && !g.isQuestItemNPCHidden(*unit) {
		return true
	}
	hasItem, item := g.IsQuestItemAtPosition(pos)

	return hasItem && g.IsQuestItemVisible(pos, item)
}

// MoongateTravelResult contains the result of preparing moongate travel
type MoongateTravelResult struct {
	// Success indicates whether travel preparation succeeded
	Success bool
	// Route is captured before the travel workflow starts and advances time.
	Route MoongateTravelRoute
	// FailedButActivated indicates the moongate activated but travel failed (destination unavailable)
	FailedButActivated bool
}

// ReviewMoongateTravel resolves current travel readiness without callbacks or state mutation.
func (g *GameState) ReviewMoongateTravel() MoongateTravelResult {
	// Check if player is on an active moongate
	if !g.IsPlayerOnActiveMoongate() {
		return MoongateTravelResult{Success: false}
	}

	// Check if teleportation already in progress
	if g.IsTeleportationActive() {
		return MoongateTravelResult{Success: false}
	}

	// Get destination - may fail if destination moonstone is in inventory
	route, err := g.GetMoongateTravelRoute()
	if err != nil {
		return MoongateTravelResult{Success: false, FailedButActivated: true}
	}

	return MoongateTravelResult{
		Success: true,
		Route:   route,
	}
}

// ApplyMoongateTravelEffects contracts all gates before a failed activation or
// Spirituality handoff and applies the common travel feedback.
func (g *GameState) ApplyMoongateTravelEffects(success bool) {
	g.moongateStage = MoongateStageHidden
	g.applyMoongateTravelFeedback(success)
}

// ApplyMoongateDepartureEffects keeps an ordinary source gate visible during
// the Avatar dissolve and applies the common travel feedback.
func (g *GameState) ApplyMoongateDepartureEffects() {
	// REDUX DEVIATION: gate entry contracts gate stage to zero before
	// gate travel. Keep the source gate steady beneath the Avatar dissolve.
	// See ref:MOON-0003.
	g.applyMoongateTravelFeedback(true)
}

func (g *GameState) applyMoongateTravelFeedback(success bool) {
	g.SystemCallbacks.Audio.Play(SoundMoongateTravel).Start()
	if success {
		g.SystemCallbacks.Message.AddRowStr("Through the moongate!")
	}
}

// Note: StartTeleportationWorkflow and IsTeleportationActive are now implemented
// in teleportation_sequence.go using event queues instead of a manager interface
// Note: Moongate entry is orchestrated by GameScene.checkMoongateEntry(), which
// reviews and captures the route before building a unified event queue.
