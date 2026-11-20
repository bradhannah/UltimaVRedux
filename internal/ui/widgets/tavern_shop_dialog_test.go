package widgets

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func TestTavernShopDialogLocationTabsCanonicalRowsAndBrightActiveMode(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	tests := []struct {
		name          string
		state         game_state.TavernShopState
		information   string
		shortcut      ebiten.Key
		wantOrderRows []string
	}{
		{
			name: "mutton ale rations", state: testTavernState(references.TavernMenuMuttonAleRations),
			information: "Chat", shortcut: ebiten.KeyC, wantOrderRows: []string{"Mutton", "Ale", "Rations"},
		},
		{
			name: "wine cheese", state: testTavernState(references.TavernMenuWineCheese),
			information: "Talk", shortcut: ebiten.KeyT, wantOrderRows: []string{"Cheese", "Wine"},
		},
		{
			name: "rum boar", state: testTavernState(references.TavernMenuRumBoar),
			information: "Help", shortcut: ebiten.KeyH, wantOrderRows: []string{"Boar", "Rum"},
		},
		{
			name: "stout fruit provisions", state: testTavernState(references.TavernMenuStoutFruitProvisions),
			information: "Advice", shortcut: ebiten.KeyA,
			wantOrderRows: []string{"Fruit", "Stout", "Provisions"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			keyboard := newWidgetTestKeyboard(0)
			dialog := newTestTavernShopDialog(keyboard, test.state, nil)

			require.Equal(t, TavernShopOrder, dialog.GetMode())
			require.Contains(t, dialog.GetModeHeaderText(), "Order")
			require.Contains(t, dialog.GetModeHeaderText(), test.information)
			require.NotContains(t, dialog.GetModeHeaderText(), "[", "the tab titles carry no key letters")
			require.Equal(t, []shopTab{{"Order", true}, {test.information, false}}, dialog.tabs())
			require.Contains(t, dialog.GetShell().GetFooterView().Guidance, "[Tab] Order or "+test.information)
			require.Equal(t, test.wantOrderRows, tavernOrderRowNames(dialog.GetOrderRows()))
			require.Contains(t, dialog.GetShell().GetFooterView().Guidance, "[Enter] Order")
			require.Contains(t, dialog.GetShell().GetFooterView().Guidance, "[Esc] Leave")
			workspace := compactTavernText(dialog.GetWorkspaceText())
			require.Contains(t, workspace, "serve 3 party members")
			require.Contains(t, workspace, fmt.Sprintf("%d each", test.state.Menu.Meal.BasePrice))
			require.Contains(t, workspace, fmt.Sprintf("%d total", test.state.Meal.Charge))

			pressTavernShopKey(dialog, keyboard, test.shortcut)
			require.Equal(t, TavernShopConversation, dialog.GetMode())
			require.Equal(t, []shopTab{{"Order", false}, {test.information, true}}, dialog.tabs())
			require.Contains(t, dialog.GetShell().GetFooterView().Guidance, "[Tab/Esc] Back to Order")
			require.Contains(t, compactTavernText(dialog.GetWorkspaceText()), "becomes available after an order")
			pressTavernShopKey(dialog, keyboard, ebiten.KeyLeft)
			require.Equal(t, TavernShopOrder, dialog.GetMode())
			pressTavernShopKey(dialog, keyboard, ebiten.KeyTab)
			require.Equal(t, TavernShopConversation, dialog.GetMode())

			visible := strings.ToLower(dialog.GetModeHeaderText() + dialog.GetWorkspaceText() +
				dialog.GetShell().GetFooterView().Guidance)
			require.NotContains(t, visible, "mouse")
			require.NotContains(t, visible, "click")
		})
	}
}

func TestTavernOrderRowsUseSharedSelectionBarAndStateUnaffordableGold(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	state := testTavernState(references.TavernMenuRumBoar)
	setTestTavernGold(&state, 0)
	dialog := newTestTavernShopDialog(newWidgetTestKeyboard(0), state, nil)
	workspace := compactTavernText(dialog.GetWorkspaceText())
	require.Contains(t, workspace, fmt.Sprintf("(need %d gold)", state.Meal.Charge))
	require.NotContains(t, workspace, "> [")
	require.False(t, dialog.rowSelectionBounds.Empty())
}

