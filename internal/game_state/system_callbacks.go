package game_state

import (
	"errors"
	"image/color"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/pkg/semantic_input"
)

// Sentinel errors for missing mandatory callbacks.
var (
	errUICallbackRequired             = errors.New("UICallbacks.ShowActionFeedback is required")
	errSearchFeedbackCallbackRequired = errors.New("UICallbacks.ShowSearchFeedback is required")
	errKapowCallbackRequired          = errors.New("VisualCallbacks.PlayKapowAtPosition is required")
	errCombatImpactCallbackRequired   = errors.New("VisualCallbacks.ShowCombatImpact is required")
	errCombatEventCallbackRequired    = errors.New("CombatCallbacks.PublishEvent is required")
	errCombatConclusionRequired       = errors.New("CombatCallbacks.PublishConclusion is required")
	errZStatsCallbackRequired         = errors.New("UICallbacks.ShowZStatsDossier is required")
	errQuestJournalCallbackRequired   = errors.New("UICallbacks.ShowQuestJournalDossier is required")
	errJournalChangedCallbackRequired = errors.New("UICallbacks.JournalChanged is required")
	errAtlasUpdatedCallbackRequired   = errors.New("UICallbacks.AtlasUpdated is required")
	errTransientIconCallbackRequired  = errors.New("UICallbacks.ShowTransientIcon is required")
	errExperienceAwardRequired        = errors.New("UICallbacks.ShowExperienceAward is required")
	errSignDialogCallbackRequired     = errors.New("UICallbacks.ShowSignDialog is required")
	errSpellExpiredCallbackRequired   = errors.New("UICallbacks.ShowSpellExpiredPopup is required")
)

// SoundEffect represents different types of sound effects
type SoundEffect int

// Sound effect identities are append-only so authored cue mappings remain stable.
const (
	SoundCannonFire SoundEffect = iota
	SoundBroadside
	SoundGlassBreak
	SoundEarthquake
	SoundSummon
	SoundAbsorb
	SoundPushObject
	SoundOpenDoor
	SoundCloseDoor
	SoundUnlock
	SoundTrapTrigger
	SoundStepOnTrap
	SoundFireBurn
	SoundHeal
	SoundMoongateTravel
	// SoundWalkGrassLeft begins the variants used for natural footstep cycling.
	SoundWalkGrassLeft
	SoundWalkGrassRight
	SoundWalkStone1
	SoundWalkStone2
	SoundWalkStone3
	SoundWalkStone4
	SoundWalkWood1
	SoundWalkWood2
	SoundWalkWood3
	SoundWalkMetal1
	// SoundAmbientHum represents ambient humming.
	SoundAmbientHum
	// SoundClimbingSuccess marks successful climbing; falling uses separate cues.
	SoundClimbingSuccess
	SoundFallingMinor
	SoundFallingSevere
	SoundWhiteNoiseDamage
	// SoundGoodNight is the bedtime cue in the rest and sleep sound family.
	SoundGoodNight      // Peaceful bedtime chime/melody
	SoundRoosterCrow    // Morning wake-up rooster
	SoundThrownOutOfBed // Startled/surprised sound for NPC interruption
	SoundStarvation     // Hunger/weakness sound for starvation damage
	SoundDeath          // Somber bell for character death
	// SoundBardLute accompanies the camping guard's musical performance.
	SoundBardLute           // Bard playing lute during camping
	SoundFireCrackle        // Campfire crackling sound (background ambient)
	SoundCampingStart       // Short pleasant sound when camping begins
	SoundPartyRested        // Completion sound when camping finishes successfully
	SoundNoEffect           // Subtle "dud" sound when no healing occurred
	SoundAmbushAlert        // Alert/danger sound when ambushed during camping
	SoundGuardFootstepGrass // Single grass footstep for guard wandering
	SoundHealingTick        // Small healing sound per character
	// SoundMagic accompanies magical effects such as the Wanderer's appearance.
	SoundMagic // Magical effect sound for wanderer appearance
	SoundApparitionAppears
	SoundLevelUp // Character level up sound
	SoundWind
	SoundFountainDrink // Fountain drinking sound
	// SoundCoinSplash marks a coin dropped into a wishing well.
	SoundCoinSplash // Coin dropped into wishing well
	SoundMagicPoof  // Horse spawns from wish
	// SoundWhirlpoolVortex is the looping water cue for whirlpool transport.
	SoundWhirlpoolVortex // Swirling water vortex (looping)
	SoundWhirlpoolSplash // Teleport splash/whoosh
	SoundHullCrunch      // Hull takes damage from whirlpool
	// SoundShoalScrape marks a ship collision with shoals.
	SoundShoalScrape // Ship hits shoals
	// SoundMoveHorse begins the vehicle movement cues (some recordings remain empty).
	SoundMoveHorse   // Horse galloping/hooves
	SoundMoveSkiff   // Skiff rowing/water
	SoundMoveFrigate // Frigate sailing/creaking
	SoundMoveCarpet  // Magic carpet whoosh
	// SoundTrollGrowl marks the start of a troll bridge encounter.
	SoundTrollGrowl  // Troll appearance/threat
	SoundTrollCaught // Alert sound when caught by trolls
	// SoundShadowlordSummon begins the Shadowlord presence cues.
	SoundShadowlordSummon   // Original successful-Yell pulse accompanying the summon fizz
	SoundShadowlordPresence // Ominous sound when Shadowlord is present at town entry
	SoundShardPulse         // Original 2640 Hz pulse sweep when a Shard is raised
	SoundShadowlordVictory  // Victory cue after successful permanent destruction
	// SoundDeathVision accompanies a failed crystal-ball vision.
	SoundDeathVision   // Eerie/unsettling sound for failed crystal ball vision
	SoundStrangeVision // Mystical/mysterious sound for successful crystal ball vision
	// SoundCure accompanies poison removal in the potion-effect sound family.
	SoundCure          // Cure poison effect (cleansing chime)
	SoundAwaken        // Wake from sleep effect (alert/bell sound)
	SoundPoison        // Apply poison effect (bubbling/toxic)
	SoundSleep         // Apply sleep effect (drowsy/lullaby)
	SoundXRay          // X-Ray vision activation (mystical reveal)
	SoundSpellFailure  // Spell/potion failure (fizzle/dud)
	SoundGuardianBlock // Codex guardian blocks passage (placeholder)
	// SoundShrineQuestAssign accompanies a shrine quest assignment.
	// Original frequencies from original shrine rules - to be synthesized as authentic PC speaker tones
	// See ref:CMD-0001.
	SoundShrineQuestAssign // Quest assignment fanfare
	// Original: 7 pulse tones at 3300, 3925, 3925, 3925, 3925, 3700, 3925 Hz
	// Durations: 7000, 6000, 3000, 3000, 3000, 3000, 8000 ms

	// SoundShrineOffering accompanies an accepted shrine offering.
	SoundShrineOffering // Gold offering accepted
	// Original: 2700 Hz pulse sweep (ascending 2000→25000, descending 25000→2000, step 50)

	// SoundShrineReward accompanies shrine quest completion.
	SoundShrineReward // Quest completion reward
	// Original: 3100 Hz pulse sweep (ascending 2000→25000, descending 25000→2000, step 50)

	// SoundAwakenSpell begins the spell-specific cues.
	// Spell-specific sounds (detached from caller - usable by spells, scrolls, potions)
	SoundAwakenSpell // An Zu spell - wake sleeping character
	SoundCreateFood  // In Xen Mani spell - conjure food
	SoundLocate      // In Wis spell - display coordinates
	SoundGreatHeal   // Vas Mani spell - major healing
	SoundResurrect   // In Mani Corp spell - raise the dead
	SoundMagicUnlock // An Ex Por spell - unlock magic door
	SoundMagicLock   // In Ex Por spell - magic lock door
	SoundDispel      // An Grav spell - dispel magic field
	// SoundKarmaUp provides positive karma feedback; decreases use a separate cue.
	SoundKarmaUp   // Pleasant chime for karma increase
	SoundKarmaDown // Warning tone for karma decrease
	// SoundTavernBardMusic is tavern bard ambient music
	SoundTavernBardMusic
	// SoundShardRitualEarthquake is the workflow-owned loop for Shard destruction.
	SoundShardRitualEarthquake
	// SoundHarpsichord0 begins the reconstructed harpsichord notes for digits 0-9.
	SoundHarpsichord0
	SoundHarpsichord1
	SoundHarpsichord2
	SoundHarpsichord3
	SoundHarpsichord4
	SoundHarpsichord5
	SoundHarpsichord6
	SoundHarpsichord7
	SoundHarpsichord8
	SoundHarpsichord9
	// SoundKeyBreak starts the semantic outcome slots, whose recordings are empty until authored.
	SoundKeyBreak
	SoundItemPickup
	SoundQuestPickup
	SoundEquip
	SoundUnequip
	SoundRingVanish
	SoundTorchIgnite
	SoundVehicleBoard
	SoundVehicleExit
	SoundPotionDrink
	SoundScrollInvoke
	SoundMixComplete
	SoundCombatSwing
	SoundCombatMiss
	SoundCombatGraze
	SoundCombatHit
	SoundCombatShatter
	SoundCombatVictory
	SoundCombatFlee
	SoundSpellImpact
	SoundMerchantTransaction
	SoundLocationEnter
	SoundCombatRangedLaunch
	// SoundMonsterGate starts the signature cues, with empty recording slots until authored.
	SoundMonsterGate
	SoundMonsterPossession
	SoundMonsterTeleport
	SoundCorpserCapture
	SoundCorpserRelease
	SoundCombatInterrupted
	// SoundElectricalDamage accompanies resolved electrical-field injury.
	SoundElectricalDamage
)

