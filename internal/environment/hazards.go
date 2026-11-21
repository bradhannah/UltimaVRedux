// Package environment implements world hazards (swamp, lava, fire damage) and sailing wind.
package environment

import (
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// SystemCallbacks interface to avoid import cycle with game_state
type SystemCallbacks interface {
	AddRowStr(message string)
}

// HazardType classifies the damaging tile a party is standing on.
type HazardType int

const (
	// NoHazard means the tile is harmless (also used for generic damage results).
	NoHazard HazardType = iota
	// PoisonSwamp poisons on a Dexterity-based roll.
	PoisonSwamp
	// LavaBurn burns every active member.
	LavaBurn
	// FireplaceBurn burns every active member, indoors only.
	FireplaceBurn
)

// DamageType represents different types of damage sources for generic damage application
type DamageType int

const (
	// TrapFallDamage is damage from a dungeon trap fall.
	TrapFallDamage DamageType = iota
	// ClimbingFallDamage is damage from falling while climbing.
	ClimbingFallDamage
	// Future: CombatDamage, SpellDamage, etc.
)

// namesInPair is the list length that joins with a bare "and" instead of commas.
const namesInPair = 2

// Hazard game mechanics constants.

const (
	// PoisonRollMax is the number of poison roll values (30); the minimum depends on map context.
	PoisonRollMax = 30

	// BurnDamageMin and BurnDamageMax bound the separate inclusive 1..8 draw each living member takes; ref:ENV-0001.
	BurnDamageMin = 1
	// BurnDamageMax is the highest burn damage a member takes in one draw.
	BurnDamageMax = 8

	// FlashColorRed is the red channel (0..1) of the fire-damage screen flash.
	FlashColorRed = 1.0
	// FlashColorGreen is the green channel of the fire flash.
	FlashColorGreen = 0.2
	// FlashColorBlue is the blue channel of the fire flash.
	FlashColorBlue = 0.1
	// FlashIntensity is the fire flash opacity (0..1).
	FlashIntensity = 0.7
	// FlashDurationMs is how long the fire flash lasts.
	FlashDurationMs = 300
)

// EnvironmentalHazards resolves swamp poison, lava and fireplace burns with the shared RNG.
type EnvironmentalHazards struct {
	rng       randomIntn
	callbacks SystemCallbacks
}

// HazardResult reports the hazard hit, the affected party slots and the total damage.
type HazardResult struct {
	Type         HazardType
	Affected     []int // Party member indices affected
	MessageShown bool
	DamageDealt  int
}

// DamageConfig specifies parameters for generic damage application
type DamageConfig struct {
	DamageType    DamageType
	FixedDamage   int // If > 0, apply this exact damage to each character
	MinDamage     int // For random damage range (ignored if FixedDamage > 0)
	MaxDamage     int // For random damage range (ignored if FixedDamage > 0)
	Message       string
	ScreenFlash   *ScreenFlashConfig
	TargetIndices []int // If specified, only affect these character indices (nil = all party)
}

// ScreenFlashConfig defines screen flash visual effect parameters
type ScreenFlashConfig struct {
	Red, Green, Blue, Intensity float64
	DurationMs                  int
}

// NewEnvironmentalHazards constructs the deterministic environmental hazard system.
func NewEnvironmentalHazards(rng randomIntn, callbacks SystemCallbacks) *EnvironmentalHazards {
	return &EnvironmentalHazards{
		rng:       rng,
		callbacks: callbacks,
	}
}

// SetCallbacks binds external effects without replacing the deterministic system.
func (h *EnvironmentalHazards) SetCallbacks(callbacks SystemCallbacks) {
	h.callbacks = callbacks
}

// damageUpToCurrentHP keeps large configured damage from wrapping through the
// character's uint16 HP field. The reported roll remains unchanged; only the
// amount passed to SubtractHp is capped to the HP that can actually be lost.
func damageUpToCurrentHP(damage int, currentHP uint16) uint16 {
	if damage < 0 {
		panic("environmental damage cannot be negative")
	}
	if damage >= int(currentHP) {
		return currentHP
	}

	return uint16(damage) //nolint:gosec // Nonnegative and strictly below current uint16 HP.
}

// ResolveTileHazards applies authoritative hazard state without presentation callbacks.
func (h *EnvironmentalHazards) ResolveTileHazards(
	tile *references.Tile,
	party *party_state.PartyState,
	location references.GeneralMapType,
) HazardResult {
	hazardType := h.detectHazardType(tile, location)

	switch hazardType {
	case PoisonSwamp:
		return h.checkSwampPoison(party, location)
	case LavaBurn, FireplaceBurn:
		return h.checkBurningHazard(party, hazardType)
	case NoHazard:
		fallthrough
	default:
		return HazardResult{Type: NoHazard, Affected: nil, MessageShown: false, DamageDealt: 0}
	}
}

// DetectTileHazard classifies a tile without consuming RNG or mutating party state.
func (h *EnvironmentalHazards) DetectTileHazard(tile *references.Tile, location references.GeneralMapType) HazardType {
	return h.detectHazardType(tile, location)
}

func (h *EnvironmentalHazards) detectHazardType(tile *references.Tile, location references.GeneralMapType) HazardType {
	if tile == nil {
		return NoHazard
	}

	// Tagless: SpriteIndex has hundreds of values and only these three are hazards.
	switch {
	case tile.Index == indexes.Swamp:
		return PoisonSwamp
	case tile.Index == indexes.Lava:
		return LavaBurn
	case tile.Index == indexes.Fireplace && location == references.SmallMapType:
		return FireplaceBurn // Only dangerous indoors
	}

	return NoHazard
}

func (h *EnvironmentalHazards) checkSwampPoison(
	party *party_state.PartyState,
	location references.GeneralMapType,
) HazardResult {
	result := HazardResult{Type: PoisonSwamp, Affected: nil, MessageShown: false, DamageDealt: 0}
	if location != references.SmallMapType && location != references.LargeMapType {
		return result
	}

	party.ForEachCharacter(func(index int, member *party_state.PlayerCharacter) {
		if member.IsDead() || member.IsPoisoned() || member.PartyMap != party_state.PartyMapActive {
			return // Skip dead, already poisoned, or not active party members
		}

		// Town poison checks draw 0..29 inclusive; outdoor checks draw 1..30 inclusive; ref:ENV-0002.
		poisonRoll := h.rng.Intn(PoisonRollMax)
		if location == references.LargeMapType {
			poisonRoll++
		}
		if poisonRoll > int(member.Dexterity) {
			member.SetStatus(party_state.Poisoned) // Modifies original - member is already a pointer
			result.Affected = append(result.Affected, index)
		}
	})

	return result
}

func (h *EnvironmentalHazards) checkBurningHazard(party *party_state.PartyState, hazardType HazardType) HazardResult {
	result := HazardResult{Type: hazardType, Affected: nil, MessageShown: false, DamageDealt: 0}

	// Each living member takes an independent inclusive 1..8 damage draw; ref:ENV-0001.
	totalDamage := 0
	party.ForEachCharacter(func(index int, member *party_state.PlayerCharacter) {
		if !member.IsDead() && member.PartyMap == party_state.PartyMapActive {
			damage := h.rng.Intn(BurnDamageMax) + BurnDamageMin // 1..8 range
			member.SubtractHp(damageUpToCurrentHP(damage, member.GetCurrentHp()))
			totalDamage += damage
			result.Affected = append(result.Affected, index)

			if member.GetCurrentHp() == 0 {
				member.MarkAsDead()
			}
		}
	})

	result.DamageDealt = totalDamage

	return result
}

// joinNames creates a grammatically correct list of names
func joinNames(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	case namesInPair:
		return names[0] + " and " + names[1]
	default:
		// For 3+ names: "Alice, Bob, and Charlie"
		result := ""
		for i, name := range names {
			switch {
			case i == 0:
				result = name
			case i == len(names)-1:
				result += ", and " + name
			default:
				result += ", " + name
			}
		}

		return result
	}
}

