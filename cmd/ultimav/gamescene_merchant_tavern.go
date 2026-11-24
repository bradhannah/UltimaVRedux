package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

var (
	errTavernInformationProseTopic = errors.New("tavern information prose topic is unavailable")
	errTavernMerchantText          = errors.New("tavern merchant text is unavailable")
)

type tavernShopWorkflowProse struct {
	dialog         widgets.TavernShopDialogProse
	rudeFarewell   string
	politeFarewell string
	drinkWarning   string
	prudentChoice  string
}

type tavernInformationProse struct {
	pitch  string
	answer string
}

func (g *GameScene) thenRunTavernShop(
	session game_state.MerchantSession,
	workflowState *merchantTalkWorkflowState,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return g.thenRunTavernShopAuthority(session, game_state.TavernShopReview{}, false, workflowState)
}

func (g *GameScene) thenRunTavernPreview(
	review game_state.TavernShopReview,
	workflowState *merchantTalkWorkflowState,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	// Preview exposes the detached information path without issuing a customer session.
	review.State.CustomerEligible = true

	return g.thenRunTavernShopAuthority(game_state.MerchantSession{}, review, true, workflowState)
}

func (g *GameScene) thenRunTavernShopAuthority(
	session game_state.MerchantSession,
	review game_state.TavernShopReview,
	preview bool,
	workflowState *merchantTalkWorkflowState,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		var dialog *widgets.TavernShopDialog
		var state game_state.TavernShopState
		var prose tavernShopWorkflowProse
		var informationQuestion string
		var smashedAtEntry int
		var setupErr error

		return q.Then(func() {
			state = review.State
			merchantSpriteIndex := review.MerchantSpriteIndex
			if !preview {
				var err error
				state, err = g.gameState.GetTavernShopState(session)
				if err != nil {
					setupErr = fmt.Errorf("loading Tavern shop state: %w", err)

					return
				}
				merchantSpriteIndex = session.MerchantSpriteIndex()
			}
			smashedAtEntry = state.Smashed
			var err error
			prose, err = g.resolveTavernShopProse(state)
			if err != nil {
				setupErr = err

				return
			}
			if g.spriteSheet == nil {
				setupErr = fmt.Errorf("loaded Tavern merchant sprite is unavailable: %w", game_state.ErrInvalidMerchantSession)

				return
			}
			merchantSprite := g.spriteSheet.GetSprite(merchantSpriteIndex)
			if merchantSprite == nil {
				setupErr = fmt.Errorf("loaded Tavern merchant sprite is unavailable: %w", game_state.ErrInvalidMerchantSession)

				return
			}

			dialog = widgets.NewTavernShopDialog(widgets.TavernShopDialogConfig{
				Keyboard:         g.keyboard,
				MerchantSprite:   merchantSprite,
				State:            state,
				Prose:            prose.dialog,
				Preview:          preview,
				IsKeyPressed:     g.keyboard.IsKeyHeld,
				IsWindowFocused:  g.commandInputFocused,
				AppendInputChars: g.appendInputChars,
			})
			g.keyboard.PrepareForTransition(0)
			workflowState.pushOwnedDialog(&g.dialogStack, dialog)
		}).If(
			func() bool { return setupErr != nil },
			func(errorQ *event_queue.SequentialEventBuilder) {
				logging.Errorf(logging.CategoryConversation, "Tavern shop setup failed: %v", setupErr)
				errorQ.ThenSequence(g.thenShowConfirmationDialog("Tavern service is unavailable."))
			},
		).While(
			func() bool { return dialog != nil && !dialog.IsDismissed() },
			func() {
				if g.handleTavernActionDialog(
					session, dialog, preview, prose, informationQuestion, &state, workflowState,
				) {
					return
				}
				if topic, pending := dialog.ConsumePendingTopicIntent(); pending {
					var quote game_state.TavernInformationQuote
					var err error
					if preview {
						quote, err = g.gameState.ReviewTavernInformationByMerchantID(state.Merchant.ID, topic.Input)
					} else {
						quote, err = g.gameState.QuoteTavernInformation(session, topic.Input)
					}
					if errors.Is(err, game_state.ErrTavernInformationTopicUnknown) {
						dialog.RejectInformationTopic(topic)
						notice := widgets.NewTavernAdviceNotice(
							g.keyboard, dialog.GetWorkspaceBounds,
							state.Merchant.OwnerName+" says, "+prose.dialog.UnknownTopic,
						)
						workflowState.tavernNotice = notice
						g.keyboard.PrepareForTransition(0)
						workflowState.pushOwnedDialog(&g.dialogStack, notice)

						return
					}
					if err != nil {
						logging.Errorf(logging.CategoryConversation, "Tavern information quote failed: %v", err)
						dialog.ApplySessionError("The Tavern information could not be quoted.")

						return
					}
					informationProse, err := g.resolveTavernInformationProse(state, quote.Topic)
					if err != nil {
						logging.Errorf(logging.CategoryConversation, "Tavern information prose failed: %v", err)
						dialog.ApplySessionError("The Tavern information could not be presented.")

						return
					}
					informationQuestion = topic.Input
					dialog.ApplyInformationQuote(topic, quote, informationProse.pitch)

					return
				}
				intent, pending := dialog.ConsumePendingActionIntent()
				if !pending {
					return
				}
				if intent.Action == widgets.TavernActionProvision {
					pitch, pitchErr := g.resolveTavernProvisionPitch(state)
					if pitchErr != nil {
						logging.Errorf(logging.CategoryConversation, "Tavern provision pitch failed: %v", pitchErr)
						dialog.ApplySessionError("The Tavern provisions could not be presented.")

						return
					}
					intent.ProvisionPitch = pitch
				}
				if intent.Drink.Warning {
					intent.DrinkWarning = prose.drinkWarning
				}
				actionDialog := widgets.NewTavernActionDialog(widgets.TavernActionDialogConfig{
					Keyboard:     g.keyboard,
					Intent:       intent,
					IsKeyPressed: g.keyboard.IsKeyHeld,
				})
				workflowState.tavernAction = actionDialog
				g.keyboard.PrepareForTransition(0)
				workflowState.pushOwnedDialog(&g.dialogStack, actionDialog)
			},
			merchantDialogPollingRateMs,
		).Then(func() {
			if !preview && smashedAtEntry == 0 && state.Smashed > 0 {
				// REDUX DEVIATION: MERCH-0014: original service uses only the ordinary Tavern farewell
				// after intoxication. Redux adds a timed status notice so the lasting effect is clear.
				g.showTavernStatusOverlay("The party is officially smashed.", widgets.DialogStatusWarningColor)
			}
		})
	}
}

