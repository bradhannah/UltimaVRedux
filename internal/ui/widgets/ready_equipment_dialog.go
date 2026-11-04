package widgets

import (
	"errors"
	"fmt"
	"image"
	stdcolor "image/color"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/profiling"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

// Ready Equipment Dialog specific colors (unexported - unique to this dialog)
//
//nolint:gochecknoglobals // Immutable semantic palette values retain the color.Color interface for mixed styles.
var (
	// readyEquippedMarkerColor is for the "[E]" marker showing items currently equipped
	readyEquippedMarkerColor stdcolor.Color = color.EGALightGreen
	// readySlotIconColor is for the equipment slot type icons (helm, weapon, etc.)
	readySlotIconColor stdcolor.Color = color.EGAYellow
)

var _ Widget = (*ReadyEquipmentDialog)(nil)

// ReadyEquipmentDialog presents an allowance header, equipped/available/comparison
// panes, and a footer. It emits reviewed intents; the scene commits gameplay changes.
type ReadyEquipmentDialog struct {
	navigation
	// Layout components - summary, three gear panes, and status bar.
	outerBorder  *Border
	charBorder   *Border // Character and Strength summary.
	slotsBorder  *Border // Equipped slots.
	itemsBorder  *Border // Available items.
	detailBorder *Border // Item comparison.
	statusBorder *Border // Bottom: status bar with controls
	frameRects   []image.Rectangle

	// Display components
	font            *text.UltimaFont
	titleFont       *text.UltimaFont
	titleOutput     *text.Output
	sectionOutput   *text.Output
	charOutput      *text.Output
	slotsOutput     *text.Output
	itemsOutput     *text.Output
	statusOutput    *text.Output
	detailOutput    *text.Output
	indicatorOutput *text.Output
	detailPage      int
	spriteSheet     *sprites.SpriteSheet

	// Input handling
	keyboard *input.Keyboard

	// Party data
	partyState *party_state.PartyState
	inventory  *party_state.Inventory
	gameState  *game_state.GameState

	// State
	selectedCharIdx  int                  // 0-5 party member index
	availableItems   []availableEquipment // Filtered list of equippable items
	itemListNav      *ScrollableListState // Navigation for item list
	warningMessage   string               // Error/warning message to display
	request          game_state.ReadyEquipmentRequest
	targetHand       references.EquipmentSlot
	review           *game_state.ReadyEquipmentTransaction
	confirmation     *MessageDialog
	pending          *game_state.ReadyEquipmentTransaction
	waitingForResult bool

	// Dialog state
	isDismissed bool
	isVisible   bool

	// Drawing
	drawOp *ebiten.DrawImageOptions

	lastScreenWidth  int
	lastScreenHeight int

	content              *ebiten.Image
	contentKey           readyContentKey
	contentSet           bool
	contentRevision      uint64
	contentBuilds        uint64
	selectionDrawStarted time.Time
}

// availableEquipment represents an item in the available items list
type availableEquipment struct {
	Equipment       references.Equipment
	Quantity        uint16
	Equipped        bool   // True if currently equipped on selected character
	CanEquip        bool   // True if character can equip this item (STR check, etc.)
	EquipError      string // Error message if CanEquip is false
	StrengthBlocked bool   // The budget owns the explanation of this refusal.
	Preview         game_state.ReadyEquipmentPreview
}

// NewReadyEquipmentDialog creates a new ready equipment dialog.
// gameState is required so normal play and debug tooling share equipment transactions.
func NewReadyEquipmentDialog(
	keyboard *input.Keyboard,
	spriteSheet *sprites.SpriteSheet,
	gameState *game_state.GameState,
	request game_state.ReadyEquipmentRequest,
) *ReadyEquipmentDialog {
	if gameState == nil {
		panic("ReadyEquipmentDialog: gameState is required")
	}
	if keyboard == nil {
		panic("ReadyEquipmentDialog: keyboard is required")
	}

	//nolint:exhaustruct_v5 // Layout is initialized below; transient selection and cache start empty.
	dialog := &ReadyEquipmentDialog{
		navigation:      newNavigation(keyboard.IsKeyHeld),
		keyboard:        keyboard,
		spriteSheet:     spriteSheet,
		partyState:      &gameState.PartyState,
		inventory:       &gameState.PartyState.Inventory,
		gameState:       gameState,
		selectedCharIdx: request.CharacterIndex,
		request:         request,
		isVisible:       true,
		drawOp:          &ebiten.DrawImageOptions{},
	}

	dialog.ensureLayout()

	// Build available items list
	dialog.updateAvailableItems()

	return dialog
}

// initializeTypography sets up fonts and text rendering
func (d *ReadyEquipmentDialog) initializeTypography() {
	// Intentional compact typography: Ready's dense gear columns need the original
	// 20-size body, not the 23-size main-screen body used by broader readers.
	d.font, _ = GetScaledFontWithOutput(FontConfig{
		Size:        FontSizeCompact,
		MaxLines:    1,
		MaxChars:    readyInitialChars,
		LineSpacing: 0, LineSpacingMultiplier: 0,
	})

	// The character name is the window title; panel headings are section labels.
	d.titleFont, d.titleOutput = GetScaledFontWithOutput(FontConfig{
		Size:        FontSizeTitle,
		MaxLines:    1,
		MaxChars:    readyInitialChars,
		LineSpacing: 0, LineSpacingMultiplier: 0,
	})
	_, d.sectionOutput = GetScaledFontWithOutput(FontConfig{
		Size:        FontSizeBody,
		MaxLines:    1,
		MaxChars:    readyInitialChars,
		LineSpacing: 0, LineSpacingMultiplier: 0,
	})
}

// initializeOutputs creates text outputs with proper maxCharsPerLine
func (d *ReadyEquipmentDialog) initializeOutputs() {
	lineSpacing := d.font.TextFace.Size * readyLineSpacingMultiplier

	// Character panel output
	charBounds := d.charBorder.GetInteriorBounds()
	charChars := d.font.CharsForWidth(charBounds.Dx() - itemDialogTextPadding*readyCenterDivisor)
	d.charOutput = text.NewOutput(d.font, lineSpacing, readyCharacterLines, charChars)
	d.indicatorOutput = text.NewOutput(d.font, lineSpacing, 1, readyIndicatorChars)

	// Slots panel output (used for row-by-row rendering)
	d.slotsOutput = text.NewOutput(d.font, lineSpacing, 1, readyInitialSlotChars)

	// Items list output (single line at a time)
	d.itemsOutput = text.NewOutput(d.font, lineSpacing, 1, readyInitialItemChars)

	// Status bar output
	statusBounds := d.statusBorder.GetInteriorBounds()
	statusChars := d.font.CharsForWidth(statusBounds.Dx() - itemDialogTextPadding*readyCenterDivisor)
	d.statusOutput = text.NewOutput(d.font, lineSpacing, readyStatusLines, statusChars)

	// Retain all wrapped details; the detail viewport exposes them through paging.
	detailBounds := d.detailBorder.GetInteriorBounds()
	detailChars := d.font.CharsForWidth(detailBounds.Dx() - itemDialogTextPadding*readyCenterDivisor)
	d.detailOutput = text.NewOutput(d.font, lineSpacing, readyDetailLines, detailChars)
}

// createBorders creates a summary, three gear panes, and a controls footer.
//
//nolint:funlen // Keep the interdependent pane placements and shared frame grid in one geometry pass.
func (d *ReadyEquipmentDialog) createBorders() {
	// Dialog positioning (5% margin on each side = 90% width, 10% wider than standard)
	const readyDialogMarginX = 0.05
	dialogPlacement := sprites.PercentBasedPlacement{
		StartPercentX: readyDialogMarginX,
		EndPercentX:   1.0 - readyDialogMarginX,
		StartPercentY: itemDialogMarginY,
		EndPercentY:   1.0 - itemDialogMarginY,
	}

	// Outer dialog border
	d.outerBorder = NewOpaqueBorder(dialogPlacement, itemDialogBorderWidth, color.DialogBackground.RGBA())

	// Size the two identity columns from full names, icons and quantities.
	slotsWidth, itemsWidth := d.readyColumnWidths(d.outerBorder.GetBounds().Dx())

	// X coordinates for column boundaries
	xStart := readyDialogMarginX
	xSplit1 := xStart + float64(slotsWidth)/float64(d.lastScreenWidth)
	xSplit2 := xSplit1 + float64(itemsWidth)/float64(d.lastScreenWidth)
	xEnd := 1.0 - readyDialogMarginX

	// Horizontal overlap for vertical borders between columns
	overlap := 0.0038

	// Reserve space for wrapped controls and the persistent Strength summary.
	statusHeightPercent := 0.16
	summaryHeightPercent := 0.13
	dialogHeight := 1.0 - readyCenterDivisor*itemDialogMarginY
	mainPaneStartY := itemDialogMarginY + dialogHeight*summaryHeightPercent
	mainPaneEndY := (1.0 - itemDialogMarginY) - dialogHeight*statusHeightPercent

	// Vertical overlap for horizontal border between main panes and status bar
	verticalOverlap := LayoutSpacingMedium - LayoutSpacingSmall
	statusStartY := mainPaneEndY - verticalOverlap

	// Full-width character and Strength summary.
	charPlacement := sprites.PercentBasedPlacement{
		StartPercentX: xStart,
		EndPercentX:   xEnd,
		StartPercentY: itemDialogMarginY,
		EndPercentY:   mainPaneStartY,
	}
	d.charBorder = NewOpaqueBorder(charPlacement, itemDialogBorderWidth, color.DialogBackground.RGBA())

	// Pane 2: Equipment Slots
	slotsPlacement := sprites.PercentBasedPlacement{
		StartPercentX: xStart,
		EndPercentX:   xSplit1,
		StartPercentY: mainPaneStartY - verticalOverlap,
		EndPercentY:   mainPaneEndY,
	}
	d.slotsBorder = NewOpaqueBorder(slotsPlacement, itemDialogBorderWidth, color.DialogBackground.RGBA())

	// Pane 3: Available Items
	itemsPlacement := sprites.PercentBasedPlacement{
		StartPercentX: xSplit1 - overlap,
		EndPercentX:   xSplit2,
		StartPercentY: mainPaneStartY - verticalOverlap,
		EndPercentY:   mainPaneEndY,
	}
	d.itemsBorder = NewOpaqueBorder(itemsPlacement, itemDialogBorderWidth, color.DialogBackground.RGBA())

	// Pane 4: Item Details (right)
	detailPlacement := sprites.PercentBasedPlacement{
		StartPercentX: xSplit2 - overlap,
		EndPercentX:   xEnd,
		StartPercentY: mainPaneStartY - verticalOverlap,
		EndPercentY:   mainPaneEndY,
	}
	d.detailBorder = NewOpaqueBorder(detailPlacement, itemDialogBorderWidth, color.DialogBackground.RGBA())

	// --- STATUS BAR (bottom, full width) ---
	statusPlacement := sprites.PercentBasedPlacement{
		StartPercentX: xStart,
		EndPercentX:   xEnd,
		StartPercentY: statusStartY,
		EndPercentY:   1.0 - itemDialogMarginY,
	}
	d.statusBorder = NewOpaqueBorder(statusPlacement, itemDialogBorderWidth, color.DialogBackground.RGBA())
	d.frameRects = connectClassicPaneGrid([][]image.Rectangle{
		{d.charBorder.GetBounds()},
		{
			d.slotsBorder.GetBounds(),
			d.itemsBorder.GetBounds(),
			d.detailBorder.GetBounds(),
		},
		{
			d.statusBorder.GetBounds(),
		},
	}, getClassicUIPixelUnit(d.lastScreenWidth, d.lastScreenHeight))
}

// updateAvailableItems rebuilds the list of available equipment from inventory
// Preserves the current selection position when possible
func (d *ReadyEquipmentDialog) updateAvailableItems() {
	d.contentRevision++
	d.detailPage = 0
	// Save current position before rebuilding
	var savedIndex int
	if d.itemListNav != nil {
		savedIndex = d.itemListNav.GetCurrentIndex()
	}

	d.availableItems = nil

	// Get currently equipped items for the selected character
	char := d.getSelectedCharacter()
	if char == nil {
		return
	}

	// Build set of equipped equipment for quick lookup
	equippedSet := make(map[references.Equipment]bool)
	for slot := references.SlotHelm; slot <= references.SlotAmulet; slot++ {
		equipped := char.GetEquippedItem(slot)
		if equipped != references.NoEquipment {
			equippedSet[equipped] = true
		}
	}

	// Iterate through all equipment types and check inventory
	for equipType := references.LeatherHelm; equipType <= references.Ankh; equipType++ {
		qty := d.inventory.Equipment.Get(equipType)
		isEquipped := equippedSet[equipType]

		// Only skip if not equipped AND qty is 0
		// (equipped items must remain visible even with 0 extras in inventory)
		if qty == 0 && !isEquipped {
			continue
		}

		preview, err := d.gameState.PreviewReadyEquipment(d.transactionFor(equipType))
		canEquip := err == nil
		equipError := ""
		if err != nil {
			equipError = err.Error()
		}

		d.availableItems = append(d.availableItems, availableEquipment{
			Equipment:       equipType,
			Quantity:        qty,
			Equipped:        isEquipped,
			CanEquip:        canEquip,
			EquipError:      equipError,
			StrengthBlocked: errors.Is(err, game_state.ErrReadyEquipmentStrength),
			Preview:         preview,
		})
	}

	// Initialize/update navigation state, preserving position
	visibleSlots := d.calculateVisibleItemSlots()
	d.itemListNav = NewScrollableListState(len(d.availableItems), visibleSlots)

	// Restore position (clamped to valid range)
	if savedIndex > 0 && len(d.availableItems) > 0 {
		// Clamp to valid range
		if savedIndex >= len(d.availableItems) {
			savedIndex = len(d.availableItems) - 1
		}
		// Move to saved position
		for range savedIndex {
			d.itemListNav.MoveDown()
		}
	}
}

// calculateVisibleItemSlots calculates how many items can fit in the items panel
func (d *ReadyEquipmentDialog) calculateVisibleItemSlots() int {
	itemsRect := d.itemsBorder.GetInteriorBounds()
	headerHeight := int(d.sectionOutput.Font.TextFace.Size) + itemDialogTextPadding*readyCenterDivisor
	lineHeight := d.itemRowHeight()

	availableHeight := itemsRect.Dy() - headerHeight

	return availableHeight / lineHeight
}

// getSelectedCharacter returns the currently selected party member
func (d *ReadyEquipmentDialog) getSelectedCharacter() *party_state.PlayerCharacter {
	return d.partyState.GetCharacter(d.selectedCharIdx)
}

// getSelectedItem returns the currently selected equipment from available list
func (d *ReadyEquipmentDialog) getSelectedItem() *availableEquipment {
	if len(d.availableItems) == 0 {
		return nil
	}
	idx := d.itemListNav.GetCurrentIndex()
	if idx < 0 || idx >= len(d.availableItems) {
		return nil
	}

	return &d.availableItems[idx]
}

// Update handles per-frame updates including input
func (d *ReadyEquipmentDialog) Update() {
	d.ensureLayout()

	if !d.isVisible || d.isDismissed {
		return
	}
	if d.confirmation != nil {
		d.updateConfirmation()

		return
	}
	if d.waitingForResult {
		return
	}

	// Handle ESC to dismiss
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.dismiss()

		return
	}

	if d.handleCharacterSelectionInput() {
		return
	}

	if d.handleItemNavigationInput() {
		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyBracketLeft) {
		d.cycleDetailPage(-1)

		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeySpace) || d.keyboard.IsKeyPressedAndRegister(ebiten.KeyBracketRight) {
		d.cycleDetailPage(1)

		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyH) {
		d.switchComparisonHand()

		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyB) {
		d.showStrengthBreakdown()

		return
	}

	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) || d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter) {
		d.toggleEquipSelected()

		return
	}
}