// HarpsichordSoundEffectForDigit returns the note sound assigned to a digit.
func HarpsichordSoundEffectForDigit(digit int) (SoundEffect, bool) {
	sounds := [...]SoundEffect{
		SoundHarpsichord0,
		SoundHarpsichord1,
		SoundHarpsichord2,
		SoundHarpsichord3,
		SoundHarpsichord4,
		SoundHarpsichord5,
		SoundHarpsichord6,
		SoundHarpsichord7,
		SoundHarpsichord8,
		SoundHarpsichord9,
	}
	if digit < 0 || digit >= len(sounds) {
		return 0, false
	}

	return sounds[digit], true
}

// SystemCallbacks provides comprehensive dependency injection for all external systems
// that game logic needs to interact with, maintaining separation of concerns.
// Use NewSystemCallbacks() to create with validation.
type SystemCallbacks struct {
	// Message Output System
	Message MessageCallbacks

	// Visual Effects System
	Visual VisualCallbacks

	// Audio System
	Audio AudioCallbacks

	// Screen Update System
	Screen ScreenCallbacks

	// Game Flow System
	Flow FlowCallbacks

	// Talk Dialog System
	Talk TalkCallbacks

	// UI Dialog System
	UI UICallbacks

	// Structured combat event output.
	Combat CombatCallbacks
}

