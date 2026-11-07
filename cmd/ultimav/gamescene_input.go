package main

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	// keyPressDelay = 120
	keyPressDelay = 165
)

//nolint:gochecknoglobals // Immutable keyboard binding table shared by scene input polling.
var boundKeysGame = []ebiten.Key{
	// Movement and basic actions
	ebiten.KeySpace,
	ebiten.KeyDown,
	ebiten.KeyUp,
	ebiten.KeyEnter,
	ebiten.KeyLeft,
	ebiten.KeyPeriod,
	ebiten.KeyRight,

	// Primary game commands (A-Z alphabetical)
	ebiten.KeyA, // Attack
	ebiten.KeyB, // Board
	ebiten.KeyC, // Cast spell
	ebiten.KeyD, // Original no-op command feedback
	ebiten.KeyE, // Enter
	ebiten.KeyF, // Fire/Flee
	ebiten.KeyG, // Get
	ebiten.KeyH, // Hole up/Heal
	ebiten.KeyI, // Ignite torch
	ebiten.KeyJ, // Jimmy
	ebiten.KeyK, // Klimb
	ebiten.KeyL, // Look
	ebiten.KeyM, // Mix reagents
	ebiten.KeyN, // New order / Ctrl+N: mark current position
	ebiten.KeyO, // Open
	ebiten.KeyP, // Push
	ebiten.KeyQ, // Quick save / Ctrl+Q: reload the active slot
	ebiten.KeyR, // Ready weapon/Resurrect
	ebiten.KeyS, // Search
	ebiten.KeyT, // Talk
	ebiten.KeyU, // Use
	ebiten.KeyV, // Volume
	ebiten.KeyW, // Original no-op command feedback
	ebiten.KeyX, // Exit
	ebiten.KeyY, // Yell
	ebiten.KeyZ, // Z-stats

	// Number keys for active player selection
	ebiten.Key0, // All party members (clear active player)
	ebiten.Key1, // Character 1
	ebiten.Key2, // Character 2
	ebiten.Key3, // Character 3
	ebiten.Key4, // Character 4
	ebiten.Key5, // Character 5
	ebiten.Key6, // Character 6
	ebiten.Key7,
	ebiten.Key8,
	ebiten.Key9,
	ebiten.KeyNumpad0,
	ebiten.KeyNumpad1,
	ebiten.KeyNumpad2,
	ebiten.KeyNumpad3,
	ebiten.KeyNumpad4,
	ebiten.KeyNumpad5,
	ebiten.KeyNumpad6,
	ebiten.KeyNumpad7,
	ebiten.KeyNumpad8,
	ebiten.KeyNumpad9,

	// Special keys
	ebiten.KeyTab,      // Combat auto advance/attack
	ebiten.KeySlash,    // Help
	ebiten.KeyF1,       // Help (alias for /)
	ebiten.KeyEscape,   // Escape menu
	ebiten.KeyF2,       // Combat detail toggle
	ebiten.KeyF3,       // Review noteworthy combat results
	ebiten.KeyF4,       // Review last completed battle
	ebiten.KeyPageUp,   // Combat history older
	ebiten.KeyPageDown, // Combat history newer
	ebiten.KeyHome,     // Combat history oldest
	ebiten.KeyEnd,      // Combat history live
}

