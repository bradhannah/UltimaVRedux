package config

import (
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/spf13/viper"

	"github.com/bradhannah/UltimaVRedux/internal/logging"
)

var (
	// ErrGameplayPreference reports a malformed numeric preference or specification.
	ErrGameplayPreference = errors.New("invalid gameplay preference")
	// ErrConfigurationEditing rejects an immediate write over a staged player transaction.
	ErrConfigurationEditing = errors.New("close Configuration with Apply or Cancel before changing persistent preferences")
)

const (
	gameplayCombatSpeedKey      = "Gameplay.Combat.SpeedFactor"
	gameplayConfirmQuickSaveKey = "Gameplay.Saves.ConfirmQuickSave"
	gameplayShowCombatHPBarsKey = "Gameplay.Combat.ShowHealthBars"
	gameplayMaxActionPopupsKey  = "Gameplay.Presentation.MaxVisibleActionPopups"
	legacyCombatSpeedKey        = "Debug.CombatMovementSpeedFactor"
	numericGridTolerance        = 1e-9 // In step-index units; half-steps remain exactly representable.
)

// NumericSettingSpec defines a bounded discrete preference. Values are derived
// from integer step indices rather than repeated floating-point addition. A
// nonempty Values list narrows the grid to those ascending choices.
type NumericSettingSpec struct {
	Min, Max, Step, Default float64
	Values                  []float64
}

// CombatSpeedSpec returns the single specification used by persistence and UI.
// Every choice lies on the half-step grid, so percent transport stays exact.
func CombatSpeedSpec() NumericSettingSpec {
	return NumericSettingSpec{
		Min: 0.5, Max: 20, Step: 0.5, Default: 1, //nolint:mnd // Approved player preference domain.
		Values: []float64{0.5, 1, 1.5, 2, 3, 4, 6, 8, 12, 16, 20},
	}
}

// ActionPopupsSpec returns the integer popup preference domain; zero means Off.
func ActionPopupsSpec() NumericSettingSpec {
	return NumericSettingSpec{
		Min: 0, Max: 3, Step: 1, Default: 3, Values: nil, //nolint:mnd // Approved player preference domain.
	}
}

// Validate checks specification invariants and finite membership in Choices.
func (s NumericSettingSpec) Validate(value float64) error {
	if err := s.ValidateGrid(value); err != nil {
		return err
	}
	if len(s.Values) > 0 && !slices.Contains(s.Values, value) {
		return fmt.Errorf("%w: %g must be one of %v", ErrGameplayPreference, value, s.Values)
	}

	return nil
}

// ValidateGrid checks specification invariants and finite, bounded grid
// membership without requiring a listed value. Loading uses it to accept a
// former grid choice before moving it to the nearest current choice.
func (s NumericSettingSpec) ValidateGrid(value float64) error {
	for _, number := range []float64{s.Min, s.Max, s.Step, s.Default, value} {
		if math.IsNaN(number) || math.IsInf(number, 0) {
			return fmt.Errorf("%w: numbers must be finite", ErrGameplayPreference)
		}
	}
	if s.Step <= 0 || s.Max < s.Min || !s.onGrid(s.Max) || !s.onGrid(s.Default) || !s.validValues() {
		return fmt.Errorf("%w: invalid numeric specification", ErrGameplayPreference)
	}
	if !s.onGrid(value) {
		return fmt.Errorf("%w: %g must be %g..%g in steps of %g", ErrGameplayPreference, value, s.Min, s.Max, s.Step)
	}

	return nil
}

func (s NumericSettingSpec) onGrid(value float64) bool {
	index := (value - s.Min) / s.Step

	return value >= s.Min && value <= s.Max && math.Abs(index-math.Round(index)) <= numericGridTolerance
}

// validValues accepts no list, or ascending grid values spanning Min..Max
// that include Default.
func (s NumericSettingSpec) validValues() bool {
	if len(s.Values) == 0 {
		return true
	}
	if s.Values[0] != s.Min || s.Values[len(s.Values)-1] != s.Max || !slices.Contains(s.Values, s.Default) {
		return false
	}
	for index, value := range s.Values {
		if !s.onGrid(value) || (index > 0 && value <= s.Values[index-1]) {
			return false
		}
	}

	return true
}

