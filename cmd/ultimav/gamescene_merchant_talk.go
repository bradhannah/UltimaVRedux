package main

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

const (
	merchantDialogPollingRateMs = 16
	merchantHorseSemanticName   = "horse"
)

type merchantTalkWorkflowState struct {
	session                  game_state.MerchantSession
	sessionStarted           bool
	advanceTime              bool
	finished                 bool
	ownedDialogs             merchantDialogRegistry
	armsTrade                *widgets.ArmsTradeDialog
	guildPurchase            *widgets.GuildPurchaseDialog
	healerTreatment          *widgets.HealerTreatmentDialog
	innAction                *widgets.InnActionDialog
	tavernAction             *widgets.TavernActionDialog
	tavernResult             *widgets.TavernResultDialog
	tavernNotice             *widgets.TavernAdviceNotice
	tavernDismissAfterResult bool
	reagentPurchase          *widgets.ReagentPurchaseDialog
	horsePurchase            *widgets.HorsePurchaseDialog
	shipPurchase             *widgets.ShipPurchaseDialog
	shipDispatch             *widgets.ShipDispatchDialog
	shipTravel               *widgets.ShipTravelDialog
	interactionTranscript    *widgets.InteractionTranscript
}

type shipShopWorkflowProse struct {
	dialog         widgets.ShipShopDialogProse
	politeFarewell string
	fundsRefusal   string
}

type innShopWorkflowProse struct {
	dialog              widgets.InnShopDialogProse
	politeFarewell      string
	restFundsRefusal    string
	pickupGuardRefusal  string
	leaveAcknowledgment string
}

type merchantSessionStarter func() (game_state.MerchantSession, error)

func (s *merchantTalkWorkflowState) dismissAndRemoveOwnedDialogs(stack *widgets.DialogStack) {
	s.ownedDialogs.dismissAndRemoveAll(stack)
}

func (g *GameScene) thenHandleMerchantTalk(
	npc *map_units.NPCFriendly,
	workflowState *merchantTalkWorkflowState,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return g.thenHandleMerchantSession(func() (game_state.MerchantSession, error) {
		return g.gameState.BeginMerchantSession(npc)
	}, workflowState)
}

func (g *GameScene) thenHandleMerchantSession(
	startSession merchantSessionStarter,
	workflowState *merchantTalkWorkflowState,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		var session game_state.MerchantSession
		var sessionErr error

		return q.Then(func() {
			session, sessionErr = startSession()
			if sessionErr == nil {
				workflowState.session = session
				workflowState.sessionStarted = true
			}
		}).If(
			func() bool { return sessionErr != nil },
			func(errorQ *event_queue.SequentialEventBuilder) {
				logging.Warnf(logging.CategoryConversation, "merchant session unavailable: %v", sessionErr)
				errorQ.ThenSequence(g.thenShowConfirmationDialog(merchantSessionFeedback(sessionErr)))
			},
		).If(
			func() bool {
				return sessionErr == nil && session.Family() == references.MerchantFamilyShip
			},
			func(shipQ *event_queue.SequentialEventBuilder) {
				shipQ.ThenSequence(g.thenRunShipShop(session, workflowState))
			},
		).If(
			func() bool {
				return sessionErr == nil && session.Family() == references.MerchantFamilyArms
			},
			func(armsQ *event_queue.SequentialEventBuilder) {
				armsQ.ThenSequence(g.thenRunArmsShop(session, workflowState))
			},
		).If(
			func() bool {
				return sessionErr == nil && session.Family() == references.MerchantFamilyReagent
			},
			func(reagentQ *event_queue.SequentialEventBuilder) {
				reagentQ.ThenSequence(g.thenRunReagentShop(session, workflowState))
			},
		).If(
			func() bool {
				return sessionErr == nil && session.Family() == references.MerchantFamilyGuild
			},
			func(guildQ *event_queue.SequentialEventBuilder) {
				guildQ.ThenSequence(g.thenRunGuildShop(session, workflowState))
			},
		).If(
			func() bool {
				return sessionErr == nil && session.Family() == references.MerchantFamilyHealer
			},
			func(healerQ *event_queue.SequentialEventBuilder) {
				healerQ.ThenSequence(g.thenRunHealerShop(session, workflowState))
			},
		).If(
			func() bool {
				return sessionErr == nil && session.Family() == references.MerchantFamilyTavern
			},
			func(tavernQ *event_queue.SequentialEventBuilder) {
				tavernQ.ThenSequence(g.thenRunTavernShop(session, workflowState))
			},
		).If(
			func() bool {
				return sessionErr == nil && session.Family() == references.MerchantFamilyHorse
			},
			func(horseQ *event_queue.SequentialEventBuilder) {
				horseQ.ThenSequence(g.thenRunHorseShop(session, workflowState))
			},
		).If(
			func() bool {
				return sessionErr == nil && session.Family() == references.MerchantFamilyInn
			},
			func(innQ *event_queue.SequentialEventBuilder) {
				innQ.ThenSequence(g.thenRunInnShop(session, workflowState))
			},
		)
	}
}

// REDUX DEVIATION: MERCH-0004: original Arms Talk accepts one Buy or Sell mode then exits; Redux permits B/S/Tab mode switching and repeated trades within the same owning Talk minute.
func (g *GameScene) thenRunArmsShop(
	session game_state.MerchantSession,
	workflowState *merchantTalkWorkflowState,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return g.thenRunArmsShopAuthority(session, game_state.ArmsShopReview{}, false, workflowState)
}

func (g *GameScene) thenRunArmsPreview(
	review game_state.ArmsShopReview,
	workflowState *merchantTalkWorkflowState,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return g.thenRunArmsShopAuthority(game_state.MerchantSession{}, review, true, workflowState)
}

func (g *GameScene) thenRunArmsShopAuthority(
	session game_state.MerchantSession,
	review game_state.ArmsShopReview,
	preview bool,
	workflowState *merchantTalkWorkflowState,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		var dialog *widgets.ArmsShopDialog
		var state game_state.ArmsShopState
		var setupErr error

		return q.Then(func() {
			state = review.State
			merchantSpriteIndex := review.MerchantSpriteIndex
			if !preview {
				var err error
				state, err = g.gameState.GetArmsShopState(session)
				if err != nil {
					setupErr = fmt.Errorf("loading Arms shop state: %w", err)

					return
				}
				merchantSpriteIndex = session.MerchantSpriteIndex()
			}
			prose, err := g.resolveArmsShopProse(state)
			if err != nil {
				setupErr = err

				return
			}
			merchantSprite := g.spriteSheet.GetSprite(merchantSpriteIndex)
			if merchantSprite == nil {
				setupErr = fmt.Errorf(
					"loaded merchant sprite is unavailable: %w",
					game_state.ErrInvalidMerchantSession,
				)

				return
			}

			dialog = widgets.NewArmsShopDialog(widgets.ArmsShopDialogConfig{
				Keyboard:       g.keyboard,
				MerchantSprite: merchantSprite,
				State:          state,
				Prose:          prose,
				Preview:        preview,
				IsKeyPressed:   g.keyboard.IsKeyHeld,
			})
			g.keyboard.PrepareForTransition(0)
			workflowState.pushOwnedDialog(&g.dialogStack, dialog)
		}).If(
			func() bool { return setupErr != nil },
			func(errorQ *event_queue.SequentialEventBuilder) {
				logging.Errorf(logging.CategoryConversation, "Arms shop setup failed: %v", setupErr)
				errorQ.ThenSequence(g.thenShowConfirmationDialog("Arms service is unavailable."))
			},
		).While(
			func() bool { return dialog != nil && !dialog.IsDismissed() },
			func() {
				if workflowState.armsTrade != nil {
					tradeDialog := workflowState.armsTrade
					if !tradeDialog.IsDismissed() {
						return
					}
					g.dialogStack.RemoveWidgetInstance(tradeDialog)
					workflowState.armsTrade = nil

					request, confirmed := tradeDialog.ConsumePendingRequest()
					if tradeDialog.IsCancelled() {
						dialog.CancelPendingTrade()

						return
					}
					if refusal := tradeDialog.GetRefusalKind(); refusal != widgets.ArmsTradeNoRefusal {
						dialog.ApplyRefusal(resolveArmsLocalRefusalDisplay(state, tradeDialog.GetIntent(), refusal))
						if refusal.IsTerminal() {
							dialog.Dismiss()
						}

						return
					}
					if !confirmed {
						dialog.CancelPendingTrade()

						return
					}

					dialog.AcceptTradeIntent(request)
					if preview {
						dialog.ApplyPreviewStopped(request)

						return
					}
					if request.Mode == widgets.ArmsShopBuy {
						g.applyArmsBuyRequest(session, dialog, request.Buy, &state)

						return
					}
					g.applyArmsSellRequest(session, dialog, request.Sell, &state)

					return
				}

				intent, pending := dialog.ConsumePendingTradeIntent()
				if !pending {
					return
				}
				prose, err := g.resolveArmsTradeProse(state, intent)
				if err != nil {
					logging.Errorf(logging.CategoryConversation, "Arms trade prose failed: %v", err)
					dialog.ApplySessionError("The Arms transaction could not be prepared.")

					return
				}
				tradeDialog := widgets.NewArmsTradeDialog(widgets.ArmsTradeDialogConfig{
					Keyboard: g.keyboard,
					Intent:   intent,
					Prose:    prose,
				})
				workflowState.armsTrade = tradeDialog
				g.keyboard.PrepareForTransition(0)
				workflowState.pushOwnedDialog(&g.dialogStack, tradeDialog)
			},
			merchantDialogPollingRateMs,
		)
	}
}

func (g *GameScene) applyArmsBuyRequest(
	session game_state.MerchantSession,
	dialog *widgets.ArmsShopDialog,
	request game_state.ArmsBuyRequest,
	state *game_state.ArmsShopState,
) {
	current, reviewErr := g.gameState.GetArmsShopState(session)
	if reviewErr == nil && !reflect.DeepEqual(current, *state) {
		for _, quote := range current.BuyQuotes {
			if quote.Equipment == request.Equipment && request.Quantity <= quote.MaxQuantity {
				dialog.ApplySessionError("The ledger changed while you browsed. Leave and speak to the merchant again.")

				return
			}
		}
		// Existing capacity/funds refusals below remain authoritative and never mutate.
	}
	result, err := g.gameState.BuyArmsEquipment(session, request)
	if err != nil {
		logging.Errorf(logging.CategoryConversation, "Arms purchase failed: %v", err)
		switch {
		case errors.Is(err, game_state.ErrArmsCapacity):
			g.refreshArmsAfterRefusal(
				session,
				dialog,
				state,
				resolveArmsBuyRefusalDisplay(*state, request.Equipment, err),
			)
		case errors.Is(err, game_state.ErrArmsInsufficientFunds):
			dialog.ApplyTerminalRefusal(resolveArmsBuyRefusalDisplay(*state, request.Equipment, err))
		default:
			dialog.ApplySessionError("The Arms transaction could not be completed.")
		}

		return
	}
	g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundMerchantTransaction)
	refreshed, err := g.gameState.GetArmsShopState(session)
	if err != nil {
		logging.Errorf(logging.CategoryConversation, "Arms purchase refresh failed: %v", err)
		dialog.ApplySessionError("The Arms ledger could not be refreshed.")

		return
	}
	*state = refreshed
	dialog.ApplyBuyResult(result, refreshed, resolveArmsBuyResultDisplay(state.Merchant, result))
}

func (g *GameScene) applyArmsSellRequest(
	session game_state.MerchantSession,
	dialog *widgets.ArmsShopDialog,
	request game_state.ArmsSellRequest,
	state *game_state.ArmsShopState,
) {
	result, err := g.gameState.SellArmsEquipment(session, request)
	if err != nil {
		logging.Errorf(logging.CategoryConversation, "Arms sale failed: %v", err)
		switch {
		case errors.Is(err, game_state.ErrArmsUnsellableAmmunition):
			dialog.ApplyTerminalRefusal(
				resolveArmsSellAuthorityRefusalDisplay(*state, request.Equipment, err),
			)
		case errors.Is(err, game_state.ErrArmsZeroPrice), errors.Is(err, game_state.ErrArmsGoldCapacity):
			g.refreshArmsAfterRefusal(
				session,
				dialog,
				state,
				resolveArmsSellAuthorityRefusalDisplay(*state, request.Equipment, err),
			)
		default:
			dialog.ApplySessionError("The Arms transaction could not be completed.")
		}

		return
	}
	g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundMerchantTransaction)
	refreshed, err := g.gameState.GetArmsShopState(session)
	if err != nil {
		logging.Errorf(logging.CategoryConversation, "Arms sale refresh failed: %v", err)
		dialog.ApplySessionError("The Arms ledger could not be refreshed.")

		return
	}
	*state = refreshed
	dialog.ApplySellResult(result, refreshed, resolveArmsSellResultDisplay(state.Merchant, result))
}

