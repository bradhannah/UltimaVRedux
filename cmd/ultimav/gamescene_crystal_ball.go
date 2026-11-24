package main

import (
	"math"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

// ShowCrystalBallInteraction handles the crystal ball viewing sequence using event queue pattern.
// Flow:
//  1. "Look-Thou dost see a crystal ball" message
//  2. Eligible active viewer, or character selection dialog ("Who will look?")
//  3. Intelligence check: roll 1-30, success if roll < character.Intelligence
//  4. Failure: "Death vision!" + 1 HP damage + red flash
//  5. Success: "Strange vision!" + show GemViewDialog
//
// Crystal-ball risk and observation follow ref:VIEW-0004.
// Intelligence check: a draw from 1..30 at or above the character's Intelligence means failure.
func (g *GameScene) ShowCrystalBallInteraction() {
	g.StartCrystalBallWorkflow()
}

// StartCrystalBallWorkflow initiates the crystal ball viewing sequence.
func (g *GameScene) StartCrystalBallWorkflow() {
	var characterSelectionDialog *widgets.CharacterSelectionDialog
	var gemViewDialog *widgets.GemViewDialog
	var transcript *widgets.InteractionTranscript
	var selectionMade bool
	selectedCharacterIndex := -1
	removeTranscript := func() {
		if transcript != nil {
			g.dialogStack.RemoveWidgetInstance(transcript)
			transcript = nil
		}
	}

	g.gameState.EventQueueManager.Workflow("crystal_ball_workflow").
		Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().
		First(func() {
			g.output.AddText("Look-Thou dost see a crystal ball")
			transcript = widgets.NewInteractionTranscript(g.keyboard, widgets.InteractionTranscriptConfig{
				MaxLines:       3,
				MaxInputLength: 1,
				WidthSamples: []string{
					"Thou dost see a crystal ball.",
					"MMMMMMMMMMMM looks into the crystal ball.",
					"Strange vision!",
					"Death vision!",
				},
			})
			transcript.AppendLine("Thou dost see a crystal ball.")
			g.dialogStack.PushModalDialog(transcript)
		}).
		Then(func() {
			activePlayer := g.gameState.GetActivePlayer()
			// REDUX DEVIATION: VIEW-0004: original actor selection trusts the active designation; use the
			// selector's Good/Poisoned policy here too so disabled actors cannot bypass it.
			if isCharacterActorEligible(activePlayer) {
				selectedCharacterIndex = g.gameState.GetActivePlayerIndex()
				transcript.AppendLine(activePlayer.GetNameAsString() + " looks into the crystal ball.")
				selectionMade = true

				return
			}
			if activePlayer != nil {
				g.gameState.ClearActivePlayer()
			}
			// REDUX DEVIATION: whowill auto-selects a sole eligible fallback; retain
			// explicit confirmation of crystal-ball risk when no active viewer exists.
			// See ref:VIEW-0004.
			characterSelectionDialog = widgets.NewCharacterSelectionDialog(widgets.CharacterSelectionConfig{
				Title:   "Crystal Ball: Choose Viewer",
				Options: g.characterSelectionOptions(characterSelectionContext{Profile: characterSelectionCrystalBall}),
				OnSelect: func(characterIndex int) {
					g.dialogStack.RemoveWidgetInstance(characterSelectionDialog)
					selectedCharacterIndex = characterIndex
					character := g.gameState.PartyState.GetCharacter(characterIndex)
					transcript.AppendLine(character.GetNameAsString() + " looks into the crystal ball.")
					selectionMade = true
				},
				OnCancel: func() {
					g.dialogStack.RemoveWidgetInstance(characterSelectionDialog)
					selectedCharacterIndex = -1
					selectionMade = true
				},
				Keyboard:    g.keyboard,
				SpriteSheet: g.spriteSheet,
				Placement:   gameScreenPercents,
			})

			g.dialogStack.PushModalDialog(characterSelectionDialog)
		}).
		While(
			func() bool { return !selectionMade },
			func() {},
			DialogPollingRateMs,
		).
		Then(func() {
			g.keyboard.SetForceWaitAnyKey(KeyboardDebounceMs)
		}).
		Wait(KeyboardDebounceShortMs).If(

		func() bool { return selectedCharacterIndex >= 0 },
		func(subQ *event_queue.SequentialEventBuilder) {
			subQ.ThenSequence(func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
				if g.gameState.CrystalBallLook(selectedCharacterIndex) {
					q.Then(func() {
						g.addRowStr("Strange vision!")
						transcript.AppendLine("Strange vision!")
						g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundStrangeVision)
					}).ThenSequence(g.thenWaitForInteractionContinuation(transcript)).
						Then(func() { removeTranscript() }).
						ThenSequence(g.thenShowGemViewDialog(&gemViewDialog))
				} else {
					q.Then(func() {
						g.addRowStr("Death vision!")
						transcript.AppendLine("Death vision!")
					}).ThenSequence(

						game_state.NewPartyStateBuilder(g.gameState).
							WithFixedDamage(game_state.CrystalBallDeathVisionDamage).
							ToCharacter(selectedCharacterIndex).
							FlashRed().
							WithSound(game_state.SoundDeathVision).
							Build()).
						ThenSequence(g.thenWaitForInteractionAcknowledgement(transcript))
				}

				return q
			})
		}).Finally(

		func() {
			if characterSelectionDialog != nil {
				g.dialogStack.RemoveWidgetInstance(characterSelectionDialog)
			}
			if gemViewDialog != nil {
				g.dialogStack.RemoveWidgetInstance(gemViewDialog)
			}
			removeTranscript()
			g.keyboard.ResetReleaseDebouncing()
			g.gameState.AdvanceGameplayMinutesAndProcessProvisions(1)
		}).
		Start()
}