func TestTavernShopDialogLockedConversationHidesTopicsAndUnlockedInputIsVisible(t *testing.T) {
	queue := newTavernRuneQueue()
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestTavernShopDialog(keyboard, testTavernState(references.TavernMenuMuttonAleRations), queue.Append)

	pressTavernShopKey(dialog, keyboard, ebiten.KeyC)
	locked := strings.ToLower(compactTavernText(dialog.GetWorkspaceText()))
	require.Contains(t, locked, "becomes available after an order")
	for _, undiscovered := range []string{"hone", "comp", "crown", "scepter", "shrine"} {
		require.NotContains(t, locked, undiscovered)
	}
	queue.Push("hone")
	dialog.Update()
	queue.Push("")
	require.Empty(t, dialog.GetTopicInput(), "locked conversation must not accept topic input")

	refreshed := dialog.GetState()
	refreshed.CustomerEligible = true
	dialog.ApplyState(refreshed)
	queue.Push("honesty")
	dialog.Update()
	queue.Push("")
	require.Equal(t, "honesty", dialog.GetTopicInput())
	workspace := strings.ToLower(compactTavernText(dialog.GetWorkspaceText()))
	require.Contains(t, workspace, "subject: honesty_")

	pressTavernShopKey(dialog, keyboard, ebiten.KeyEnter)
	topic, ok := dialog.ConsumePendingTopicIntent()
	require.True(t, ok)
	require.Equal(t, TavernTopicIntent{Input: "honesty"}, topic)
	pressTavernShopKey(dialog, keyboard, ebiten.KeyEnter)
	_, duplicate := dialog.ConsumePendingTopicIntent()
	require.False(t, duplicate, "awaiting authority must block duplicate topic submission")

	dialog.RejectInformationTopic(topic)
	require.Equal(t, TavernShopBrowse, dialog.GetPhase())
	require.Equal(t, TavernShopConversation, dialog.GetMode())
	require.Empty(t, dialog.GetTopicInput())
	require.Empty(t, dialog.GetInlineFeedback())
	require.Contains(t, dialog.GetLatestExchange(), `"That, I cannot help thee with."`)
	require.Contains(t, dialog.GetLatestExchange(), "Of what wouldst thou hear my lore")
	require.NotContains(t, strings.ToLower(dialog.GetWorkspaceText()), "honesty")
}

func TestTavernShopDialogSpacesSubmitAsUnknownTopic(t *testing.T) {
	queue := newTavernRuneQueue()
	state := testTavernState(references.TavernMenuMuttonAleRations)
	state.CustomerEligible = true
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestTavernShopDialog(keyboard, state, queue.Append)

	pressTavernShopKey(dialog, keyboard, ebiten.KeyC)
	queue.Push("   ")
	dialog.Update()
	queue.Push("")
	pressTavernShopKey(dialog, keyboard, ebiten.KeyEnter)

	topic, ok := dialog.ConsumePendingTopicIntent()
	require.True(t, ok)
	require.Equal(t, TavernTopicIntent{Input: "   "}, topic)
	dialog.RejectInformationTopic(topic)
	require.Equal(t, TavernShopConversation, dialog.GetMode())
	require.Contains(t, dialog.GetLatestExchange(), `"That, I cannot help thee with."`)
}

func TestTavernShopDialogRuneEventsPreserveRapidRepeatedInputLimitAndBackspace(t *testing.T) {
	held := map[ebiten.Key]bool{}
	queue := newTavernRuneQueue()
	state := testTavernState(references.TavernMenuMuttonAleRations)
	state.CustomerEligible = true
	config := testTavernShopConfig(newWidgetTestKeyboard(0), state, queue.Append)
	config.IsKeyPressed = func(key ebiten.Key) bool { return held[key] }
	dialog := NewTavernShopDialog(config)

	held[ebiten.KeyC] = true
	queue.Push("c")
	pressTavernShopKey(dialog, config.Keyboard, ebiten.KeyC)
	require.Equal(t, TavernShopConversation, dialog.GetMode())
	require.Empty(t, dialog.GetTopicInput(), "the shortcut frame must not be interpreted as subject text")
	require.Equal(t, []rune("c"), queue.Append(nil), "AppendInputChars is a frame snapshot, not a drain")

	queue.Push("bookkeeper")
	dialog.Update()
	require.Empty(t, dialog.GetTopicInput(), "held conversation shortcut must gate text entry")
	held[ebiten.KeyC] = false
	queue.Push("bookkeeper")
	dialog.Update()
	require.Equal(t, "bookkeeper", dialog.GetTopicInput(), "release frame must not drop rapid or repeated runes")

	queue.Push("!!世界abcdef")
	dialog.Update()
	queue.Push("")
	require.Equal(t, tavernTopicRuneLimit, len([]rune(dialog.GetTopicInput())))
	require.Equal(t, "bookkeeper!!世界a", dialog.GetTopicInput())

	pressTavernShopKey(dialog, config.Keyboard, ebiten.KeyBackspace)
	require.Equal(t, "bookkeeper!!世界", dialog.GetTopicInput())
}

