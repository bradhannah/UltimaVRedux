package game_state

import (
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/environment"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// Earthquake effect timing constants
const (
	earthquakeRandomFeedbackName = "earthquake_random_effect"

	// Random earthquake parameters (triggered in underworld)
	earthquakeRandomDurationMs = 4000 // 4 second shake duration
	earthquakeRandomPixels     = 10.0 // Shake pixel range

	// Audio fade timing
	earthquakeSoundFadeInMs  = 500 // Standard earthquake sound fade in
	earthquakeSoundFadeOutMs = 500 // Standard fade out
)

// Environmental hazard timing constants
const (
	turnHazardFeedbackName = "turn_hazards"

	// Poison atmospheric delay between warning and effect
	poisonAtmosphericDelayMs = 75

	// Dexterity check range for swamp poison (1-30)
	poisonDexterityCheckRange = 30
)

// messageCallbacksAdapter adapts GameState SystemCallbacks to environment.SystemCallbacks interface
type messageCallbacksAdapter struct {
	systemCallbacks *SystemCallbacks
}

func (m *messageCallbacksAdapter) AddRowStr(message string) {
	if m.systemCallbacks != nil && m.systemCallbacks.Message.AddRowStr != nil {
		m.systemCallbacks.Message.AddRowStr(message)
	}
}

// windSystemCallbacksAdapter adapts GameState SystemCallbacks to environment.WindSystemCallbacks interface
type windSystemCallbacksAdapter struct {
	systemCallbacks *SystemCallbacks
}

func (w *windSystemCallbacksAdapter) ShowWindUI(_ environment.WindDirection) {
	// The original Ultima V shows no message when the wind changes.
}

func (w *windSystemCallbacksAdapter) AddRowStr(message string) {
	if w.systemCallbacks != nil && w.systemCallbacks.Message.AddRowStr != nil {
		w.systemCallbacks.Message.AddRowStr(message)
	}
}

type playerUpdateStatusCache struct {
	Statuses [party_state.MaxActiveParty]party_state.CharacterStatus
	Active   [party_state.MaxActiveParty]bool
	Diners   int
}

func (g *GameState) cachePlayerUpdateStatuses() playerUpdateStatusCache {
	result := playerUpdateStatusCache{}
	for index := range party_state.MaxActiveParty {
		character := g.PartyState.GetCharacter(index)
		if character == nil || character.PartyMap != party_state.PartyMapActive {
			continue
		}
		result.Active[index] = true
		result.Statuses[index] = character.GetStatus()
		if result.Statuses[index] != party_state.Dead && result.Statuses[index] != party_state.Sleep {
			result.Diners++
		}
	}
	if active := g.activePlayerIndex; active >= 0 && result.Active[active] && result.Statuses[active] == party_state.Dead {
		g.activePlayerIndex = -1
	}

	return result
}

func (g *GameState) applyCachedPlayerUpdatePoison(cache playerUpdateStatusCache, dungeonFeedback bool) []int {
	result := g.resolveCachedPlayerUpdatePoison(cache)
	if dungeonFeedback {
		g.presentDungeonInjury(dungeonInjuryResult{kind: DungeonInjuryPoison, hpLost: result.hpLost})
	}

	return result.deadIndexes
}

type playerUpdatePoisonResult struct {
	deadIndexes []int
	hpLost      int
}

func (g *GameState) resolveCachedPlayerUpdatePoison(cache playerUpdateStatusCache) playerUpdatePoisonResult {
	result := playerUpdatePoisonResult{deadIndexes: make([]int, 0), hpLost: 0}
	for index := range party_state.MaxActiveParty {
		if !cache.Active[index] || cache.Statuses[index] != party_state.Poisoned {
			continue
		}
		character := g.PartyState.GetCharacter(index)
		before := character.GetCurrentHp()
		character.SubtractHp(1)
		result.hpLost += int(before - character.GetCurrentHp())
		if before > 0 && character.IsDead() {
			result.deadIndexes = append(result.deadIndexes, index)
		}
	}

	return result
}

// processPoisonedCharactersForPlayerUpdate applies the updateplayers poison step.
func (g *GameState) processPoisonedCharactersForPlayerUpdate(cache playerUpdateStatusCache) {
	deadIndexes := g.applyCachedPlayerUpdatePoison(cache, false)
	if len(deadIndexes) == 0 && !g.PartyState.HasPoisonedCharacter() {
		return
	}

	// Handle character deaths from poison
	if len(deadIndexes) > 0 {
		for _, charIndex := range deadIndexes {
			charName := g.PartyState.GetCharacter(charIndex).GetNameAsString()
			g.SystemCallbacks.Message.AddRowStr(fmt.Sprintf("Alas! %s has died!", charName))
		}
		g.ShowDeathFeedback()

		return // Death feedback already shown, skip poison flash
	}

	// Show poison visual feedback (green flash + sound) if anyone is still poisoned
	if g.PartyState.HasPoisonedCharacter() {
		g.ShowPoisonFeedback()
	}
}

// ProcessEnvironmentalHazardsOnTurnAdvancement should be called during turn processing for standing-on-tile effects
func (g *GameState) ProcessEnvironmentalHazardsOnTurnAdvancement() {
	if g.environmentalHazards == nil {
		// System corruption: environmental hazards should be initialized in GameState constructor
		panic("ProcessEnvironmentalHazardsOnTurnAdvancement: environmentalHazards is nil")
	}

	mapType := g.MapState.PlayerLocation.Location.GetMapType()
	if g.CombatScene != nil || mapType == references.CombatMapType {
		return // Combat terrain hazards resolve against the acting local combatant.
	}
	currentTile := g.GetCurrentAvatarGroundTile()
	// Original town rules and original outdoor rules require the foot-player shape
	// for swamp exposure only. Lava and existing poison still affect riders.
	// See ref:ENV-0002.
	if g.PartyVehicle.GetVehicleDetails().VehicleType != references.NoPartyVehicle &&
		g.detectTileHazardType(currentTile, mapType) == environment.PoisonSwamp {
		return
	}
	hazardResult := g.environmentalHazards.ResolveTileHazards(
		currentTile,
		&g.PartyState,
		mapType,
	)
	if hazardResult.Type == environment.NoHazard {
		return
	}

	if hazardResult.Type == environment.PoisonSwamp && len(hazardResult.Affected) > 0 {
		g.SystemCallbacks.UI.ShowTransientIcon(TransientIconPoisonOnset)
	}
	g.triggerTurnHazardFeedback(hazardResult)
}

// detectTileHazardType is a wrapper around the environmental hazards detectHazardType method
func (g *GameState) detectTileHazardType(tile *references.Tile, location references.GeneralMapType) environment.HazardType {
	if g.environmentalHazards == nil {
		// System corruption: environmental hazards should be initialized in GameState constructor
		panic("detectTileHazardType: environmentalHazards is nil")
	}

	return g.environmentalHazards.DetectTileHazard(tile, location)
}

func (g *GameState) triggerTurnHazardFeedback(result environment.HazardResult) {
	if len(result.Affected) == 0 {
		return
	}
	targets := append([]int(nil), result.Affected...)
	g.EventQueueManager.TriggerFeedback(turnHazardFeedbackName, event_queue.PriorityNormal,
		func(sequence *event_queue.SequentialEventBuilder) {
			builder := NewPartyStateBuilder(g).
				ToCharacters(targets)
			message := ""
			switch result.Type {
			case environment.LavaBurn, environment.FireplaceBurn:
				message = g.hazardAffectedMessage("Burning", targets)
				builder.WithFlash(
					environment.FlashDurationMs,
					environment.FlashColorRed,
					environment.FlashColorGreen,
					environment.FlashColorBlue,
					environment.FlashIntensity,
				)
			case environment.PoisonSwamp:
				message = g.hazardAffectedMessage("Poisoned", targets)
				builder.FlashGreen()
			case environment.NoHazard:
				panic("turn hazard feedback requires a hazard")
			default:
				panic("turn hazard feedback received an unsupported hazard")
			}

			sequence.First(func() {
				g.SystemCallbacks.Message.AddRowStr(message)
				if result.Type == environment.LavaBurn || result.Type == environment.FireplaceBurn {
					// REDUX DEVIATION: original town rules town status processing and original outdoor effect rules burning damage
					// route burning through generic damage noise. Redux uses semantic fire feedback.
					// See ref:ENV-0002.
					g.SystemCallbacks.Audio.PlaySoundEffect(SoundFireBurn)
				}
			}).ThenSequence(builder.Build())
		},
	)
}

func (g *GameState) hazardAffectedMessage(prefix string, targets []int) string {
	names := make([]string, 0, len(targets))
	for _, memberIndex := range targets {
		if memberIndex < 0 || memberIndex >= g.PartyState.GetCharacterCount() {
			continue
		}
		name := g.PartyState.GetCharacter(memberIndex).GetNameAsString()
		if name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return prefix + "!"
	}

	return prefix + ": " + joinNames(names)
}

// thenApplyHazardEffects returns an event queue modifier that applies environmental hazard effects
// based on the detected hazard type. This shared helper is used by both turn advancement and fall workflows
// to avoid nested event queue creation while maintaining consistent visual effects.
func (g *GameState) thenApplyHazardEffects(hazardType environment.HazardType) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		switch hazardType {
		case environment.LavaBurn, environment.FireplaceBurn:
			// Show warning message first
			q = q.Then(func() {
				if hazardType == environment.LavaBurn {
					g.SystemCallbacks.Message.AddRowStr("Ouch! Hot!")
				} else {
					g.SystemCallbacks.Message.AddRowStr("Burning!")
				}
			})

			// Apply fire damage + visual effects using PartyStateBuilder
			q = q.ThenSequence(
				NewPartyStateBuilder(g).
					WithRandomDamage(1, 8).
					ToAllParty().
					WithFlash(
						environment.FlashDurationMs,
						environment.FlashColorRed,
						environment.FlashColorGreen,
						environment.FlashColorBlue,
						environment.FlashIntensity,
					).
					WithSound(SoundFireBurn).
					Build())

			return q

		case environment.PoisonSwamp:
			// Perform dexterity checks to see who gets poisoned
			poisonTargets := g.performSwampPoisonDexterityChecks()

			if len(poisonTargets) == 0 {
				return q // No one to poison
			}

			// Show atmospheric warning
			q = q.Then(func() {
				g.SystemCallbacks.Message.AddRowStr("Poisonous vapors rise from the swamp...")
			})

			// Apply visual effects (green mist)
			q = q.ThenSequence(
				NewPartyStateBuilder(g).
					ToAllParty().
					FlashGreen().
					WithSound(SoundFireBurn).
					Build())

			// Brief pause for atmosphere
			q = q.Wait(poisonAtmosphericDelayMs)

			// Apply poison status and play sickness sound
			q = q.Then(func() {
				g.applySwampPoisonToTargets(poisonTargets)
				g.SystemCallbacks.Audio.PlaySoundEffect(SoundHeal) // Coughing sound placeholder
			})

			return q

		case environment.NoHazard:
			return q

		default:
			return q
		}
	}
}