func (g *GameScene) refreshArmsAfterRefusal(
	session game_state.MerchantSession,
	dialog *widgets.ArmsShopDialog,
	state *game_state.ArmsShopState,
	display widgets.ArmsTradeResultDisplay,
) {
	refreshed, err := g.gameState.GetArmsShopState(session)
	if err != nil {
		logging.Errorf(logging.CategoryConversation, "Arms refusal refresh failed: %v", err)
		dialog.ApplySessionError("The Arms ledger could not be refreshed.")

		return
	}
	*state = refreshed
	dialog.ApplyRefusalWithRefresh(refreshed, display)
}

func (g *GameScene) thenRunReagentShop(
	session game_state.MerchantSession,
	workflowState *merchantTalkWorkflowState,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return g.thenRunReagentShopAuthority(session, game_state.ReagentShopReview{}, false, workflowState)
}

func (g *GameScene) thenRunReagentPreview(
	review game_state.ReagentShopReview,
	workflowState *merchantTalkWorkflowState,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return g.thenRunReagentShopAuthority(game_state.MerchantSession{}, review, true, workflowState)
}

func (g *GameScene) thenRunReagentShopAuthority(
	session game_state.MerchantSession,
	review game_state.ReagentShopReview,
	preview bool,
	workflowState *merchantTalkWorkflowState,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		var dialog *widgets.ReagentShopDialog
		var state game_state.ReagentShopState
		var setupErr error

		return q.Then(func() {
			state = review.State
			merchantSpriteIndex := review.MerchantSpriteIndex
			if !preview {
				var err error
				state, err = g.gameState.GetReagentShopState(session)
				if err != nil {
					setupErr = fmt.Errorf("loading reagent shop state: %w", err)

					return
				}
				merchantSpriteIndex = session.MerchantSpriteIndex()
			}
			prose, err := g.resolveReagentShopProse(state)
			if err != nil {
				setupErr = err

				return
			}
			merchantSprite := g.spriteSheet.GetSprite(merchantSpriteIndex)
			if merchantSprite == nil {
				setupErr = fmt.Errorf(
					"loaded merchant sprite is unavailable: %w",
					game_state.ErrInvalidMerchantSession,
				)

				return
			}

			dialog = widgets.NewReagentShopDialog(widgets.ReagentShopDialogConfig{
				Keyboard:       g.keyboard,
				MerchantSprite: merchantSprite,
				State:          state,
				Prose:          prose,
				Preview:        preview,
			})
			g.keyboard.PrepareForTransition(0)
			workflowState.pushOwnedDialog(&g.dialogStack, dialog)
		}).If(
			func() bool { return setupErr != nil },
			func(errorQ *event_queue.SequentialEventBuilder) {
				logging.Errorf(logging.CategoryConversation, "reagent shop setup failed: %v", setupErr)
				errorQ.ThenSequence(g.thenShowConfirmationDialog("Reagent service is unavailable."))
			},
		).While(
			func() bool { return dialog != nil && !dialog.IsDismissed() },
			func() {
				if workflowState.reagentPurchase != nil {
					purchaseDialog := workflowState.reagentPurchase
					if !purchaseDialog.IsDismissed() {
						return
					}
					g.dialogStack.RemoveWidgetInstance(purchaseDialog)
					workflowState.reagentPurchase = nil

					request, confirmed := purchaseDialog.ConsumePendingPurchaseRequest()
					if purchaseDialog.IsCancelled() || !confirmed {
						dialog.CancelQuantitySelection()

						return
					}

					dialog.AcceptQuantitySelection(request)
					if preview {
						dialog.ApplyPreviewStopped(request)

						return
					}
					current, reviewErr := g.gameState.GetReagentShopState(session)
					if reviewErr != nil || !reflect.DeepEqual(current, state) {
						dialog.ApplySessionError("The reagent transaction could not be completed.")

						return
					}
					result, err := g.gameState.PurchaseReagentOffer(session, request)
					if err != nil {
						logging.Errorf(logging.CategoryConversation, "reagent purchase failed: %v", err)
						dialog.ApplySessionError("The reagent transaction could not be completed.")

						return
					}
					g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundMerchantTransaction)
					refreshed, err := g.gameState.GetReagentShopState(session)
					if err != nil {
						logging.Errorf(logging.CategoryConversation, "reagent refresh failed: %v", err)
						dialog.ApplySessionError("The reagent ledger could not be refreshed.")

						return
					}
					state = refreshed
					dialog.ApplyPurchaseResult(result, refreshed, resolveReagentPurchaseResultDisplay(state.Merchant, result))

					return
				}

				intent, pending := dialog.ConsumePendingPurchaseIntent()
				if !pending {
					return
				}
				purchaseDialog := widgets.NewReagentPurchaseDialog(widgets.ReagentPurchaseDialogConfig{
					Keyboard: g.keyboard,
					Quote:    intent.Quote,
					Preview:  preview,
				})
				workflowState.reagentPurchase = purchaseDialog
				g.keyboard.PrepareForTransition(0)
				workflowState.pushOwnedDialog(&g.dialogStack, purchaseDialog)
			},
			merchantDialogPollingRateMs,
		)
	}
}

func (g *GameScene) thenRunHorseShop(
	session game_state.MerchantSession,
	workflowState *merchantTalkWorkflowState,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return g.thenRunHorseShopAuthority(session, game_state.HorseShopReview{}, false, workflowState)
}

func (g *GameScene) thenRunHorsePreview(
	review game_state.HorseShopReview,
	workflowState *merchantTalkWorkflowState,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return g.thenRunHorseShopAuthority(game_state.MerchantSession{}, review, true, workflowState)
}

func (g *GameScene) thenRunHorseShopAuthority(
	session game_state.MerchantSession,
	review game_state.HorseShopReview,
	preview bool,
	workflowState *merchantTalkWorkflowState,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		var dialog *widgets.HorseShopDialog
		var setupErr error

		return q.Then(func() {
			state := review.State
			merchantSpriteIndex := review.MerchantSpriteIndex
			if !preview {
				var err error
				state, err = g.gameState.GetHorseShopState(session)
				if err != nil {
					setupErr = fmt.Errorf("loading horse shop state: %w", err)

					return
				}
				merchantSpriteIndex = session.MerchantSpriteIndex()
			}
			prose, err := g.resolveHorseShopProse(state)
			if err != nil {
				setupErr = err

				return
			}
			if g.spriteSheet == nil {
				setupErr = fmt.Errorf("loaded horse shop sprites are unavailable: %w", game_state.ErrInvalidMerchantSession)

				return
			}
			merchantSprite := g.spriteSheet.GetSprite(merchantSpriteIndex)
			horseSprite := g.spriteSheet.GetSprite(indexes.HorseRight)
			if merchantSprite == nil || horseSprite == nil {
				setupErr = fmt.Errorf("loaded horse shop sprites are unavailable: %w", game_state.ErrInvalidMerchantSession)

				return
			}

			dialog = widgets.NewHorseShopDialog(widgets.HorseShopDialogConfig{
				Keyboard:       g.keyboard,
				MerchantSprite: merchantSprite,
				HorseSprite:    horseSprite,
				State:          state,
				Prose:          prose,
				Preview:        preview,
			})
			g.keyboard.PrepareForTransition(0)
			workflowState.pushOwnedDialog(&g.dialogStack, dialog)
		}).If(
			func() bool { return setupErr != nil },
			func(errorQ *event_queue.SequentialEventBuilder) {
				logging.Errorf(logging.CategoryConversation, "horse shop setup failed: %v", setupErr)
				errorQ.ThenSequence(g.thenShowConfirmationDialog("Horse service is unavailable."))
			},
		).While(
			func() bool { return dialog != nil && !dialog.IsDismissed() },
			func() {
				if workflowState.horsePurchase != nil {
					purchaseDialog := workflowState.horsePurchase
					if !purchaseDialog.IsDismissed() {
						return
					}
					g.dialogStack.RemoveWidgetInstance(purchaseDialog)
					workflowState.horsePurchase = nil

					request, confirmed := purchaseDialog.ConsumePendingPurchaseRequest()
					if purchaseDialog.IsCancelled() || !confirmed {
						dialog.CancelPurchaseApproval()

						return
					}

					dialog.AcceptPurchaseApproval(request)
					if preview {
						dialog.ApplyPreviewStopped(request)

						return
					}
					result, err := g.gameState.PurchaseHorse(session, request)
					if err != nil {
						logging.Warnf(logging.CategoryConversation, "horse purchase failed: %v", err)
						switch {
						case errors.Is(err, game_state.ErrHorseInsufficientFunds):
							dialog.ApplyFundsRefusal()
						case errors.Is(err, game_state.ErrHorseStableClosed):
							dialog.ApplyStableClosed()
						default:
							dialog.ApplySessionError("The horse transaction could not be completed.")
						}

						return
					}
					g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundMerchantTransaction)
					dialog.ApplyPurchaseResult(result)

					return
				}

				intent, pending := dialog.ConsumePendingPurchaseIntent()
				if !pending {
					return
				}
				purchaseDialog := widgets.NewHorsePurchaseDialog(widgets.HorsePurchaseDialogConfig{
					Keyboard: g.keyboard,
					Intent:   intent,
				})
				workflowState.horsePurchase = purchaseDialog
				g.keyboard.PrepareForTransition(0)
				workflowState.pushOwnedDialog(&g.dialogStack, purchaseDialog)
			},
			merchantDialogPollingRateMs,
		)
	}
}

func (g *GameScene) thenRunShipShop(
	session game_state.MerchantSession,
	workflowState *merchantTalkWorkflowState,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return g.thenRunShipShopAuthority(session, game_state.ShipShopReview{}, false, workflowState)
}

func (g *GameScene) thenRunShipPreview(
	review game_state.ShipShopReview,
	workflowState *merchantTalkWorkflowState,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return g.thenRunShipShopAuthority(game_state.MerchantSession{}, review, true, workflowState)
}

func (g *GameScene) thenRunShipShopAuthority(
	session game_state.MerchantSession,
	review game_state.ShipShopReview,
	preview bool,
	workflowState *merchantTalkWorkflowState,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		var dialog *widgets.ShipShopDialog
		var state game_state.ShipShopState
		var prose shipShopWorkflowProse
		var acceptedTravel *game_state.HarbourTravelReview
		var setupErr error

		return q.Then(func() {
			state = review.State
			merchantSpriteIndex := review.MerchantSpriteIndex
			if !preview {
				var err error
				state, err = g.gameState.GetShipShopState(session)
				if err != nil {
					setupErr = fmt.Errorf("loading ship shop state: %w", err)

					return
				}
				merchantSpriteIndex = session.MerchantSpriteIndex()
			}
			var err error
			prose, err = g.resolveShipShopProse(state)
			if err != nil {
				setupErr = err

				return
			}
			if g.spriteSheet == nil {
				setupErr = fmt.Errorf("loaded ship shop sprites are unavailable: %w", game_state.ErrInvalidMerchantSession)

				return
			}
			merchantSprite := g.spriteSheet.GetSprite(merchantSpriteIndex)
			frigateSprite := g.spriteSheet.GetSprite(indexes.ShipNoSailsRight)
			skiffSprite := g.spriteSheet.GetSprite(indexes.SkiffRight)
			if merchantSprite == nil || frigateSprite == nil || skiffSprite == nil {
				setupErr = fmt.Errorf("loaded ship shop sprites are unavailable: %w", game_state.ErrInvalidMerchantSession)

				return
			}

			dialog = widgets.NewShipShopDialog(widgets.ShipShopDialogConfig{
				Keyboard:       g.keyboard,
				MerchantSprite: merchantSprite,
				FrigateSprite:  frigateSprite,
				SkiffSprite:    skiffSprite,
				State:          state,
				Prose:          prose.dialog,
				Preview:        preview,
				IsKeyPressed:   g.keyboard.IsKeyHeld,
			})
			g.keyboard.PrepareForTransition(0)
			workflowState.pushOwnedDialog(&g.dialogStack, dialog)
		}).If(
			func() bool { return setupErr != nil },
			func(errorQ *event_queue.SequentialEventBuilder) {
				logging.Errorf(logging.CategoryConversation, "ship shop setup failed: %v", setupErr)
				errorQ.ThenSequence(g.thenShowConfirmationDialog("Ship service is unavailable."))
			},
		).While(
			func() bool { return dialog != nil && !dialog.IsDismissed() && acceptedTravel == nil },
			func() {
				if g.handleShipPurchaseDialog(session, dialog, preview, prose, &state, workflowState) {
					return
				}
				if g.handleShipDispatchDialog(session, dialog, &state, &acceptedTravel, workflowState) {
					return
				}
				if g.handleShipTravelDialog(
					session,
					dialog,
					preview,
					&state,
					&acceptedTravel,
					workflowState,
				) {
					return
				}
				g.openPendingShipDialog(dialog, workflowState)
			},
			merchantDialogPollingRateMs,
		).If(
			func() bool { return acceptedTravel != nil },
			func(travelQ *event_queue.SequentialEventBuilder) {
				travelQ.ThenSequence(g.gameState.ThenHarbourTravelTransition(*acceptedTravel))
			},
		)
	}
}

