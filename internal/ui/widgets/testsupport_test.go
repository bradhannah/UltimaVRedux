package widgets

import (
	"image"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/text"
)

// GetScrollOffset returns the active table's first visible row index.
func (d *ArmsShopDialog) GetScrollOffset() int { return d.activeMode().list.GetScrollOffset() }

// GetVisibleRows returns detached views for the active scroll window.
func (d *ArmsShopDialog) GetVisibleRows() []ArmsShopRowView {
	start, end := d.activeMode().list.GetVisibleRange()
	rows := make([]ArmsShopRowView, 0, end-start)
	for index := start; index < end; index++ {
		rows = append(rows, d.rowView(index, index == d.GetSelectedIndex()))
	}

	return rows
}

// GetWorkspaceContentBounds returns the bounded Arms-owned workspace.
func (d *ArmsShopDialog) GetWorkspaceContentBounds() image.Rectangle {
	d.ensureLayout()

	return d.workspaceContent
}

// GetTableBounds returns the active table frame.
func (d *ArmsShopDialog) GetTableBounds() image.Rectangle {
	d.ensureLayout()

	return d.tableBounds
}

// GetDetailBounds returns the remaining-space detail/receipt panel.
func (d *ArmsShopDialog) GetDetailBounds() image.Rectangle {
	d.ensureLayout()

	return d.detailBounds
}

// GetSortBounds returns the sort status region below the table frame.
func (d *ArmsShopDialog) GetSortBounds() image.Rectangle {
	d.ensureLayout()

	return d.sortBounds
}

// GetBodyText returns the current finalized prose and projections.
func (d *ArmsTradeDialog) GetBodyText() string {
	return outputText(d.window.body)
}

// GetFooterGuidance returns the keyboard guidance as displayed.
func (d *ArmsTradeDialog) GetFooterGuidance() string {
	return outputText(d.window.footer)
}

// flatText returns everything the window shows, title first, as single-spaced words.
func (w *amountWindow) flatText() string {
	return strings.Join(strings.Fields(strings.Join([]string{
		outputText(w.header), outputText(w.unit), outputText(w.body), outputText(w.footer),
	}, " ")), " ")
}

// GetBodyText returns the current quantity and projected-value presentation.
func (d *GuildPurchaseDialog) GetBodyText() string {
	return outputText(d.window.body)
}

// HasPendingPurchaseIntent reports whether the workflow has an unconsumed selection intent.
func (d *GuildShopDialog) HasPendingPurchaseIntent() bool {
	return d.pendingIntent != nil
}

// GetSelectedIndex returns the selected canonical catalog row.
func (d *GuildShopDialog) GetSelectedIndex() int {
	return d.listState.GetCurrentIndex()
}

// GetCatalogQuotes returns the canonical Keys, Gems, Torches snapshot.
func (d *GuildShopDialog) GetCatalogQuotes() [guildShopOfferCount]game_state.GuildOfferQuote {
	return d.offers
}

// GetStatusText returns the currently displayed selection note or receipt.
func (d *GuildShopDialog) GetStatusText() string {
	return outputText(d.statusOutput)
}

// GetCatalogBounds returns the compact currently rendered catalog bounds.
func (d *GuildShopDialog) GetCatalogBounds() image.Rectangle {
	d.ensureLayout()

	return d.catalogBounds
}

// GetStatusBounds returns the always-visible status panel beneath the catalog.
func (d *GuildShopDialog) GetStatusBounds() image.Rectangle {
	d.ensureLayout()

	return d.statusBounds
}

// GetCatalogRowBounds returns one current fixed-height row.
func (d *GuildShopDialog) GetCatalogRowBounds(index int) image.Rectangle {
	if index < 0 || index >= guildShopOfferCount {
		panic("GuildShopDialog.GetCatalogRowBounds: index out of range")
	}
	d.ensureLayout()

	return d.catalogRows[index]
}

// IsAwaitingResult reports whether duplicate workflow completion is blocked.
func (d *GuildShopDialog) IsAwaitingResult() bool {
	return d.phase == GuildShopAwaitingResult
}

// GetDetailText returns the current four-line action projection.
func (d *HealerShopDialog) GetDetailText() string {
	return outputText(d.detailOutput)
}

