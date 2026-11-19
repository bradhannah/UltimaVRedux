package main

import (
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

const (
	shardEarthquakeCount = 3
	shardImpactCount     = 7
)

// StartShardWorkflow presents and resolves the shard ritual (ref:SHADOW-0005).
func (g *GameScene) StartShardWorkflow(shard references.QuestItem) {
	g.startShardWorkflow(shard, nil)
}

func (g *GameScene) startShardWorkflow(shard references.QuestItem, complete func()) {
	commitSequence := g.gameState.CastUseCommitSequence()
	review := game_state.ShardRitualReview{Result: game_state.ShardUsageNotAttempted}
	var commit *game_state.ShardRitualCommit
	earthquakeStarted := false

	g.gameState.EventQueueManager.Workflow("shard_workflow").
		Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().
		First(func() {
			g.shadowlordShardActive = true
			review = g.gameState.ReviewShardRitual(shard)
		}).If(
		func() bool { return review.Result != game_state.ShardUsageNotAttempted },
		func(q *event_queue.SequentialEventBuilder) {
			q.ThenSequence(func(innerQ *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
				shardName := g.gameReferences.InventoryItemReferences.GetQuestItemDisplayName(shard)
				message := fmt.Sprintf("Gem Shard\n\nThou dost hold above thee\nthe evil %s...", shardName)

				return g.thenShowShardConfirmationDialog(message, nil)(innerQ)
			}).Then(func() {
				g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundShardPulse)
				review = g.gameState.ReviewShardRitual(shard)
			})
		}).If(
		func() bool { return review.Result == game_state.ShardUsageNoEffect && !review.AtMatchingFlame },
		func(q *event_queue.SequentialEventBuilder) {
			q.ThenSequence(g.thenShowShardConfirmationDialog("No effect!", nil))
		}).If(
		func() bool { return review.AtMatchingFlame },
		func(q *event_queue.SequentialEventBuilder) {
			q.ThenSequence(func(innerQ *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
				message := fmt.Sprintf("...and cast it into the\nFlame of %s!", review.Flame.GetPrinciple())

				return g.thenShowShardConfirmationDialog(message, nil)(innerQ)
			}).ThenSequence(
				g.thenShardEarthquakeEffects(&earthquakeStarted),
			).Then(func() {
				commit, review = g.gameState.BeginShardRitualCommit(shard)
				if review.Result == game_state.ShardUsageSuccess {
					review = g.gameState.CommitShardRitual(commit)
				}
			}).If(
				func() bool { return review.Result == game_state.ShardUsageSuccess },
				func(successQ *event_queue.SequentialEventBuilder) {
					successQ.ThenSequence(
						g.thenShardImpacts(review.Target),
					).ThenSequence(func(innerQ *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
						message := fmt.Sprintf(
							"The doom of the Shadowlord\n%s is wrought!",
							review.Shadowlord.String(),
						)

						return g.thenShowShardExperienceDialog(message, review.Experience, func() {
							g.gameState.DiscoverShadowlordKnowledge(
								review.Shadowlord, game_state.ShadowlordKnowledgeName,
								game_state.RevelationKnowledgeSource("Banishment"),
							)
							g.showExperienceAward(review.Experience, false)
						})(innerQ)
					}).Then(func() {
						g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundShadowlordVictory)
					})
				},
			)
		}).Finally(func() {
		if earthquakeStarted {
			g.gameState.SystemCallbacks.Audio.Stop(game_state.SoundShardRitualEarthquake).
				FadeOut(earthquakeFadeOutMs).
				Now()
		}
		g.clearShadowlordShardPresentation()
		g.keyboard.ResetReleaseDebouncing()
		if complete == nil && !g.disposed && g.gameState.CastUseCommitSequence() != commitSequence {
			g.gameState.FinishTurn()
		}
		if complete != nil {
			complete()
		}
	}).
		Start()
}

func (g *GameScene) thenShowShardConfirmationDialog(message string, onPresented func()) event_queue.Sequence {
	return g.thenShowShardExperienceDialog(message, game_state.DeedExperienceAward{}, onPresented)
}

// thenShowShardExperienceDialog is the ritual dialog with any deed experience
// the ritual paid, in the experience colour.
func (g *GameScene) thenShowShardExperienceDialog(
	message string, experience game_state.DeedExperienceAward, onPresented func(),
) event_queue.Sequence {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		dialog := widgets.NewOverlayDialog(message).
			AsConfirmation().
			WithUserDismissed(g.keyboard).
			AutoPopFromStack(&g.dialogStack).
			WithStyledLines(experienceDialogLines(experience)...).
			Build()

		return q.Then(func() {
			g.shadowlordShardDialog = dialog
			dialog.ShowWithoutAnimation()
			g.dialogStack.PushModalDialog(dialog)
			if onPresented != nil {
				onPresented()
			}
		}).While(func() bool { return !dialog.IsDismissed() }, func() {}, DialogPollingRateMs).Then(func() {
			g.keyboard.ResetReleaseDebouncing()
			if g.shadowlordShardDialog == dialog {
				g.shadowlordShardDialog = nil
			}
		})
	}
}

func (g *GameScene) clearShadowlordShardPresentation() {
	if dialog := g.shadowlordShardDialog; dialog != nil {
		dialog.HideWithoutAnimation()
		g.dialogStack.RemoveWidgetInstance(dialog)
		g.shadowlordShardDialog = nil
	}
	g.gameState.SystemCallbacks.Visual.SetScreenShakeOffset(0, 0)
	g.shadowlordShardActive = false
}

// thenShardEarthquakeEffects inserts the three source-ordered earthquake calls.
func (g *GameScene) thenShardEarthquakeEffects(started *bool) event_queue.Sequence {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		q = q.Then(func() {
			*started = true
			g.gameState.SystemCallbacks.Audio.Play(game_state.SoundShardRitualEarthquake).
				Loop().
				FadeIn(earthquakeFadeInMs).
				Start()
		})
		for range shardEarthquakeCount {
			q = q.ThenSequence(
				game_state.NewPartyStateBuilder(g.gameState).
					ToAllParty().
					WithShake(2000, 10.0).
					Build(),
			)
		}

		return q
	}
}

// thenShardImpacts inserts the seven source-ordered kapow effects at the destroyed NPC position.
func (g *GameScene) thenShardImpacts(position references.Position) event_queue.Sequence {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		return q.RepeatWithDelay(shardImpactCount, 250, func(_ int) bool {
			g.playKapowAtPosition(position)

			return true
		})
	}
}