// VisualCallbacks handles visual effects
type VisualCallbacks struct {
	// SetWandererDissolveLevel sets wanderer transparency (0.0 = fully visible, 1.0 = fully dissolved)
	SetWandererDissolveLevel func(level float64)

	// InitializeWandererDissolveEffect creates the GPU-accelerated dissolve effect for wanderer
	InitializeWandererDissolveEffect func()

	// CleanupWandererDissolveEffect releases wanderer dissolve effect resources
	CleanupWandererDissolveEffect func()

	// SetAvatarDissolveLevel sets avatar transparency for teleportation (0.0 = fully visible, 1.0 = fully dissolved)
	SetAvatarDissolveLevel func(level float64)

	// InitializeAvatarDissolveEffect creates the GPU-accelerated dissolve effect for avatar
	InitializeAvatarDissolveEffect func()

	// CleanupAvatarDissolveEffect releases avatar dissolve effect resources
	CleanupAvatarDissolveEffect func()

	// SetMapFadeLevel sets map fade to black (0.0 = fully visible, 1.0 = fully black)
	SetMapFadeLevel func(level float64)

	// Event queue-friendly screen flash control
	// SetScreenFlashColor sets the flash color without triggering timing
	SetScreenFlashColor func(r, g, b float64)
	// SetScreenFlashIntensity sets current flash intensity (0.0-1.0), event queue controls fade
	SetScreenFlashIntensity func(intensity float64)

	// Event queue-friendly screen shake control
	// SetScreenShakeOffset sets camera offset in pixels, event queue controls jolt pattern
	SetScreenShakeOffset func(x, y float64)

	// SetScreenInvert applies screen color inversion effect (X-ray style)
	// intensity: 0.0 (normal) to 1.0 (fully inverted)
	// durationMs: how long to sustain the effect
	// IMPORTANT: Should ONLY be called from:
	//   1. Inside event queues with proper Wait() sequencing
	//   2. Via PartyStateBuilder.FlashInverted() which handles timing internally
	// Direct calls without timing control result in fire-and-forget behavior.
	SetScreenInvert func(intensity float64, durationMs int)

	// BlockAllInput enables or disables all game-window input.
	BlockAllInput func(blocked bool)

	// IsAnyKeyPressed checks if any key is currently pressed for event queue continuation
	IsAnyKeyPressed func() bool

	// GetPressedKey returns the currently pressed key for event queue key-aware loops
	// Returns nil if no key is pressed
	// Used by WhileAwaitingInput() for automatic sailing direction changes and stop detection
	GetPressedKey func() *semantic_input.Key

	// ResetTeleportationSourceRenderer clears moongate renderers from the source location.
	ResetTeleportationSourceRenderer func()

	// StabilizeTeleportationDestinationRenderer prepares the destination moongate for the fade-in.
	StabilizeTeleportationDestinationRenderer func(destination MoongateDestination)

	// Projectile system callbacks

	// StartProjectile begins a projectile animation from source to target position
	// Returns duration in milliseconds for use with Animate()
	StartProjectile func(fromPos, toPos references.Position, missileType references.MissileType) uint64

	// UpdateProjectileProgress updates projectile position based on progress (0.0 to 1.0)
	// Called from Animate() callback each frame
	UpdateProjectileProgress func(progress float32)

	// ClearProjectile removes the active projectile after animation completes
	ClearProjectile func()

	// PlayKapowAtPosition shows an explosion effect at the specified world position
	// Used for cannon hits, combat impacts, etc.
	PlayKapowAtPosition func(pos references.Position)

	// ShowCombatImpact starts local feedback for one resolved combat result.
	ShowCombatImpact func(CombatAttackFeedback)

	// ShowDungeonInjury starts a dedicated HP-loss pulse; None clears it.
	// Feedback owns sequencing/cleanup, independently of ordinary screen flashes.
	ShowDungeonInjury func(DungeonInjuryKind)

	// Note: Death scene apparition dissolve callbacks removed - now uses generic tile dissolve system
	// See gamescene_dissolve.go: StartTileDissolveWithOptions(), UpdateTileDissolveProgress(), StopTileDissolve()
}

// CombatFeedbackResult identifies the floating result shown for an attack.
type CombatFeedbackResult int

const (
	// CombatFeedbackMiss indicates that the attack did not connect.
	CombatFeedbackMiss CombatFeedbackResult = iota
	// CombatFeedbackGraze indicates that armor deflected all damage.
	CombatFeedbackGraze
	// CombatFeedbackDamage indicates that the target lost hit points.
	CombatFeedbackDamage
	// CombatFeedbackPoison indicates that the target became poisoned.
	CombatFeedbackPoison
	// CombatFeedbackSleep indicates that a Gazer put the target to sleep.
	CombatFeedbackSleep
	// CombatFeedbackTheft indicates that a Gremlin stole party food.
	CombatFeedbackTheft
)

// CombatFeedbackEntity identifies one rendered combatant.
type CombatFeedbackEntity struct {
	IsPartyMember bool
	Index         int
	Slot          int
	Position      references.Position
}

// CombatAttackDelivery identifies how an attack reaches its target for presentation.
type CombatAttackDelivery int

const (
	// CombatAttackDeliveryUnknown is reserved for feedback without a classified source.
	CombatAttackDeliveryUnknown CombatAttackDelivery = iota
	// CombatAttackDeliveryMelee covers adjacent physical and natural attacks.
	CombatAttackDeliveryMelee
	// CombatAttackDeliveryProjectile covers physical and monster missile attacks.
	CombatAttackDeliveryProjectile
	// CombatAttackDeliverySpell covers direct spell attacks.
	CombatAttackDeliverySpell
	// CombatAttackDeliveryEnvironmental covers fields and other source-less hazards.
	CombatAttackDeliveryEnvironmental
)

// CombatAttackPresentation describes the stable inputs used to select attack feedback visuals.
type CombatAttackPresentation struct {
	Delivery CombatAttackDelivery
	Source   references.Equipment
}

// CombatAttackFeedback describes one complete visual attack response.
type CombatAttackFeedback struct {
	Attacker     CombatFeedbackEntity
	Target       CombatFeedbackEntity
	Result       CombatFeedbackResult
	HPLost       int
	Fatal        bool
	Presentation CombatAttackPresentation
}

