package game_state

import (
	"fmt"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/camping"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// Camping timing constants
const (
	// CampingCycleDelayMs Event queue delays (milliseconds)
	// REDUX DEVIATION: CAMP-0004: original Bard presentation uses 52 update/delay
	// iterations, not a certified wall-time duration. Redux keeps these cosmetic
	// delays for readable presentation; gameplay advances only on logical cycles.
	CampingCycleDelayMs      = 200
	CampingFastCycleDelayMs  = 50
	BardMusicDurationMs      = 5500
	WandererFadeAnimationMs  = 3000 // Duration for wanderer fade in/out
	WandererVisiblePauseMs   = 1000 // Pause when wanderer fully visible
	WandererDepartureDelayMs = 500  // Delay before wanderer departure

	// MinimumWandererHours Camping requirements
	MinimumWandererHours = 5 // Minimum hours camping to encounter wanderer

	// DebugAmbushCycle Ambush cycle constants
	DebugAmbushCycle = 18 // Cycle number for forced ambush in debug mode

	// DefaultCampingHours Default camping parameters
	DefaultCampingHours = 8  // Default hours when called without parameters
	NoGuardAssigned     = -1 // Guard index indicating no guard selected
	NoAmbush            = -1 // Sentinel value for no ambush scheduled

	// WaterTileMin Water tile range (cannot camp on water)
	WaterTileMin = 1 // First water tile index (Water1)
	WaterTileMax = 3 // Last water tile index (WaterCoast)

	// FullyDissolved Dissolve effect levels
	FullyDissolved = 1.0 // Wanderer completely invisible
	FullyVisible   = 0.0 // Wanderer completely visible

	// DirectionCount Guard movement (original game: random(0,3) for 25% chance)
	DirectionCount = 4 // Number of possible movement directions
	GuardMoveOdds  = 4 // 1-in-N odds for guard to move per cycle (25% chance)

	// WandererHealMin Wanderer healing (original: random(1,63) one-time heal)
	WandererHealMin = 1  // Minimum HP healed by wanderer
	WandererHealMax = 63 // Maximum HP healed by wanderer
)

// ActionHoleUpLargeMap starts outdoor camp or anchored-ship repair selection.
func (g *GameState) ActionHoleUpLargeMap() bool {
	if g.EventQueueManager.HasActiveWorkflows() {
		return false
	}
	g.SystemCallbacks.Message.AddRowStr("Hole up & ")

	// Ship repair check - must come BEFORE camping vehicle check
	// Original Ultima V: original command rules check ship first
	// Ship repair is separate from camping (quick 25-minute action, no guard/hours)
	// See ref:CAMP-0005.
	if g.isOnFrigate() {
		return g.ActionHoleUpRepairShip()
	}

	return g.actionHoleUpLargeMap(DefaultCampingHours, NoGuardAssigned, false, false, true)
}

// ActionHoleUpLargeMapWithDebug executes large map camping with all parameters
func (g *GameState) ActionHoleUpLargeMapWithDebug(hours int, guardIndex int, forceWanderer bool, forceAmbush bool) bool {
	if g.EventQueueManager.HasActiveWorkflows() {
		return false
	}

	return g.actionHoleUpLargeMap(hours, guardIndex, forceWanderer, forceAmbush, false)
}

func (g *GameState) actionHoleUpLargeMap(hours int, guardIndex int, forceWanderer bool, forceAmbush bool, requestDuration bool) bool {
	playerPos := &g.MapState.PlayerLocation.Position
	currentTile := g.GetLayeredMapByCurrentLocation().GetTopTile(playerPos)

	// REDUX DEVIATION: rest dispatch requires the surface party to be on foot.
	// Permit horses and carpets on eligible land. A ridden carpet is stowed at
	// camping commitment so it remains in inventory through an ambush or death.
	// See ref:CAMP-0005.
	vehicleType := g.PartyVehicle.GetVehicleDetails().VehicleType
	logging.Debugf(logging.CategoryGameState, "ActionHoleUpLargeMap: vehicleType=%v, NoPartyVehicle=%v", vehicleType, references.NoPartyVehicle)
	if vehicleType != references.NoPartyVehicle && vehicleType != references.HorseVehicle && vehicleType != references.CarpetVehicle {
		g.SystemCallbacks.Message.AddRowStr("On foot!")

		return false
	}
	if !requestDuration && guardIndex != NoGuardAssigned && !g.isValidCampingGuard(guardIndex) {
		g.SystemCallbacks.Message.AddRowStr("Who will guard?")

		return false
	}
	if vehicleType == references.CarpetVehicle && g.PartyState.Inventory.QuestItems.Get(references.Carpet) >= party_state.ItemQuantitySmallMax {
		g.SystemCallbacks.UI.ShowActionFeedback("No room to stow the carpet!", true)

		return false
	}

	// Check terrain before camping: reject water and water-edge travel surfaces. ref:CAMP-0001
	if currentTile != nil {
		tileIndex := int(currentTile.Index)
		if tileIndex >= WaterTileMin && tileIndex <= WaterTileMax {
			g.SystemCallbacks.Message.AddRowStr("On land or ship!")

			return false
		}
	}

	// Check if this is the default call (normal mode) or debug mode with explicit parameters
	if requestDuration {
		// Normal mode - show duration selection dialog
		g.SystemCallbacks.Flow.Rest.RequestCampDuration()
	} else {
		// Debug mode - execute camping directly with specified parameters
		// Save current player location to return after camping
		originalPlayerLocation := g.MapState.SavePlayerLocation()

		// Load the camping combat map reference for authentic positions
		campingMapRef, err := references.LoadCampingMap(g.GameConfig)
		if err != nil {
			// Startup validated the original data, so the camp map must load.
			panic(fmt.Sprintf("GameState camping: loading camping map: %v", err))
		}
		campingScene := camping.NewCampingSceneWithCombatRef(hours, guardIndex, originalPlayerLocation, campingMapRef)

		// Create a temporary event queue for debug camping and add sequence to it
		q := g.EventQueueManager.Workflow("camping_debug_workflow").
			Priority(event_queue.PriorityNormal).
			Sequential().BlockInput().
			First(func() {})

		g.AddCampingSequenceToBuilder(q, campingScene, forceWanderer, forceAmbush, nil)

		// Start the queue after adding camping sequence
		q.Start()
	}

	return true
}

// CampingState tracks the state of camping across Event Queue callbacks
type CampingState struct {
	TotalCycles    int
	CurrentCycle   int
	Interrupted    bool
	DeathOccurred  bool
	GuardIndex     int                 // -1 means no guard
	GuardPosition  references.Position // Guard's current position during camping
	EncounterCheck bool

	// NEW FIELDS for wanderer and ambush system:
	WandererEncountered bool // Flag if wanderer will appear
	AmbushedAt          int  // Cycle when ambushed (NoAmbush if not)
	ForceWanderer       bool // Debug flag
	ForceAmbush         bool // Debug flag
	BardMusicPlaying    bool // Flag if bard music is currently playing
	AmbushCheckHour     byte // Last live Britannian hour checked for an ambush
	ActivePlayerIndex   int  // Exploration selection restored after a camping encounter

	partyInvisible          [party_state.MaxActiveParty]bool
	specialRingsInitialized bool
}

// AddCampingSequenceToBuilder adds the complete camping sequence to the event queue builder.
// This function should ONLY be called during queue setup phase, never from inside executing callbacks.
// All event queue builder methods (Then, Wait, When, etc.) are called here to fully describe
// the camping workflow before the queue starts executing.
func (g *GameState) AddCampingSequenceToBuilder(
	q *event_queue.SequentialEventBuilder,
	campingScene *camping.CampingScene,
	forceWanderer bool,
	forceAmbush bool,
	completionCleanup func(),
) {
	// This step runs after duration/watch selection and before any camp time,
	// sleep state or ambush. Merely opening or cancelling the prompt cannot stow.
	q.ThenSequence(func(active *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		if campingScene.TotalCycles <= 0 || !g.commitOutdoorCampingVehicle(campingScene.GetOriginalPlayerLocation()) {
			if completionCleanup != nil {
				completionCleanup()
			}

			return active
		}
		g.addCampingSequenceToBuilder(active, campingScene, forceWanderer, forceAmbush, completionCleanup)

		return active
	})
}

func (g *GameState) commitOutdoorCampingVehicle(expected references.PlayerLocation) bool {
	if g.MapState.PlayerLocation != expected {
		g.SystemCallbacks.UI.ShowActionFeedback("Camping location changed!", true)

		return false
	}
	vehicle := g.PartyVehicle.GetVehicleDetails().VehicleType
	if vehicle != references.NoPartyVehicle && vehicle != references.HorseVehicle && vehicle != references.CarpetVehicle {
		g.SystemCallbacks.UI.ShowActionFeedback("On foot!", true)

		return false
	}
	position := expected.Position
	if tile := g.GetLayeredMapByCurrentLocation().GetTopTile(&position); tile != nil &&
		tile.Index >= WaterTileMin && tile.Index <= WaterTileMax {
		g.SystemCallbacks.UI.ShowActionFeedback("On land or ship!", true)

		return false
	}
	if vehicle == references.CarpetVehicle {
		inventory := &g.PartyState.Inventory.QuestItems
		if inventory.Get(references.Carpet) >= party_state.ItemQuantitySmallMax {
			g.SystemCallbacks.UI.ShowActionFeedback("No room to stow the carpet!", true)

			return false
		}
		inventory.IncrementByOne(references.Carpet)
		g.PartyVehicle = map_units.NewNPCFriendlyVehiceNoVehicle()
	}

	return true
}

//nolint:maintidx // One cohesive workflow graph keeps camp, Wanderer and cleanup ordering explicit.
func (g *GameState) addCampingSequenceToBuilder(
	q *event_queue.SequentialEventBuilder,
	campingScene *camping.CampingScene,
	forceWanderer, forceAmbush bool,
	completionCleanup func(),
) {
	campingState := g.newCampingState(campingScene, forceWanderer, forceAmbush)

	logging.Debugf(logging.CategoryGameState, "Starting camping sequence with scene for %d hours (%d cycles), guard: %d",
		campingScene.Hours, campingState.TotalCycles, campingScene.GuardIndex)

	if campingState.TotalCycles <= 0 {
		logging.Debugf(logging.CategoryGameState, "No cycles to execute - returning")

		return
	}

	// Add camping sequence to the existing event queue
	q.
		Then(func() {
			campingScene.StartCamping()
			if err := g.TransitionToCampingMap(); err != nil {
				panic("Failed to transition to camping map - critical system error")
			}
			g.SystemCallbacks.Screen.SetMessageDialogFadeDurations(WandererFadeAnimationMs, WandererFadeAnimationMs)
			g.initializeCampingPartyRings(campingState)
			g.setupCampingScene(campingScene)

			g.SystemCallbacks.Audio.Play(SoundWind).Loop().Start()
		}).
		Wait(WandererFadeAnimationMs).
		Then(func() {
			g.SystemCallbacks.Message.AddRowStr("Setting camp...")
		}).
		Then(func() {
			g.setupBardGuard(campingScene, campingState)
		}).
		Then(func() {
			g.SystemCallbacks.Message.AddRowStr("")
			g.SystemCallbacks.Message.AddRowStr("Zzzzzzz...")
		}).If(func() bool { return campingState.BardMusicPlaying }, func(sequence *event_queue.SequentialEventBuilder) {
		// Original rest rules complete presentation wait before entering the camp loop.
		// As in dungeon camping, await presentation without cosmetic-update RNG
		// or logical cycles; guard rolls must not depend on update increments.
		// See ref:CAMP-0005.
		sequence.Then(func() {}).Wait(BardMusicDurationMs).Then(func() {
			g.finishBardGuardPerformance(campingState)
		})
	}).
		Then(campingScene.BeginSleeping).
		RepeatWithRuntimeDelay(
			campingState.TotalCycles,
			func() int64 { return campingCycleDelay(campingScene) },
			func(_ int) bool {
				if campingState.Interrupted {
					return false
				}

				if !campingScene.AdvanceExactCycle() {
					return false
				}

				campingState.CurrentCycle = campingScene.CurrentCycle

				interrupted := g.processSingleCampingCycle(campingScene, campingState)
				if interrupted {
					campingState.Interrupted = true

					return false
				}

				return true
			},
		).
		Then(func() {
			campingScene.EndSleeping()
			result := g.resolveCampingRestAndWanderer(campingScene, campingState)
			if !campingState.WandererEncountered && result != "" {
				g.SystemCallbacks.Screen.CreateAndPushCampingResultDialog(result)
			} else {
				g.SystemCallbacks.Screen.CreateAndPushMessageDialog()
			}
		}).If(

		func() bool { return campingState.WandererEncountered }, func(q *event_queue.SequentialEventBuilder) {
			leveledUpByCharacter := make([]bool, party_state.MaxActiveParty)
			wandererChanges := transientStatusChanges{}

			q.Then(func() {
				if g.CampingScene == nil {
					panic("executeCampingSequenceWithScene: g.CampingScene is nil in wanderer entrance - should never happen")
				}

				logging.Debugf(logging.CategoryGameState, "Camping: Wanderer entrance - materializing apparition")

				g.SystemCallbacks.Screen.ClearMessageDialog()

				g.SystemCallbacks.Message.AddRowStr("An apparition!")
				g.SystemCallbacks.Audio.PlaySoundEffect(SoundApparitionAppears)

				g.CampingScene.SetWandererPresent(true)

				g.SystemCallbacks.Visual.InitializeWandererDissolveEffect()

				g.SystemCallbacks.Visual.SetWandererDissolveLevel(FullyDissolved)
				g.CampingScene.SetWandererDissolveLevel(FullyDissolved)
			}).
				Wait(int64(g.SystemCallbacks.Screen.GetMessageDialogFadeInDuration())).
				Animate(WandererFadeAnimationMs, func(progress float32) {
					if g.CampingScene == nil {
						logging.Errorf(logging.CategoryGameState, "Camping: CampingScene is nil during wanderer fade-in at progress=%.2f", progress)
						panic("executeCampingSequenceWithScene: g.CampingScene is nil during fade-in animation - should never happen")
					}

					dissolveLevel := float64(FullyDissolved - progress)
					g.SystemCallbacks.Visual.SetWandererDissolveLevel(dissolveLevel)
					g.CampingScene.SetWandererDissolveLevel(dissolveLevel)

					if int(progress*100)%25 == 0 {
						logging.Debugf(logging.CategoryGameState, "Camping: Wanderer fade-in animation progress=%.1f%%", progress*100)
					}
				}).
				Then(func() {
					g.SystemCallbacks.Visual.SetWandererDissolveLevel(FullyVisible)
					g.CampingScene.SetWandererDissolveLevel(FullyVisible)
				}).
				Wait(WandererVisiblePauseMs).ThenSequence(

				func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
					for i := range party_state.MaxActiveParty {
						charIndex := i

						q = q.Then(func() {
							leveledUp, changes := g.processWandererCharacter(charIndex, campingState)
							leveledUpByCharacter[charIndex] = leveledUp
							wandererChanges.healing = wandererChanges.healing || changes.healing
							wandererChanges.poisonCure = wandererChanges.poisonCure || changes.poisonCure
							wandererChanges.levelUp = wandererChanges.levelUp || changes.levelUp
						})

						q = q.If(
							func() bool { return leveledUpByCharacter[charIndex] },
							func(subQ *event_queue.SequentialEventBuilder) {
								subQ.ThenSequence(
									NewPartyStateBuilder(g).
										ToCharacter(charIndex).
										FlashInverted().
										WithSound(SoundLevelUp).
										Build()).
									Then(func() {
										g.SystemCallbacks.Screen.ShowPressAnyKeyPrompt()
									}).
									WaitForAnyKey().
									Then(func() {
										g.SystemCallbacks.Screen.ClearMessagePrompt()
									})
							})

						if i < party_state.MaxActiveParty-1 {
							q = q.Wait(750)
						}
					}

					return q
				}).
				Then(func() {
					g.publishTransientStatusChanges(wandererChanges)
				}).
				Then(func() {
					karmaMsg, err := g.GameReferences.KarmaReports.Select(
						g.PartyState.Karma.Value,
						references.KarmaReportCamping,
					)
					if err != nil {
						panic(fmt.Sprintf("selecting camping karma report: %v", err))
					}

					g.SystemCallbacks.Message.AppendStyledMessage(StyledMessage{
						LeadingBlank: true,
						Segments: []MessageSegment{
							{Text: "\"", Style: MessageQuotation},
							{Text: karmaMsg, Style: MessageSpoken},
							{Text: "\"", Style: MessageQuotation},
						},
					})
					g.SystemCallbacks.Screen.ShowPressAnyKeyPrompt()
				}).
				WaitForAnyKey().
				Then(func() {
					g.SystemCallbacks.Screen.ClearMessagePrompt()
					g.SystemCallbacks.Message.AddRowStr(" ")
					g.SystemCallbacks.Message.AddRowStr("The strangely familiar old man vanishes...")
				}).
				Wait(WandererDepartureDelayMs).
				Animate(WandererFadeAnimationMs, func(progress float32) {
					if g.CampingScene == nil {
						logging.Errorf(
							logging.CategoryGameState,
							"Camping: CampingScene nil during wanderer fade-out at progress=%.2f",
							progress,
						)
						panic("executeCampingSequenceWithScene: CampingScene nil during wanderer fade-out")
					}

					dissolveLevel := float64(progress)
					g.SystemCallbacks.Visual.SetWandererDissolveLevel(dissolveLevel)
					g.CampingScene.SetWandererDissolveLevel(dissolveLevel)

					if int(progress*100)%25 == 0 {
						logging.Debugf(logging.CategoryGameState, "Camping: Wanderer fade-out animation progress=%.1f%%", progress*100)
					}
				}).
				Then(func() {
					logging.Debugf(logging.CategoryGameState, "Camping: Wanderer fade-out animation completed")
				}).
				Then(func() {
					if g.CampingScene == nil {
						panic("executeCampingSequenceWithScene: g.CampingScene is nil during wanderer cleanup - should never happen")
					}

					g.CampingScene.SetWandererPresent(false)
					g.SystemCallbacks.Visual.SetWandererDissolveLevel(FullyVisible)

					g.SystemCallbacks.Visual.CleanupWandererDissolveEffect()
				}).
				Then(func() {
					g.SystemCallbacks.Screen.ShowPressAnyKeyPrompt()
				}).
				WaitForAnyKey().
				Then(func() {
					g.SystemCallbacks.Screen.ClearMessagePrompt()
				}).
				Then(func() {
					g.SystemCallbacks.Screen.HideMessageDialog()

					logging.Debugf(logging.CategoryGameState, "Camping: MessageDialog fade-out animation started")
				}).
				Wait(int64(g.SystemCallbacks.Screen.GetMessageDialogFadeOutDuration())).
				Then(func() {
					g.SystemCallbacks.Screen.PopMessageDialog()

					logging.Debugf(logging.CategoryGameState, "Camping: MessageDialog fade-out completed, dialog popped from stack")
				}).
				Wait(1250)
		}).If(

		func() bool {
			return !campingState.WandererEncountered && !campingState.Interrupted
		}, func(q *event_queue.SequentialEventBuilder) {
			q.Then(func() {
				g.SystemCallbacks.Screen.ShowPressAnyKeyPrompt()
			}).
				WaitForAnyKey().
				Then(func() {
					g.SystemCallbacks.Screen.ClearMessagePrompt()

					g.SystemCallbacks.Screen.HideMessageDialog()
					logging.Debugf(logging.CategoryGameState, "Camping: Non-wanderer - MessageDialog fade-out started")
				}).
				Wait(int64(g.SystemCallbacks.Screen.GetMessageDialogFadeOutDuration())).
				Then(func() {
					g.SystemCallbacks.Screen.PopMessageDialog()
					logging.Debugf(logging.CategoryGameState, "Camping: Non-wanderer - MessageDialog popped")
				})
		}).TakeOverFinally(func() {
		g.finalizeCampingSequence(campingScene, campingState, completionCleanup)
	})

	// Note: Don't call Start() - we're adding to an existing queue that's already started
}