func (d *ReadyEquipmentDialog) handleCharacterSelectionInput() bool {
	if d.request.LockCharacter {
		return false
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyTab) {
		delta := 1
		if d.keyboard.IsModifierPressed(input.ModifierShift) {
			delta = -1
		}
		d.cycleCharacter(delta)

		return true
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyLeft) {
		d.cycleCharacter(-1)

		return true
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyRight) {
		d.cycleCharacter(1)

		return true
	}
	for index, key := range []ebiten.Key{
		ebiten.Key1, ebiten.Key2, ebiten.Key3, ebiten.Key4, ebiten.Key5, ebiten.Key6,
	} {
		if d.keyboard.IsKeyPressedAndRegister(key) {
			d.selectCharacter(index)

			return true
		}
	}

	return false
}

func (d *ReadyEquipmentDialog) handleItemNavigationInput() bool {
	key, owned := d.Poll(d.keyboard)
	if owned && key != nil {
		previous := d.itemListNav.GetCurrentIndex()
		navigateScrollableList(d.itemListNav, *key)
		if previous != d.itemListNav.GetCurrentIndex() {
			d.detailPage = 0
			profiling.Count(profiling.ReadySelections)
			d.selectionDrawStarted = profiling.Begin()
		}
		d.warningMessage = ""
	}

	return owned
}

