package main

import (
	"fmt"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

// Dialog positioning constants
const defaultMessageVertical = 0.7

// Earthquake effect constants for shrine and dungeon sequences
const (
	// Dialog fade timing (milliseconds)
	earthquakeFadeDurationMs = 500
	earthquakeFadeInMs       = 200
	earthquakeFadeOutMs      = 500

	// Shake parameters (duration in ms and pixel amplitude)
	earthquakeShakeDurationMs = 2000
	earthquakeShakePixels     = 6.0

	// Message timing for dungeon earthquake sequence
	earthquakeMessageDelayMs     = 800  // Delay before showing "entrance opens/closes"
	earthquakePostMessageDelayMs = 1700 // Delay after message shown before fade out
)

// Shrine quest completion effect constants
const (
	shrineFlashDurationMs      = 600 // White flash animation duration
	shrineFlashMaxIntensity    = 0.8 // Peak flash brightness (0-1)
	shrineQuestShakeDurationMs = 1500
	shrineQuestShakePixels     = 5.0
	shrineQuestFadeOutMs       = 400
)

// showOverlayAndWait creates the common pattern: show overlay, poll for dismiss, reset keyboard.
// This helper reduces boilerplate in confirmation dialog workflows.
//
// The overlay must be pre-configured with:
//   - User dismissal mode (WithUserDismissed)
//   - AutoPopFromStack if desired
//
// Returns a modifier function that can be used with ThenSequence for clean syntax.
//
// Usage:
//
//	overlay := widgets.NewOverlayDialog("Message").
//	    AsConfirmation().
//	    WithUserDismissed(g.keyboard).
//	    AutoPopFromStack(&g.dialogStack).
//	    Build()
//	q.ThenSequence(g.showOverlayAndWait(overlay))
func (g *GameScene) showOverlayAndWait(
	overlay *widgets.OverlayDialog,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		return q.Then(func() {
			overlay.ShowWithoutAnimation()
			g.dialogStack.PushModalDialog(overlay)
		}).
			While(
				func() bool { return !overlay.IsDismissed() },
				func() {},
				DialogPollingRateMs,
			).
			Then(func() {
				g.keyboard.ResetReleaseDebouncing()
			})
	}
}

// thenShowYesNoDialog is a GameScene helper that creates a Yes/No dialog with branching
// event chains. The onYes and onNo callbacks receive sub-builders for their respective branches.
//
// This encapsulates the common pattern:
//   - Create YesNo dialog with callbacks
//   - Poll until user responds
//   - Apply keyboard debouncing
//   - Branch to yes or no event chains
//
// Returns a modifier function that can be used with ThenSequence for clean syntax.
//
// Usage with ThenSequence:
//
//	g.gameState.EventQueueManager.Workflow("name").
//	    Sequential().
//	    ThenSequence(g.thenShowYesNoDialog(
//	        "Drop a coin?",
//	        func(yesQ *event_queue.SequentialEventBuilder) {
//	            yesQ.Then(func() { /* yes logic */ })
//	        },
//	        func(noQ *event_queue.SequentialEventBuilder) {
//	            // no logic - can be empty
//	        },
//	    )).
//	    Start()
func (g *GameScene) thenShowYesNoDialog(
	prompt string,
	onYes func(*event_queue.SequentialEventBuilder),
	onNo func(*event_queue.SequentialEventBuilder),
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		var userChoseYes bool
		var choiceMade bool

		return q.Then(func() {
			yesNoCmd := widgets.NewYesNoCommand(
				func() {
					userChoseYes = true
					choiceMade = true
				},
				func() {
					userChoseYes = false
					choiceMade = true
				},
				&g.dialogStack,
			)
			yesNoDialog := widgets.NewYesNoInputBox(prompt, yesNoCmd, g.keyboard, &g.dialogStack)
			g.dialogStack.PushModalDialog(yesNoDialog)
		}).
			While(
				func() bool { return !choiceMade },
				func() {},
				DialogPollingRateMs,
			).
			Then(func() {
				g.keyboard.SetForceWaitAnyKey(KeyboardDebounceMs)
			}).
			Wait(KeyboardDebounceMs).If(

			func() bool { return userChoseYes },
			onYes).If(

			func() bool { return !userChoseYes },
			onNo)
	}
}

// thenShowConfirmationDialog is a GameScene helper that returns a modifier function
// for adding a confirmation OverlayDialog to an event queue with proper dismissal
// handling and keyboard debouncing.
//
// This encapsulates the common pattern:
//   - Then() to create and show dialog (auto-converts to First() if queue empty)
//   - While() to wait for IsDismissed()
//   - Then() to reset keyboard state
//
// Uses the OverlayDialog builder with AsConfirmation() template for consistent styling:
//   - Compact sizing (2 lines, 35 chars)
//   - Map-center-lower positioning
//   - No fade effects
//   - Centered text alignment
//
// Returns a modifier function that can be used with ThenSequence for clean syntax.
//
// Usage with ThenSequence (recommended):
//
//	g.gameState.EventQueueManager.Workflow("name").
//	    Sequential().
//	    ThenSequence(g.thenShowConfirmationDialog("Your message here")).
//	    Then(func() { /* next operation */ }).
//	    Start()
//
// Or with ThenSequence for runtime variable access:
//
//	ThenSequence(func(q *event_queue.SequentialEventBuilder) {
//	    g.thenShowConfirmationDialog("message")(q)
//	})
func (g *GameScene) thenShowConfirmationDialog(message string) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		overlay := widgets.NewOverlayDialog(message).
			AsConfirmation().
			WithUserDismissed(g.keyboard).
			AutoPopFromStack(&g.dialogStack).
			Build()

		return q.ThenSequence(g.showOverlayAndWait(overlay))
	}
}

