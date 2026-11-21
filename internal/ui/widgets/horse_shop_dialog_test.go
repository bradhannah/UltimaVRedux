package widgets

import (
	"image"
	"math"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	pkgcolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func TestHorseShopDialogReadyDeferredClosedAndUnaffordableContent(t *testing.T) {
	ready := newTestHorseShopDialog(newWidgetTestKeyboard(0), testHorseShopState())
	assert.Equal(t, HorseShopBrowse, ready.GetPhase())
	assert.Equal(t, "Horse for Sale Horse 170 gold", compactHorseText(ready.GetShowroomText()))
	assert.Equal(t, "Enter to review this purchase.", compactHorseText(ready.GetStatusText()))
	assert.Equal(t, "[Enter] Review purchase   [Esc] Leave", ready.GetShell().GetFooterView().Guidance)

	deferredState := testHorseShopState()
	deferredState.Quote.Placement = game_state.HorsePlacementReview{
		BlockStatus: game_state.HorseShopBlockAvailabilityDeferred,
	}
	deferredConfig := testHorseShopDialogConfig(newWidgetTestKeyboard(0), deferredState)
	deferredConfig.Preview = true
	deferred := NewHorseShopDialog(deferredConfig)
	deferredStatus := compactHorseText(deferred.GetStatusText())
	assert.True(t, deferred.IsPreview())
	assert.Contains(t, deferredStatus, "Preview")
	assert.Contains(t, deferredStatus, "Availability will be checked in Live.")
	assert.Contains(t, deferredStatus, "Enter to review this purchase.")
	assert.NotContains(t, strings.ToLower(deferredStatus), "deliver")

	for _, blockStatus := range []game_state.HorseShopBlockStatus{
		game_state.HorseShopBlockMapUnitCapacity,
		game_state.HorseShopBlockNoLegalPlacement,
	} {
		state := testHorseShopState()
		state.Quote.Placement = game_state.HorsePlacementReview{BlockStatus: blockStatus}
		keyboard := newWidgetTestKeyboard(0)
		closed := newTestHorseShopDialog(keyboard, state)

		assert.Equal(t, HorseShopStableClosed, closed.GetPhase())
		assert.Equal(t, horseStableClosedText, compactHorseText(closed.GetStatusText()))
		assert.Equal(t, horseStableClosedText, closed.GetLatestExchange())
		assert.Empty(t, closed.GetShowroomText())
		assert.True(t, closed.GetShowroomBounds().Empty())
		assert.Equal(t, "[Enter/Esc] Leave", closed.GetShell().GetFooterView().Guidance)
		_, pending := closed.ConsumePendingPurchaseIntent()
		assert.False(t, pending)
		pressHorseShopKey(closed, keyboard, ebiten.KeyEnter)
		assert.True(t, closed.IsDismissed())
	}

	poorState := testHorseShopState()
	setHorseShopGold(&poorState, 50)
	poor := newTestHorseShopDialog(newWidgetTestKeyboard(0), poorState)
	poorStatus := compactHorseText(poor.GetStatusText())
	assert.Equal(t, "Enter to review this purchase.", poorStatus)
	assert.NotContains(t, poorStatus, "Need")
}

func TestHorseShopDialog320x180DeferredUnaffordablePreviewStatusIsOrderedAndVisible(t *testing.T) {
	setMerchantShellTestResolution(t, 320, 180)
	state := testHorseShopState()
	setHorseShopGold(&state, 50)
	state.Quote.Placement = game_state.HorsePlacementReview{
		BlockStatus: game_state.HorseShopBlockAvailabilityDeferred,
	}
	config := testHorseShopDialogConfig(newWidgetTestKeyboard(0), state)
	config.Preview = true
	dialog := NewHorseShopDialog(config)
	status := compactHorseText(dialog.GetStatusText())

	assert.Equal(
		t,
		"Preview Availability will be checked in Live. Enter to review this purchase.",
		status,
	)
	assert.True(t, strings.HasPrefix(strings.TrimSpace(dialog.GetStatusText()), "Preview"))
	assert.NotContains(t, status, "Need")
	previewIndex := strings.Index(status, "Preview")
	availabilityIndex := strings.Index(status, "Availability will be checked in Live.")
	reviewIndex := strings.Index(status, "Enter to review this purchase.")
	require.Less(t, previewIndex, availabilityIndex)
	require.Less(t, availabilityIndex, reviewIndex)
	requireHorseShopLayoutValid(t, dialog)
}

func TestHorseShopDialogPlayerFacingOutputOmitsAuthorityDetails(t *testing.T) {
	config := testHorseShopDialogConfig(newWidgetTestKeyboard(0), testHorseShopState())
	config.Preview = true
	dialog := NewHorseShopDialog(config)
	visible := strings.ToLower(strings.Join([]string{
		dialog.GetShowroomText(),
		dialog.GetStatusText(),
		dialog.GetLatestExchange(),
		dialog.GetReceiptText(),
		dialog.GetShell().GetHeaderView().ShopIdentity,
		dialog.GetShell().GetFooterView().Guidance,
	}, " "))

	for _, forbidden := range horseDialogForbiddenDetails() {
		assert.NotContains(t, visible, forbidden)
	}
	assert.NotContains(t, visible, "mouse")
	assert.NotContains(t, visible, "click")
}

func TestHorseShopDialogKeyboardApprovalBackAndDuplicateBlocking(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestHorseShopDialog(keyboard, testHorseShopState())
	greeting := dialog.GetLatestExchange()

	pressHorseShopKey(dialog, keyboard, ebiten.KeyEnter)
	assert.Equal(t, HorseShopAwaitingApproval, dialog.GetPhase())
	assert.True(t, dialog.HasPendingPurchaseIntent())
	intent, ok := dialog.ConsumePendingPurchaseIntent()
	require.True(t, ok)
	assert.Equal(t, dialog.GetQuote(), intent.Quote)
	assert.Equal(t, horsePurchaseRequest(testHorseShopState()), intent.Request)
	assert.False(t, intent.Preview)

	pressHorseShopKey(dialog, keyboard, ebiten.KeyEnter)
	_, duplicate := dialog.ConsumePendingPurchaseIntent()
	assert.False(t, duplicate, "parent must remain inert while the child is active")
	dialog.CancelPurchaseApproval()
	assert.Equal(t, HorseShopBrowse, dialog.GetPhase())
	assert.Equal(t, greeting, dialog.GetLatestExchange())

	pressHorseShopKey(dialog, keyboard, ebiten.KeyEnter)
	intent, ok = dialog.ConsumePendingPurchaseIntent()
	require.True(t, ok)
	dialog.AcceptPurchaseApproval(intent.Request)
	assert.Equal(t, HorseShopAwaitingResult, dialog.GetPhase())
	assert.True(t, dialog.IsAwaitingResult())
	pressHorseShopKey(dialog, keyboard, ebiten.KeyEnter)
	_, duplicate = dialog.ConsumePendingPurchaseIntent()
	assert.False(t, duplicate, "parent must remain inert while authority is active")

	require.Panics(t, func() { dialog.CancelPurchaseApproval() })
	require.Panics(t, func() { dialog.AcceptPurchaseApproval(intent.Request) })
}

func TestHorseShopDialogBrowseEscapeUsesRudeFarewellAcknowledgement(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestHorseShopDialog(keyboard, testHorseShopState())

	pressHorseShopKey(dialog, keyboard, ebiten.KeyEscape)
	assert.Equal(t, HorseShopFarewell, dialog.GetPhase())
	assert.True(t, dialog.IsTerminal())
	assert.Equal(t, "\"Then waste no more of my time.\"", dialog.GetLatestExchange())
	assert.False(t, dialog.IsDismissed())
	assert.Equal(t, "[Enter/Esc] Leave", dialog.GetShell().GetFooterView().Guidance)

	pressHorseShopKey(dialog, keyboard, ebiten.KeyNumpadEnter)
	assert.True(t, dialog.IsDismissed())
}

func TestHorseShopDialogTerminalAuthorityResults(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		keyboard := newWidgetTestKeyboard(0)
		dialog, intent := acceptedHorsePurchase(t, keyboard, testHorseShopState())
		dialog.ApplyPurchaseResult(successfulHorsePurchaseResult(intent, 25))

		assert.Equal(t, HorseShopSuccess, dialog.GetPhase())
		assert.True(t, dialog.IsTerminal())
		assert.Equal(t, "\"May thy new steed serve thee well.\"", dialog.GetLatestExchange())
		assert.Contains(t, dialog.GetReceiptText(), "Bought: one horse for 170 gold.")
		assert.Contains(t, dialog.GetReceiptText(), "25 more gold was stolen!")
		assert.Equal(t, "305", dialog.GetShell().GetHeaderView().PartyGold)
		assert.NotContains(t, strings.ToLower(dialog.GetReceiptText()), "terrain")
		assert.Panics(t, func() { dialog.ApplyPurchaseResult(successfulHorsePurchaseResult(intent, 0)) })
	})

	t.Run("funds refusal has no farewell", func(t *testing.T) {
		state := testHorseShopState()
		setHorseShopGold(&state, 50)
		dialog, _ := acceptedHorsePurchase(t, newWidgetTestKeyboard(0), state)
		dialog.ApplyFundsRefusal()

		assert.Equal(t, HorseShopFundsRefusal, dialog.GetPhase())
		assert.Equal(t, "\"Thou couldst not afford to feed it!\"", dialog.GetLatestExchange())
		assert.NotEqual(t, dialog.prose.RudeFarewell, dialog.GetLatestExchange())
		assert.NotEqual(t, dialog.prose.PoliteFarewell, dialog.GetLatestExchange())
		assert.Equal(t, "Need 170 gold; party has 50.", dialog.GetReceiptText())
	})

	t.Run("stale stable closed", func(t *testing.T) {
		dialog, _ := acceptedHorsePurchase(t, newWidgetTestKeyboard(0), testHorseShopState())
		dialog.ApplyStableClosed()

		assert.Equal(t, HorseShopStableClosed, dialog.GetPhase())
		assert.Equal(t, horseStableClosedText, compactHorseText(dialog.GetStatusText()))
		assert.Empty(t, dialog.GetShowroomText())
		assert.True(t, dialog.GetShowroomBounds().Empty())
		assert.NotContains(t, strings.ToLower(dialog.GetStatusText()), "capacity")
		assert.NotContains(t, strings.ToLower(dialog.GetStatusText()), "placement")
	})

	t.Run("preview stopped", func(t *testing.T) {
		config := testHorseShopDialogConfig(newWidgetTestKeyboard(0), testHorseShopState())
		config.Preview = true
		dialog := NewHorseShopDialog(config)
		intent := approveHorsePurchase(t, dialog, config.Keyboard)
		dialog.ApplyPreviewStopped(intent.Request)

		assert.Equal(t, HorseShopPreviewStopped, dialog.GetPhase())
		assert.Contains(t, dialog.GetStatusText(), "Preview")
		assert.Contains(t, compactHorseText(dialog.GetStatusText()), "no purchase was committed")
		assert.Equal(t, uint16(500), dialog.state.Gold)
	})

	t.Run("session error", func(t *testing.T) {
		keyboard := newWidgetTestKeyboard(0)
		dialog := newTestHorseShopDialog(keyboard, testHorseShopState())
		pressHorseShopKey(dialog, keyboard, ebiten.KeyEnter)
		dialog.ApplySessionError("The horse ledger failed.")

		assert.Equal(t, HorseShopSessionError, dialog.GetPhase())
		assert.Equal(t, "The horse ledger failed.", dialog.GetLatestExchange())
		assert.False(t, dialog.HasPendingPurchaseIntent())
		pressHorseShopKey(dialog, keyboard, ebiten.KeyEscape)
		assert.True(t, dialog.IsDismissed())
	})
}