func TestTavernShopDialogConversationAcceptsPhysicalOrderLetterAsText(t *testing.T) {
	queue := newTavernRuneQueue()
	state := testTavernState(references.TavernMenuMuttonAleRations)
	state.CustomerEligible = true
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestTavernShopDialog(keyboard, state, queue.Append)
	pressTavernShopKey(dialog, keyboard, ebiten.KeyC)

	keyboard.SimulateKeyPress(ebiten.KeyO)
	queue.Push("honor")
	dialog.Update()
	queue.Push("")

	require.Equal(t, TavernShopConversation, dialog.GetMode())
	require.Equal(t, "honor", dialog.GetTopicInput())
}

func TestTavernShopDialogHeldEnterEscapeAndShortcutDoNotLeakAcrossTransitions(t *testing.T) {
	held := map[ebiten.Key]bool{ebiten.KeyEnter: true, ebiten.KeyEscape: true}
	queue := newTavernRuneQueue()
	state := testTavernState(references.TavernMenuMuttonAleRations)
	state.CustomerEligible = true
	config := testTavernShopConfig(newWidgetTestKeyboard(0), state, queue.Append)
	config.IsKeyPressed = func(key ebiten.Key) bool { return held[key] }
	dialog := NewTavernShopDialog(config)

	// Constructor release gating prevents held controls from acting on the parent.
	dialog.Update()
	require.Equal(t, TavernShopOrder, dialog.GetMode())
	require.False(t, dialog.IsDismissed())
	held[ebiten.KeyEnter] = false
	held[ebiten.KeyEscape] = false
	dialog.Update()

	pressTavernShopKey(dialog, config.Keyboard, ebiten.KeyC)
	queue.Push("valor")
	dialog.Update()
	queue.Push("")
	require.Equal(t, "valor", dialog.GetTopicInput())

	held[ebiten.KeyEnter] = true
	dialog.prepareForTransition()
	dialog.Update()
	require.False(t, dialog.HasPendingTopicIntent())
	held[ebiten.KeyEnter] = false
	dialog.Update()
	require.False(t, dialog.HasPendingTopicIntent(), "held Enter release must not submit")
	pressTavernShopKey(dialog, config.Keyboard, ebiten.KeyEnter)
	require.True(t, dialog.HasPendingTopicIntent())

	topic, ok := dialog.ConsumePendingTopicIntent()
	require.True(t, ok)
	dialog.RejectInformationTopic(topic)
	held[ebiten.KeyEscape] = true
	dialog.prepareForTransition()
	dialog.Update()
	require.Equal(t, TavernShopConversation, dialog.GetMode())
	held[ebiten.KeyEscape] = false
	dialog.Update()
	require.Equal(t, TavernShopConversation, dialog.GetMode(), "held Escape release must not leave typing")
	pressTavernShopKey(dialog, config.Keyboard, ebiten.KeyEscape)
	require.Equal(t, TavernShopOrder, dialog.GetMode())
	require.False(t, dialog.IsDismissed())
}

