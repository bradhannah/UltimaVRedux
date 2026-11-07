package game_state

import "github.com/bradhannah/UltimaVRedux/internal/references"

// MagicState records successfully mixed recipes and legacy starter knowledge.
// The spell catalog combines Book-of-Lore formulas with PlayerKnowledge for
// browsing and auto-selection; an ordinary recipe does not require a prior mix.
type MagicState struct {
	// KnownRecipes maps spells to their discovered reagent combinations
	// Once a spell is successfully mixed, the correct reagent mask is stored here
	KnownRecipes map[references.Spell]uint16
}

// NewMagicState creates a new MagicState with starting recipes.
// Experiment history starts with In Lor and An Nox as basic starter recipes.
func NewMagicState() *MagicState {
	ms := &MagicState{
		KnownRecipes: make(map[references.Spell]uint16),
	}

	// Initialize with starting recipes (In Lor and An Nox)
	ms.KnownRecipes[references.InLor] = references.GetSpellReagents(references.InLor) // SulfurAsh
	ms.KnownRecipes[references.AnNox] = references.GetSpellReagents(references.AnNox) // Ginseng, Garlic

	return ms
}

// IsRecipeKnown returns true if the player has discovered the recipe for this spell.
func (ms *MagicState) IsRecipeKnown(spell references.Spell) bool {
	_, exists := ms.KnownRecipes[spell]

	return exists
}

// RecordRecipe stores a successfully mixed spell recipe for future reference.
// This enables automatic reagent selection when the player types this spell in the future.
func (ms *MagicState) RecordRecipe(spell references.Spell, reagentMask uint16) {
	ms.KnownRecipes[spell] = reagentMask
}