// NewVisualCallbacks validates a complete named set of visual capabilities.
func NewVisualCallbacks(callbacks VisualCallbacks) VisualCallbacks {
	if callbacks.SetWandererDissolveLevel == nil {
		panic("VisualCallbacks.SetWandererDissolveLevel is required - cannot be nil")
	}
	if callbacks.InitializeWandererDissolveEffect == nil {
		panic("VisualCallbacks.InitializeWandererDissolveEffect is required - cannot be nil")
	}
	if callbacks.CleanupWandererDissolveEffect == nil {
		panic("VisualCallbacks.CleanupWandererDissolveEffect is required - cannot be nil")
	}
	if callbacks.SetAvatarDissolveLevel == nil {
		panic("VisualCallbacks.SetAvatarDissolveLevel is required - cannot be nil")
	}
	if callbacks.InitializeAvatarDissolveEffect == nil {
		panic("VisualCallbacks.InitializeAvatarDissolveEffect is required - cannot be nil")
	}
	if callbacks.CleanupAvatarDissolveEffect == nil {
		panic("VisualCallbacks.CleanupAvatarDissolveEffect is required - cannot be nil")
	}
	if callbacks.SetMapFadeLevel == nil {
		panic("VisualCallbacks.SetMapFadeLevel is required - cannot be nil")
	}
	if callbacks.SetScreenFlashColor == nil {
		panic("VisualCallbacks.SetScreenFlashColor is required - cannot be nil")
	}
	if callbacks.SetScreenFlashIntensity == nil {
		panic("VisualCallbacks.SetScreenFlashIntensity is required - cannot be nil")
	}
	if callbacks.SetScreenShakeOffset == nil {
		panic("VisualCallbacks.SetScreenShakeOffset is required - cannot be nil")
	}
	if callbacks.SetScreenInvert == nil {
		panic("VisualCallbacks.SetScreenInvert is required - cannot be nil")
	}
	if callbacks.BlockAllInput == nil {
		panic("VisualCallbacks.BlockAllInput is required - cannot be nil")
	}
	if callbacks.IsAnyKeyPressed == nil {
		panic("VisualCallbacks.IsAnyKeyPressed is required - cannot be nil")
	}
	if callbacks.GetPressedKey == nil {
		panic("VisualCallbacks.GetPressedKey is required - cannot be nil")
	}
	if callbacks.ResetTeleportationSourceRenderer == nil {
		panic("VisualCallbacks.ResetTeleportationSourceRenderer is required - cannot be nil")
	}
	if callbacks.StabilizeTeleportationDestinationRenderer == nil {
		panic("VisualCallbacks.StabilizeTeleportationDestinationRenderer is required - cannot be nil")
	}
	if callbacks.StartProjectile == nil {
		panic("VisualCallbacks.StartProjectile is required - cannot be nil")
	}
	if callbacks.UpdateProjectileProgress == nil {
		panic("VisualCallbacks.UpdateProjectileProgress is required - cannot be nil")
	}
	if callbacks.ClearProjectile == nil {
		panic("VisualCallbacks.ClearProjectile is required - cannot be nil")
	}
	if callbacks.PlayKapowAtPosition == nil {
		panic("VisualCallbacks.PlayKapowAtPosition is required - cannot be nil")
	}
	if callbacks.ShowCombatImpact == nil {
		panic("VisualCallbacks.ShowCombatImpact is required - cannot be nil")
	}
	if callbacks.ShowDungeonInjury == nil {
		panic("VisualCallbacks.ShowDungeonInjury is required - cannot be nil")
	}

	return callbacks
}

// AudioCallbacks handles typed sound requests and music lifecycle operations.
type AudioCallbacks struct {
	// Play returns a SoundBuilder for configuring and playing sound effects
	Play func(effect SoundEffect) *SoundBuilder

	// Stop returns a StopBuilder for configuring how to stop sound effects
	Stop func(effect SoundEffect) *StopBuilder

	// PlaySoundEffect dispatches a full-volume one-shot request.
	PlaySoundEffect func(effect SoundEffect)

	// RequestPlayback dispatches one fully configured playback request.
	RequestPlayback func(request SoundPlaybackRequest)
	// RequestVoice dispatches one independently owned presentation instance.
	RequestVoice func(request SoundPlaybackRequest) SoundVoice
	// RequestStop dispatches one fully configured stop request.
	RequestStop func(request SoundStopRequest)

	// UpdateLocationMusic updates background music based on current location
	UpdateLocationMusic func()
	// PauseBackgroundMusic temporarily pauses background music (for camping, dialogs, etc.)
	PauseBackgroundMusic func()
	// ResumeBackgroundMusic resumes previously paused background music
	ResumeBackgroundMusic func()
}

// NewAudioCallbacks creates AudioCallbacks with required function validation.
func NewAudioCallbacks(requestPlayback func(SoundPlaybackRequest), requestStop func(SoundStopRequest), updateLocationMusic func(),
	pauseBackgroundMusic func(), resumeBackgroundMusic func(), requestVoice func(SoundPlaybackRequest) SoundVoice) AudioCallbacks {
	callbacks := AudioCallbacks{
		RequestPlayback:       requestPlayback,
		RequestVoice:          requestVoice,
		RequestStop:           requestStop,
		UpdateLocationMusic:   updateLocationMusic,
		PauseBackgroundMusic:  pauseBackgroundMusic,
		ResumeBackgroundMusic: resumeBackgroundMusic,
	}

	callbacks.PlaySoundEffect = func(effect SoundEffect) {
		requestPlayback(SoundPlaybackRequest{Effect: effect, Mode: SoundPlaybackOneShot, Volume: 1.0})
	}
	callbacks.Play = func(effect SoundEffect) *SoundBuilder {
		return newSoundBuilder(effect, &callbacks)
	}
	callbacks.Stop = func(effect SoundEffect) *StopBuilder {
		return newStopBuilder(effect, &callbacks)
	}
	callbacks.validate()

	return callbacks
}

func (callbacks AudioCallbacks) validate() {
	if callbacks.RequestVoice == nil {
		panic("AudioCallbacks.RequestVoice is required - cannot be nil")
	}
	if callbacks.Play == nil {
		panic("AudioCallbacks.Play is required - cannot be nil")
	}
	if callbacks.Stop == nil {
		panic("AudioCallbacks.Stop is required - cannot be nil")
	}
	if callbacks.PlaySoundEffect == nil {
		panic("AudioCallbacks.PlaySoundEffect is required - cannot be nil")
	}
	if callbacks.RequestPlayback == nil {
		panic("AudioCallbacks.RequestPlayback is required - cannot be nil")
	}
	if callbacks.RequestStop == nil {
		panic("AudioCallbacks.RequestStop is required - cannot be nil")
	}
	if callbacks.UpdateLocationMusic == nil {
		panic("AudioCallbacks.UpdateLocationMusic is required - cannot be nil")
	}
	if callbacks.PauseBackgroundMusic == nil {
		panic("AudioCallbacks.PauseBackgroundMusic is required - cannot be nil")
	}
	if callbacks.ResumeBackgroundMusic == nil {
		panic("AudioCallbacks.ResumeBackgroundMusic is required - cannot be nil")
	}
}