// performSwampPoisonDexterityChecks performs dexterity checks for swamp poison resistance
// Returns indices of characters who failed their checks and will be poisoned
func (g *GameState) performSwampPoisonDexterityChecks() []int {
	var poisonTargets []int
	if g.PartyVehicle.GetVehicleDetails().VehicleType != references.NoPartyVehicle {
		return poisonTargets
	}
	mapType := g.MapState.PlayerLocation.Location.GetMapType()
	if g.CombatScene != nil || (mapType != references.SmallMapType && mapType != references.LargeMapType) {
		return poisonTargets
	}

	g.PartyState.ForEachCharacter(func(i int, member *party_state.PlayerCharacter) {
		// Skip dead, already poisoned, or inactive party members
		if member.IsDead() ||
			member.IsPoisoned() ||
			member.PartyMap != party_state.PartyMapActive {
			return
		}

		// Original town rules use random(0,29); swamp exposure uses random(1,30).
		// See ref:ENV-0002.
		poisonRoll := g.rng.Intn(poisonDexterityCheckRange)
		if mapType == references.LargeMapType {
			poisonRoll++
		}
		if poisonRoll > int(member.Dexterity) {
			poisonTargets = append(poisonTargets, i)
		}
	})

	return poisonTargets
}

// applySwampPoisonToTargets applies poison status to the specified party members
func (g *GameState) applySwampPoisonToTargets(poisonTargets []int) {
	if len(poisonTargets) == 0 {
		return
	}

	poisonedNames := make([]string, 0, len(poisonTargets))
	changes := transientStatusChanges{}

	// Apply poison status to failed characters
	for _, memberIndex := range poisonTargets {
		if memberIndex >= 0 && memberIndex < g.PartyState.GetCharacterCount() {
			member := g.PartyState.GetCharacter(memberIndex)
			statusBefore := member.GetStatus()
			member.SetStatus(party_state.Poisoned)
			changes.observePoison(statusBefore, member)

			// Extract character name for message
			name := member.GetNameAsString()
			if name != "" {
				poisonedNames = append(poisonedNames, name)
			}
		}
	}
	g.publishTransientStatusChanges(changes)

	// Show poisoned message with character names
	if len(poisonedNames) > 0 {
		message := "Poisoned: " + joinNames(poisonedNames)
		g.SystemCallbacks.Message.AddRowStr(message)
	}
}