// GetServicePaneBounds returns the rendered service pane.
func (d *HealerShopDialog) GetServicePaneBounds() image.Rectangle {
	d.ensureLayout()

	return d.serviceBounds
}

// GetRecipientPaneBounds returns the rendered recipient pane.
func (d *HealerShopDialog) GetRecipientPaneBounds() image.Rectangle {
	d.ensureLayout()

	return d.recipientBounds
}

// GetDetailBounds returns the remaining-space status panel.
func (d *HealerShopDialog) GetDetailBounds() image.Rectangle {
	d.ensureLayout()

	return d.detailBounds
}

// IsAwaitingResult reports whether duplicate workflow completion is blocked.
func (d *HealerShopDialog) IsAwaitingResult() bool { return d.phase == HealerShopAwaitingResult }

// GetIntent returns the detached treatment shown by the modal.
func (d *HealerTreatmentDialog) GetIntent() HealerTreatmentIntent { return d.intent }

// HasPendingPurchaseRequest reports whether confirmation emitted an unconsumed request.
func (d *HorsePurchaseDialog) HasPendingPurchaseRequest() bool { return d.pending != nil }

// HasPendingPurchaseIntent reports whether an unconsumed approval intent exists.
func (d *HorseShopDialog) HasPendingPurchaseIntent() bool { return d.pendingIntent != nil }

// GetShowroomBounds returns the natural-height card, or an empty rectangle when the stable is closed.
func (d *HorseShopDialog) GetShowroomBounds() image.Rectangle {
	d.ensureLayout()

	return d.showroomBounds
}

// GetStatusBounds returns the framed panel consuming the remaining inset workspace.
func (d *HorseShopDialog) GetStatusBounds() image.Rectangle {
	d.ensureLayout()

	return d.statusBounds
}

// GetHorseSpriteBounds returns the scaled horse drawing region inside the showroom.
func (d *HorseShopDialog) GetHorseSpriteBounds() image.Rectangle {
	d.ensureLayout()

	return d.horseSpriteBounds
}

// IsAwaitingResult reports whether duplicate workflow completion is blocked.
func (d *HorseShopDialog) IsAwaitingResult() bool { return d.phase == HorseShopAwaitingResult }

// HasPendingRequest reports whether Enter approved an unconsumed request.
func (d *InnActionDialog) HasPendingRequest() bool { return d.pending != nil }

// GetTitleText returns the visible action title, including PREVIEW when applicable.
func (d *InnActionDialog) GetTitleText() string {
	return outputText(d.window.header)
}

// GetFooterGuidance returns the keyboard guidance as displayed.
func (d *InnActionDialog) GetFooterGuidance() string {
	return outputText(d.window.footer)
}

// GetBodyText returns the exact mode-specific approval consequences.
func (d *InnActionDialog) GetBodyText() string {
	return outputText(d.window.body)
}

// GetBodyBounds returns the semantically backed modal body.
func (d *InnActionDialog) GetBodyBounds() image.Rectangle {
	d.window.ensureLayout()

	return d.window.frame.bodyBounds
}

// IsPreview reports whether confirmation stops before Inn authority.
func (d *InnActionDialog) IsPreview() bool { return d.intent.Preview }

// ShowWithoutAnimation makes a non-complete approval immediately visible.
func (d *InnActionDialog) ShowWithoutAnimation() {
	if !d.dismissed {
		d.visible = true
	}
}

// IsComplete reports whether the approval has confirmed, cancelled, or been dismissed.
func (d *InnActionDialog) IsComplete() bool { return d.dismissed }

// HasPendingActionIntent reports whether the workflow must open InnActionDialog.
func (d *InnShopDialog) HasPendingActionIntent() bool { return d.pendingIntent != nil }

// ApplyState replaces a complete detached snapshot while preserving mode and stable row identity.
func (d *InnShopDialog) ApplyState(refreshed game_state.InnShopState) {
	if d.phase != InnShopBrowse {
		panic("InnShopDialog.ApplyState: dialog must be browsing")
	}
	d.applyState(refreshed)
	d.clearTransientPresentation()
	d.syncPresentation()
}

// IsHelpVisible reports whether the concise Inn service guide owns the workspace.
func (d *InnShopDialog) IsHelpVisible() bool { return d.helpVisible }