func (g *GameScene) handleTavernActionDialog(
	session game_state.MerchantSession,
	dialog *widgets.TavernShopDialog,
	preview bool,
	prose tavernShopWorkflowProse,
	informationQuestion string,
	state *game_state.TavernShopState,
	workflowState *merchantTalkWorkflowState,
) bool {
	notice := workflowState.tavernNotice
	if notice != nil {
		if !notice.IsDismissed() {
			return true
		}
		g.dialogStack.RemoveWidgetInstance(notice)
		workflowState.tavernNotice = nil

		return true
	}
	resultDialog := workflowState.tavernResult
	if resultDialog != nil {
		if !resultDialog.IsDismissed() {
			return true
		}
		g.dialogStack.RemoveWidgetInstance(resultDialog)
		workflowState.tavernResult = nil
		if workflowState.tavernDismissAfterResult {
			workflowState.tavernDismissAfterResult = false
			dialog.Dismiss()
		}

		return true
	}
	actionDialog := workflowState.tavernAction
	if actionDialog == nil {
		return false
	}
	if !actionDialog.IsDismissed() {
		return true
	}
	g.dialogStack.RemoveWidgetInstance(actionDialog)
	workflowState.tavernAction = nil

	request, confirmed := actionDialog.ConsumePendingRequest()
	if actionDialog.IsCancelled() || !confirmed {
		dialog.CancelActionApproval()

		return true
	}
	dialog.AcceptActionApproval(request)
	if preview {
		dialog.PreviewStop(request)

		return true
	}
	resultDialog, dismissAfterResult := g.commitTavernAction(
		session, dialog, request, prose, informationQuestion, state,
	)
	if resultDialog != nil {
		workflowState.tavernResult = resultDialog
		workflowState.tavernDismissAfterResult = dismissAfterResult
		g.keyboard.PrepareForTransition(0)
		workflowState.pushOwnedDialog(&g.dialogStack, resultDialog)
	}

	return true
}

