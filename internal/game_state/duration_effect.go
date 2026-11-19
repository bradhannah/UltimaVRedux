package game_state

import (
	"errors"
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// ErrInvalidDurationEffectDebugOperation indicates an invalid or stale duration-effect edit.
var ErrInvalidDurationEffectDebugOperation = errors.New("invalid duration effect debug operation")

// DurationEffectType represents active duration spell effects. Redux retains one
// duration-spell slot competing with Crown; Badge and Amulet remain independent.
type DurationEffectType byte

// DisplayName is the player-facing name shared by expiration feedback and inspectors.
func (e DurationEffectType) DisplayName() string {
	switch e {
	case DurationEffectNone:
		return ""
	case DurationEffectLight:
		return "Light"
	case DurationEffectProtection:
		return "Protection"
	case DurationEffectNegateMagic:
		return "Negate Magic"
	case DurationEffectTimeStop:
		return "Time Stop"
	case DurationEffectQuickness:
		return "Quickness"
	case DurationEffectConfusion:
		return "Confusion"
	default:
		panic(fmt.Sprintf("unknown duration effect %q", byte(e)))
	}
}

// Duration effect type constants for active spell effects.
const (
	DurationEffectNone DurationEffectType = 0
	// DurationEffectLight remains for legacy/debug compatibility. Runtime
	// illumination uses GameState.MagicLightMinutesRemaining independently
	// of the active duration-effect slot. ref:SPELL-0004
	DurationEffectLight       DurationEffectType = 'L'
	DurationEffectProtection  DurationEffectType = 'P' // In Sanct (100 turns from scroll)
	DurationEffectNegateMagic DurationEffectType = 'N' // In An (20 turns from scroll)
	DurationEffectTimeStop    DurationEffectType = 'T' // An Tym (20 turns from scroll)
	DurationEffectQuickness   DurationEffectType = 'Q' // Rel Tym spell (30 turns)
	DurationEffectConfusion   DurationEffectType = 'C' // Quas An Wis spell (20 turns)
)

// ToggleItemEffectDuration marks effects that never expire (Crown, Amulet, Badge).
// Original turn processing also treated duration 255 as permanent.
const ToggleItemEffectDuration = 255

// DurationEffect tracks the currently active duration spell.
type DurationEffect struct {
	EffectType       DurationEffectType
	TurnsRemaining   int  // ToggleItemEffectDuration (255) = never decrements (toggle items only)
	IsToggleItemOnly bool // True for Crown/Amulet/Badge - never decrements
}

// DurationEffectStatus is a detached view of the single active duration effect.
type DurationEffectStatus struct {
	Active           bool
	EffectCode       string
	Name             string
	TurnsRemaining   int
	MagicLightActive bool
}

// MagicLightStatus is a detached view of independent magical illumination.
type MagicLightStatus struct {
	Active           bool
	MinutesRemaining int
}

// DurationEffectsStatus is a detached view of both independently persistent effects.
type DurationEffectsStatus struct {
	ActiveSpell DurationEffectStatus
	MagicLight  MagicLightStatus
}

// DurationEffectOperationResult reports exact state surrounding a debug duration-effect edit.
type DurationEffectOperationResult struct {
	Before DurationEffectsStatus
	After  DurationEffectsStatus
}

// CrownDisplacedMessage tells the player a duration spell took the Crown off.
// REDUX DEVIATION: QUEST-0003: the original overwrites the worn Crown silently
// when a duration spell takes the shared effect slot; Redux keeps the rule and
// names the loss so the Crown's protection does not vanish unexplained.
const CrownDisplacedMessage = "Crown removed!"

// setDurationEffect activates or refreshes one effect, replacing any other effect.
func (gs *GameState) setDurationEffect(effectType DurationEffectType, turns int) bool {
	wasTimeStopped := gs.HasActiveDurationEffect(DurationEffectTimeStop)
	if gs.CrownState.IsActive {
		gs.SystemCallbacks.UI.ShowActionFeedback(CrownDisplacedMessage, true)
	}
	gs.CrownState = ToggleItemState{}
	gs.ActiveDurationEffect = DurationEffect{
		EffectType:       effectType,
		TurnsRemaining:   turns,
		IsToggleItemOnly: false, // Spells/scrolls are not toggle items
	}
	if wasTimeStopped != gs.HasActiveDurationEffect(DurationEffectTimeStop) {
		gs.SystemCallbacks.Screen.UpdateDisplay()
	}

	return true
}

// ClearDurationEffect removes the active effect.
func (gs *GameState) ClearDurationEffect() {
	wasTimeStopped := gs.HasActiveDurationEffect(DurationEffectTimeStop)
	gs.ActiveDurationEffect = DurationEffect{
		EffectType:       DurationEffectNone,
		TurnsRemaining:   0,
		IsToggleItemOnly: false,
	}
	if wasTimeStopped {
		gs.SystemCallbacks.Screen.UpdateDisplay()
	}
}

// GetDurationEffectStatus returns a complete detached view of the active effect.
func (gs *GameState) GetDurationEffectStatus() DurationEffectStatus {
	effect := gs.ActiveDurationEffect
	status := DurationEffectStatus{
		Active:           effect.EffectType != DurationEffectNone,
		EffectCode:       "",
		Name:             "",
		TurnsRemaining:   effect.TurnsRemaining,
		MagicLightActive: gs.MapState.Lighting.HasMagicLight(),
	}
	if !status.Active {
		return status
	}

	status.EffectCode = string(effect.EffectType)
	status.Name = effect.EffectType.DisplayName()

	return status
}

// GetDurationEffectsStatus returns active-spell and magical-light state together.
func (gs *GameState) GetDurationEffectsStatus() DurationEffectsStatus {
	return DurationEffectsStatus{
		ActiveSpell: gs.GetDurationEffectStatus(),
		MagicLight: MagicLightStatus{
			Active:           gs.MagicLightMinutesRemaining > 0,
			MinutesRemaining: gs.MagicLightMinutesRemaining,
		},
	}
}

// SetDebugDurationEffectTurns changes remaining turns after validating reviewed state.
func (gs *GameState) SetDebugDurationEffectTurns(
	expectedEffect string,
	expectedTurns int,
	expectedLightMinutes int,
	turnsRemaining int,
) (DurationEffectOperationResult, error) {
	before := gs.GetDurationEffectsStatus()
	result := DurationEffectOperationResult{Before: before, After: before}
	if !before.ActiveSpell.Active || before.ActiveSpell.EffectCode != expectedEffect || before.ActiveSpell.TurnsRemaining != expectedTurns ||
		before.MagicLight.MinutesRemaining != expectedLightMinutes {
		return result, fmt.Errorf("%w: active effect changed after review", ErrInvalidDurationEffectDebugOperation)
	}
	if turnsRemaining <= 0 {
		return result, fmt.Errorf("%w: remaining turns must be positive", ErrInvalidDurationEffectDebugOperation)
	}

	gs.ActiveDurationEffect.TurnsRemaining = turnsRemaining
	result.After = gs.GetDurationEffectsStatus()

	return result, nil
}

// ActivateDebugDurationEffect explicitly activates a supported effect at its authentic scroll duration.
func (gs *GameState) ActivateDebugDurationEffect(
	effectType DurationEffectType,
	expectedEffect string,
	expectedTurns int,
	expectedLightMinutes int,
) (DurationEffectOperationResult, error) {
	before := gs.GetDurationEffectsStatus()
	result := DurationEffectOperationResult{Before: before, After: before}
	if before.ActiveSpell.EffectCode != expectedEffect || before.ActiveSpell.TurnsRemaining != expectedTurns ||
		before.MagicLight.MinutesRemaining != expectedLightMinutes {
		return result, fmt.Errorf("%w: spell effects changed after review", ErrInvalidDurationEffectDebugOperation)
	}
	var turns int
	//nolint:exhaustive // Debug activation intentionally supports only Light and Time Stop scroll effects.
	switch effectType {
	case DurationEffectLight:
		turns = gs.GameReferences.InventoryItemReferences.GetScrollEffectDuration(references.ScrollVasLor)
	case DurationEffectTimeStop:
		turns = gs.GameReferences.InventoryItemReferences.GetScrollEffectDuration(references.ScrollAnTym)
	default:
		return result, fmt.Errorf("%w: unsupported effect %q", ErrInvalidDurationEffectDebugOperation, effectType)
	}

	if effectType == DurationEffectLight {
		gs.setDebugMagicLightMinutes(turns)
	} else {
		gs.setDurationEffect(effectType, turns)
	}
	result.After = gs.GetDurationEffectsStatus()

	return result, nil
}

// SetDebugMagicLightMinutes changes the independent timer after validating reviewed state.
func (gs *GameState) SetDebugMagicLightMinutes(
	expectedEffect string,
	expectedTurns int,
	expectedMinutes int,
	minutesRemaining int,
) (DurationEffectOperationResult, error) {
	before := gs.GetDurationEffectsStatus()
	result := DurationEffectOperationResult{Before: before, After: before}
	if before.ActiveSpell.EffectCode != expectedEffect || before.ActiveSpell.TurnsRemaining != expectedTurns ||
		before.MagicLight.MinutesRemaining != expectedMinutes {
		return result, fmt.Errorf("%w: magical light changed after review", ErrInvalidDurationEffectDebugOperation)
	}
	if minutesRemaining < 0 {
		return result, fmt.Errorf("%w: magical-light minutes cannot be negative", ErrInvalidDurationEffectDebugOperation)
	}

	gs.setDebugMagicLightMinutes(minutesRemaining)
	result.After = gs.GetDurationEffectsStatus()

	return result, nil
}

func (gs *GameState) setDebugMagicLightMinutes(minutes int) {
	gs.MagicLightMinutesRemaining = minutes
	gs.MapState.Lighting.SetMagicLight(minutes > 0)
	gs.GetCurrentLayeredMap().InvalidateLightSourceCache()
}

// HasActiveDurationEffect checks if specific effect is active.
func (gs *GameState) HasActiveDurationEffect(effectType DurationEffectType) bool {
	return gs.ActiveDurationEffect.EffectType == effectType
}

// HasAnyDurationEffect checks if any duration effect is active.
func (gs *GameState) HasAnyDurationEffect() bool {
	return gs.ActiveDurationEffect.EffectType != DurationEffectNone
}

// processDurationEffects decrements and expires effects.
// Called from ProcessTurnEffects each turn.
func (gs *GameState) processDurationEffects() {
	effect := &gs.ActiveDurationEffect
	if effect.EffectType == DurationEffectNone {
		return
	}

	// Party status update leaves zero and 255 untouched; toggle items are
	// likewise permanent until explicitly removed.
	// See ref:SPELL-0004.
	if effect.IsToggleItemOnly || effect.TurnsRemaining == 0 || effect.TurnsRemaining == ToggleItemEffectDuration {
		return
	}

	effect.TurnsRemaining--
	if effect.TurnsRemaining <= 0 {
		name := effect.EffectType.DisplayName()

		gs.ClearDurationEffect()

		gs.SystemCallbacks.UI.ShowSpellExpiredPopup(name)
	}
}
