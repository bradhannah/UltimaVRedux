package main

import (
	"fmt"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/internal/visual"
)

// Horse spawn materialization animation duration in milliseconds.
// Matches moongate/wanderer dissolve timing for visual consistency.
const (
	horseSpawnMaterializeDurationMs = 500
	wishingWellWorkflowName         = "wishing_well_workflow"
)

// ShowWellInteraction handles the modern wishing-well sequence on one persistent transcript.
// The Event Queue owns the dialog lifecycle, prompt sequencing, gameplay mutation, and cleanup.
// Gold is deducted AFTER wish validation (UX improvement over original Ultima V).
func (g *GameScene) ShowWellInteraction(pos *references.Position) {
	if pos == nil {
		panic("ShowWellInteraction called with nil position")
	}
	if g.gameState.EventQueueManager.HasWorkflow(wishingWellWorkflowName) {
		return
	}
	wellPosition := *pos
	var dialog *widgets.InteractionTranscript
	var dropCoin bool
	var wishText string
	var wishSubmitted bool
	var isHorseWish bool
	var wishCommitted bool
	var needsAcknowledgement bool
	var review game_state.WishingWellReview
	var outcome game_state.WishingWellOutcome
	appendOutcome := func(message string) {
		dialog.AppendLine(message)
		g.addRowStr(message)
		needsAcknowledgement = true
	}

	g.gameState.EventQueueManager.Workflow(wishingWellWorkflowName).
		Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().Then(func() {
		dialog = widgets.NewInteractionTranscript(g.keyboard, widgets.InteractionTranscriptConfig{
			MaxLines:       4,
			MaxInputLength: WishMaxLength,
			WidthSamples: []string{
				"Thou dost see a well.",
				"Drop a coin?  > Yes",
				"Thy wish?  > " + strings.Repeat("M", WishMaxLength),
				"The well answers not.",
				"Not enough gold!",
				"Thy wish can no longer be granted.",
				"No effect!",
				"Poof a Horse!",
			},
		})
		dialog.AppendLine("Thou dost see a well.")
		dialog.BeginYesNo("Drop a coin?")
		g.dialogStack.PushModalDialog(dialog)
	}).While(
		func() bool { return !dialog.InputComplete() },
		func() {},
		DialogPollingRateMs,
	).Then(func() {
		dropCoin = dialog.Response() == "Yes"
		g.keyboard.SetForceWaitAnyKey(KeyboardDebounceMs)
	}).Wait(KeyboardDebounceMs).If(
		func() bool { return dropCoin },
		func(yesQ *event_queue.SequentialEventBuilder) {
			yesQ.Then(func() {
				var err error
				review, err = g.gameState.ReviewWishingWell(wellPosition)
				if err != nil {
					appendOutcome("The well answers not.")

					return
				}
				if !review.CanAfford {
					appendOutcome(review.Message)
				}
			}).If(
				func() bool { return review.CanAfford && !needsAcknowledgement },
				func(wishQ *event_queue.SequentialEventBuilder) {
					wishQ.Then(func() {
						dialog.BeginText("Thy wish?", fmt.Sprintf("^[[:print:]]{0,%d}$", WishMaxLength))
					}).While(
						func() bool { return !dialog.InputComplete() },
						func() {},
						DialogPollingRateMs,
					).Then(func() {
						wishText = dialog.Response()
						wishSubmitted = dialog.Submitted()
						g.keyboard.SetForceWaitAnyKey(KeyboardDebounceMs)
					}).Wait(KeyboardDebounceMs).If(
						func() bool { return wishSubmitted },
						func(commitQ *event_queue.SequentialEventBuilder) {
							commitQ.Then(func() {
								var err error
								outcome, err = g.gameState.CommitWishingWellWish(game_state.WishingWellCommitRequest{
									WellPosition:        wellPosition,
									Wish:                wishText,
									ExpectedFingerprint: review.Fingerprint,
								})
								if err != nil {
									appendOutcome(outcome.FailureMessage)

									return
								}

								wishCommitted = true
								isHorseWish = outcome.Effect == game_state.WishingWellHorseEffect
								g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundCoinSplash)
							}).If(
								func() bool { return wishCommitted && isHorseWish },
								func(horseQ *event_queue.SequentialEventBuilder) {
									var horseDissolveID int
									horseQ.Then(func() {
										horseDissolveID = g.StartTileDissolveWithOptions(TileDissolveOptions{
											Position:     outcome.SpawnPosition,
											SpriteIndex:  indexes.HorseRight,
											Mode:         visual.DissolveModeReveal,
											OutlineColor: sprites.WhiteOutline,
											PositionMode: PositionModeWorld,
											RenderLayer:  RenderLayerDefault,
										})
										g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundMagicPoof)
									}).Animate(horseSpawnMaterializeDurationMs, func(progress float32) {
										g.UpdateTileDissolveProgress(horseDissolveID, float64(progress))
									}).Then(func() {
										g.StopTileDissolve(horseDissolveID)
										g.addRowStr(outcome.AppearanceMessage)
										appendOutcome(outcome.FeedbackMessage)
									})
								}).If(
								func() bool { return wishCommitted && !isHorseWish },
								func(_ *event_queue.SequentialEventBuilder) {
									appendOutcome(outcome.FeedbackMessage)
								})
						})
				})
		}).If(
		func() bool { return !dropCoin },
		func(noQ *event_queue.SequentialEventBuilder) {
			noQ.Then(func() { needsAcknowledgement = true })
		}).If(
		func() bool { return needsAcknowledgement },
		func(ackQ *event_queue.SequentialEventBuilder) {
			ackQ.Then(func() {
				dialog.BeginAcknowledgement()
			}).While(
				func() bool { return !dialog.InputComplete() },
				func() {},
				DialogPollingRateMs,
			)
		}).Finally(func() {
		if dialog != nil {
			g.dialogStack.MustRemoveWidgetInstance(dialog)
		}
		g.keyboard.ResetReleaseDebouncing()
	}).Start()
}
