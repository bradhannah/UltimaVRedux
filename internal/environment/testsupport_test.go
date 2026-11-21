package environment

import (
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func (h *EnvironmentalHazards) CheckTileHazards(tile *references.Tile, party *party_state.PartyState, location references.GeneralMapType) HazardResult {
	result := h.ResolveTileHazards(tile, party, location)
	h.emitHazardResult(&result, party)

	return result
}

// IncrementUpdateCount increments the internal update counter for sailing timing
func (w *WindSystem) IncrementUpdateCount() {
	w.updateCounter++
}

// ShouldMoveShip returns true if the ship should move based on wind timing
// Used in sailing mode: updtcnt >= num % 3
func (w *WindSystem) ShouldMoveShip(direction references.Direction) bool {
	speed := w.GetFrigateSailingSpeed(direction)

	return w.updateCounter >= (speed % 3)
}

func (h *EnvironmentalHazards) emitHazardResult(result *HazardResult, party *party_state.PartyState) {
	if len(result.Affected) == 0 {
		return
	}
	names := make([]string, 0, len(result.Affected))
	for _, memberIndex := range result.Affected {
		name := party.GetCharacter(memberIndex).GetNameAsString()
		if name != "" {
			names = append(names, name)
		}
	}

	switch result.Type {
	case PoisonSwamp:
		message := "Poisoned!"
		if len(names) > 0 {
			message = "Poisoned: " + joinNames(names)
		}
		h.callbacks.AddRowStr(message)
		result.MessageShown = true
	case LavaBurn, FireplaceBurn:
		message := "Burning!"
		if len(names) > 0 {
			message = "Burning: " + joinNames(names)
		}
		h.callbacks.AddRowStr(message)
		result.MessageShown = true
	case NoHazard:
	}
}