func (g *GameScene) commitTavernAction(
	session game_state.MerchantSession,
	dialog *widgets.TavernShopDialog,
	request widgets.TavernActionRequest,
	prose tavernShopWorkflowProse,
	informationQuestion string,
	state *game_state.TavernShopState,
) (*widgets.TavernResultDialog, bool) {
	var commitErr error
	var mealResult game_state.TavernMealResult
	var drinkResult game_state.TavernDrinkResult
	var stopResult game_state.TavernStopResult
	var provisionResult game_state.TavernProvisionResult
	var informationResult game_state.TavernInformationResult

	switch request.Action {
	case widgets.TavernActionMeal:
		mealResult, commitErr = g.gameState.PurchaseTavernMeal(session, request.Meal)
		if commitErr == nil {
			g.installTavernMealTableVisual(mealResult.TransientTableVisual)
		}
	case widgets.TavernActionDrink:
		drinkResult, commitErr = g.gameState.PurchaseTavernDrink(session, request.Drink)
	case widgets.TavernActionStop:
		stopResult, commitErr = g.gameState.StopTavernDrinking(session, request.Stop)
	case widgets.TavernActionProvision:
		provisionResult, commitErr = g.gameState.PurchaseTavernProvisions(session, request.Provision)
	case widgets.TavernActionInformation:
		informationResult, commitErr = g.gameState.PurchaseTavernInformation(session, request.Information)
	case widgets.TavernActionUnknown:
		panic("Tavern workflow received an empty action")
	default:
		panic("Tavern workflow received an unknown action")
	}
	if commitErr != nil {
		logging.Errorf(logging.CategoryConversation, "Tavern action failed: %v", commitErr)
		dialog.ApplySessionError("The Tavern transaction could not be completed.")

		return nil, false
	}
	refreshed, err := g.gameState.GetTavernShopState(session)
	if err != nil {
		logging.Errorf(logging.CategoryConversation, "Tavern refresh failed: %v", err)
		dialog.ApplySessionError("The Tavern account could not be refreshed.")

		return nil, false
	}
	*state = refreshed
	var display widgets.TavernActionResultDisplay
	switch request.Action {
	case widgets.TavernActionMeal:
		display = resolveTavernMealResultDisplay(prose, mealResult)
	case widgets.TavernActionDrink:
		display = g.resolveTavernDrinkResultDisplay(refreshed, prose, drinkResult)
	case widgets.TavernActionStop:
		display = resolveTavernStopResultDisplay(prose, stopResult)
	case widgets.TavernActionProvision:
		display, commitErr = g.resolveTavernProvisionResultDisplay(refreshed, provisionResult, prose)
	case widgets.TavernActionInformation:
		display, commitErr = g.resolveTavernInformationResultDisplay(refreshed, informationResult, informationQuestion)
	case widgets.TavernActionUnknown:
		panic("Tavern workflow refreshed an empty action")
	default:
		panic("Tavern workflow refreshed an unknown action")
	}
	if commitErr != nil {
		logging.Errorf(logging.CategoryConversation, "Tavern result prose failed: %v", commitErr)
		dialog.ApplySessionError("The Tavern result could not be displayed.")

		return nil, false
	}
	dismissAfterResult := display.Dismiss
	var resultView *widgets.TavernResultView
	switch request.Action {
	case widgets.TavernActionMeal:
		view := tavernMealResultView(mealResult)
		resultView = &view
	case widgets.TavernActionProvision:
		view := tavernProvisionResultView(provisionResult)
		resultView = &view
	case widgets.TavernActionInformation:
		if informationResult.Outcome == game_state.TavernInformationInsufficientFunds {
			resultView = &widgets.TavernResultView{
				Title: "Information Refused", Item: informationResult.Topic.Key,
				Note: display.MerchantExchange, Tone: widgets.TavernResultFailure,
			}
		}
	case widgets.TavernActionDrink, widgets.TavernActionStop:
	case widgets.TavernActionUnknown:
		panic("Tavern workflow cannot present an empty result")
	default:
		panic("Tavern workflow cannot present an unknown result")
	}
	if resultView != nil {
		display.Dismiss = false
	}
	dialog.ApplyResult(request, refreshed, display)
	if resultView == nil {
		return nil, false
	}

	return widgets.NewTavernResultDialog(widgets.TavernResultDialogConfig{
		Keyboard: g.keyboard,
		View:     *resultView,
	}), dismissAfterResult
}