// ScreenCallbacks handles screen management and UI updates
type ScreenCallbacks struct {
	// UpdateDisplay refreshes presentation immediately after authoritative
	// map changes, including newly revealed ground items.
	UpdateDisplay func()

	// Overlay visibility control functions
	HideAllOverlays func() // Hides all overlays (moongates, NPCs, items, quest items, ambient sounds)
	ShowAllOverlays func() // Shows all overlays (default state)

	// MessageDialog control functions (required for camping)
	// NEW STACK-BASED PATTERN - Creates new MessageDialog instances
	CreateAndPushMessageDialog       func()               // Creates a new MessageDialog and pushes it to the stack
	CreateAndPushCampingResultDialog func(message string) // Compact centred rest result, owned by the camping workflow.
	PopMessageDialog                 func()               // Pops MessageDialog from stack when done
	HideMessageDialog                func()               // Triggers fade-out animation (call before Pop)

	// CONTENT MANIPULATION - Methods to modify dialog content and query properties
	ClearMessageDialog              func()                                 // Clears MessageDialog content
	SetMessagePrompt                func(prompt string)                    // Sets yellow prompt
	SetMessagePromptColor           func(prompt string, color color.Color) // Sets colored prompt
	ClearMessagePrompt              func()                                 // Clears prompt
	SetMessageDialogFadeDurations   func(fadeInMs, fadeOutMs int)          // Sets custom fade timing
	GetMessageDialogFadeInDuration  func() int                             // Gets current fade-in duration for Wait() sync
	GetMessageDialogFadeOutDuration func() int                             // Gets current fade-out duration for Wait() sync
	ShowPressAnyKeyPrompt           func()                                 // Helper: Shows "Press any key to continue..." in LightBlue
}

// NewScreenCallbacks validates a complete named set of screen capabilities.
func NewScreenCallbacks(callbacks ScreenCallbacks) ScreenCallbacks {
	if callbacks.UpdateDisplay == nil {
		panic("ScreenCallbacks.UpdateDisplay is required - cannot be nil")
	}
	if callbacks.HideAllOverlays == nil {
		panic("ScreenCallbacks.HideAllOverlays is required - cannot be nil")
	}
	if callbacks.ShowAllOverlays == nil {
		panic("ScreenCallbacks.ShowAllOverlays is required - cannot be nil")
	}
	if callbacks.CreateAndPushMessageDialog == nil {
		panic("ScreenCallbacks.CreateAndPushMessageDialog is required - cannot be nil")
	}
	if callbacks.CreateAndPushCampingResultDialog == nil {
		panic("ScreenCallbacks.CreateAndPushCampingResultDialog is required - cannot be nil")
	}
	if callbacks.PopMessageDialog == nil {
		panic("ScreenCallbacks.PopMessageDialog is required - cannot be nil")
	}
	if callbacks.HideMessageDialog == nil {
		panic("ScreenCallbacks.HideMessageDialog is required - cannot be nil")
	}
	if callbacks.ClearMessageDialog == nil {
		panic("ScreenCallbacks.ClearMessageDialog is required - cannot be nil")
	}
	if callbacks.SetMessagePrompt == nil {
		panic("ScreenCallbacks.SetMessagePrompt is required - cannot be nil")
	}
	if callbacks.SetMessagePromptColor == nil {
		panic("ScreenCallbacks.SetMessagePromptColor is required - cannot be nil")
	}
	if callbacks.ClearMessagePrompt == nil {
		panic("ScreenCallbacks.ClearMessagePrompt is required - cannot be nil")
	}
	if callbacks.SetMessageDialogFadeDurations == nil {
		panic("ScreenCallbacks.SetMessageDialogFadeDurations is required - cannot be nil")
	}
	if callbacks.GetMessageDialogFadeInDuration == nil {
		panic("ScreenCallbacks.GetMessageDialogFadeInDuration is required - cannot be nil")
	}
	if callbacks.GetMessageDialogFadeOutDuration == nil {
		panic("ScreenCallbacks.GetMessageDialogFadeOutDuration is required - cannot be nil")
	}
	if callbacks.ShowPressAnyKeyPrompt == nil {
		panic("ScreenCallbacks.ShowPressAnyKeyPrompt is required - cannot be nil")
	}

	return callbacks
}

// FlowCallbacks groups presentation-owned game flow hooks by stable concern.
type FlowCallbacks struct {
	Consequences     FlowConsequenceCallbacks
	Rest             FlowRestCallbacks
	WorldInteraction FlowWorldInteractionCallbacks
	Observation      FlowObservationCallbacks
	Shadowlord       FlowShadowlordCallbacks
	Items            FlowItemCallbacks
	SacredSites      FlowSacredSiteCallbacks
}

// FlowConsequenceCallbacks presents major consequences resolved by GameState.
type FlowConsequenceCallbacks struct {
	// TriggerDeathSequence starts the death and resurrection sequence when the entire party dies.
	TriggerDeathSequence func()

	// TriggerStonegateTrap presents an already committed original town rules volcano
	// consequence and owns the single death/resurrection handoff. It cannot
	// decide, delay, or roll back the authoritative trap result.
	// See ref:CMD-0001.
	TriggerStonegateTrap func(StonegateTrapOutcome)
}

// FlowRestCallbacks collects player input needed by rest workflows.
type FlowRestCallbacks struct {
	// RequestSleepDuration requests hours of sleep from player
	RequestSleepDuration func()

	// RequestCampDuration requests hours of camping from player
	RequestCampDuration func()
}

// FlowWorldInteractionCallbacks presents direct world-object interactions.
type FlowWorldInteractionCallbacks struct {
	// ShowWellInteraction initiates wishing well dialog sequence (coin → wish → result)
	// Only called after location validation (Paws/New Magincia) passes in GameState
	ShowWellInteraction func(pos *references.Position)

	// ShowFountainInteraction initiates fountain drinking dialog sequence (character selection → drink → refreshing message)
	// Cosmetic only - no healing or stat changes, works in all locations
	ShowFountainInteraction func()

	// ShowHorseRefusalDialog displays "Nay!" when an owned horse refuses mounting.
	ShowHorseRefusalDialog func()
}