func (g *GameState) resolveCampingRestAndWanderer(
	campingScene *camping.CampingScene,
	campingState *CampingState,
) string {
	eligibleForRest := g.LastCamped < 1 && campingScene.Hours > MinimumWandererHours
	eligibleForWanderer := eligibleForRest || campingState.ForceWanderer

	logging.Debugf(logging.CategoryGameState, "Camping: Checking wanderer conditions - hours=%d (req >5 or forced=%t), interrupted=%t, death=%t",
		campingScene.Hours, campingState.ForceWanderer, campingState.Interrupted, campingState.DeathOccurred)
	if campingState.Interrupted || campingState.DeathOccurred {
		logging.Debugf(logging.CategoryGameState, "Camping: Wanderer conditions NOT met - skipping rest and wanderer check")

		return ""
	}

	// Camp restoration restores each eligible party member before rolling for the Wanderer.
	// See ref:CAMP-0005.
	result := g.applyCampingHealing(campingScene)
	if !eligibleForWanderer {
		logging.Debugf(logging.CategoryGameState, "Camping: Wanderer conditions NOT met - skipping wanderer check")

		return result
	}

	logging.Debugf(logging.CategoryGameState, "Camping: Wanderer conditions met - calling CheckForWandererAppearance")
	campingState.WandererEncountered = g.CheckForWandererAppearance(campingState.ForceWanderer)

	return result
}