// cycleCharacter cycles through party members
func (d *ReadyEquipmentDialog) cycleCharacter(delta int) {
	const maxParty = 6
	for i := 1; i <= maxParty; i++ {
		nextIdx := (d.selectedCharIdx + delta*i + maxParty) % maxParty
		if !d.partyState.HasCharacter(nextIdx) {
			continue
		}
		char := d.partyState.GetCharacter(nextIdx)
		if char != nil && char.IsAlive() {
			d.selectCharacter(nextIdx)

			return
		}
	}
}

// selectCharacter selects a specific party member by index
func (d *ReadyEquipmentDialog) selectCharacter(idx int) {
	if idx < 0 || idx >= 6 || !d.partyState.HasCharacter(idx) {
		return
	}
	char := d.partyState.GetCharacter(idx)
	if char == nil || !char.IsAlive() {
		return
	}
	d.selectedCharIdx = idx
	d.targetHand = references.SlotNone
	d.review = nil
	d.updateAvailableItems()
	d.warningMessage = ""
}

// toggleEquipSelected equips or unequips the selected item
func (d *ReadyEquipmentDialog) toggleEquipSelected() {
	item := d.getSelectedItem()
	if item == nil {
		return
	}

	char := d.getSelectedCharacter()
	if char == nil {
		return
	}

	transaction := d.transactionFor(item.Equipment)
	preview, err := d.gameState.PreviewReadyEquipment(transaction)
	if err != nil {
		d.warningMessage = err.Error()
		d.updateAvailableItems()

		return
	}
	transaction.Confirmed = &preview
	if preview.RequiresConfirmation() {
		d.review = &transaction
		d.showReplacementConfirmation(transaction)

		return
	}
	d.submitTransaction(transaction)
}