// joinNames creates a grammatically correct list of names
func joinNames(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	case 2:
		return names[0] + " and " + names[1]
	default:
		// For 3+ names: "Alice, Bob, and Charlie"
		result := ""
		for i, name := range names {
			switch i {
			case 0:
				result = name
			case len(names) - 1:
				result += ", and " + name
			default:
				result += ", " + name
			}
		}

		return result
	}
}

// Wind System Methods

// ProcessWindThroughElapsed advances the deterministic wind clock through elapsedMs.
func (g *GameState) ProcessWindThroughElapsed(elapsedMs int64) {
	if g.windSystem == nil {
		panic("ProcessWindThroughElapsed: windSystem is nil")
	}
	g.windSystem.AdvanceThrough(elapsedMs)
}

// SetWindDirection manually sets wind direction (used by spells, debug commands)
func (g *GameState) SetWindDirection(direction environment.WindDirection, immediate bool) {
	if g.windSystem == nil {
		// System corruption: wind system should be initialized in GameState constructor
		panic("SetWindDirection: windSystem is nil")
	}

	g.windSystem.SetWind(direction, immediate)
}

// earthquakeCallbacksAdapter presents an earthquake after authoritative damage commits.
type earthquakeCallbacksAdapter struct {
	gameState *GameState
}