// Choices returns every allowed value in ascending order.
func (s NumericSettingSpec) Choices() []float64 {
	if len(s.Values) > 0 {
		return slices.Clone(s.Values)
	}
	last := int(math.Round((s.Max - s.Min) / s.Step))
	choices := make([]float64, 0, last+1)
	for index := range last + 1 {
		choices = append(choices, s.Min+float64(index)*s.Step)
	}

	return choices
}

// Nearest returns the allowed value closest to a finite value, clamping to the
// range; an exact midpoint between two choices resolves upward.
func (s NumericSettingSpec) Nearest(value float64) float64 {
	choices := s.Choices()
	nearest := choices[0]
	for _, choice := range choices[1:] {
		if math.Abs(value-choice) <= math.Abs(value-nearest) {
			nearest = choice
		}
	}

	return nearest
}

// Advance moves by whole choices, clamping at either boundary.
func (s NumericSettingSpec) Advance(value float64, steps int) float64 {
	if err := s.Validate(value); err != nil {
		panic(err) // Definitions and staged models are validated before use.
	}
	choices := s.Choices()
	index := int(math.Round((value - s.Min) / s.Step))
	if len(s.Values) > 0 {
		index = slices.Index(choices, value)
	}
	index = max(0, min(len(choices)-1, index+steps))

	return choices[index]
}

// ValidateGameplayConfiguration rejects invalid explicit numeric settings.
func ValidateGameplayConfiguration(settings GameplayConfiguration) error {
	if err := CombatSpeedSpec().Validate(settings.CombatSpeedFactor); err != nil {
		return fmt.Errorf("%s: %w", gameplayCombatSpeedKey, err)
	}
	if err := ActionPopupsSpec().Validate(float64(settings.MaxVisibleActionPopups)); err != nil {
		return fmt.Errorf("%s: %w", gameplayMaxActionPopupsKey, err)
	}

	return nil
}

// DefaultGameplayConfiguration returns a complete valid model, including numeric
// defaults. These are the only gameplay defaults: loading starts from them and
// applies what the configuration file sets, and detached settings and
// lightweight fixtures use them directly.
func DefaultGameplayConfiguration() *GameplayConfiguration {
	return &GameplayConfiguration{
		BatchMerchantTheft:              true,
		AllowRepeatedMonsterDivision:    false,
		ShowConcealedTurnOrderSlots:     true,
		PersistHiddenDungeonDiscoveries: true,
		ShowSubduedUnlitDungeons:        true,
		SmoothSurfaceMovement:           false,
		FlexibleSextantUse:              false,
		AnkhWandererBonus:               false,
		CombatSpeedFactor:               CombatSpeedSpec().Default,
		ConfirmQuickSave:                true,
		ShowCombatHPBars:                true,
		MaxVisibleActionPopups:          int(ActionPopupsSpec().Default),
	}
}

// loadGameplayPreferences replaces each default with the configured value, if
// the configuration file sets one.
func (uc *UltimaVConfiguration) loadGameplayPreferences() {
	settings := DefaultGameplayConfiguration()
	uc.GameplayConfiguration = settings
	uc.configuredBoolean(&settings.BatchMerchantTheft, gameplayBatchMerchantTheftKey)
	uc.configuredBoolean(&settings.SmoothSurfaceMovement, gameplaySmoothSurfaceMovementKey)
	uc.configuredBoolean(&settings.FlexibleSextantUse, gameplayFlexibleSextantUseKey)
	uc.configuredBoolean(&settings.AnkhWandererBonus, gameplayAnkhWandererBonusKey)
	uc.configuredBoolean(&settings.AllowRepeatedMonsterDivision, "Gameplay.Combat.AllowRepeatedMonsterDivision")
	uc.configuredBoolean(&settings.ShowConcealedTurnOrderSlots, gameplayShowConcealedTurnOrderSlotsKey)
	uc.configuredBoolean(&settings.PersistHiddenDungeonDiscoveries, gameplayPersistHiddenDungeonDiscoveriesKey)
	uc.configuredBoolean(&settings.ShowSubduedUnlitDungeons, gameplayShowSubduedUnlitDungeonsKey)
	uc.configuredBoolean(&settings.ConfirmQuickSave, gameplayConfirmQuickSaveKey)
	uc.configuredBoolean(&settings.ShowCombatHPBars, gameplayShowCombatHPBarsKey)
	if uc.source.IsSet(gameplayCombatSpeedKey) {
		settings.CombatSpeedFactor = uc.normalizedCombatSpeed(uc.source.GetFloat64(gameplayCombatSpeedKey), "combat speed")
	} else if uc.source.IsSet(legacyCombatSpeedKey) {
		settings.CombatSpeedFactor = uc.normalizedCombatSpeed(
			uc.source.GetFloat64(legacyCombatSpeedKey), "legacy combat speed")
	}
	if uc.source.IsSet(gameplayMaxActionPopupsKey) {
		settings.MaxVisibleActionPopups = uc.source.GetInt(gameplayMaxActionPopupsKey)
	}
}