func (g *GameScene) handleShipPurchaseDialog(
	session game_state.MerchantSession,
	dialog *widgets.ShipShopDialog,
	preview bool,
	prose shipShopWorkflowProse,
	state *game_state.ShipShopState,
	workflowState *merchantTalkWorkflowState,
) bool {
	purchaseDialog := workflowState.shipPurchase
	if purchaseDialog == nil {
		return false
	}
	if !purchaseDialog.IsDismissed() {
		return true
	}
	g.dialogStack.RemoveWidgetInstance(purchaseDialog)
	workflowState.shipPurchase = nil

	request, confirmed := purchaseDialog.ConsumePendingPurchaseRequest()
	if purchaseDialog.IsCancelled() || !confirmed {
		dialog.CancelPurchaseApproval()

		return true
	}
	dialog.AcceptPurchaseApproval(request)
	if preview {
		dialog.ApplyPreviewStopped(request)

		return true
	}
	result, err := g.gameState.PurchaseShip(session, request)
	if err != nil {
		g.applyShipPurchaseRefusal(session, dialog, request, prose, state, err)

		return true
	}
	g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundMerchantTransaction)
	refreshed, err := g.gameState.GetShipShopState(session)
	if err != nil {
		logging.Errorf(logging.CategoryConversation, "ship purchase refresh failed: %v", err)
		dialog.ApplySessionError("The shipwright's manifest could not be refreshed.")

		return true
	}
	*state = refreshed
	dialog.ApplyPurchaseResult(result, refreshed, resolveShipPurchaseResultDisplay(prose, result))
	if result.Action == game_state.ShipPurchaseCreateFrigate ||
		result.Action == game_state.ShipPurchaseCreateStandaloneSkiff {
		dispatchDialog := widgets.NewShipDispatchDialog(widgets.ShipDispatchDialogConfig{
			Keyboard: g.keyboard,
			State:    refreshed,
		})
		workflowState.shipDispatch = dispatchDialog
		g.keyboard.PrepareForTransition(0)
		workflowState.pushOwnedDialog(&g.dialogStack, dispatchDialog)
	}

	return true
}

func (g *GameScene) handleShipDispatchDialog(
	session game_state.MerchantSession,
	dialog *widgets.ShipShopDialog,
	state *game_state.ShipShopState,
	acceptedTravel **game_state.HarbourTravelReview,
	workflowState *merchantTalkWorkflowState,
) bool {
	dispatchDialog := workflowState.shipDispatch
	if dispatchDialog == nil {
		return false
	}
	if !dispatchDialog.IsDismissed() {
		return true
	}
	g.dialogStack.RemoveWidgetInstance(dispatchDialog)
	workflowState.shipDispatch = nil

	if intent, travel := dispatchDialog.ConsumePendingTravelIntent(); travel {
		dialog.AcceptDispatchTravel(intent.Review)
		if g.authorizeShipTravel(session, dialog, intent.Review, state, workflowState) {
			reviewed := intent.Review
			*acceptedTravel = &reviewed
		}

		return true
	}
	if dispatchDialog.ShouldContinueShopping() {
		dialog.ContinueShoppingAfterSuccess()

		return true
	}

	return true
}

func (g *GameScene) handleShipTravelDialog(
	session game_state.MerchantSession,
	dialog *widgets.ShipShopDialog,
	preview bool,
	state *game_state.ShipShopState,
	acceptedTravel **game_state.HarbourTravelReview,
	workflowState *merchantTalkWorkflowState,
) bool {
	travelDialog := workflowState.shipTravel
	if travelDialog == nil {
		return false
	}
	if !travelDialog.IsDismissed() {
		return true
	}
	g.dialogStack.RemoveWidgetInstance(travelDialog)
	workflowState.shipTravel = nil

	reviewed, confirmed := travelDialog.ConsumePendingTravelReview()
	if travelDialog.IsCancelled() || !confirmed {
		dialog.CancelTravelApproval()

		return true
	}
	dialog.AcceptTravelApproval(reviewed)
	if preview {
		dialog.ApplyTravelPreviewStopped(reviewed)

		return true
	}
	if g.authorizeShipTravel(session, dialog, reviewed, state, workflowState) {
		*acceptedTravel = &reviewed
	}

	return true
}

func (g *GameScene) openPendingShipDialog(
	dialog *widgets.ShipShopDialog,
	workflowState *merchantTalkWorkflowState,
) {
	if intent, pending := dialog.ConsumePendingPurchaseIntent(); pending {
		purchaseDialog := widgets.NewShipPurchaseDialog(widgets.ShipPurchaseDialogConfig{
			Keyboard: g.keyboard,
			Intent:   intent,
		})
		workflowState.shipPurchase = purchaseDialog
		g.keyboard.PrepareForTransition(0)
		workflowState.pushOwnedDialog(&g.dialogStack, purchaseDialog)

		return
	}
	if intent, pending := dialog.ConsumePendingTravelIntent(); pending {
		travelDialog := widgets.NewShipTravelDialog(widgets.ShipTravelDialogConfig{
			Keyboard: g.keyboard,
			Intent:   intent,
		})
		workflowState.shipTravel = travelDialog
		g.keyboard.PrepareForTransition(0)
		workflowState.pushOwnedDialog(&g.dialogStack, travelDialog)
	}
}

func (g *GameScene) applyShipPurchaseRefusal(
	session game_state.MerchantSession,
	dialog *widgets.ShipShopDialog,
	request game_state.ShipPurchaseRequest,
	prose shipShopWorkflowProse,
	state *game_state.ShipShopState,
	purchaseErr error,
) {
	logging.Warnf(logging.CategoryConversation, "ship purchase refused: %v", purchaseErr)
	if errors.Is(purchaseErr, game_state.ErrShipInsufficientFunds) {
		dialog.ApplyFundsRefusal(resolveShipFundsRefusalDisplay(prose, *state, request))

		return
	}
	refreshed, err := g.gameState.GetShipShopState(session)
	if err != nil {
		logging.Errorf(logging.CategoryConversation, "ship refusal refresh failed: %v", err)
		dialog.ApplySessionError("The shipwright's manifest could not be refreshed.")

		return
	}
	*state = refreshed
	dialog.ApplyPurchaseRefusal(refreshed, resolveShipPurchaseRefusalDisplay(refreshed, request, purchaseErr))
}

func (g *GameScene) authorizeShipTravel(
	session game_state.MerchantSession,
	dialog *widgets.ShipShopDialog,
	review game_state.HarbourTravelReview,
	state *game_state.ShipShopState,
	workflowState *merchantTalkWorkflowState,
) bool {
	if err := g.gameState.RevalidateHarbourTravel(session, review); err != nil {
		logging.Warnf(logging.CategoryConversation, "harbour travel refused: %v", err)
		refreshed, refreshErr := g.gameState.GetShipShopState(session)
		if refreshErr != nil {
			logging.Errorf(logging.CategoryConversation, "harbour refusal refresh failed: %v", refreshErr)
			dialog.ApplySessionError("The harbour manifest could not be refreshed.")

			return false
		}
		*state = refreshed
		dialog.ApplyTravelRefusal(refreshed, widgets.ShipShopResultDisplay{
			MerchantExchange: fmt.Sprintf("\"The harbour route has changed,\" says %s.", refreshed.Merchant.OwnerName),
			Receipt:          "Harbour escort unavailable: arrangements changed. No travel time was charged.",
		})

		return false
	}

	dialog.ApplyTravelCompleted(review)
	g.finishMerchantWorkflow(workflowState)

	return true
}

func (g *GameScene) thenRunGuildShop(
	session game_state.MerchantSession,
	workflowState *merchantTalkWorkflowState,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return g.thenRunGuildShopAuthority(session, game_state.GuildShopReview{}, false, workflowState)
}

func (g *GameScene) thenRunGuildPreview(
	review game_state.GuildShopReview,
	workflowState *merchantTalkWorkflowState,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return g.thenRunGuildShopAuthority(game_state.MerchantSession{}, review, true, workflowState)
}

func (g *GameScene) thenRunGuildShopAuthority(
	session game_state.MerchantSession,
	review game_state.GuildShopReview,
	preview bool,
	workflowState *merchantTalkWorkflowState,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		var dialog *widgets.GuildShopDialog
		var prose widgets.GuildShopDialogProse
		var setupErr error

		return q.Then(func() {
			state := review.State
			merchantSpriteIndex := review.MerchantSpriteIndex
			if !preview {
				var err error
				state, err = g.gameState.GetGuildShopState(session)
				if err != nil {
					setupErr = fmt.Errorf("loading guild shop state: %w", err)

					return
				}
				merchantSpriteIndex = session.MerchantSpriteIndex()
			}
			var proseErr error
			prose, proseErr = g.resolveGuildShopProse(state)
			if proseErr != nil {
				setupErr = proseErr

				return
			}
			merchantSprite := g.spriteSheet.GetSprite(merchantSpriteIndex)
			if merchantSprite == nil {
				setupErr = fmt.Errorf(
					"loaded merchant sprite is unavailable: %w",
					game_state.ErrInvalidMerchantSession,
				)

				return
			}

			dialog = widgets.NewGuildShopDialog(widgets.GuildShopDialogConfig{
				Keyboard:       g.keyboard,
				MerchantSprite: merchantSprite,
				State:          state,
				Prose:          prose,
				Preview:        preview,
				IsKeyPressed:   g.keyboard.IsKeyHeld,
			})
			g.keyboard.PrepareForTransition(0)
			workflowState.pushOwnedDialog(&g.dialogStack, dialog)
		}).If(
			func() bool { return setupErr != nil },
			func(errorQ *event_queue.SequentialEventBuilder) {
				logging.Errorf(logging.CategoryConversation, "guild shop setup failed: %v", setupErr)
				errorQ.ThenSequence(g.thenShowConfirmationDialog("Guild service is unavailable."))
			},
		).While(
			func() bool { return dialog != nil && !dialog.IsDismissed() },
			func() {
				if workflowState.guildPurchase != nil {
					purchaseDialog := workflowState.guildPurchase
					if !purchaseDialog.IsDismissed() {
						return
					}
					g.dialogStack.RemoveWidgetInstance(purchaseDialog)
					workflowState.guildPurchase = nil

					request, confirmed := purchaseDialog.ConsumePendingPurchaseRequest()
					if purchaseDialog.IsCancelled() || !confirmed {
						dialog.CancelQuantitySelection()

						return
					}

					dialog.AcceptQuantitySelection(request)
					if preview {
						dialog.ApplyPreviewStopped(request)

						return
					}
					current, reviewErr := g.gameState.QuoteGuildOffer(session, request.Provision)
					if reviewErr != nil || current != dialog.GetSelectedQuote() {
						dialog.ApplySessionError("The guild transaction could not be completed.")

						return
					}
					result, err := g.gameState.PurchaseGuildOffer(session, request)
					if err != nil {
						logging.Errorf(logging.CategoryConversation, "guild purchase failed: %v", err)
						dialog.ApplySessionError("The guild transaction could not be completed.")

						return
					}
					g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundMerchantTransaction)
					refreshed, err := g.gameState.GetGuildShopState(session)
					if err != nil {
						logging.Errorf(logging.CategoryConversation, "guild refresh failed: %v", err)
						dialog.ApplySessionError("The guild ledger could not be refreshed.")

						return
					}
					display := g.resolveGuildPurchaseResultDisplay(session, result)
					dialog.ApplyPurchaseResult(result, refreshed, display)

					return
				}

				intent, pending := dialog.ConsumePendingPurchaseIntent()
				if !pending {
					return
				}
				purchaseDialog := widgets.NewGuildPurchaseDialog(widgets.GuildPurchaseDialogConfig{
					Keyboard: g.keyboard,
					Quote:    intent.Quote,
					Pitch:    intent.Pitch,
				})
				workflowState.guildPurchase = purchaseDialog
				g.keyboard.PrepareForTransition(0)
				workflowState.pushOwnedDialog(&g.dialogStack, purchaseDialog)
			},
			merchantDialogPollingRateMs,
		)
	}
}

func (g *GameScene) thenRunHealerShop(
	session game_state.MerchantSession,
	workflowState *merchantTalkWorkflowState,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return g.thenRunHealerShopAuthority(session, game_state.HealerShopReview{}, false, workflowState)
}

func (g *GameScene) thenRunHealerPreview(
	review game_state.HealerShopReview,
	workflowState *merchantTalkWorkflowState,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return g.thenRunHealerShopAuthority(game_state.MerchantSession{}, review, true, workflowState)
}