// thenShowArrestDialog is a GameScene helper that creates the complete arrest dialog
// sequence with Yes/No choice and inlined jail sequence.
//
// This encapsulates the full arrest flow:
//   - Show "Thou art under arrest! Wilt thou come quietly?" dialog
//   - YES branch: Execute complete jail sequence (unconscious → fade → teleport → cleanup)
//   - NO branch: Queue source-backed Guard combat
//
// The jail sequence is inlined to avoid nested event queue conflicts. This helper can be
// used from any context (guard extortion, criminal talks to guard, etc.) without creating
// competing event queues.
//
// Returns a modifier function that can be used with ThenSequence for clean syntax.
//
// Usage with ThenSequence:
//
//	g.gameState.EventQueueManager.Workflow("name").
//	    Sequential().
//	    ThenSequence(g.thenShowArrestDialog()).
//	    Start()
//
// Or within an existing event chain:
//
//	noQ.ThenSequence(g.thenShowArrestDialog())
func (g *GameScene) thenShowArrestDialog(
	source *map_units.NPCFriendly,
	workflowState *merchantTalkWorkflowState,
	dialog *widgets.InteractionTranscript,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		return q.Then(func() {
			// ref:CRIME-0003: original manual Talk completes its minute before arrest asks
			// whether the party will surrender. NPC-initiated Talk already owns
			// the preceding command turn and must not charge a second minute.
			if workflowState != nil && workflowState.advanceTime {
				g.gameState.AdvanceGameplayMinutes(1)
				workflowState.advanceTime = false
			}
			dialog.AppendLine("Thou art under arrest!")
		}).ThenSequence(g.thenPromptInteractionYesNo(
			dialog,
			"Wilt thou come quietly?",
			func(yesQ *event_queue.SequentialEventBuilder) {
				yesQ.Then(func() {
					dialog.AppendLine("The guard strikes thee unconscious!")
					g.gameState.SystemCallbacks.Message.AddRowStr("The guard strikes thee unconscious!")
				}).
					Wait(ArrestSequenceUnconsciousDelayMs).
					Animate(ArrestSequenceFadeDurationMs, func(progress float32) {
						g.gameState.SystemCallbacks.Visual.SetMapFadeLevel(float64(progress))
					}).
					Then(func() {
						g.gameState.ExecuteJailSentence()

						g.gameState.SystemCallbacks.Message.AddRowStr("")
						g.gameState.SystemCallbacks.Message.AddRowStr("Thou dost awaken to...")
						dialog.AppendLine("Thou dost awaken to...")
					}).
					Wait(ArrestSequenceBlackScreenPauseMs).
					Animate(ArrestSequenceFadeDurationMs, func(progress float32) {
						g.gameState.SystemCallbacks.Visual.SetMapFadeLevel(float64(1.0 - progress))
					}).ThenSequence(g.thenWaitForInteractionAcknowledgement(dialog))
			},
			func(noQ *event_queue.SequentialEventBuilder) {
				noQ.Then(func() {
					g.gameState.RefuseArrest(source, false)
				})
			},
		))
	}
}

// thenShowInputBox is a GameScene helper that creates a text input dialog with event queue integration.
//
// This encapsulates the input dialog pattern:
//   - Create InputBox with validation
//   - Poll until user responds
//   - Apply keyboard debouncing
//   - Execute callback with captured text
//
// Returns a modifier function that can be used with ThenSequence for clean syntax.
//
// Parameters:
//   - prompt: Question to display
//   - validation: Regex pattern (e.g., "^[a-zA-Z]{0,10}$")
//   - onResult: Callback receiving (text string, submitted bool)
//
// Usage with ThenSequence:
//
//	g.gameState.EventQueueManager.Workflow("input").
//	    Sequential().
//	    ThenSequence(g.thenShowInputBox(
//	        "Enter name:",
//	        "^[a-zA-Z]{1,10}$",
//	        func(text string, submitted bool) {
//	            if submitted { processName(text) }
//	        },
//	    )).
//	    Start()
func (g *GameScene) thenShowInputBox(
	prompt string,
	validation string,
	onResult func(text string, submitted bool),
	instance ...**widgets.InputBox,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return g.thenShowInputBoxConfigured(prompt, validation, onResult, false, instance...)
}

func (g *GameScene) thenShowInputBoxWithEscapeClear(
	prompt string,
	validation string,
	onResult func(text string, submitted bool),
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return g.thenShowInputBoxConfigured(prompt, validation, onResult, true)
}