func (g *GameState) newCampingState(
	campingScene *camping.CampingScene,
	forceWanderer bool,
	forceAmbush bool,
) *CampingState {
	return &CampingState{
		TotalCycles:             campingScene.TotalCycles,
		CurrentCycle:            0,
		Interrupted:             false,
		DeathOccurred:           false,
		GuardIndex:              campingScene.GuardIndex,
		GuardPosition:           campingScene.GetPartyMemberPosition(campingScene.GuardIndex),
		EncounterCheck:          false,
		WandererEncountered:     false,
		AmbushedAt:              NoAmbush,
		ForceWanderer:           forceWanderer,
		ForceAmbush:             forceAmbush,
		BardMusicPlaying:        false,
		AmbushCheckHour:         g.DateTime.Hour,
		ActivePlayerIndex:       g.GetActivePlayerIndex(),
		partyInvisible:          [party_state.MaxActiveParty]bool{},
		specialRingsInitialized: false,
	}
}

func (g *GameState) finalizeCampingSequence(
	campingScene *camping.CampingScene,
	campingState *CampingState,
	completionCleanup func(),
) {
	logging.Debugf(logging.CategoryGameState, "Camping: Finally() cleanup starting at time=%d", g.ElapsedMs)
	campingScene.EndSleeping()
	if g.CampingScene == nil {
		logging.Warnf(logging.CategoryGameState,
			"Camping: Finally() called but CampingScene already nil - possible double cleanup?")
	}
	if campingState.Interrupted && !campingState.WandererEncountered {
		g.SystemCallbacks.Screen.ClearMessagePrompt()
		g.SystemCallbacks.Screen.ClearMessageDialog()
		g.SystemCallbacks.Screen.HideMessageDialog()
		g.SystemCallbacks.Screen.PopMessageDialog()
	}
	if g.CombatScene != nil {
		panic("camping workflow cannot clean up after combat has started")
	}

	g.TransitionFromCampingMap()
	g.MapState.RestorePlayerLocation(campingScene.GetOriginalPlayerLocation())
	g.cleanupCampingScene(campingScene, campingState)
	g.SystemCallbacks.Audio.Stop(SoundWind).Now()
	g.CampingScene = nil
	if g.pendingCampingCombat == nil {
		g.restorePreCombatActivePlayer(campingState.ActivePlayerIndex)
	}
	if completionCleanup != nil {
		completionCleanup()
	}
	logging.Debugf(logging.CategoryGameState,
		"Camping: Finally() cleanup completed - CampingScene set to nil at time=%d", g.ElapsedMs)
}

