package main

import (
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

const staleUseSelectionMessage = "You cannot use that now."

func (g *GameScene) startSurfaceUseWorkflow() {
	g.startUseWorkflow(game_state.UsageField, true)
}

// StartCombatUseWorkflow owns selection, effect presentation, and action completion.
func (g *GameScene) StartCombatUseWorkflow() {
	g.startUseWorkflow(game_state.UsageCombat, false)
}

func (g *GameScene) startUseWorkflow(context game_state.UsageContext, completeSurfaceCycle bool) {
	commitSequence := g.gameState.CastUseCommitSequence()
	origin := g.gameState.MapState.PlayerLocation
	combatOwner := g.gameState.CaptureCombatUseOwner()
	originLocation := g.gameState.MapState.PlayerLocation.Location
	originMapType := g.gameState.MapState.PlayerLocation.Location.GetMapType()
	// Get usable items from game state
	gsItems := g.gameState.GetUsableItemsForContext(context)

	// Check if there are any usable items
	if len(gsItems) == 0 {
		g.addRowStr("No usable items!")

		return
	}
	// Create and show the item selection dialog
	var dialog *widgets.ItemSelectionDialog
	var selectedItem *game_state.UseableItemInfo
	var executableItem *game_state.UseableItemInfo
	var hmsCapeConfirmation *hmsCapeConfirmationState
	combatItemCommitted := false
	fieldItemAccepted := false
	parentFinalized := false
	effectFinalized := false
	completionDelivered := false
	completeOwnedSurfaceCycle := func() {
		if completionDelivered || !parentFinalized || (fieldItemAccepted && !effectFinalized) {
			return
		}
		completionDelivered = true
		state := surfaceCommandCancelled
		if fieldItemAccepted {
			state = surfaceCommandCompleted
		}
		result := newSurfaceCommandResult(state, originMapType)
		result.ConsumesNormalCycle = g.gameState.CastUseCommitSequence() != commitSequence
		result.MapTransitioned = g.gameState.MapState.PlayerLocation.Location != originLocation
		g.completeSurfaceCommand(result)
	}
	effectComplete := func() {
		if effectFinalized {
			return
		}
		effectFinalized = true
		completeOwnedSurfaceCycle()
	}

	g.gameState.EventQueueManager.Workflow("use_item_workflow").
		Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().
		First(func() {
			dialog = widgets.NewItemSelectionDialog(widgets.ItemSelectionDialogConfig{
				Items:       gsItems,
				Keyboard:    g.keyboard,
				SpriteSheet: g.spriteSheet,
				LastUsed:    g.lastUsedItem,
				OnSelect: func(item game_state.UseableItemInfo) {
					g.lastUsedItem = &item
					if g.gameState.MapState.PlayerLocation != origin {
						g.gameState.SystemCallbacks.UI.ShowActionFeedback("You cannot use that here.", true)

						return
					}
					if context == game_state.UsageCombat {
						selectedItem = &item
					} else {
						current, ok := g.gameState.RevalidateUseItemSelection(item, context)
						if !ok {
							g.gameState.SystemCallbacks.UI.ShowActionFeedback("You have none!", true)

							return
						}
						if completeSurfaceCycle {
							fieldItemAccepted = g.gameState.UseItemForSurface(current, effectComplete)
						} else {
							fieldItemAccepted = g.gameState.UseItem(current)
						}
					}
				},
				OnCancel: func() {

				},
			})
			g.dialogStack.PushModalDialog(dialog)
		}).
		While(
			func() bool { return !dialog.IsDismissed() },
			func() {},
			16,
		).If(
		func() bool { return selectedItem != nil },
		func(q *event_queue.SequentialEventBuilder) {
			q.Then(func() {
				if !g.gameState.IsCurrentCombatUseOwner(combatOwner) || g.gameState.MapState.PlayerLocation != origin {
					g.gameState.SystemCallbacks.UI.ShowActionFeedback("Not thy turn!", true)

					return
				}
				current, ok := g.gameState.RevalidateUseItemSelection(*selectedItem, context)
				if !ok {
					g.gameState.SystemCallbacks.UI.ShowActionFeedback("You have none!", true)

					return
				}
				executableItem = &current
			}).If(
				func() bool { return executableItem != nil },
				func(effectQ *event_queue.SequentialEventBuilder) {
					effectQ.ThenSequence(func(useQ *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
						canCommit := func() bool {
							return g.validateCombatUseSelection(*selectedItem, combatOwner, origin)
						}

						return g.thenUseCombatItem(
							*executableItem, &hmsCapeConfirmation, canCommit, &combatItemCommitted,
						)(useQ)
					})
				},
			)
		}).Finally(

		func() {
			g.dialogStack.RemoveWidgetInstance(dialog)
			g.cleanupHMSCapeConfirmation(hmsCapeConfirmation)
			if context == game_state.UsageCombat {
				g.clearCastUseDirectionPrompt()
				g.clearPocketWatchDialog()
				g.clearPotionEffectDialog()
				g.clearCombatInvocationEffects()
				g.gameState.SystemCallbacks.Visual.SetScreenFlashIntensity(0)
				g.gameState.SystemCallbacks.Visual.SetScreenInvert(0, 0)
			}
			g.keyboard.ResetReleaseDebouncing()
			if hmsCapeConfirmation != nil && hmsCapeConfirmation.committed {
				combatItemCommitted = true
			}
			if context == game_state.UsageCombat && combatItemCommitted && g.gameState.IsCurrentCombatUseOwner(combatOwner) {
				g.gameState.FinishCombatAction()
			}
			if context == game_state.UsageField && completeSurfaceCycle {
				parentFinalized = true
				completeOwnedSurfaceCycle()
			}
		}).
		Start()
}

// validateCombatUseSelection runs immediately before a deferred item commitment.
// The caller retains the original menu owner and quantity, rather than rebasing
// them when a nested sequence or direction prompt is constructed.
func (g *GameScene) validateCombatUseSelection(
	item game_state.UseableItemInfo,
	owner game_state.CombatUseOwner,
	origin references.PlayerLocation,
) bool {
	valid := g.gameState.IsCurrentCombatUseOwner(owner) && g.gameState.MapState.PlayerLocation == origin
	if valid {
		_, valid = g.gameState.RevalidateUseItemSelection(item, game_state.UsageCombat)
	}
	if !valid {
		g.gameState.SystemCallbacks.UI.ShowActionFeedback(staleUseSelectionMessage, true)

		return false
	}

	return true
}

// StartDungeonUseWorkflow retains the accepted command while selecting a supported dungeon item.
func (g *GameScene) StartDungeonUseWorkflow(token game_state.DungeonCommandCycle) {
	origin := g.gameState.MapState.PlayerLocation
	gsItems := g.gameState.GetUsableItemsForContext(game_state.UsageDungeon)
	if len(gsItems) == 0 {
		g.gameState.SystemCallbacks.Message.AddRowStr("No usable items!")
		g.gameState.CompleteDungeonCommandCycle(token, game_state.DungeonFullEndPlayer)

		return
	}

	var dialog *widgets.ItemSelectionDialog
	var dungeonViewDialog *widgets.DungeonViewDialog
	var selectedItem *game_state.UseableItemInfo
	var executableItem *game_state.UseableItemInfo
	var model game_state.DungeonViewModel
	viewReady := false
	effectOwned := false
	effectFinalized := false
	parentFinalized := false
	completionDelivered := false
	completeCommand := func() {
		if completionDelivered || !parentFinalized || (effectOwned && !effectFinalized) {
			return
		}
		completionDelivered = true
		g.gameState.CompleteDungeonCommandCycle(token, game_state.DungeonFullEndPlayer)
	}
	effectComplete := func() {
		if effectFinalized {
			return
		}
		effectFinalized = true
		completeCommand()
	}

	g.gameState.EventQueueManager.Workflow("dungeon_use_item_workflow").
		Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().
		First(func() {
			dialog = widgets.NewItemSelectionDialog(widgets.ItemSelectionDialogConfig{
				Items:       gsItems,
				Keyboard:    g.keyboard,
				SpriteSheet: g.spriteSheet,
				LastUsed:    g.lastUsedItem,
				OnSelect: func(item game_state.UseableItemInfo) {
					g.lastUsedItem = &item
					selectedItem = &item
				},
				OnCancel: func() {},
			})
			g.dialogStack.PushModalDialog(dialog)
		}).While(
		func() bool { return !dialog.IsDismissed() },
		func() {},
		DialogPollingRateMs,
	).If(
		func() bool { return selectedItem != nil },
		func(q *event_queue.SequentialEventBuilder) {
			q.Then(func() {
				if g.gameState.MapState.PlayerLocation != origin {
					g.gameState.SystemCallbacks.UI.ShowActionFeedback("You cannot use that here.", true)

					return
				}
				current, ok := g.gameState.RevalidateUseItemSelection(*selectedItem, game_state.UsageDungeon)
				if !ok {
					g.gameState.SystemCallbacks.UI.ShowActionFeedback("You have none!", true)

					return
				}
				executableItem = &current
			}).If(
				func() bool { return executableItem != nil },
				func(useQ *event_queue.SequentialEventBuilder) {
					useQ.Then(func() {
						current, ok := g.gameState.RevalidateUseItemSelection(*selectedItem, game_state.UsageDungeon)
						if !ok || g.gameState.MapState.PlayerLocation != origin {
							g.gameState.SystemCallbacks.UI.ShowActionFeedback(staleUseSelectionMessage, true)

							return
						}
						effectOwned = true
						g.dispatchDungeonUseItem(current, &model, &viewReady, effectComplete)
					})
				},
			).If(
				func() bool { return viewReady },
				func(viewQ *event_queue.SequentialEventBuilder) {
					viewQ.ThenSequence(g.thenShowDungeonViewDialog(&model, &dungeonViewDialog)).
						Then(effectComplete)
				},
			)
		},
	).Finally(func() {
		if dialog != nil {
			g.dialogStack.RemoveWidgetInstance(dialog)
		}
		if dungeonViewDialog != nil {
			dungeonViewDialog.Dismiss()
			g.dialogStack.RemoveWidgetInstance(dungeonViewDialog)
		}
		if viewReady {
			effectComplete()
		}
		parentFinalized = true
		completeCommand()
	}).
		Start()
}

func (g *GameScene) dispatchDungeonUseItem(
	item game_state.UseableItemInfo,
	viewModel *game_state.DungeonViewModel,
	viewReady *bool,
	complete func(),
) {
	switch itemType := item.ItemType.(type) {
	case references.Potion:
		g.startDungeonPotionWorkflow(itemType, complete)
	case references.Scroll:
		g.dispatchDungeonScroll(itemType, viewModel, viewReady, complete)
	case references.QuestItem:
		g.dispatchDungeonQuestItem(item, itemType, complete)
	case references.Moonstone:
		g.gameState.SystemCallbacks.Message.AddRowStr("Moonstone cannot be buried here!")
		complete()
	case references.Provision:
		if itemType != references.SkullKeys {
			panic("dungeon Use received unsupported provision")
		}
		g.gameState.SystemCallbacks.Message.AddRowStr("Not here!")
		complete()
	default:
		panic("dungeon Use received unsupported item type")
	}
}

func (g *GameScene) dispatchDungeonScroll(
	scroll references.Scroll,
	viewModel *game_state.DungeonViewModel,
	viewReady *bool,
	complete func(),
) {
	if scroll == references.ScrollInManiCorp {
		g.startResurrectionScrollWorkflow(complete)

		return
	}
	if !g.gameState.CommitScrollUse(scroll) {
		g.gameState.SystemCallbacks.UI.ShowActionFeedback("You have none!", true)
		complete()

		return
	}

	switch scroll {
	case references.ScrollVasLor:
		g.gameState.SystemCallbacks.Message.AddRowStr("Light!")
		g.startLightScrollWorkflow(complete)
	case references.ScrollInSanct:
		g.gameState.SystemCallbacks.Message.AddRowStr("Protection!")
		g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundMagic)
		duration := g.gameState.GameReferences.InventoryItemReferences.GetScrollEffectDuration(scroll)
		g.gameState.ActivateProtectionEffect(duration)
		complete()
	case references.ScrollInAn:
		g.gameState.SystemCallbacks.Message.AddRowStr("Negate magic!")
		g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundMagic)
		duration := g.gameState.GameReferences.InventoryItemReferences.GetScrollEffectDuration(scroll)
		g.gameState.ActivateNegateMagicEffect(duration)
		complete()
	case references.ScrollInQuasWis:
		var err error
		*viewModel, err = g.gameState.CurrentDungeonViewModel()
		if err != nil {
			panic(err)
		}
		*viewReady = true
		g.gameState.SystemCallbacks.Message.AddRowStr("View!")
		g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundMagic)
	case references.ScrollInManiCorp:
		panic("resurrection scroll must select its target first")
	case references.ScrollAnTym:
		g.startTimeStopScrollWorkflow(complete)
	case references.ScrollRelHur, references.ScrollKalXenCorp:
		panic("dungeon Use received unsupported scroll")
	}
}