// FlowObservationCallbacks presents astronomy, vision, and time displays.
type FlowObservationCallbacks struct {
	// ShowTelescopeInteraction initiates telescope/astronomy viewing sequence
	// Night: Shows star map with planets and Shadowlord comets
	// Day: Shows "the sun!" message with 1 HP damage to active player
	ShowTelescopeInteraction func()

	// ShowCrystalBallInteraction initiates crystal ball viewing sequence
	// 1. Character selection dialog ("Who will look?")
	// 2. Intelligence check: random(1,30) >= player.intelligence means FAILURE
	// 3. Failure: "Death vision!" + 1 HP damage + red flash
	// 4. Success: "Strange vision!" + overhead gem view display
	ShowCrystalBallInteraction func()

	// ShowGemViewInteraction initiates gem overhead view display
	// Called by V command after gem consumption - no character selection (authentic behavior)
	// Shows overhead map view centered on player position
	ShowGemViewInteraction func(complete func())

	// ShowSpyglassInteraction initiates spyglass viewing sequence from Use command.
	ShowSpyglassInteraction func(complete func())

	// ShowPocketWatchTime displays the current time in a dismissible message dialog.
	ShowPocketWatchTime func(timeString string, complete func())
}

// FlowShadowlordCallbacks presents Shadowlord and shard workflows.
type FlowShadowlordCallbacks struct {
	// TriggerShadowlordSummoned initiates the Shadowlord summoning visual/audio sequence
	// Called when player successfully summons a Shadowlord by yelling their name at a Keep
	// Based on original command rules - fizz animation + "A shadowlord appears" message
	// See ref:CMD-0001.
	TriggerShadowlordSummoned func(lord references.Shadowlord)

	// ThenShadowlordPresenceEffect returns a builder modifier for Shadowlord presence UI effects.
	// Handles both single Shadowlord (towns) and multiple (Stonegate).
	// Returns a function that chains effects onto an existing event queue builder.
	// Features: slow screen flash, ominous sound, message dialog that closes on any key.
	// See ref:CMD-0001.
	ThenShadowlordPresenceEffect func(lords []references.Shadowlord) func(
		*event_queue.SequentialEventBuilder,
	) *event_queue.SequentialEventBuilder

	// ShowShardUsageWorkflow initiates the shard destruction workflow with sequential popups.
	// Called when player uses 'U' command at a Keep location while having a shard.
	// Based on original item-use rules - sequential popups requiring keypresses.
	// See ref:CMD-0001.
	ShowShardUsageWorkflow func(references.QuestItem, func())
}

// FlowItemCallbacks presents item and equipment workflows.
type FlowItemCallbacks struct {
	// StartPotionWorkflow initiates the potion usage workflow with character selection.
	// Consumes potion, handles random effects, routes by potion type.
	// Based on original item-use rules potion mechanics.
	// See ref:CMD-0001.
	StartPotionWorkflow func(commit *PotionUseCommit, complete func())

	// StartWindChangeScrollWorkflow initiates the Wind Change scroll workflow.
	// Prompts for direction, sets wind to that direction, consumes scroll.
	// Based on original item-use rules - "Direction-" prompt followed by wind change.
	// See ref:CMD-0001.
	StartWindChangeScrollWorkflow func(complete func())

	// StartLightScrollWorkflow initiates the Light scroll workflow with visual effects.
	// Plays magic sound, shows invert effect, then activates the light.
	// Called when ScrollVasLor is used.
	StartLightScrollWorkflow func(complete func())

	// StartTimeStopScrollWorkflow initiates the Time Stop scroll workflow with visual effects.
	// Plays magic sound, shows invert effect, then activates time stop and shows popup.
	// Called when ScrollAnTym is used.
	StartTimeStopScrollWorkflow func(complete func())

	// StartResurrectionScrollWorkflow initiates the Resurrection scroll workflow.
	// Checks for dead party members, shows character selection, applies resurrection effect.
	// Scroll consumed only on successful resurrection; not consumed if no dead members.
	// Called when ScrollInManiCorp is used.
	StartResurrectionScrollWorkflow func(complete func())

	// TriggerToggleItemFlash triggers a screen flash effect when equipping toggle items.
	// Crown/Amulet get white/gold flash (royal items), Badge gets dark/blackish flash (sinister).
	// Uses event queue with Animate() for triangle wave flash pattern (0 → peak → 0).
	TriggerToggleItemFlash func(itemType ToggleItemType)

	// StartMoonstoneBurialWorkflow initiates the moonstone burial workflow with event queue.
	// Called when player uses a moonstone from the Use menu.
	// Based on original item-use rules moonstone burial logic.
	// Moonstone is the specific moonstone type being buried (0-7 corresponding to moon phases).
	// See ref:CMD-0001.
	StartMoonstoneBurialWorkflow func(moonstone references.Moonstone, complete func())

	// StartCarpetBoardingWorkflow initiates the magic carpet boarding workflow with event queue.
	// Called when player uses carpet from the Use menu after location validation passes.
	// Based on original item-use rules carpet boarding logic.
	// Removes carpet from inventory, spawns at player position, and boards player.
	// See ref:CMD-0001.
	StartCarpetBoardingWorkflow func(commit *CarpetBoardingCommit, complete func())

	// StartSkullKeyWorkflow initiates the skull key unlock workflow with event queue.
	// Called when player uses skull key from the Use menu after location validation passes.
	// Based on original item-use rules and magical unlocking.
	// Consumes skull key, prompts for direction, and unlocks magic doors.
	// See ref:CMD-0001.
	StartSkullKeyWorkflow func(commit *SkullKeyUseCommit, complete func())

	// StartHMSCapeWorkflow presents and commits one reviewed global preference change.
	StartHMSCapeWorkflow func(commit *HMSCapeUseCommit, complete func())

	// StartSceptreWorkflow initiates the Sceptre of Lord British usage workflow.
	// Called when player uses Sceptre from the Use menu.
	// Clears nearby barriers on surface/town maps.
	// See ref:CMD-0001.
	StartSceptreWorkflow func(complete func())

	// ShowReadyDialog opens the Ready Equipment dialog for its authoritative target.
	// Called from ActionReady*Map methods when player presses 'R' key.
	ShowReadyDialog func(request ReadyEquipmentRequest)
}

