package main

import (
	"fmt"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	rng "github.com/bradhannah/berad-sdk/modules/berad-rng"

	"github.com/bradhannah/UltimaVRedux/internal/audio"
	"github.com/bradhannah/UltimaVRedux/internal/camping"
	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/minimap"
	"github.com/bradhannah/UltimaVRedux/internal/player_save"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/visual"

	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/mainscreen"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	borderWidthScaling        = widgets.BorderWidthStandard
	xTilesVisibleOnGameScreen = 19
	yTilesVisibleOnGameScreen = 13
)

const (
	// Line spacing multiplier: 25/23 ≈ 1.087 for slightly extra vertical space
	defaultOutputFontSize        = widgets.FontSizeBody
	defaultOutputLineSpacingMult = 1.087 // 25/23 for original 25pt spacing at 23pt font
	maxCharsPerLine              = 16
	visibleLinesForOutput        = 11
	maxLinesForOutput            = 64
)

type InputState int

const (
	PrimaryInput InputState = iota
	OpenDirectionInput
	JimmyDoorDirectionInput
	KlimbDirectionInput
	CombatKlimbLadderDirectionInput
	PushDirectionInput
	GetDirectionInput
	LookDirectionInput
	TalkDirectionInput
	SearchDirectionInput
	AttackDirectionInput
	CombatAttackTargetInput
	CombatSpellTargetInput
	CombatConeTargetInput
	CombatAdjacentSpellTargetInput
	FireDirectionInput
	DungeonQuitInput
	SurfaceQuitInput
)

// VisualEffect represents a temporary visual effect on screen
type VisualEffect struct {
	// World-based positioning (preferred for game effects)
	WorldPos    *references.Position // World coordinates (nil = legacy screen coords mode)
	SpriteIndex indexes.SpriteIndex  // Sprite to render (0 = explosion)

	// Scale animation parameters
	ScaleMin float64 // Minimum scale (e.g., 0.5)
	ScaleMax float64 // Maximum scale (e.g., 1.0)

	// Screen-coordinate fallback for presentation effects without world positions.
	X, Y     int     // Screen coordinates (used if WorldPos is nil)
	Type     string  // Effect type (for example, kapow)
	Duration float64 // Duration in seconds
	Elapsed  float64 // Time elapsed since creation

	// Combat bursts use admission time, never the preceding frame's delta.
	combatClock     bool
	combatStartedMs int64
}

// DissolveEffectState encapsulates all state needed for a sprite dissolve effect.
// Groups the dissolve level, GPU-accelerated effect, and persistent render buffer together.
type DissolveEffectState struct {
	level  float64                     // 0.0 = fully visible, 1.0 = fully dissolved (for TeleportationManager compatibility)
	effect *visual.PixelDissolveEffect // GPU-accelerated dissolve effect
	buffer *ebiten.Image               // Persistent masked buffer (reused per frame to eliminate allocations)
}

// IsActive returns true if the dissolve effect is currently initialized and active.
func (d *DissolveEffectState) IsActive() bool {
	return d.effect != nil
}

// Initialize creates the dissolve effect for a sprite with the given dimensions.
func (d *DissolveEffectState) Initialize(spriteWidth, spriteHeight int, mode visual.DissolveMode, rng *rng.Source) {
	d.effect = visual.NewPixelDissolveEffect(
		spriteWidth, spriteHeight,
		spriteWidth, spriteHeight,
		mode,
		rng,
	)
	d.buffer = ebiten.NewImage(spriteWidth, spriteHeight)
	d.level = 0.0
}

// UpdateProgress updates the dissolve effect progress (0.0 to 1.0).
func (d *DissolveEffectState) UpdateProgress(progress float64) {
	d.level = progress
	if d.effect != nil {
		d.effect.UpdateProgress(progress)
	}
}

// GetLevel returns the current dissolve level (for TeleportationManager compatibility).
func (d *DissolveEffectState) GetLevel() float64 {
	return d.level
}

// Cleanup releases all resources associated with the dissolve effect.
func (d *DissolveEffectState) Cleanup() {
	if d.effect != nil {
		d.effect.Dispose()
	}
	if d.buffer != nil {
		d.buffer.Deallocate()
	}
	d.effect = nil
	d.buffer = nil
	d.level = 0.0
}