// processSingleCampingCycle processes a single camping cycle and returns true if camping was interrupted
func (g *GameState) processSingleCampingCycle(campingScene *camping.CampingScene, campingState *CampingState) bool {
	g.ProcessRegenerationRingSweep()

	// Original rest rules check the hour crossed by the previous cycle before advancing this one.
	// See ref:CAMP-0005.
	hourChanged := g.DateTime.Hour != campingState.AmbushCheckHour
	forcedAmbush := campingState.ForceAmbush && campingScene.CurrentCycle == DebugAmbushCycle
	if campingState.AmbushedAt == NoAmbush && (hourChanged || forcedAmbush) {
		// Prevent random ambushes when testing wanderer in debug mode
		if campingState.ForceWanderer && !campingState.ForceAmbush {
			logging.Debugf(logging.CategoryGameState, "DEBUG: Skipping random ambush check - testing wanderer")
		} else if g.CheckForAmbushDuringHour(forcedAmbush) {
			ambush, err := g.QueueOutdoorCampingAmbush(
				campingScene, campingState, campingState.ActivePlayerIndex,
			)
			if err != nil {
				logging.Errorf(logging.CategoryGameState, "Unable to prepare camping combat: %v", err)
				g.SystemCallbacks.UI.ShowActionFeedback("Combat could not begin.", true)
				campingState.Interrupted = true
				campingScene.InterruptCamping()

				return true
			}
			campingState.AmbushedAt = campingScene.CurrentCycle
			campingState.Interrupted = true

			// Request the ambush-alert cue after combat preparation succeeds.
			g.SystemCallbacks.Audio.PlaySoundEffect(SoundAmbushAlert)

			g.SystemCallbacks.Message.AddRowStr("") // Blank line for separation
			g.SystemCallbacks.Message.AddRowStr("Ambushed!")
			logging.Infof(logging.CategoryGameState,
				"Prepared camping ambush: monster=%s primary=%d friends=%d",
				ambush.MonsterType,
				ambush.PrimaryCount,
				ambush.FriendCount,
			)

			campingScene.InterruptCamping()

			return true // Interrupt camping
		}
	}
	if hourChanged {
		campingState.AmbushCheckHour = g.DateTime.Hour
	}

	// REDUX DEVIATION: original rest rules stop at a target hour and can lose the
	// starting-minute offset. Redux advances the requested H*60 minutes exactly.
	// See ref:CAMP-0005.
	g.advanceGameplayMinutes(camping.MinutesPerCycle, false)

	// Guard wandering
	// Guard randomly moves around camp to simulate keeping watch
	// 25% chance per cycle, only if guard exists and not playing music
	// See ref:CAMP-0005.
	g.tryGuardWander(campingState)

	return false // Camping not interrupted
}