func (g *GameScene) thenRunHealerShopAuthority(
	session game_state.MerchantSession,
	review game_state.HealerShopReview,
	preview bool,
	workflowState *merchantTalkWorkflowState,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		var dialog *widgets.HealerShopDialog
		var state game_state.HealerShopState
		var prose widgets.HealerShopDialogProse
		var setupErr error

		return q.Then(func() {
			state = review.State
			merchantSpriteIndex := review.MerchantSpriteIndex
			if !preview {
				var err error
				state, err = g.gameState.GetHealerShopState(session)
				if err != nil {
					setupErr = fmt.Errorf("loading healer shop state: %w", err)

					return
				}
				merchantSpriteIndex = session.MerchantSpriteIndex()
			}
			var proseErr error
			prose, proseErr = g.resolveHealerShopProse(state)
			if proseErr != nil {
				setupErr = proseErr

				return
			}
			merchantSprite := g.spriteSheet.GetSprite(merchantSpriteIndex)
			if merchantSprite == nil {
				setupErr = fmt.Errorf(
					"loaded merchant sprite is unavailable: %w",
					game_state.ErrInvalidMerchantSession,
				)

				return
			}

			dialog = widgets.NewHealerShopDialog(widgets.HealerShopDialogConfig{
				Keyboard:       g.keyboard,
				MerchantSprite: merchantSprite,
				State:          state,
				Prose:          prose,
				Preview:        preview,
			})
			g.keyboard.PrepareForTransition(0)
			workflowState.pushOwnedDialog(&g.dialogStack, dialog)
		}).If(
			func() bool { return setupErr != nil },
			func(errorQ *event_queue.SequentialEventBuilder) {
				logging.Errorf(logging.CategoryConversation, "healer shop setup failed: %v", setupErr)
				errorQ.ThenSequence(g.thenShowConfirmationDialog("Healer service is unavailable."))
			},
		).While(
			func() bool { return dialog != nil && !dialog.IsDismissed() },
			func() {
				if workflowState.healerTreatment != nil {
					treatmentDialog := workflowState.healerTreatment
					if !treatmentDialog.IsDismissed() {
						return
					}
					g.dialogStack.RemoveWidgetInstance(treatmentDialog)
					workflowState.healerTreatment = nil
					request, confirmed := treatmentDialog.ConsumePendingRequest()
					if treatmentDialog.IsCancelled() || !confirmed {
						dialog.CancelTreatmentApproval()

						return
					}
					dialog.AcceptTreatmentApproval(request)
					g.commitHealerTreatment(session, dialog, request, prose, &state, preview)

					return
				}

				intent, pending := dialog.ConsumePendingTreatmentIntent()
				if !pending {
					return
				}
				treatmentDialog := widgets.NewHealerTreatmentDialog(widgets.HealerTreatmentDialogConfig{
					Keyboard: g.keyboard,
					Intent:   intent,
				})
				workflowState.healerTreatment = treatmentDialog
				g.keyboard.PrepareForTransition(0)
				workflowState.pushOwnedDialog(&g.dialogStack, treatmentDialog)
			},
			merchantDialogPollingRateMs,
		)
	}
}

func (g *GameScene) commitHealerTreatment(
	session game_state.MerchantSession,
	dialog *widgets.HealerShopDialog,
	request game_state.HealerServiceRequest,
	prose widgets.HealerShopDialogProse,
	state *game_state.HealerShopState,
	preview bool,
) {
	if preview {
		dialog.ApplyPreviewStopped(request)

		return
	}
	current, reviewErr := g.gameState.GetHealerShopState(session)
	if reviewErr != nil || !reflect.DeepEqual(current, *state) {
		dialog.ApplySessionError("The healer's records changed while you browsed. Leave and speak again.")

		return
	}
	if request.CharacterIndex == game_state.HealerAllRecipientsIndex {
		g.applyHealerAllService(session, dialog, request, prose, state)

		return
	}
	result, err := g.gameState.ProvideHealerService(session, request)
	if err != nil {
		g.applyHealerRefusal(session, dialog, request, prose, state, err)

		return
	}
	refreshed, err := g.gameState.GetHealerShopState(session)
	if err != nil {
		logging.Errorf(logging.CategoryConversation, "healer refresh failed: %v", err)
		dialog.ApplySessionError("The healer's records could not be refreshed.")

		return
	}
	*state = refreshed
	dialog.ApplyServiceResult(result, refreshed, resolveHealerResultDisplay(state.Merchant, result))
	g.triggerHealerServiceFeedback(result)
}

func (g *GameScene) applyHealerAllService(
	session game_state.MerchantSession,
	dialog *widgets.HealerShopDialog,
	request game_state.HealerServiceRequest,
	prose widgets.HealerShopDialogProse,
	state *game_state.HealerShopState,
) {
	result, err := g.gameState.ProvideHealerServiceToAll(session, request.Service)
	if err != nil {
		g.applyHealerRefusal(session, dialog, request, prose, state, err)

		return
	}
	refreshed, err := g.gameState.GetHealerShopState(session)
	if err != nil {
		logging.Errorf(logging.CategoryConversation, "aggregate healer refresh failed: %v", err)
		dialog.ApplySessionError("The healer's records could not be refreshed.")

		return
	}
	*state = refreshed
	dialog.ApplyAllServiceResult(result, refreshed, resolveHealerAllResultDisplay(state.Merchant, result))
	g.triggerHealerServiceFeedbackForRecipients(result.Service, result.CharacterIndexes)
}

func (g *GameScene) applyHealerRefusal(
	session game_state.MerchantSession,
	dialog *widgets.HealerShopDialog,
	request game_state.HealerServiceRequest,
	prose widgets.HealerShopDialogProse,
	state *game_state.HealerShopState,
	serviceErr error,
) {
	logging.Warnf(logging.CategoryConversation, "healer service refused: %v", serviceErr)
	if !errors.Is(serviceErr, game_state.ErrHealerServiceUnneeded) &&
		!errors.Is(serviceErr, game_state.ErrHealerInsufficientFunds) {
		dialog.ApplySessionError("The healer service could not be completed.")

		return
	}
	refreshed, err := g.gameState.GetHealerShopState(session)
	if err != nil {
		logging.Errorf(logging.CategoryConversation, "healer refusal refresh failed: %v", err)
		dialog.ApplySessionError("The healer's records could not be refreshed.")

		return
	}
	*state = refreshed
	display := resolveHealerRefusalDisplay(refreshed, request, prose, serviceErr)
	dialog.ApplyRefusal(refreshed, request, display)
}

func (g *GameScene) triggerHealerServiceFeedback(result game_state.HealerServiceResult) {
	g.triggerHealerServiceFeedbackForRecipients(result.Service, []int{result.CharacterIndex})
}

func (g *GameScene) triggerHealerServiceFeedbackForRecipients(
	service references.HealerService,
	characterIndexes []int,
) {
	sound := game_state.SoundHeal
	switch service {
	case references.HealerCure:
		sound = game_state.SoundCure
	case references.HealerHeal:
		sound = game_state.SoundHeal
	case references.HealerResurrect:
		sound = game_state.SoundResurrect
	default:
		panic("triggerHealerServiceFeedback: unknown service")
	}
	g.gameState.EventQueueManager.TriggerFeedback(
		"healer_service_feedback",
		event_queue.PriorityNormal,
		func(sequence *event_queue.SequentialEventBuilder) {
			sequence.First(func() {}).ThenSequence(
				game_state.NewPartyStateBuilder(g.gameState).
					ToCharacters(characterIndexes).
					FlashInverted().
					WithSound(sound).
					Build(),
			)
		},
	)
}

func (g *GameScene) thenRunInnShop(
	session game_state.MerchantSession,
	workflowState *merchantTalkWorkflowState,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return g.thenRunInnShopAuthority(session, game_state.InnShopReview{}, false, workflowState)
}

func (g *GameScene) thenRunInnPreview(
	review game_state.InnShopReview,
	workflowState *merchantTalkWorkflowState,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return g.thenRunInnShopAuthority(game_state.MerchantSession{}, review, true, workflowState)
}

func (g *GameScene) thenRunInnShopAuthority(
	session game_state.MerchantSession,
	review game_state.InnShopReview,
	preview bool,
	workflowState *merchantTalkWorkflowState,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		var dialog *widgets.InnShopDialog
		var state game_state.InnShopState
		var prose innShopWorkflowProse
		var terminalMessage string
		var awaitRestFeedback bool
		var setupErr error

		return q.Then(func() {
			state = review.State
			merchantSpriteIndex := review.MerchantSpriteIndex
			if !preview {
				var err error
				state, err = g.gameState.GetInnShopState(session)
				if err != nil {
					setupErr = fmt.Errorf("loading Inn shop state: %w", err)

					return
				}
				merchantSpriteIndex = session.MerchantSpriteIndex()
			}
			var err error
			prose, err = g.resolveInnShopProse(state)
			if err != nil {
				setupErr = err

				return
			}
			if g.spriteSheet == nil {
				setupErr = fmt.Errorf("loaded Inn merchant sprite is unavailable: %w", game_state.ErrInvalidMerchantSession)

				return
			}
			merchantSprite := g.spriteSheet.GetSprite(merchantSpriteIndex)
			if merchantSprite == nil {
				setupErr = fmt.Errorf("loaded Inn merchant sprite is unavailable: %w", game_state.ErrInvalidMerchantSession)

				return
			}

			dialog = widgets.NewInnShopDialog(widgets.InnShopDialogConfig{
				Keyboard:       g.keyboard,
				MerchantSprite: merchantSprite,
				State:          state,
				Prose:          prose.dialog,
				Preview:        preview,
				IsKeyPressed:   g.keyboard.IsKeyHeld,
			})
			g.keyboard.PrepareForTransition(0)
			workflowState.pushOwnedDialog(&g.dialogStack, dialog)
		}).If(
			func() bool { return setupErr != nil },
			func(errorQ *event_queue.SequentialEventBuilder) {
				logging.Errorf(logging.CategoryConversation, "Inn shop setup failed: %v", setupErr)
				errorQ.ThenSequence(g.thenShowConfirmationDialog("Inn service is unavailable."))
			},
		).While(
			func() bool { return dialog != nil && !dialog.IsDismissed() },
			func() {
				if workflowState.innAction != nil {
					actionDialog := workflowState.innAction
					if !actionDialog.IsDismissed() {
						return
					}
					g.dialogStack.RemoveWidgetInstance(actionDialog)
					workflowState.innAction = nil

					request, confirmed := actionDialog.ConsumePendingRequest()
					if actionDialog.IsCancelled() || !confirmed {
						dialog.CancelActionApproval()

						return
					}
					dialog.AcceptActionApproval(request)
					if preview {
						dialog.PreviewStop(request)

						return
					}

					switch request.Action {
					case widgets.InnActionRest:
						terminalMessage, awaitRestFeedback = g.commitInnRest(session, dialog, request, prose, &state)
					case widgets.InnActionLeave:
						g.commitInnLeave(session, dialog, request, prose, &state)
					case widgets.InnActionPickup:
						terminalMessage = g.commitInnPickup(session, dialog, request, prose, &state)
					default:
						panic("Inn workflow received an unknown action")
					}

					return
				}

				intent, pending := dialog.ConsumePendingActionIntent()
				if !pending {
					return
				}
				actionDialog := widgets.NewInnActionDialog(widgets.InnActionDialogConfig{
					Keyboard:     g.keyboard,
					Intent:       intent,
					IsKeyPressed: g.keyboard.IsKeyHeld,
				})
				workflowState.innAction = actionDialog
				g.keyboard.PrepareForTransition(0)
				workflowState.pushOwnedDialog(&g.dialogStack, actionDialog)
			},
			merchantDialogPollingRateMs,
		).If(
			func() bool { return awaitRestFeedback },
			func(restQ *event_queue.SequentialEventBuilder) {
				restQ.AwaitFeedback("inn_rest_feedback", event_queue.PriorityNormal, g.buildInnRestFeedback)
			},
		).If(
			func() bool { return terminalMessage != "" },
			func(terminalQ *event_queue.SequentialEventBuilder) {
				terminalQ.ThenSequence(g.thenShowInnTerminalDialog(terminalMessage))
			},
		).If(
			func() bool {
				return !preview && dialog != nil && g.gameState.PartyState.IsAllPartyDead()
			},
			func(deathQ *event_queue.SequentialEventBuilder) {
				deathQ.Then(func() {
					// The original Talk minute is skipped at a wipe boundary; death owns the next workflow.
					workflowState.advanceTime = false
				}).HandoffTo(deathResurrectionWorkflowName, event_queue.PriorityCritical, g.buildDeathResurrectionWorkflow)
			},
		)
	}
}

