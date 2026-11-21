package main

import (
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

// Troll bridge encounter timing constants
const (
	// trollEncounterInitialDelayMs is the pause after "Thou spieth trolls" message
	trollEncounterInitialDelayMs = 1000

	// trollSneakDotDelayMs is the delay between each dot appearing in the sneak animation
	trollSneakDotDelayMs = 500

	// trollTranscriptLines bounds the visible encounter history while retaining recent stages.
	trollTranscriptLines = 6
)

// StartBridgeTrollEncounterWorkflow initiates the troll bridge encounter sequence.
// Uses event queue for multi-stage dialog flow with proper input blocking.
//
// Flow:
//  1. Show "Thou spieth trolls under the bridge!" transcript
//  2. For each living, awake party member:
//     - Show "{Name} sneaks across" with animated dots
//     - Perform sneak check (a draw from 1..30 above Dexterity means caught)
//  3. If all pass: Show "Trolls evaded!"
//  4. If caught: Show toll demand and payment prompt in the same transcript
//
// Bridge admission, evasion, and toll behavior follow ref:ENV-0004.
func (g *GameScene) StartBridgeTrollEncounterWorkflow() {
	var (
		dialog *widgets.InteractionTranscript
		caught bool
		toll   int
		paid   bool
	)
	livingMembers := g.gameState.GetLivingAwakePartyMembers()
	widthSamples := make([]string, 0, 5+len(livingMembers))
	widthSamples = append(widthSamples,
		"Thou spieth trolls under the bridge!",
		"The trolls demand a -666 gp toll!",
		"Dost thou pay?  > Yes",
		"Paid -666 gold.",
		"Thou hast not enough gold. The toll is unpaid.",
	)
	for _, member := range livingMembers {
		widthSamples = append(widthSamples, member.GetNameAsString()+" sneaks across...")
	}

	g.gameState.EventQueueManager.Workflow("bridge_troll_encounter").
		Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().
		First(func() {
			g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundTrollGrowl)
			dialog = widgets.NewInteractionTranscript(g.keyboard, widgets.InteractionTranscriptConfig{
				MaxLines:       trollTranscriptLines,
				MaxInputLength: 1,
				WidthSamples:   widthSamples,
			})
			dialog.AppendLine("Thou spieth trolls under the bridge!")
			g.dialogStack.PushModalDialog(dialog)
		}).
		Wait(trollEncounterInitialDelayMs).ThenSequence(
		func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
			for _, member := range livingMembers {
				memberCopy := member
				q.If(func() bool { return !caught }, func(memberQ *event_queue.SequentialEventBuilder) {
					memberQ.Then(func() {
						dialog.AppendLine(memberCopy.GetNameAsString() + " sneaks across")
						g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundGuardFootstepGrass)
					}).
						Wait(trollSneakDotDelayMs).Then(func() { dialog.AppendToLastLine(".") }).
						Wait(trollSneakDotDelayMs).Then(func() { dialog.AppendToLastLine(".") }).
						Wait(trollSneakDotDelayMs).Then(func() { dialog.AppendToLastLine(".") }).
						Then(func() {
							if g.gameState.PerformSneakCheck(memberCopy) {
								caught = true
								toll = g.gameState.CalculateTrollToll(memberCopy)
							}
						})
				})
			}

			return q
		}).
		If(
			func() bool { return !caught }, func(q *event_queue.SequentialEventBuilder) {
				q.Then(func() {
					dialog.AppendLine("Trolls evaded!")
				}).ThenSequence(
					// REDUX DEVIATION: ENV-0004: original evasion returns immediately because its text remains
					// in the scrolling output. Redux waits so the transient transcript stays readable.
					g.thenWaitForInteractionAcknowledgement(dialog),
				)
			}).If(
		func() bool { return caught }, func(q *event_queue.SequentialEventBuilder) {
			q.Then(func() {
				g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundTrollCaught)
				dialog.AppendLine("Caught!")
				dialog.AppendLine(fmt.Sprintf("The trolls demand a %d gp toll!", toll))
			}).ThenSequence(func(tollQ *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
				return g.thenPromptInteractionYesNo(
					dialog,
					"Dost thou pay?",
					func(yesQ *event_queue.SequentialEventBuilder) {
						yesQ.Then(func() {
							paid = g.gameState.PayTrollToll(toll)
						}).If(
							func() bool { return paid },
							func(paidQ *event_queue.SequentialEventBuilder) {
								paidQ.Then(func() {
									g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundCoinSplash)
									dialog.AppendLine(fmt.Sprintf("Paid %d gold.", toll))
								}).ThenSequence(
									// REDUX DEVIATION: ENV-0004: original toll payment returns without success text.
									// Redux confirms payment and keeps the transient transcript readable.
									g.thenWaitForInteractionAcknowledgement(dialog),
								)
							},
						).If(
							func() bool { return !paid },
							func(unpaidQ *event_queue.SequentialEventBuilder) {
								// REDUX DEVIATION: ENV-0004: original unpaid toll enters combat without explanation.
								// Explain the failed payment and await acknowledgement before closing the transcript.
								unpaidQ.Then(func() {
									dialog.AppendLine("Thou hast not enough gold. The toll is unpaid.")
								}).ThenSequence(g.thenWaitForInteractionAcknowledgement(dialog)).
									Then(func() { g.gameState.QueueBridgeTrollCombat() })
							},
						)
					},
					func(noQ *event_queue.SequentialEventBuilder) {
						noQ.Then(func() { g.gameState.QueueBridgeTrollCombat() })
					},
				)(tollQ)
			})
		}).Finally(
		func() {
			if dialog != nil {
				g.dialogStack.RemoveWidgetInstance(dialog)
			}
			g.keyboard.ResetReleaseDebouncing()
		}).
		Start()
}

// checkBridgeTrollEncounter checks if a troll encounter should occur at the current position.
// Returns true if an encounter was triggered and the workflow started.
//
// Conditions checked:
//   - Player is on a bridge tile
//   - Player is on foot (NoPartyVehicle)
//   - 1/8 random chance passes
func (g *GameScene) checkBridgeTrollEncounter(avatarTopTile *references.Tile) bool {
	// Must be on a bridge tile
	if avatarTopTile == nil || !avatarTopTile.IsTrollBridge() {
		return false
	}

	// Check encounter conditions (on foot, 1/8 chance)
	result := g.gameState.CheckBridgeTrollEncounter()
	if result == nil || !result.Triggered {
		return false
	}

	// Start the encounter workflow
	g.StartBridgeTrollEncounterWorkflow()

	return true
}