// Draw renders the ready equipment dialog
func (d *ReadyEquipmentDialog) Draw(screen *ebiten.Image) {
	if !d.isVisible {
		return
	}
	d.ensureLayout()
	d.drawCachedParent(screen)
	profiling.EndReadySelection(d.selectionDrawStarted)
	d.selectionDrawStarted = time.Time{}
	if d.confirmation != nil {
		d.confirmation.Draw(screen)
	}
}

func (d *ReadyEquipmentDialog) drawParentContent(screen *ebiten.Image) {
	pixelUnit := getClassicUIPixelUnit(d.lastScreenWidth, d.lastScreenHeight)
	fillClassicPaneBackgrounds(screen, d.frameRects, pixelUnit, color.DialogBackground.RGBA())

	// Draw content panels
	d.drawCharacterPanel(screen)
	d.drawSlotsPanel(screen)
	d.drawItemsPanel(screen)
	d.drawDetailPanel(screen)
	d.drawStatusBar(screen)

	drawClassicPaneFrames(screen, d.frameRects, pixelUnit, DialogRuleColor)
}

func (d *ReadyEquipmentDialog) ensureLayout() {
	screenWidth, screenHeight := display.GetManager().GetCurrentSize()
	if screenWidth <= 0 || screenHeight <= 0 {
		return
	}
	if d.outerBorder != nil && screenWidth == d.lastScreenWidth && screenHeight == d.lastScreenHeight {
		return
	}

	selectedItemIndex := 0
	if d.itemListNav != nil {
		selectedItemIndex = d.itemListNav.GetCurrentIndex()
	}
	d.lastScreenWidth = screenWidth
	d.lastScreenHeight = screenHeight
	d.releaseContent()
	d.initializeTypography()
	d.createBorders()
	d.initializeOutputs()
	if d.itemListNav != nil {
		d.itemListNav = NewScrollableListState(len(d.availableItems), d.calculateVisibleItemSlots())
		d.itemListNav.SetIndex(selectedItemIndex)
	}
}