func (g *GameState) finishBardGuardPerformance(campingState *CampingState) {
	if !campingState.BardMusicPlaying {
		return
	}
	g.SystemCallbacks.Audio.Stop(SoundBardLute).Now()
	g.SpriteOverrides.Clear("camping", "guard")
	campingState.BardMusicPlaying = false
}

// tryGuardWander attempts to move the guard randomly during camping
// See ref:CAMP-0005.
func (g *GameState) tryGuardWander(campingState *CampingState) {
	// No guard or invalid guard index
	if !g.isValidCampingGuard(campingState.GuardIndex) {
		return
	}

	// Don't wander while bard is playing music (bard stands still with instrument)
	if campingState.BardMusicPlaying {
		return
	}

	// GuardMoveOdds chance to move per cycle (original: random(0,3) == 2 for 25%)
	if !g.OneInXOdds(GuardMoveOdds) {
		return
	}

	// Pick random direction (original: random(0,3) for 4 directions)
	directionRoll := g.RollDice(DirectionCount)
	var direction references.Direction
	switch directionRoll {
	case 1:
		direction = references.Up // North (tempy--)
	case 2:
		direction = references.Down // South (tempy++)
	case 3:
		direction = references.Right // East (tempx++)
	case 4:
		direction = references.Left // West (tempx--)
	default:
		direction = references.Up // Fallback
	}

	// Calculate new position
	newPosition := direction.GetNewPositionInDirection(&campingState.GuardPosition)

	// Validate new position (original: onscreen && legalmove2)
	if !g.isValidCampingPosition(newPosition) {
		logging.Debugf(logging.CategoryGameState,
			"Camping: Guard wander attempted %v but position invalid/impassable", direction)

		return
	}

	// Move guard to new position
	oldPosition := campingState.GuardPosition
	campingState.GuardPosition = *newPosition

	logging.Debugf(logging.CategoryGameState,
		"Camping: Guard wandered %v from (%d,%d) to (%d,%d)",
		direction, oldPosition.X, oldPosition.Y, newPosition.X, newPosition.Y)

	// Request the guard footstep cue after committing the move.
	g.SystemCallbacks.Audio.PlaySoundEffect(SoundGuardFootstepGrass)

	// Update visual representation
	if g.CampingScene != nil {
		g.CampingScene.UpdateGuardPosition(*newPosition)
	}
}