// GameScene is another scene (e.g., the actual game)
type GameScene struct {
	gameConfig              *config.UltimaVConfiguration
	gameReferences          *references.GameReferences
	spriteSheet             *sprites.SpriteSheet
	keyboard                *input.Keyboard
	output                  *text.Output
	outputClipImage         *ebiten.Image
	ultimaFont              *text.UltimaFont
	dungeonHUD              dungeonHUDOutputs
	sky                     skyPresentation
	mapImage                *ebiten.Image
	surfaceCamera           surfaceCameraProjection
	surfaceTerrainImage     *ebiten.Image
	terrainDither           terrainDitherFrame
	outerTerrainDither      sprites.DitherMaskImage
	outerDitherKey          outerTerrainKey
	outerDitherValid        bool
	unscaledMapImage        *ebiten.Image
	dungeonSprites          *sprites.DungeonSprites
	dungeonCanvas           *ebiten.Image
	dungeonClipLayer        *ebiten.Image
	dungeonGraffitiOutput   *text.Output
	dungeonSkeletonCrumbles []dungeonSkeletonCrumbleEffect
	dungeonPitReveals       []dungeonPitRevealEffect
	dungeonFieldDispels     []dungeonFieldDispelEffect
	dungeonInjury           dungeonInjuryPulse
	actorAnimationClocks    actorAnimationClockSet
	characterSummary        *mainscreen.CharacterSummary
	provisionSummary        *mainscreen.ProvisionSummary
	provisionChanges        provisionChangePresentation

	dialogStack                      widgets.DialogStack
	journalVisits                    journal.Visits
	lastConversation                 journal.Conversation
	excerpts                         journal.Excerpts
	journalSession                   *journalSession
	atlasSession                     *atlasSession
	atlasView                        widgets.AtlasViewState
	atlasChordHeld                   bool
	atlasMarkChordHeld               bool
	quickLoadChordHeld               bool
	journalNotice                    journalNotice
	journalNoticeSuppressed          bool
	transientActionFeedback          []*transientActionFeedback
	combatChestFeedback              map[*transientActionFeedback]combatChestFeedbackScope
	transientActionFeedbackAt        int64
	transientActionFeedbackClockSet  bool
	transientActionFeedbackLayout    transientFeedbackLayoutContext
	transientActionFeedbackLayoutSet bool
	tavernMealTableOverlay           *tavernMealTableOverlay
	appendInputChars                 func([]rune) []rune

	debugMessage string

	gameState *game_state.GameState

	activePlayerSaveSlot     player_save.SlotID
	activePlayerSaveRevision string
	activePlaythrough        playthroughMetadata
	activeDoomRunID          string

	startTime     time.Time // Scene activation anchor for elapsed presentation time
	elapsedBaseMs int64
	now           func() time.Time // Injectable scene clock; nil selects the production wall clock.

	// Real-time tracking for effects
	lastEffectsUpdate time.Time

	secondaryKeyState             InputState
	transientCommandInput         transientCommandInputState
	observedSailingStopEpoch      uint64
	surfaceSpellWorkflowOwnsCycle bool
	castUseDirectionDialog        *widgets.MessageDialog
	potionEffectDialog            *widgets.OverlayDialog
	pocketWatchDialog             *widgets.OverlayDialog
	dungeonOpenCommandToken       game_state.DungeonCommandCycle
	dungeonJimmyCommandToken      game_state.DungeonCommandCycle
	dungeonKlimbCommandToken      game_state.DungeonCommandCycle
	dungeonLookCommandToken       game_state.DungeonCommandCycle
	dungeonSearchCommandToken     game_state.DungeonCommandCycle
	dungeonQuitCommandToken       game_state.DungeonCommandCycle
	saveQuestionDialog            *widgets.OverlayDialog
	combatLootPreview             *combatLootProjection
	dungeonLookCharacterIndex     int
	dungeonSearchCharacterIndex   int

	borders        gameBorders
	rightRail      rightRailLayout
	runicReference runicReferenceState

	lastCheckedResolution config.ScreenResolution

	// Visual effects system
	visualEffects          []VisualEffect
	combatConeCast         *combatConeCastEffect
	combatUtilityEffect    *combatUtilitySpellEffect
	combatTeleportEffect   *combatTeleportSpellEffect
	combatSummonEffect     *combatSummonSpellEffect
	combatRepelEffect      *combatRepelSpellEffect
	combatDivisionEffects  []CombatDivisionEffect
	combatImpactEffects    []CombatImpactEffect
	combatImpactCapture    []game_state.CombatAttackFeedback
	combatSceptreEffects   []combatSceptreActorEffect
	combatFieldDispels     []combatFieldDispelEffect
	combatVanishEffects    []combatVanishEffect
	combatSignatureEffects []*combatSignatureEffect
	combatResolutionTiming *combatPresentationTiming
	combatAttackTiming     combatAttackTimingSnapshot
	combatTurnHandOff      combatTurnHandOffMark
	combatAllegiance       combatAllegianceState
	combatRenderFrame      combatRenderFrame
	combatMovement         CombatMovementAnimation
	combatDamageOutput     *text.Output
	combatHUD              *CombatHUD
	combatConclusion       *game_state.CombatConclusion
	pendingBattleSummaries []game_state.CombatConclusion
	// deedExperienceSettled and deedExperienceTier remember the last back-pay check.
	deedExperienceSettled bool
	deedExperienceTier    int
	lastBattleSummary     *game_state.CombatConclusion
	battleSummaryDialog   *widgets.CombatLootDialog

	// Auto-open door animation state
	autoOpenDoorState *AutoOpenDoorState

	// Audio system
	audioManager *audio.GameAudioManager
	ambientTiles []audio.TileWithPosition // Borrowed by the synchronous audio update; refilled each scan.

	// Debug overlay renderer
	overlayRenderer *OverlayRenderer

	// Moongate visual system
	moongateVisualSystem *MoongateVisualSystem

	// Wind display overlay
	windDisplay *WindDisplay

	// Active effects overlay (equipment worn, spell effects)
	activeEffectsOverlay *ActiveEffectsOverlay
	activeEffectsPanel   *ActiveEffectsPanel

	// Teleportation visual effects
	avatarDissolve   DissolveEffectState // Avatar dissolve state (GPU-accelerated)
	wandererDissolve DissolveEffectState // Wanderer dissolve state (GPU-accelerated)
	mapFadeLevel     float64             // 0.0 = fully visible, 1.0 = fully black

	// Note: Death scene dissolve fields removed - now uses generic tile dissolve system
	inputBlocked bool // true when all input should be blocked

	// Generic tile dissolve system (supports multiple simultaneous dissolves)
	// Used for death scene apparitions, camping wanderer, item spawning, etc.
	activeTileDissolves []TileDissolveState // Active tile dissolve animations
	nextDissolveID      int                 // Counter for unique dissolve IDs

	shadowlordSummonPresentation *shadowlordSummonPresentation
	shadowlordPresenceDialog     *widgets.MessageDialog
	shadowlordPresenceActive     bool
	shadowlordShardDialog        *widgets.OverlayDialog
	shadowlordShardActive        bool

	// Screen flash effects (event queue pattern - intensity set via Animate() callback)
	screenFlashR         float64 // Red component (0.0 to 1.0)
	screenFlashG         float64 // Green component (0.0 to 1.0)
	screenFlashB         float64 // Blue component (0.0 to 1.0)
	screenFlashIntensity float64 // Current intensity (0.0 to 1.0), set by SetScreenFlashIntensity

	// Screen shake effects (event queue pattern - offset set via Animate() callback)
	screenShakeOffsetX float64 // Current X offset in pixels, set by SetScreenShakeOffset
	screenShakeOffsetY float64 // Current Y offset in pixels, set by SetScreenShakeOffset

	// Screen invert effect (X-ray style)
	screenInvertIntensity float64 // Invert intensity (0.0 = normal, 1.0 = fully inverted)
	screenInvertStartTime int64   // When invert started (in milliseconds)
	screenInvertDuration  int64   // Duration in milliseconds

	// Performance optimization: Pre-allocated draw options to reduce GC pressure
	mapDrawOptions      *ebiten.DrawImageOptions // Reusable for main map drawing
	dungeonDrawOptions  *ebiten.DrawImageOptions // Reusable for the aspect-fitted dungeon canvas
	avatarDrawOptions   *ebiten.DrawImageOptions // Reusable for avatar overlay
	npcDrawOptions      *ebiten.DrawImageOptions // Reusable for NPC/enemy drawing
	itemDrawOptions     *ebiten.DrawImageOptions // Reusable for item stack drawing
	moongateDrawOptions *ebiten.DrawImageOptions // Reusable for moongate drawing

	// FPS limiting for draw calls
	lastDrawTime int64 // Last draw time in milliseconds for FPS limiting

	// Overlay visibility flags - control what gets rendered
	hideMapUnits      bool // NPCs, enemies, vehicles
	hideMoongates     bool // Moongate portals
	hideQuestItems    bool // Shards, artifacts
	hideItemStacks    bool // Equipment on ground
	hideAmbientSounds bool // Ambient sound updates

	// X-Ray Vision effect (Black potion) - reveals full map until keypress
	xRayVisionActive bool

	// Active projectile (one at a time - cannonballs, arrows, magic bolts)
	activeProjectile *ActiveProjectile

	// Timed overlay message (auto-dismissing dialog)
	timedOverlayDialog    widgets.DismissibleDialog // Currently displayed timed overlay (OverlayDialog or MessageDialog)
	timedOverlayDismissMs int64                     // When to auto-dismiss (game elapsed ms)

	// MiniMap scene system for scripted cutscenes (shrine, codex, endgame)
	miniMapScene       *minimap.Scene
	finalePresentation *lordBritishFinalePresentation

	// windowCloseDepartureOpen marks save choices opened by the window's close
	// button, so a second close request quits without asking again.
	windowCloseDepartureOpen bool

	active   bool
	disposed bool
}