func (g *GameScene) resolveTavernShopProse(
	state game_state.TavernShopState,
) (tavernShopWorkflowProse, error) {
	merchantReferences, err := g.tavernMerchantReferences()
	if err != nil {
		return tavernShopWorkflowProse{}, err
	}
	values := references.MerchantTextValues{
		OwnerName:     state.Merchant.OwnerName,
		ShopName:      state.Merchant.ShopName,
		ItemName:      state.Menu.Meal.Name,
		Price:         int(state.Meal.Charge),
		Quantity:      state.DinerCount,
		InformantName: state.PricingCharacterName,
		LocationName:  state.Merchant.Location.String(),
		TimeOfDay:     merchantTimeOfDay(g.gameState.DateTime.Hour),
	}
	// REDUX DEVIATION: MERCH-0012: original Tavern greetings and rude/polite
	// farewells use gameplay RNG. Redux derives cosmetic Tavern prose from
	// stable merchant identity so Preview remains pure and Tavern authority owns RNG.
	baseIndex := deterministicMerchantProseIndex(state.Merchant.ID, 4)
	greeting, err := merchantReferences.FormatText(
		references.MerchantTextID("BAR_TXT_"+strconv.Itoa(baseIndex)), values,
	)
	if err != nil {
		return tavernShopWorkflowProse{}, fmt.Errorf("formatting Tavern greeting: %w", err)
	}
	orderPrompt, err := tavernMerchantText(
		merchantReferences,
		references.MerchantTextID("BAR_PITCH_"+strconv.Itoa(int(state.Menu.Variant))),
	)
	if err != nil {
		return tavernShopWorkflowProse{}, fmt.Errorf("loading Tavern order pitch: %w", err)
	}
	conversationPrompt := quoteMerchantConversation(fmt.Sprintf(
		"Of what wouldst thou hear my lore, %s?",
		g.tavernBuyerHonorific(state),
	))
	rude, err := tavernMerchantText(
		merchantReferences,
		references.MerchantTextID("RUDE_BAR_"+strconv.Itoa((baseIndex+1)%4)),
	)
	if err != nil {
		return tavernShopWorkflowProse{}, fmt.Errorf("loading Tavern rude farewell: %w", err)
	}
	polite, err := tavernMerchantText(
		merchantReferences,
		references.MerchantTextID("POLITE_BAR_"+strconv.Itoa((baseIndex+2)%4)),
	)
	if err != nil {
		return tavernShopWorkflowProse{}, fmt.Errorf("loading Tavern polite farewell: %w", err)
	}
	if err = g.validateTavernSourceProse(state); err != nil {
		return tavernShopWorkflowProse{}, err
	}

	return tavernShopWorkflowProse{
		dialog: widgets.TavernShopDialogProse{
			Greeting:           quoteMerchantConversation(greeting),
			OrderPrompt:        quoteMerchantConversation(orderPrompt),
			ConversationPrompt: conversationPrompt,
			ConversationExit:   quoteMerchantConversation("Anything else for thee?"),
			UnknownTopic:       quoteMerchantConversation("That, I cannot help thee with."),
		},
		rudeFarewell:   quoteMerchantConversation(rude),
		politeFarewell: quoteMerchantConversation(polite),
		drinkWarning: fmt.Sprintf(
			"\"I beg thy pardon, %s,\" says %s. \"But haven't ye had enough to drink?\"",
			g.tavernBuyerHonorific(state), state.Merchant.OwnerName,
		),
		prudentChoice: fmt.Sprintf(
			"\"A wise choice, %s. Best keep thy wits about thee,\" says %s.\n\n%s",
			g.tavernBuyerHonorific(state), state.Merchant.OwnerName,
			quoteMerchantConversation("Anything else for thee?"),
		),
	}, nil
}