// isValidCampingPosition checks if a position is valid for guard movement in camping scene
// Original: onscreen(tempx, tempy) && legalmove2(gtile, tempx, tempy)
func (g *GameState) isValidCampingPosition(position *references.Position) bool {
	// Check if camping scene exists
	if g.CampingScene == nil || g.CampingScene.CampingMap == nil {
		return false
	}

	// Basic bounds check (camping map is typically 11x11)
	// This matches the original's combat bounds check
	// See ref:CAMP-0005.
	const campingMapWidth = 11
	const campingMapHeight = 11

	if position.X < 0 || position.X >= campingMapWidth ||
		position.Y < 0 || position.Y >= campingMapHeight {
		return false
	}
	if g.CampingScene.CampingMap.HasGeneratedDungeonChestAt(*position) {
		return false
	}

	// Check tile passability using LayeredMap (from BRIT.CBT via ResetAndCreateCombatMap)
	// Original uses creature terrain validation for passability check
	// See ref:CAMP-0005.
	combatLayeredMap := g.MapState.LayeredMaps.GetLayeredMap(references.CombatMapType, 0)
	if combatLayeredMap == nil {
		logging.Debugf(logging.CategoryGameState,
			"Camping: No combat LayeredMap available for guard validation")

		return false
	}

	// Get the tile from the LayeredMap
	tile := combatLayeredMap.GetTopTile(position)
	if tile == nil {
		logging.Debugf(logging.CategoryGameState,
			"Camping: Guard movement blocked at (%d,%d) - no tile found",
			position.X, position.Y)

		return false
	}

	// Check if tile is passable for walking (NoPartyVehicle = on foot)
	if !tile.IsPassable(references.NoPartyVehicle) {
		logging.Debugf(logging.CategoryGameState,
			"Camping: Guard movement blocked at (%d,%d) - tile %s (index %d) is not passable",
			position.X, position.Y, tile.Name, tile.Index)

		return false
	}

	// Check if any party member is at this position (guard shouldn't walk through sleeping party members)
	for i := range party_state.MaxActiveParty {
		if i == g.CampingScene.GuardIndex {
			continue // Skip the guard themselves
		}
		if g.PartyState.GetCharacter(i).PartyMap != party_state.PartyMapActive {
			continue
		}
		partyMemberPos := g.CampingScene.GetPartyMemberPosition(i)
		if partyMemberPos.X == position.X && partyMemberPos.Y == position.Y {
			logging.Debugf(logging.CategoryGameState,
				"Camping: Guard movement blocked at (%d,%d) - party member %d is there",
				position.X, position.Y, i)

			return false // Party member blocking
		}
	}

	return true
}