func TestTavernShopDialogMealDrinkWineProvisionAndEscapePaths(t *testing.T) {
	t.Run("meal and ordinary drink", func(t *testing.T) {
		keyboard := newWidgetTestKeyboard(0)
		dialog := newTestTavernShopDialog(keyboard, testTavernState(references.TavernMenuMuttonAleRations), nil)

		pressTavernShopKey(dialog, keyboard, ebiten.KeyEnter)
		meal, ok := dialog.ConsumePendingActionIntent()
		require.True(t, ok)
		require.Equal(t, TavernActionMeal, meal.Action)
		require.Equal(t, dialog.GetState().Meal.Charge, meal.Request.Meal.ReviewedCharge)
		dialog.CancelActionApproval()

		pressTavernShopKey(dialog, keyboard, ebiten.KeyDown)
		pressTavernShopKey(dialog, keyboard, ebiten.KeyEnter)
		drink, ok := dialog.ConsumePendingActionIntent()
		require.True(t, ok)
		require.Equal(t, TavernActionDrink, drink.Action)
		require.Equal(t, game_state.TavernDrinkApprovalPurchase, drink.Request.Drink.Approval)
		require.Equal(t, game_state.TavernOrdinaryDrinkWineIndex, drink.Request.Drink.ReviewedWineIndex)
		require.Equal(t, emptyTavernActionRequest(), drink.StopRequest)
	})

	t.Run("wine picker", func(t *testing.T) {
		keyboard := newWidgetTestKeyboard(0)
		dialog := newTestTavernShopDialog(keyboard, testTavernState(references.TavernMenuWineCheese), nil)

		pressTavernShopKey(dialog, keyboard, ebiten.KeyW)
		require.Equal(t, TavernOrderWinePicker, dialog.GetOrderSubflow())
		pressTavernShopKey(dialog, keyboard, ebiten.KeyDown)
		require.Equal(t, 1, dialog.GetSelectedWineIndex())
		pressTavernShopKey(dialog, keyboard, ebiten.KeyEscape)
		require.Equal(t, TavernOrderMenu, dialog.GetOrderSubflow())
		require.False(t, dialog.IsDismissed())

		pressTavernShopKey(dialog, keyboard, ebiten.KeyW)
		pressTavernShopKey(dialog, keyboard, ebiten.KeyE)
		intent, ok := dialog.ConsumePendingActionIntent()
		require.True(t, ok)
		require.Equal(t, 4, intent.Drink.WineIndex)
		require.Equal(t, "Moselle", intent.Drink.Wine.Name)
		require.Equal(t, uint16(275), intent.Drink.Charge)
		dialog.AcceptActionApproval(intent.Request)
		refreshed := dialog.GetState()
		refreshed.CustomerEligible = true
		dialog.ApplyResult(intent.Request, refreshed, TavernActionResultDisplay{
			Action: TavernActionDrink, MerchantExchange: `"Ah, a fine choice, sir. Enjoy!"`,
			Receipt: "The wine is served.", SuppressInlineReceipt: true,
		})
		require.Equal(t, TavernOrderMenu, dialog.GetOrderSubflow())
		require.Contains(t, dialog.GetWorkspaceText(), "ORDER")
		require.Empty(t, dialog.GetReceiptText())
	})

	t.Run("provision quantity modal", func(t *testing.T) {
		keyboard := newWidgetTestKeyboard(0)
		dialog := newTestTavernShopDialog(keyboard, testTavernState(references.TavernMenuStoutFruitProvisions), nil)

		pressTavernShopKey(dialog, keyboard, ebiten.KeyP)
		intent, ok := dialog.ConsumePendingActionIntent()
		require.True(t, ok)
		require.Equal(t, TavernActionProvision, intent.Action)
		require.Equal(t, 1, intent.PackCount)
		intent.ProvisionPitch = `"Fresh provisions for the road."`
		action := NewTavernActionDialog(TavernActionDialogConfig{
			Keyboard: keyboard, Intent: intent, IsKeyPressed: keyboard.IsKeyHeld,
		})
		pressTavernActionKey(action, keyboard, ebiten.KeyRight)
		require.Equal(t, 2, action.GetProvisionPackCount())
		body := compactTavernText(action.GetBodyText())
		require.Contains(t, body, "FOOD 50 -> 100")
		require.Contains(t, body, "PRICE EACH / TOTAL 30 / 60 GOLD")
		require.Contains(t, body, "GOLD 1000 -> 940")
		pressTavernActionKey(action, keyboard, ebiten.KeyM)
		require.Equal(t, intent.Provision.MaximumUsefulPacks, action.GetProvisionPackCount())
		require.Contains(t, compactTavernText(action.GetBodyText()), "Only 33 of 99 packs can currently be paid for.")
		pressTavernActionKey(action, keyboard, ebiten.KeyEnter)
		request, ok := action.ConsumePendingRequest()
		require.True(t, ok)
		require.Equal(t, intent.Provision.MaximumUsefulPacks, request.Provision.PackCount)
		require.NotPanics(t, func() {
			dialog.AcceptActionApproval(request)
			dialog.ApplyResult(request, dialog.GetState(), TavernActionResultDisplay{
				Action: TavernActionProvision, MerchantExchange: "Very well.", Receipt: "Provision test result.",
			})
		})
		require.Equal(t, TavernOrderMenu, dialog.GetOrderSubflow())
		pressTavernShopKey(dialog, keyboard, ebiten.KeyEscape)
		require.True(t, dialog.IsDismissed())
	})
}

func TestTavernShopDialogWarnedStopReturnsToOrderAfterRefresh(t *testing.T) {
	state := testTavernState(references.TavernMenuMuttonAleRations)
	setTestTavernDrunk(&state, 3)
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestTavernShopDialog(keyboard, state, nil)
	pressTavernShopKey(dialog, keyboard, ebiten.KeyA)

	intent, ok := dialog.ConsumePendingActionIntent()
	require.True(t, ok)
	require.True(t, intent.Drink.Warning)
	require.Equal(t, game_state.TavernDrinkApprovalContinue, intent.Request.Drink.Approval)
	require.Equal(t, TavernActionStop, intent.StopRequest.Action)
	require.Equal(t, game_state.TavernStopRequest{
		ReviewedMerchantID: state.Merchant.ID,
		ReviewedDrunk:      3,
	}, intent.StopRequest.Stop)

	dialog.AcceptActionApproval(intent.StopRequest)
	refreshed := state
	refreshed.CustomerEligible = true
	dialog.ApplyResult(intent.StopRequest, refreshed, TavernActionResultDisplay{
		Action: TavernActionStop, MerchantExchange: "A prudent choice.",
		Receipt: "No payment was made.", SuppressInlineReceipt: true,
	})
	require.Equal(t, TavernShopOrder, dialog.GetMode())
	require.Equal(t, TavernShopBrowse, dialog.GetPhase())
	require.True(t, dialog.GetState().CustomerEligible)
	require.Empty(t, dialog.GetTopicInput())
}