func (g *GameScene) thenShowInputBoxConfigured(
	prompt string,
	validation string,
	onResult func(text string, submitted bool),
	escapeClearsText bool,
	instance ...**widgets.InputBox,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		var inputComplete bool
		var inputDialog *widgets.InputBox

		return q.Then(func() {
			logging.Debugf(logging.CategoryUI, "thenShowInputBox: Creating InputBox with prompt=%q", prompt)
			// Create InputBox with result callback that sets flags
			builder := widgets.NewInputBox(prompt, nil, g.keyboard).
				WithRegexValidation(validation).
				WithResultCallback(func(text string, wasSubmitted bool) {
					logging.Debugf(logging.CategoryUI, "thenShowInputBox: resultCallback invoked, text=%q, submitted=%v, inputComplete was %v",
						text, wasSubmitted, inputComplete)
					inputComplete = true
					logging.Debugf(logging.CategoryUI, "thenShowInputBox: inputComplete set to true, calling onResult")
					onResult(text, wasSubmitted)
					logging.Debugf(logging.CategoryUI, "thenShowInputBox: onResult completed")
				}).
				WithAutoReplace(false).
				WithTextLines(2).
				WithCharsPerLine(40).
				WithHorizontalPosition(widgets.DialogHorizontalMapOnly).
				WithVerticalPosition(defaultMessageVertical) // Bottom third positioning (matches yell/sleep dialogs)
			if escapeClearsText {
				builder.WithEscapeClearsText()
			}
			inputDialog = builder.Build()
			if len(instance) > 0 {
				*instance[0] = inputDialog
			}

			g.dialogStack.PushModalDialog(inputDialog)
			logging.Debugf(logging.CategoryUI, "thenShowInputBox: InputBox pushed to dialog stack")
		}).
			While(
				func() bool {
					result := !inputComplete
					logging.Debugf(logging.CategoryUI, "thenShowInputBox: While condition check, inputComplete=%v, continuing=%v",
						inputComplete, result)

					return result
				},
				func() {},
				DialogPollingRateMs,
			).
			Then(func() {
				logging.Debugf(logging.CategoryUI, "thenShowInputBox: While loop exited, removing dialog")
				// Remove specific dialog instance from stack
				// Using RemoveWidgetInstance instead of PopModalDialog to avoid race condition
				// where other dialogs might be pushed on top before we pop
				g.dialogStack.RemoveWidgetInstance(inputDialog)
				if len(instance) > 0 {
					*instance[0] = nil
				}
				g.keyboard.SetForceWaitAnyKey(KeyboardDebounceMs)
				logging.Debugf(logging.CategoryUI, "thenShowInputBox: Dialog removed, keyboard debounce set")
			}).
			Wait(KeyboardDebounceMs)
	}
}

// thenShowSleepDurationInput is a GameScene helper that creates a sleep duration input dialog with event queue integration.
//
// This encapsulates the sleep duration input pattern:
//   - Create InputBox for hour entry (1-9 only)
//   - Single digit entry with auto-replace
//   - Poll until user responds
//   - Apply keyboard debouncing
//   - Execute callback with parsed integer hours (0 if cancelled)
//
// Returns a modifier function that can be used with ThenSequence for clean syntax.
//
// Parameters:
//   - onResult: Callback receiving (hours int) - receives 0 if cancelled or invalid
//
// Usage with ThenSequence:
//
//	g.gameState.EventQueueManager.Workflow("sleep").
//	    Sequential().
//	    ThenSequence(g.thenShowSleepDurationInput(func(hours int) {
//	        if hours > 0 { processSleep(hours) }
//	    })).
//	    Start()
func (g *GameScene) thenShowSleepDurationInput(
	onResult func(hours int),
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return g.thenShowCampDurationInput(onResult, false)
}

func (g *GameScene) thenShowDungeonCampDurationInput(
	onResult func(hours int),
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return g.thenShowCampDurationInput(onResult, true)
}

func (g *GameScene) thenShowCampDurationInput(
	onResult func(hours int),
	sourceCancellation bool,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		var inputComplete bool
		var inputDialog *widgets.InputBox

		return q.Then(func() {
			// Create InputBox with sleep-specific configuration
			pattern := "^[1-9]$"
			if sourceCancellation {
				pattern = "^[0-9]$"
			}
			builder := widgets.NewInputBox("How many hours?", nil, g.keyboard).
				WithRegexValidation(pattern).
				WithResultCallback(func(text string, wasSubmitted bool) {
					inputComplete = true

					// Parse hours from text (0 if cancelled or invalid)
					if !wasSubmitted || text == "" {
						onResult(0)

						return
					}

					// Convert string to int (already validated by regex)
					hours := 0
					if len(text) > 0 {
						hours = int(text[0] - '0') // Fast conversion for single digit
					}
					onResult(hours)
				}).WithAutoReplace(true) // Auto-replace for single digit entry
			if sourceCancellation {
				builder.WithSpaceCancellation()
			}
			inputDialog = builder.
				WithTextLines(2).
				WithCharsPerLine(20). // Smaller than default for compact sleep dialog
				WithHorizontalPosition(widgets.DialogHorizontalMapOnly).
				WithVerticalPosition(defaultMessageVertical). // Bottom third positioning
				Build()

			g.dialogStack.PushModalDialog(inputDialog)
		}).
			While(
				func() bool { return !inputComplete },
				func() {},
				DialogPollingRateMs,
			).
			Then(func() {
				// Remove specific dialog instance from stack
				// Using RemoveWidgetInstance instead of PopModalDialog to avoid race condition
				// where other dialogs might be pushed on top before we pop
				g.dialogStack.RemoveWidgetInstance(inputDialog)
				g.keyboard.SetForceWaitAnyKey(KeyboardDebounceMs)
			}).
			Wait(KeyboardDebounceMs)
	}
}