func (g *GameScene) validateTavernSourceProse(state game_state.TavernShopState) error {
	merchantReferences, err := g.tavernMerchantReferences()
	if err != nil {
		return err
	}
	if _, err = tavernMerchantText(merchantReferences, references.MerchantTextID("BAR_MISC_0")); err != nil {
		return fmt.Errorf("loading Tavern wine refusal: %w", err)
	}
	for _, id := range []references.MerchantTextID{"BAR_MISC_1", "BAR_MISC_2"} {
		if _, err = merchantReferences.FormatText(
			id,
			references.MerchantTextValues{OwnerName: state.Merchant.OwnerName},
		); err != nil {
			return fmt.Errorf("formatting Tavern result prose %q: %w", id, err)
		}
	}
	if state.Menu.Provision.Available {
		if _, err = g.resolveTavernProvisionPitch(state); err != nil {
			return err
		}
	}
	for index, topic := range merchantReferences.TavernInformationTopics() {
		if _, err = formatTavernInformationProse(merchantReferences, state, topic, index); err != nil {
			return err
		}
	}

	return nil
}

func (g *GameScene) resolveTavernProvisionPitch(state game_state.TavernShopState) (string, error) {
	merchantReferences, err := g.tavernMerchantReferences()
	if err != nil {
		return "", err
	}
	index := (deterministicMerchantProseIndex(state.Merchant.ID, 7) + int(state.Menu.Variant)) % 7
	pitch, err := merchantReferences.FormatText(
		references.MerchantTextID("PRO_PITCH_"+strconv.Itoa(index)),
		references.MerchantTextValues{Price: state.Provision.PricePerPack},
	)
	if err != nil {
		return "", fmt.Errorf("formatting Tavern provision pitch: %w", err)
	}

	return quoteMerchantConversation(pitch), nil
}

func (g *GameScene) resolveTavernInformationProse(
	state game_state.TavernShopState,
	topic references.TavernInformationTopic,
) (tavernInformationProse, error) {
	merchantReferences, err := g.tavernMerchantReferences()
	if err != nil {
		return tavernInformationProse{}, err
	}
	topicOffset := -1
	for index, candidate := range merchantReferences.TavernInformationTopics() {
		if candidate == topic {
			topicOffset = index

			break
		}
	}
	if topicOffset < 0 {
		return tavernInformationProse{}, fmt.Errorf("%w: %q", errTavernInformationProseTopic, topic.Key)
	}

	return formatTavernInformationProse(merchantReferences, state, topic, topicOffset)
}