func TestTavernShopDialogInformationWorkflowFullRefreshTranscriptAndScroll(t *testing.T) {
	setMerchantShellTestResolution(t, 320, 180)
	queue := newTavernRuneQueue()
	state := testTavernState(references.TavernMenuMuttonAleRations)
	state.CustomerEligible = true
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestTavernShopDialog(keyboard, state, queue.Append)
	pressTavernShopKey(dialog, keyboard, ebiten.KeyC)
	longAnswer := strings.TrimSpace(strings.Repeat("canonical response ", 12))

	for index := range 8 {
		question := fmt.Sprintf("question %d", index)
		queue.Push(question)
		dialog.Update()
		queue.Push("")
		pressTavernShopKey(dialog, keyboard, ebiten.KeyEnter)
		topic, ok := dialog.ConsumePendingTopicIntent()
		require.True(t, ok)
		quote := testTavernInformationQuote(dialog.GetState(), "hone", 50)
		dialog.ApplyInformationQuote(topic, quote, "I may know someone.")
		intent, ok := dialog.ConsumePendingActionIntent()
		require.True(t, ok)
		require.Equal(t, TavernActionInformation, intent.Action)
		require.Equal(t, quote.Topic, intent.Request.Information.ReviewedTopic)
		dialog.AcceptActionApproval(intent.Request)

		refreshed := dialog.GetState()
		setTestTavernGold(&refreshed, refreshed.Gold-50)
		answer := fmt.Sprintf("answer %d", index)
		if index == 7 {
			answer = longAnswer
		}
		dialog.ApplyResult(intent.Request, refreshed, TavernActionResultDisplay{
			Action: TavernActionInformation, MerchantExchange: "I know someone.", Receipt: fmt.Sprintf(
				"Information: payment 50; final gold %d; subject hone.", refreshed.Gold,
			),
			PaidInformation: &TavernTranscriptEntry{Question: question, Answer: answer},
		})
		require.Equal(t, TavernShopConversation, dialog.GetMode())
		require.Equal(t, TavernShopBrowse, dialog.GetPhase())
	}

	require.Equal(t, uint16(600), dialog.GetState().Gold)
	require.Equal(t, "600", dialog.GetShell().GetHeaderView().PartyGold)
	transcript := dialog.GetTranscript()
	require.Len(t, transcript, 8)
	require.Equal(t, TavernTranscriptEntry{Question: "question 0", Answer: "answer 0"}, transcript[0])
	require.Equal(t, TavernTranscriptEntry{Question: "question 7", Answer: longAnswer}, transcript[7])
	require.Contains(t, compactTavernText(dialog.GetWorkspaceText()), longAnswer)
	dialog.topicInput = []rune("unmatched")
	dialog.submitTopic()
	unknown, ok := dialog.ConsumePendingTopicIntent()
	require.True(t, ok)
	dialog.RejectInformationTopic(unknown)
	require.Contains(t, dialog.GetLatestExchange(), `"That, I cannot help thee with."`)
	require.Contains(t, compactTavernText(dialog.GetWorkspaceText()), longAnswer)
	transcript[0].Question = "mutated"
	require.Equal(t, "question 0", dialog.GetTranscript()[0].Question)
	require.Greater(t, dialog.GetTranscriptScroll(), 0)

	latestScroll := dialog.GetTranscriptScroll()
	pressTavernShopKey(dialog, keyboard, ebiten.KeyPageUp)
	require.Less(t, dialog.GetTranscriptScroll(), latestScroll)
	pressTavernShopKey(dialog, keyboard, ebiten.KeyPageDown)
	require.Equal(t, latestScroll, dialog.GetTranscriptScroll())
	require.NotEmpty(t, dialog.GetTranscriptViewport())
	require.Contains(t, compactTavernText(dialog.GetWorkspaceText()), longAnswer)

	// A completed information result without a successful paid pair cannot add history.
	dialog.topicInput = []rune("unknown")
	dialog.submitTopic()
	topic, ok := dialog.ConsumePendingTopicIntent()
	require.True(t, ok)
	quote := testTavernInformationQuote(dialog.GetState(), "comp", 75)
	dialog.ApplyInformationQuote(topic, quote, "I may know someone.")
	intent, ok := dialog.ConsumePendingActionIntent()
	require.True(t, ok)
	dialog.AcceptActionApproval(intent.Request)
	dialog.ApplyResult(intent.Request, dialog.GetState(), TavernActionResultDisplay{
		Action: TavernActionInformation, MerchantExchange: "No payment.", Receipt: "Insufficient gold result.",
	})
	require.Len(t, dialog.GetTranscript(), 8)
}