// drawPanelHeader draws a header title at the top of a panel
func (d *ReadyEquipmentDialog) drawPanelHeader(screen *ebiten.Image, border *Border, headerText string) {
	headerBounds := border.GetInteriorBounds()
	leftPadding := itemDialogTextPadding + itemDialogMinimumPadding
	headerY := headerBounds.Min.Y + itemDialogTextPadding
	contentWidth := headerBounds.Dx() - leftPadding*readyCenterDivisor

	d.sectionOutput.Reset()
	d.sectionOutput.AppendLine().Text(headerText, DialogHeaderColor).Done()
	d.sectionOutput.DrawPanelContent(
		screen,
		image.Pt(headerBounds.Min.X+leftPadding, headerY),
		contentWidth,
		etext.AlignStart,
	)
}

// drawSlotsPanel draws the equipment slots panel (Pane 2)
//
//nolint:funlen // Render the six paired slot rows and bottom-anchored ammunition in their shared pane coordinates.
func (d *ReadyEquipmentDialog) drawSlotsPanel(screen *ebiten.Image) {
	d.drawPanelHeader(screen, d.slotsBorder, "EQUIPPED")

	bounds := d.slotsBorder.GetInteriorBounds()
	headerFontSize := d.sectionOutput.Font.TextFace.Size
	leftPadding := itemDialogTextPadding + readyDetailInset
	fontSize := d.font.TextFace.Size
	lineHeight := int(fontSize * readyLineSpacingMultiplier)

	contentStartY := bounds.Min.Y + itemDialogTextPadding + int(headerFontSize) + itemDialogTextPadding
	textLeft := bounds.Min.X + leftPadding

	char := d.getSelectedCharacter()
	if char == nil {
		return
	}

	// The persisted Weapon/Shield field names are legacy; Ready presents them as generic hands.
	slots := []struct {
		slot  references.EquipmentSlot
		label string
	}{
		{slot: references.SlotHelm, label: "Helm"},
		{slot: references.SlotArmour, label: "Armour"},
		{slot: references.SlotWeapon, label: references.SlotWeapon.DisplayName()},
		{slot: references.SlotShield, label: references.SlotShield.DisplayName()},
		{slot: references.SlotRing, label: "Ring"},
		{slot: references.SlotAmulet, label: "Amulet"},
	}

	currentY := contentStartY
	for _, entry := range slots {
		availability, itemName := readySlotText(char, entry.slot)

		// Line 1: Slot name
		d.slotsOutput.Reset()
		d.slotsOutput.AppendLine().Text(entry.label+": ", DialogLabelColor).
			Text(availability, DialogValueColor).Done()
		d.slotsOutput.DrawPanelContent(
			screen,
			image.Pt(textLeft, currentY),
			bounds.Dx()-leftPadding*readyCenterDivisor,
			etext.AlignStart,
		)
		currentY += lineHeight

		// Line 2: Occupying item, including the full two-handed reservation label.
		d.slotsOutput.Reset()
		d.slotsOutput.SetMaxCharsPerLine(max(1, d.font.CharsForWidth(bounds.Dx()-leftPadding*readyCenterDivisor)))
		d.slotsOutput.AppendLine().Text(itemName, DialogValueColor).Done()
		d.slotsOutput.DrawPanelContent(
			screen,
			image.Pt(textLeft, currentY),
			bounds.Dx()-leftPadding*readyCenterDivisor,
			etext.AlignStart,
		)
		currentY += lineHeight
	}

	// Add ammunition counts at very bottom of panel, just above border
	arrowQty := d.inventory.Equipment.Get(references.Arrows)
	quarrelQty := d.inventory.Equipment.Get(references.Quarrels)

	// Calculate Y positions from bottom of panel
	// Leave small padding from bottom border
	bottomPadding := itemDialogTextPadding
	quarrelsY := bounds.Max.Y - bottomPadding - lineHeight
	arrowsY := quarrelsY - lineHeight

	// Arrows
	d.slotsOutput.Reset()
	d.slotsOutput.AppendLine().Text("Arrows: ", DialogLabelColor).Textf("%d", DialogValueColor, arrowQty).Done()
	d.slotsOutput.DrawPanelContent(
		screen,
		image.Pt(textLeft, arrowsY),
		bounds.Dx()-leftPadding*readyCenterDivisor,
		etext.AlignStart,
	)

	// Quarrels
	d.slotsOutput.Reset()
	d.slotsOutput.AppendLine().Text("Quarrels: ", DialogLabelColor).Textf("%d", DialogValueColor, quarrelQty).Done()
	d.slotsOutput.DrawPanelContent(
		screen,
		image.Pt(textLeft, quarrelsY),
		bounds.Dx()-leftPadding*readyCenterDivisor,
		etext.AlignStart,
	)
}