// GetSelectedIdentity returns the selected stable character identity for list modes.
func (d *InnShopDialog) GetSelectedIdentity() (game_state.InnCharacterIdentity, bool) {
	switch d.mode {
	case InnShopLeave:
		if len(d.state.LeaveRows) > 0 {
			return d.state.LeaveRows[d.selectedLeave].Identity, true
		}
	case InnShopPickup:
		if len(d.state.GuestRows) > 0 {
			return d.state.GuestRows[d.selectedPickup].Identity, true
		}
	case InnShopRest:
	default:
		if d.mode != InnShopRest {
			panic("InnShopDialog.GetSelectedIdentity: unknown mode")
		}
	}

	return emptyInnCharacterIdentity(), false
}

// GetLeaveRows returns defensive copies in original active-party order.
func (d *InnShopDialog) GetLeaveRows() []game_state.InnLeaveRow {
	rows := make([]game_state.InnLeaveRow, len(d.state.LeaveRows))
	for index, row := range d.state.LeaveRows {
		rows[index] = cloneInnLeaveRow(row)
	}

	return rows
}

// GetDetailText returns the active consequence card, row detail, or receipt.
func (d *InnShopDialog) GetDetailText() string {
	return outputText(d.detailOutput)
}

// GetCardBounds returns the Rest consequence card or list-mode detail card.
func (d *InnShopDialog) GetCardBounds() image.Rectangle {
	d.ensureLayout()

	return d.cardBounds
}

// GetTableBounds returns the Leave/Pick Up table, or an empty rectangle in Rest mode.
func (d *InnShopDialog) GetTableBounds() image.Rectangle {
	d.ensureLayout()

	return d.tableBounds
}

// IsAwaitingResult reports whether workflow authority owns an approved request.
func (d *InnShopDialog) IsAwaitingResult() bool { return d.phase == InnShopAwaitingResult }

// GetSelectedCategory returns the active non-empty tab index.
func (d *ItemSelectionDialog) GetSelectedCategory() int { return d.activeCategory }

// GetSelectedItemIndex returns the selected index within the active category.
func (d *ItemSelectionDialog) GetSelectedItemIndex() int {
	if len(d.categories) == 0 {
		return -1
	}

	return d.selections[d.currentCategory()]
}

// GetDetailPage returns the zero-based detail page.
func (d *ItemSelectionDialog) GetDetailPage() int { return d.detailPage }

// GetHeaderBounds returns the full-width common header region.
func (s *MerchantDialogShell) GetHeaderBounds() image.Rectangle {
	s.ensureLayout()

	return s.layout.header
}

// GetBodyBounds returns the complete two-column body region.
func (s *MerchantDialogShell) GetBodyBounds() image.Rectangle {
	s.ensureLayout()

	return s.layout.body
}

// GetMerchantPaneBounds returns the common portrait and latest-exchange column.
func (s *MerchantDialogShell) GetMerchantPaneBounds() image.Rectangle {
	s.ensureLayout()

	return s.layout.merchantPane
}

// GetPortraitBounds returns the common merchant portrait and name region.
func (s *MerchantDialogShell) GetPortraitBounds() image.Rectangle {
	s.ensureLayout()

	return s.layout.portrait
}

// GetExchangeBounds returns the bounded latest-exchange region.
func (s *MerchantDialogShell) GetExchangeBounds() image.Rectangle {
	s.ensureLayout()

	return s.layout.exchange
}

// GetFooterBounds returns the full-width keyboard-guidance footer.
func (s *MerchantDialogShell) GetFooterBounds() image.Rectangle {
	s.ensureLayout()

	return s.layout.footer
}

// GetFirstSelectedIndex returns the locked character index.
func (d *NewOrderDialog) GetFirstSelectedIndex() int { return d.firstSelectedIndex }

// GetStage returns the active selection stage.
func (d *NewOrderDialog) GetStage() NewOrderStage { return d.stage }

// GetTitle returns the contextual current title.
func (d *NewOrderDialog) GetTitle() string { return d.title() }

// GetCurrentSelectionIndex returns the inspected party character index.
func (d *NewOrderDialog) GetCurrentSelectionIndex() int {
	return d.options[d.selected].CharacterIndex
}

