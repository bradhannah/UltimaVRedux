package main

import (
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

// spellMixSession is owned by either the standalone Mix workflow or the retained
// Cast selector. Reusable sequence steps await it; no independent child workflow
// can race the parent or release its input ownership.
type spellMixSession struct {
	selected     *references.Spell
	dialog       *widgets.ReagentMixingDialog
	receipt      *widgets.OverlayDialog
	outcome      game_state.MixOutcome
	origin       references.Location
	mapType      references.GeneralMapType
	dungeonToken game_state.DungeonCommandCycle
	commandOwned bool
}

func (g *GameScene) thenSpellMixSession(session *spellMixSession) event_queue.Sequence {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		return q.Then(func() {
			g.keyboard.PrepareForTransition(0)
			session.dialog = widgets.NewReagentMixingDialog(g.keyboard, g.gameState.CurrentSpellCatalog(), g.buildReagentInventory())
			if session.selected != nil {
				session.dialog.ChooseSpell(*session.selected, true)
			}
			g.dialogStack.PushModalDialog(session.dialog)
		}).While(func() bool { return !session.dialog.IsDismissed() }, func() {
			if session.dialog.HasPendingMixRequest() {
				session.outcome = g.gameState.ExecuteMixing(session.dialog.ConsumePendingMixRequest())
				if session.outcome.Success {
					g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundMagic)
				}
				// A plain success goes straight back to Mix's list, which reports it on
				// its header. A newly learned recipe, and a mix opened from Cast or
				// Z-stats with no Mix list to return to, keep the result box.
				if session.outcome.Success && session.dialog.NeedsResultBox() {
					session.receipt = widgets.NewOverlayDialog(g.buildMixConfirmationMessage(session.outcome)).
						WithSizing(widgets.SizingAutoFitDefault).WithPosition(widgets.PositionScreenCenter).
						WithEffects(widgets.EffectsNone).WithContentAlignment(widgets.ContentAlignCenter).
						WithTextAlignment(widgets.TextAlignCenter).WithUserDismissed(g.keyboard).Build()
					session.receipt.Show()
					g.dialogStack.PushModalDialog(session.receipt)
				} else {
					session.dialog.Acknowledge(session.outcome, g.gameState.CurrentSpellCatalog(), g.buildReagentInventory())
				}
			}
			if session.receipt != nil && session.receipt.IsDismissed() {
				g.dialogStack.MustRemoveWidgetInstance(session.receipt)
				session.receipt = nil
				session.dialog.Acknowledge(session.outcome, g.gameState.CurrentSpellCatalog(), g.buildReagentInventory())
				g.keyboard.PrepareForTransition(0)
			}
		}, DialogPollingRateMs).Then(func() {
			g.dialogStack.MustRemoveWidgetInstance(session.dialog)
			session.dialog = nil
		}).ThenSequence(func(builder *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
			if session.outcome.TrapResult != nil {
				return g.applyMixTrapEffects(builder, session.outcome.TrapResult)
			}

			return builder
		})
	}
}

func (g *GameScene) cleanupSpellMixSession(session *spellMixSession) {
	if session.receipt != nil {
		g.dialogStack.RemoveWidgetInstance(session.receipt)
		session.receipt = nil
	}
	if session.dialog != nil {
		g.dialogStack.RemoveWidgetInstance(session.dialog)
		session.dialog = nil
	}
	g.clearCombatInvocationEffects()
	g.keyboard.PrepareForTransition(0)
}

func (g *GameScene) beginBrowserMixCommand(session *spellMixSession) {
	session.origin = g.gameState.MapState.PlayerLocation.Location
	session.mapType = session.origin.GetMapType()
	session.commandOwned = true
	if session.mapType == references.DungeonMapType {
		session.dungeonToken, _ = g.gameState.BeginDungeonCommandCycle()
		g.gameState.ClaimDungeonFullCommand(session.dungeonToken)
	}
}

func (g *GameScene) completeBrowserMixCommand(session *spellMixSession) {
	if !session.commandOwned {
		return
	}
	session.commandOwned = false
	if session.mapType == references.DungeonMapType {
		g.gameState.CompleteDungeonCommandCycle(session.dungeonToken, game_state.DungeonFullEndPlayer)
	} else {
		g.completeSurfaceCommand(newSurfaceCommandResult(surfaceCommandCompleted, session.mapType))
	}
}

func (g *GameScene) canResumeSpellBrowser(session *spellMixSession) bool {
	return g.gameState.GetCurrentSpellContext() != references.ContextCombat &&
		g.gameState.MapState.PlayerLocation.Location == session.origin && !g.gameState.PartyState.IsAllPartyDead()
}