// thenShowGuardSelection is a GameScene helper that creates a guard selection dialog with event queue integration.
//
// This encapsulates the guard selection pattern:
//   - Create CharacterSelectionDialog with validation
//   - Poll until user responds
//   - Apply keyboard debouncing
//   - Execute callback with captured guard index (-1 = explicit no guard) and cancellation
//
// Returns a modifier function that can be used with ThenSequence for clean syntax.
//
// Parameters:
//   - onResult: Receives an explicit cancellation separately from a No One choice.
//
// Usage with ThenSequence:
//
//	g.gameState.EventQueueManager.Workflow("camping").
//	    Sequential().
//	    ThenSequence(g.thenShowGuardSelection(func(guardIndex int, cancelled bool) {
//	        if !cancelled { processGuard(guardIndex) }
//	    })).
//	    Start()
func (g *GameScene) thenShowGuardSelection(
	onResult func(guardIndex int, cancelled bool),
	dialog **widgets.CharacterSelectionDialog,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		var selectionComplete bool

		return q.Then(func() {
			// Create CharacterSelectionDialog with result callbacks
			options := g.characterSelectionOptions(characterSelectionContext{Profile: characterSelectionGuard})
			options = append(options, characterSelectionNoOneOption())
			*dialog = widgets.NewCharacterSelectionDialog(widgets.CharacterSelectionConfig{
				Title:          "Guard: Choose Character",
				GuardResources: true,
				Options:        options,
				CancelLabel:    "Cancel camping",
				OnSelect: func(characterIndex int) {
					// Character selected as guard
					selectionComplete = true
					if characterIndex == game_state.NoGuardAssigned {
						onResult(game_state.NoGuardAssigned, false)

						return
					}
					// A non-Good selection camps without a watch, as in the original.
					// ref:CAMP-0001
					if !g.gameState.PartyState.GetCharacter(characterIndex).IsGood() {
						onResult(game_state.NoGuardAssigned, false)

						return
					}
					onResult(characterIndex, false)
				},
				OnCancel: func() {
					// REDUX DEVIATION: CAMP-0001. The original treats a cancelled guard
					// prompt as No One and camps anyway; Redux Escape backs out of setup.
					selectionComplete = true
					onResult(game_state.NoGuardAssigned, true)
				},
				Keyboard:    g.keyboard,
				SpriteSheet: g.spriteSheet,
				Placement:   gameScreenPercents,
			})

			if *dialog == nil {
				// Never commit camping when a selection could not be presented.
				selectionComplete = true
				onResult(game_state.NoGuardAssigned, true)

				return
			}

			g.dialogStack.PushModalDialog(*dialog)
		}).
			While(
				func() bool { return !selectionComplete },
				func() {},
				DialogPollingRateMs,
			).
			Then(func() {
				// Remove specific dialog instance from stack
				// Using RemoveWidgetInstance instead of PopModalDialog to avoid race condition
				// where other dialogs might be pushed on top before we pop
				if *dialog != nil {
					g.dialogStack.RemoveWidgetInstance(*dialog)
				}
				g.keyboard.SetForceWaitAnyKey(KeyboardDebounceMs)
			}).
			Wait(KeyboardDebounceMs)
	}
}

// thenShowDungeonEarthquake is a GameScene helper that creates a dungeon entrance earthquake sequence.
//
// This encapsulates the complete earthquake flow:
//   - Shows MessageDialog with "The earth trembles!" before effects start
//   - Sound and screen shake effects
//   - Appends "The sealed entrance opens/closes!" to dialog during shake
//   - Fades out sound and dialog after earthquake completes
//
// The sequence is inlined to avoid nested event queue conflicts. This helper can be
// used from any context (yell workflow, debug commands, etc.) without creating
// competing event queues.
//
// Returns a modifier function that can be used with ThenSequence for clean syntax.
//
// Parameters:
//   - isOpening: true if entrance is opening, false if closing
//
// Usage with ThenSequence:
//
//	g.gameState.EventQueueManager.Workflow("yell").
//	    Sequential().
//	    ThenSequence(g.thenShowDungeonEarthquake(true)).
//	    Start()
func (g *GameScene) thenShowDungeonEarthquake() func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		var dialog *widgets.MessageDialog

		return q.Then(func() {
			dialog = widgets.NewMessageDialog("\nThe earth trembles!").
				WithFadeDurations(earthquakeFadeDurationMs, earthquakeFadeDurationMs).
				WithTextLines(4).
				WithEffects(true).
				Build()
			dialog.Show()
			g.dialogStack.PushModalDialog(dialog)

			g.gameState.SystemCallbacks.Audio.Play(game_state.SoundEarthquake).FadeIn(earthquakeFadeInMs).Start()
		}).ThenSequence(

			game_state.NewPartyStateBuilder(g.gameState).
				ToAllParty().
				WithShake(earthquakeShakeDurationMs, earthquakeShakePixels).
				Build()).
			Wait(earthquakeMessageDelayMs).
			Then(func() {
				// Stage 2: Add entrance state change message to dialog
				// Get dungeon toggle state from GameState (stored by ProcessWordOfPower)
				if g.gameState.WasLastDungeonToggleOpening() {
					dialog.AppendLine("The sealed entrance opens!")
				} else {
					dialog.AppendLine("The entrance seals shut!")
				}
			}).
			Wait(earthquakePostMessageDelayMs).
			Then(func() {
				// Stage 3: Fade out sound and begin dialog hide animation
				g.gameState.SystemCallbacks.Audio.Stop(game_state.SoundEarthquake).FadeOut(earthquakeFadeOutMs).Now()
				dialog.Hide() // Starts fade-out animation
			}).
			Wait(earthquakeFadeDurationMs). // Wait for dialog fade-out to complete
			Then(func() {
				// Stage 4: Remove dialog from stack after fade completes
				// Using RemoveWidgetInstance instead of PopModalDialog to avoid race condition
				// where other dialogs might be pushed on top before we pop
				g.dialogStack.RemoveWidgetInstance(dialog)
			})
	}
}