// normalizedCombatSpeed moves a legacy value, or a former canonical grid value
// such as 2.5, to the nearest current choice and records the change.
func (uc *UltimaVConfiguration) normalizedCombatSpeed(value float64, source string) float64 {
	normalized := CombatSpeedSpec().Nearest(value)
	if value != normalized {
		message := fmt.Sprintf("At load, normalized %s %g to %g.", source, value, normalized)
		uc.configurationDiagnostics = append(uc.configurationDiagnostics, message)
		logging.Warnf(logging.CategoryConfig, "%s", message)
	}

	return normalized
}

// GetCombatSpeedFactor returns the canonical master combat presentation speed.
func (uc *UltimaVConfiguration) GetCombatSpeedFactor() float64 {
	if uc == nil || uc.GameplayConfiguration == nil {
		return CombatSpeedSpec().Default
	}

	return uc.GameplayConfiguration.CombatSpeedFactor
}

// GetConfirmQuickSave defaults to confirmation when a fixture has no settings.
func (uc *UltimaVConfiguration) GetConfirmQuickSave() bool {
	return uc == nil || uc.GameplayConfiguration == nil || uc.GameplayConfiguration.ConfirmQuickSave
}

// GetShowCombatHPBars defaults to visible when a fixture has no settings.
func (uc *UltimaVConfiguration) GetShowCombatHPBars() bool {
	return uc == nil || uc.GameplayConfiguration == nil || uc.GameplayConfiguration.ShowCombatHPBars
}

// GetMaxVisibleActionPopups preserves explicit zero (Off).
func (uc *UltimaVConfiguration) GetMaxVisibleActionPopups() int {
	if uc == nil || uc.GameplayConfiguration == nil {
		return int(ActionPopupsSpec().Default)
	}

	return uc.GameplayConfiguration.MaxVisibleActionPopups
}

// ConfigurationEditing reports a staged player transaction on this model.
func (uc *UltimaVConfiguration) ConfigurationEditing() bool {
	return uc != nil && uc.configurationEditors > 0
}

// BeginConfigurationEditing reserves persistent preference editing until release.
// Dialog owners must call the returned release function exactly once on dismissal.
func (uc *UltimaVConfiguration) BeginConfigurationEditing() func() {
	uc.configurationEditors++
	released := false

	return func() {
		if !released {
			uc.configurationEditors--
			released = true
		}
	}
}

func (uc *UltimaVConfiguration) saveGameplayPreference(update func(*GameplayConfiguration)) error {
	if uc == nil || uc.GameplayConfiguration == nil {
		return ErrGameplayConfigNil
	}
	if uc.ConfigurationEditing() {
		return ErrConfigurationEditing
	}
	staged := *uc.GameplayConfiguration
	update(&staged)
	if err := ValidateGameplayConfiguration(staged); err != nil {
		return err
	}
	if err := uc.saveConfiguration(func(candidate *viper.Viper) {
		setGameplayConfiguration(candidate, staged)
	}); err != nil {
		return fmt.Errorf("saving gameplay preference: %w", err)
	}
	*uc.GameplayConfiguration = staged

	return nil
}

// SetCombatSpeedFactor validates and persists before changing the live model.
func (uc *UltimaVConfiguration) SetCombatSpeedFactor(value float64) error {
	return uc.saveGameplayPreference(func(settings *GameplayConfiguration) { settings.CombatSpeedFactor = value })
}