// drawItemsPanel draws the available items panel (Pane 3)
func (d *ReadyEquipmentDialog) drawItemsPanel(screen *ebiten.Image) {
	d.drawPanelHeader(screen, d.itemsBorder, "AVAILABLE")

	bounds := d.itemsBorder.GetInteriorBounds()
	headerFontSize := d.sectionOutput.Font.TextFace.Size
	leftPadding := itemDialogTextPadding + readyDetailInset
	fontSize := d.font.TextFace.Size
	lineHeight := d.itemRowHeight()

	contentStartY := bounds.Min.Y + itemDialogTextPadding + int(headerFontSize) + itemDialogTextPadding
	textLeft := bounds.Min.X + leftPadding

	// Handle empty list
	if len(d.availableItems) == 0 {
		d.itemsOutput.Reset()
		d.itemsOutput.AppendLine().Text("No equipment", DialogEmptyColor).Done()
		d.itemsOutput.DrawPanelContent(
			screen,
			image.Pt(textLeft, contentStartY),
			bounds.Dx()-leftPadding*readyCenterDivisor,
			etext.AlignStart,
		)

		return
	}

	// Get visible item range
	startIdx, endIdx := d.itemListNav.GetVisibleRange()
	cursorPos := d.itemListNav.GetCursorScreenPosition()

	for i := startIdx; i < endIdx; i++ {
		screenRow := i - startIdx
		rowTop := contentStartY + screenRow*lineHeight
		d.drawItemRow(screen, bounds, rowTop, d.availableItems[i], screenRow == cursorPos)
	}

	// Centre the shared cues on the highlighted text rows inside the item gutter.
	list := image.Rect(bounds.Min.X, contentStartY-(lineHeight-int(fontSize))/readyCenterDivisor,
		bounds.Max.X-itemDialogTextPadding-readyDetailInset, bounds.Max.Y)
	drawMasterDetailScrollIndicators(screen, list, startIdx, endIdx, len(d.availableItems), lineHeight,
		d.itemScrollGutter(), d.font, d.itemsOutput)
}

// drawItemRow draws one available item at rowTop, highlighted when selected and
// clipped to its text row.
func (d *ReadyEquipmentDialog) drawItemRow(
	screen *ebiten.Image, bounds image.Rectangle, rowTop int, item availableEquipment, isSelected bool,
) {
	// Draw selection highlight
	if isSelected {
		fontSize := d.font.TextFace.Size
		highlightPadding := float32(fontSize * readyHighlightPaddingRatio)
		vector.FillRect(screen,
			float32(bounds.Min.X),
			float32(rowTop)-highlightPadding,
			float32(bounds.Dx()),
			float32(fontSize)+highlightPadding*readyCenterDivisor,
			color.EGAWhite,
			false)
	}

	row := d.itemTextRect(bounds, rowTop)
	d.populateItemRow(item, isSelected, row.Dx())
	clipBounds := row.Intersect(screen.Bounds())
	if !clipBounds.Empty() {
		clipped, ok := screen.SubImage(clipBounds).(*ebiten.Image)
		if !ok {
			panic("Ready item row clip is not an image")
		}
		d.itemsOutput.DrawPanelContent(clipped, row.Min, row.Dx(), etext.AlignStart)
	}
}