func (g *GameScene) commitInnRest(
	session game_state.MerchantSession,
	dialog *widgets.InnShopDialog,
	request widgets.InnActionRequest,
	prose innShopWorkflowProse,
	state *game_state.InnShopState,
) (string, bool) {
	result, err := g.gameState.RestAtInn(session, request.Rest)
	if err != nil {
		logging.Warnf(logging.CategoryConversation, "Inn Rest refused: %v", err)
		if errors.Is(err, game_state.ErrInnRestUnavailable) {
			dialog.ApplySessionError(prose.restFundsRefusal)
			dialog.Dismiss()

			return prose.restFundsRefusal + "\n\n" + widgets.FormatInnReceipt(widgets.InnActionResultDisplay{
				Action: widgets.InnActionRest, Refused: true, ListedCharge: request.Rest.ReviewedCharge,
				MerchantExchange: prose.restFundsRefusal, FaulineiTheft: 0, FinalGold: state.Gold,
				PartySize: state.PartySize, RoomOccupancy: state.RoomOccupancy, CharacterName: "", Members: nil,
			}), false
		}
		dialog.ApplySessionError("The Inn stay could not be completed.")

		return "", false
	}
	// Authority has already advanced to morning; cover the map before the
	// dialog closes so the wake location is revealed only by the fade-in.
	g.gameState.SystemCallbacks.Visual.SetMapFadeLevel(1)
	resultExchange := innRestResultExchange(prose.politeFarewell, state.Merchant, result)
	display := widgets.InnActionResultDisplay{
		Action:           widgets.InnActionRest,
		MerchantExchange: resultExchange,
		ListedCharge:     result.Charge,
		FaulineiTheft:    result.Theft,
		FinalGold:        result.GoldAfterTheft,
		PartySize:        state.PartySize,
		RoomOccupancy:    state.RoomOccupancy,
		Refused:          false,
		CharacterName:    "",
		Members:          result.Members,
	}
	if g.gameState.PartyState.IsAllPartyDead() {
		// A pure post-Rest review has no eligible buyer at this terminal boundary.
		dialog.ApplySessionError(resultExchange)
		dialog.Dismiss()

		return resultExchange + "\n\n" + widgets.FormatInnReceipt(display), true
	}
	refreshedReview, err := g.gameState.ReviewInnShopByMerchantID(state.Merchant.ID)
	if err != nil {
		logging.Errorf(logging.CategoryConversation, "Inn Rest refresh failed: %v", err)
		g.gameState.SystemCallbacks.Visual.SetMapFadeLevel(0)
		dialog.ApplySessionError("The Inn register could not be refreshed.")

		return "", false
	}
	refreshed := refreshedReview.State
	*state = refreshed
	display.PartySize = refreshed.PartySize
	display.RoomOccupancy = refreshed.RoomOccupancy
	dialog.ApplyResult(request, refreshed, display)
	message := dialog.GetLatestExchange() + "\n\n" + dialog.GetReceiptText()
	dialog.Dismiss()

	return message, true
}

func innRestResultExchange(
	defaultExchange string,
	merchant references.MerchantDefinition,
	result game_state.InnRestResult,
) string {
	deaths := make([]string, 0)
	for _, member := range result.Members {
		if member.StatusBefore == party_state.Poisoned && member.StatusAfter == party_state.Dead {
			deaths = append(deaths, member.Identity.Name)
		}
	}
	if len(deaths) == 0 {
		return defaultExchange
	}

	return fmt.Sprintf(
		"\"Morning. %s passed away during the night,\" says %s.",
		strings.Join(deaths, ", "),
		merchant.OwnerName,
	)
}

func (g *GameScene) commitInnLeave(
	session game_state.MerchantSession,
	dialog *widgets.InnShopDialog,
	request widgets.InnActionRequest,
	prose innShopWorkflowProse,
	state *game_state.InnShopState,
) {
	result, err := g.gameState.LeaveCompanionAtInn(session, request.Leave)
	if err != nil {
		logging.Warnf(logging.CategoryConversation, "Inn Leave failed: %v", err)
		dialog.ApplySessionError("The companion could not be left at this Inn.")

		return
	}
	*state = result.State
	dialog.ApplyResult(request, result.State, widgets.InnActionResultDisplay{
		Action:           widgets.InnActionLeave,
		MerchantExchange: prose.leaveAcknowledgment,
		ListedCharge:     0,
		FaulineiTheft:    0,
		FinalGold:        result.GoldAfter,
		PartySize:        result.PartySizeAfter,
		RoomOccupancy:    result.RoomOccupancyAfter,
		Refused:          false,
		CharacterName:    result.Character.Name,
		Members:          nil,
	})
}

func (g *GameScene) commitInnPickup(
	session game_state.MerchantSession,
	dialog *widgets.InnShopDialog,
	request widgets.InnActionRequest,
	prose innShopWorkflowProse,
	state *game_state.InnShopState,
) string {
	result, err := g.gameState.PickUpInnGuest(session, request.Pickup)
	if err != nil {
		logging.Warnf(logging.CategoryConversation, "Inn Pick Up failed: %v", err)
		dialog.ApplySessionError("The guest could not be picked up from this Inn.")

		return ""
	}
	*state = result.State
	exchange := prose.politeFarewell
	if result.Projection.StatusBefore == party_state.Poisoned && result.Projection.StatusAfter == party_state.Dead {
		exchange = fmt.Sprintf("\"Thy friend %s has died, by the way,\" says %s.", result.Character.Name, state.Merchant.OwnerName)
	}
	display := widgets.InnActionResultDisplay{
		Action:           widgets.InnActionPickup,
		MerchantExchange: exchange,
		ListedCharge:     result.ListedCharge,
		FaulineiTheft:    result.Theft,
		FinalGold:        result.FinalGold,
		PartySize:        result.State.PartySize,
		RoomOccupancy:    result.State.RoomOccupancy,
		Refused:          result.Outcome == game_state.InnPickupRefusedWithGuards,
		CharacterName:    result.Character.Name,
		Members:          []game_state.InnCharacterProjection{result.Projection},
	}
	if result.Outcome == game_state.InnPickupRefusedWithGuards {
		display.MerchantExchange = prose.pickupGuardRefusal
	}
	dialog.ApplyResult(request, result.State, display)
	if result.Outcome == game_state.InnPickupRefusedWithGuards {
		message := dialog.GetLatestExchange() + "\n\n" + dialog.GetReceiptText()
		dialog.Dismiss()

		return message
	}
	g.triggerInnPickupFeedback(result)

	return ""
}

func (g *GameScene) buildInnRestFeedback(sequence *event_queue.SequentialEventBuilder) {
	sequence.First(func() {
		g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundGoodNight)
	}).Wait(game_state.BedRestFadeToBlackMs).Then(func() {
		g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundRoosterCrow)
	}).Animate(game_state.BedRestFadeFromBlackMs, func(progress float32) {
		g.gameState.SystemCallbacks.Visual.SetMapFadeLevel(1 - float64(progress))
	}).Finally(func() {
		g.gameState.SystemCallbacks.Visual.SetMapFadeLevel(0)
	})
}

func (g *GameScene) triggerInnPickupFeedback(result game_state.InnPickupResult) {
	sound := game_state.SoundHeal
	if result.Projection.StatusAfter == party_state.Dead {
		sound = game_state.SoundDeath
	}
	g.gameState.EventQueueManager.TriggerFeedback(
		"inn_pickup_feedback",
		event_queue.PriorityNormal,
		func(sequence *event_queue.SequentialEventBuilder) {
			sequence.First(func() {}).ThenSequence(
				game_state.NewPartyStateBuilder(g.gameState).
					ToCharacters([]int{result.CharacterIndexAfter}).
					FlashInverted().
					WithSound(sound).
					Build(),
			)
		},
	)
}

func (g *GameScene) thenShowInnTerminalDialog(
	message string,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		overlay := widgets.NewOverlayDialog(message).
			AsConfirmation().
			WithSizing(widgets.SizingAutoFitDefault).
			WithUserDismissed(g.keyboard).
			AutoPopFromStack(&g.dialogStack).
			Build()

		return q.ThenSequence(g.showOverlayAndWait(overlay))
	}
}

func (g *GameScene) startDebugMerchantTalkWorkflow(id references.MerchantID) {
	workflowState := &merchantTalkWorkflowState{advanceTime: true}
	g.gameState.EventQueueManager.Workflow("merchant_tester_live").
		Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().
		First(func() {}).
		ThenSequence(g.thenHandleMerchantSession(func() (game_state.MerchantSession, error) {
			return g.gameState.BeginDebugMerchantSessionByID(id)
		}, workflowState)).
		Finally(func() { g.finishMerchantWorkflow(workflowState) }).
		Start()
}

func (g *GameScene) startGuildMerchantPreviewWorkflow(review game_state.GuildShopReview) {
	workflowState := new(merchantTalkWorkflowState)
	g.gameState.EventQueueManager.Workflow("merchant_tester_preview").
		Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().
		First(func() {}).
		ThenSequence(g.thenRunGuildPreview(review, workflowState)).
		Finally(func() { g.finishMerchantWorkflow(workflowState) }).
		Start()
}

func (g *GameScene) startArmsMerchantPreviewWorkflow(review game_state.ArmsShopReview) {
	workflowState := new(merchantTalkWorkflowState)
	g.gameState.EventQueueManager.Workflow("merchant_tester_preview").
		Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().
		First(func() {}).
		ThenSequence(g.thenRunArmsPreview(review, workflowState)).
		Finally(func() { g.finishMerchantWorkflow(workflowState) }).
		Start()
}

func (g *GameScene) startHealerMerchantPreviewWorkflow(review game_state.HealerShopReview) {
	workflowState := new(merchantTalkWorkflowState)
	g.gameState.EventQueueManager.Workflow("merchant_tester_preview").
		Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().
		First(func() {}).
		ThenSequence(g.thenRunHealerPreview(review, workflowState)).
		Finally(func() { g.finishMerchantWorkflow(workflowState) }).
		Start()
}

func (g *GameScene) startReagentMerchantPreviewWorkflow(review game_state.ReagentShopReview) {
	workflowState := new(merchantTalkWorkflowState)
	g.gameState.EventQueueManager.Workflow("merchant_tester_preview").
		Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().
		First(func() {}).
		ThenSequence(g.thenRunReagentPreview(review, workflowState)).
		Finally(func() { g.finishMerchantWorkflow(workflowState) }).
		Start()
}

func (g *GameScene) startHorseMerchantPreviewWorkflow(review game_state.HorseShopReview) {
	workflowState := new(merchantTalkWorkflowState)
	g.gameState.EventQueueManager.Workflow("merchant_tester_preview").
		Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().
		First(func() {}).
		ThenSequence(g.thenRunHorsePreview(review, workflowState)).
		Finally(func() { g.finishMerchantWorkflow(workflowState) }).
		Start()
}

func (g *GameScene) startShipMerchantPreviewWorkflow(review game_state.ShipShopReview) {
	workflowState := new(merchantTalkWorkflowState)
	g.gameState.EventQueueManager.Workflow("merchant_tester_preview").
		Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().
		First(func() {}).
		ThenSequence(g.thenRunShipPreview(review, workflowState)).
		Finally(func() { g.finishMerchantWorkflow(workflowState) }).
		Start()
}

func (g *GameScene) startInnMerchantPreviewWorkflow(review game_state.InnShopReview) {
	workflowState := new(merchantTalkWorkflowState)
	g.gameState.EventQueueManager.Workflow("merchant_tester_preview").
		Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().
		First(func() {}).
		ThenSequence(g.thenRunInnPreview(review, workflowState)).
		Finally(func() { g.finishMerchantWorkflow(workflowState) }).
		Start()
}

func (g *GameScene) startTavernMerchantPreviewWorkflow(review game_state.TavernShopReview) {
	workflowState := new(merchantTalkWorkflowState)
	g.gameState.EventQueueManager.Workflow("merchant_tester_preview").
		Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().
		First(func() {}).
		ThenSequence(g.thenRunTavernPreview(review, workflowState)).
		Finally(func() { g.finishMerchantWorkflow(workflowState) }).
		Start()
}

func (g *GameScene) finishMerchantWorkflow(workflowState *merchantTalkWorkflowState) {
	if workflowState == nil || workflowState.finished {
		return
	}
	workflowState.finished = true
	workflowState.dismissAndRemoveOwnedDialogs(&g.dialogStack)
	if workflowState.interactionTranscript != nil {
		g.dialogStack.RemoveWidgetInstance(workflowState.interactionTranscript)
		workflowState.interactionTranscript = nil
	}
	if workflowState.sessionStarted {
		g.gameState.EndMerchantSession(workflowState.session)
		workflowState.sessionStarted = false
	}
	g.keyboard.ResetReleaseDebouncing()
	if workflowState.advanceTime {
		g.gameState.AdvanceGameplayMinutes(1)
	}
	g.gameState.ProcessPendingProvisionHours()
}