// thenApplyShadowlordDamage is a GameScene helper that applies party damage with visual effects
// using PartyStateBuilder for automatic flash cleanup.
//
// This fixes the "red screen stays forever" bug by using PartyStateBuilder which
// guarantees cleanup of visual effects after animation completes.
//
// The party-damage pattern follows ref:ENV-0001:
//   - 1d8 damage to all living party members
//   - Red flash effect (200ms, 0.7 intensity)
//   - White noise damage sound
//
// Returns a modifier function that can be used with ThenSequence for clean syntax.
//
// Usage with ThenSequence:
//
//	g.gameState.EventQueueManager.Workflow("talk").
//	    Sequential().
//	    ThenSequence(g.thenApplyShadowlordDamage()).
//	    Start()
func (g *GameScene) thenApplyShadowlordDamage() func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return game_state.NewPartyStateBuilder(g.gameState).
		WithRandomDamage(1, 8).
		ToAllParty().
		FlashRed(). // 200ms flash with automatic reset to 0.0
		WithSound(game_state.SoundWhiteNoiseDamage).
		Build()
}

// thenShowShrineRestorationEarthquake is a GameScene helper that creates an earthquake sequence
// for shrine restoration. Shows "A word of power is uttered!" followed by earthquake effects.
//
// Unlike thenShowDungeonEarthquake, this is used when restoring a ruined shrine via Word of Power.
// The earthquake is triggered FIRST when the word is uttered, before the virtue/mantra prompts.
//
// Returns a modifier function that can be used with ThenSequence for clean syntax.
//
// Usage with ThenSequence:
//
//	g.gameState.EventQueueManager.Workflow("shrine_restore").
//	    Sequential().
//	    ThenSequence(g.thenShowShrineRestorationEarthquake()).
//	    Start()
func (g *GameScene) thenShowShrineRestorationEarthquake() func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		var dialog *widgets.MessageDialog

		return q.Then(func() {
			dialog = widgets.NewMessageDialog("A word of power is uttered!").
				WithFadeDurations(earthquakeFadeDurationMs, earthquakeFadeDurationMs).
				WithTextLines(2).
				WithVerticalPosition(defaultMessageVertical).
				WithHorizontalPosition(widgets.DialogHorizontalMapOnly).
				WithContentTextAlignment(widgets.ContentAlignCenter).
				WithEffects(true).
				Build()
			dialog.Show()
			g.dialogStack.PushModalDialog(dialog)

			g.gameState.SystemCallbacks.Audio.Play(game_state.SoundEarthquake).FadeIn(earthquakeFadeInMs).Start()
		}).ThenSequence(

			game_state.NewPartyStateBuilder(g.gameState).
				ToAllParty().
				WithShake(earthquakeShakeDurationMs, earthquakeShakePixels).
				Build()).
			Wait(earthquakeShakeDurationMs).
			Then(func() {
				// Fade out sound and begin dialog hide animation
				g.gameState.SystemCallbacks.Audio.Stop(game_state.SoundEarthquake).FadeOut(earthquakeFadeOutMs).Now()
				dialog.Hide()
			}).
			Wait(earthquakeFadeDurationMs).
			Then(func() {
				// Remove dialog from stack after fade completes
				g.dialogStack.RemoveWidgetInstance(dialog)
			})
	}
}

// thenShowShrineRestoredEarthquake is a GameScene helper that creates the earthquake sequence
// shown after successful shrine restoration. Shows "The Shrine is restored!" with earthquake.
//
// This is the second earthquake - triggered after virtue and mantra validation succeeds.
//
// Returns a modifier function that can be used with ThenSequence for clean syntax.
func (g *GameScene) thenShowShrineRestoredEarthquake() func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		var dialog *widgets.MessageDialog

		return q.Then(func() {
			dialog = widgets.NewMessageDialog("\nThe Shrine is restored!").
				WithFadeDurations(earthquakeFadeDurationMs, earthquakeFadeDurationMs).
				WithTextLines(2).
				WithContentTextAlignment(widgets.ContentAlignCenter).
				WithEffects(true).
				Build()
			dialog.Show()
			g.dialogStack.PushModalDialog(dialog)

			g.gameState.SystemCallbacks.Audio.Play(game_state.SoundEarthquake).FadeIn(earthquakeFadeInMs).Start()
		}).ThenSequence(

			game_state.NewPartyStateBuilder(g.gameState).
				ToAllParty().
				WithShake(earthquakeShakeDurationMs, earthquakeShakePixels).
				Build()).
			Wait(earthquakeShakeDurationMs).
			Then(func() {
				// Fade out sound and begin dialog hide animation
				g.gameState.SystemCallbacks.Audio.Stop(game_state.SoundEarthquake).FadeOut(earthquakeFadeOutMs).Now()
				dialog.Hide()
			}).
			Wait(earthquakeFadeDurationMs).
			Then(func() {
				// Remove dialog from stack after fade completes
				g.dialogStack.RemoveWidgetInstance(dialog)
			})
	}
}