func formatTavernInformationProse(
	merchantReferences *references.MerchantReferences,
	state game_state.TavernShopState,
	topic references.TavernInformationTopic,
	topicOffset int,
) (tavernInformationProse, error) {
	values := references.MerchantTextValues{
		Price:         topic.Price,
		InformantName: topic.Informant,
		LocationName:  topic.Location,
	}
	pitch, err := merchantReferences.FormatText(references.MerchantTextID("AMNESIA"), values)
	if err != nil {
		return tavernInformationProse{}, fmt.Errorf("formatting Tavern information pitch: %w", err)
	}
	answerIndex := (deterministicMerchantProseIndex(state.Merchant.ID, 4) + topicOffset) % 4
	answer, err := merchantReferences.FormatText(
		references.MerchantTextID("SENS_"+strconv.Itoa(answerIndex)), values,
	)
	if err != nil {
		return tavernInformationProse{}, fmt.Errorf("formatting Tavern information answer: %w", err)
	}

	return tavernInformationProse{
		pitch:  quoteMerchantConversation(pitch),
		answer: quoteMerchantConversation(answer),
	}, nil
}

func (g *GameScene) tavernMerchantReferences() (*references.MerchantReferences, error) {
	if g.gameState == nil || g.gameState.GameReferences == nil ||
		g.gameState.GameReferences.MerchantReferences == nil {
		return nil, fmt.Errorf(
			"resolving Tavern prose: merchant references are unavailable: %w",
			game_state.ErrInvalidMerchantSession,
		)
	}

	return g.gameState.GameReferences.MerchantReferences, nil
}

func tavernMerchantText(
	merchantReferences *references.MerchantReferences,
	id references.MerchantTextID,
) (string, error) {
	value, ok := merchantReferences.Text(id)
	if !ok {
		return "", fmt.Errorf("%w: %q", errTavernMerchantText, id)
	}

	return strings.TrimSpace(value), nil
}

func resolveTavernMealResultDisplay(
	prose tavernShopWorkflowProse,
	result game_state.TavernMealResult,
) widgets.TavernActionResultDisplay {
	switch result.Outcome {
	case game_state.TavernMealCompleted:
		return widgets.TavernActionResultDisplay{
			Action:           widgets.TavernActionMeal,
			MerchantExchange: prose.politeFarewell,
		}
	case game_state.TavernMealInsufficientFunds:
		return widgets.TavernActionResultDisplay{
			Action:           widgets.TavernActionMeal,
			MerchantExchange: prose.rudeFarewell,
			Dismiss:          true,
		}
	case game_state.TavernMealOutcomeUnknown:
		panic("Tavern meal result has no outcome")
	default:
		panic("Tavern meal result has an unknown outcome")
	}
}

func tavernMealResultView(result game_state.TavernMealResult) widgets.TavernResultView {
	view := widgets.TavernResultView{
		Title: "Meal Served", Item: result.Meal.Name,
		Detail: fmt.Sprintf(
			"%d %s x %d gold each", result.DinerCount, dinerWord(result.DinerCount), result.Meal.BasePrice,
		),
		ChargeLabel: "Total", Charge: int(result.Charge),
		ShowFood: true, FoodBefore: result.FoodBefore, FoodAfter: result.FoodAfter,
		ShowGold: true, GoldBefore: result.GoldBefore, GoldAfter: result.GoldAfterTheft,
		Theft: result.Theft, Tone: widgets.TavernResultSuccess,
	}
	if result.Outcome == game_state.TavernMealInsufficientFunds {
		view.Title = "Meal Refused"
		view.ChargeLabel = "Required"
		view.Note = "Thou hast not enough gold for this meal."
		view.Tone = widgets.TavernResultFailure
	}

	return view
}