func (g *GameScene) resolveArmsShopProse(
	state game_state.ArmsShopState,
) (widgets.ArmsShopDialogProse, error) {
	if g.gameState == nil || g.gameState.GameReferences == nil ||
		g.gameState.GameReferences.MerchantReferences == nil {
		return widgets.ArmsShopDialogProse{}, fmt.Errorf(
			"resolving Arms prose: merchant references are unavailable: %w",
			game_state.ErrInvalidMerchantSession,
		)
	}
	timeOfDay := merchantTimeOfDay(g.gameState.DateTime.Hour)

	return widgets.ArmsShopDialogProse{
		Greeting: quoteMerchantConversation(fmt.Sprintf(
			"Good %s, and welcome to %s! Buy or Sell?",
			timeOfDay,
			state.Merchant.ShopName,
		)),
		BuyPrompt:       state.Merchant.OwnerName + " asks, \"What may I show thee?\"",
		SellPrompt:      state.Merchant.OwnerName + " asks, \"What dost thou wish to sell?\"",
		CapacityRefusal: fmt.Sprintf("\"Thou canst not carry any more!\" says %s.", state.Merchant.OwnerName),
		FundsRefusal:    fmt.Sprintf("\"Can't pay?! Out with thee!\" yells %s.", state.Merchant.OwnerName),
	}, nil
}

func (g *GameScene) resolveInnShopProse(
	state game_state.InnShopState,
) (innShopWorkflowProse, error) {
	if g.gameState == nil || g.gameState.GameReferences == nil ||
		g.gameState.GameReferences.MerchantReferences == nil {
		return innShopWorkflowProse{}, fmt.Errorf(
			"resolving Inn prose: merchant references are unavailable: %w",
			game_state.ErrInvalidMerchantSession,
		)
	}
	merchantReferences := g.gameState.GameReferences.MerchantReferences
	values := references.MerchantTextValues{
		OwnerName:    state.Merchant.OwnerName,
		ShopName:     state.Merchant.ShopName,
		ItemName:     "room",
		Price:        int(state.Rest.Charge),
		Quantity:     state.PartySize,
		LocationName: state.Merchant.Location.String(),
		TimeOfDay:    merchantTimeOfDay(g.gameState.DateTime.Hour),
	}
	// REDUX DEVIATION: MERCH-0009: original greeting and farewell select Inn prose with
	// gameplay RNG. Redux maps cosmetic variants from canonical merchant identity
	// so Preview remains pure and gameplay RNG is reserved for Inn authority.
	baseIndex := deterministicMerchantProseIndex(state.Merchant.ID, 4)
	greetingID := references.MerchantTextID("INN_TXT_" + strconv.Itoa(baseIndex))
	politeID := references.MerchantTextID("POLITE_INN_" + strconv.Itoa((baseIndex+2)%4))
	greeting, err := merchantReferences.FormatText(greetingID, values)
	if err != nil {
		return innShopWorkflowProse{}, fmt.Errorf("formatting Inn greeting %q: %w", greetingID, err)
	}
	polite, err := merchantReferences.FormatText(politeID, values)
	if err != nil {
		return innShopWorkflowProse{}, fmt.Errorf("formatting Inn farewell %q: %w", politeID, err)
	}
	pickupRefusal, err := merchantReferences.FormatText(references.MerchantTextID("INN_MISC_2"), values)
	if err != nil {
		return innShopWorkflowProse{}, fmt.Errorf("formatting Inn guard refusal: %w", err)
	}

	return innShopWorkflowProse{
		dialog: widgets.InnShopDialogProse{
			Greeting:     quoteMerchantConversation(greeting),
			RestPrompt:   state.Merchant.OwnerName + " asks, \"Wilt thou Rest for the night?\"",
			LeavePrompt:  state.Merchant.OwnerName + " asks, \"Who will stay?\"",
			PickupPrompt: state.Merchant.OwnerName + " asks, \"Whom wilt thou Pick up?\"",
		},
		politeFarewell: quoteMerchantConversation(polite),
		restFundsRefusal: fmt.Sprintf(
			"\"Highwaymen! Cheap, at that! OUT!\" screams %s.",
			state.Merchant.OwnerName,
		),
		pickupGuardRefusal:  quoteMerchantConversation(pickupRefusal),
		leaveAcknowledgment: fmt.Sprintf("\"I thank thee.\" says %s.", state.Merchant.OwnerName),
	}, nil
}

func (g *GameScene) resolveHealerShopProse(
	state game_state.HealerShopState,
) (widgets.HealerShopDialogProse, error) {
	if g.gameState == nil || g.gameState.GameReferences == nil ||
		g.gameState.GameReferences.MerchantReferences == nil {
		return widgets.HealerShopDialogProse{}, fmt.Errorf(
			"resolving healer prose: merchant references are unavailable: %w",
			game_state.ErrInvalidMerchantSession,
		)
	}
	merchantReferences := g.gameState.GameReferences.MerchantReferences
	values := references.MerchantTextValues{
		OwnerName:     state.Merchant.OwnerName,
		ShopName:      state.Merchant.ShopName,
		Price:         int(state.Services[references.HealerCure].SourcePrice),
		Quantity:      1,
		ItemName:      references.HealerCure.String(),
		LocationName:  state.Merchant.Location.String(),
		TimeOfDay:     merchantTimeOfDay(g.gameState.DateTime.Hour),
		InformantName: state.PricingCharacterName,
	}
	// REDUX DEVIATION: MERCH-0007: original healer prose selects one of four variants with gameplay RNG; Redux maps cosmetic prose deterministically by merchant identity so Preview and gameplay RNG remain isolated.
	greetingID := references.MerchantTextID(
		"HEAL_TXT_" + strconv.Itoa(deterministicMerchantProseIndex(state.Merchant.ID, 4)),
	)
	greeting, err := merchantReferences.FormatText(greetingID, values)
	if err != nil {
		return widgets.HealerShopDialogProse{}, fmt.Errorf("formatting healer greeting %q: %w", greetingID, err)
	}
	fundsRefusal, err := merchantReferences.FormatText(references.MerchantTextID("HEAL_MISC_0"), values)
	if err != nil {
		return widgets.HealerShopDialogProse{}, fmt.Errorf("formatting healer funds refusal: %w", err)
	}

	return widgets.HealerShopDialogProse{
		Greeting:     quoteMerchantConversation(greeting),
		NoNeed:       quoteMerchantConversation("Thou hast no need of this art!"),
		FundsRefusal: quoteMerchantConversation(fundsRefusal),
	}, nil
}

func (g *GameScene) resolveReagentShopProse(
	state game_state.ReagentShopState,
) (widgets.ReagentShopDialogProse, error) {
	if g.gameState == nil || g.gameState.GameReferences == nil ||
		g.gameState.GameReferences.MerchantReferences == nil {
		return widgets.ReagentShopDialogProse{}, fmt.Errorf(
			"resolving reagent prose: merchant references are unavailable: %w",
			game_state.ErrInvalidMerchantSession,
		)
	}
	values := references.MerchantTextValues{
		OwnerName:    state.Merchant.OwnerName,
		ShopName:     state.Merchant.ShopName,
		LocationName: state.Merchant.Location.String(),
		TimeOfDay:    merchantTimeOfDay(g.gameState.DateTime.Hour),
	}
	// REDUX DEVIATION: MERCH-0005: original reagent greeting/farewell draws a gameplay value from 0..3; Redux maps the four greetings deterministically by merchant identity so cosmetic prose and Preview never consume gameplay RNG.
	greetingID := references.MerchantTextID(
		"REAG_TXT_" + strconv.Itoa(deterministicMerchantProseIndex(state.Merchant.ID, 4)),
	)
	greeting, err := g.gameState.GameReferences.MerchantReferences.FormatText(greetingID, values)
	if err != nil {
		return widgets.ReagentShopDialogProse{}, fmt.Errorf("formatting reagent greeting %q: %w", greetingID, err)
	}
	rudeFarewellID := references.MerchantTextID(
		"RUDE_REAG_" + strconv.Itoa(deterministicReagentFarewellProseIndex(state.Merchant.ID, false)),
	)
	rudeFarewell, err := g.gameState.GameReferences.MerchantReferences.FormatText(rudeFarewellID, values)
	if err != nil {
		return widgets.ReagentShopDialogProse{}, fmt.Errorf(
			"formatting reagent rude farewell %q: %w", rudeFarewellID, err,
		)
	}
	politeFarewellID := references.MerchantTextID(
		"POLITE_REAG_" + strconv.Itoa(deterministicReagentFarewellProseIndex(state.Merchant.ID, true)),
	)
	politeFarewell, err := g.gameState.GameReferences.MerchantReferences.FormatText(politeFarewellID, values)
	if err != nil {
		return widgets.ReagentShopDialogProse{}, fmt.Errorf(
			"formatting reagent polite farewell %q: %w", politeFarewellID, err,
		)
	}

	return widgets.ReagentShopDialogProse{
		Greeting:       quoteMerchantConversation(greeting),
		RudeFarewell:   quoteMerchantConversation(rudeFarewell),
		PoliteFarewell: quoteMerchantConversation(politeFarewell),
	}, nil
}

func (g *GameScene) resolveHorseShopProse(
	state game_state.HorseShopState,
) (widgets.HorseShopDialogProse, error) {
	if g.gameState == nil || g.gameState.GameReferences == nil ||
		g.gameState.GameReferences.MerchantReferences == nil {
		return widgets.HorseShopDialogProse{}, fmt.Errorf(
			"resolving horse prose: merchant references are unavailable: %w",
			game_state.ErrInvalidMerchantSession,
		)
	}
	merchantReferences := g.gameState.GameReferences.MerchantReferences
	values := references.MerchantTextValues{
		OwnerName:    state.Merchant.OwnerName,
		ShopName:     state.Merchant.ShopName,
		ItemName:     merchantHorseSemanticName,
		Price:        state.Quote.Price,
		Quantity:     1,
		LocationName: state.Merchant.Location.String(),
		TimeOfDay:    merchantTimeOfDay(g.gameState.DateTime.Hour),
	}
	// REDUX DEVIATION: MERCH-0008: original horse greeting/farewell selects prose with
	// a gameplay draw from 0..3; Redux maps cosmetic variants from stable merchant
	// identity so Preview remains pure and gameplay RNG is reserved for Faulinei.
	baseIndex := deterministicMerchantProseIndex(state.Merchant.ID, 4)
	greetingID := references.MerchantTextID("HORSE_TXT_" + strconv.Itoa(baseIndex))
	rudeFarewellID := references.MerchantTextID("RUDE_HORSE_" + strconv.Itoa((baseIndex+1)%4))
	politeFarewellID := references.MerchantTextID("POLITE_HORSE_" + strconv.Itoa((baseIndex+2)%4))
	greeting, err := merchantReferences.FormatText(greetingID, values)
	if err != nil {
		return widgets.HorseShopDialogProse{}, fmt.Errorf("formatting horse greeting %q: %w", greetingID, err)
	}
	rudeFarewell, err := merchantReferences.FormatText(rudeFarewellID, values)
	if err != nil {
		return widgets.HorseShopDialogProse{}, fmt.Errorf(
			"formatting horse rude farewell %q: %w",
			rudeFarewellID,
			err,
		)
	}
	politeFarewell, err := merchantReferences.FormatText(politeFarewellID, values)
	if err != nil {
		return widgets.HorseShopDialogProse{}, fmt.Errorf(
			"formatting horse polite farewell %q: %w",
			politeFarewellID,
			err,
		)
	}

	return widgets.HorseShopDialogProse{
		Greeting:       quoteMerchantConversation(greeting),
		RudeFarewell:   quoteMerchantConversation(rudeFarewell),
		PoliteFarewell: quoteMerchantConversation(politeFarewell),
		FundsRefusal: fmt.Sprintf(
			"\"Thou couldst not afford to feed it!\" yells %s.",
			state.Merchant.OwnerName,
		),
	}, nil
}

func (g *GameScene) resolveShipShopProse(
	state game_state.ShipShopState,
) (shipShopWorkflowProse, error) {
	if g.gameState == nil || g.gameState.GameReferences == nil ||
		g.gameState.GameReferences.MerchantReferences == nil {
		return shipShopWorkflowProse{}, fmt.Errorf(
			"resolving ship prose: merchant references are unavailable: %w",
			game_state.ErrInvalidMerchantSession,
		)
	}
	merchantReferences := g.gameState.GameReferences.MerchantReferences
	values := references.MerchantTextValues{
		OwnerName:    state.Merchant.OwnerName,
		ShopName:     state.Merchant.ShopName,
		ItemName:     "vessel",
		Price:        state.FrigateQuote.Price,
		Quantity:     1,
		LocationName: state.Merchant.Location.String(),
		TimeOfDay:    merchantTimeOfDay(g.gameState.DateTime.Hour),
	}
	// REDUX DEVIATION: MERCH-0008: original ship greetings and rude/polite
	// farewells use gameplay RNG. Redux maps cosmetic ship
	// prose from stable merchant identity so Preview remains pure.
	baseIndex := deterministicMerchantProseIndex(state.Merchant.ID, 4)
	greetingID := references.MerchantTextID("SHIP_TXT_" + strconv.Itoa(baseIndex))
	rudeID := references.MerchantTextID("RUDE_SHIP_" + strconv.Itoa((baseIndex+1)%4))
	politeID := references.MerchantTextID("POLITE_SHIP_" + strconv.Itoa((baseIndex+2)%4))
	greeting, err := merchantReferences.FormatText(greetingID, values)
	if err != nil {
		return shipShopWorkflowProse{}, fmt.Errorf("formatting ship greeting %q: %w", greetingID, err)
	}
	rudeFarewell, err := merchantReferences.FormatText(rudeID, values)
	if err != nil {
		return shipShopWorkflowProse{}, fmt.Errorf("formatting ship rude farewell %q: %w", rudeID, err)
	}
	politeFarewell, err := merchantReferences.FormatText(politeID, values)
	if err != nil {
		return shipShopWorkflowProse{}, fmt.Errorf("formatting ship polite farewell %q: %w", politeID, err)
	}
	funds, err := merchantReferences.FormatText(references.MerchantTextID("SHIP_MSG_3"), values)
	if err != nil {
		return shipShopWorkflowProse{}, fmt.Errorf("formatting ship funds refusal: %w", err)
	}

	return shipShopWorkflowProse{
		dialog: widgets.ShipShopDialogProse{
			Greeting:               quoteMerchantConversation(greeting),
			Farewell:               quoteMerchantConversation(rudeFarewell),
			BlockedAcknowledgement: quoteMerchantConversation("This harbour cannot take that order."),
		},
		politeFarewell: quoteMerchantConversation(politeFarewell),
		fundsRefusal: fmt.Sprintf(
			"%s yells %s.",
			quoteMerchantConversation(funds),
			state.Merchant.OwnerName,
		),
	}, nil
}

