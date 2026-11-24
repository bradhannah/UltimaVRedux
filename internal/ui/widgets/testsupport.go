package widgets

import (
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// Test support: fixtures and seams that tests in other packages use. The
// dead-code check treats this file as test support.

// GetHours returns the hours of rest currently chosen.
func (p *HoursPrompt) GetHours() int { return p.hours }

// GetMode returns the active Buy or Sell table.
func (d *ArmsShopDialog) GetMode() ArmsShopMode { return d.mode }

// GetPhase returns the workflow-owned shop phase.
func (d *ArmsShopDialog) GetPhase() ArmsShopDialogPhase { return d.phase }

// GetSelectedEquipment returns stable selected equipment identity.
func (d *ArmsShopDialog) GetSelectedEquipment() references.Equipment { return d.selectedEquipment() }

// GetRows returns detached active sorted row views.
func (d *ArmsShopDialog) GetRows() []ArmsShopRowView {
	mode := d.activeMode()
	rows := make([]ArmsShopRowView, d.rowCount())
	for index := range rows {
		rows[index] = d.rowView(index, index == mode.list.GetCurrentIndex())
	}

	return rows
}

// GetReceiptText returns the compact detail-strip receipt, if any.
func (d *ArmsShopDialog) GetReceiptText() string { return d.receiptText }

// GetDetailText returns the current two-line detail or receipt presentation.
func (d *ArmsShopDialog) GetDetailText() string {
	if d.detailOutput == nil {
		return ""
	}

	return d.detailOutput.GetOutputStr(false)
}

// GetLatestExchange returns the current finalized merchant exchange.
func (d *ArmsShopDialog) GetLatestExchange() string { return d.latestExchange }

// IsPreview reports whether the dialog stops before transaction authority.
func (d *ArmsShopDialog) IsPreview() bool { return d.preview }

// HasPendingRequest reports whether Enter emitted an unconsumed request.
func (d *ArmsTradeDialog) HasPendingRequest() bool { return d.pending != nil }

// GetQuantity returns the selected transaction quantity.
func (d *ArmsTradeDialog) GetQuantity() uint16 { return d.quantity }

// ButtonWidthPercent reports the configured row width.
func (b *ButtonListModal) ButtonWidthPercent() float64 {
	return b.buttonWidthPercent
}

// ButtonTexts returns the button labels in navigation order.
func (b *ButtonListModal) ButtonTexts() []string {
	texts := make([]string, len(b.items))
	for index, item := range b.items {
		texts[index] = item.text
	}

	return texts
}

// GetTitle returns the action-specific dialog title.
func (d *CharacterSelectionDialog) GetTitle() string { return d.title }

// GetCurrentSelection returns the inspected character index, or -1 for No One.
func (d *CharacterSelectionDialog) GetCurrentSelection() int {
	return d.options[d.selected].CharacterIndex
}

// GetCurrentOption returns a defensive copy of the inspected projection.
func (d *CharacterSelectionDialog) GetCurrentOption() CharacterSelectionOption {
	return cloneCharacterSelectionOption(d.options[d.selected])
}

// LayoutProblemsAt lists what the selector cannot show whole at one screen
// size: a title that needs more than its one row, a roster name the row would
// cut, or detail text taller than its box. An owner uses it to test its wording
// at every supported resolution; the display manager must already be that size.
func (d *CharacterSelectionDialog) LayoutProblemsAt(width, height int) []string {
	shell := d.shell
	shell.ensureLayout(width, height, len(d.options), d.footer())
	var problems []string
	shell.titleOutput.Reset()
	shell.titleOutput.AppendLine().Text(d.title, DialogHeaderColor).Done()
	if shown := strings.TrimSpace(shell.titleOutput.GetOutputStr(false)); shown != d.title {
		problems = append(problems, "title shows as "+shown)
	}
	detailRect := shell.layout.detailTextRect
	shell.detailOutput.SetMaxCharsPerLine(max(1, shell.detailOutput.Font.CharsForWidth(detailRect.Dx())))
	for index, option := range d.options {
		nameRect, _ := shell.rosterRowTextRects(shell.layout.rowRects[index], option.Status)
		shell.appendRosterRow(option, index == d.selected, nameRect.Dx())
		if !strings.Contains(shell.rowOutput.GetOutputStr(false), option.Name) {
			problems = append(problems, "name cut: "+option.Name+" beside "+option.Status)
		}
		shell.detailOutput.Reset()
		shell.appendDetailText(option)
		lines := shell.detailOutput.GetPanelContentLineCount()
		if lines >= characterRosterDialogDetailMaximumLines ||
			shell.detailOutput.GetTotalHeightForLines(lines) > float64(detailRect.Dy()) {
			problems = append(problems, "detail too tall: "+option.Name)
		}
	}

	return problems
}

// GetOptions returns defensive copies of all visible rows.
func (d *CharacterSelectionDialog) GetOptions() []CharacterSelectionOption {
	return cloneCharacterSelectionOptions(d.options)
}

// Count returns the number of dialogs currently on the stack.
func (d *DialogStack) Count() int {
	return len(d.entries)
}

// HasPendingPurchaseRequest reports whether confirmation emitted an unconsumed request.
func (d *GuildPurchaseDialog) HasPendingPurchaseRequest() bool {
	return d.pending != nil
}

// GetBundleCount returns the currently selected number of bundles.
func (d *GuildPurchaseDialog) GetBundleCount() uint16 {
	return d.bundleCount
}

// GetQuote returns the trusted quote displayed by the modal.
func (d *GuildPurchaseDialog) GetQuote() game_state.GuildOfferQuote {
	return d.quote
}

// GetPitch returns the finalized original pitch displayed by the modal.
func (d *GuildPurchaseDialog) GetPitch() string {
	return d.pitch
}

// GetPhase returns the current workflow phase.
func (d *GuildShopDialog) GetPhase() GuildShopDialogPhase {
	return d.phase
}

// GetLatestExchange returns the one currently displayed merchant exchange.
func (d *GuildShopDialog) GetLatestExchange() string {
	return d.latestExchange
}

// IsPreview reports whether the dialog stops before purchase authority.
func (d *GuildShopDialog) IsPreview() bool { return d.preview }

// GetReceiptText returns the current detached receipt/status text.
func (d *GuildShopDialog) GetReceiptText() string {
	return d.receiptText
}

// GetFocus returns the active keyboard pane.
func (d *HealerShopDialog) GetFocus() HealerShopFocus { return d.focus }

// GetPhase returns the inline healer phase.
func (d *HealerShopDialog) GetPhase() HealerShopDialogPhase { return d.phase }

// GetLatestExchange returns the currently displayed merchant speech.
func (d *HealerShopDialog) GetLatestExchange() string { return d.latestExchange }

// GetReceiptText returns the current detached result or refusal text.
func (d *HealerShopDialog) GetReceiptText() string { return d.receiptText }

// IsPreview reports whether this dialog stops before treatment authority.
func (d *HealerShopDialog) IsPreview() bool { return d.preview }

// GetBodyText returns the target, effect, and charge presentation.
func (d *HealerTreatmentDialog) GetBodyText() string {
	return d.window.body.GetOutputStr(false)
}

// GetIntent returns the detached approval data displayed by the modal.
func (d *HorsePurchaseDialog) GetIntent() HorsePurchaseIntent { return d.intent }

// IsPreview reports whether this approval stops before purchase authority.
func (d *HorsePurchaseDialog) IsPreview() bool { return d.intent.Preview }

// GetPhase returns the current one-off workflow phase.
func (d *HorseShopDialog) GetPhase() HorseShopDialogPhase { return d.phase }

// GetQuote returns the detached trusted quote displayed by the showroom.
func (d *HorseShopDialog) GetQuote() game_state.HorsePurchaseQuote { return d.state.Quote }

// GetLatestExchange returns the currently displayed merchant exchange.
func (d *HorseShopDialog) GetLatestExchange() string { return d.latestExchange }

// GetReceiptText returns the current result or terminal status text.
func (d *HorseShopDialog) GetReceiptText() string { return d.receiptText }

// GetShowroomText returns the visible showroom title, item, and adjusted price.
func (d *HorseShopDialog) GetShowroomText() string {
	if !d.showroomVisible() || d.showroomTitleOutput == nil || d.showroomDetailsOutput == nil {
		return ""
	}

	return strings.TrimSpace(
		d.showroomTitleOutput.GetOutputStr(false) + " " + d.showroomDetailsOutput.GetOutputStr(false),
	)
}

// GetStatusText returns the current player-facing status panel content.
func (d *HorseShopDialog) GetStatusText() string {
	if d.statusOutput == nil {
		return ""
	}

	return d.statusOutput.GetOutputStr(false)
}

// IsPreview reports whether this dialog stops before purchase authority.
func (d *HorseShopDialog) IsPreview() bool { return d.preview }

// IsTerminal reports whether the dialog is waiting only for final acknowledgement.
func (d *HorseShopDialog) IsTerminal() bool {
	switch d.phase {
	case HorseShopFarewell, HorseShopSuccess, HorseShopFundsRefusal,
		HorseShopStableClosed, HorseShopPreviewStopped, HorseShopSessionError:
		return true
	case HorseShopBrowse, HorseShopAwaitingApproval, HorseShopAwaitingResult:
		return false
	default:
		panic("HorseShopDialog.IsTerminal: unknown phase")
	}
}

// GetIntent returns a defensive copy of the exact detached approval data.
func (d *InnActionDialog) GetIntent() InnActionIntent { return cloneInnActionIntent(d.intent) }

// GetState returns a defensive copy of the current detached Inn snapshot.
func (d *InnShopDialog) GetState() game_state.InnShopState { return cloneInnShopState(d.state) }

// GetMode returns the persistent Rest, Leave, or Pick Up mode.
func (d *InnShopDialog) GetMode() InnShopMode { return d.mode }

// GetPhase returns the workflow-owned parent phase.
func (d *InnShopDialog) GetPhase() InnShopDialogPhase { return d.phase }

// GetPickupRows returns defensive copies in current-Inn roster order.
func (d *InnShopDialog) GetPickupRows() []game_state.InnGuestRow {
	rows := make([]game_state.InnGuestRow, len(d.state.GuestRows))
	for index, row := range d.state.GuestRows {
		rows[index] = cloneInnGuestRow(row)
	}

	return rows
}

// GetModeHeaderText returns the visible Preview label and persistent mode tabs.
func (d *InnShopDialog) GetModeHeaderText() string {
	if d.modeOutput == nil {
		return ""
	}

	return d.modeOutput.GetOutputStr(false)
}

// GetInlineFeedback returns the latest blocked-action explanation.
func (d *InnShopDialog) GetInlineFeedback() string { return d.inlineFeedback }

// IsPreview reports whether approved actions stop before authority.
func (d *InnShopDialog) IsPreview() bool { return d.preview }

// ActivePrompt returns the prompt currently accepting text.
func (d *InteractionTranscript) ActivePrompt() string {
	return d.activePrompt
}

// TranscriptText returns the currently visible bounded transcript text.
func (d *InteractionTranscript) TranscriptText() string {
	return d.transcriptOutput.GetOutputStr(false)
}

// ControlHint returns the currently visible keyboard guidance.
func (d *InteractionTranscript) ControlHint() string {
	return d.footerOutput.GetOutputStr(false)
}

// GetHeaderView returns the shell's current detached header value.
func (s *MerchantDialogShell) GetHeaderView() MerchantDialogHeaderView {
	return s.header
}

// GetExchangeView returns the shell's current latest exchange.
func (s *MerchantDialogShell) GetExchangeView() MerchantDialogExchangeView {
	return s.exchange
}

// GetFooterView returns the shell's current detached footer value.
func (s *MerchantDialogShell) GetFooterView() MerchantDialogFooterView {
	return s.footer
}

// DissolveLevel reports the current visual dissolve amount for lifecycle tests.
func (md *MessageDialog) DissolveLevel() float64 {
	return md.dissolveLevel
}

// GetMessage returns the dialog message.
func (o *OverlayDialog) GetMessage() string {
	return o.message
}

// GetContentText returns all currently rendered dialog content.
func (o *OverlayDialog) GetContentText() string {
	if o.dialog == nil || o.dialog.output == nil {
		return ""
	}

	return o.dialog.output.GetOutputStr(false)
}

// IsTimed returns true if this is a timed auto-dismiss dialog.
func (o *OverlayDialog) IsTimed() bool {
	return o.isTimed
}

// IsUserDismissed returns true if this is a user-dismissed dialog.
func (o *OverlayDialog) IsUserDismissed() bool {
	return !o.isTimed
}

// GetActiveTab returns the current journal tab.
func (d *QuestJournalDialog) GetActiveTab() QuestJournalTab { return d.activeTab }

// GetSelectedCharacterIndex returns the currently selected character index
func (d *ReadyEquipmentDialog) GetSelectedCharacterIndex() int {
	return d.selectedCharIdx
}

// IsAwaitingConfirmation reports whether the owner is processing an attempt.
func (d *ReagentMixingDialog) IsAwaitingConfirmation() bool { return d.awaitingConfirmation }

// HasPendingPurchaseRequest reports whether confirmation emitted an unconsumed request.
func (d *ReagentPurchaseDialog) HasPendingPurchaseRequest() bool { return d.pending != nil }

// GetBundleCount returns the currently selected number of bundles.
func (d *ReagentPurchaseDialog) GetBundleCount() uint16 { return d.bundleCount }

// GetQuote returns a detached copy of the trusted quote displayed by the modal.
func (d *ReagentPurchaseDialog) GetQuote() game_state.ReagentOfferQuote {
	return cloneReagentOfferQuote(d.quote)
}

// IsPreview reports whether the modal stops before purchase authority.
func (d *ReagentPurchaseDialog) IsPreview() bool { return d.preview }

// GetSelectedQuote returns a detached copy of the selected trusted quote.
func (d *ReagentShopDialog) GetSelectedQuote() game_state.ReagentOfferQuote {
	return cloneReagentOfferQuote(d.selectedQuote())
}

// GetSelectedReagent returns the stable identity of the selected row.
func (d *ReagentShopDialog) GetSelectedReagent() references.Reagent { return d.selectedQuote().Reagent }

// GetCatalogQuotes returns detached copies of all current catalog rows.
func (d *ReagentShopDialog) GetCatalogQuotes() []game_state.ReagentOfferQuote {
	offers := make([]game_state.ReagentOfferQuote, len(d.state.Offers))
	for index, quote := range d.state.Offers {
		offers[index] = cloneReagentOfferQuote(quote)
	}

	return offers
}

// GetPhase returns the current workflow phase.
func (d *ReagentShopDialog) GetPhase() ReagentShopDialogPhase { return d.phase }

// GetLatestExchange returns the currently displayed merchant speech.
func (d *ReagentShopDialog) GetLatestExchange() string { return d.latestExchange }

// GetReceiptText returns the current authoritative receipt or preview status.
func (d *ReagentShopDialog) GetReceiptText() string { return d.receiptText }

// GetDetailText returns the currently displayed selection detail or receipt.
func (d *ReagentShopDialog) GetDetailText() string {
	if d.detailOutput == nil {
		return ""
	}

	return d.detailOutput.GetOutputStr(false)
}

// IsPreview reports whether the dialog stops before purchase authority.
func (d *ReagentShopDialog) IsPreview() bool { return d.preview }

// Submit validates and submits the current text programmatically.
func (m *SaveDescriptionEntryModal) Submit() {
	if m.finished {
		return
	}
	m.submit()
}

// GetText returns the entered description.
func (m *SaveDescriptionEntryModal) GetText() string {
	return m.field.Text()
}

// GetSelectedView returns a detached copy of the selected view.
func (b *SaveSlotBrowser) GetSelectedView() (SaveSlotView, bool) {
	view := b.selectedView()
	if view == nil {
		var emptyView SaveSlotView

		return emptyView, false
	}

	return cloneSaveSlotView(*view), true
}

// HasPendingTravelIntent reports whether D selected an available reviewed escort.
func (d *ShipDispatchDialog) HasPendingTravelIntent() bool { return d.pendingTravel != nil }

// GetState returns the detached refreshed state displayed by the modal.
func (d *ShipDispatchDialog) GetState() game_state.ShipShopState { return d.state }

// HasPendingPurchaseRequest reports whether Enter confirmed this approval.
func (d *ShipPurchaseDialog) HasPendingPurchaseRequest() bool { return d.pending != nil }

// GetIntent returns the trusted detached purchase presentation.
func (d *ShipPurchaseDialog) GetIntent() ShipPurchaseIntent { return d.intent }

// IsPreview reports whether confirmation stops before purchase authority.
func (d *ShipPurchaseDialog) IsPreview() bool { return d.intent.Preview }

// GetState returns the current detached ship-shop snapshot.
func (d *ShipShopDialog) GetState() game_state.ShipShopState { return d.state }

// GetPhase returns the workflow-owned parent phase.
func (d *ShipShopDialog) GetPhase() ShipShopDialogPhase { return d.phase }

// GetManifestText returns the player-facing harbour and selected-offer summary.
func (d *ShipShopDialog) GetManifestText() string {
	if d.manifestOutput == nil {
		return ""
	}

	return d.manifestOutput.GetOutputStr(false)
}

// GetReceiptText returns the current receipt, refusal, or Preview status.
func (d *ShipShopDialog) GetReceiptText() string { return d.receiptText }

// IsPreview reports whether actions stop before workflow authority.
func (d *ShipShopDialog) IsPreview() bool { return d.preview }

// GetIntent returns the detached travel intent displayed by the modal.
func (d *ShipTravelDialog) GetIntent() ShipTravelIntent { return d.intent }

// IsPreview reports whether confirmation stops before transition authority.
func (d *ShipTravelDialog) IsPreview() bool { return d.intent.Preview }

// GetIntent returns a detached copy of the immutable approval union.
func (d *TavernActionDialog) GetIntent() TavernActionIntent { return cloneTavernActionIntent(d.intent) }

// GetProvisionPackCount returns the selected provision quantity.
func (d *TavernActionDialog) GetProvisionPackCount() int { return d.provisionPacks }

// IsPreview reports whether workflow will stop before Tavern authority.
func (d *TavernActionDialog) IsPreview() bool { return d.intent.Preview }

// GetContentText returns the merchant response without dismissal guidance.
func (n *TavernAdviceNotice) GetContentText() string { return n.message }

// GetView returns the immutable finalized presentation.
func (d *TavernResultDialog) GetView() TavernResultView { return d.view }

// GetBodyText returns the rich result body for focused tests.
func (d *TavernResultDialog) GetBodyText() string {
	return d.window.body.GetOutputStr(false)
}

// HasPendingTopicIntent reports whether workflow must ask authority to match raw input.
func (d *TavernShopDialog) HasPendingTopicIntent() bool { return d.pendingTopic != nil }

// GetState returns a defensive copy of the current detached Tavern snapshot.
func (d *TavernShopDialog) GetState() game_state.TavernShopState {
	return cloneTavernShopState(d.state)
}

// GetMode returns the persistent Order or location-specific conversation mode.
func (d *TavernShopDialog) GetMode() TavernShopMode { return d.mode }

// GetPhase returns the workflow-owned parent phase.
func (d *TavernShopDialog) GetPhase() TavernShopDialogPhase { return d.phase }

// GetOrderRows returns only the canonical meal, drink, and available provision choices.
func (d *TavernShopDialog) GetOrderRows() []TavernOrderRow {
	return append([]TavernOrderRow(nil), tavernOrderRows(d.state)...)
}

// GetSelectedOrderIndex returns the stable Order row selection.
func (d *TavernShopDialog) GetSelectedOrderIndex() int { return d.selectedOrder }

// GetTranscript returns a defensive append-only successful paid transcript.
func (d *TavernShopDialog) GetTranscript() []TavernTranscriptEntry {
	return append([]TavernTranscriptEntry(nil), d.transcript...)
}

// GetModeHeaderText returns visible Preview and bright/subdued persistent tabs.
func (d *TavernShopDialog) GetModeHeaderText() string {
	if d.modeOutput == nil {
		return ""
	}

	return d.modeOutput.GetOutputStr(false)
}

// GetWorkspaceText returns current rendered workspace copy for focused tests.
func (d *TavernShopDialog) GetWorkspaceText() string {
	if d.bodyOutput == nil {
		return ""
	}

	return d.bodyOutput.GetOutputStr(false)
}

// GetLatestExchange returns current finalized merchant copy.
func (d *TavernShopDialog) GetLatestExchange() string { return d.latestExchange }

// GetReceiptText returns current workflow-supplied result copy.
func (d *TavernShopDialog) GetReceiptText() string { return d.receiptText }

// GetInlineFeedback returns the current neutral topic failure or session error.
func (d *TavernShopDialog) GetInlineFeedback() string { return d.inlineFeedback }

// IsPreview reports whether approved requests stop before authority.
func (d *TavernShopDialog) IsPreview() bool { return d.preview }

// GetActiveTab returns the selected top tab.
func (d *ZStatsDossierDialog) GetActiveTab() ZStatsDossierTab { return d.activeTab }

// GetSelectedCharacterPosition returns the selected displayed party position, or -1.
func (d *ZStatsDossierDialog) GetSelectedCharacterPosition() int { return d.selectedCharacter }