// thenShowShrineQuestCompletionEffects is a GameScene helper that creates the full shrine quest
// completion effect sequence for shrine rewards (ref:SHRINE-0002):
// 1. Show completion dialog with message
// 2. White screen flash
// 3. Play reward sweep sound
// 4. Earthquake shake effect
// 5. Wait for any key
// 6. Cleanup dialog
//
// Original reward presentation combines a white flash, repeated audio sweeps,
// and screen shaking before waiting for acknowledgement (ref:SHRINE-0002).
//
// Returns a modifier function that can be used with ThenSequence for clean syntax.
//
// Usage with ThenSequence:
//
//	g.gameState.EventQueueManager.Workflow("shrine_complete").
//	    Sequential().
//	    ThenSequence(g.thenShowShrineQuestCompletionEffects(completionMessage)).
//	    Start()
func (g *GameScene) thenShowShrineQuestCompletionEffects(message string) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		var dialog *widgets.MessageDialog

		return q.Then(func() {
			dialog = widgets.NewMessageDialog(message).
				WithFadeDurations(earthquakeFadeDurationMs, earthquakeFadeDurationMs).
				WithAutoSizeDefault().
				WithSeparatePromptArea().
				WithContentTextAlignment(widgets.ContentAlignCenter).
				WithEffects(true).
				Build()
			dialog.Show()
			g.dialogStack.PushModalDialog(dialog)

			g.gameState.SystemCallbacks.Visual.SetScreenFlashColor(1.0, 1.0, 1.0)
			g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundShrineReward)
		}).
			Animate(shrineFlashDurationMs, func(progress float32) {
				intensity := (1.0 - math.Abs(float64(progress)*2-1)) * shrineFlashMaxIntensity
				g.gameState.SystemCallbacks.Visual.SetScreenFlashIntensity(intensity)
			}).
			Then(func() {
				g.gameState.SystemCallbacks.Visual.SetScreenFlashIntensity(0)
				g.gameState.SystemCallbacks.Audio.Play(game_state.SoundEarthquake).FadeIn(earthquakeFadeInMs).Start()
			}).ThenSequence(

			game_state.NewPartyStateBuilder(g.gameState).
				ToAllParty().
				WithShake(shrineQuestShakeDurationMs, shrineQuestShakePixels).
				Build()).
			Wait(shrineQuestShakeDurationMs).
			Then(func() {
				// Fade out earthquake sound
				g.gameState.SystemCallbacks.Audio.Stop(game_state.SoundEarthquake).FadeOut(shrineQuestFadeOutMs).Now()
				dialog.SetAcknowledgementPrompt(widgets.DialogAnyKeyHint)
			}).
			WaitForAnyKey().
			Then(func() {
				dialog.ClearPrompt()
				dialog.Hide()
			}).
			Wait(earthquakeFadeDurationMs).
			Then(func() {
				g.dialogStack.RemoveWidgetInstance(dialog)
			})
	}
}

// thenShowLinearTalkDialog is a GameScene helper that creates a LinearTalkDialog for NPC conversation
// and polls until the dialog self-removes from the stack.
//
// LinearTalkDialog is fundamentally different from simple confirmation dialogs:
//   - Stateful: Maintains conversation engine, input/output state
//   - Long-running: Player asks multiple questions in one session
//   - Self-managing: Handles its own Update()/Draw() loop via Widget interface
//   - Self-removing: Removes itself from dialog stack when conversation ends (BYE)
//
// This helper integrates LinearTalkDialog into event queues using While() polling pattern.
//
// Returns a modifier function that can be used with ThenSequence for clean syntax.
//
// Usage with ThenSequence:
//
//	g.gameState.EventQueueManager.Workflow("talk").
//	    Sequential().
//	    ThenSequence(g.thenShowLinearTalkDialog(npcFriendly, &completed)).
//	    Start()
func (g *GameScene) thenShowLinearTalkDialog(
	npc *map_units.NPCFriendly,
	completed *bool,
	owned **LinearTalkDialog,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		var dialog *LinearTalkDialog
		var dialogCreated bool
		var fallbackMessage string

		return q.
			Then(func() {
				dialog = NewLinearTalkDialog(g, npc)
				*owned = dialog
				if dialog != nil {
					g.dialogStack.PushModalDialog(dialog)
					dialog.mount()
					dialogCreated = true
				} else {
					fallbackMessage = g.getSpecialNPCResponseMessage(npc)
					dialogCreated = false
				}
			}).
			While(
				func() bool {
					return dialogCreated && g.dialogStack.ContainsWidgetInstance(dialog)
				},
				func() {},
				DialogPollingRateMs,
			).Then(func() {
			*completed = dialog != nil && dialog.ConversationCompleted()
			if dialog != nil {
				dialog.discardPendingKnowledge()
			}
		}).If(

			func() bool { return !dialogCreated && fallbackMessage != "" },
			func(fallbackQ *event_queue.SequentialEventBuilder) {
				fallbackQ.ThenSequence(func(innerQ *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
					g.thenShowConfirmationDialog(fallbackMessage)(innerQ)

					return innerQ
				})
			})
	}
}