// FlowSacredSiteCallbacks presents shrine and Codex workflows.
type FlowSacredSiteCallbacks struct {
	// StartShrineWorkflow initiates the shrine meditation workflow with event queue.
	StartShrineWorkflow func(shrineIndex int)

	// StartCodexWorkflow initiates the Codex interaction workflow with event queue.
	StartCodexWorkflow func()
}

// TransientIconKind identifies one bounded gameplay event for brief visual feedback.
type TransientIconKind uint8

const (
	// TransientIconEating reports an automatic meal consuming provisions.
	TransientIconEating TransientIconKind = iota
	// TransientIconStarving reports the start of an hourly starvation consequence.
	TransientIconStarving
	// TransientIconHealing reports an approved explicit recovery increasing current HP.
	TransientIconHealing
	// TransientIconPoisonOnset reports an active party member becoming poisoned.
	TransientIconPoisonOnset
	// TransientIconPoisonCure reports an active party member recovering from poison.
	TransientIconPoisonCure
	// TransientIconLevelUp reports an upward Wanderer level reconciliation.
	TransientIconLevelUp
	// TransientIconKarmaIncrease reports an actual increase in clamped party karma.
	TransientIconKarmaIncrease
	// TransientIconKarmaDecrease reports an actual decrease in clamped party karma.
	TransientIconKarmaDecrease
)

// UICallbacks handles UI dialog interactions
type UICallbacks struct {
	// ShowZStatsDossier presents the detached, read-only party dossier.
	ShowZStatsDossier func(ZStatsDossier)

	// ShowQuestJournalDossier presents the detached, read-only quest dossier.
	ShowQuestJournalDossier func(QuestJournalDossier)
	// JournalChanged publishes an actual curated change, never initialization or hydration.
	JournalChanged func(tab, entry int)
	// AtlasUpdated follows a live discovery/name/survey commit, never a reader or load.
	AtlasUpdated func(journal.AtlasChange)

	// ShowYellDialogLargeMap displays input dialog for Words of Power entry on large map
	ShowYellDialogLargeMap func()

	// ShowTimedOverlayMessage displays a non-interactive overlay dialog that auto-dismisses
	// message: the text to display
	// durationMs: how long to show the message before auto-hiding (milliseconds)
	// centerHorizontally: if true, centers on whole screen (50% X); if false, centers on map area only (37% X)
	// alsoAddToOutput: if true, also adds message to main output pane
	// The overlay appears immediately and runs concurrently with other effects (shake, kapow, sounds)
	// Ignores all keyboard input - purely time-based dismissal
	ShowTimedOverlayMessage func(message string, durationMs int, centerHorizontally bool, alsoAddToOutput bool)

	// ShowActionFeedback displays a popup for action command responses (e.g., "Unlocked!", "Key broke!")
	// GameScene owns its elapsed-time fade lifecycle independently of event queues.
	// Features 400ms fade-in, 300ms fade-out, and auto-dismiss after 2 seconds.
	// message: short feedback text to display
	// alsoAddToOutput: if true, also adds message to main output pane (consolidates dual call pattern)
	ShowActionFeedback func(message string, alsoAddToOutput bool)

	// ShowSearchFeedback displays complete multiline Search output without mirroring it.
	ShowSearchFeedback func(message string)

	// ShowSpellExpiredPopup displays a dismissible popup when a duration effect expires.
	// Shows message like "Light wore off!" with any-key dismissal.
	ShowSpellExpiredPopup func(spellName string)

	// ShowTransientIcon displays bounded semantic feedback without exposing presentation details to gameplay.
	ShowTransientIcon func(TransientIconKind)

	// ShowExperienceAward announces experience paid to the party for quest deeds.
	ShowExperienceAward func(DeedExperienceAward)

	// ShowSignDialog displays a sign using runic/normal bitmap fonts.
	// sign: the SignRecord to display (nil shows default "LIVE BY THE EIGHT LAWS" message)
	// Called from Look command when looking at a sign tile.
	ShowSignDialog func(sign *references.SignRecord)
}