// thenShowGemViewDialog returns an event queue modifier that shows the gem view dialog.
// Used by crystal ball success path and V command.
func (g *GameScene) thenShowGemViewDialog(
	dialog **widgets.GemViewDialog,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		return q.Then(func() {
			// Create gem view dialog centered on player position
			playerX := int(g.gameState.MapState.PlayerLocation.Position.X)
			playerY := int(g.gameState.MapState.PlayerLocation.Position.Y)

			wrapMap := g.gameState.MapState.PlayerLocation.Location.GetMapType() == references.LargeMapType

			*dialog = widgets.NewGemViewDialog(widgets.GemViewDialogConfig{
				Placement:    centeredGameDialogPercents,
				Keyboard:     g.keyboard,
				PlayerWorldX: playerX,
				PlayerWorldY: playerY,
				GetTileAt:    g.getSurfaceViewTileIndex,
				WrapMap:      wrapMap,
			})

			g.dialogStack.PushModalDialog(*dialog)
		}).
			While(
				func() bool { return !(*dialog).IsDismissed() },
				func() { (*dialog).ToggleFlash() }, // Toggle flash at player position
				GemViewFlashIntervalMs,             // ~250ms for authentic flash rate
			).
			Then(func() {
				g.dialogStack.RemoveWidgetInstance(*dialog)
				*dialog = nil
				g.keyboard.ResetReleaseDebouncing()
			})
	}
}

// ShowGemViewInteraction handles the V command gem view display.
// Called after gem is consumed by ActionView* methods.
// No character selection - shows overhead map view immediately (authentic Ultima V behavior).
func (g *GameScene) ShowGemViewInteraction(complete func()) {
	g.startGemViewWorkflowWithCompletion(complete)
}

func (g *GameScene) startGemViewWorkflowWithCompletion(complete func()) {
	var gemViewDialog *widgets.GemViewDialog

	g.gameState.EventQueueManager.Workflow("gem_view_workflow").
		Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().
		First(func() {

		}).ThenSequence(

		g.thenShowGemViewDialog(&gemViewDialog)).Finally(

		func() {
			if gemViewDialog != nil {
				g.dialogStack.RemoveWidgetInstance(gemViewDialog)
			}
			g.keyboard.ResetReleaseDebouncing()
			if complete != nil {
				complete()
			}
		}).
		Start()
}

// getSurfaceViewTileIndex returns the current map terrain at a surface-view position.
// Dynamic units are intentionally excluded, matching the original map-buffer lookup.
func (g *GameScene) getSurfaceViewTileIndex(worldX, worldY int) int {
	// Get the current layered map
	layeredMap := g.gameState.GetLayeredMapByCurrentLocation()
	if layeredMap == nil {
		return 0
	}

	// No map extends beyond the Coordinate range.
	if worldX < math.MinInt16 || worldX > math.MaxInt16 || worldY < math.MinInt16 || worldY > math.MaxInt16 {
		return 0
	}

	// Create position (handle wrapping for large map)
	pos := references.Position{X: references.Coordinate(worldX), Y: references.Coordinate(worldY)}

	// For large map, wrap coordinates
	if g.gameState.MapState.PlayerLocation.Location.GetMapType() == references.LargeMapType {
		// Wrap X: 0-255
		for pos.X < 0 {
			pos.X += 256
		}

		pos.X %= 256

		// Wrap Y: 0-255
		for pos.Y < 0 {
			pos.Y += 256
		}

		pos.Y %= 256
	}

	// Include authoritative map overrides such as opened doors, but not unit sprites.
	tile := layeredMap.GetTileTopMapOnlyTile(&pos)
	if tile == nil {
		return 0
	}

	return int(tile.Index)
}
