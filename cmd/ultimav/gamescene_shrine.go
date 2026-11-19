package main

import (
	"fmt"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/minimap"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/shrine"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

const shrineMeditationWorkflowName = "shrine_meditation"

// StartShrineWorkflow initiates the shrine meditation workflow with event queue.
// Called when player enters a shrine tile on the large map.
// Shrine interaction and meditation follow ref:SHRINE-0002.
//
// Flow:
//  1. Transition to shrine map
//  2. Walk-in animation (4 steps from entrance to altar)
//  3. Display "Enter the shrine of\n[Virtue Name]"
//  4. Prompt "What virtue dost thou meditate upon?" - validate input
//  5. Prompt "Mantra:" 3 times - validate each mantra
//  6. If any wrong: "Thy thoughts are unfocused." and exit
//  7. Branch based on quest state (assign, complete, or offering)
//  8. Walk-out animation (4 steps from altar to entrance)
//  9. Transition back to overworld
func (g *GameScene) StartShrineWorkflow(shrineIndex int) {
	if g.gameState.EventQueueManager.HasWorkflow(shrineMeditationWorkflowName) {
		return
	}
	shrineData := game_state.GetShrineData(shrineIndex)
	if shrineData == nil {
		g.gameState.SystemCallbacks.Message.AddRowStr("Invalid shrine!")

		return
	}

	q := g.gameState.EventQueueManager.Workflow(shrineMeditationWorkflowName).
		Priority(event_queue.PriorityNormal).
		Sequential()
	g.buildShrineWorkflow(q, shrineIndex)
	q.Start()
}

// buildShrineWorkflow configures a shrine queue created by either the public starter or HandoffTo.
func (g *GameScene) buildShrineWorkflow(q *event_queue.SequentialEventBuilder, shrineIndex int) {
	shrineData := game_state.GetShrineData(shrineIndex)
	if shrineData == nil {
		panic(fmt.Sprintf("buildShrineWorkflow: invalid shrine index %d", shrineIndex))
	}

	// Variables for tracking validation state
	var virtueValid bool
	var meditationCancelled bool
	mantraAllValid := true
	var enteredShrine bool
	transcript := widgets.NewInteractionTranscript(g.keyboard, widgets.InteractionTranscriptConfig{
		MaxLines:       8,
		MaxInputLength: 40,
		WidthSamples: []string{
			"Enter the shrine of " + shrineData.VirtueName + ".",
			"What virtue dost thou meditate upon?",
			"Virtue?  > " + strings.Repeat("M", 20),
			"Mantra:  > " + strings.Repeat("M", 20),
			"Offer how many hundredweights gold?",
			"Hundredweights?  > 9",
			"Thou hast not that much gold!",
		},
	})
	removeTranscript := func() {
		if transcript != nil {
			g.dialogStack.RemoveWidgetInstance(transcript)
			transcript = nil
		}
	}

	// Define shrine positions
	entrancePos := references.Position{X: shrine.CenterX, Y: shrine.EntranceY}
	altarPos := references.Position{X: shrine.CenterX, Y: shrine.AltarY}

	// Create walk-in script: from entrance (5,10) to one tile south of altar (5,6)
	// Switch to the kneeling/praying beggar sprite after reaching the altar (ref:SHRINE-0002).
	walkInScript := minimap.NewMiniMapScript("shrine_walk_in").
		WalkTo(minimap.ActorAvatar, altarPos.Down(), shrine.WalkDelayMs).
		WithWalkingSounds().
		SetSprite(minimap.ActorAvatar, indexes.Begger1). // Key frame (364) for animation
		Build()

	// Create walk-out script: restore avatar sprite, walk back to entrance, then hide actor
	walkOutScript := minimap.NewMiniMapScript("shrine_walk_out").
		SetSprite(minimap.ActorAvatar, indexes.AvatarKeyIndex).
		WalkTo(minimap.ActorAvatar, entrancePos.Down(), shrine.WalkDelayMs).
		WithWalkingSounds().
		SetVisible(minimap.ActorAvatar, false).
		Build()

	q.BlockInput().
		First(func() {
			if err := g.gameState.TransitionToShrineMap(shrineIndex); err != nil {
				g.gameState.SystemCallbacks.Message.AddRowStr("Cannot enter shrine!")

				return
			}

			g.miniMapScene = minimap.NewMiniMapScene()
			g.miniMapScene.AddActor(minimap.ActorAvatar, entrancePos, indexes.AvatarKeyIndex)
			enteredShrine = true
		}).ThenSequence(

		g.thenRunMiniMapScript(walkInScript)).
		Then(func() {
			g.gameState.SystemCallbacks.Message.AddRowStr(
				"Enter the shrine of\n" + shrineData.VirtueName)
			transcript.AppendLine("Enter the shrine of " + shrineData.VirtueName + ".")
			transcript.AppendLine("What virtue dost thou meditate upon?")
			g.dialogStack.PushModalDialog(transcript)
		}).ThenSequence(

		g.thenPromptInteractionText(
			transcript,
			"Virtue?",
			func(text string, submitted bool) {
				if !submitted || text == "" {
					meditationCancelled = true

					return
				}
				virtueValid = game_state.ValidateVirtue(text, shrineData.VirtueKey)
			},
		)).If(

		func() bool { return !meditationCancelled && !virtueValid },
		func(q *event_queue.SequentialEventBuilder) {
			// REDUX DEVIATION: SHRINE-0002: original asks for all mantras after
			// a wrong nonempty virtue. Redux rejects immediately to avoid futile prompts.
			q.Then(func() {
				g.gameState.SystemCallbacks.Message.AddRowStr("Thy thoughts are unfocused.")
				transcript.AppendLine("Thy thoughts are unfocused.")
			}).ThenSequence(g.thenWaitForInteractionAcknowledgement(transcript))
		}).If(

		func() bool { return virtueValid },
		func(q *event_queue.SequentialEventBuilder) {
			for range game_state.MantraRepetitions {
				q.If(func() bool { return !meditationCancelled }, func(mq *event_queue.SequentialEventBuilder) {
					mq.ThenSequence(g.thenPromptInteractionText(transcript, "Mantra:", func(text string, submitted bool) {
						if !submitted || text == "" {
							meditationCancelled = true

							return
						}
						if !game_state.ValidateMantra(text, shrineData.Mantra) {
							mantraAllValid = false
						}
					}))
				})
			}
			q.If(

				func() bool { return !meditationCancelled && !mantraAllValid },
				func(failQ *event_queue.SequentialEventBuilder) {
					failQ.Then(func() {
						g.gameState.SystemCallbacks.Message.AddRowStr("Thy thoughts are unfocused.")
						transcript.AppendLine("Thy thoughts are unfocused.")
					}).ThenSequence(g.thenWaitForInteractionAcknowledgement(transcript))
				}).If(

				func() bool { return !meditationCancelled && mantraAllValid },
				func(successQ *event_queue.SequentialEventBuilder) {
					g.addShrineQuestStateBranching(successQ, shrineIndex, transcript, removeTranscript)
				})
		}).
		Then(func() { removeTranscript() }).
		ThenSequence(

			g.thenRunMiniMapScript(walkOutScript)).
		Wait(1000).Finally(

		func() {
			removeTranscript()
			g.miniMapScene = nil
			if enteredShrine {
				if _, err := g.gameState.FinishShrineVisit(); err != nil {
					g.gameState.SystemCallbacks.Message.AddRowStr("Cannot leave shrine!")
				}
			}
			g.keyboard.ResetReleaseDebouncing()
		})
}

// addShrineQuestStateBranching adds the quest state branching logic to the event queue
// Note: This function does NOT add Finally() - cleanup is handled at the end of the main queue
// to ensure walk-out animation runs after quest branching completes.
func (g *GameScene) addShrineQuestStateBranching(
	q *event_queue.SequentialEventBuilder,
	shrineIndex int,
	transcript *widgets.InteractionTranscript,
	removeTranscript func(),
) {
	appendOutcome := func(message string) {
		g.gameState.SystemCallbacks.Message.AddRowStr(message)
		transcript.AppendLine(message)
	}
	review, err := g.gameState.ReviewShrineMeditation(shrineIndex)
	if err != nil {
		q.Then(func() { appendOutcome("Something feels wrong...") }).
			ThenSequence(g.thenWaitForInteractionAcknowledgement(transcript))

		return
	}

	switch review.QuestState {
	case game_state.QuestStateNotAssigned:
		var outcome game_state.ShrineQuestOutcome
		var committed bool
		q.Then(func() {
			outcome, err = g.gameState.CommitShrineQuestAssignment(shrineIndex, review.Fingerprint)
			committed = err == nil
		}).If(func() bool { return !committed }, func(failQ *event_queue.SequentialEventBuilder) {
			failQ.Then(func() { appendOutcome("The quest is no longer available.") }).
				ThenSequence(g.thenWaitForInteractionAcknowledgement(transcript))
		}).If(func() bool { return committed }, func(assignQ *event_queue.SequentialEventBuilder) {
			assignQ.Then(func() {
				removeTranscript()
				g.gameState.SystemCallbacks.Message.AddRowStr(outcome.PrimaryMessage)
				g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundShrineQuestAssign)
			}).ThenSequence(
				g.thenShowShrineDialog(
					outcome.PrimaryMessage + "\n" +
						"'Tis now thy sacred Quest to go unto the Codex and learn\n" +
						outcome.QuestText)).
				Then(func() {
					g.gameState.SystemCallbacks.Message.AddRowStr(outcome.FollowupMessage)
				}).ThenSequence(g.thenShowConfirmationDialog(outcome.FollowupMessage))
		})

	case game_state.QuestStateReadyToComplete:
		var outcome game_state.ShrineQuestOutcome
		var committed bool
		q.Then(func() {
			outcome, err = g.gameState.CommitShrineQuestCompletion(shrineIndex, review.Fingerprint)
			committed = err == nil
		}).If(func() bool { return !committed }, func(failQ *event_queue.SequentialEventBuilder) {
			failQ.Then(func() { appendOutcome("The reward is no longer available.") }).
				ThenSequence(g.thenWaitForInteractionAcknowledgement(transcript))
		}).If(func() bool { return committed }, func(completeQ *event_queue.SequentialEventBuilder) {
			completionMessage := outcome.PrimaryMessage + "\n\nThy " + outcome.VirtueName + " hath been rewarded!"
			if len(outcome.StatBonuses) > 0 {
				completionMessage += "\n\n" + strings.Join(outcome.StatBonuses, "\n")
			}
			completeQ.Then(func() {
				removeTranscript()
				g.showExperienceAward(outcome.Experience, false)
			}).ThenSequence(g.thenShowShrineQuestCompletionEffects(completionMessage, outcome.Experience))
		})

	case game_state.QuestStateOffering:
		// Gold offering mode - player has completed quest at Codex
		// ref:KARMA-0010: digit × 100 = gold, karma = digit value only.
		q.Then(func() {
			transcript.AppendLine("Offer how many hundredweights gold? (1-9, or 0 to cancel)")
		}).ThenSequence(g.thenShrineOfferingAttempt(shrineIndex, transcript, appendOutcome))

	default:
		q.Then(func() { appendOutcome("Something feels wrong...") }).
			ThenSequence(g.thenWaitForInteractionAcknowledgement(transcript))
	}
}