// GetOptions returns the current detached rows, including stage-specific preview.
func (d *NewOrderDialog) GetOptions() []CharacterSelectionOption { return d.viewOptions() }

// GetHorizontalPercent returns the horizontal position as a percentage (0.0-1.0)
func (p Position) GetHorizontalPercent() float64 {
	switch p.Horizontal {
	case HorizontalMapCenter:
		return 0.37 // Map area center
	case HorizontalScreenCenter:
		return 0.50 // Full screen center
	default:
		return 0.50
	}
}

// GetSelectedIndex returns the current tab's selection, or -1 for an empty tab.
func (d *QuestJournalDialog) GetSelectedIndex() int {
	if d.searching {
		if len(d.searchResults) == 0 {
			return -1
		}

		return d.searchSelection
	}
	if d.currentItemCount() == 0 {
		return -1
	}

	return d.selections[d.activeTab]
}

// GetDetailPage returns the zero-based detail page.
func (d *QuestJournalDialog) GetDetailPage() int { return d.detailPage }

// GetBodyText returns the selected supplied approval presentation.
func (d *ReagentPurchaseDialog) GetBodyText() string {
	return outputText(d.window.body)
}

// GetFooterGuidance returns the keyboard guidance as displayed.
func (d *ReagentPurchaseDialog) GetFooterGuidance() string {
	return outputText(d.window.footer)
}

// HasPendingPurchaseIntent reports whether the workflow has an unconsumed modal intent.
func (d *ReagentShopDialog) HasPendingPurchaseIntent() bool { return d.pendingIntent != nil }

// GetCatalogBounds returns the catalog region spanning the family workspace width.
func (d *ReagentShopDialog) GetCatalogBounds() image.Rectangle {
	d.ensureLayout()

	return d.catalogBounds
}

// GetDetailBounds returns the detail region beneath the dynamic catalog.
func (d *ReagentShopDialog) GetDetailBounds() image.Rectangle {
	d.ensureLayout()

	return d.detailBounds
}

// IsAwaitingResult reports whether authority owns the accepted request.
func (d *ReagentShopDialog) IsAwaitingResult() bool { return d.phase == ReagentShopAwaitingResult }

// AutoCloseAndPop removes this exact widget instance after its result callback.
func (m *SaveDescriptionEntryModal) AutoCloseAndPop(stack *DialogStack) *SaveDescriptionEntryModal {
	if stack == nil {
		panic("SaveDescriptionEntryModal.AutoCloseAndPop: dialog stack is nil")
	}
	m.autoCloseStack = stack

	return m
}

// Dismiss cancels the modal programmatically. Repeated dismissal is harmless.
func (m *SaveDescriptionEntryModal) Dismiss() {
	m.finish(false)
}

// GetValidationMessage returns the current inline validation or native input error.
func (m *SaveDescriptionEntryModal) GetValidationMessage() string {
	return m.validationMessage
}

// SetValidationMessage replaces the current inline validation message.
func (m *SaveDescriptionEntryModal) SetValidationMessage(message string) {
	m.validationMessage = strings.TrimSpace(message)
}

// IsFinished reports whether the modal has submitted or cancelled.
func (m *SaveDescriptionEntryModal) IsFinished() bool {
	return m.finished
}

// GetResult returns the modal's SaveDescriptionEntryResult.
func (m *SaveDescriptionEntryModal) GetResult() interface{} {
	return m.result
}

// SetNotice replaces the browser-level status message without changing selection.
func (b *SaveSlotBrowser) SetNotice(notice string) {
	b.notice = strings.TrimSpace(notice)
	b.populateDetailOutput()
	b.populateFooterOutput()
}

// IsScrollingNeeded returns true if there are more items than visible slots.
func (s *ScrollableListState) IsScrollingNeeded() bool {
	return s.totalItems > s.visibleSlots
}

// GetTitleText returns the dispatch title.
func (d *ShipDispatchDialog) GetTitleText() string {
	return outputText(d.window.header)
}

// GetFooterGuidance returns the keyboard guidance as displayed.
func (d *ShipDispatchDialog) GetFooterGuidance() string {
	return outputText(d.window.footer)
}

// GetBodyText returns the player-facing berth, vessel, skiff, and escort summary.
func (d *ShipDispatchDialog) GetBodyText() string {
	return outputText(d.window.body)
}