func (g *GameScene) thenChooseSpell(state *spellCastingWorkflowState) event_queue.Sequence {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		return q.Then(func() {
			if state.spellDialog == nil {
				state.spellDialog = widgets.NewSpellCastingDialog(g.keyboard, &g.gameState.PartyState,
					g.gameState.GetCurrentSpellContext(), g.gameState.CurrentSpellCatalog(), g.gameState.ValidateSpellTargetingPrerequisites)
				state.spellDialog.SetInitialCaster(state.selectedCasterIdx)
				if g.gameState.GetCurrentSpellContext() == references.ContextCombat {
					state.spellDialog.LockCasterSelection(state.selectedCasterIdx)
				}
				g.dialogStack.PushModalDialog(state.spellDialog)
			}
		}).While(func() bool {
			return !state.spellDialog.IsDismissed() && !state.spellDialog.HasPendingCastRequest() &&
				!state.spellDialog.HasPendingMixRequest()
		}, func() {}, DialogPollingRateMs).ThenSequence(func(builder *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
			if state.spellDialog.HasPendingMixRequest() {
				spell := state.spellDialog.ConsumePendingMixSpell()
				state.mixSession = &spellMixSession{selected: &spell}

				return builder.Then(func() {
					if state.ownsDungeonCommand {
						g.gameState.CompleteDungeonCommandCycle(state.dungeonCommandToken, game_state.DungeonFullEndPlayer)
						state.ownsDungeonCommand = false
					}
					g.beginBrowserMixCommand(state.mixSession)
				}).ThenSequence(g.thenSpellMixSession(state.mixSession)).Then(func() {
					g.completeBrowserMixCommand(state.mixSession)
					g.cleanupSpellMixSession(state.mixSession)
				}).While(func() bool {
					return g.canResumeSpellBrowser(state.mixSession) && state.mixSession.mapType == references.DungeonMapType && !g.gameState.IsAtDungeonCommandBoundary()
				}, func() {}, DialogPollingRateMs).ThenSequence(func(resume *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
					if !g.canResumeSpellBrowser(state.mixSession) {
						return resume.Then(func() { state.spellDialog.Dismiss() })
					}

					return resume.Then(func() {
						if state.mixSession.mapType == references.DungeonMapType {
							state.dungeonCommandToken = g.gameState.BeginDeferredDungeonCommandCycle()
							state.ownsDungeonCommand = true
						}
						state.spellDialog.RefreshCatalog(g.gameState.CurrentSpellCatalog())
						state.mixSession = nil
					}).ThenSequence(g.thenChooseSpell(state))
				})
			}

			return builder.Then(func() {
				if state.spellDialog.HasPendingCastRequest() {
					state.pendingRequest = state.spellDialog.ConsumePendingCastRequest()
					state.hasRequest = true
					// "Cast...In Lor": the message window names the spell before its result.
					g.appendToCurrentRowStr(references.GetSpellName(state.pendingRequest.Spell))
					state.targetingProfile = references.GetSpellTargetingProfile(state.pendingRequest.Spell, g.gameState.GetCurrentSpellContext())
				}
				state.spellDialog.Dismiss()
				g.dialogStack.MustRemoveWidgetInstance(state.spellDialog)
			})
		})
	}
}

func (g *GameScene) startZStatsMix(dialog *widgets.ZStatsDossierDialog) {
	spell := dialog.ConsumePendingMixSpell()
	if g.gameState.GetCurrentSpellContext() == references.ContextCombat {
		dialog.RefreshDossier(g.gameState.CurrentZStatsDossier())
		g.gameState.SystemCallbacks.UI.ShowActionFeedback("Cannot mix during combat.", true)

		return
	}
	session := &spellMixSession{selected: &spell}
	returned := false
	g.gameState.EventQueueManager.Workflow("reagent_mixing_workflow").Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().First(func() { g.beginBrowserMixCommand(session) }).
		ThenSequence(g.thenSpellMixSession(session)).Then(func() { g.completeBrowserMixCommand(session) }).
		While(func() bool {
			return g.canResumeSpellBrowser(session) && session.mapType == references.DungeonMapType &&
				!g.gameState.IsAtDungeonCommandBoundary()
		}, func() {}, DialogPollingRateMs).
		Then(func() {
			if g.canResumeSpellBrowser(session) {
				dialog.RefreshDossier(g.gameState.CurrentZStatsDossier())
			} else {
				dialog.Dismiss()
			}
			returned = true
		}).Finally(func() {
		g.cleanupSpellMixSession(session)
		if !returned {
			dialog.Dismiss()
		}
	}).Start()
}