// Update method for the GameScene
//
//nolint:maintidx // Ordered input-owner precedence stays together so early returns are auditable.
func (g *GameScene) Update(game *Game) error {
	inputAllowed := g.beginTransientCommandInputFrame()
	// Sample before readers, queues or buffered commands can change ownership.
	// A ready quickload returns before outgoing clocks, RNG, or update finalizers.
	if g.captureQuickLoadRequest(inputAllowed) && g.quickLoadInputEligible() {
		g.handleQuickLoad(game)

		return nil
	}
	if g.quickLoadChordHeld {
		// Mask raw any-key/text readers too, not just bound gameplay Q. The
		// current owner may advance presentation, but cannot accept this chord.
		inputAllowed = false
	}
	g.syncAutomaticSailingInterruption()
	defer g.updateActorAnimationClocks()
	// Presentation service must run even when a reader consumes the whole frame.
	defer g.updateSceneMusic()
	if inputAllowed {
		defer g.finishTransientCommandInputFrame()
	} else {
		// Reject input, not presentation. Keep the ordinary sky, wind, queue,
		// automatic-door and effect path; retained readers still own a full pause.
		restoreKeyboard := g.keyboard.SuspendInputPolling()
		defer restoreKeyboard()
		g.suspendNativeDialogInput()
		if g.atlasSession != nil || g.journalSession != nil {
			return nil
		}
		restoreQueueInput := g.gameState.EventQueueManager.SuspendInputPolling()
		defer restoreQueueInput()
	}
	defer g.updateJournalNoticeAfterFrame(g.knowledgeNoticeBlocked())
	g.observeProvisionChanges()
	defer g.observeProvisionChanges()
	// Capture the hidden chord before any retained reader can accept raw T.
	if g.captureRunicTranslationChord() {
		g.keyboard.ClearFrameConsumption()
		g.syncRunicReference()
		g.handleRunicTranslationShortcut()

		return nil
	}
	// A reader frame, including dismissal, precedes every gameplay update.
	if g.atlasSession != nil {
		g.keyboard.ClearFrameConsumption()
		g.updateAtlasSession()

		return nil
	}
	g.keyboard.ClearFrameConsumption()
	atlasRequested := g.pollAtlasRequest()
	markRequested := g.pollAtlasMarkRequest()
	if atlasRequested && g.canOpenAtlas() || markRequested && g.canOpenAtlasMark() {
		// Pending encounters are excluded by canOpenAtlas; automatic NPC speech
		// still precedes opening just as it precedes ordinary player commands.
		if g.processNPCAutoTalk() {
			return nil
		}
		g.openAtlas(game)
		if markRequested {
			g.handleAtlasNoteIntent(widgets.AtlasNoteIntent{Action: widgets.AtlasNoteMark, ID: 0})
		}

		return nil
	}
	g.syncSkyPresentation()
	// Clear frame-based key consumption at start of frame
	// This allows each frame to have fresh key state, preventing same-frame duplicate key processing
	g.keyboard.ClearFrameConsumption()
	g.syncRunicReference()
	defer g.syncRunicReference()
	if g.handleRunicReferenceShortcut() {
		return nil
	}
	if g.journalSession != nil {
		if g.journalSession.closing {
			if !g.keyboard.IsWaitingForRelease() {
				g.finishJournalPause(g.journalSession)
			}

			return nil
		}
		g.updateDialogs()
		// The closing key must not advance gameplay in its dismissal frame.
		return nil
	}
	if g.finalePresentation == nil && !g.textEntryFocused() &&
		(g.keyboard.IsModifierComboPressed(input.ModifierShift, ebiten.KeyJ) &&
			g.keyboard.IsKeyPressedAndRegister(ebiten.KeyJ) ||
			g.keyboard.IsKeyPressedAndRegister(widgets.QuestJournalFunctionKey)) {
		g.ResetTransientCommandInput()
		g.gameState.ActionQuestJournal()

		return nil
	}
	g.clearDoomRunAssociationOutsideRun()
	switch g.updateLordBritishFinaleInput() {
	case finaleInputReturnBeforeDoom:
		if err := game.returnBeforeDoom(g); err != nil {
			logging.Warnf(logging.CategoryGameState, "Unable to restore pre-Doom recovery checkpoint: %v", err)
			g.showDoomRecoveryFailure(err)
		}

		return nil
	case finaleInputReturnToMainMenu:
		if err := game.returnToMainMenu(g.gameConfig); err != nil {
			return fmt.Errorf("returning to main menu after Lord British finale: %w", err)
		}

		return nil
	case finaleInputNone:
	}

	if inputAllowed {
		g.captureTransientCommandInput()
	}
	// Update wind and visual effects using real time
	g.updateWindAndEffects()
	g.syncAutomaticSailingInterruption()
	g.reconcileTransientInputAfterQueues()
	consumedFinaleHandoff, err := g.consumeDoomMirrorFinaleHandoff()
	if err != nil {
		return err
	}
	if consumedFinaleHandoff {
		return nil
	}
	// Expire transient feedback.
	g.activeEffectsOverlay.Update(g.gameState.ElapsedMs)

	// Update auto-open door animation if active
	if g.autoOpenDoorState != nil && g.autoOpenDoorState.IsActive() {
		g.autoOpenDoorState.Update(g, g.getElapsedMs())
		// Don't process other input while animation is playing
		return nil
	}

	// Note: Teleportation visual effects are now handled by event queue system
	// (updated automatically via EventQueueManager.Update() in GameState)

	// Update dialogs BEFORE input blocking check - dissolve animations must progress even when input blocked
	if !inputAllowed && g.updateDialogsWithoutInput() {
		return nil
	}
	if g.updateDialogs() {
		// only run a single update routine at a time
		return nil
	}
	g.handleCampingAcceleration()

	// Block all input during teleportation sequence
	if g.inputBlocked {
		// Continue updating visual effects but don't process input
		return nil
	}
	if !g.gameState.CanAcceptGameplayCommands() {
		return nil
	}
	if g.showPendingBattleSummary() {
		return nil
	}

	if g.gameState.HasPendingOverworldContact() {
		if g.gameState.ShouldDeferOverworldContact() {
			return nil
		}
		if err := g.gameState.ResolvePendingOverworldContact(); err != nil {
			logging.Errorf(logging.CategoryGameState, "Unable to resolve overworld contact: %v", err)
			g.gameState.SystemCallbacks.Message.AddRowStr("Combat could not begin.")
		}

		return nil
	}

	if g.gameState.HasPendingDungeonRoomEncounter() {
		if err := g.gameState.ResolvePendingDungeonRoomEncounter(); err != nil {
			logging.Errorf(logging.CategoryGameState, "Unable to enter fixed dungeon room: %v", err)
			g.gameState.SystemCallbacks.Message.AddRowStr("Room combat could not begin.")
		}

		return nil
	}

	if g.gameState.CombatScene != nil && !g.gameState.CombatScene.CombatTurnsStarted() {
		g.gameState.BeginCombatTurns(game_state.CombatTurnsPresented)
		g.reviewScheduledCombatStep()

		return nil
	}

	if g.gameState.CombatScene != nil && g.reviewScheduledCombatStep() {
		return nil
	}

	// Process NPC auto-conversation before player input (original Ultima V behavior)
	// NPC contact is processed before player commands (ref:NPC-0005).
	if g.processNPCAutoTalk() {
		return nil
	}
	// A waterfall may have yielded its physical input owner to the initiating
	// outdoor actor phase. Reacquire only after contact/scheduler/auto-talk work,
	// before either secondary input or an ordinary movement/Pass can run.
	if g.gameState.ResumeWaterfallContinuation() {
		return nil
	}

	// Note: TRAP fall sequences are now handled by event queue system
	// (updated automatically via EventQueueManager.Update() in GameState)

	// Don't process input during auto-open sequence
	if g.autoOpenDoorState.IsActive() {
		return nil
	}
	if !inputAllowed {
		return nil
	}

	mapType := g.gameState.MapState.PlayerLocation.Location.GetMapType()
	if g.atlasChordHeld || g.atlasMarkChordHeld || g.quickLoadChordHeld {
		// Unavailable exact chords never become secondary input, Mix, New Order
		// or smashed movement. Higher-priority owners have already updated.
		return nil
	}
	g.acceptPendingCombatCommand()

	if g.secondaryKeyState != PrimaryInput {
		if g.secondaryKeyState == SurfaceQuitInput {
			g.surfaceQuitSecondaryInput(game)

			return nil
		}
		switch mapType {
		case references.LargeMapType:
			g.largeMapHandleSecondaryInput()
		case references.SmallMapType:
			g.smallMapHandleSecondaryInput()
		case references.CombatMapType:
			g.combatMapHandleSecondaryInput()
		case references.DungeonMapType:
			g.dungeonMapHandleSecondaryInput(game)
		case references.CampingMapType:
			// Camping owns its workflow; it has no secondary map command routing.
		}

		return nil
	}

	// Handle gameplay logic here
	boundKey, passOwned := g.explorationPassKey()
	if !passOwned {
		boundKey = g.keyboard.GetBoundKeyPressed(&boundKeysGame)
	}
	if boundKey == nil {
		// allow quick tapping
		g.keyboard.SetAllowKeyPressImmediately()

		return nil
	}

	// Arrow keys need repeatable input for continuous movement
	// Other keys use single-press debouncing to prevent dialog key bleeding
	isArrowKey := *boundKey == ebiten.KeyUp || *boundKey == ebiten.KeyDown ||
		*boundKey == ebiten.KeyLeft || *boundKey == ebiten.KeyRight ||
		(!g.gameState.CanPlayHarpsichord() && primaryMovementKey(*boundKey) != *boundKey)

	// The scene-owned Pass cadence already accepted its sample.
	switch {
	case !passOwned && isArrowKey:
		if !g.keyboard.TryToRegisterRepeatableKeyPress(*boundKey) {
			return nil
		}
	case !passOwned:
		if !g.keyboard.TryToRegisterKeyPress(*boundKey) {
			return nil
		}
	}

	acceptedKey := *boundKey
	// Menus and readers below must not open over a ship that keeps sailing.
	if g.interruptAutomaticSailingForCommand(acceptedKey) {
		return nil
	}
	if g.handleBattleSummaryShortcut(acceptedKey) {
		return nil
	}
	if g.handleCommandHelpShortcut(acceptedKey) {
		return nil
	}
	g.clearSurfaceCameraForInput(acceptedKey)
	if g.handleQuestJournalShortcut(
		acceptedKey,
		g.keyboard.IsModifierPressed(input.ModifierShift),
	) {
		return nil
	}
	if mapType == references.SmallMapType {
		interception, err := g.gameState.InterceptSmashedCommand()
		if err != nil {
			return fmt.Errorf("intercepting intoxicated small-map command: %w", err)
		}
		if interception.Intercepted {
			acceptedKey = smallMapMovementKey(interception.Direction)
			if interception.SmashedBefore > 0 && interception.SmashedAfter == 0 {
				// REDUX DEVIATION: MERCH-0013: original town input prints only "Hic!" when smashed
				// reaches zero. Redux adds a timed blue notice when the effect ends.
				g.showTavernSoberedOverlay()
			}
		}
	}

	// Handle number keys for active player selection (context-aware)
	if g.handleNumberKey(acceptedKey) {
		return nil
	}
	acceptedKey = primaryMovementKey(acceptedKey)

	escapeMenuMap := mapType == references.LargeMapType ||
		mapType == references.SmallMapType ||
		mapType == references.DungeonMapType
	if acceptedKey == ebiten.KeyEscape && escapeMenuMap {
		g.DoEscapeMenu(game)

		return nil
	}

	switch mapType {
	case references.LargeMapType:
		g.largeMapInputHandlerWithGame(game, acceptedKey)
	case references.SmallMapType:
		g.smallMapInputHandlerWithGame(game, acceptedKey)
	case references.CombatMapType:
		g.combatMapInputHandler(acceptedKey)
	case references.DungeonMapType:
		g.dungeonMapInputHandlerWithGame(game, acceptedKey)
	case references.CampingMapType:
	default:
	}
	g.appendOrdinaryDirectionHint(mapType)

	return nil
}