func TestHorseShopDialogNaturalShowroomStatusRemainderAndSemanticBacking(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	dialog := newTestHorseShopDialog(newWidgetTestKeyboard(0), testHorseShopState())
	workspace := dialog.GetShell().GetWorkspaceBounds()
	inset := max(horseShopMinimumInset, min(workspace.Dx(), workspace.Dy())/horseShopInsetDivisor)
	content := insetRectangle(workspace, inset)
	gap := max(inset, dialog.pixelUnit()*horseShopDoubleInset)
	titleHeight := int(math.Ceil(dialog.showroomTitleOutput.GetFontHeight())) + inset*horseShopDoubleInset
	detailHeight := int(math.Ceil(
		dialog.showroomDetailsOutput.GetTotalHeightForLines(horseShopShowroomDetailLines),
	)) + inset*horseShopDoubleInset
	spriteHeight := max(dialog.horseSprite.Bounds().Dy()*horseShopDoubleInset,
		dialog.pixelUnit()*horseShopSpriteHeightUnits)
	showroom := dialog.GetShowroomBounds()
	status := dialog.GetStatusBounds()

	assert.True(t, showroom.In(content))
	assert.True(t, status.In(content))
	assert.Equal(t, titleHeight+spriteHeight+detailHeight, showroom.Dy(), "standard layout uses natural card height")
	assert.Equal(t, showroom.Max.Y+gap, status.Min.Y)
	assert.Equal(t, content.Max.Y, status.Max.Y)
	assert.GreaterOrEqual(t, dialog.statusTextBounds.Dy(),
		int(math.Ceil(dialog.statusOutput.GetTotalHeightForLines(horseShopStatusMinimumLines))))
	assert.Greater(t, dialog.GetHorseSpriteBounds().Dy(), dialog.horseSprite.Bounds().Dy()*4)

	screen := ebiten.NewImage(1280, 720)
	dialog.Draw(screen)
	statusBackground := image.Pt(status.Max.X-inset*2, status.Max.Y-inset*2)
	requireColorRGBA(t, MerchantDialogBackgroundColor, screen.At(statusBackground.X, statusBackground.Y))
	requireImageRegionContainsColor(t, screen, dialog.GetHorseSpriteBounds(), pkgcolor.EGALightMagenta)
}