// GetBodyBounds returns the semantically backed dispatch body.
func (d *ShipDispatchDialog) GetBodyBounds() image.Rectangle {
	d.window.ensureLayout()

	return d.window.frame.bodyBounds
}

// GetQuantity returns the selected vessel or skiff quantity.
func (d *ShipPurchaseDialog) GetQuantity() int { return d.quantity }

// GetBodyText returns the vessel, berth, skiff, and payment presentation.
func (d *ShipPurchaseDialog) GetBodyText() string {
	return outputText(d.window.body)
}

// GetBodyBounds returns the semantically backed modal body.
func (d *ShipPurchaseDialog) GetBodyBounds() image.Rectangle {
	d.window.ensureLayout()

	return d.window.frame.bodyBounds
}

// HasPendingPurchaseIntent reports whether a purchase modal must be opened.
func (d *ShipShopDialog) HasPendingPurchaseIntent() bool { return d.pendingBuy != nil }

// RefreshBrowse replaces detached state while preserving stable vehicle selection.
func (d *ShipShopDialog) RefreshBrowse(refreshed game_state.ShipShopState) {
	if d.phase != ShipShopBrowse {
		panic("ShipShopDialog.RefreshBrowse: dialog is not browsing")
	}
	d.applyRefresh(refreshed, d.GetSelectedVehicle())
	d.syncPresentation()
}

// GetSelectedIndex returns zero for Frigate and one for Skiff.
func (d *ShipShopDialog) GetSelectedIndex() int { return d.selected }

// GetOfferCardText returns the visible text for one vessel card.
func (d *ShipShopDialog) GetOfferCardText(index int) string {
	if index < 0 || index >= shipShopOfferCount {
		panic("ShipShopDialog.GetOfferCardText: index out of range")
	}
	if d.cardOutputs[index] == nil {
		return ""
	}

	return d.cardOutputs[index].GetOutputStr(false)
}

// GetLatestExchange returns the current finalized merchant exchange.
func (d *ShipShopDialog) GetLatestExchange() string { return d.latestExchange }

// GetShowroomBounds returns the dual-card region.
func (d *ShipShopDialog) GetShowroomBounds() image.Rectangle {
	d.ensureLayout()

	return d.showroomBounds
}

// GetOfferCardBounds returns one Frigate or Skiff card region.
func (d *ShipShopDialog) GetOfferCardBounds(index int) image.Rectangle {
	if index < 0 || index >= shipShopOfferCount {
		panic("ShipShopDialog.GetOfferCardBounds: index out of range")
	}
	d.ensureLayout()

	return d.cardBounds[index]
}

// GetManifestBounds returns the remaining-space manifest region.
func (d *ShipShopDialog) GetManifestBounds() image.Rectangle {
	d.ensureLayout()

	return d.manifestBounds
}

// IsAwaitingResult reports whether purchase or travel authority currently owns the action.
func (d *ShipShopDialog) IsAwaitingResult() bool {
	return d.phase == ShipShopAwaitingPurchaseResult || d.phase == ShipShopAwaitingTravelResult
}

// HasPendingTravelReview reports whether Enter confirmed this approval.
func (d *ShipTravelDialog) HasPendingTravelReview() bool { return d.pending != nil }

// GetTitleText returns the harbour approval title.
func (d *ShipTravelDialog) GetTitleText() string {
	return outputText(d.window.header)
}

// GetFooterGuidance returns the keyboard guidance as displayed.
func (d *ShipTravelDialog) GetFooterGuidance() string {
	return outputText(d.window.footer)
}

// GetBodyText returns the player-facing harbour, berth, vessel, and time summary.
func (d *ShipTravelDialog) GetBodyText() string {
	return outputText(d.window.body)
}

// GetBodyBounds returns the semantically backed travel body.
func (d *ShipTravelDialog) GetBodyBounds() image.Rectangle {
	d.window.ensureLayout()

	return d.window.frame.bodyBounds
}

// GetMinValue returns the minimum value (test accessor)
func (s *Slider) GetMinValue() int {
	return s.minValue
}

// GetLabel returns the slider label (test accessor)
func (s *Slider) GetLabel() string {
	return s.label
}

