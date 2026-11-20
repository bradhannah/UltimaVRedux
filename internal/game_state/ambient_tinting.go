package game_state

import "github.com/bradhannah/UltimaVRedux/internal/references"

// AmbientTintDarkness is the cosmetic night strength for the current location.
// It must not be used for visibility, carried-light fuel, or gameplay time.
func (g *GameState) AmbientTintDarkness() float32 {
	if (g.MapState.IsUnderworld() && !g.IsInCombat()) || g.isCampedBelowGround() {
		// REDUX DEVIATION: calendar advancement uses time-independent underground
		// sight, not RGB lighting. Underworld exploration, and a camp pitched in
		// the Underworld or a dungeon, use Redux's existing full-night color
		// composition while leaving gameplay sight untouched. See ref:LIGHT-0008.
		return 1
	}

	return g.DateTime.GetAmbientTintDarkness()
}

// isCampedBelowGround reports a camp pitched where the surface sun never
// reaches. The camp map replaces the player location, so the place the party
// lay down is read from the camp scene.
func (g *GameState) isCampedBelowGround() bool {
	if g.CampingScene == nil || !g.IsInCamping() {
		return false
	}
	origin := g.CampingScene.OriginalPlayerLocation
	mapType := origin.Location.GetMapType()

	return mapType == references.DungeonMapType ||
		(mapType == references.LargeMapType && origin.Floor == references.FloorNumber(references.UNDERWORLD))
}