func TestHorseShopDialogLowResolutionResizePreservesApprovalPhase(t *testing.T) {
	setMerchantShellTestResolution(t, 320, 180)
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestHorseShopDialog(keyboard, testHorseShopState())
	pressHorseShopKey(dialog, keyboard, ebiten.KeyEnter)
	lowShowroom := dialog.GetShowroomBounds()
	lowStatus := dialog.GetStatusBounds()
	requireHorseShopLayoutValid(t, dialog)

	setResponsiveDialogTestResolution(t, 1280, 720)
	dialog.ensureLayout()

	assert.Equal(t, HorseShopAwaitingApproval, dialog.GetPhase())
	assert.Greater(t, dialog.GetShowroomBounds().Dx(), lowShowroom.Dx())
	assert.Greater(t, dialog.GetShowroomBounds().Dy(), lowShowroom.Dy())
	assert.Greater(t, dialog.GetStatusBounds().Dy(), lowStatus.Dy())
	requireHorseShopLayoutValid(t, dialog)
	intent, ok := dialog.ConsumePendingPurchaseIntent()
	require.True(t, ok)
	assert.Equal(t, 170, intent.Request.ReviewedPrice)
}

func TestHorseShopDialogStrictDetachedContracts(t *testing.T) {
	valid := testHorseShopDialogConfig(newWidgetTestKeyboard(0), testHorseShopState())
	tests := []struct {
		name   string
		mutate func(*HorseShopDialogConfig)
	}{
		{name: "keyboard", mutate: func(config *HorseShopDialogConfig) { config.Keyboard = nil }},
		{name: "merchant sprite", mutate: func(config *HorseShopDialogConfig) { config.MerchantSprite = nil }},
		{name: "horse sprite", mutate: func(config *HorseShopDialogConfig) { config.HorseSprite = nil }},
		{name: "family", mutate: func(config *HorseShopDialogConfig) {
			config.State.Merchant.Family = references.MerchantFamilyGuild
		}},
		{name: "gold consequence", mutate: func(config *HorseShopDialogConfig) {
			config.State.Quote.GoldAfterPayment++
		}},
		{name: "deferred live", mutate: func(config *HorseShopDialogConfig) {
			config.State.Quote.Placement.BlockStatus = game_state.HorseShopBlockAvailabilityDeferred
		}},
		{name: "prose", mutate: func(config *HorseShopDialogConfig) { config.Prose = HorseShopDialogProse{} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := valid
			test.mutate(&config)
			require.Panics(t, func() { NewHorseShopDialog(config) })
		})
	}
}