// GetMaxValue returns the maximum value
func (s *Slider) GetMaxValue() int {
	return s.maxValue
}

// IsVisible returns whether the slider is visible
func (s *Slider) IsVisible() bool {
	return s.isVisible
}

// SetVisible sets whether the slider is visible
func (s *Slider) SetVisible(visible bool) {
	s.isVisible = visible
}

// HasPendingRequest reports whether an approval emitted an unconsumed request.
func (d *TavernActionDialog) HasPendingRequest() bool { return d.pending != nil }

// GetTitleText returns the visible action title, including PREVIEW when applicable.
func (d *TavernActionDialog) GetTitleText() string {
	return outputText(d.window.header)
}

// GetFooterGuidance returns the keyboard guidance as displayed.
func (d *TavernActionDialog) GetFooterGuidance() string {
	return outputText(d.window.footer)
}

// GetBodyText returns exact mode-specific approval presentation.
func (d *TavernActionDialog) GetBodyText() string {
	return outputText(d.window.body)
}

// ShowWithoutAnimation makes a non-complete approval immediately visible.
func (d *TavernActionDialog) ShowWithoutAnimation() {
	if !d.dismissed {
		d.visible = true
	}
}

// IsComplete reports whether approval confirmed, cancelled, or was dismissed.
func (d *TavernActionDialog) IsComplete() bool { return d.dismissed }

// GetBodyBounds returns the semantically backed body region.
func (d *TavernResultDialog) GetBodyBounds() image.Rectangle {
	d.window.ensureLayout()

	return d.window.frame.bodyBounds
}

// ShowWithoutAnimation makes an incomplete result visible.
func (d *TavernResultDialog) ShowWithoutAnimation() {
	if !d.dismissed {
		d.visible = true
	}
}

// IsComplete reports whether the result was acknowledged or dismissed.
func (d *TavernResultDialog) IsComplete() bool { return d.dismissed }

// HasPendingActionIntent reports whether workflow must open TavernActionDialog.
func (d *TavernShopDialog) HasPendingActionIntent() bool { return d.pendingIntent != nil }

// ApplyState replaces the complete detached Tavern snapshot while preserving stable presentation state.
func (d *TavernShopDialog) ApplyState(refreshed game_state.TavernShopState) {
	if d.phase != TavernShopBrowse {
		panic("TavernShopDialog.ApplyState: dialog must be browsing")
	}
	d.applyState(refreshed)
	d.syncTopicFocus()
	d.clearTransientPresentation()
	d.syncPresentation()
}

// GetOrderSubflow returns the current menu or wine state.
func (d *TavernShopDialog) GetOrderSubflow() TavernOrderSubflow { return d.subflow }

// GetSelectedWineIndex returns the source-order selected wine row.
func (d *TavernShopDialog) GetSelectedWineIndex() int { return d.selectedWine }

// GetTranscriptViewport returns a defensive copy of the currently visible paid pairs.
func (d *TavernShopDialog) GetTranscriptViewport() []TavernTranscriptEntry {
	start, end := d.transcriptViewportRange()

	return append([]TavernTranscriptEntry(nil), d.transcript[start:end]...)
}

// GetTranscriptScroll returns the first visible transcript pair index.
func (d *TavernShopDialog) GetTranscriptScroll() int { return d.transcriptScroll }

// ShowWithoutAnimation makes a non-complete Tavern parent immediately visible.
func (d *TavernShopDialog) ShowWithoutAnimation() {
	if !d.dismissed {
		d.visible = true
		d.prepareForTransition()
	}
}

// Hide releases the native subject session while retaining the Tavern workspace.
func (d *TavernShopDialog) Hide() {
	d.visible = false
	d.prepareForTransition()
}

// IsComplete reports whether the parent has been permanently dismissed.
func (d *TavernShopDialog) IsComplete() bool { return d.dismissed }

// GetSelectedListIndex returns the current tab's selection.
func (d *ZStatsDossierDialog) GetSelectedListIndex() int {
	if d.currentItemCount() == 0 {
		return -1
	}

	return d.currentSelection()
}

// outputText returns the finalized text of output, or "" before layout creates it.
func outputText(output *text.Output) string {
	if output == nil {
		return ""
	}

	return output.GetOutputStr(false)
}
