package widgets

import (
	"math"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func TestReagentShopDialogUsesFullWorkspaceAndProjectionFreeCatalog(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	dialog := newTestReagentShopDialog(newWidgetTestKeyboard(0), testReagentShopState())
	workspace := dialog.GetShell().GetWorkspaceBounds()

	assert.Equal(t, [4]string{"Reagent", "Bundle", "Owned", "Price"}, dialog.GetCatalogHeadings())
	assert.Len(t, dialog.GetCatalogQuotes(), 5)
	assert.True(t, dialog.GetCatalogBounds().In(workspace))
	assert.True(t, dialog.GetDetailBounds().In(workspace))
	assert.Greater(t, dialog.GetCatalogBounds().Dx(), workspace.Dx()*9/10)
	assert.Equal(t, [4]string{"Ginseng", "x10", "10", "20"}, dialog.GetCatalogRowText(0))
	assert.Equal(t, "--", dialog.GetCatalogRowText(1)[3])

	var catalog strings.Builder
	for index := range dialog.GetCatalogQuotes() {
		row := dialog.GetCatalogRowText(index)
		catalog.WriteString(strings.Join(row[:], " "))
	}
	text := strings.ToLower(catalog.String())
	assert.NotContains(t, text, "->")
	assert.NotContains(t, text, "(+")
	assert.NotContains(t, text, "receipt")
	assert.NotContains(t, text, "max")
	assert.Contains(t, dialog.GetDetailText(), "Per bundle: 10 Ginseng for 20 gold.")
	assert.Contains(t, dialog.GetDetailText(), "Max: 9 bundles.")
	assert.NotContains(t, dialog.GetDetailText(), "->")
	assert.Equal(t, "[Up/Down] Select   [Enter] Buy   [Esc] Leave", dialog.GetShell().GetFooterView().Guidance)
	requireReagentStatusConsumesRemainingSpace(t, dialog)
}

func TestReagentShopDialogWrapsSelectionAndClearsReceipt(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestReagentShopDialog(keyboard, testReagentShopState())
	dialog.receiptText = "Old receipt."
	dialog.populateDetailOutput()

	pressReagentShopKey(dialog, keyboard, ebiten.KeyUp)
	assert.Equal(t, 4, dialog.GetSelectedIndex())
	assert.Equal(t, references.MandrakeRoot, dialog.GetSelectedReagent())
	assert.Empty(t, dialog.GetReceiptText())
	assert.NotContains(t, dialog.GetDetailText(), "Old receipt")

	pressReagentShopKey(dialog, keyboard, ebiten.KeyDown)
	assert.Zero(t, dialog.GetSelectedIndex())
	assert.Equal(t, references.Ginseng, dialog.GetSelectedReagent())

	for expected := 1; expected < len(dialog.GetCatalogQuotes()); expected++ {
		pressReagentShopKey(dialog, keyboard, ebiten.KeyDown)
		assert.Equal(t, expected, dialog.GetSelectedIndex())
	}
	for expected := len(dialog.GetCatalogQuotes()) - 2; expected >= 0; expected-- {
		pressReagentShopKey(dialog, keyboard, ebiten.KeyUp)
		assert.Equal(t, expected, dialog.GetSelectedIndex())
	}
}

func TestReagentShopDialogBlockedRowsRemainReadableAndEmitNoIntent(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestReagentShopDialog(keyboard, testReagentShopState())

	pressReagentShopKey(dialog, keyboard, ebiten.KeyDown)
	assert.Equal(t, references.Garlic, dialog.GetSelectedReagent())
	assert.Contains(t, compactReagentText(dialog.GetDetailText()), "not carry any more Garlic")
	nameColor, valueColor := reagentCatalogRowColors(dialog.selectedQuote(), true)
	assert.Equal(t, DialogSelectedText, nameColor)
	assert.Equal(t, DialogSelectedText, valueColor)
	pressReagentShopKey(dialog, keyboard, ebiten.KeyEnter)
	assert.Equal(t, ReagentShopBrowse, dialog.GetPhase())
	assert.False(t, dialog.HasPendingPurchaseIntent())

	pressReagentShopKey(dialog, keyboard, ebiten.KeyDown)
	assert.Equal(t, references.SpiderSilk, dialog.GetSelectedReagent())
	assert.Contains(t, compactReagentText(dialog.GetDetailText()), "need 500 gold per bundle; party has 200")
	nameColor, valueColor = reagentCatalogRowColors(dialog.selectedQuote(), false)
	assert.Equal(t, DialogEmptyColor, nameColor)
	assert.Equal(t, DialogEmptyColor, valueColor)
	pressReagentShopKey(dialog, keyboard, ebiten.KeyEnter)
	assert.Equal(t, ReagentShopBrowse, dialog.GetPhase())
	assert.False(t, dialog.HasPendingPurchaseIntent())

	availableName, availableValue := reagentCatalogRowColors(dialog.state.Offers[0], false)
	assert.Equal(t, DialogItemNameColor, availableName)
	assert.Equal(t, DialogValueColor, availableValue)
	fullName, fullValue := reagentCatalogRowColors(dialog.state.Offers[1], false)
	assert.Equal(t, DialogEmptyColor, fullName)
	assert.Equal(t, DialogEmptyColor, fullValue)
}

func TestReagentShopDialogIntentCancelResultAndIdentityRefresh(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	state := testReagentShopState()
	dialog := newTestReagentShopDialog(keyboard, state)

	pressReagentShopKey(dialog, keyboard, ebiten.KeyEnter)
	require.Equal(t, ReagentShopAwaitingQuantity, dialog.GetPhase())
	intent, ok := dialog.ConsumePendingPurchaseIntent()
	require.True(t, ok)
	assert.Equal(t, references.Ginseng, intent.Quote.Reagent)
	_, duplicate := dialog.ConsumePendingPurchaseIntent()
	assert.False(t, duplicate)
	pressReagentShopKey(dialog, keyboard, ebiten.KeyDown)
	assert.Equal(t, references.Ginseng, dialog.GetSelectedReagent(), "parent must be inert while the child owns input")
	dialog.CancelQuantitySelection()
	assert.Equal(t, ReagentShopBrowse, dialog.GetPhase())

	pressReagentShopKey(dialog, keyboard, ebiten.KeyDown)
	pressReagentShopKey(dialog, keyboard, ebiten.KeyDown)
	require.Equal(t, references.SpiderSilk, dialog.GetSelectedReagent())
	pressReagentShopKey(dialog, keyboard, ebiten.KeyDown)
	require.Equal(t, references.NightShade, dialog.GetSelectedReagent())
	pressReagentShopKey(dialog, keyboard, ebiten.KeyEnter)
	intent, ok = dialog.ConsumePendingPurchaseIntent()
	require.True(t, ok)
	request := game_state.ReagentPurchaseRequest{Reagent: references.NightShade, BundleCount: 2}
	dialog.AcceptQuantitySelection(request)
	require.True(t, dialog.IsAwaitingResult())
	pressReagentShopKey(dialog, keyboard, ebiten.KeyEscape)
	assert.False(t, dialog.IsDismissed(), "parent must be inert while authority owns the request")

	refreshed := refreshedReagentShopState(state, request, 2)
	refreshed.Offers = []game_state.ReagentOfferQuote{
		refreshed.Offers[4], refreshed.Offers[3], refreshed.Offers[0],
	}
	result := game_state.ReagentPurchaseResult{
		Reagent:          request.Reagent,
		ReagentName:      "Night Shade",
		BundleCount:      request.BundleCount,
		QuantityAfter:    refreshed.Offers[1].QuantityBefore,
		GoldAfterTheft:   refreshed.Gold,
		GoldAfterPayment: refreshed.Gold,
	}
	dialog.ApplyPurchaseResult(result, refreshed, ReagentPurchaseResultDisplay{
		MerchantExchange: "\"A wise purchase.\"",
		Receipt:          "Received 2 Night Shade for 20 gold.",
	})

	assert.Equal(t, ReagentShopBrowse, dialog.GetPhase())
	assert.Equal(t, references.NightShade, dialog.GetSelectedReagent())
	assert.Equal(t, 1, dialog.GetSelectedIndex(), "refresh must preserve reagent identity, not the old index")
	assert.Equal(t, "180", dialog.GetShell().GetHeaderView().PartyGold)
	assert.Contains(t, dialog.GetDetailText(), "Received 2 Night Shade")
	assert.Equal(t, "\"A wise purchase.\"", dialog.GetLatestExchange())
	assert.Equal(t, "Received 2 Night Shade for 20 gold.", dialog.GetReceiptText())
	assert.Len(t, dialog.catalogRows, 3)
	requireReagentStatusConsumesRemainingSpace(t, dialog)

	pressReagentShopKey(dialog, keyboard, ebiten.KeyDown)
	assert.Empty(t, dialog.GetReceiptText())
	assert.Contains(t, dialog.GetDetailText(), "Per bundle")
}

func TestReagentShopDialogRefreshExpandsCatalogGeometry(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	keyboard := newWidgetTestKeyboard(0)
	fullState := testReagentShopState()
	state := cloneReagentShopState(fullState)
	state.Offers = state.Offers[:3]
	dialog := newTestReagentShopDialog(keyboard, state)
	pressReagentShopKey(dialog, keyboard, ebiten.KeyEnter)
	intent, ok := dialog.ConsumePendingPurchaseIntent()
	require.True(t, ok)
	request := game_state.ReagentPurchaseRequest{Reagent: intent.Quote.Reagent, BundleCount: 1}
	dialog.AcceptQuantitySelection(request)
	refreshed := refreshedReagentShopState(fullState, request, 10)
	selected := reagentOfferIndex(refreshed.Offers, request.Reagent)
	dialog.ApplyPurchaseResult(game_state.ReagentPurchaseResult{
		Reagent:          request.Reagent,
		ReagentName:      refreshed.Offers[selected].ReagentName,
		BundleCount:      request.BundleCount,
		QuantityAfter:    refreshed.Offers[selected].QuantityBefore,
		GoldAfterTheft:   refreshed.Gold,
		GoldAfterPayment: refreshed.Gold,
	}, refreshed, ReagentPurchaseResultDisplay{
		MerchantExchange: "\"A wise purchase.\"",
		Receipt:          "Received 10 Ginseng for 20 gold.",
	})

	require.Len(t, dialog.catalogRows, 5)
	require.Equal(t, references.Ginseng, dialog.GetSelectedReagent())
	requireReagentStatusConsumesRemainingSpace(t, dialog)
}

func TestReagentShopDialogPreviewAndSessionLifecycle(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	config := testReagentShopDialogConfig(keyboard, testReagentShopState())
	config.Preview = true
	dialog := NewReagentShopDialog(config)
	require.True(t, dialog.IsPreview())
	require.Contains(t, dialog.GetDetailText(), "PREVIEW")

	pressReagentShopKey(dialog, keyboard, ebiten.KeyEnter)
	intent, ok := dialog.ConsumePendingPurchaseIntent()
	require.True(t, ok)
	request := game_state.ReagentPurchaseRequest{Reagent: intent.Quote.Reagent, BundleCount: 1}
	dialog.AcceptQuantitySelection(request)
	before := cloneReagentShopState(dialog.state)
	dialog.ApplyPreviewStopped(request)
	assert.Equal(t, before, dialog.state)
	assert.Equal(t, ReagentShopBrowse, dialog.GetPhase())
	assert.Contains(t, dialog.GetReceiptText(), "no purchase was committed")
	assert.Contains(t, dialog.GetDetailText(), "PREVIEW")

	dialog.ApplySessionError("The reagent ledger failed.")
	assert.Equal(t, ReagentShopSessionError, dialog.GetPhase())
	assert.Equal(t, "The reagent ledger failed.", dialog.GetShell().GetExchangeView().Text)
	pressReagentShopKey(dialog, keyboard, ebiten.KeyEscape)
	assert.True(t, dialog.IsDismissed())
}

func TestReagentShopDialogUsesRudeThenPoliteDeterministicFarewell(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestReagentShopDialog(keyboard, testReagentShopState())

	pressReagentShopKey(dialog, keyboard, ebiten.KeyEscape)
	assert.Equal(t, ReagentShopFarewell, dialog.GetPhase())
	assert.Equal(t, "\"Return when thy purse is ready.\"", dialog.GetLatestExchange())
	assert.False(t, dialog.IsDismissed())
	pressReagentShopKey(dialog, keyboard, ebiten.KeyEnter)
	assert.True(t, dialog.IsDismissed())

	keyboard = newWidgetTestKeyboard(0)
	dialog = newTestReagentShopDialog(keyboard, testReagentShopState())
	dialog.purchased = true
	pressReagentShopKey(dialog, keyboard, ebiten.KeyEscape)
	assert.Equal(t, "\"May thy mixtures serve thee well.\"", dialog.GetLatestExchange())
}

func TestReagentShopDialogStrictDetachedContractsAndDynamicRows(t *testing.T) {
	valid := testReagentShopDialogConfig(newWidgetTestKeyboard(0), testReagentShopState())
	tests := []struct {
		name   string
		mutate func(*ReagentShopDialogConfig)
	}{
		{name: "keyboard", mutate: func(config *ReagentShopDialogConfig) { config.Keyboard = nil }},
		{name: "sprite", mutate: func(config *ReagentShopDialogConfig) { config.MerchantSprite = nil }},
		{name: "greeting", mutate: func(config *ReagentShopDialogConfig) { config.Prose.Greeting = " " }},
		{name: "family", mutate: func(config *ReagentShopDialogConfig) { config.State.Merchant.Family = references.MerchantFamilyGuild }},
		{name: "too few offers", mutate: func(config *ReagentShopDialogConfig) { config.State.Offers = config.State.Offers[:2] }},
		{name: "duplicate reagent", mutate: func(config *ReagentShopDialogConfig) {
			config.State.Offers[1] = cloneReagentOfferQuote(config.State.Offers[0])
		}},
		{name: "approval", mutate: func(config *ReagentShopDialogConfig) {
			config.State.Offers[0].ApprovalViews[0].TotalPrice++
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := valid
			config.State = cloneReagentShopState(valid.State)
			test.mutate(&config)
			require.Panics(t, func() { NewReagentShopDialog(config) })
		})
	}

	three := testReagentShopState()
	three.Offers = three.Offers[:3]
	dialog := newTestReagentShopDialog(newWidgetTestKeyboard(0), three)
	assert.Len(t, dialog.GetCatalogQuotes(), 3)
	assert.Len(t, dialog.catalogRows, 3)
}

func testReagentShopState() game_state.ReagentShopState {
	const gold uint16 = 200

	return game_state.ReagentShopState{
		Merchant: references.MerchantDefinition{
			ID:        references.MerchantReagentMoonglow,
			Family:    references.MerchantFamilyReagent,
			OwnerName: "Nilrem",
			ShopName:  "The Herbalist",
		},
		PricingCharacterIndex: 0,
		PricingCharacterName:  "Avatar",
		PricingIntelligence:   18,
		Gold:                  gold,
		Offers: []game_state.ReagentOfferQuote{
			testReagentQuote(references.Ginseng, "Ginseng", 10, 20, 10, gold),
			testReagentQuote(references.Garlic, "Garlic", 6, 18, 99, gold),
			testReagentQuote(references.SpiderSilk, "Spider Silk", 4, 500, 5, gold),
			testReagentQuote(references.NightShade, "Night Shade", 1, 10, 5, gold),
			testReagentQuote(references.MandrakeRoot, "Mandrake Root", 1, 15, 7, gold),
		},
	}
}

func testReagentQuote(
	reagent references.Reagent,
	name string,
	bundle uint16,
	price int,
	owned uint16,
	gold uint16,
) game_state.ReagentOfferQuote {
	useful := 0
	if owned < party_state.ItemQuantitySmallMax {
		useful = (int(party_state.ItemQuantitySmallMax-owned) + int(bundle) - 1) / int(bundle)
	}
	affordable := int(gold) / price
	maximum := min(useful, affordable)
	reason := game_state.ReagentOfferBlockNone
	switch {
	case owned == party_state.ItemQuantitySmallMax:
		reason = game_state.ReagentOfferBlockInventoryFull
	case affordable == 0:
		reason = game_state.ReagentOfferBlockUnaffordable
	}
	quote := game_state.ReagentOfferQuote{
		Reagent:           reagent,
		ReagentName:       name,
		BundleQuantity:    bundle,
		BasePrice:         price,
		PricePerBundle:    price,
		Gold:              gold,
		QuantityBefore:    owned,
		UsefulBundles:     uint16(useful),
		AffordableBundles: uint16(affordable),
		MaxBundles:        uint16(maximum),
		Full:              owned == party_state.ItemQuantitySmallMax,
		BlockReason:       reason,
		ApprovalViews:     make([]game_state.ReagentPurchaseApprovalView, maximum),
	}
	for index := range quote.ApprovalViews {
		count := uint16(index + 1)
		requested := bundle * count
		after := min(party_state.ItemQuantitySmallMax, owned+requested)
		receipt := after - owned
		total := price * int(count)
		quote.ApprovalViews[index] = game_state.ReagentPurchaseApprovalView{
			Reagent:           reagent,
			ReagentName:       name,
			BundleQuantity:    bundle,
			BundleCount:       count,
			MaxBundles:        quote.MaxBundles,
			RequestedQuantity: requested,
			QuantityBefore:    owned,
			QuantityAfter:     after,
			ActualReceipt:     receipt,
			PricePerBundle:    price,
			TotalPrice:        total,
			GoldBefore:        gold,
			GoldAfterPayment:  gold - uint16(total),
			PaidOverflow:      receipt < requested,
			OverflowQuantity:  requested - receipt,
		}
	}

	return quote
}

func refreshedReagentShopState(
	state game_state.ReagentShopState,
	request game_state.ReagentPurchaseRequest,
	actualReceipt uint16,
) game_state.ReagentShopState {
	selected := reagentOfferIndex(state.Offers, request.Reagent)
	approval := state.Offers[selected].ApprovalViews[request.BundleCount-1]
	gold := approval.GoldAfterPayment
	owned := state.Offers[selected].QuantityBefore + actualReceipt
	refreshed := cloneReagentShopState(state)
	refreshed.Gold = gold
	for index, quote := range refreshed.Offers {
		quantity := quote.QuantityBefore
		if quote.Reagent == request.Reagent {
			quantity = owned
		}
		refreshed.Offers[index] = testReagentQuote(
			quote.Reagent,
			quote.ReagentName,
			quote.BundleQuantity,
			quote.PricePerBundle,
			quantity,
			gold,
		)
	}

	return refreshed
}

func testReagentShopDialogConfig(
	keyboard *input.Keyboard,
	state game_state.ReagentShopState,
) ReagentShopDialogConfig {
	return ReagentShopDialogConfig{
		Keyboard:       keyboard,
		MerchantSprite: ebiten.NewImage(16, 16),
		State:          state,
		Prose: ReagentShopDialogProse{
			Greeting:       "\"Choose thy reagents.\"",
			RudeFarewell:   "\"Return when thy purse is ready.\"",
			PoliteFarewell: "\"May thy mixtures serve thee well.\"",
		},
		Preview: false,
	}
}

func newTestReagentShopDialog(
	keyboard *input.Keyboard,
	state game_state.ReagentShopState,
) *ReagentShopDialog {
	return NewReagentShopDialog(testReagentShopDialogConfig(keyboard, state))
}

func pressReagentShopKey(dialog *ReagentShopDialog, keyboard *input.Keyboard, key ebiten.Key) {
	keyboard.SimulateKeyPress(key)
	dialog.Update()
}

func compactReagentText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func requireReagentStatusConsumesRemainingSpace(t *testing.T, dialog *ReagentShopDialog) {
	t.Helper()
	workspace := dialog.GetShell().GetWorkspaceBounds()
	inset := max(reagentShopMinimumInset, min(workspace.Dx(), workspace.Dy())/reagentShopInsetDivisor)
	content := insetRectangle(workspace, inset)
	gap := max(inset, dialog.pixelUnit()*reagentShopDoubleInset)
	catalog := dialog.GetCatalogBounds()
	detail := dialog.GetDetailBounds()

	require.True(t, catalog.In(content))
	require.True(t, detail.In(content))
	require.Equal(t, catalog.Max.Y+gap, detail.Min.Y)
	require.Equal(t, content.Max.Y, detail.Max.Y)
	require.GreaterOrEqual(t, dialog.detailTextBounds.Dy(),
		int(math.Ceil(dialog.detailOutput.GetTotalHeightForLines(reagentShopDetailLines))))
}