func (a *earthquakeCallbacksAdapter) PresentEarthquake(_ EarthquakeResult) {
	a.gameState.SystemCallbacks.Message.AddRowStr("EARTHQUAKE!")
	// REDUX DEVIATION: earthquake check blocks in earthquake feedback. Redux
	// presents nonblocking feedback after damage so command completion remains synchronous.
	// See ref:ENV-0004.
	a.gameState.EventQueueManager.TriggerFeedback(
		earthquakeRandomFeedbackName,
		event_queue.PriorityNormal,
		func(sequence *event_queue.SequentialEventBuilder) {
			var voice SoundVoice = SilentSoundVoice{}
			sequence.First(func() {
				voice = a.gameState.SystemCallbacks.Audio.Play(SoundEarthquake).FadeIn(earthquakeSoundFadeInMs).StartOwned()
			}).Animate(earthquakeRandomDurationMs, func(progress float32) {
				x, y := calculateScreenShakeOffset(progress, earthquakeRandomPixels*(1-float64(progress)))
				a.gameState.SystemCallbacks.Visual.SetScreenShakeOffset(x, y)
			}).Finally(func() {
				a.gameState.SystemCallbacks.Visual.SetScreenShakeOffset(0, 0)
				voice.Stop(earthquakeSoundFadeOutMs)
			})
		},
	)
}

// Earthquake System Methods

// checkForUnderworldEarthquake checks for random earthquakes after movement in underworld
func (g *GameState) checkForUnderworldEarthquake() {
	if g.earthquakeSystem == nil {
		// System corruption: earthquake system should be initialized in GameState constructor
		panic("checkForUnderworldEarthquake: earthquakeSystem is nil")
	}

	g.earthquakeSystem.CheckForRandomEarthquake(g.MapState.IsUnderworld(), &g.PartyState)
}

// SetWindByDirection sets wind using references.Direction (for spell integration)
func (g *GameState) SetWindByDirection(direction references.Direction, immediate bool) bool {
	if g.windSystem == nil {
		// System corruption: wind system should be initialized in GameState constructor
		panic("SetWindByDirection: windSystem is nil")
	}

	return g.windSystem.SetWindByDirection(direction, immediate)
}

// GetCurrentWind returns the current wind direction
func (g *GameState) GetCurrentWind() environment.WindDirection {
	if g.windSystem == nil {
		// System corruption: wind system should be initialized in GameState constructor
		panic("GetCurrentWind: windSystem is nil")
	}

	return g.windSystem.GetCurrentWind()
}

// GetPirateShipSpeedMultiplier returns speed multiplier for pirate ships based on current wind
func (g *GameState) GetPirateShipSpeedMultiplier(shipDirection references.Direction) int {
	if g.windSystem == nil {
		// System corruption: wind system should be initialized in GameState constructor
		panic("GetPirateShipSpeedMultiplier: windSystem is nil")
	}

	return g.windSystem.GetPirateShipSpeedMultiplier(shipDirection)
}

// IsWindCalm returns true if current wind is calm
func (g *GameState) IsWindCalm() bool {
	if g.windSystem == nil {
		// System corruption: wind system should be initialized in GameState constructor
		panic("IsWindCalm: windSystem is nil")
	}

	return g.windSystem.IsWindCalm()
}
