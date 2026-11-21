package widgets

import (
	"image"
	"image/color"
	"math"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	pkgcolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func TestGuildShopDialogConstructorContractsAndCanonicalOrder(t *testing.T) {
	valid := testGuildShopDialogConfig(newWidgetTestKeyboard(0), testGuildShopState())

	tests := []struct {
		name   string
		mutate func(*GuildShopDialogConfig)
	}{
		{name: "keyboard", mutate: func(config *GuildShopDialogConfig) { config.Keyboard = nil }},
		{name: "merchant sprite", mutate: func(config *GuildShopDialogConfig) { config.MerchantSprite = nil }},
		{name: "detached state", mutate: func(config *GuildShopDialogConfig) { config.State = game_state.GuildShopState{} }},
		{name: "prose", mutate: func(config *GuildShopDialogConfig) { config.Prose = GuildShopDialogProse{} }},
		{name: "complete catalog", mutate: func(config *GuildShopDialogConfig) {
			config.State.Offers[0].Provision = references.Gem
		}},
		{name: "trusted maximum", mutate: func(config *GuildShopDialogConfig) {
			config.State.Offers[0].MaxBundles++
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := valid
			test.mutate(&config)
			require.Panics(t, func() { NewGuildShopDialog(config) })
		})
	}

	state := testGuildShopState()
	state.Offers = [3]game_state.GuildOfferQuote{state.Offers[2], state.Offers[0], state.Offers[1]}
	dialog := newTestGuildShopDialog(newWidgetTestKeyboard(0), state)
	offers := dialog.GetCatalogQuotes()
	require.Equal(t, references.Key, offers[0].Provision)
	require.Equal(t, references.Gem, offers[1].Provision)
	require.Equal(t, references.Torches, offers[2].Provision)
}

func TestGuildShopDialogSelectionKeepsOnlyGreetingAndHeldRepeat(t *testing.T) {
	held := map[ebiten.Key]bool{}
	config := testGuildShopDialogConfig(newWidgetTestKeyboard(0), testGuildShopState())
	config.IsKeyPressed = func(key ebiten.Key) bool { return held[key] }
	dialog := NewGuildShopDialog(config)
	greeting := dialog.GetLatestExchange()

	held[ebiten.KeyDown] = true
	dialog.Update()
	require.Equal(t, 1, dialog.GetSelectedIndex())
	require.Equal(t, greeting, dialog.GetLatestExchange())
	require.NotContains(t, dialog.GetShell().exchangeOutput.GetOutputStr(false), "Gems pitch")

	for range 17 {
		dialog.Update()
	}
	require.Equal(t, 1, dialog.GetSelectedIndex())
	dialog.Update()
	require.Equal(t, 2, dialog.GetSelectedIndex())
	require.Equal(t, greeting, dialog.GetLatestExchange())
}

func TestGuildShopDialogIgnoresTransitionHeldDirectionUntilFreshPress(t *testing.T) {
	held := map[ebiten.Key]bool{ebiten.KeyDown: true}
	config := testGuildShopDialogConfig(newWidgetTestKeyboard(0), testGuildShopState())
	config.IsKeyPressed = func(key ebiten.Key) bool { return held[key] }
	dialog := NewGuildShopDialog(config)

	for range 60 {
		dialog.Update()
	}
	require.Equal(t, 0, dialog.GetSelectedIndex())

	held[ebiten.KeyDown] = false
	dialog.Update()
	held[ebiten.KeyDown] = true
	dialog.Update()
	require.Equal(t, 1, dialog.GetSelectedIndex())
}

func TestGuildShopDialogIntentQuantityCancelAndResultLifecycle(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	state := testGuildShopState()
	config := testGuildShopDialogConfig(keyboard, state)
	config.Preview = true
	dialog := NewGuildShopDialog(config)
	require.True(t, dialog.IsPreview())
	require.Contains(t, dialog.GetStatusText(), "PREVIEW")
	greeting := dialog.GetLatestExchange()
	pressGuildShopKey(dialog, keyboard, ebiten.KeyDown)

	pressGuildShopKey(dialog, keyboard, ebiten.KeyEnter)
	require.Equal(t, GuildShopAwaitingQuantity, dialog.GetPhase())
	require.True(t, dialog.HasPendingPurchaseIntent())
	intent, ok := dialog.ConsumePendingPurchaseIntent()
	require.True(t, ok)
	require.Equal(t, state.Offers[1], intent.Quote)
	require.Equal(t, "Gems pitch.", intent.Pitch)
	require.Equal(t, greeting, dialog.GetLatestExchange(), "pitch first appears in the quantity modal")

	pressGuildShopKey(dialog, keyboard, ebiten.KeyEnter)
	_, duplicate := dialog.ConsumePendingPurchaseIntent()
	require.False(t, duplicate, "awaiting quantity must not emit duplicate intents")
	dialog.CancelQuantitySelection()
	require.Equal(t, GuildShopBrowse, dialog.GetPhase())
	require.Equal(t, greeting, dialog.GetLatestExchange())
	require.Empty(t, dialog.GetReceiptText())

	pressGuildShopKey(dialog, keyboard, ebiten.KeyEnter)
	intent, ok = dialog.ConsumePendingPurchaseIntent()
	require.True(t, ok)
	request := game_state.GuildPurchaseRequest{Provision: intent.Quote.Provision, BundleCount: 2}
	dialog.AcceptQuantitySelection(request)
	require.True(t, dialog.IsAwaitingResult())
	pressGuildShopKey(dialog, keyboard, ebiten.KeyEnter)
	require.False(t, dialog.HasPendingPurchaseIntent())

	refreshed := refreshedGuildState(state, references.Gem, 28, 100)
	result := successfulGuildResult(intent.Quote, request.BundleCount, refreshed)
	dialog.ApplyPurchaseResult(result, refreshed, GuildPurchaseResultDisplay{
		MerchantExchange: "Sold by Braunam.",
		Receipt:          "Received 8 gems for 400 gold.",
	})

	require.Equal(t, GuildShopBrowse, dialog.GetPhase())
	require.Equal(t, 1, dialog.GetSelectedIndex())
	require.Equal(t, "Sold by Braunam.", dialog.GetLatestExchange())
	require.Equal(t, "Sold by Braunam.", dialog.GetShell().GetExchangeView().Text)
	require.Equal(t, "Received 8 gems for 400 gold.", dialog.GetReceiptText())
	require.Contains(t, strings.Join(strings.Fields(dialog.GetStatusText()), " "), "Received 8 gems for 400 gold.")
	require.Equal(t, uint16(28), dialog.GetSelectedQuote().QuantityBefore)
	require.Equal(t, "100", dialog.GetShell().GetHeaderView().PartyGold)
	pressGuildShopKey(dialog, keyboard, ebiten.KeyDown)
	require.Empty(t, dialog.GetReceiptText())
	require.NotContains(t, dialog.GetStatusText(), "Received 8 gems")
}

func TestGuildShopDialogLatestExchangeReplacementAndSessionErrorDismissal(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestGuildShopDialog(keyboard, testGuildShopState())
	pressGuildShopKey(dialog, keyboard, ebiten.KeyEnter)
	dialog.ApplySessionError("The ledger failed.")

	require.Equal(t, GuildShopError, dialog.GetPhase())
	require.Equal(t, "The ledger failed.", dialog.GetLatestExchange())
	require.Equal(t, "The ledger failed.", dialog.GetShell().GetExchangeView().Text)
	require.False(t, dialog.HasPendingPurchaseIntent())
	require.Empty(t, dialog.GetReceiptText())
	pressGuildShopKey(dialog, keyboard, ebiten.KeyEnter)
	require.True(t, dialog.IsDismissed())
}

func TestGuildShopDialogApplyPreviewStoppedReturnsToBrowseWithoutResult(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	state := testGuildShopState()
	dialog := newTestGuildShopDialog(keyboard, state)
	pressGuildShopKey(dialog, keyboard, ebiten.KeyEnter)
	intent, ok := dialog.ConsumePendingPurchaseIntent()
	require.True(t, ok)
	request := game_state.GuildPurchaseRequest{Provision: intent.Quote.Provision, BundleCount: 2}
	dialog.AcceptQuantitySelection(request)

	dialog.ApplyPreviewStopped(request)

	require.Equal(t, GuildShopBrowse, dialog.GetPhase())
	require.Equal(t, state, dialog.state)
	require.Equal(t, "Preview stopped before purchase authority.", dialog.GetLatestExchange())
	require.Equal(t,
		"Preview only: no purchase was committed; gold and inventory are unchanged.",
		dialog.GetReceiptText(),
	)
	require.Contains(t, dialog.GetShell().GetExchangeView().Text, "Preview stopped")
	require.Panics(t, func() { dialog.ApplyPreviewStopped(request) })
}

func TestGuildShopDialogCatalogUsesActualRowsAndStatusConsumesRemainder(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	dialog := newTestGuildShopDialog(newWidgetTestKeyboard(0), testGuildShopState())
	workspace := dialog.GetShell().GetWorkspaceBounds()
	catalog := dialog.GetCatalogBounds()
	status := dialog.GetStatusBounds()
	rowHeight := dialog.GetCatalogRowBounds(0).Dy()
	inset := max(guildShopMinimumInset, min(workspace.Dx(), workspace.Dy())/guildShopInsetDivisor)
	content := insetRectangle(workspace, inset)
	gap := max(inset, dialog.pixelUnit()*guildShopDoubleInset)
	minimumStatusHeight := int(math.Ceil(dialog.statusOutput.GetTotalHeightForLines(guildShopStatusMinimumLines))) +
		inset*guildShopDoubleInset

	require.True(t, catalog.In(content))
	require.True(t, status.In(content))
	require.Less(t, catalog.Dx(), workspace.Dx())
	require.Equal(t, dialog.catalogHeader.Dy()+rowHeight*guildShopOfferCount, catalog.Dy())
	require.Greater(t, dialog.catalogHeader.Dy(), rowHeight)
	require.Equal(t,
		int(math.Ceil(dialog.catalogOutput.GetFontHeight()))+max(inset, dialog.pixelUnit()*guildShopDoubleInset),
		rowHeight,
	)
	for index := range guildShopOfferCount {
		row := dialog.GetCatalogRowBounds(index)
		require.Equal(t, rowHeight, row.Dy())
		if index > 0 {
			require.Equal(t, dialog.GetCatalogRowBounds(index-1).Max.Y, row.Min.Y)
		}
	}
	require.Equal(t, catalog.Max.Y+gap, status.Min.Y)
	require.Equal(t, content.Max.Y, status.Max.Y)
	require.Greater(t, status.Dy(), minimumStatusHeight, "standard layout should give status all surplus height")
	require.GreaterOrEqual(t, dialog.statusText.Dy(),
		int(math.Ceil(dialog.statusOutput.GetTotalHeightForLines(guildShopStatusMinimumLines))))
	bundleWidth, _ := dialog.catalogFont.MeasureString("Bundle")
	ownedWidth, _ := dialog.catalogFont.MeasureString("Owned")
	bundleRight := float64(dialog.catalogColumns[1].Min.X+dialog.catalogColumns[1].Dx()/2) + bundleWidth/2
	ownedLeft := float64(dialog.catalogColumns[2].Min.X+dialog.catalogColumns[2].Dx()/2) - ownedWidth/2
	require.Greater(t, ownedLeft-bundleRight, float64(dialog.pixelUnit()*2))
	footer := strings.ToLower(dialog.GetShell().GetFooterView().Guidance)
	require.NotContains(t, footer, "arrow")
	require.NotContains(t, footer, "mouse")
	require.Equal(t, "[up/down] select   [enter] buy   [esc] leave", footer)
}

func TestGuildShopDialogGuidanceUsesBracketedActionableKeys(t *testing.T) {
	dialog := newTestGuildShopDialog(newWidgetTestKeyboard(0), testGuildShopState())

	require.Equal(t, "[Up/Down] Select   [Enter] Buy   [Esc] Leave", dialog.footerGuidance())
	dialog.phase = GuildShopAwaitingQuantity
	require.Equal(t, "Choose purchase quantity", dialog.footerGuidance())
	dialog.phase = GuildShopAwaitingResult
	require.Empty(t, dialog.footerGuidance())
	dialog.phase = GuildShopError
	require.Equal(t, "[Enter/Esc] Leave", dialog.footerGuidance())
}

func TestGuildShopDialogCatalogUsesTextOnlyRows(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	dialog := newTestGuildShopDialog(newWidgetTestKeyboard(0), testGuildShopState())
	require.Len(t, dialog.catalogColumns, guildShopCatalogColumns)
	require.Equal(t, dialog.catalogBounds.Min.X, dialog.catalogColumns[0].Min.X)
	require.Equal(t, dialog.catalogBounds.Max.X, dialog.catalogColumns[guildShopCatalogColumns-1].Max.X)

	screen := ebiten.NewImage(1280, 720)
	dialog.Draw(screen)
	selectedRow := dialog.GetCatalogRowBounds(0)
	requireImageRegionContainsColor(t, screen, selectedRow, pkgcolor.EGAWhite)
}

func TestGuildShopDialogStatusPanelAlwaysExistsAndSwitchesFromNoteToReceipt(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestGuildShopDialog(keyboard, testGuildShopState())
	status := dialog.GetStatusBounds()
	statusFramePoint := image.Pt(status.Min.X+status.Dx()/2, status.Min.Y)

	entryScreen := ebiten.NewImage(1280, 720)
	dialog.Draw(entryScreen)
	requireColorRGBA(t, DialogRuleLightColor, entryScreen.At(statusFramePoint.X, statusFramePoint.Y))

	pressGuildShopKey(dialog, keyboard, ebiten.KeyEnter)
	dialog.ConsumePendingPurchaseIntent()
	dialog.CancelQuantitySelection()
	cancelledScreen := ebiten.NewImage(1280, 720)
	dialog.Draw(cancelledScreen)
	requireColorRGBA(t, DialogRuleLightColor, cancelledScreen.At(statusFramePoint.X, statusFramePoint.Y))

	pressGuildShopKey(dialog, keyboard, ebiten.KeyEnter)
	intent, ok := dialog.ConsumePendingPurchaseIntent()
	require.True(t, ok)
	request := game_state.GuildPurchaseRequest{Provision: intent.Quote.Provision, BundleCount: 1}
	dialog.AcceptQuantitySelection(request)
	dialog.ApplyPreviewStopped(request)
	receiptScreen := ebiten.NewImage(1280, 720)
	dialog.Draw(receiptScreen)
	requireColorRGBA(t, DialogRuleLightColor, receiptScreen.At(statusFramePoint.X, statusFramePoint.Y))
	require.Contains(t, strings.Join(strings.Fields(dialog.GetStatusText()), " "), "no purchase was committed")
	requireOutputVerticallyCentered(t, dialog.statusText, dialog.statusOutput)
	requireOutputVerticallyCentered(t, dialog.GetShell().footerText, dialog.GetShell().footerOutput)
}

func TestGuildShopDialogResolutionRebuildPreservesSelectionAndPhase(t *testing.T) {
	setMerchantShellTestResolution(t, 320, 180)
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestGuildShopDialog(keyboard, testGuildShopState())
	pressGuildShopKey(dialog, keyboard, ebiten.KeyDown)
	pressGuildShopKey(dialog, keyboard, ebiten.KeyEnter)
	lowCatalog := dialog.GetCatalogBounds()
	lowStatus := dialog.GetStatusBounds()
	requireGuildShopLayoutValid(t, dialog)

	setResponsiveDialogTestResolution(t, 1280, 720)
	dialog.ensureLayout()

	require.Equal(t, 1, dialog.GetSelectedIndex())
	require.Equal(t, GuildShopAwaitingQuantity, dialog.GetPhase())
	require.Equal(t, references.Gem, dialog.GetSelectedQuote().Provision)
	require.Greater(t, dialog.GetCatalogBounds().Dx(), lowCatalog.Dx())
	require.Greater(t, dialog.GetCatalogBounds().Dy(), lowCatalog.Dy())
	require.Greater(t, dialog.GetStatusBounds().Dy(), lowStatus.Dy())
	requireGuildShopLayoutValid(t, dialog)
}

func requireGuildShopLayoutValid(t *testing.T, dialog *GuildShopDialog) {
	t.Helper()
	workspace := dialog.GetShell().GetWorkspaceBounds()
	inset := max(guildShopMinimumInset, min(workspace.Dx(), workspace.Dy())/guildShopInsetDivisor)
	content := insetRectangle(workspace, inset)
	gap := max(inset, dialog.pixelUnit()*guildShopDoubleInset)
	catalog := dialog.GetCatalogBounds()
	status := dialog.GetStatusBounds()

	require.True(t, catalog.In(content))
	require.True(t, status.In(content))
	require.Equal(t, catalog.Max.Y+gap, status.Min.Y)
	require.Equal(t, content.Max.Y, status.Max.Y)
	require.Greater(t, status.Dx(), 0)
	require.Greater(t, status.Dy(), 0)
	require.GreaterOrEqual(t, dialog.statusText.Dy(),
		int(math.Ceil(dialog.statusOutput.GetTotalHeightForLines(guildShopStatusMinimumLines))))
}

func testGuildShopState() game_state.GuildShopState {
	const gold uint16 = 500

	return game_state.GuildShopState{
		Merchant: references.MerchantDefinition{
			ID:        references.MerchantGuildNewMagincia,
			Family:    references.MerchantFamilyGuild,
			OwnerName: "Braunam",
			ShopName:  "The Den",
		},
		PricingCharacterName: "Avatar",
		Gold:                 gold,
		Offers: [3]game_state.GuildOfferQuote{
			testGuildQuote(references.Key, 3, 190, 10, gold),
			testGuildQuote(references.Gem, 4, 200, 20, gold),
			testGuildQuote(references.Torches, 5, 12, 30, gold),
		},
	}
}

func testGuildQuote(
	provision references.Provision,
	bundle uint16,
	price int,
	quantity uint16,
	gold uint16,
) game_state.GuildOfferQuote {
	remaining := int(party_state.ItemQuantitySmallMax - quantity)
	usefulBundles := (remaining + int(bundle) - 1) / int(bundle)
	affordableBundles := int(gold) / price

	return game_state.GuildOfferQuote{
		Provision:      provision,
		BundleQuantity: bundle,
		BasePrice:      price,
		PricePerBundle: price,
		Gold:           gold,
		QuantityBefore: quantity,
		MaxBundles:     uint16(min(usefulBundles, affordableBundles)),
	}
}

func testGuildShopDialogConfig(
	keyboard *input.Keyboard,
	state game_state.GuildShopState,
) GuildShopDialogConfig {
	return GuildShopDialogConfig{
		Keyboard:       keyboard,
		MerchantSprite: ebiten.NewImage(16, 16),
		State:          state,
		Prose: GuildShopDialogProse{
			Greeting:     "Welcome to the guild.",
			OfferPitches: [3]string{"Keys pitch.", "Gems pitch.", "Torches pitch."},
		},
		Preview:      false,
		IsKeyPressed: nil,
	}
}

func newTestGuildShopDialog(
	keyboard *input.Keyboard,
	state game_state.GuildShopState,
) *GuildShopDialog {
	return NewGuildShopDialog(testGuildShopDialogConfig(keyboard, state))
}

func pressGuildShopKey(dialog *GuildShopDialog, keyboard *input.Keyboard, key ebiten.Key) {
	keyboard.SimulateKeyPress(key)
	dialog.Update()
}

func refreshedGuildState(
	state game_state.GuildShopState,
	provision references.Provision,
	quantity uint16,
	gold uint16,
) game_state.GuildShopState {
	state.Gold = gold
	for index := range state.Offers {
		quote := &state.Offers[index]
		quote.Gold = gold
		if quote.Provision == provision {
			quote.QuantityBefore = quantity
		}
		remaining := 0
		if quote.QuantityBefore < party_state.ItemQuantitySmallMax {
			remaining = int(party_state.ItemQuantitySmallMax - quote.QuantityBefore)
		}
		usefulBundles := 0
		if remaining > 0 {
			usefulBundles = (remaining + int(quote.BundleQuantity) - 1) / int(quote.BundleQuantity)
		}
		quote.MaxBundles = uint16(min(usefulBundles, int(gold)/quote.PricePerBundle))
	}

	return state
}

func successfulGuildResult(
	quote game_state.GuildOfferQuote,
	bundleCount uint16,
	refreshed game_state.GuildShopState,
) game_state.GuildPurchaseResult {
	totalCost := quote.PricePerBundle * int(bundleCount)
	quantityAfter := quote.QuantityBefore + quote.BundleQuantity*bundleCount
	quantityAfter = min(quantityAfter, party_state.ItemQuantitySmallMax)

	return game_state.GuildPurchaseResult{
		Provision:       quote.Provision,
		BasePrice:       quote.BasePrice,
		PricePerBundle:  quote.PricePerBundle,
		BundleQuantity:  quote.BundleQuantity,
		BundleCount:     bundleCount,
		OfferedQuantity: quote.BundleQuantity * bundleCount,
		TotalCost:       totalCost,
		GoldBefore:      quote.Gold,
		GoldAfter:       refreshed.Gold,
		QuantityBefore:  quote.QuantityBefore,
		QuantityAfter:   quantityAfter,
		ActualReceipt:   quantityAfter - quote.QuantityBefore,
	}
}

func requireColorRGBA(t *testing.T, expected, actual color.Color) {
	t.Helper()
	expectedR, expectedG, expectedB, expectedA := expected.RGBA()
	actualR, actualG, actualB, actualA := actual.RGBA()
	require.Equal(t, expectedR, actualR)
	require.Equal(t, expectedG, actualG)
	require.Equal(t, expectedB, actualB)
	require.Equal(t, expectedA, actualA)
}

func requireColorNotRGBA(t *testing.T, unexpected, actual color.Color) {
	t.Helper()
	unexpectedR, unexpectedG, unexpectedB, unexpectedA := unexpected.RGBA()
	actualR, actualG, actualB, actualA := actual.RGBA()
	require.NotEqual(t, [4]uint32{unexpectedR, unexpectedG, unexpectedB, unexpectedA},
		[4]uint32{actualR, actualG, actualB, actualA})
}

func requireImageRegionContainsColor(
	t *testing.T,
	image *ebiten.Image,
	region image.Rectangle,
	expected color.Color,
) {
	t.Helper()
	for y := region.Min.Y; y < region.Max.Y; y++ {
		for x := region.Min.X; x < region.Max.X; x++ {
			if colorsEqual(expected, image.At(x, y)) {
				return
			}
		}
	}
	t.Fatalf("region %v does not contain color %v", region, expected)
}

func colorsEqual(first, second color.Color) bool {
	firstR, firstG, firstB, firstA := first.RGBA()
	secondR, secondG, secondB, secondA := second.RGBA()

	return firstR == secondR && firstG == secondG && firstB == secondB && firstA == secondA
}