func primaryMovementKey(key ebiten.Key) ebiten.Key {
	//nolint:exhaustive // Only cardinal keypad keys normalize to movement.
	switch key {
	case ebiten.KeyNumpad2:
		return ebiten.KeyDown
	case ebiten.KeyNumpad4:
		return ebiten.KeyLeft
	case ebiten.KeyNumpad6:
		return ebiten.KeyRight
	case ebiten.KeyNumpad8:
		return ebiten.KeyUp
	default:
		return key
	}
}

func (g *GameScene) updateDialogs() bool {
	// Phase 1D: MessageDialog now updates through DialogStack for proper modal isolation
	// (removed standalone g.messageDialog.Update() call)

	modalDialog := g.dialogStack.PeekTopModalDialog()
	if modalDialog == nil {
		if g.dialogStack.HasModalDialog() {
			panic("GameScene.updateDialogs: modal state has no modal dialog")
		}

		return false
	}

	// Only the highest modal receives input. Non-blocking dialogs animate elsewhere.
	modalDialog.Update()
	if dossier, ok := modalDialog.(*widgets.ZStatsDossierDialog); ok && dossier.HasPendingMixRequest() {
		g.startZStatsMix(dossier)
	}

	return true
}

// handleNumberKey handles number keys for active player selection or other contexts
// Returns true if the key was handled, false if it should be processed by other handlers
func (g *GameScene) handleNumberKey(key ebiten.Key) bool {
	digit, isNumberKey := gameplayNumberKeyDigit(key)
	if !isNumberKey {
		return false
	}

	if g.gameState.CanPlayHarpsichord() {
		return g.gameState.ActionPlayHarpsichordNote(digit)
	}
	if key < ebiten.Key0 || key > ebiten.Key9 {
		return false
	}

	// Default context: active player selection
	if g.handleActivePlayerSelection(key) {
		return true
	}

	// Digits without a current gameplay meaning are consumed rather than
	// leaking into map handlers as unknown commands.
	return true
}