// thenShrineOfferingAttempt inserts only one attempt; an acknowledged refusal adds the next at runtime.
func (g *GameScene) thenShrineOfferingAttempt(
	shrineIndex int,
	transcript *widgets.InteractionTranscript,
	appendOutcome func(string),
) event_queue.Sequence {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		var offeringDigit int
		var offeringValid bool
		var offeringReview game_state.ShrineOfferingReview
		var offeringOutcome game_state.ShrineOfferingOutcome
		var reviewed, affordable, committed bool
		failureMessage := "Something feels wrong..."

		return q.ThenSequence(g.thenPromptInteractionTextWithValidation(
			transcript,
			"Hundredweights?",
			"^[0-9]$",
			func(text string, submitted bool) {
				if !submitted || len(text) == 0 {
					offeringDigit = 0

					return
				}
				offeringDigit = int(text[0] - '0')
				offeringValid = true
			},
		)).If(func() bool { return offeringDigit == 0 }, func(cancelQ *event_queue.SequentialEventBuilder) {
			cancelQ.Then(func() { appendOutcome("Thou dost leave the shrine.") }).
				ThenSequence(g.thenWaitForInteractionAcknowledgement(transcript))
		}).If(func() bool { return offeringDigit > 0 && offeringValid }, func(offerQ *event_queue.SequentialEventBuilder) {
			offerQ.Then(func() {
				var err error
				offeringReview, err = g.gameState.ReviewShrineOffering(shrineIndex, offeringDigit)
				reviewed = err == nil
				affordable = reviewed && offeringReview.Affordable
				if reviewed && !affordable {
					failureMessage = offeringReview.Message
				}
			}).If(func() bool { return !reviewed }, func(failQ *event_queue.SequentialEventBuilder) {
				failQ.Then(func() { appendOutcome(failureMessage) }).
					ThenSequence(g.thenWaitForInteractionAcknowledgement(transcript))
			}).If(func() bool { return reviewed && !affordable }, func(retryQ *event_queue.SequentialEventBuilder) {
				retryQ.Then(func() { appendOutcome(failureMessage) }).
					ThenSequence(g.thenWaitForInteractionAcknowledgement(transcript)).
					ThenSequence(g.thenShrineOfferingAttempt(shrineIndex, transcript, appendOutcome))
			}).If(func() bool { return affordable }, func(commitQ *event_queue.SequentialEventBuilder) {
				commitQ.Then(func() {
					var err error
					offeringOutcome, err = g.gameState.CommitShrineOffering(offeringReview)
					committed = err == nil
					if !committed {
						failureMessage = "The offering is no longer possible."
					}
				}).If(func() bool { return !committed }, func(failQ *event_queue.SequentialEventBuilder) {
					failQ.Then(func() { appendOutcome(failureMessage) }).
						ThenSequence(g.thenWaitForInteractionAcknowledgement(transcript))
				}).If(func() bool { return committed }, func(successQ *event_queue.SequentialEventBuilder) {
					successQ.Then(func() {
						g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundShrineOffering)
						g.gameState.SystemCallbacks.Visual.SetScreenFlashColor(1.0, 1.0, 1.0)
						g.gameState.SystemCallbacks.Visual.SetScreenFlashIntensity(0.8)
					}).Animate(200, func(progress float32) {
						intensity := 0.8 * (1.0 - float64(progress))
						g.gameState.SystemCallbacks.Visual.SetScreenFlashIntensity(intensity)
					}).Then(func() {
						appendOutcome(offeringOutcome.Message)
					}).ThenSequence(g.thenWaitForInteractionAcknowledgement(transcript))
				})
			})
		})
	}
}