type transientFeedbackPhase int

const (
	transientFeedbackFadingIn transientFeedbackPhase = iota
	transientFeedbackHolding
	transientFeedbackFadingOut
)

type transientActionFeedback struct {
	dialog         *widgets.MessageDialog
	startedAt      int64
	phase          transientFeedbackPhase
	consumedMs     int64
	animationMs    int64
	cardWidth      int
	cardHeight     int
	positioned     bool
	x, y           float64
	motion         transientFeedbackMotion
	protectedUntil int64
}

// NewGameScene constructor and related helpers moved to gamescene_initialization.go

// openConfigurationDialog creates and displays the player configuration dialog.
func (g *GameScene) openConfigurationDialog() {
	// Configuration can use the complete gameplay pane while leaving the right rail visible.
	placement := gameScreenPercents

	// Declare dialog variable first so it can be captured in closure
	var configurationDialog *ConfigurationDialog

	// The dialog owns apply persistence and cancel reversion; the scene owns exact stack cleanup.
	onDismiss := func() {
		g.dialogStack.MustRemoveWidgetInstance(configurationDialog)
		g.keyboard.SetForceWaitAnyKey(250) // Prevent immediate input
	}

	var settingsAudioManager ConfigurationAudioManager
	if g.audioManager != nil {
		settingsAudioManager = g.audioManager
	}

	// Create and show the dialog
	configurationDialog = NewConfigurationDialog(
		&placement,
		g.keyboard,
		onDismiss,
		g.gameConfig,         // Pass full config so it can save to Viper
		settingsAudioManager, // Nil when audio is disabled for this session.
	)

	// Push to dialog stack
	g.dialogStack.PushModalDialog(configurationDialog)
}