// NewFlowCallbacks validates a complete named set of outward flow capabilities.
func NewFlowCallbacks(callbacks FlowCallbacks) FlowCallbacks {
	if callbacks.Consequences.TriggerDeathSequence == nil {
		panic("FlowCallbacks.Consequences.TriggerDeathSequence is required - cannot be nil")
	}
	if callbacks.Consequences.TriggerStonegateTrap == nil {
		panic("FlowCallbacks.Consequences.TriggerStonegateTrap is required - cannot be nil")
	}
	if callbacks.Rest.RequestSleepDuration == nil {
		panic("FlowCallbacks.Rest.RequestSleepDuration is required - cannot be nil")
	}
	if callbacks.Rest.RequestCampDuration == nil {
		panic("FlowCallbacks.Rest.RequestCampDuration is required - cannot be nil")
	}
	if callbacks.WorldInteraction.ShowWellInteraction == nil {
		panic("FlowCallbacks.WorldInteraction.ShowWellInteraction is required - cannot be nil")
	}
	if callbacks.WorldInteraction.ShowFountainInteraction == nil {
		panic("FlowCallbacks.WorldInteraction.ShowFountainInteraction is required - cannot be nil")
	}
	if callbacks.WorldInteraction.ShowHorseRefusalDialog == nil {
		panic("FlowCallbacks.WorldInteraction.ShowHorseRefusalDialog is required - cannot be nil")
	}
	if callbacks.Observation.ShowTelescopeInteraction == nil {
		panic("FlowCallbacks.Observation.ShowTelescopeInteraction is required - cannot be nil")
	}
	if callbacks.Observation.ShowCrystalBallInteraction == nil {
		panic("FlowCallbacks.Observation.ShowCrystalBallInteraction is required - cannot be nil")
	}
	if callbacks.Observation.ShowGemViewInteraction == nil {
		panic("FlowCallbacks.Observation.ShowGemViewInteraction is required - cannot be nil")
	}
	if callbacks.Observation.ShowSpyglassInteraction == nil {
		panic("FlowCallbacks.Observation.ShowSpyglassInteraction is required - cannot be nil")
	}
	if callbacks.Observation.ShowPocketWatchTime == nil {
		panic("FlowCallbacks.Observation.ShowPocketWatchTime is required - cannot be nil")
	}
	if callbacks.Shadowlord.TriggerShadowlordSummoned == nil {
		panic("FlowCallbacks.Shadowlord.TriggerShadowlordSummoned is required - cannot be nil")
	}
	if callbacks.Shadowlord.ThenShadowlordPresenceEffect == nil {
		panic("FlowCallbacks.Shadowlord.ThenShadowlordPresenceEffect is required - cannot be nil")
	}
	if callbacks.Shadowlord.ShowShardUsageWorkflow == nil {
		panic("FlowCallbacks.Shadowlord.ShowShardUsageWorkflow is required - cannot be nil")
	}
	if callbacks.Items.StartPotionWorkflow == nil {
		panic("FlowCallbacks.Items.StartPotionWorkflow is required - cannot be nil")
	}
	if callbacks.Items.StartWindChangeScrollWorkflow == nil {
		panic("FlowCallbacks.Items.StartWindChangeScrollWorkflow is required - cannot be nil")
	}
	if callbacks.Items.StartLightScrollWorkflow == nil {
		panic("FlowCallbacks.Items.StartLightScrollWorkflow is required - cannot be nil")
	}
	if callbacks.Items.StartTimeStopScrollWorkflow == nil {
		panic("FlowCallbacks.Items.StartTimeStopScrollWorkflow is required - cannot be nil")
	}
	if callbacks.Items.StartResurrectionScrollWorkflow == nil {
		panic("FlowCallbacks.Items.StartResurrectionScrollWorkflow is required - cannot be nil")
	}
	if callbacks.Items.TriggerToggleItemFlash == nil {
		panic("FlowCallbacks.Items.TriggerToggleItemFlash is required - cannot be nil")
	}
	if callbacks.Items.StartMoonstoneBurialWorkflow == nil {
		panic("FlowCallbacks.Items.StartMoonstoneBurialWorkflow is required - cannot be nil")
	}
	if callbacks.Items.StartCarpetBoardingWorkflow == nil {
		panic("FlowCallbacks.Items.StartCarpetBoardingWorkflow is required - cannot be nil")
	}
	if callbacks.Items.StartSkullKeyWorkflow == nil {
		panic("FlowCallbacks.Items.StartSkullKeyWorkflow is required - cannot be nil")
	}
	if callbacks.Items.StartHMSCapeWorkflow == nil {
		panic("FlowCallbacks.Items.StartHMSCapeWorkflow is required - cannot be nil")
	}
	if callbacks.Items.StartSceptreWorkflow == nil {
		panic("FlowCallbacks.Items.StartSceptreWorkflow is required - cannot be nil")
	}
	if callbacks.Items.ShowReadyDialog == nil {
		panic("FlowCallbacks.Items.ShowReadyDialog is required - cannot be nil")
	}
	if callbacks.SacredSites.StartShrineWorkflow == nil {
		panic("FlowCallbacks.SacredSites.StartShrineWorkflow is required - cannot be nil")
	}
	if callbacks.SacredSites.StartCodexWorkflow == nil {
		panic("FlowCallbacks.SacredSites.StartCodexWorkflow is required - cannot be nil")
	}

	return callbacks
}

// NewSystemCallbacks creates SystemCallbacks with validation for all subsystems
func NewSystemCallbacks(
	message MessageCallbacks,
	visual VisualCallbacks,
	audio AudioCallbacks,
	screen ScreenCallbacks,
	flow FlowCallbacks,
	talk TalkCallbacks,
	uiCallbacks UICallbacks,
	combat CombatCallbacks,
) (*SystemCallbacks, error) {
	// Validate that all callback structs have required functions
	if message.AddRowStr == nil {
		return nil, errors.New("MessageCallbacks.AddRowStr is required")
	}
	if message.AppendToCurrentRowStr == nil {
		return nil, errors.New("MessageCallbacks.AppendToCurrentRowStr is required")
	}
	audio.validate()
	if visual.PlayKapowAtPosition == nil {
		return nil, errKapowCallbackRequired
	}
	if visual.ShowCombatImpact == nil {
		return nil, errCombatImpactCallbackRequired
	}
	if uiCallbacks.ShowYellDialogLargeMap == nil {
		return nil, errors.New("UICallbacks.ShowYellDialogLargeMap is required")
	}
	if uiCallbacks.ShowTimedOverlayMessage == nil {
		return nil, errors.New("UICallbacks.ShowTimedOverlayMessage is required")
	}
	if uiCallbacks.ShowActionFeedback == nil {
		return nil, errUICallbackRequired
	}
	if uiCallbacks.ShowSearchFeedback == nil {
		return nil, errSearchFeedbackCallbackRequired
	}
	if uiCallbacks.ShowZStatsDossier == nil {
		return nil, errZStatsCallbackRequired
	}
	if uiCallbacks.ShowQuestJournalDossier == nil {
		return nil, errQuestJournalCallbackRequired
	}
	if uiCallbacks.JournalChanged == nil {
		return nil, errJournalChangedCallbackRequired
	}
	if uiCallbacks.AtlasUpdated == nil {
		return nil, errAtlasUpdatedCallbackRequired
	}
	if uiCallbacks.ShowTransientIcon == nil {
		return nil, errTransientIconCallbackRequired
	}
	if uiCallbacks.ShowExperienceAward == nil {
		return nil, errExperienceAwardRequired
	}
	if uiCallbacks.ShowSignDialog == nil {
		return nil, errSignDialogCallbackRequired
	}
	if uiCallbacks.ShowSpellExpiredPopup == nil {
		return nil, errSpellExpiredCallbackRequired
	}
	if combat.PublishEvent == nil {
		return nil, errCombatEventCallbackRequired
	}
	if combat.PublishConclusion == nil {
		return nil, errCombatConclusionRequired
	}

	message = NewMessageCallbacks(message)
	visual = NewVisualCallbacks(visual)
	screen = NewScreenCallbacks(screen)
	flow = NewFlowCallbacks(flow)

	return &SystemCallbacks{
		Message: message,
		Visual:  visual,
		Audio:   audio,
		Screen:  screen,
		Flow:    flow,
		Talk:    talk,
		UI:      uiCallbacks,
		Combat:  combat,
	}, nil
}
