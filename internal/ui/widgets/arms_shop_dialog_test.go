package widgets

import (
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

func TestArmsShopDialogModesSortAndIndependentSelection(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestArmsShopDialog(keyboard, testArmsShopState())

	require.Equal(t, ArmsShopBuy, dialog.GetMode())
	require.Equal(t, ArmsSortName, dialog.GetSort())
	require.Len(t, dialog.GetRows(), 7)
	require.Equal(t, references.Arrows, dialog.GetRows()[0].Equipment)
	require.Equal(t, pkgcolor.EGABlack, dialog.GetRows()[0].IconColor)
	require.Equal(t, pkgcolor.EGAYellow, dialog.GetRows()[1].IconColor)
	require.Equal(t, uint16(2), dialog.GetRows()[1].Readied)

	for range 6 {
		pressArmsShopKey(dialog, keyboard, ebiten.KeyDown)
	}
	buySelection := dialog.GetSelectedEquipment()
	require.Zero(t, dialog.GetScrollOffset(), "all seven dealer-stock rows must fit at the supported resolution")
	pressArmsShopKey(dialog, keyboard, ebiten.KeyR)
	require.Equal(t, ArmsSortQuantity, dialog.GetSort())
	require.Equal(t, buySelection, dialog.GetSelectedEquipment())
	require.Equal(t, []references.Equipment{
		references.Arrows, references.Dagger, references.Bow, references.SmallShield,
		references.LongSword, references.PlateMail, references.LeatherHelm,
	}, armsRowEquipment(dialog.GetRows()))

	pressArmsShopKey(dialog, keyboard, ebiten.KeyS)
	require.Equal(t, ArmsShopSell, dialog.GetMode())
	require.Equal(t, ArmsSortName, dialog.GetSort())
	pressArmsShopKey(dialog, keyboard, ebiten.KeyDown)
	sellSelection := dialog.GetSelectedEquipment()
	pressArmsShopKey(dialog, keyboard, ebiten.KeyR)
	pressArmsShopKey(dialog, keyboard, ebiten.KeyR)
	require.Equal(t, ArmsSortValue, dialog.GetSort())
	require.Equal(t, sellSelection, dialog.GetSelectedEquipment())
	require.Equal(t, []references.Equipment{
		references.Bow, references.Dagger, references.Ankh, references.Arrows,
	}, armsRowEquipment(dialog.GetRows()))

	pressArmsShopKey(dialog, keyboard, ebiten.KeyTab)
	require.Equal(t, ArmsShopBuy, dialog.GetMode())
	require.Equal(t, ArmsSortQuantity, dialog.GetSort())
	require.Equal(t, buySelection, dialog.GetSelectedEquipment())
	pressArmsShopKey(dialog, keyboard, ebiten.KeyB)
	require.Equal(t, ArmsShopBuy, dialog.GetMode())
}

func TestArmsShopDialogSellListScrollsPastSevenRows(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	state := testArmsShopState()
	state.SellQuotes = nil
	for _, equipment := range []references.Equipment{
		references.LeatherHelm,
		references.ChainCoif,
		references.IronHelm,
		references.SmallShield,
		references.LargeShield,
		references.Dagger,
		references.Sling,
		references.Club,
	} {
		state.SellQuotes = append(state.SellQuotes, game_state.ArmsSellQuote{
			Equipment: equipment, BasePrice: equipment.GetBasePrice(), UnitOffer: 1,
			Gold: state.Gold, QuantityBefore: 1, MaxQuantity: 1, Sellable: true,
		})
	}
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestArmsShopDialog(keyboard, state)
	pressArmsShopKey(dialog, keyboard, ebiten.KeyS)
	for range 7 {
		pressArmsShopKey(dialog, keyboard, ebiten.KeyDown)
	}

	require.Len(t, dialog.rowBounds, armsShopMaximumVisibleRows)
	require.Len(t, dialog.GetVisibleRows(), armsShopMaximumVisibleRows)
	require.True(t, dialog.activeMode().list.IsScrollingNeeded())
	require.Positive(t, dialog.GetScrollOffset())
}

func TestArmsShopDialogUnavailableBuyRowsAreSubduedAndCannotOpenTrade(t *testing.T) {
	state := testArmsShopState()
	for index := range state.BuyQuotes {
		if state.BuyQuotes[index].Equipment == references.Arrows {
			state.BuyQuotes[index].QuantityBefore = party_state.ItemQuantitySmallMax
			state.BuyQuotes[index].MaxQuantity = 0
		}
	}
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestArmsShopDialog(keyboard, state)
	row := dialog.GetRows()[0]

	require.Equal(t, references.Arrows, row.Equipment)
	require.False(t, row.Available)
	require.Equal(t, pkgcolor.EGADarkGray, row.IconColor)
	require.Contains(t, row.Reason, "not carry any more")
	detail := strings.Join(strings.Fields(dialog.GetDetailText()), " ")
	require.Contains(t, detail, "Thou canst not carry any more!")
	require.Equal(t, 3, dialog.detailOutput.GetPanelContentLineCount())
	pressArmsShopKey(dialog, keyboard, ebiten.KeyEnter)
	require.Equal(t, ArmsShopBrowse, dialog.GetPhase())
	require.Contains(t, dialog.GetLatestExchange(), "Thou canst not carry any more")
	_, emitted := dialog.ConsumePendingTradeIntent()
	require.False(t, emitted)

	state = refreshedArmsState(testArmsShopState(), 698)
	state.BuyQuotes[6] = game_state.ArmsBuyQuote{
		Equipment: references.RingInvisibility, BasePrice: 450, UnitPrice: 1001,
		Gold: state.Gold, QuantityBefore: 0, ReadiedQuantity: 2, MaxQuantity: 0,
	}
	dialog = newTestArmsShopDialog(newWidgetTestKeyboard(0), state)
	for dialog.GetSelectedEquipment() != references.RingInvisibility {
		pressArmsShopKey(dialog, dialog.keyboard, ebiten.KeyDown)
	}
	detail = dialog.GetDetailText()
	require.Equal(t, 4, dialog.detailOutput.GetPanelContentLineCount())
	require.Contains(t, detail, "Ring of Invisibility")
	require.Contains(t, detail, "ATK 0  DEF 0  RNG 0")
	require.Contains(t, detail, "Cannot buy: need 1001 gold;")
	require.Contains(t, detail, "party has 698.")
	require.Less(t, strings.Index(detail, "Ring of Invisibility"), strings.Index(detail, "ATK 0"))
	require.Less(t, strings.Index(detail, "ATK 0"), strings.Index(detail, "Cannot buy"))
	require.Less(t, strings.Index(detail, "Cannot buy"), strings.Index(detail, "party has 698"))
	pressArmsShopKey(dialog, dialog.keyboard, ebiten.KeyEnter)
	require.Equal(t, ArmsShopTerminalRefusal, dialog.GetPhase())
	require.Contains(t, dialog.GetLatestExchange(), "Can't pay?! Out with thee!")
	require.False(t, dialog.IsDismissed())
	pressArmsShopKey(dialog, dialog.keyboard, ebiten.KeyEnter)
	require.True(t, dialog.IsDismissed())
}

func TestArmsShopDialogSortTieBreaksAndDirectionalNavigation(t *testing.T) {
	buy := []game_state.ArmsBuyQuote{
		{Equipment: references.LongSword, QuantityBefore: 2, UnitPrice: 50},
		{Equipment: references.Dagger, QuantityBefore: 2, UnitPrice: 50},
		{Equipment: references.Bow, QuantityBefore: 3, UnitPrice: 40},
	}
	sortArmsBuyRows(buy, ArmsSortQuantity)
	require.Equal(t, []references.Equipment{
		references.Bow, references.Dagger, references.LongSword,
	}, []references.Equipment{buy[0].Equipment, buy[1].Equipment, buy[2].Equipment})
	sortArmsBuyRows(buy, ArmsSortValue)
	require.Equal(t, []references.Equipment{
		references.Dagger, references.LongSword, references.Bow,
	}, []references.Equipment{buy[0].Equipment, buy[1].Equipment, buy[2].Equipment})

	sell := []game_state.ArmsSellQuote{
		{Equipment: references.LongSword, QuantityBefore: 2, UnitOffer: 50},
		{Equipment: references.Dagger, QuantityBefore: 2, UnitOffer: 50},
		{Equipment: references.Bow, QuantityBefore: 3, UnitOffer: 40},
	}
	sortArmsSellRows(sell, ArmsSortQuantity)
	require.Equal(t, []references.Equipment{
		references.Bow, references.Dagger, references.LongSword,
	}, []references.Equipment{sell[0].Equipment, sell[1].Equipment, sell[2].Equipment})

	keyboard := newWidgetTestKeyboard(0)
	config := testArmsShopDialogConfig(keyboard, testArmsShopState())
	config.IsKeyPressed = func(ebiten.Key) bool { return false }
	dialog := NewArmsShopDialog(config)
	for expected := 1; expected < 7; expected++ {
		pressArmsShopKey(dialog, keyboard, ebiten.KeyDown)
		require.Equal(t, expected, dialog.GetSelectedIndex())
	}
	for expected := 5; expected >= 0; expected-- {
		pressArmsShopKey(dialog, keyboard, ebiten.KeyUp)
		require.Equal(t, expected, dialog.GetSelectedIndex(), "Up must work from every non-first row")
	}
}

func TestArmsShopDialogFullNamesIconsSeparatorsAndHeaderPlacement(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	state := testArmsShopState()
	state.BuyQuotes[6] = game_state.ArmsBuyQuote{
		Equipment: references.RingInvisibility, BasePrice: 450, UnitPrice: 450,
		Gold: state.Gold, QuantityBefore: 0, ReadiedQuantity: 2, MaxQuantity: 1,
	}
	dialog := newTestArmsShopDialog(newWidgetTestKeyboard(0), state)

	var ringRow ArmsShopRowView
	for _, row := range dialog.GetRows() {
		if row.Equipment == references.RingInvisibility {
			ringRow = row
			break
		}
	}
	require.Equal(t, "Ring of Invisibility", ringRow.Name)
	require.Equal(t, uint16(2), ringRow.Readied)

	dialog.prepareCellOutput(" "+ringRow.Name, DialogItemNameColor, ringRow.Equipment)
	rendered := dialog.rowOutput.GetOutputStr(false)
	require.Contains(t, rendered, string([]byte{references.RingInvisibility.GetIconChar()}))
	require.Contains(t, strings.Join(strings.Fields(rendered), " "), "Ring of Invisibility")
	require.NotContains(t, dialog.headerOutput.GetOutputStr(false), "|")
	require.NotContains(t, dialog.GetShell().GetFooterView().Guidance, "|")
	require.Greater(t, dialog.tableHeaderContentBounds().Min.Y, dialog.tableHeader.Min.Y)
	require.Len(t, dialog.columns, 4)
	for index, heading := range []string{"Item", "Carried", "Ready", "Price"} {
		width, _ := dialog.rowFont.MeasureString(heading)
		cell := dialog.cellContentBounds(dialog.tableHeaderContentBounds(), dialog.columns[index], references.NoEquipment)
		require.LessOrEqual(t, width, float64(cell.Dx()), "%s heading must fit its column", heading)
	}
	noOfferWidth, _ := dialog.rowFont.MeasureString("No offer")
	offerCell := dialog.cellContentBounds(dialog.rowBounds[0], dialog.columns[3], references.NoEquipment)
	require.LessOrEqual(t, noOfferWidth, float64(offerCell.Dx()))
	plainCell := dialog.cellContentBounds(dialog.rowBounds[0], dialog.columns[0], references.NoEquipment)
	iconCell := dialog.cellContentBounds(dialog.rowBounds[0], dialog.columns[0], references.RingInvisibility)
	require.Greater(t, iconCell.Min.X, plainCell.Min.X, "equipment icons need an additional right inset")
	require.Greater(t, dialog.tableBounds.Max.Y, dialog.rowBounds[len(dialog.rowBounds)-1].Max.Y)
	require.NotContains(t, dialog.GetDetailText(), "Max")
	require.Equal(t, []shopTab{{"Buy", true}, {"Sell", false}}, dialog.tabs())
	require.Contains(t, dialog.headerOutput.GetOutputStr(false), "Buy")
	require.Contains(t, dialog.headerOutput.GetOutputStr(false), "Sell")
	require.NotContains(t, dialog.headerOutput.GetOutputStr(false), "[")
	require.NotContains(t, dialog.headerOutput.GetOutputStr(false), "SORT")
	require.Contains(t, dialog.sortOutput.GetOutputStr(false), "Sort: Name A-Z")
	require.GreaterOrEqual(t, dialog.sortBounds.Min.Y, dialog.tableBounds.Max.Y)
	require.LessOrEqual(t, dialog.sortBounds.Max.Y, dialog.detailBounds.Min.Y)
	baseDetail := insetRectangle(dialog.detailBounds, armsShopMinimumInset)
	require.Greater(t, dialog.detailTextBounds.Min.X, baseDetail.Min.X)
	pressArmsShopKey(dialog, dialog.keyboard, ebiten.KeyS)
	require.Equal(t, []shopTab{{"Buy", false}, {"Sell", true}}, dialog.tabs())
	pressArmsShopKey(dialog, dialog.keyboard, ebiten.KeyLeft)
	require.Equal(t, ArmsShopBuy, dialog.GetMode())
	pressArmsShopKey(dialog, dialog.keyboard, ebiten.KeyRight)
	require.Equal(t, ArmsShopSell, dialog.GetMode())
	pressArmsShopKey(dialog, dialog.keyboard, ebiten.KeyTab)
	require.Equal(t, ArmsShopBuy, dialog.GetMode())
}

func TestArmsShopDialogIntentLifecycleRefreshAndReceiptClearing(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	state := testArmsShopState()
	originalSell := append([]game_state.ArmsSellQuote(nil), state.SellQuotes...)
	dialog := newTestArmsShopDialog(keyboard, state)
	selected := dialog.GetSelectedEquipment()

	pressArmsShopKey(dialog, keyboard, ebiten.KeyEnter)
	require.Equal(t, ArmsShopAwaitingTrade, dialog.GetPhase())
	intent, ok := dialog.ConsumePendingTradeIntent()
	require.True(t, ok)
	require.Equal(t, selected, intent.Equipment())
	pressArmsShopKey(dialog, keyboard, ebiten.KeyEnter)
	_, duplicate := dialog.ConsumePendingTradeIntent()
	require.False(t, duplicate)
	dialog.CancelPendingTrade()
	require.Equal(t, ArmsShopBrowse, dialog.GetPhase())

	pressArmsShopKey(dialog, keyboard, ebiten.KeyEnter)
	intent, ok = dialog.ConsumePendingTradeIntent()
	require.True(t, ok)
	request := ArmsTradeRequest{
		Mode: ArmsShopBuy,
		Buy:  game_state.ArmsBuyRequest{Equipment: intent.Equipment(), Quantity: 1},
	}
	dialog.AcceptTradeIntent(request)
	refreshed := refreshedArmsState(state, 450)
	for index := range refreshed.BuyQuotes {
		if refreshed.BuyQuotes[index].Equipment == intent.Equipment() {
			refreshed.BuyQuotes[index].QuantityBefore++
		}
	}
	dialog.ApplyBuyResult(game_state.ArmsBuyResult{
		Equipment: intent.Equipment(), Quantity: 1,
	}, refreshed, ArmsTradeResultDisplay{
		MerchantExchange: "A fair purchase.",
		Receipt:          "Bought one item for fifty gold.",
	})

	require.Equal(t, ArmsShopBrowse, dialog.GetPhase())
	require.Equal(t, intent.Equipment(), dialog.GetSelectedEquipment())
	require.Equal(t, "450", dialog.GetShell().GetHeaderView().PartyGold)
	require.Contains(t, dialog.GetDetailText(), "Bought one item")
	pressArmsShopKey(dialog, keyboard, ebiten.KeyDown)
	require.Empty(t, dialog.GetReceiptText())
	require.NotContains(t, dialog.GetDetailText(), "Bought one item")
	require.Equal(t, originalSell, state.SellQuotes, "widget sorting and refresh must not mutate caller state")
}

func TestArmsShopDialogSellRefreshRecalculatesRowCountGeometry(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	keyboard := newWidgetTestKeyboard(0)
	state := testArmsShopState()
	state.SellQuotes = state.SellQuotes[:1]
	dialog := newTestArmsShopDialog(keyboard, state)
	pressArmsShopKey(dialog, keyboard, ebiten.KeyS)
	require.Len(t, dialog.rowBounds, 1)
	detailBefore := dialog.GetDetailBounds()

	pressArmsShopKey(dialog, keyboard, ebiten.KeyEnter)
	intent, ok := dialog.ConsumePendingTradeIntent()
	require.True(t, ok)
	require.Equal(t, references.Dagger, intent.Equipment())
	dialog.AcceptTradeIntent(ArmsTradeRequest{
		Mode: ArmsShopSell,
		Sell: game_state.ArmsSellRequest{Equipment: intent.Equipment(), Quantity: 2},
	})
	refreshed := refreshedArmsState(state, 580)
	refreshed.SellQuotes = nil
	dialog.ApplySellResult(game_state.ArmsSellResult{
		Equipment: intent.Equipment(), Quantity: 2,
	}, refreshed, ArmsTradeResultDisplay{
		MerchantExchange: "A fair sale.",
		Receipt:          "Sold two daggers for eighty gold.",
	})

	require.Equal(t, ArmsShopSell, dialog.GetMode())
	require.Equal(t, ArmsSortName, dialog.GetSort())
	require.Empty(t, dialog.GetRows())
	require.Empty(t, dialog.rowBounds)
	require.Equal(t, references.NoEquipment, dialog.GetSelectedEquipment())
	require.Less(t, dialog.GetDetailBounds().Min.Y, detailBefore.Min.Y)
	require.Contains(t, dialog.GetDetailText(), "Sold two daggers")
	requireArmsStatusConsumesRemainingSpace(t, dialog)
}

func TestArmsShopDialogPreviewRefusalErrorAndDismissal(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	config := testArmsShopDialogConfig(keyboard, testArmsShopState())
	config.Preview = true
	dialog := NewArmsShopDialog(config)
	require.True(t, dialog.IsPreview())
	require.Contains(t, dialog.headerOutput.GetOutputStr(false), "PREVIEW")
	pressArmsShopKey(dialog, keyboard, ebiten.KeyEnter)
	intent, ok := dialog.ConsumePendingTradeIntent()
	require.True(t, ok)
	request := ArmsTradeRequest{Mode: ArmsShopBuy, Buy: game_state.ArmsBuyRequest{
		Equipment: intent.Equipment(), Quantity: 1,
	}}
	dialog.AcceptTradeIntent(request)
	dialog.ApplyPreviewStopped(request)
	require.Contains(t, dialog.GetReceiptText(), "unchanged")

	pressArmsShopKey(dialog, keyboard, ebiten.KeyEnter)
	dialog.ConsumePendingTradeIntent()
	dialog.ApplyRefusal(ArmsTradeResultDisplay{
		MerchantExchange: "I cannot do that.",
		Receipt:          "No trade was made.",
	})
	require.Equal(t, "No trade was made.", dialog.GetReceiptText())

	pressArmsShopKey(dialog, keyboard, ebiten.KeyEnter)
	dialog.ApplySessionError("The Arms ledger failed.")
	require.Equal(t, ArmsShopSessionError, dialog.GetPhase())
	pressArmsShopKey(dialog, keyboard, ebiten.KeyEnter)
	require.True(t, dialog.IsDismissed())
}

func TestArmsShopDialogTerminalRefusalWaitsForAcknowledgement(t *testing.T) {
	for _, key := range []ebiten.Key{ebiten.KeyEnter, ebiten.KeyEscape} {
		t.Run(key.String(), func(t *testing.T) {
			keyboard := newWidgetTestKeyboard(0)
			dialog := newTestArmsShopDialog(keyboard, testArmsShopState())
			pressArmsShopKey(dialog, keyboard, ebiten.KeyEnter)
			intent, ok := dialog.ConsumePendingTradeIntent()
			require.True(t, ok)
			dialog.AcceptTradeIntent(ArmsTradeRequest{
				Mode: ArmsShopBuy,
				Buy: game_state.ArmsBuyRequest{
					Equipment: intent.Equipment(),
					Quantity:  1,
				},
				Sell: game_state.ArmsSellRequest{},
			})
			dialog.ApplyTerminalRefusal(ArmsTradeResultDisplay{
				MerchantExchange: "Out with thee!",
				Receipt:          "No purchase: insufficient gold.",
			})

			require.Equal(t, ArmsShopTerminalRefusal, dialog.GetPhase())
			require.False(t, dialog.IsDismissed())
			require.Equal(t, "No purchase: insufficient gold.", dialog.GetReceiptText())
			require.Contains(t, dialog.GetDetailText(), "insufficient gold")
			require.Contains(t, dialog.GetShell().GetFooterView().Guidance, "[Enter/Esc] Leave")

			pressArmsShopKey(dialog, keyboard, key)
			require.True(t, dialog.IsDismissed())
		})
	}
}

func TestArmsShopDialogRefusalRefreshPreservesBrowseContext(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	state := testArmsShopState()
	dialog := newTestArmsShopDialog(keyboard, state)
	pressArmsShopKey(dialog, keyboard, ebiten.KeyR)
	for dialog.GetSelectedEquipment() != references.Bow {
		pressArmsShopKey(dialog, keyboard, ebiten.KeyDown)
	}
	pressArmsShopKey(dialog, keyboard, ebiten.KeyEnter)
	intent, ok := dialog.ConsumePendingTradeIntent()
	require.True(t, ok)
	dialog.AcceptTradeIntent(ArmsTradeRequest{
		Mode: ArmsShopBuy,
		Buy: game_state.ArmsBuyRequest{
			Equipment: intent.Equipment(),
			Quantity:  1,
		},
		Sell: game_state.ArmsSellRequest{},
	})
	refreshed := refreshedArmsState(state, 999)
	for index := range refreshed.BuyQuotes {
		if refreshed.BuyQuotes[index].Equipment == references.Bow {
			refreshed.BuyQuotes[index].QuantityBefore = 99
			refreshed.BuyQuotes[index].MaxQuantity = 0
		}
	}
	dialog.ApplyRefusalWithRefresh(refreshed, ArmsTradeResultDisplay{
		MerchantExchange: "Thou canst not carry any more!",
		Receipt:          "No Bow purchased: carried equipment capacity is full.",
	})

	require.Equal(t, ArmsShopBrowse, dialog.GetPhase())
	require.Equal(t, ArmsShopBuy, dialog.GetMode())
	require.Equal(t, ArmsSortQuantity, dialog.GetSort())
	require.Equal(t, references.Bow, dialog.GetSelectedEquipment())
	require.Equal(t, "999", dialog.GetShell().GetHeaderView().PartyGold)
	for _, row := range dialog.GetRows() {
		if row.Equipment == references.Bow {
			require.Equal(t, uint16(99), row.Quantity)
			require.Zero(t, row.Maximum)

			return
		}
	}
	t.Fatal("refreshed Bow row was not present")
}

func TestArmsShopDialogGeometryAndEmptySellAreBounded(t *testing.T) {
	for _, size := range [][2]int{{1280, 720}, {320, 180}} {
		setMerchantShellTestResolution(t, size[0], size[1])
		state := testArmsShopState()
		state.SellQuotes = nil
		keyboard := newWidgetTestKeyboard(0)
		dialog := newTestArmsShopDialog(keyboard, state)
		require.True(t, dialog.GetWorkspaceContentBounds().In(dialog.GetShell().GetWorkspaceBounds()))
		require.True(t, dialog.GetTableBounds().In(dialog.GetShell().GetWorkspaceBounds()))
		require.True(t, dialog.GetDetailBounds().In(dialog.GetShell().GetWorkspaceBounds()))
		pressArmsShopKey(dialog, keyboard, ebiten.KeyS)
		require.Empty(t, dialog.GetRows())
		require.Empty(t, dialog.rowBounds)
		require.Equal(t, dialog.tableHeader, dialog.GetTableBounds())
		require.Equal(t, dialog.GetTableBounds().Max.Y, dialog.GetSortBounds().Min.Y)
		require.Contains(t, dialog.GetDetailText(), "No equipment")
		requireArmsStatusConsumesRemainingSpace(t, dialog)
		pressArmsShopKey(dialog, keyboard, ebiten.KeyEnter)
		_, ok := dialog.ConsumePendingTradeIntent()
		require.False(t, ok)
	}
}

func TestArmsShopDialogAcceptsDualWieldedReadiedQuantities(t *testing.T) {
	// Each active member can ready one copy in each hand.
	const readied = 2 * party_state.MaxActiveParty
	state := testArmsShopState()
	state.BuyQuotes[0].ReadiedQuantity = readied
	state.SellQuotes[0].ReadiedQuantity = readied
	keyboard := newWidgetTestKeyboard(0)

	var dialog *ArmsShopDialog
	require.NotPanics(t, func() { dialog = newTestArmsShopDialog(keyboard, state) })
	for _, mode := range []ebiten.Key{ebiten.KeyB, ebiten.KeyS} {
		pressArmsShopKey(dialog, keyboard, mode)
		for dialog.GetSelectedEquipment() != references.Dagger {
			pressArmsShopKey(dialog, keyboard, ebiten.KeyDown)
		}
		require.Contains(t, strings.Join(strings.Fields(dialog.GetDetailText()), " "), "Ready 12")
	}
}

func testArmsShopState() game_state.ArmsShopState {
	const gold uint16 = 500
	state := game_state.ArmsShopState{
		Merchant: references.MerchantDefinition{
			ID: references.MerchantArmsBritain, Family: references.MerchantFamilyArms,
			OwnerName: "Gwenneth", ShopName: "Iolo's Bows",
		},
		PricingCharacterIndex: 0,
		PricingCharacterName:  "Avatar",
		PricingIntelligence:   20,
		Gold:                  gold,
	}
	equipment := [...]references.Equipment{
		references.Dagger, references.Arrows, references.Bow, references.SmallShield,
		references.LeatherHelm, references.PlateMail, references.LongSword,
	}
	quantities := [...]uint16{8, 20, 5, 4, 1, 2, 3}
	readied := [...]uint16{0, 0, 2, 0, 1, 0, 0}
	prices := [...]int{50, 10, 100, 70, 30, 300, 200}
	for index := range equipment {
		state.BuyQuotes[index] = game_state.ArmsBuyQuote{
			Equipment: equipment[index], BasePrice: prices[index], UnitPrice: prices[index], Gold: gold,
			QuantityBefore: quantities[index], ReadiedQuantity: readied[index], MaxQuantity: 2,
			AmmoRefill: equipment[index].IsAmmo(),
		}
		if equipment[index].IsAmmo() {
			state.BuyQuotes[index].MaxQuantity = 1
		}
	}
	state.SellQuotes = []game_state.ArmsSellQuote{
		{Equipment: references.Dagger, BasePrice: 50, UnitOffer: 40, Gold: gold, QuantityBefore: 2, ReadiedQuantity: 1, MaxQuantity: 2, Sellable: true},
		{Equipment: references.Arrows, BasePrice: 10, Gold: gold, QuantityBefore: 20},
		{Equipment: references.Ankh, Gold: gold, QuantityBefore: 1},
		{Equipment: references.Bow, BasePrice: 100, UnitOffer: 80, Gold: gold, QuantityBefore: 1, MaxQuantity: 0, Sellable: true},
	}

	return state
}

func refreshedArmsState(state game_state.ArmsShopState, gold uint16) game_state.ArmsShopState {
	state.Gold = gold
	state.SellQuotes = append([]game_state.ArmsSellQuote(nil), state.SellQuotes...)
	for index := range state.BuyQuotes {
		state.BuyQuotes[index].Gold = gold
	}
	for index := range state.SellQuotes {
		state.SellQuotes[index].Gold = gold
	}

	return state
}

func newTestArmsShopDialog(keyboard *input.Keyboard, state game_state.ArmsShopState) *ArmsShopDialog {
	return NewArmsShopDialog(testArmsShopDialogConfig(keyboard, state))
}

func testArmsShopDialogConfig(keyboard *input.Keyboard, state game_state.ArmsShopState) ArmsShopDialogConfig {
	return ArmsShopDialogConfig{
		Keyboard: keyboard, MerchantSprite: ebiten.NewImage(16, 16), State: state,
		Prose: ArmsShopDialogProse{
			Greeting:        "Welcome to my Arms shop.",
			BuyPrompt:       "What wouldst thou buy?",
			SellPrompt:      "What wouldst thou sell?",
			CapacityRefusal: "\"Thou canst not carry any more!\" says Gwenneth.",
			FundsRefusal:    "\"Can't pay?! Out with thee!\" yells Gwenneth.",
		},
		Preview:      false,
		IsKeyPressed: nil,
	}
}

func pressArmsShopKey(dialog *ArmsShopDialog, keyboard *input.Keyboard, key ebiten.Key) {
	keyboard.SimulateKeyPress(key)
	dialog.Update()
}

func armsRowEquipment(rows []ArmsShopRowView) []references.Equipment {
	equipment := make([]references.Equipment, len(rows))
	for index, row := range rows {
		equipment[index] = row.Equipment
	}

	return equipment
}

func requireArmsStatusConsumesRemainingSpace(t *testing.T, dialog *ArmsShopDialog) {
	t.Helper()
	workspace := dialog.GetWorkspaceContentBounds()
	sortBounds := dialog.GetSortBounds()
	detailBounds := dialog.GetDetailBounds()
	shellWorkspace := dialog.GetShell().GetWorkspaceBounds()
	inset := max(armsShopMinimumInset, min(shellWorkspace.Dx(), shellWorkspace.Dy())/armsShopInsetDivisor)

	require.Equal(t, sortBounds.Max.Y+inset, detailBounds.Min.Y)
	require.Equal(t, workspace.Max.Y, detailBounds.Max.Y)
	require.GreaterOrEqual(t, dialog.detailTextBounds.Dy(),
		int(math.Ceil(dialog.detailOutput.GetTotalHeightForLines(armsShopDetailLines))))
	require.LessOrEqual(t, sortBounds.Max.Y, detailBounds.Min.Y)
}