// drawDetailPanel draws the item details panel (Pane 4 - right)
func (d *ReadyEquipmentDialog) drawDetailPanel(screen *ebiten.Image) {
	d.prepareDetailPage()
	content := d.detailContentRect()
	capacity := max(1, d.detailOutput.GetLineCapacity(content.Dy()))
	d.drawPanelHeader(screen, d.detailBorder, "DETAILS")
	if pages := d.detailPageCount(); pages > 1 {
		bounds := d.detailBorder.GetInteriorBounds().Inset(itemDialogTextPadding + readyDetailInset)
		d.indicatorOutput.DrawPanelContent(screen, bounds.Min, bounds.Dx(), etext.AlignEnd)
	}
	d.detailOutput.DrawPanelContentRange(screen, content.Min, content.Dx(),
		etext.AlignStart, d.detailPage*capacity, capacity)
}

func (d *ReadyEquipmentDialog) prepareDetailPage() {
	d.populateDetailOutput()
	pages := d.detailPageCount()
	d.detailPage = min(d.detailPage, pages-1)
	d.indicatorOutput.Reset()
	if pages > 1 {
		d.indicatorOutput.AppendLine().Textf("%d/%d", DialogValueColor, d.detailPage+1, pages).Done()
	}
}

func (d *ReadyEquipmentDialog) cycleDetailPage(delta int) {
	d.populateDetailOutput()
	pages := d.detailPageCount()
	d.detailPage = (d.detailPage + delta + pages) % pages
}

func (d *ReadyEquipmentDialog) detailContentRect() image.Rectangle {
	bounds := d.detailBorder.GetInteriorBounds()
	padding := itemDialogTextPadding + readyDetailInset
	top := bounds.Min.Y + itemDialogTextPadding*readyCenterDivisor + int(d.sectionOutput.GetTotalHeightForLines(1))

	return image.Rect(bounds.Min.X+padding, top, bounds.Max.X-padding, bounds.Max.Y-itemDialogTextPadding)
}

func (d *ReadyEquipmentDialog) detailPageCount() int {
	capacity := max(1, d.detailOutput.GetLineCapacity(d.detailContentRect().Dy()))

	return max(1, (d.detailOutput.GetPanelContentLineCount()+capacity-1)/capacity)
}

func (d *ReadyEquipmentDialog) populateDetailOutput() {
	d.detailOutput.Reset()
	d.detailOutput.SetMaxCharsPerLine(max(1, d.font.CharsForWidth(d.detailContentRect().Dx())))

	item := d.getSelectedItem()
	if item == nil {
		d.detailOutput.AppendLine().Text("No item selected", DialogEmptyColor).Done()

		return
	}

	d.appendEquipmentFacts(item)
	d.appendEquipmentComparison(item)
	d.appendEquipmentWarnings(item)
}

func (d *ReadyEquipmentDialog) appendEquipmentFacts(item *availableEquipment) {
	equip := item.Equipment
	// Item name
	d.detailOutput.AppendLine().Text(equip.String(), DialogHeaderColor).WordWrap().Done()

	d.appendReplacementTargets(item)
	if equip.IsWeapon() {
		d.detailOutput.AppendLine().Text("Range: ", DialogLabelColor).
			Textf("%d tiles", DialogValueColor, equip.GetAttackRange()).Done()
	}
	if ammo := equip.GetRequiredAmmo(); ammo != references.NoEquipment {
		d.detailOutput.AppendLine().Text("Ammo: ", DialogLabelColor).
			Textf("%s (%d)", DialogValueColor, ammo.String(), d.inventory.Equipment.Get(ammo)).WordWrap().Done()
	}
	for _, ability := range equip.GetAbilities() {
		d.detailOutput.AppendLine().Text("Property: ", DialogLabelColor).
			Text(ability.String(), DialogValueColor).WordWrap().Done()
	}
}

func (d *ReadyEquipmentDialog) appendEquipmentComparison(item *availableEquipment) {
	equip := item.Equipment
	if item.Equipped {
		d.appendEquippedRatings(equip)

		return
	}
	preview := item.Preview
	if preview.Selection == game_state.ReadyEquipmentNoChange {
		return
	}
	d.appendComparisonRating("Attack", equip.GetAttackValue(), preview.Resulting.Attack-preview.Current.Attack)
	d.appendComparisonRating("Defense", equip.GetDefenseValue(), preview.Resulting.Defense-preview.Current.Defense)
	d.appendStrengthBudget(item)
}

func (d *ReadyEquipmentDialog) appendEquipmentWarnings(item *availableEquipment) {
	if item.CanEquip || item.EquipError == "" || item.StrengthBlocked {
		return
	}
	label := "Cannot equip: "
	if item.Equipped {
		label = "Cannot unequip: "
	}
	// The authority's refusal is the only failure explanation. Do not separately
	// reconstruct Strength/ammunition errors in the comparison or footer.
	d.detailOutput.AppendLine().Done()
	d.detailOutput.AppendLine().Text(label, DialogLabelColor).
		Text(item.EquipError, DialogWarningColor).WordWrap().Done()
}