func testHorseShopState() game_state.HorseShopState {
	const gold uint16 = 500
	return game_state.HorseShopState{
		Merchant: references.MerchantDefinition{
			ID:         references.MerchantHorseTrinsic,
			Family:     references.MerchantFamilyHorse,
			Location:   references.Trinsic,
			NPCIndex:   1,
			CatalogRow: 0,
			OwnerName:  "Hettar",
			ShopName:   "Horse & Rider",
		},
		PricingCharacterIndex: 0,
		PricingCharacterName:  "Avatar",
		PricingIntelligence:   10,
		Gold:                  gold,
		Quote: game_state.HorsePurchaseQuote{
			BasePrice:        100,
			Price:            170,
			GoldBefore:       gold,
			GoldAfterPayment: 330,
			Affordable:       true,
			Placement: game_state.HorsePlacementReview{
				Position:    references.Position{X: 10, Y: 11},
				Floor:       1,
				Terrain:     indexes.Grass,
				BlockStatus: game_state.HorseShopBlockNone,
			},
		},
	}
}

func testHorseShopDialogConfig(keyboard *input.Keyboard, state game_state.HorseShopState) HorseShopDialogConfig {
	merchantSprite := ebiten.NewImage(16, 16)
	merchantSprite.Fill(pkgcolor.EGALightCyan)
	horseSprite := ebiten.NewImage(16, 16)
	horseSprite.Fill(pkgcolor.EGALightMagenta)

	return HorseShopDialogConfig{
		Keyboard:       keyboard,
		MerchantSprite: merchantSprite,
		HorseSprite:    horseSprite,
		State:          state,
		Prose: HorseShopDialogProse{
			Greeting:       "\"Welcome, traveler.\"",
			RudeFarewell:   "\"Then waste no more of my time.\"",
			PoliteFarewell: "\"May thy new steed serve thee well.\"",
			FundsRefusal:   "\"Thou couldst not afford to feed it!\"",
		},
		Preview: false,
	}
}