func gameplayNumberKeyDigit(key ebiten.Key) (int, bool) {
	switch {
	case key >= ebiten.Key0 && key <= ebiten.Key9:
		return int(key - ebiten.Key0), true
	case key >= ebiten.KeyNumpad0 && key <= ebiten.KeyNumpad9:
		return int(key - ebiten.KeyNumpad0), true
	default:
		return 0, false
	}
}

// handleActivePlayerSelection handles number keys for active player selection.
// In combat, number keys also control solo move mode (ref:COMBAT-0003).
// Selecting the current PC does not consume a turn. Selecting another PC passes
// the current actor and advances through the DEX queue until that PC can act.
func (g *GameScene) handleActivePlayerSelection(key ebiten.Key) bool {
	// Handle Key0 specially (clear active player / all-party mode)
	if key == ebiten.Key0 {
		if g.gameState.IsInCombat() && g.gameState.CombatScene != nil {
			g.gameState.SelectCombatPlayer(-1)
		} else {
			g.gameState.ClearActivePlayer()
			g.gameState.SystemCallbacks.Message.AddRowStr("All party members active")
		}

		return true
	}

	// For keys 1-6, use math to convert to character index (0-5)
	if key >= ebiten.Key1 && key <= ebiten.Key6 {
		keyNumber := int(key - ebiten.Key1 + 1) // Convert to 1-6
		characterIndex := keyNumber - 1         // Convert to 0-5 for array indexing

		switch {
		case g.gameState.IsInCombat() && g.gameState.CombatScene != nil:
			if !g.gameState.SelectCombatPlayer(characterIndex) {
				g.gameState.SystemCallbacks.Message.AddRowStr(fmt.Sprintf("Character %d not available", keyNumber))
			}
		case g.gameState.SetActivePlayer(characterIndex):
			g.gameState.SystemCallbacks.Message.AddRowStr(fmt.Sprintf("Character %d active", keyNumber))
		default:
			g.gameState.SystemCallbacks.Message.AddRowStr(fmt.Sprintf("Character %d not available", keyNumber))
		}

		return true
	}

	return false // Not a number key we handle
}

// textEntry is a modal that turns letter keys into typed text.
type textEntry interface {
	AcceptsText() bool
}

// textEntryFocused lets Shift+J type a capital J instead of opening the Journal.
func (g *GameScene) textEntryFocused() bool {
	entry, ok := g.dialogStack.PeekTopModalDialog().(textEntry)

	return ok && entry.AcceptsText()
}