func resolveShipPurchaseResultDisplay(
	prose shipShopWorkflowProse,
	result game_state.ShipPurchaseResult,
) widgets.ShipShopResultDisplay {
	var receipt string
	switch result.Action {
	case game_state.ShipPurchaseCreateFrigate:
		receipt = fmt.Sprintf(
			"Bought: a frigate with %d skiffs for %d gold. It awaits thee in the harbour.",
			result.SkiffCountAfter,
			result.Price,
		)
	case game_state.ShipPurchaseCreateStandaloneSkiff:
		receipt = fmt.Sprintf("Bought: a standalone skiff for %d gold. It awaits thee in the harbour.", result.Price)
	case game_state.ShipPurchaseLoadSkiff:
		receipt = fmt.Sprintf(
			"Bought: %d %s for %d gold. The frigate now carries %d.",
			result.Quantity,
			shipSkiffNoun(result.Quantity),
			result.Price,
			result.SkiffCountAfter,
		)
	default:
		panic("resolveShipPurchaseResultDisplay: unknown purchase action")
	}
	if result.Theft > 0 {
		receipt += fmt.Sprintf(" %d more gold was stolen!", result.Theft)
	}
	receipt += fmt.Sprintf(" Thou hast %d gold left.", result.GoldAfterTheft)

	return widgets.ShipShopResultDisplay{MerchantExchange: prose.politeFarewell, Receipt: receipt}
}

func shipSkiffNoun(quantity int) string {
	if quantity == 1 {
		return "skiff"
	}

	return "skiffs"
}

func resolveShipFundsRefusalDisplay(
	prose shipShopWorkflowProse,
	state game_state.ShipShopState,
	request game_state.ShipPurchaseRequest,
) widgets.ShipShopResultDisplay {
	name := "frigate"
	if request.VehicleType == references.SkiffVehicle {
		name = shipSkiffNoun(max(1, request.Quantity))
	}

	return widgets.ShipShopResultDisplay{
		MerchantExchange: prose.fundsRefusal,
		Receipt: fmt.Sprintf(
			"No %s bought: need %d gold; party has %d.",
			name,
			request.ReviewedTotalPrice,
			state.Gold,
		),
	}
}

func resolveShipPurchaseRefusalDisplay(
	state game_state.ShipShopState,
	request game_state.ShipPurchaseRequest,
	purchaseErr error,
) widgets.ShipShopResultDisplay {
	name := "frigate"
	quote := state.FrigateQuote
	if request.VehicleType == references.SkiffVehicle {
		name = "skiff"
		quote = state.SkiffQuote
	}
	reason := "the reviewed offer or harbour state changed"
	if errors.Is(purchaseErr, game_state.ErrShipPurchaseRefused) {
		switch quote.BlockReason {
		case game_state.ShipPurchaseBlockPendingOrderElsewhere:
			reason = "another harbour order must arrive first"
		case game_state.ShipPurchaseBlockBerthOccupied:
			reason = "the berth is obstructed"
		case game_state.ShipPurchaseBlockStandaloneSkiff:
			reason = "a standalone skiff already holds the berth"
		case game_state.ShipPurchaseBlockFrigateAlreadyPresent:
			reason = "a frigate already holds the berth"
		case game_state.ShipPurchaseBlockMapCapacity:
			reason = "the harbour has no room for another vessel"
		case game_state.ShipPurchaseBlockSkiffLimit:
			reason = "the frigate cannot carry another skiff"
		case game_state.ShipPurchaseBlockNone:
			reason = "delivery authority refused the order"
		default:
			panic("resolveShipPurchaseRefusalDisplay: unknown block reason")
		}
	}

	return widgets.ShipShopResultDisplay{
		MerchantExchange: fmt.Sprintf("\"I cannot complete that order,\" says %s.", state.Merchant.OwnerName),
		Receipt:          fmt.Sprintf("No %s bought: %s.", name, reason),
	}
}

func deterministicReagentFarewellProseIndex(id references.MerchantID, polite bool) int {
	offset := 1
	if polite {
		offset = 2
	}

	return (deterministicMerchantProseIndex(id, 4) + offset) % 4
}

func resolveReagentPurchaseResultDisplay(
	merchant references.MerchantDefinition,
	result game_state.ReagentPurchaseResult,
) widgets.ReagentPurchaseResultDisplay {
	bundleNoun := "bundles"
	if result.BundleCount == 1 {
		bundleNoun = "bundle"
	}
	receipt := fmt.Sprintf(
		"Bought: %d %s of %s for %d gold. Thou hast now %d.",
		result.BundleCount,
		bundleNoun,
		result.ReagentName,
		result.TotalPrice,
		result.QuantityAfter,
	)
	if result.Theft > 0 {
		receipt += fmt.Sprintf(" %d more gold was stolen!", result.Theft)
	}
	receipt += fmt.Sprintf(" Thou hast %d gold left.", result.GoldAfterTheft)

	return widgets.ReagentPurchaseResultDisplay{
		MerchantExchange: fmt.Sprintf("\"I thank thee!\" says %s. \"Anything else?\"", merchant.OwnerName),
		Receipt:          receipt,
	}
}

func resolveHealerResultDisplay(
	merchant references.MerchantDefinition,
	result game_state.HealerServiceResult,
) widgets.HealerServiceResultDisplay {
	var receipt string
	switch result.Service {
	case references.HealerCure:
		receipt = result.CharacterName + ": Poison cured!"
	case references.HealerHeal:
		receipt = fmt.Sprintf("%s healed: HP %d -> %d/%d.", result.CharacterName, result.HPBefore, result.HPAfter, result.MaxHPAfter)
	case references.HealerResurrect:
		receipt = fmt.Sprintf(
			"%s resurrected: L%d -> L%d; HP %d/%d.",
			result.CharacterName,
			result.LevelBefore,
			result.LevelAfter,
			result.HPAfter,
			result.MaxHPAfter,
		)
	default:
		panic("resolveHealerResultDisplay: unknown service")
	}
	switch result.ChargeKind {
	case game_state.HealerChargePaid:
		receipt += fmt.Sprintf(" Paid %d gold.", result.Charge)
		if result.Theft > 0 {
			receipt += fmt.Sprintf(" %d more gold was stolen!", result.Theft)
		}
		receipt += fmt.Sprintf(" Thou hast %d gold left.", result.GoldAfterTheft)
	case game_state.HealerChargeFree:
		receipt += " Free care."
	case game_state.HealerChargeCharitable:
		receipt += " Charitable care."
	default:
		panic("resolveHealerResultDisplay: unknown charge kind")
	}

	return widgets.HealerServiceResultDisplay{
		MerchantExchange: fmt.Sprintf(
			"\"It is done,\" says %s. \"May I aid thee further?\"",
			merchant.OwnerName,
		),
		Receipt: receipt,
	}
}

func resolveHealerAllResultDisplay(
	merchant references.MerchantDefinition,
	result game_state.HealerAllServiceResult,
) widgets.HealerServiceResultDisplay {
	var verb string
	switch result.Service {
	case references.HealerCure:
		verb = "Cured"
	case references.HealerHeal:
		verb = "Healed"
	case references.HealerResurrect:
		verb = "Resurrected"
	default:
		panic("resolveHealerAllResultDisplay: unknown service")
	}
	recipientLabel := "party members"
	if len(result.CharacterIndexes) == 1 {
		recipientLabel = "party member"
	}
	receipt := fmt.Sprintf("%s %d %s.", verb, len(result.CharacterIndexes), recipientLabel)
	switch result.ChargeKind {
	case game_state.HealerChargePaid:
		receipt += fmt.Sprintf(" Paid %d gold.", result.Charge)
		if result.Theft > 0 {
			receipt += fmt.Sprintf(" %d more gold was stolen!", result.Theft)
		}
		receipt += fmt.Sprintf(" Thou hast %d gold left.", result.GoldAfterTheft)
	case game_state.HealerChargeFree:
		receipt += " Free care."
	case game_state.HealerChargeCharitable:
		receipt += " Charitable care."
	default:
		panic("resolveHealerAllResultDisplay: unknown charge kind")
	}

	return widgets.HealerServiceResultDisplay{
		MerchantExchange: fmt.Sprintf(
			"\"It is done,\" says %s. \"May I aid thee further?\"",
			merchant.OwnerName,
		),
		Receipt: receipt,
	}
}

func resolveHealerRefusalDisplay(
	state game_state.HealerShopState,
	request game_state.HealerServiceRequest,
	prose widgets.HealerShopDialogProse,
	serviceErr error,
) widgets.HealerServiceResultDisplay {
	if request.CharacterIndex == game_state.HealerAllRecipientsIndex {
		service := state.AllServices[int(request.Service)]
		if errors.Is(serviceErr, game_state.ErrHealerInsufficientFunds) {
			return widgets.HealerServiceResultDisplay{
				MerchantExchange: prose.FundsRefusal,
				Receipt: fmt.Sprintf(
					"No %s for all: need %d gold; party has %d.",
					strings.ToLower(request.Service.String()),
					service.Charge,
					state.Gold,
				),
			}
		}

		return widgets.HealerServiceResultDisplay{
			MerchantExchange: prose.NoNeed,
			Receipt:          fmt.Sprintf("No party members need %s.", request.Service),
		}
	}
	recipient := healerRecipientForRequest(state, request.CharacterIndex)
	if errors.Is(serviceErr, game_state.ErrHealerInsufficientFunds) {
		service := state.Services[int(request.Service)]

		return widgets.HealerServiceResultDisplay{
			MerchantExchange: prose.FundsRefusal,
			Receipt: fmt.Sprintf(
				"No %s for %s: need %d gold; party has %d.",
				strings.ToLower(request.Service.String()),
				recipient.Name,
				service.Charge,
				state.Gold,
			),
		}
	}

	return widgets.HealerServiceResultDisplay{
		MerchantExchange: prose.NoNeed,
		Receipt: fmt.Sprintf(
			"No %s for %s: %s",
			strings.ToLower(request.Service.String()),
			recipient.Name,
			healerEligibilityReceipt(recipient.Treatments[int(request.Service)].Eligibility),
		),
	}
}

func healerRecipientForRequest(
	state game_state.HealerShopState,
	characterIndex int,
) game_state.HealerRecipientState {
	for _, recipient := range state.Recipients {
		if recipient.CharacterIndex == characterIndex {
			return recipient
		}
	}
	panic("healer recipient missing from refreshed state")
}

func healerEligibilityReceipt(eligibility game_state.HealerEligibility) string {
	switch eligibility {
	case game_state.HealerNotPoisoned, game_state.HealerNotInjured:
		return `"Thou hast no need of this art!"`
	case game_state.HealerDeadCannotHeal:
		return "recipient is dead."
	case game_state.HealerLivingCannotResurrect:
		return "recipient is still living."
	case game_state.HealerEligible:
		return "it cannot be done now."
	default:
		return "treatment is unavailable."
	}
}