func (g *GameScene) resolveTavernDrinkResultDisplay(
	state game_state.TavernShopState,
	prose tavernShopWorkflowProse,
	result game_state.TavernDrinkResult,
) widgets.TavernActionResultDisplay {
	switch result.Outcome {
	case game_state.TavernDrinkPurchased:
		exchange := fmt.Sprintf(
			"\"That will be %d gold for the %s of ye, %s. Enjoy!\"\n\n%s",
			result.Charge, tavernPartySizeWord(result.DinerCount), g.tavernBuyerHonorific(state),
			prose.dialog.ConversationExit,
		)
		if result.WineIndex != game_state.TavernOrdinaryDrinkWineIndex {
			exchange = fmt.Sprintf(
				"\"Ah, a fine choice, %s. Enjoy!\"\n\n%s",
				g.tavernBuyerHonorific(state), prose.dialog.ConversationExit,
			)
		}

		return widgets.TavernActionResultDisplay{
			Action: widgets.TavernActionDrink, MerchantExchange: exchange,
		}
	case game_state.TavernDrinkInsufficientFunds:
		return widgets.TavernActionResultDisplay{
			Action: widgets.TavernActionDrink, MerchantExchange: prose.rudeFarewell, Dismiss: true,
		}
	case game_state.TavernDrinkCancelled:
		panic("Tavern approval emitted a cancelled drink request")
	case game_state.TavernDrinkOutcomeUnknown:
		panic("Tavern drink result has no outcome")
	default:
		panic("Tavern drink result has an unknown outcome")
	}
}

func resolveTavernStopResultDisplay(
	prose tavernShopWorkflowProse,
	result game_state.TavernStopResult,
) widgets.TavernActionResultDisplay {
	if result.Outcome != game_state.TavernStopAccepted {
		panic("Tavern Stop result was not accepted")
	}

	return widgets.TavernActionResultDisplay{
		Action:           widgets.TavernActionStop,
		MerchantExchange: prose.prudentChoice,
	}
}

func (g *GameScene) resolveTavernProvisionResultDisplay(
	state game_state.TavernShopState,
	result game_state.TavernProvisionResult,
	prose tavernShopWorkflowProse,
) (widgets.TavernActionResultDisplay, error) {
	display := widgets.TavernActionResultDisplay{Action: widgets.TavernActionProvision}
	switch result.Outcome {
	case game_state.TavernProvisionCompleted, game_state.TavernProvisionPartial,
		game_state.TavernProvisionSaturated:
		display.MerchantExchange = prose.politeFarewell
	case game_state.TavernProvisionCharityEjection:
		merchantReferences, err := g.tavernMerchantReferences()
		if err != nil {
			return widgets.TavernActionResultDisplay{}, err
		}
		text, err := merchantReferences.FormatText(
			references.MerchantTextID("BAR_MISC_1"),
			references.MerchantTextValues{OwnerName: state.Merchant.OwnerName},
		)
		if err != nil {
			return widgets.TavernActionResultDisplay{}, fmt.Errorf("formatting Tavern charity ejection: %w", err)
		}
		display.MerchantExchange = quoteMerchantConversation(text)
		display.Dismiss = true
	case game_state.TavernProvisionEjection:
		display.MerchantExchange = quoteMerchantConversation(
			"Thou hast neither gold nor need! Out!",
		) + " yells " + state.Merchant.OwnerName + "."
		display.Dismiss = true
	case game_state.TavernProvisionOutcomeUnknown:
		panic("Tavern provision result has no outcome")
	default:
		panic("Tavern provision result has an unknown outcome")
	}

	return display, nil
}