func TestTavernShopDialogInformationQuoteVisibilityCancelAndPreviewPurity(t *testing.T) {
	queue := newTavernRuneQueue()
	state := testTavernState(references.TavernMenuMuttonAleRations)
	state.CustomerEligible = true
	keyboard := newWidgetTestKeyboard(0)
	config := testTavernShopConfig(keyboard, state, queue.Append)
	config.Preview = true
	dialog := NewTavernShopDialog(config)
	pressTavernShopKey(dialog, keyboard, ebiten.KeyC)
	queue.Push("honesty")
	dialog.Update()
	queue.Push("")
	pressTavernShopKey(dialog, keyboard, ebiten.KeyEnter)
	topic, ok := dialog.ConsumePendingTopicIntent()
	require.True(t, ok)

	quote := testTavernInformationQuote(dialog.GetState(), "hone", 50)
	dialog.ApplyInformationQuote(topic, quote, "I may know someone.")
	intent, ok := dialog.ConsumePendingActionIntent()
	require.True(t, ok)
	require.Equal(t, "hone", intent.Information.Topic.Key)
	dialog.CancelActionApproval()
	require.Equal(t, "honesty", dialog.GetTopicInput())
	require.Empty(t, dialog.GetTranscript())

	pressTavernShopKey(dialog, keyboard, ebiten.KeyEscape)
	pressTavernShopKey(dialog, keyboard, ebiten.KeyEnter)
	meal, ok := dialog.ConsumePendingActionIntent()
	require.True(t, ok)
	before := dialog.GetState()
	dialog.AcceptActionApproval(meal.Request)
	dialog.PreviewStop(meal.Request)
	require.Equal(t, before, dialog.GetState())
	require.Empty(t, dialog.GetTranscript())
	require.Contains(t, dialog.GetReceiptText(), "PREVIEW")
	require.Contains(t, dialog.GetReceiptText(), "unchanged")
}

func TestTavernShopDialogResponsiveDrawSessionErrorAndLifecycle(t *testing.T) {
	setMerchantShellTestResolution(t, 320, 180)
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestTavernShopDialog(keyboard, testTavernState(references.TavernMenuWineCheese), nil)
	require.False(t, dialog.GetWorkspaceBounds().Empty())
	require.True(t, dialog.IsVisible())
	require.False(t, dialog.IsComplete())

	dialog.ApplySessionError("The Tavern session expired.")
	require.Equal(t, TavernShopSessionError, dialog.GetPhase())
	require.Contains(t, dialog.GetLatestExchange(), "expired")
	pressTavernShopKey(dialog, keyboard, ebiten.KeyEnter)
	require.True(t, dialog.IsDismissed())
	require.True(t, dialog.IsComplete())
	require.False(t, dialog.IsVisible())
	dialog.ShowWithoutAnimation()
	require.False(t, dialog.IsVisible(), "completed parent cannot be shown again")
}

type tavernRuneQueue struct {
	frame []rune
}

func newTavernRuneQueue() *tavernRuneQueue { return &tavernRuneQueue{} }

func (q *tavernRuneQueue) Push(value string) {
	// Explicitly advance the frame; reading never consumes Ebitengine's snapshot.
	q.frame = []rune(value)
}

func (q *tavernRuneQueue) Append(dst []rune) []rune {
	return append(dst, q.frame...)
}

func newTestTavernShopDialog(
	keyboard *input.Keyboard,
	state game_state.TavernShopState,
	appendInputChars func([]rune) []rune,
) *TavernShopDialog {
	return NewTavernShopDialog(testTavernShopConfig(keyboard, state, appendInputChars))
}

func testTavernShopConfig(
	keyboard *input.Keyboard,
	state game_state.TavernShopState,
	appendInputChars func([]rune) []rune,
) TavernShopDialogConfig {
	if appendInputChars == nil {
		appendInputChars = noWidgetTestInputChars
	}
	return TavernShopDialogConfig{
		Keyboard: keyboard, MerchantSprite: ebiten.NewImage(16, 16), State: state,
		Prose: TavernShopDialogProse{
			Greeting: "Welcome to my Tavern.", OrderPrompt: "What will you order?",
			ConversationPrompt: `"Of what wouldst thou hear my lore, sir?"`,
			ConversationExit:   `"Anything else for thee?"`,
			UnknownTopic:       `"That, I cannot help thee with."`,
		},
		IsKeyPressed: func(ebiten.Key) bool { return false }, AppendInputChars: appendInputChars,
	}
}