func (g *GameScene) resolveArmsTradeProse(
	state game_state.ArmsShopState,
	intent widgets.ArmsTradeIntent,
) (widgets.ArmsTradeDialogProse, error) {
	if g.gameState == nil || g.gameState.GameReferences == nil {
		return widgets.ArmsTradeDialogProse{}, fmt.Errorf(
			"resolving Arms trade prose: merchant references are unavailable: %w",
			game_state.ErrInvalidMerchantSession,
		)
	}
	merchantReferences := g.gameState.GameReferences.MerchantReferences
	if merchantReferences == nil {
		return widgets.ArmsTradeDialogProse{}, fmt.Errorf(
			"resolving Arms trade prose: merchant references are unavailable: %w",
			game_state.ErrInvalidMerchantSession,
		)
	}
	equipment := intent.Equipment()
	values := references.MerchantTextValues{
		OwnerName:     state.Merchant.OwnerName,
		ShopName:      state.Merchant.ShopName,
		ItemName:      equipment.String(),
		Quantity:      1,
		LocationName:  state.Merchant.Location.String(),
		TimeOfDay:     merchantTimeOfDay(g.gameState.DateTime.Hour),
		InformantName: state.PricingCharacterName,
	}
	prose := widgets.ArmsTradeDialogProse{
		Pitch:   fmt.Sprintf("A fine %s, fairly priced.", equipment),
		Offer:   fmt.Sprintf("I can offer fair gold for thy %s.", equipment),
		Refusal: fmt.Sprintf("I cannot accept thy %s.", equipment),
	}
	if intent.Mode == widgets.ArmsShopBuy {
		values.Price = intent.BuyQuote.UnitPrice
		values.Quantity = int(max(uint16(1), intent.BuyQuote.MaxQuantity))
		pitchID := references.MerchantTextID("ARMS_PITCH_" + strconv.Itoa(equipment.ID()))
		pitch, err := merchantReferences.FormatText(pitchID, values)
		if err != nil {
			return widgets.ArmsTradeDialogProse{}, fmt.Errorf("formatting Arms pitch %q: %w", pitchID, err)
		}
		prose.Pitch = strings.TrimSpace(pitch)

		return prose, nil
	}

	values.Price = intent.SellQuote.UnitOffer
	values.Quantity = int(max(uint16(1), intent.SellQuote.MaxQuantity))
	if intent.SellQuote.Sellable && intent.SellQuote.UnitOffer > 0 {
		// REDUX DEVIATION: MERCH-0004: original offers choose one of eight descriptions with a gameplay draw from 0..7; Redux hashes merchant and equipment so Preview prose is stable and cosmetic selection never consumes gameplay RNG.
		offerID := references.MerchantTextID("SCUZZ_" + strconv.Itoa(
			deterministicArmsSellProseIndex(state.Merchant.ID, equipment),
		))
		offer, err := merchantReferences.FormatText(offerID, values)
		if err != nil {
			return widgets.ArmsTradeDialogProse{}, fmt.Errorf("formatting Arms offer %q: %w", offerID, err)
		}
		prose.Offer = strings.TrimSpace(offer)
	}
	prose.Refusal = resolveArmsTradeRefusalProse(state.Merchant, intent)

	return prose, nil
}

func deterministicArmsSellProseIndex(id references.MerchantID, equipment references.Equipment) int {
	return (deterministicMerchantProseIndex(id, 8) + equipment.ID()) % 8
}

func resolveArmsTradeRefusalProse(
	merchant references.MerchantDefinition,
	intent widgets.ArmsTradeIntent,
) string {
	if intent.Mode == widgets.ArmsShopBuy {
		return merchant.OwnerName + " cannot complete that purchase."
	}
	switch {
	case intent.SellQuote.Equipment.IsAmmo():
		return "We don't deal in used ammunition!"
	case intent.SellQuote.BasePrice == 0 || intent.SellQuote.UnitOffer == 0 || !intent.SellQuote.Sellable:
		return "That, I cannot buy from thee."
	case intent.SellQuote.MaxQuantity == 0:
		return fmt.Sprintf("Spend some gold before selling more, says %s.", merchant.OwnerName)
	default:
		return merchant.OwnerName + " cannot accept that equipment."
	}
}

func resolveArmsLocalRefusalDisplay(
	state game_state.ArmsShopState,
	intent widgets.ArmsTradeIntent,
	refusal widgets.ArmsTradeRefusalKind,
) widgets.ArmsTradeResultDisplay {
	return resolveArmsSellRefusalDisplay(state.Merchant, intent.Equipment(), refusal)
}

func resolveArmsBuyRefusalDisplay(
	state game_state.ArmsShopState,
	equipment references.Equipment,
	err error,
) widgets.ArmsTradeResultDisplay {
	if errors.Is(err, game_state.ErrArmsCapacity) {
		return widgets.ArmsTradeResultDisplay{
			MerchantExchange: fmt.Sprintf("\"Thou canst not carry any more!\" says %s.", state.Merchant.OwnerName),
			Receipt:          fmt.Sprintf("No %s bought: thou canst not carry any more!", equipment),
		}
	}

	return widgets.ArmsTradeResultDisplay{
		MerchantExchange: fmt.Sprintf("\"Can't pay?! Out with thee!\" yells %s.", state.Merchant.OwnerName),
		Receipt:          fmt.Sprintf("No %s bought: thou hast not enough gold!", equipment),
	}
}

func resolveArmsSellAuthorityRefusalDisplay(
	state game_state.ArmsShopState,
	equipment references.Equipment,
	err error,
) widgets.ArmsTradeResultDisplay {
	switch {
	case errors.Is(err, game_state.ErrArmsUnsellableAmmunition):
		return resolveArmsSellRefusalDisplay(
			state.Merchant,
			equipment,
			widgets.ArmsTradeUsedAmmunitionRefusal,
		)
	case errors.Is(err, game_state.ErrArmsGoldCapacity):
		return resolveArmsSellRefusalDisplay(
			state.Merchant,
			equipment,
			widgets.ArmsTradeGoldCapacityRefusal,
		)
	default:
		return resolveArmsSellRefusalDisplay(
			state.Merchant,
			equipment,
			widgets.ArmsTradeZeroPriceRefusal,
		)
	}
}

func resolveArmsSellRefusalDisplay(
	merchant references.MerchantDefinition,
	equipment references.Equipment,
	refusal widgets.ArmsTradeRefusalKind,
) widgets.ArmsTradeResultDisplay {
	switch refusal {
	case widgets.ArmsTradeUsedAmmunitionRefusal:
		return widgets.ArmsTradeResultDisplay{
			MerchantExchange: fmt.Sprintf("\"We don't deal in used ammunition!\" growls %s.", merchant.OwnerName),
			Receipt:          fmt.Sprintf("No %s sold: \"We don't deal in used ammunition!\"", equipment),
		}
	case widgets.ArmsTradeZeroPriceRefusal:
		return widgets.ArmsTradeResultDisplay{
			MerchantExchange: fmt.Sprintf("\"That, I cannot buy from thee.\" says %s.", merchant.OwnerName),
			Receipt:          fmt.Sprintf("No %s sold: \"That, I cannot buy from thee.\"", equipment),
		}
	case widgets.ArmsTradeGoldCapacityRefusal:
		return widgets.ArmsTradeResultDisplay{
			MerchantExchange: fmt.Sprintf("\"Spend some gold before selling more.\" says %s.", merchant.OwnerName),
			Receipt:          fmt.Sprintf("No %s sold: thou canst not carry any more gold!", equipment),
		}
	case widgets.ArmsTradeNoRefusal:
		panic("resolveArmsSellRefusalDisplay: refusal is required")
	default:
		panic("resolveArmsSellRefusalDisplay: unknown refusal")
	}
}

func resolveArmsBuyResultDisplay(
	merchant references.MerchantDefinition,
	result game_state.ArmsBuyResult,
) widgets.ArmsTradeResultDisplay {
	receipt := fmt.Sprintf(
		"Bought: %s x%d for %d gold. Thou hast now %d.",
		result.Equipment,
		result.Quantity,
		result.TotalCost,
		result.QuantityAfter,
	)
	if result.AmmoRefill {
		receipt = fmt.Sprintf(
			"Bought: %s x%d ammunition refill for %d gold. Thou hast now %d.",
			result.Equipment,
			result.Quantity,
			result.TotalCost,
			result.QuantityAfter,
		)
	}
	if result.Theft > 0 {
		receipt += fmt.Sprintf(" %d more gold was stolen!", result.Theft)
	}

	return widgets.ArmsTradeResultDisplay{
		MerchantExchange: fmt.Sprintf("\"Sold!\" says %s. \"Anything else?\"", merchant.OwnerName),
		Receipt:          receipt,
	}
}

func resolveArmsSellResultDisplay(
	merchant references.MerchantDefinition,
	result game_state.ArmsSellResult,
) widgets.ArmsTradeResultDisplay {
	return widgets.ArmsTradeResultDisplay{
		MerchantExchange: fmt.Sprintf("\"Done!\" says %s. \"What else can ye offer me?\"", merchant.OwnerName),
		Receipt: fmt.Sprintf(
			"Sold: %s x%d for %d gold. Thou hast now %d.",
			result.Equipment,
			result.Quantity,
			result.TotalOffer,
			result.QuantityAfter,
		),
	}
}

func (g *GameScene) resolveGuildShopProse(
	state game_state.GuildShopState,
) (widgets.GuildShopDialogProse, error) {
	merchantReferences := g.gameState.GameReferences.MerchantReferences
	if merchantReferences == nil {
		return widgets.GuildShopDialogProse{}, fmt.Errorf(
			"resolving guild prose: merchant references are unavailable: %w",
			game_state.ErrInvalidMerchantSession,
		)
	}

	common := references.MerchantTextValues{
		OwnerName:    state.Merchant.OwnerName,
		ShopName:     state.Merchant.ShopName,
		LocationName: state.Merchant.Location.String(),
		TimeOfDay:    merchantTimeOfDay(g.gameState.DateTime.Hour),
	}
	greetingID := references.MerchantTextID(
		"GUILD_TXT_" + strconv.Itoa(deterministicMerchantProseIndex(state.Merchant.ID, 4)),
	)
	greeting, err := merchantReferences.FormatText(greetingID, common)
	if err != nil {
		return widgets.GuildShopDialogProse{}, fmt.Errorf("formatting guild greeting %q: %w", greetingID, err)
	}

	prose := widgets.GuildShopDialogProse{
		Greeting: normalizeOriginalGuildGreeting(greeting),
	}
	for index, quote := range state.Offers {
		values := common
		values.ItemName = strings.ToLower(widgets.GuildProvisionName(quote.Provision))
		values.Price = quote.PricePerBundle
		values.Quantity = int(quote.BundleQuantity)
		pitchID := references.MerchantTextID("GUILD_PITCH_" + strconv.Itoa(index))
		pitch, pitchErr := merchantReferences.FormatText(pitchID, values)
		if pitchErr != nil {
			return widgets.GuildShopDialogProse{}, fmt.Errorf("formatting guild pitch %q: %w", pitchID, pitchErr)
		}
		prose.OfferPitches[index] = strings.TrimSpace(pitch)
	}

	return prose, nil
}

func normalizeOriginalGuildGreeting(greeting string) string {
	return quoteMerchantConversation(greeting)
}

func quoteMerchantConversation(value string) string {
	return `"` + strings.Trim(strings.TrimSpace(value), `"`) + `"`
}

func (g *GameScene) resolveGuildPurchaseResultDisplay(
	session game_state.MerchantSession,
	result game_state.GuildPurchaseResult,
) widgets.GuildPurchaseResultDisplay {
	honorific := "m'lord"
	buyer := g.gameState.PartyState.GetCharacter(session.PricingCharacterIndex())
	if buyer.Gender == party_state.Female {
		honorific = "m'lady"
	}
	bundleNoun := "bundles"
	if result.BundleCount == 1 {
		bundleNoun = "bundle"
	}
	receipt := fmt.Sprintf(
		"Bought: %d %s of %s for %d gold. Thou hast now %d.",
		result.BundleCount,
		bundleNoun,
		strings.ToLower(widgets.GuildProvisionName(result.Provision)),
		result.TotalCost,
		result.QuantityAfter,
	)
	if result.Theft > 0 {
		receipt += fmt.Sprintf(" %d more gold was stolen!", result.Theft)
	}

	return widgets.GuildPurchaseResultDisplay{
		MerchantExchange: fmt.Sprintf(
			"\"Sold!\" says %s. \"What else, %s?\"",
			session.Merchant().OwnerName,
			honorific,
		),
		Receipt: receipt,
	}
}

func merchantSessionFeedback(err error) string {
	switch {
	case errors.Is(err, game_state.ErrMerchantClosed):
		return "This shop is closed."
	case errors.Is(err, game_state.ErrMerchantHorsebackRefusal):
		return "Dismount before trading here."
	case errors.Is(err, game_state.ErrMerchantBuyerUnavailable):
		return "No-one in thy party can trade."
	default:
		return "Merchant service is unavailable."
	}
}

func merchantTimeOfDay(hour byte) references.MerchantTimeOfDay {
	switch {
	case hour < 12:
		return references.MerchantMorning
	case hour < 18:
		return references.MerchantAfternoon
	default:
		return references.MerchantEvening
	}
}

func deterministicMerchantProseIndex(id references.MerchantID, count int) int {
	if count <= 0 {
		panic("merchant prose count must be positive")
	}
	index := 0
	for _, character := range string(id) {
		index = (index*31 + int(character)) % count
	}

	return index
}