// getSpecialNPCResponseMessage returns the appropriate feedback message for NPCs
// that don't have full dialog systems (NoResponse, merchants,“ etc.)
func (g *GameScene) getSpecialNPCResponseMessage(npc *map_units.NPCFriendly) string {
	dialogNum := references.DialogNumber(npc.NPCReference.DialogNumber)

	switch {
	case dialogNum == references.NoResponse:
		return "No response!"
	case dialogNum.IsMerchant():
		merchantType := dialogNum.GetMerchantType()

		return "The " + strings.ToLower(merchantType) + "\nnods but says nothing."
	default:
		return "They don't seem\ninterested in talking."
	}
}

// thenHandleGuardTalk is a GameScene helper that handles guard NPC encounters.
//
// Routes to different flows based on criminal status:
//   - Criminal: Show arrest dialog (surrender → jail OR refuse → combat)
//   - Not criminal: Show guard extortion dialog (pay tribute OR refuse → arrest)
//
// Returns a modifier function that can be used with ThenSequence for clean syntax.
//
// Usage with ThenSequence:
//
//	g.gameState.EventQueueManager.Workflow("talk").
//	    Sequential().
//	    ThenSequence(g.thenHandleGuardTalk()).
//	    Start()
func (g *GameScene) thenHandleGuardTalk(
	source *map_units.NPCFriendly,
	workflowState *merchantTalkWorkflowState,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		if workflowState == nil {
			panic("thenHandleGuardTalk: workflow state is required")
		}
		criminalAtStart := g.gameState.IsCriminal()
		palaceGuard := g.gameState.MapState.PlayerLocation.Location == references.PalaceOfBlackthorn
		badgeActive := g.gameState.IsBadgeActive()
		passwordAccepted := false
		review := g.gameState.ReviewGuardTribute()
		tributeStatement := fmt.Sprintf("A guard demands a %d gp tribute to Blackthorn!", review.Amount)
		if review.Kind == game_state.GuardTributeMinocCharity {
			tributeStatement = "\"Thou wilt give half thy gold to charity!\""
		}
		dialog := widgets.NewInteractionTranscript(g.keyboard, widgets.InteractionTranscriptConfig{
			MaxLines:       6,
			MaxInputLength: 14,
			WidthSamples: []string{
				tributeStatement,
				"Give now the password, bearer of the Badge!",
				"Your response?  > IMPE",
				"\"Pass, friend!\"",
				"Dost thou pay?  > Yes",
				"Thou hast not enough gold. The tribute is unpaid.",
				"Thou art under arrest!",
				"Wilt thou come quietly?  > Yes",
				"The guard strikes thee unconscious!",
				"Thou dost awaken to...",
			},
		})
		workflowState.interactionTranscript = dialog

		return q.Then(func() {
			g.dialogStack.PushModalDialog(dialog)
		}).If(

			func() bool { return palaceGuard && !badgeActive },
			func(arrestQ *event_queue.SequentialEventBuilder) {
				arrestQ.ThenSequence(g.thenShowArrestDialog(source, workflowState, dialog))
			}).If(

			func() bool { return palaceGuard && badgeActive },
			func(passwordQ *event_queue.SequentialEventBuilder) {
				passwordQ.Then(func() {
					dialog.AppendLine("\"Give now the password, bearer of the Badge!\"")
				}).ThenSequence(g.thenPromptInteractionText(
					dialog,
					"Your response?",
					func(response string, submitted bool) {
						passwordAccepted = submitted && game_state.IsBlackthornPasswordAccepted(response)
					},
				)).If(
					func() bool { return passwordAccepted },
					func(passQ *event_queue.SequentialEventBuilder) {
						passQ.Then(func() {
							dialog.AppendLine("\"Pass, friend!\"")
						}).ThenSequence(g.thenWaitForInteractionAcknowledgement(dialog))
					},
				).If(
					func() bool { return !passwordAccepted },
					func(arrestQ *event_queue.SequentialEventBuilder) {
						arrestQ.ThenSequence(g.thenShowArrestDialog(source, workflowState, dialog))
					},
				)
			}).If(

			func() bool { return !palaceGuard && criminalAtStart },
			func(arrestQ *event_queue.SequentialEventBuilder) {
				arrestQ.ThenSequence(g.thenShowArrestDialog(source, workflowState, dialog))
			}).If(

			func() bool { return !palaceGuard && !criminalAtStart },
			func(extortQ *event_queue.SequentialEventBuilder) {
				var paymentResult game_state.GuardTributePaymentResult
				extortQ.Then(func() {
					dialog.AppendLine(tributeStatement)
				}).ThenSequence(g.thenPromptInteractionYesNo(
					dialog,
					"Dost thou pay?",
					func(yesQ *event_queue.SequentialEventBuilder) {
						yesQ.Then(func() {
							paymentResult = g.gameState.CommitGuardTribute(review)
						}).If(
							func() bool { return paymentResult == game_state.GuardTributeInsufficientGold },
							func(arrestQ *event_queue.SequentialEventBuilder) {
								// REDUX DEVIATION: CRIME-0001: original tribute refusal returns arrest without explanation.
								// Supplemental text explains the failed payment without changing arrest authority.
								arrestQ.Then(func() {
									dialog.AppendLine("Thou hast not enough gold. The tribute is unpaid.")
								}).ThenSequence(g.thenShowArrestDialog(source, workflowState, dialog))
							},
						)
					},
					func(noQ *event_queue.SequentialEventBuilder) {
						noQ.ThenSequence(g.thenShowArrestDialog(source, workflowState, dialog))
					},
				))
			})
	}
}