func (g *GameScene) dispatchDungeonQuestItem(
	item game_state.UseableItemInfo,
	questItem references.QuestItem,
	complete func(),
) {
	switch questItem {
	case references.Amulet, references.Crown, references.BlackBadge, references.PocketWatch, references.HMSCape:
		if !g.gameState.UseItemWithCompletion(item, complete) {
			complete()
		}
	case references.Sceptre:
		g.gameState.SystemCallbacks.Message.AddRowStr("Sceptre")
		g.gameState.SystemCallbacks.Message.AddRowStr("Wielding the Sceptre of Lord British...")
		result := g.gameState.UseDungeonSceptre()
		if result.Cleared {
			g.presentDungeonFieldDispel(result.Change)
		}
		complete()
	case references.Carpet, references.Spyglass:
		g.gameState.SystemCallbacks.Message.AddRowStr("Not here!")
		complete()
	case references.Sextant:
		g.gameState.SystemCallbacks.Message.AddRowStr("Only outdoors!")
		complete()
	case references.WoodenBox:
		g.gameState.SystemCallbacks.Message.AddRowStr("How?")
		complete()
	case references.ShardOfFalsehood, references.ShardOfHatred, references.ShardOfCowardice:
		g.gameState.SystemCallbacks.Message.AddRowStr("No effect!")
		complete()
	case references.Grapple:
		panic("dungeon Use received unsupported quest item")
	}
}

