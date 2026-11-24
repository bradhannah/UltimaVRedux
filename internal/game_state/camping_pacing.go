package game_state

import "github.com/bradhannah/UltimaVRedux/internal/camping"

// RequestCampingAcceleration latches pacing only during the current sleep loop.
// Completion, calendar advancement and random decisions remain with the workflow.
func (g *GameState) RequestCampingAcceleration() bool {
	return g.CampingScene.RequestAcceleration()
}

// RequestCampingSkip sleeps through the remaining cycles at once. Each cycle's
// calendar step and random decisions still run, in order, so the night is the same.
func (g *GameState) RequestCampingSkip() bool {
	return g.CampingScene.RequestSkip()
}

func campingCycleDelay(scene *camping.CampingScene) int64 {
	switch {
	case scene.IsSkipping():
		return 0
	case scene.IsAccelerated():
		return CampingFastCycleDelayMs
	default:
		return CampingCycleDelayMs
	}
}