func newTestHorseShopDialog(keyboard *input.Keyboard, state game_state.HorseShopState) *HorseShopDialog {
	return NewHorseShopDialog(testHorseShopDialogConfig(keyboard, state))
}

func setHorseShopGold(state *game_state.HorseShopState, gold uint16) {
	state.Gold = gold
	state.Quote.GoldBefore = gold
	state.Quote.Affordable = int(gold) >= state.Quote.Price
	state.Quote.GoldAfterPayment = gold
	if state.Quote.Affordable {
		state.Quote.GoldAfterPayment = gold - uint16(state.Quote.Price)
	}
}

func pressHorseShopKey(dialog *HorseShopDialog, keyboard *input.Keyboard, key ebiten.Key) {
	keyboard.SimulateKeyPress(key)
	dialog.Update()
}

func approveHorsePurchase(
	t *testing.T,
	dialog *HorseShopDialog,
	keyboard *input.Keyboard,
) HorsePurchaseIntent {
	t.Helper()
	pressHorseShopKey(dialog, keyboard, ebiten.KeyEnter)
	intent, ok := dialog.ConsumePendingPurchaseIntent()
	require.True(t, ok)
	dialog.AcceptPurchaseApproval(intent.Request)

	return intent
}

func acceptedHorsePurchase(
	t *testing.T,
	keyboard *input.Keyboard,
	state game_state.HorseShopState,
) (*HorseShopDialog, HorsePurchaseIntent) {
	t.Helper()
	dialog := newTestHorseShopDialog(keyboard, state)
	intent := approveHorsePurchase(t, dialog, keyboard)

	return dialog, intent
}

func successfulHorsePurchaseResult(intent HorsePurchaseIntent, theft uint16) game_state.HorsePurchaseResult {
	price := uint16(intent.Quote.Price)
	return game_state.HorsePurchaseResult{
		MerchantID:            intent.Request.ReviewedMerchantID,
		PricingCharacterIndex: intent.Request.ReviewedPricingCharacterIndex,
		BasePrice:             intent.Quote.BasePrice,
		Price:                 intent.Quote.Price,
		GoldBefore:            intent.Quote.GoldBefore,
		GoldAfterPayment:      intent.Quote.GoldBefore - price,
		Theft:                 theft,
		GoldAfterTheft:        intent.Quote.GoldBefore - price - theft,
		VehicleType:           references.HorseVehicle,
	}
}

func requireHorseShopLayoutValid(t *testing.T, dialog *HorseShopDialog) {
	t.Helper()
	workspace := dialog.GetShell().GetWorkspaceBounds()
	inset := max(horseShopMinimumInset, min(workspace.Dx(), workspace.Dy())/horseShopInsetDivisor)
	content := insetRectangle(workspace, inset)
	gap := max(inset, dialog.pixelUnit()*horseShopDoubleInset)
	showroom := dialog.GetShowroomBounds()
	status := dialog.GetStatusBounds()

	require.True(t, showroom.In(content))
	require.True(t, status.In(content))
	require.Equal(t, showroom.Max.Y+gap, status.Min.Y)
	require.Equal(t, content.Max.Y, status.Max.Y)
	require.Positive(t, showroom.Dx())
	require.Positive(t, showroom.Dy())
	require.Positive(t, status.Dx())
	require.Positive(t, status.Dy())
	require.GreaterOrEqual(t, dialog.statusTextBounds.Dy(),
		int(math.Ceil(dialog.statusOutput.GetTotalHeightForLines(horseShopStatusMinimumLines))))
}

func horseDialogForbiddenDetails() []string {
	return []string{"delivery", "terrain", "facing", "ownership", "position", "coordinate", "floor", "placement"}
}

func compactHorseText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