// thenUseCombatItem records only this selection's authoritative commitment.
// H.M.S. Cape retains its commit result in its confirmation state for cleanup.
func (g *GameScene) thenUseCombatItem(
	item game_state.UseableItemInfo,
	hmsCapeConfirmation **hmsCapeConfirmationState,
	canCommit func() bool,
	didCommit *bool,
) event_queue.Sequence {
	if !canCommit() {
		return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder { return q }
	}

	switch itemType := item.ItemType.(type) {
	case references.Potion:
		return g.thenUseCombatPotion(itemType, canCommit, didCommit)
	case references.Scroll:
		return g.thenUseCombatScroll(itemType, canCommit, didCommit)
	case references.Provision:
		if itemType == references.SkullKeys {
			return g.thenUseCombatSkullKey(canCommit, didCommit)
		}
	case references.QuestItem:
		if itemType == references.PocketWatch {
			return g.thenUseCombatPocketWatch(canCommit, didCommit)
		}
		if itemType == references.Crown || itemType == references.Amulet || itemType == references.BlackBadge {
			return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
				return q.Then(func() {
					if !canCommit() {
						return
					}
					*didCommit = g.gameState.UseItem(item)
				})
			}
		}
		if itemType == references.Sceptre {
			return g.thenUseCombatSceptre(canCommit, didCommit)
		}
		if itemType == references.HMSCape {
			commit, review := g.gameState.BeginHMSCapeUse()
			if commit == nil || review.Result != game_state.HMSCapeUseReady {
				return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
					return q.Then(func() { g.gameState.SystemCallbacks.UI.ShowActionFeedback("You have none!", true) })
				}
			}
			confirmation := &hmsCapeConfirmationState{}
			*hmsCapeConfirmation = confirmation

			return g.thenConfirmHMSCape(commit, confirmation)
		}
	}

	panic("combat Use workflow received an unsupported item")
}