// Visual effects methods moved to gamescene_visual_effects.go

// Note: updateScreenShake() removed - screen shake now managed by event queue pattern
// Event queues call SetScreenShakeOffset() directly via Animate() callbacks

// Input and moongate control methods moved to gamescene_input_helpers.go

// Screen callback implementations for SystemCallbacks integration (moved to gamescene_message_dialog.go)

// Guard, effects, and update helper methods moved to gamescene_input_helpers.go

// Note: TRAP fall sequences now managed by event queue system
// in internal/game_state/action_fall_through.go - no polling needed

// Teleportation effect methods moved to gamescene_teleportation.go

// Audio system methods moved to gamescene_audio.go

// Dialog methods moved to gamescene_dialogs.go

// Coordinate conversion and audio music control methods moved to gamescene_input_helpers.go and gamescene_audio.go

// Teleportation system methods moved to gamescene_teleportation.go

// triggerDeathSequence handles the death and resurrection sequence
// Presents the full-party death and recovery sequence (ref:RECOVERY-0002).
func (g *GameScene) triggerDeathSequence() {
	g.StartDeathResurrectionWorkflow()
}

// RequestSleepDuration shows dialog for bed rest duration selection and executes bed rest in a single unified event queue.
// Uses thenShowSleepDurationInput helper directly incorporated via ThenSequence() to avoid split queues.
func (g *GameScene) RequestSleepDuration() {
	var hours int
	var restState *game_state.BedRestState

	g.gameState.EventQueueManager.Workflow("bed_rest_workflow").
		Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().
		ThenSequence(

			g.thenShowSleepDurationInput(func(h int) {
				hours = h
			})).If(

		func() bool { return hours > 0 },
		func(q *event_queue.SequentialEventBuilder) {
			q.ThenSequence(g.thenExecuteBedRestSequence(hours, &restState))
		}).Finally(

		func() {
			g.gameState.FinishBedRest(restState)
			g.gameState.SystemCallbacks.Visual.SetMapFadeLevel(0)
			g.keyboard.ResetReleaseDebouncing()
		}).
		Start()
}

