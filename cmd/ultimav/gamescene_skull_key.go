package main

import (
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

func (g *GameScene) startSkullKeyWorkflow(commit *game_state.SkullKeyUseCommit, complete func()) {
	commitSequence := g.gameState.CastUseCommitSequence()
	canUnlock := false
	g.gameState.EventQueueManager.Workflow("skull_key_workflow").
		Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().
		First(func() {
			g.gameState.SystemCallbacks.Message.AddRowStr("Skull Key")
			canUnlock = g.gameState.ReviewSkullKeyUse().Result == game_state.SkullKeyUseReady
			if !canUnlock {
				g.gameState.SystemCallbacks.UI.ShowActionFeedback("Not here!", true)
			}
		}).If(func() bool { return canUnlock }, func(q *event_queue.SequentialEventBuilder) {
		q.ThenSequence(g.thenShowCastUseDirectionPrompt(
			"Direction-",
			func(dir references.Direction) {
				if g.gameState.CommitSkullKeyUse(commit) != game_state.SkullKeyUseConsumed {
					g.gameState.SystemCallbacks.UI.ShowActionFeedback("You have none!", true)

					return
				}
				review := g.gameState.ReviewSkullKeyUnlock(commit, dir)
				result := g.gameState.CommitSkullKeyUnlockForSurfaceUse(review)
				g.showSkullKeyResult(result, review.Target)
			},
			func() {},
		))
	}).Finally(

		func() {
			g.clearCastUseDirectionPrompt()
			g.gameState.CancelSkullKeyUnlock(commit)
			g.keyboard.ResetReleaseDebouncing()
			if complete != nil {
				complete()
			} else if g.gameState.CastUseCommitSequence() != commitSequence {
				g.gameState.AdvanceGameplayMinutesAndProcessProvisions(1)
			}
		}).
		Start()
}

// thenUseCombatSkullKey borrows the owning Use workflow's prompt and completion.
func (g *GameScene) thenUseCombatSkullKey(canCommit func() bool, didCommit *bool) event_queue.Sequence {
	commit, _ := g.gameState.BeginSkullKeyUse()
	var dialog *widgets.MessageDialog
	inputComplete := false

	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		return q.Then(func() {
			if !canCommit() || commit == nil {
				return
			}
			g.gameState.SystemCallbacks.Message.AddRowStr("Skull Key")
			dialog = widgets.NewMessageDialog("Direction-\n\nArrows/keypad; Esc/Space cancels").
				WithTextLines(5).
				WithCharsPerLine(24).
				WithContentTextAlignment(widgets.ContentAlignCenter).
				WithTextHorizontalCenter(true).
				WithVerticalPosition(defaultMessageVertical).
				WithHorizontalPosition(widgets.DialogHorizontalMapOnly).
				Build()
			dialog.ShowWithoutAnimation()
			g.castUseDirectionDialog = dialog
			g.dialogStack.PushModalDialog(dialog)
			g.keyboard.SetAllowKeyPressImmediately()
		}).While(
			func() bool { return dialog != nil && !inputComplete },
			func() {
				if g.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
					inputComplete = true

					return
				}
				input := g.ordinaryDirectionInput()
				if !canCommit() {
					inputComplete = true

					return
				}
				switch input.Outcome {
				case ordinaryDirectionActive:
					return
				case ordinaryDirectionPassed:
					inputComplete = true
				case ordinaryDirectionSelected:
					inputComplete = true
					g.commitCombatSkullKeyDirection(commit, input.Direction, didCommit)
				}
			},
			DialogPollingRateMs,
		).Then(func() {
			g.clearCastUseDirectionPrompt()
			g.keyboard.ResetReleaseDebouncing()
		})
	}
}

func (g *GameScene) commitCombatSkullKeyDirection(
	commit *game_state.SkullKeyUseCommit,
	direction references.Direction,
	didCommit *bool,
) {
	review, committed := g.gameState.CommitCombatSkullKeyUse(commit, direction)
	*didCommit = committed
	if !committed {
		g.gameState.SystemCallbacks.UI.ShowActionFeedback(staleUseSelectionMessage, true)

		return
	}
	g.showSkullKeyResult(review.Result, review.Target)
}

func (g *GameScene) showSkullKeyResult(result game_state.SkullKeyUnlockResult, target references.Position) {
	if result != game_state.SkullKeyUnlockSuccess {
		g.gameState.SystemCallbacks.UI.ShowActionFeedback("No effect!", true)

		return
	}
	g.playKapowAtPosition(target)
	g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundUnlock)
	g.gameState.SystemCallbacks.UI.ShowActionFeedback("Unlocked!", true)
}