func testTavernState(variant references.TavernMenuVariant) game_state.TavernShopState {
	menu, merchant := testTavernMenuAndMerchant(variant)
	const (
		gold   = uint16(1000)
		food   = uint16(50)
		diners = 3
	)
	mealCharge := uint16(menu.Meal.BasePrice * diners)
	state := game_state.TavernShopState{
		Merchant: merchant, PricingCharacterIndex: 0, PricingCharacterName: "Avatar",
		PricingIntelligence: 20, Gold: gold, Food: food, DinerCount: diners,
		CustomerEligible: false, Drunk: 0, Smashed: 0, Karma: 50, Menu: menu,
		Meal: game_state.TavernMealQuote{
			Meal: menu.Meal, DinerCount: diners, Charge: mealCharge, Affordable: true,
			GoldBefore: gold, GoldAfterPayment: gold - mealCharge, FoodBefore: food,
		},
		Provision: game_state.TavernProvisionQuote{
			Provision: menu.Provision, PricePerPack: 0, GoldBefore: gold, FoodBefore: food,
		},
	}
	if menu.Drink.IsWineList {
		wines := []references.TavernWine{
			{Name: "Rose", Price: 18}, {Name: "Claret", Price: 192},
			{Name: "Sauterne", Price: 79}, {Name: "Muscatel", Price: 30},
			{Name: "Moselle", Price: 275}, {Name: "Chablis", Price: 98},
		}
		state.Wines = make([]game_state.TavernDrinkQuote, len(wines))
		for index, wine := range wines {
			state.Wines[index] = testTavernDrinkQuote(state, index, wine, uint16(wine.Price))
		}
	} else {
		charge := uint16(menu.Drink.UnitPrice * diners)
		state.Drink = testTavernDrinkQuote(state, game_state.TavernOrdinaryDrinkWineIndex, references.TavernWine{}, charge)
	}
	if menu.Provision.Available {
		state.Provision.PricePerPack = menu.Provision.BasePrice
		state.Provision.MaximumUsefulPacks = tavernMaximumProvisionPacks
		state.Provision.Approvals = testTavernProvisionApprovals(state.Provision)
	}

	return state
}

func testTavernProvisionApprovals(
	quote game_state.TavernProvisionQuote,
) []game_state.TavernProvisionApproval {
	approvals := make([]game_state.TavernProvisionApproval, tavernMaximumProvisionPacks)
	for packCount := 1; packCount <= tavernMaximumProvisionPacks; packCount++ {
		purchased := min(packCount, int(quote.GoldBefore)/quote.PricePerPack)
		total := purchased * quote.PricePerPack
		foodAfter := uint16(min(9999, int(quote.FoodBefore)+purchased*int(quote.Provision.PackQuantity)))
		outcome := game_state.TavernProvisionCompleted
		if purchased < packCount {
			outcome = game_state.TavernProvisionPartial
		}
		approvals[packCount-1] = game_state.TavernProvisionApproval{
			PackCount: packCount, PurchasedPacks: purchased, Outcome: outcome,
			TotalCost: total, GoldAfterPurchases: quote.GoldBefore - uint16(total),
			FoodAfter: foodAfter, ActualFood: foodAfter - quote.FoodBefore,
		}
	}

	return approvals
}

func testTavernMenuAndMerchant(
	variant references.TavernMenuVariant,
) (references.TavernMenu, references.MerchantDefinition) {
	merchant := references.MerchantDefinition{
		ID: references.MerchantTavernBritain, Family: references.MerchantFamilyTavern,
		Location: references.Britain, NPCIndex: 3, CatalogRow: 1, OwnerName: "Tika", ShopName: "The Wayfarer Tavern",
	}
	menu := references.TavernMenu{MerchantID: merchant.ID, Variant: variant}
	switch variant {
	case references.TavernMenuMuttonAleRations:
		menu.Meal = references.TavernMeal{Name: "Mutton", Key: 'M', BasePrice: 4}
		menu.Drink = references.TavernDrink{Name: "Ale", Key: 'A', UnitPrice: 1}
		menu.Provision = references.TavernProvision{Name: "Rations", Key: 'R', Available: true, PackQuantity: 25, BasePrice: 15}
		menu.Information = references.TavernInformationMenu{Name: "Chat", Key: 'C'}
	case references.TavernMenuWineCheese:
		merchant.ID = references.MerchantTavernWestBritanny
		merchant.Location = references.WestBritanny
		merchant.OwnerName = "Jaymes"
		merchant.ShopName = "The Blue Boar Tavern"
		menu.MerchantID = merchant.ID
		menu.Meal = references.TavernMeal{Name: "Cheese", Key: 'C', BasePrice: 5}
		menu.Drink = references.TavernDrink{Name: "Wine", Key: 'W', IsWineList: true}
		menu.Provision = references.TavernProvision{PackQuantity: 25, BasePrice: 25}
		menu.Information = references.TavernInformationMenu{Name: "Talk", Key: 'T'}
	case references.TavernMenuRumBoar:
		merchant.ID = references.MerchantTavernYew
		merchant.Location = references.Yew
		merchant.OwnerName = "Gregory"
		merchant.ShopName = "The Slaughtered Lamb"
		menu.MerchantID = merchant.ID
		menu.Meal = references.TavernMeal{Name: "Boar", Key: 'B', BasePrice: 3}
		menu.Drink = references.TavernDrink{Name: "Rum", Key: 'R', UnitPrice: 1}
		menu.Provision = references.TavernProvision{PackQuantity: 25, BasePrice: 25}
		menu.Information = references.TavernInformationMenu{Name: "Help", Key: 'H'}
	case references.TavernMenuStoutFruitProvisions:
		merchant.ID = references.MerchantTavernNewMagincia
		merchant.Location = references.NewMagincia
		merchant.OwnerName = "Felicity"
		merchant.ShopName = "The Humble Palate"
		menu.MerchantID = merchant.ID
		menu.Meal = references.TavernMeal{Name: "Fruit", Key: 'F', BasePrice: 2}
		menu.Drink = references.TavernDrink{Name: "Stout", Key: 'S', UnitPrice: 1}
		menu.Provision = references.TavernProvision{Name: "Provisions", Key: 'P', Available: true, PackQuantity: 25, BasePrice: 30}
		menu.Information = references.TavernInformationMenu{Name: "Advice", Key: 'A'}
	default:
		panic("testTavernMenuAndMerchant: unknown variant")
	}

	return menu, merchant
}