// RequestCampDuration shows duration selection dialog for camping using unified event queue.
// Creates single event queue for complete camping workflow: duration input → guard selection (if multi-party) → camping execution
func (g *GameScene) RequestCampDuration() {
	var hours int // Capture hours from duration input
	cancelled := false
	committed := false
	originMapType := g.gameState.MapState.PlayerLocation.Location.GetMapType()
	guardIndex := game_state.NoGuardAssigned
	var guardDialog *widgets.CharacterSelectionDialog
	completed := false
	complete := func() {
		if completed {
			return
		}
		completed = true
		if guardDialog != nil {
			g.dialogStack.RemoveWidgetInstance(guardDialog)
		}
		g.keyboard.ResetReleaseDebouncing()
		if committed {
			g.completeSurfaceCommand(newSurfaceCommandResult(surfaceCommandCompleted, originMapType))
		}
	}

	g.gameState.EventQueueManager.Workflow("camping_workflow").
		Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().
		ThenSequence(

			g.thenShowSleepDurationInput(func(h int) {
				hours = h
			})).If(

		func() bool {
			return hours > 0 && g.gameState.PartyState.CountActiveMembers() > 1
		},
		func(q *event_queue.SequentialEventBuilder) {
			q.ThenSequence(g.thenShowGuardSelection(func(gi int, cancel bool) {
				guardIndex = gi
				cancelled = cancel
			}, &guardDialog))
		}).If(

		func() bool { return hours > 0 && !cancelled },
		func(q *event_queue.SequentialEventBuilder) {
			q.Then(func() { committed = true }).ThenSequence(g.thenExecuteCampingSequence(hours, guardIndex, complete))
		}).Finally(complete).
		Start()
}

// ShowHorseRefusalDialog displays the horse refusal message using event queue pattern.
// Original Ultima V behavior: owned horses refuse mounting with simple "Nay!" message.
// Uses thenShowConfirmationDialog helper for consistent dialog lifecycle management.
func (g *GameScene) ShowHorseRefusalDialog() {
	g.gameState.EventQueueManager.Workflow("horse_refusal_prompt").
		Priority(event_queue.PriorityLow).
		Sequential().ThenSequence(

		g.thenShowConfirmationDialog("\"Nay!\"")).
		Start()
}

// thenExecuteCampingSequence is a helper that creates the complete camping sequence
// following the ThenSequence pattern for external functionality integration.
//
// IMPORTANT: This function uses If() with runtime evaluation to ensure camping events
// are added with fresh data when the sequence reaches this point, NOT during build phase.
// This prevents stale location/config data and ensures deterministic event ordering.
//
// This encapsulates the full camping flow:
//   - Save player location for return after camping (setup phase)
//   - Load camping combat map reference (BRIT.CBT) with fallback (setup phase)
//   - Create camping scene with hours and guard index (setup phase)
//   - Add all camping events to builder via AddCampingSequenceToBuilder (setup phase)
//
// Returns a modifier function that can be used with ThenSequence for clean syntax.
//
// Usage with ThenSequence:
//
//	When(
//	    func() bool { return hours > 0 },
//	    func(q *event_queue.SequentialEventBuilder) {
//	        q.ThenSequence(g.thenExecuteCampingSequence(hours, guardIndex, complete))
//	    },
//	)
func (g *GameScene) thenExecuteCampingSequence(hours int, guardIndex int, complete func()) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		logging.Debugf(logging.CategoryGameState, "Adding camping sequence to event queue for %d hours with guard %d", hours, guardIndex)

		// Use If() to perform setup and add camping events at RUNTIME
		// This ensures fresh data (location, config) when camping actually starts
		return q.If(
			func() bool { return true },
			func(subQ *event_queue.SequentialEventBuilder) {
				originalPlayerLocation := g.gameState.MapState.SavePlayerLocation()

				campingMapRef, err := references.LoadCampingMap(g.gameState.GameConfig)
				if err != nil {
					// Startup validated the original data, so the camp map must load.
					panic(fmt.Sprintf("GameScene camping: loading camping map: %v", err))
				}
				campingScene := camping.NewCampingSceneWithCombatRef(hours, guardIndex, originalPlayerLocation, campingMapRef)

				g.gameState.AddCampingSequenceToBuilder(
					subQ,
					campingScene,
					false,
					false,
					complete,
				)
			})
	}
}