// StartCodexWorkflow initiates the Codex interaction workflow with event queue.
// Called when player enters the Codex tile.
// Codex revelation follows ref:SHRINE-0003.
//
// Flow:
//  1. "The book doth open..."
//  2. Find first active quest
//  3. If no active quest: "The pages are blank!" and exit
//  4. Show answer for that quest
//  5. Mark quest as completed
//  6. If all 8 complete: trigger endgame sequence
func (g *GameScene) StartCodexWorkflow() {
	if g.gameState.EventQueueManager.HasWorkflow("codex_reading") {
		return
	}
	var enteredCodex bool
	var memorial game_state.CodexMemorial

	// Define positions for Codex map (11x11 map, Codex book is near top around Y=2)
	entrancePos := references.Position{X: shrine.CenterX, Y: shrine.EntranceY}
	codexBookPos := references.Position{X: shrine.CenterX, Y: 3} // Just before the Codex book at Y=2

	// Create walk-in script: from entrance (5,10) to just before Codex book (5,3)
	// Switch to the kneeling/praying beggar sprite after reaching the Codex (ref:SHRINE-0003).
	walkInScript := minimap.NewMiniMapScript("codex_walk_in").
		WalkTo(minimap.ActorAvatar, codexBookPos, shrine.WalkDelayMs).
		WithWalkingSounds().
		SetSprite(minimap.ActorAvatar, indexes.Begger1). // Key frame (364) for animation
		Build()

	// Create walk-out script: restore avatar sprite, walk back to entrance, then hide actor
	walkOutScript := minimap.NewMiniMapScript("codex_walk_out").
		SetSprite(minimap.ActorAvatar, indexes.AvatarKeyIndex).
		WalkTo(minimap.ActorAvatar, entrancePos.Down(), shrine.WalkDelayMs).
		WithWalkingSounds().
		SetVisible(minimap.ActorAvatar, false).
		Build()

	g.gameState.EventQueueManager.Workflow("codex_reading").
		Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().
		First(func() {
			if err := g.gameState.TransitionToCodexMap(); err != nil {
				g.gameState.SystemCallbacks.Message.AddRowStr("Cannot approach the Codex!")

				return
			}

			g.miniMapScene = minimap.NewMiniMapScene()
			g.miniMapScene.AddActor(minimap.ActorAvatar, entrancePos, indexes.AvatarKeyIndex)
			memorial = g.gameState.ReviewCodexMemorial()
			for index, position := range memorial.UrnPositions {
				g.miniMapScene.AddActor(
					minimap.ActorID(fmt.Sprintf("martyr-urn-%d", index)),
					position,
					indexes.ItemPotion,
				)
			}
			// REDUX DEVIATION: SHRINE-0003: original martyr placement indexes beyond its eight layout tables
			// when more than eight martyrs exist. Redux renders eight urns but lists every name safely.
			enteredCodex = true

			g.gameState.SystemCallbacks.Message.AddRowStr("Thou dost approach the Codex...")
		}).If(
		func() bool { return memorial.Text != "" },
		func(q *event_queue.SequentialEventBuilder) {
			q.Then(func() {
				g.gameState.SystemCallbacks.Message.AddRowStr(memorial.Text)
			}).ThenSequence(g.thenShowShrineDialog(memorial.Text))
		}).ThenSequence(

		g.thenRunMiniMapScript(walkInScript)).ThenSequence(

		g.thenShowConfirmationDialog("The book doth open...")).ThenSequence(

		func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
			review, err := g.gameState.ReviewCodexQuest()
			if err != nil {
				q.Then(func() {
					g.gameState.SystemCallbacks.Message.AddRowStr("Something feels wrong...")
				}).ThenSequence(g.thenShowConfirmationDialog("Something feels wrong..."))

				return q
			}
			if review.AllQuestsComplete {
				g.addCodexRevelationSequence(q)

				return q
			}

			if review.ActiveQuest < 0 {
				q.Then(func() {
					g.gameState.SystemCallbacks.Message.AddRowStr(review.EmptyMessage)
				}).ThenSequence(g.thenShowConfirmationDialog(review.EmptyMessage))

				return q
			}

			var outcome game_state.CodexQuestOutcome
			var committed bool
			q.Then(func() {
				outcome, err = g.gameState.CommitCodexQuestAnswer(review)
				committed = err == nil
				if !committed {
					g.gameState.SystemCallbacks.Message.AddRowStr("The answer is no longer available.")
				}
			}).If(func() bool { return committed }, func(answerQ *event_queue.SequentialEventBuilder) {
				answerQ.Then(func() {
					g.gameState.SystemCallbacks.Message.AddRowStr(outcome.Heading)
					g.gameState.SystemCallbacks.Message.AddRowStr(outcome.AnswerText)
					g.showExperienceAward(outcome.Experience, false)
				}).ThenSequence(g.thenShowShrineExperienceDialog(outcome.Heading+"\n"+outcome.AnswerText, outcome.Experience)).
					If(func() bool { return outcome.AllQuestsComplete }, func(endQ *event_queue.SequentialEventBuilder) {
						g.addCodexRevelationSequence(endQ)
					})
			})

			return q
		}).
		ThenSequence(

			g.thenRunMiniMapScript(walkOutScript)).
		Wait(1000).Finally(

		func() {
			g.miniMapScene = nil
			if enteredCodex {
				if _, err := g.gameState.FinishShrineVisit(); err != nil {
					g.gameState.SystemCallbacks.Message.AddRowStr("Cannot leave the Codex!")
				}
			}
			g.keyboard.ResetReleaseDebouncing()
		}).
		Start()
}