func tavernProvisionResultView(result game_state.TavernProvisionResult) widgets.TavernResultView {
	view := widgets.TavernResultView{
		Title: "Supplies Stowed", Item: result.Provision.Name,
		Detail:      fmt.Sprintf("Packs bought: %d of %d", result.PurchasedPacks, result.RequestedPacks),
		ChargeLabel: "Paid", Charge: result.TotalCost,
		ShowFood: true, FoodBefore: result.FoodBefore, FoodAfter: result.FoodAfter,
		ShowGold: true, GoldBefore: result.GoldBefore, GoldAfter: result.GoldAfterTheft,
		Theft: result.Theft, Tone: widgets.TavernResultSuccess,
	}
	switch result.Outcome {
	case game_state.TavernProvisionCompleted:
	case game_state.TavernProvisionPartial:
		view.Title = "Partial Order"
		view.Note = "The rest could not be paid for."
		view.Tone = widgets.TavernResultCaution
	case game_state.TavernProvisionSaturated:
		view.Title = "Supply Limit Reached"
		view.Note = "Carried food has reached 9999."
		view.Tone = widgets.TavernResultCaution
	case game_state.TavernProvisionCharityEjection:
		view.Title = "A Little Charity"
		view.Detail = fmt.Sprintf("The keeper offered %d food", result.ActualFood)
		view.Note = "The keeper ends this Tavern visit."
		view.Tone = widgets.TavernResultFailure
	case game_state.TavernProvisionEjection:
		view.Title = "Order Refused"
		view.Detail = "No packs bought"
		view.Note = "The keeper ends the order."
		view.Tone = widgets.TavernResultFailure
	case game_state.TavernProvisionOutcomeUnknown:
		panic("Tavern provision result view has no outcome")
	default:
		panic("Tavern provision result view has unknown outcome")
	}

	return view
}

func (g *GameScene) resolveTavernInformationResultDisplay(
	state game_state.TavernShopState,
	result game_state.TavernInformationResult,
	rawQuestion string,
) (widgets.TavernActionResultDisplay, error) {
	switch result.Outcome {
	case game_state.TavernInformationCompleted:
		prose, err := g.resolveTavernInformationProse(state, result.Topic)
		if err != nil {
			return widgets.TavernActionResultDisplay{}, err
		}

		return widgets.TavernActionResultDisplay{
			Action:           widgets.TavernActionInformation,
			MerchantExchange: prose.answer + " says " + state.Merchant.OwnerName + ".",
			PaidInformation:  &widgets.TavernTranscriptEntry{Question: rawQuestion, Answer: prose.answer},
		}, nil
	case game_state.TavernInformationInsufficientFunds:
		merchantReferences, err := g.tavernMerchantReferences()
		if err != nil {
			return widgets.TavernActionResultDisplay{}, err
		}
		misc, err := merchantReferences.FormatText(
			references.MerchantTextID("BAR_MISC_2"),
			references.MerchantTextValues{OwnerName: state.Merchant.OwnerName},
		)
		if err != nil {
			return widgets.TavernActionResultDisplay{}, fmt.Errorf("formatting Tavern information refusal: %w", err)
		}

		return widgets.TavernActionResultDisplay{
			Action:           widgets.TavernActionInformation,
			MerchantExchange: quoteMerchantConversation("Sorry, " + g.tavernBuyerHonorific(state) + strings.TrimSpace(misc)),
			Dismiss:          true,
		}, nil
	case game_state.TavernInformationOutcomeUnknown:
		panic("Tavern information result has no outcome")
	default:
		panic("Tavern information result has an unknown outcome")
	}
}

func (g *GameScene) tavernBuyerHonorific(state game_state.TavernShopState) string {
	if buyer := g.gameState.PartyState.GetCharacter(state.PricingCharacterIndex); buyer.Gender == party_state.Female {
		return "milady"
	}

	return "sir"
}

func tavernPartySizeWord(size int) string {
	switch size {
	case 1:
		return "one"
	case 2:
		return "two"
	case 3:
		return "three"
	case 4:
		return "four"
	case 5:
		return "five"
	case 6:
		return "six"
	default:
		return strconv.Itoa(size)
	}
}

func dinerWord(count int) string {
	if count == 1 {
		return "diner"
	}

	return "diners"
}