// thenExecuteBedRestSequence is a helper that creates the complete bed rest sequence
// following the ThenSequence pattern for external functionality integration.
//
// This encapsulates the full bed rest flow:
//   - Initial setup (sound, message, sleep status)
//   - Fade to black animation
//   - Rest cycles with poison damage, food consumption, starvation checks
//   - NPC bed conflict detection and interruption
//   - Wakeup sequence with fade back, status updates, NPC repositioning
//
// Returns a modifier function that can be used with ThenSequence for clean syntax.
//
// Usage with ThenSequence:
//
//	When(
//	    func() bool { return hours > 0 },
//	    func(q *event_queue.SequentialEventBuilder) {
//	        q.ThenSequence(g.thenExecuteBedRestSequence(hours, &restState))
//	    },
//	)
func (g *GameScene) thenExecuteBedRestSequence(
	hours int,
	restStateOut **game_state.BedRestState,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		if hours == 0 {
			return q
		}

		restState := g.gameState.NewBedRestState(hours)
		if restStateOut != nil {
			*restStateOut = restState
		}

		return q.RepeatWithDelay(game_state.BedRestPreSleepNPCPasses, game_state.BedRestHealingCycleDelayMs, func(_ int) bool {
			if g.gameState.ProcessBedRestPreSleepNPCPass() {
				restState.AttackInterrupted = true
				restState.Interrupted = true

				return false
			}

			return true
		}).If(func() bool { return !restState.AttackInterrupted }, func(sleepQ *event_queue.SequentialEventBuilder) {
			sleepQ.Then(func() {
				g.gameState.BeginBedRest(restState)
				g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundGoodNight)
				g.gameState.SystemCallbacks.Message.AddRowStr("Zzzzzzz...")
			}).Wait(game_state.BedRestInitialDelayMs).
				Animate(game_state.BedRestFadeToBlackMs, func(progress float32) {
					g.gameState.SystemCallbacks.Visual.SetMapFadeLevel(float64(progress))
				}).Wait(game_state.BedRestPreHealingDelayMs).
				While(func() bool {
					return int(g.gameState.DateTime.Hour) != restState.TargetHour &&
						!restState.Interrupted && !restState.TimeStopped
				}, func() {
					result := g.gameState.ProcessBedRestCycle(restState)
					for _, charIndex := range result.PoisonDeaths {
						name := g.gameState.PartyState.GetCharacter(charIndex).GetNameAsString()
						g.gameState.SystemCallbacks.Message.AddRowStr(fmt.Sprintf("Alas! %s has died!", name))
					}
					if len(result.PoisonDeaths) > 0 {
						g.gameState.ShowDeathFeedback()
					} else if restState.CurrentCycle%6 == 0 && g.gameState.PartyState.HasPoisonedCharacter() {
						g.gameState.ShowPoisonFeedback()
					}
					if result.Provision.Starving {
						g.gameState.SystemCallbacks.Message.AddRowStr("Starving!")
						g.gameState.ShowStarvationFeedback()
						for _, charIndex := range result.Provision.DeadIndexes {
							name := g.gameState.PartyState.GetCharacter(charIndex).GetNameAsString()
							g.gameState.SystemCallbacks.Message.AddRowStr(fmt.Sprintf("Alas! %s has died from starvation!", name))
						}
						if len(result.Provision.DeadIndexes) > 0 {
							g.gameState.ShowDeathFeedback()
						}
					}
					if result.NPCAtBed {
						restState.Interrupted = true
					}
				}, game_state.BedRestHealingCycleDelayMs).
				Wait(game_state.BedRestPreWakeDelayMs).
				Animate(restState.GetFadeDuration(), func(progress float32) {
					g.gameState.SystemCallbacks.Visual.SetMapFadeLevel(float64(1 - progress))
				}).Then(func() {
				switch {
				case restState.TimeStopped:
					g.gameState.SystemCallbacks.Message.AddRowStr("Time is stopped!")
				case restState.Interrupted && !restState.PartyWipePending:
					g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundThrownOutOfBed)
					g.gameState.SystemCallbacks.Message.AddRowStr("Thrown out of bed!")
				case !restState.PartyWipePending:
					g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundRoosterCrow)
					g.gameState.SystemCallbacks.Message.AddRowStr("Rested!")
				}
				if !restState.PartyWipePending {
					g.gameState.MoveAvatarOutOfBed()
				}
			})
		})
	}
}

// Dissolve effect methods moved to gamescene_dissolve.go