// thenShowShrineDialog creates an auto-fit confirmation dialog for shrine messages.
// Uses SizingAutoFitDefault instead of SizingCompact to handle multi-line quest text.
func (g *GameScene) thenShowShrineDialog(message string) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return g.thenShowShrineExperienceDialog(message, game_state.DeedExperienceAward{})
}

// thenShowShrineExperienceDialog is the shrine dialog with any deed experience
// the message's deed paid, in the experience colour.
func (g *GameScene) thenShowShrineExperienceDialog(
	message string, experience game_state.DeedExperienceAward,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		overlay := widgets.NewOverlayDialog(message).
			WithSizing(widgets.SizingAutoFitDefault).
			WithPosition(widgets.PositionMapCenterLower).
			WithEffects(widgets.EffectsNone).
			WithContentAlignment(widgets.ContentAlignCenter).
			WithTextAlignment(widgets.TextAlignCenter).
			WithUserDismissed(g.keyboard).
			AutoPopFromStack(&g.dialogStack).
			WithStyledLines(experienceDialogLines(experience)...).
			Build()

		return q.ThenSequence(g.showOverlayAndWait(overlay))
	}
}

// addCodexRevelationSequence adds the dramatic revelation sequence when all 8 quests complete.
// Codex presentation uses 3 flash/earthquake cycles followed by hints (ref:SHRINE-0003).
// Uses existing thenSmoothApparitionFlash and thenSingleLightningFlash helpers.
func (g *GameScene) addCodexRevelationSequence(q *event_queue.SequentialEventBuilder) {
	q.ThenSequence(

		g.thenSmoothApparitionFlash(1.0, 0.0, 0.0, 0.8, 500)).
		Then(func() {
			g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundEarthquake)
		}).
		Wait(300).ThenSequence(

		g.thenSingleLightningFlash(500, 0.8)).
		Wait(300).ThenSequence(

		g.thenSmoothApparitionFlash(1.0, 0.0, 0.0, 0.8, 500)).
		Then(func() {
			g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundEarthquake)
		}).ThenSequence(

		g.thenShowShrineDialog(game_state.CodexRevelationMessages[0])).ThenSequence(

		g.thenShowShrineDialog(game_state.CodexRevelationMessages[1])).ThenSequence(

		g.thenShowShrineDialog(game_state.CodexRevelationMessages[2])).ThenSequence(

		g.thenShowShrineDialog(game_state.CodexRevelationMessages[3])).Then(func() {
		// The word-bearing page has been presented and acknowledged, not merely queued.
		g.gameState.DiscoverWordOfPowerKnowledge("doom", game_state.RevelationKnowledgeSource("Codex of Ultimate Wisdom"))
		g.showExperienceAward(g.gameState.SettleDeedExperience(), true)
	}).ThenSequence(

		g.thenShowShrineDialog(game_state.CodexRevelationMessages[4])).Then(func() {
		// Final-page disclosure is distinct from the earlier word acquisition.
		g.gameState.RecordCompleteCodexRevelation()
	})
}