// setupCampingScene handles initial camping scene preparation
// Original: original rest rules (music, sleep status)
// See ref:CAMP-0005.
func (g *GameState) setupCampingScene(campingScene *camping.CampingScene) {
	// Note: No "Setting camp..." message in original
	// See ref:CAMP-0005.
	guardWasGood := g.isValidCampingGuard(campingScene.GuardIndex)
	g.setActiveCampingPartySleepStatus(true)
	if guardWasGood {
		g.PartyState.GetCharacter(campingScene.GuardIndex).SetStatus(party_state.Good)
	}
	g.ClearActivePlayer()
	g.ClearDurationEffect()

	// Hide all overlays from original map (moongates, NPCs, items, quest items, ambient sounds)
	g.SystemCallbacks.Screen.HideAllOverlays()

	// Pause background map music during camping (will be resumed when camping ends)
	g.SystemCallbacks.Audio.PauseBackgroundMusic()

	// Request the camping-start cue after pausing background music.
	g.SystemCallbacks.Audio.PlaySoundEffect(SoundCampingStart)

	// Dungeon center features can replace the campfire. Follow the presented
	// camp terrain after transition, rather than assuming every camp has a fire.
	fireTile := g.GetLayeredMapByCurrentLocation().GetTopTile(&campingScene.CampingMap.CampfirePosition)
	if fireTile != nil && fireTile.Index == indexes.CampFire {
		g.SystemCallbacks.Audio.Play(SoundFireCrackle).Loop().Start()
	}

	logging.Debugf(logging.CategoryGameState, "Camping: Scene setup - overlays hidden, music paused")

	// Store the camping scene in GameState for access during camping
	g.CampingScene = campingScene
	logging.Debugf(logging.CategoryGameState, "Camping: Scene activated with %d party members",
		g.PartyState.CountActiveMembers())
}

// setupBardGuard handles bard guard music and animation setup
// See ref:CAMP-0005.
func (g *GameState) setupBardGuard(campingScene *camping.CampingScene, campingState *CampingState) {
	// A Bard guard performs the music sequence once at camp setup, not each cycle.
	// Use the playing pose, start the song, await presentation, then restore the
	// normal pose before guard and encounter work. ref:CAMP-0004
	if !g.isValidCampingGuard(campingScene.GuardIndex) {
		return
	}

	guard := g.PartyState.GetCharacter(campingScene.GuardIndex)

	if guard.Class != party_state.Bard {
		return
	}

	// Set bard to playing animation (equivalent to BANJO_PLAYER+3 in original)
	// This is now implemented via GetGuardSpriteForCamping checking IsBardPlayingMusic flag
	// Start the bard song (equivalent to song_pos=1 in original)
	g.SystemCallbacks.Audio.Play(SoundBardLute).Loop().Start()

	// Set sprite override to show bard playing animation
	g.SpriteOverrides.Set("camping", "guard", indexes.BardPlayingKeyIndex)
	// The owning workflow awaits this performance before starting logical cycles.
	campingState.BardMusicPlaying = true

	logging.Debugf(logging.CategoryGameState, "Camping: Bard guard starting music performance")

	// Note: No "Time advances..." message in original
	// See ref:CAMP-0005.
}

// processWandererCharacter processes a single character's wanderer healing and level up
// Original: original Wanderer rules (per character loop)
// See ref:CAMP-0005.
func (g *GameState) processWandererCharacter(
	characterIndex int,
	campingState *CampingState,
) (bool, transientStatusChanges) {
	if !campingState.WandererEncountered {
		panic("Expected wanderer encountered before able to call processWandererCharacter")
	}

	char := g.PartyState.GetCharacter(characterIndex)
	if char.PartyMap != party_state.PartyMapActive {
		return false, transientStatusChanges{}
	}

	outcome := g.ProcessWandererHealingForCharacter(characterIndex)
	if !outcome.Restored {
		return false, transientStatusChanges{}
	}
	changes := transientStatusChanges{
		healing:    outcome.HPIncreased,
		poisonCure: outcome.PoisonCured,
		levelUp:    outcome.LevelAfter > outcome.LevelBefore,
	}

	g.SystemCallbacks.Message.AppendStyledMessage(StyledMessage{
		LeadingBlank: true,
		Segments: []MessageSegment{
			{Text: "\"", Style: MessageQuotation},
			{Text: fmt.Sprintf("Awake %s and feel restored!", char.GetNameAsString()), Style: MessageSpoken},
			{Text: "\"", Style: MessageQuotation},
		},
	})

	if outcome.LevelAfter > outcome.LevelBefore {
		g.showLevelUpMessages(char, outcome.StatBoosts)
		// Note: Visual/audio feedback (FlashInverted + SoundLevelUp) is handled
		// by PartyStateBuilder in the calling event queue's If() block
		return true, changes
	}
	if outcome.LevelAfter < outcome.LevelBefore {
		g.SystemCallbacks.Message.AppendStyledMessage(StyledMessage{
			Segments: []MessageSegment{{
				Text:  fmt.Sprintf("%s's level is corrected to %d.", char.GetNameAsString(), outcome.LevelAfter),
				Style: MessageSpoken,
			}},
		})
	}

	return false, changes
}

// showLevelUpMessages displays the wanderer's level up messages
// Handles both single and multi-level gains, showing all stat boosts
// See ref:CAMP-0005.
func (g *GameState) showLevelUpMessages(char *party_state.PlayerCharacter, statBoosts []string) {
	// Highlight the attained level and any stat awards, e.g. "stronger, quicker, wiser"
	// for three levels. A character with every stat at its maximum hears only the level.
	segments := []MessageSegment{
		{Text: "\"", Style: MessageQuotation},
		{Text: "Thou art now level", Style: MessageSpoken},
		{Text: fmt.Sprintf(" %d", char.Level), Style: MessageHighlight},
	}
	if len(statBoosts) > 0 {
		segments = append(segments,
			MessageSegment{Text: ", and ", Style: MessageSpoken},
			MessageSegment{Text: strings.Join(statBoosts, ", "), Style: MessageHighlight},
		)
	}
	segments = append(segments,
		MessageSegment{Text: "!", Style: MessageSpoken},
		MessageSegment{Text: "\"", Style: MessageQuotation},
	)
	g.SystemCallbacks.Message.AppendStyledMessage(StyledMessage{Segments: segments})
}