func testTavernDrinkQuote(
	state game_state.TavernShopState,
	wineIndex int,
	wine references.TavernWine,
	charge uint16,
) game_state.TavernDrinkQuote {
	return game_state.TavernDrinkQuote{
		Drink: state.Menu.Drink, WineIndex: wineIndex, Wine: wine, DinerCount: state.DinerCount,
		Charge: charge, Affordable: charge <= state.Gold, Warning: state.Drunk == 3,
		StopAvailable: state.Drunk == 3, GoldBefore: state.Gold,
		GoldAfterPayment: state.Gold - charge, Drunk: state.Drunk, Smashed: state.Smashed, Karma: state.Karma,
	}
}

func testTavernInformationQuote(
	state game_state.TavernShopState,
	key string,
	price int,
) game_state.TavernInformationQuote {
	after := state.Gold
	affordable := int(state.Gold) >= price
	if affordable {
		after -= uint16(price)
	}

	return game_state.TavernInformationQuote{
		Topic: references.TavernInformationTopic{
			Key: key, Price: price, Informant: "Malik", Location: "Moonglow",
		},
		Price: price, Affordable: affordable, GoldBefore: state.Gold, GoldAfterPayment: after,
	}
}

func setTestTavernDrunk(state *game_state.TavernShopState, drunk int) {
	state.Drunk = drunk
	if state.Menu.Drink.IsWineList {
		for index := range state.Wines {
			state.Wines[index].Drunk = drunk
			state.Wines[index].Warning = drunk == 3
			state.Wines[index].StopAvailable = drunk == 3
		}
	} else {
		state.Drink.Drunk = drunk
		state.Drink.Warning = drunk == 3
		state.Drink.StopAvailable = drunk == 3
	}
}

func setTestTavernGold(state *game_state.TavernShopState, gold uint16) {
	state.Gold = gold
	state.Meal.GoldBefore = gold
	state.Meal.Affordable = state.Meal.Charge <= gold
	state.Meal.GoldAfterPayment = gold
	if state.Meal.Affordable {
		state.Meal.GoldAfterPayment -= state.Meal.Charge
	}
	state.Provision.GoldBefore = gold
	if state.Menu.Provision.Available {
		state.Provision.Approvals = testTavernProvisionApprovals(state.Provision)
	}
	if state.Menu.Drink.IsWineList {
		for index := range state.Wines {
			setTestTavernDrinkGold(&state.Wines[index], gold)
		}
	} else {
		setTestTavernDrinkGold(&state.Drink, gold)
	}
}

func setTestTavernDrinkGold(quote *game_state.TavernDrinkQuote, gold uint16) {
	quote.GoldBefore = gold
	quote.Affordable = quote.Charge <= gold
	quote.GoldAfterPayment = gold
	if quote.Affordable {
		quote.GoldAfterPayment -= quote.Charge
	}
}

func tavernOrderRowNames(rows []TavernOrderRow) []string {
	names := make([]string, len(rows))
	for index, row := range rows {
		names[index] = row.Name
	}

	return names
}

func pressTavernShopKey(dialog *TavernShopDialog, keyboard *input.Keyboard, key ebiten.Key) {
	keyboard.SimulateKeyPress(key)
	dialog.Update()
}

func compactTavernText(value string) string { return strings.Join(strings.Fields(value), " ") }