// thenShowNewOrderDialog is a GameScene helper that creates the New Order (N) dialog
// for party member position swapping.
//
// This dialog implements a two-stage selection workflow:
//   - Stage 1: Select first character to swap (Avatar grayed, navigation skips it)
//   - Stage 2: Select second character (selecting same character = unselect, goes back to Stage 1)
//
// After successful swap, shows confirmation dialog with character names.
//
// The dialog handles:
//   - Avatar protection (always grayed and unselectable)
//   - Dead/sleeping characters (grayed but selectable - position matters for resurrection)
//   - ESC in Stage 1 = cancel dialog
//   - ESC in Stage 2 = return to Stage 1
//   - Selecting locked character = unselect (return to Stage 1)
//
// Returns a modifier function that can be used with ThenSequence for clean syntax.
//
// Usage with ThenSequence:
//
//	g.gameState.EventQueueManager.Workflow("new_order").
//	    Sequential().
//	    First(func() { g.addRowStr("New Order...") }).
//	    ThenSequence(g.thenShowNewOrderDialog()).
//	    Start()
func (g *GameScene) thenShowNewOrderDialog(
	dialog **widgets.NewOrderDialog,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		var swapExecuted bool
		var firstName, secondName string

		return q.Then(func() {
			if g.reorderablePartyMemberCount() < 2 {
				swapExecuted = false

				return
			}

			*dialog = widgets.NewNewOrderDialog(
				&g.gameState.PartyState,
				g.gameState.GetActivePlayerIndex(),
				func(first, second int) {
					firstName = g.gameState.PartyState.GetCharacter(first).GetNameAsString()
					secondName = g.gameState.PartyState.GetCharacter(second).GetNameAsString()

					err := g.gameState.SwapPartyMembers(first, second)
					if err != nil {
						logging.Warnf(logging.CategoryUI, "SwapMembers failed: %v", err)
						swapExecuted = false

						return
					}
					swapExecuted = true
				},
				func() {
					swapExecuted = false
				},
				g.keyboard,
				g.spriteSheet,
				gameScreenPercents,
			)

			if *dialog == nil {
				swapExecuted = false

				return
			}

			g.dialogStack.PushModalDialog(*dialog)
		}).
			While(
				func() bool {
					return *dialog != nil && !(*dialog).IsComplete()
				},
				func() {},
				DialogPollingRateMs,
			).
			Then(func() {
				if *dialog != nil {
					g.dialogStack.RemoveWidgetInstance(*dialog)
				}
			}).If(

			func() bool { return swapExecuted },
			func(confirmQ *event_queue.SequentialEventBuilder) {
				confirmQ.Then(func() {
					message := fmt.Sprintf("%s and %s swapped!", firstName, secondName)

					g.gameState.SystemCallbacks.UI.ShowActionFeedback(message, true)
				})
			})
	}
}

func (g *GameScene) reorderablePartyMemberCount() int {
	count := 0
	for index := 1; index < party_state.MaxActiveParty; index++ {
		character := g.gameState.PartyState.GetCharacter(index)
		if character != nil && character.PartyMap == party_state.PartyMapActive && character.GetNameAsString() != "" {
			count++
		}
	}

	return count
}

func (g *GameScene) thenShowCastUseDirectionPrompt(
	message string,
	onDirection func(references.Direction),
	onCancel func(),
) event_queue.Sequence {
	return g.thenShowDirectionPromptWithCancellation(message, "Arrow/keypad; Esc/Space cancels", onDirection, onCancel, true)
}

func (g *GameScene) thenShowDirectionPromptWithCancellation(
	message, hintText string,
	onDirection func(references.Direction),
	onPass func(),
	allowEscape bool,
) event_queue.Sequence {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		var dialog *widgets.MessageDialog
		var inputComplete bool

		return q.Then(func() {
			// Combine message and hint
			fullMessage := message + "\n\n" + hintText

			// Reuse MessageDialog for visual display
			dialog = widgets.NewMessageDialog(fullMessage).
				WithTextLines(5).
				WithCharsPerLine(24).
				WithContentTextAlignment(widgets.ContentAlignCenter).
				WithTextHorizontalCenter(true).
				WithVerticalPosition(defaultMessageVertical).
				WithHorizontalPosition(widgets.DialogHorizontalMapOnly).
				Build()
			dialog.ShowWithoutAnimation()
			if allowEscape {
				g.castUseDirectionDialog = dialog
			}
			g.dialogStack.PushModalDialog(dialog)
		}).
			While(
				func() bool { return !inputComplete },
				func() {
					if allowEscape && g.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
						inputComplete = true
						if onPass != nil {
							onPass()
						}

						return
					}
					input := g.ordinaryDirectionInput()
					switch input.Outcome {
					case ordinaryDirectionActive:
						return
					case ordinaryDirectionPassed:
						inputComplete = true
						if onPass != nil {
							onPass()
						}
					case ordinaryDirectionSelected:
						inputComplete = true
						onDirection(input.Direction)
					}
				},
				DialogPollingRateMs,
			).
			Then(func() {
				g.dialogStack.RemoveWidgetInstance(dialog)
				if allowEscape && g.castUseDirectionDialog == dialog {
					g.castUseDirectionDialog = nil
				}
				g.keyboard.ResetReleaseDebouncing()
			})
	}
}

func (g *GameScene) clearCastUseDirectionPrompt() {
	if g.castUseDirectionDialog != nil {
		g.dialogStack.RemoveWidgetInstance(g.castUseDirectionDialog)
		g.castUseDirectionDialog = nil
	}
}