// cleanupCampingScene handles cleanup when leaving camping
func (g *GameState) cleanupCampingScene(campingScene *camping.CampingScene, campingState *CampingState) {
	// Stop ambient campfire crackling sound
	g.SystemCallbacks.Audio.Stop(SoundFireCrackle).Now()

	// Resume background map music after camping
	g.SystemCallbacks.Audio.ResumeBackgroundMusic()

	// Show all overlays when leaving camping (restore normal gameplay)
	g.SystemCallbacks.Screen.ShowAllOverlays()

	logging.Debugf(logging.CategoryGameState, "Camping: Cleanup - campfire stopped, music resumed, overlays restored, MessageDialog hidden")

	if campingState.BardMusicPlaying {
		g.SystemCallbacks.Audio.Stop(SoundBardLute).Now()
		campingState.BardMusicPlaying = false
	}

	// An unguarded ambush carries sleepers into combat. Every non-ambush exit
	// follows the original blanket wake-up at original rest rules.
	// See ref:CAMP-0005.
	if campingState.AmbushedAt == NoAmbush {
		g.setActiveCampingPartySleepStatus(false)
	}

	// Clear all camping sprite overrides
	g.SpriteOverrides.ClearContext("camping")
	logging.Debugf(logging.CategoryGameState, "Camping: Sprite overrides cleared")

	// Complete the camping scene
	campingScene.CompleteCamping()
}

func (g *GameState) isValidCampingGuard(guardIndex int) bool {
	if guardIndex < 0 || guardIndex >= party_state.MaxActiveParty {
		return false
	}

	character := g.PartyState.GetCharacter(guardIndex)

	return character.PartyMap == party_state.PartyMapActive && character.IsGood()
}

func (g *GameState) setActiveCampingPartySleepStatus(sleeping bool) {
	for partyIndex := range party_state.MaxActiveParty {
		character := g.PartyState.GetCharacter(partyIndex)
		if character.PartyMap != party_state.PartyMapActive {
			continue
		}
		if sleeping && character.IsGood() {
			character.SetStatus(party_state.Sleep)
		} else if !sleeping && character.IsAsleep() {
			character.SetStatus(party_state.Good)
		}
	}
}

// applyCampingHealing applies one-time healing and MP restoration after successful camping
// See ref:CAMP-0005.
func (g *GameState) applyCampingHealing(campingScene *camping.CampingScene) string {
	// Check if party should get rested benefit
	// Requires: last_camped < 1 AND hours > 5
	// See ref:CAMP-0005.
	if g.LastCamped >= 1 || campingScene.Hours <= MinimumWandererHours {
		// Failure case
		// Request the no-effect cue when rest grants no benefit.
		// See ref:CAMP-0005.
		g.SystemCallbacks.Audio.PlaySoundEffect(SoundNoEffect)
		g.SystemCallbacks.Message.AddRowStr("") // Blank line for separation
		g.SystemCallbacks.Message.AddRowStr("No effect...")
		logging.Debugf(logging.CategoryGameState, "Camping no effect - LastCamped=%d, Hours=%d", g.LastCamped, campingScene.Hours)

		return "No effect..."
	}

	// Success case
	// Request the successful-rest cue before applying restoration.
	// See ref:CAMP-0005.
	g.SystemCallbacks.Audio.PlaySoundEffect(SoundPartyRested)
	g.SystemCallbacks.Message.AddRowStr("") // Blank line for separation
	g.SystemCallbacks.Message.AddRowStr("Party rested!")

	// Apply one-time camping healing
	// See ref:CAMP-0005.
	changes := transientStatusChanges{}
	g.PartyState.ForEachCharacter(func(i int, char *party_state.PlayerCharacter) {
		if char.PartyMap != party_state.PartyMapActive {
			return
		}
		// Skip poisoned characters - they don't heal
		// See ref:CAMP-0005.
		if char.IsPoisoned() {
			return
		}

		// Skip dead and guard
		// See ref:CAMP-0005.
		if char.IsDead() || i == campingScene.GuardIndex {
			return
		}

		// ONE-TIME random heal 1-63 HP
		// See ref:CAMP-0005.
		hpBefore := char.GetCurrentHp()
		healAmount := uint16(g.RandomIntInRange(WandererHealMin, WandererHealMax))
		char.AddHp(healAmount) // Uses helper with auto-clamping
		changes.observeHealing(hpBefore, char)

		// MP restoration
		// See ref:CAMP-0005.
		char.RestoreMpByClass() // Clean helper method

		// Per-character healing audio is not requested here.
		// A dedicated per-character healing pulse is not emitted at this point.
		// g.SystemCallbacks.Audio.PlaySoundEffect(SoundHealingTick)
	})
	g.publishTransientStatusChanges(changes)

	// Set cooldown
	// See ref:CAMP-0005.
	g.LastCamped = 14 // 14-hour cooldown
	logging.Debugf(logging.CategoryGameState, "Camping success - Party rested! LastCamped set to 14")

	return "Party rested!"
}