const (
	readyDetailLines           = 128
	readyDetailInset           = 6
	readyIndicatorChars        = 24
	readyStatusLines           = 4
	readyCenterDivisor         = 2
	readyInitialChars          = 25
	readyInitialSlotChars      = 30
	readyInitialItemChars      = 50
	readyCharacterLines        = 12
	readyLineSpacingMultiplier = 1.2
	readyHighlightPaddingRatio = 0.1
)

// drawStatusBar draws the status bar (bottom)
func (d *ReadyEquipmentDialog) drawStatusBar(screen *ebiten.Image) {
	bounds := d.statusBorder.GetInteriorBounds()
	leftPadding := itemDialogTextPadding + itemDialogMinimumPadding

	textLeft := bounds.Min.X + leftPadding
	contentWidth := bounds.Dx() - leftPadding*readyCenterDivisor
	d.populateStatusOutput(contentWidth)
	totalTextHeight := d.statusOutput.GetTotalHeightForLines(d.statusOutput.GetPanelContentLineCount())
	startY := bounds.Min.Y + max(0, (bounds.Dy()-int(totalTextHeight))/readyCenterDivisor)

	d.statusOutput.DrawPanelContent(
		screen,
		image.Pt(textLeft, startY),
		contentWidth,
		etext.AlignStart,
	)
}

func (d *ReadyEquipmentDialog) populateStatusOutput(contentWidth int) {
	d.statusOutput.Reset()

	status, action := d.selectionGuidance()
	statusColor := DialogStatusOKColor
	if action == readySelectionUnavailable {
		statusColor = DialogStatusWarningColor
	}
	// First line: Status message or warning
	item := d.getSelectedItem()
	if d.warningMessage != "" && (item == nil || d.warningMessage != item.EquipError) {
		d.statusOutput.AppendLine().Text(d.warningMessage, DialogStatusWarningColor).Done()
	} else {
		d.statusOutput.AppendLine().Text(status, statusColor).Done()
	}

	d.appendStatusControls(action, contentWidth)
}

// Space pages details; [ and ] remain unadvertised paging aliases.
func (d *ReadyEquipmentDialog) appendStatusControls(action string, width int) {
	var hints []ControlHint
	if !d.request.LockCharacter {
		hints = append(hints, ControlHint{"Tab", "Next character"}, ControlHint{"Shift+Tab", "Previous"})
	}
	hints = append(hints, ControlHint{hintKeyUpDown, hintActionItem}, ControlHint{hintKeyEnter, action},
		ControlHint{"H", "Hand"}, ControlHint{"B", "Strength breakdown"})
	if d.detailPageCount() > 1 {
		hints = append(hints, ControlHint{hintKeySpace, "Details"})
	}
	hints = append(hints, ControlHint{hintKeyEsc, hintActionClose})
	PackControlHints(d.statusOutput, hints, width, etext.AlignStart)
}

const (
	readySelectionUnavailable = "Unavailable"
)

func (d *ReadyEquipmentDialog) selectionGuidance() (string, string) {
	index := d.itemListNav.GetCurrentIndex()
	if index < 0 || index >= len(d.availableItems) {
		return "No equipment selected", readySelectionUnavailable
	}
	item := d.availableItems[index]
	if !item.CanEquip {
		return "Unavailable - see details", readySelectionUnavailable
	}
	switch item.Preview.Selection {
	case game_state.ReadyEquipmentEquip:
		if item.Preview.RequiresConfirmation() {
			return "Review removed items before replacing", "Review replacement"
		}

		return "Ready to equip", "Equip"
	case game_state.ReadyEquipmentUnready:
		return "Ready to unready", "Unready"
	case game_state.ReadyEquipmentNoChange:
		return "No equipment change", "No change"
	}
	panic("unknown Ready preview selection")
}

// formatDelta formats a stat delta with sign
func (d *ReadyEquipmentDialog) formatDelta(delta int) string {
	if delta > 0 {
		return fmt.Sprintf("(+%d)", delta)
	} else if delta < 0 {
		return fmt.Sprintf("(%d)", delta)
	}

	return "(0)"
}

// deltaColor returns the appropriate color for a stat delta
func (d *ReadyEquipmentDialog) deltaColor(delta int) stdcolor.Color {
	if delta > 0 {
		return DialogDeltaPositive
	} else if delta < 0 {
		return DialogDeltaNegative
	}

	return DialogDeltaNeutral
}

// dismiss closes the dialog
func (d *ReadyEquipmentDialog) dismiss() {
	d.releaseContent()
	d.confirmation, d.review, d.pending = nil, nil, nil
	d.isDismissed = true
	d.isVisible = false
}

// Dismiss closes the dialog (exported)
func (d *ReadyEquipmentDialog) Dismiss() {
	d.dismiss()
}

// IsDismissed returns true if the dialog has been dismissed
func (d *ReadyEquipmentDialog) IsDismissed() bool {
	return d.isDismissed
}