// ApplyDamageToParty applies damage to party members according to DamageConfig
// This provides a consistent damage application pattern for all damage sources
func (h *EnvironmentalHazards) ApplyDamageToParty(party *party_state.PartyState, config DamageConfig) HazardResult {
	// Generic damage uses NoHazard type
	result := HazardResult{Type: NoHazard, Affected: nil, MessageShown: false, DamageDealt: 0}
	totalDamage := 0

	// Determine which characters to affect
	targetIndices := config.TargetIndices
	if targetIndices == nil {
		// Default: affect all party members
		targetIndices = make([]int, party.GetCharacterCount())
		for i := range party.GetCharacterCount() {
			targetIndices[i] = i
		}
	}

	for _, index := range targetIndices {
		if index < 0 || index >= party.GetCharacterCount() {
			continue // Skip invalid indices
		}

		member := party.GetCharacter(index)
		if member.IsDead() || member.PartyMap != party_state.PartyMapActive {
			continue // Skip dead or inactive characters
		}

		// Calculate damage based on config
		var damage int
		if config.FixedDamage > 0 {
			damage = config.FixedDamage
		} else {
			// Random damage in range [MinDamage, MaxDamage]
			damage = h.rng.Intn(config.MaxDamage-config.MinDamage+1) + config.MinDamage
		}

		// Apply damage with proper clamping
		member.SubtractHp(damageUpToCurrentHP(damage, member.GetCurrentHp()))
		totalDamage += damage
		result.Affected = append(result.Affected, index)

		// Update status if character dies
		if member.GetCurrentHp() == 0 {
			member.MarkAsDead()
		}
	}

	result.DamageDealt = totalDamage

	// Apply audio/visual effects if damage was dealt and message specified
	if len(result.Affected) > 0 && config.Message != "" {
		// Build message with character names (reuse existing pattern)
		message := config.Message + ": "
		names := make([]string, 0, len(result.Affected))
		for _, memberIndex := range result.Affected {
			char := party.GetCharacter(memberIndex)
			name := char.GetNameAsString()
			if name != "" {
				names = append(names, name)
			}
		}

		if len(names) > 0 {
			message += joinNames(names)
			h.callbacks.AddRowStr(message)
			result.MessageShown = true
		} else {
			// Fallback if no names found
			h.callbacks.AddRowStr(config.Message + "!")
			result.MessageShown = true
		}
	}

	return result
}
