package widgets

import (
	"fmt"
	"image"
	stdcolor "image/color"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	pkgcolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	zStatsDossierTabCount              = 8
	zStatsDossierDefaultMarginX        = 0.05
	zStatsDossierDefaultMarginY        = 0.10
	zStatsDossierCharacterRatio        = 0.22
	zStatsDossierCharacterListRatio    = 0.30
	zStatsDossierListRatio             = spellBrowserListRatio
	zStatsDossierHeaderRatio           = 0.14
	zStatsDossierFooterRatio           = 0.10
	zStatsDossierCharacterStatsRatio   = 0.58
	zStatsDossierMinimumPadding        = 4
	zStatsDossierPaddingDivisor        = 64
	zStatsDossierMinimumRows           = 1
	zStatsDossierDetailMaxLines        = 1024
	zStatsDossierLineSpacingMultiplier = 1.20
	zStatsDossierTitleFontSize         = FontSizeTitle
	zStatsDossierTabRows               = 2
	zStatsDossierTabColumns            = 4
	zStatsDossierPercentMultiplier     = 100
	zStatsDossierBorderWidth           = BorderWidthStandard
	zStatsDossierCenterDivisor         = 2
	zStatsDossierFooterMaxLines        = 3
	zStatsDossierCharacterSelectors    = 6
	zStatsDossierPortraitLines         = 4
	zStatsDossierPortraitInsetUnits    = 2
	zStatsDossierMinimumTextChars      = 8
	zStatsDossierPaneInsetMultiplier   = 2
	zStatsDossierPortraitWidthDivisor  = 3
	zStatsDecimalBase                  = 10
	zStatsNoneLabel                    = "None"
	zStatsFooterControlWidthDivisor    = 5
	zStatsMoonstoneHeaderLines         = 2
)

var _ Widget = (*ZStatsDossierDialog)(nil)

// ZStatsDossierTab identifies one of the eight Z-Stats views.
type ZStatsDossierTab int

const (
	// ZStatsDossierCharacterTab displays the selected character's complete record.
	ZStatsDossierCharacterTab ZStatsDossierTab = iota
	// ZStatsDossierProvisionsTab displays canonical party supplies.
	ZStatsDossierProvisionsTab
	// ZStatsDossierArmoryTab displays every equipment identity.
	ZStatsDossierArmoryTab
	// ZStatsDossierSpellsTab displays the visible spellbook.
	ZStatsDossierSpellsTab
	// ZStatsDossierReagentsTab displays reagent inventory.
	ZStatsDossierReagentsTab
	// ZStatsDossierPotionsTab displays potion inventory.
	ZStatsDossierPotionsTab
	// ZStatsDossierScrollsTab displays scroll inventory.
	ZStatsDossierScrollsTab
	// ZStatsDossierItemsTab displays owned special possessions and moonstones.
	ZStatsDossierItemsTab
)

// ZStatsDossierDialogConfig configures the detached Z-Stats dossier widget.
type ZStatsDossierDialogConfig struct {
	Keyboard    *input.Keyboard
	SpriteSheet *sprites.SpriteSheet
	Placement   sprites.PercentBasedPlacement
	OnDismiss   func()
}

type zStatsDossierLayout struct {
	panelRect              image.Rectangle
	headerRect             image.Rectangle
	titleRect              image.Rectangle
	tabsRect               image.Rectangle
	bodyRect               image.Rectangle
	spellSearchRect        image.Rectangle
	footerRect             image.Rectangle
	characterPortraitRect  image.Rectangle
	characterListRect      image.Rectangle
	characterStatsRect     image.Rectangle
	characterItemRect      image.Rectangle
	globalListRect         image.Rectangle
	globalDetailRect       image.Rectangle
	tabRects               [zStatsDossierTabCount]image.Rectangle
	characterSelectorRects [zStatsDossierCharacterSelectors]image.Rectangle
	pageLabelRect          image.Rectangle
	padding                int
}

// ZStatsDossierDialog presents a read-only, presentation-owned Z-Stats projection.
type ZStatsDossierDialog struct {
	navigation
	dossier          game_state.ZStatsDossier
	keyboard         *input.Keyboard
	spriteSheet      *sprites.SpriteSheet
	placement        sprites.PercentBasedPlacement
	defaultPlacement bool
	contentPadding   int
	onDismiss        func()
	itemIndices      []int

	activeTab         ZStatsDossierTab
	selectedCharacter int
	tabSelections     [zStatsDossierTabCount]int
	detailPage        int
	detailPages       int
	dismissed         bool
	spellBrowser      *spellBrowser
	spellView         spellPresentation
	pendingMixSpell   *references.Spell
	mixInProgress     bool

	outerBorder   *Border
	bodyFont      *text.UltimaFont
	titleFont     *text.UltimaFont
	titleOutput   *text.Output
	tabOutput     *text.Output
	rowOutput     *text.Output
	rowIconOutput *text.Output
	phaseOutput   *text.Output
	summaryOutput *text.Output
	detailOutput  *text.Output
	footerOutput  *text.Output
	drawOp        *ebiten.DrawImageOptions
	rowWidth      int
	rowIconWidth  int
	layout        zStatsDossierLayout

	lastScreenWidth  int
	lastScreenHeight int
}

// NewZStatsDossierDialog creates a wide modal over an entirely detached dossier.
func NewZStatsDossierDialog(
	dossier game_state.ZStatsDossier,
	config ZStatsDossierDialogConfig,
) *ZStatsDossierDialog {
	if config.Keyboard == nil {
		panic("ZStatsDossierDialog: keyboard is required")
	}
	if config.SpriteSheet == nil {
		panic("ZStatsDossierDialog: SpriteSheet is required")
	}
	if config.OnDismiss == nil {
		panic("ZStatsDossierDialog: OnDismiss is required")
	}
	placement := config.Placement
	if zStatsDossierPlacementIsZero(placement) {
		placement = zStatsDossierDefaultPlacement()
	}
	if placement.EndPercentX <= placement.StartPercentX || placement.EndPercentY <= placement.StartPercentY {
		panic("ZStatsDossierDialog: placement must have positive width and height")
	}

	//nolint:exhaustruct_v5 // Remaining fields are presentation state initialized by ensureLayout or their zero values.
	dialog := &ZStatsDossierDialog{
		navigation:        newNavigation(config.Keyboard.IsKeyHeld),
		dossier:           cloneZStatsDossier(dossier),
		keyboard:          config.Keyboard,
		spriteSheet:       config.SpriteSheet,
		placement:         placement,
		defaultPlacement:  zStatsDossierPlacementIsZero(config.Placement),
		onDismiss:         config.OnDismiss,
		itemIndices:       zStatsOwnedItemIndices(dossier.Items),
		activeTab:         ZStatsDossierCharacterTab,
		selectedCharacter: initialZStatsCharacterPosition(dossier),
		drawOp:            new(ebiten.DrawImageOptions),
		detailPages:       1,
	}
	dialog.updateFilteredSpells()
	dialog.clampAllSelections()
	dialog.ensureLayout()

	return dialog
}

// Update processes one keyboard action while the dossier owns input focus.
func (d *ZStatsDossierDialog) Update() {
	if d.dismissed || d.mixInProgress {
		return
	}
	d.ensureLayout()
	if d.handleSpellBrowserShortcuts() {
		return
	}

	if d.handleDismissShortcut() || d.handleTabShortcut() || d.handleCharacterShortcut() {
		return
	}
	if d.handleSpellFilterShortcut() || d.handleDetailPageShortcut() || d.handleListNavigationShortcut() {
		return
	}
	d.keyboard.IsKeyPressedAndRegister(ebiten.KeyZ)
	d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter)
	d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter)
}

func (d *ZStatsDossierDialog) onDialogFocusChanged(focused bool) {
	d.navigation.onDialogFocusChanged(focused)
	d.spellBrowser.navigation.BlockUntilRelease(d.keyboard.IsKeyHeld)
}

func (d *ZStatsDossierDialog) handleSpellBrowserShortcuts() bool {
	if d.activeTab != ZStatsDossierSpellsTab {
		return false
	}
	if d.pendingMixSpell != nil {
		return true
	}
	if d.keyboard.IsModifierPressed(input.ModifierCtrl) && d.keyboard.IsTextKeyPressedAndRegister(ebiten.KeyM) {
		if selected := d.selectedSpell(); selected != nil {
			spell := references.Spell(selected.ID)
			d.pendingMixSpell = &spell
		}

		return true
	}
	if d.spellBrowser.searching && d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.spellBrowser.leaveSearch()
		d.tabSelections[ZStatsDossierSpellsTab] = d.spellBrowser.selected
		d.detailPage, d.detailPages = 0, 1

		return true
	}

	return false
}

func (d *ZStatsDossierDialog) handleDismissShortcut() bool {
	if !d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		return false
	}
	d.Dismiss()

	return true
}

func (d *ZStatsDossierDialog) handleTabShortcut() bool {
	if !d.keyboard.IsKeyPressedAndRegister(ebiten.KeyTab) {
		return false
	}
	delta := ZStatsDossierTab(1)
	if d.keyboard.IsModifierPressed(input.ModifierShift) {
		delta = -1
	}
	d.switchTab((d.activeTab + delta + zStatsDossierTabCount) % zStatsDossierTabCount)

	return true
}

func (d *ZStatsDossierDialog) handleCharacterShortcut() bool {
	if d.activeTab != ZStatsDossierCharacterTab {
		return false
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyLeft) {
		d.cycleCharacter(-1)

		return true
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyRight) {
		d.cycleCharacter(1)

		return true
	}
	keys := [...]ebiten.Key{
		ebiten.Key1, ebiten.Key2, ebiten.Key3,
		ebiten.Key4, ebiten.Key5, ebiten.Key6,
	}
	for position, key := range keys {
		if !d.keyboard.IsKeyPressedAndRegister(key) {
			continue
		}
		if position < len(d.dossier.Characters) {
			d.selectCharacterPosition(position)
		}

		return true
	}

	return false
}

func (d *ZStatsDossierDialog) handleSpellFilterShortcut() bool {
	if d.activeTab != ZStatsDossierSpellsTab {
		return false
	}
	b := d.spellBrowser
	b.selected, b.page, b.pages = d.tabSelections[ZStatsDossierSpellsTab], d.detailPage, d.detailPages
	b.visible = d.visibleListRows(d.layout.globalListRect)
	handled := b.handleInput(d.keyboard, d.bodyFont)
	d.tabSelections[ZStatsDossierSpellsTab], d.detailPage, d.detailPages = b.selected, b.page, b.pages

	return handled
}

func (d *ZStatsDossierDialog) handleListNavigationShortcut() bool {
	key, owned := d.Poll(d.keyboard)
	if key != nil {
		d.moveCurrentSelection(listNavigationDelta(*key, d.pageStep()))
	}

	return owned
}

func (d *ZStatsDossierDialog) handleDetailPageShortcut() bool {
	if !d.keyboard.IsKeyPressedAndRegister(ebiten.KeySpace) {
		return false
	}
	pageCount := d.detailPageCount()
	if pageCount > 1 {
		d.detailPage = (d.detailPage + 1) % pageCount
	}

	return true
}

func (d *ZStatsDossierDialog) resetDetailPage() {
	d.detailPage = 0
	d.detailPages = 1
}

func (d *ZStatsDossierDialog) detailPageCount() int {
	return max(1, d.detailPages)
}

// Draw renders the dimmed surround, opaque dossier, and active section.
func (d *ZStatsDossierDialog) Draw(screen *ebiten.Image) {
	if screen == nil || d.dismissed {
		return
	}
	d.ensureLayout()

	bounds := screen.Bounds()
	vector.FillRect(
		screen,
		float32(bounds.Min.X),
		float32(bounds.Min.Y),
		float32(bounds.Dx()),
		float32(bounds.Dy()),
		pkgcolor.UIBackgroundSemi.RGBA(),
		false,
	)
	for _, rect := range d.frameRects() {
		FillClassicPixelRoundedRect(screen, rect, d.pixelUnit(), pkgcolor.DialogBackground.RGBA())
	}
	d.detailPages = 1
	d.drawHeader(screen)
	if d.activeTab == ZStatsDossierSpellsTab {
		d.drawSpellFilter(screen, d.layout.spellSearchRect.Inset(d.layout.padding))
	}
	switch d.activeTab {
	case ZStatsDossierCharacterTab:
		d.drawCharacterSection(screen)
	case ZStatsDossierProvisionsTab,
		ZStatsDossierArmoryTab,
		ZStatsDossierSpellsTab,
		ZStatsDossierReagentsTab,
		ZStatsDossierPotionsTab,
		ZStatsDossierScrollsTab,
		ZStatsDossierItemsTab:
		d.drawListPane(screen, d.layout.globalListRect)
		d.drawCatalogDetailPane(screen, d.layout.globalDetailRect)
	}
	d.detailPage = min(d.detailPage, d.detailPageCount()-1)
	d.drawFooter(screen)
	d.drawFrames(screen)
}

// Dismiss closes the dialog and invokes its callback at most once.
func (d *ZStatsDossierDialog) Dismiss() {
	if d.dismissed {
		return
	}
	d.dismissed = true
	d.onDismiss()
}

// IsDismissed reports whether the dossier has been closed.
func (d *ZStatsDossierDialog) IsDismissed() bool { return d.dismissed }

func (d *ZStatsDossierDialog) cycleCharacter(delta int) {
	count := len(d.dossier.Characters)
	if count == 0 {
		return
	}
	d.selectCharacterPosition((d.selectedCharacter + delta + count) % count)
}

func (d *ZStatsDossierDialog) selectCharacterPosition(position int) {
	if position < 0 || position >= len(d.dossier.Characters) {
		return
	}
	if d.selectedCharacter != position {
		d.resetDetailPage()
	}
	d.selectedCharacter = position
	character := d.selectedCharacterModel()
	if character == nil || len(character.Equipped) == 0 {
		d.tabSelections[ZStatsDossierCharacterTab] = 0

		return
	}
	d.tabSelections[ZStatsDossierCharacterTab] = min(
		d.tabSelections[ZStatsDossierCharacterTab],
		len(character.Equipped)-1,
	)
}

func (d *ZStatsDossierDialog) switchTab(tab ZStatsDossierTab) {
	d.Reset()
	d.spellBrowser.navigation.Reset()
	if tab < ZStatsDossierCharacterTab || tab >= zStatsDossierTabCount {
		return
	}
	d.activeTab = tab
	d.resetDetailPage()
	d.clampCurrentSelection()
	if d.outerBorder != nil {
		d.calculateGlobalLayout(max(1, d.pixelUnit()))
	}
}

func (d *ZStatsDossierDialog) moveCurrentSelection(delta int) {
	count := d.currentItemCount()
	if count == 0 {
		return
	}
	selection := max(0, min(d.currentSelection()+delta, count-1))
	if selection == d.currentSelection() {
		return
	}
	d.setCurrentSelection(selection)
	d.resetDetailPage()
}

func (d *ZStatsDossierDialog) pageStep() int {
	return max(1, d.visibleListRows(d.activeListRect())/zStatsDossierCenterDivisor)
}

func (d *ZStatsDossierDialog) currentSelection() int {
	return d.tabSelections[d.activeTab]
}

func (d *ZStatsDossierDialog) setCurrentSelection(selection int) {
	d.tabSelections[d.activeTab] = selection
}

func (d *ZStatsDossierDialog) currentItemCount() int {
	switch d.activeTab {
	case ZStatsDossierCharacterTab:
		character := d.selectedCharacterModel()
		if character == nil {
			return 0
		}

		return len(character.Equipped)
	case ZStatsDossierProvisionsTab:
		return len(d.dossier.Provisions)
	case ZStatsDossierArmoryTab:
		return len(d.dossier.Armory)
	case ZStatsDossierSpellsTab:
		return len(d.spellBrowser.filtered)
	case ZStatsDossierReagentsTab:
		return len(d.dossier.Magic.Reagents)
	case ZStatsDossierPotionsTab:
		return len(d.dossier.Magic.Potions)
	case ZStatsDossierScrollsTab:
		return len(d.dossier.Magic.Scrolls)
	case ZStatsDossierItemsTab:
		return len(d.itemIndices)
	default:
		return 0
	}
}

func (d *ZStatsDossierDialog) clampAllSelections() {
	for tab := ZStatsDossierCharacterTab; tab < zStatsDossierTabCount; tab++ {
		d.activeTab = tab
		d.clampCurrentSelection()
	}
	d.activeTab = ZStatsDossierCharacterTab
}

func (d *ZStatsDossierDialog) clampCurrentSelection() {
	count := d.currentItemCount()
	if count == 0 {
		d.setCurrentSelection(0)

		return
	}
	d.setCurrentSelection(max(0, min(d.currentSelection(), count-1)))
}

func (d *ZStatsDossierDialog) updateFilteredSpells() {
	if d.spellBrowser == nil {
		d.spellBrowser = newSpellBrowser(d.dossier.Magic.Spells)
	}
	d.spellBrowser.catalog = d.dossier.Magic.Spells
	d.spellBrowser.filter()
}

// HasPendingMixRequest reports a presentation intent consumed by the scene.
func (d *ZStatsDossierDialog) HasPendingMixRequest() bool { return d.pendingMixSpell != nil }

// ConsumePendingMixSpell suspends input until the scene refreshes the dossier.
func (d *ZStatsDossierDialog) ConsumePendingMixSpell() references.Spell {
	spell := *d.pendingMixSpell
	d.pendingMixSpell = nil
	d.mixInProgress = true

	return spell
}

// RefreshDossier updates inventory and knowledge without losing navigation.
func (d *ZStatsDossierDialog) RefreshDossier(dossier game_state.ZStatsDossier) {
	d.mixInProgress = false
	activeTab := d.activeTab
	var selectedItem game_state.ZStatsOwnedItem
	itemSelection := d.tabSelections[ZStatsDossierItemsTab]
	hadItem := itemSelection >= 0 && itemSelection < len(d.itemIndices)
	if hadItem {
		selectedItem = d.dossier.Items[d.itemIndices[itemSelection]]
	}
	spellID := -1
	if spell := d.selectedSpell(); spell != nil {
		spellID = spell.ID
	}
	d.dossier = cloneZStatsDossier(dossier)
	d.itemIndices = zStatsOwnedItemIndices(d.dossier.Items)
	if hadItem {
		for position, index := range d.itemIndices {
			item := d.dossier.Items[index]
			if item.ID == selectedItem.ID && item.MoonPhase.Present == selectedItem.MoonPhase.Present {
				d.tabSelections[ZStatsDossierItemsTab] = position

				break
			}
		}
	}
	d.updateFilteredSpells()
	d.spellBrowser.selectID(spellID)
	d.tabSelections[ZStatsDossierSpellsTab] = d.spellBrowser.selected
	d.clampAllSelections()
	d.activeTab = activeTab
	d.resetDetailPage()
}

func (d *ZStatsDossierDialog) selectedSpell() *game_state.ZStatsSpell {
	selection := d.tabSelections[ZStatsDossierSpellsTab]
	if selection < 0 || selection >= len(d.spellBrowser.filtered) {
		return nil
	}
	index := d.spellBrowser.filtered[selection]
	if index < 0 || index >= len(d.dossier.Magic.Spells) {
		return nil
	}

	return &d.dossier.Magic.Spells[index]
}

func (d *ZStatsDossierDialog) selectedCharacterModel() *game_state.ZStatsCharacter {
	if d.selectedCharacter < 0 || d.selectedCharacter >= len(d.dossier.Characters) {
		return nil
	}

	return &d.dossier.Characters[d.selectedCharacter]
}

func (d *ZStatsDossierDialog) ensureLayout() {
	screenWidth, screenHeight := display.GetManager().GetCurrentSize()
	if screenWidth <= 0 || screenHeight <= 0 {
		return
	}
	if d.outerBorder != nil && screenWidth == d.lastScreenWidth && screenHeight == d.lastScreenHeight {
		return
	}

	d.resetDetailPage()
	d.lastScreenWidth = screenWidth
	d.lastScreenHeight = screenHeight
	d.spellView.layout(screenWidth, screenHeight)
	d.bodyFont, d.rowOutput = GetScaledFontWithOutput(FontConfig{
		Size:                  FontSizeBody,
		LineSpacing:           0,
		LineSpacingMultiplier: zStatsDossierLineSpacingMultiplier,
		MaxLines:              zStatsDossierDetailMaxLines,
		MaxChars:              zStatsDossierMinimumTextChars,
	})
	d.titleFont, d.titleOutput = GetScaledFontWithOutput(FontConfig{
		Size:                  zStatsDossierTitleFontSize,
		LineSpacing:           0,
		LineSpacingMultiplier: zStatsDossierLineSpacingMultiplier,
		MaxLines:              1,
		MaxChars:              zStatsDossierMinimumTextChars,
	})
	_, d.tabOutput = GetScaledFontWithOutput(FontConfig{
		Size:                  FontSizeBody,
		LineSpacing:           0,
		LineSpacingMultiplier: zStatsDossierLineSpacingMultiplier,
		MaxLines:              1,
		MaxChars:              zStatsDossierMinimumTextChars,
	})
	d.detailOutput = text.NewOutput(
		d.bodyFont,
		d.rowOutput.GetLineSpacing(),
		zStatsDossierDetailMaxLines,
		zStatsDossierMinimumTextChars,
	)
	d.summaryOutput = text.NewOutput(
		d.bodyFont,
		d.rowOutput.GetLineSpacing(),
		zStatsDossierDetailMaxLines,
		zStatsDossierMinimumTextChars,
	)
	d.footerOutput = text.NewOutput(
		d.bodyFont,
		d.rowOutput.GetLineSpacing(),
		zStatsDossierFooterMaxLines,
		zStatsDossierMinimumTextChars,
	)
	d.rowIconOutput = text.NewOutput(d.bodyFont, d.rowOutput.GetLineSpacing(), 1, 1)
	d.phaseOutput = text.NewOutput(d.bodyFont, float64(d.moonstoneIconScale()*text.BitmapCharHeight), 1, 1)
	d.outerBorder = NewOpaqueBorder(d.expandedPlacement(), zStatsDossierBorderWidth, pkgcolor.DialogBackground.RGBA())
	d.calculateLayout()
}

// Pay for both full-size tab rows and the spell strip from the surround. Keep
// baseline padding independent of the enlargement, and honor explicit bounds.
func (d *ZStatsDossierDialog) expandedPlacement() sprites.PercentBasedPlacement {
	placement := d.placement
	bounds := *sprites.GetRectangleFromPercents(placement)
	interiorHeight := bounds.Dy() - zStatsDossierPaneInsetMultiplier*int(float32(bounds.Dy())*LayoutSpacingMedium)
	interiorWidth := bounds.Dx() - zStatsDossierPaneInsetMultiplier*int(float32(bounds.Dx())*LayoutSpacingMedium)
	d.contentPadding = max(zStatsDossierMinimumPadding, min(interiorWidth, interiorHeight)/zStatsDossierPaddingDivisor)
	if !d.defaultPlacement {
		return placement
	}
	// Reserve the complete two-row tabs as additional space. The replaced small
	// tab line pays for frame overlap and rounding, leaving content at least as tall.
	extra := int(math.Ceil(d.tabOutput.GetTotalHeightForLines(zStatsDossierTabRows))) + d.spellStripHeight()
	extra = int(math.Ceil(float64(extra) / (1 - 2*float64(LayoutSpacingMedium))))
	margin := float64(max(d.contentPadding, d.pixelUnit()*zStatsDossierPaneInsetMultiplier)) / float64(d.lastScreenHeight)
	expand := float64(extra) / float64(d.lastScreenHeight) / zStatsDossierCenterDivisor
	placement.StartPercentY = max(margin, placement.StartPercentY-expand)
	placement.EndPercentY = min(1-margin, placement.EndPercentY+expand)

	return placement
}

func (d *ZStatsDossierDialog) spellStripHeight() int {
	textHeight := int(math.Ceil(d.spellView.search.GetTotalHeightForLines(spellBrowserRowLines)))

	return textHeight + d.contentPadding*zStatsDossierPaneInsetMultiplier + d.pixelUnit()
}

func (d *ZStatsDossierDialog) calculateLayout() {
	bounds := d.outerBorder.GetInteriorBounds()
	padding := d.contentPadding
	content := insetZStatsRect(bounds, padding)
	overlap := max(1, d.pixelUnit())
	headerHeight := max(
		int(math.Ceil(d.titleOutput.GetFontHeight()+d.tabOutput.GetTotalHeightForLines(zStatsDossierTabRows)))+padding*3,
		int(float64(content.Dy())*zStatsDossierHeaderRatio),
	)
	footerHeight := max(
		int(math.Ceil(d.footerOutput.GetTotalHeightForLines(zStatsDossierFooterMaxLines)))+padding*2,
		int(float64(content.Dy())*zStatsDossierFooterRatio),
	)

	var reset zStatsDossierLayout
	d.layout = reset
	d.layout.panelRect = bounds
	d.layout.padding = padding
	d.layout.headerRect = image.Rect(content.Min.X, content.Min.Y, content.Max.X, content.Min.Y+headerHeight)
	d.layout.footerRect = image.Rect(content.Min.X, content.Max.Y-footerHeight, content.Max.X, content.Max.Y)
	d.layout.bodyRect = image.Rect(
		content.Min.X,
		d.layout.headerRect.Max.Y-overlap,
		content.Max.X,
		d.layout.footerRect.Min.Y+overlap,
	)
	d.layout.titleRect = image.Rect(
		d.layout.headerRect.Min.X,
		d.layout.headerRect.Min.Y+padding,
		d.layout.headerRect.Max.X,
		d.layout.headerRect.Min.Y+padding+int(math.Ceil(d.titleOutput.GetFontHeight())),
	)
	d.layout.tabsRect = image.Rect(
		d.layout.headerRect.Min.X,
		d.layout.titleRect.Max.Y+padding,
		d.layout.headerRect.Max.X,
		d.layout.headerRect.Max.Y-padding,
	)

	d.calculateCharacterLayout(overlap)
	d.calculateGlobalLayout(overlap)
	d.calculateTabRects()
	d.calculateFooterControls()
	d.configureOutputs()
}

func (d *ZStatsDossierDialog) calculateCharacterLayout(overlap int) {
	body := d.layout.bodyRect
	portraitEnd := body.Min.X + int(math.Round(float64(body.Dx())*zStatsDossierCharacterRatio))
	characterContentRatio := zStatsDossierCharacterRatio + zStatsDossierCharacterListRatio
	listEnd := body.Min.X + int(math.Round(float64(body.Dx())*characterContentRatio))
	d.layout.characterPortraitRect = image.Rect(body.Min.X, body.Min.Y, portraitEnd, body.Max.Y)
	d.layout.characterListRect = image.Rect(portraitEnd-overlap, body.Min.Y, listEnd, body.Max.Y)
	detailRect := image.Rect(listEnd-overlap, body.Min.Y, body.Max.X, body.Max.Y)
	statsEnd := detailRect.Min.Y + int(math.Round(float64(detailRect.Dy())*zStatsDossierCharacterStatsRatio))
	d.layout.characterStatsRect = image.Rect(detailRect.Min.X, detailRect.Min.Y, detailRect.Max.X, statsEnd)
	d.layout.characterItemRect = image.Rect(detailRect.Min.X, statsEnd-overlap, detailRect.Max.X, detailRect.Max.Y)
	selectorArea := d.characterSelectorArea()
	for index := range zStatsDossierCharacterSelectors {
		left := selectorArea.Min.X + index*selectorArea.Dx()/zStatsDossierCharacterSelectors
		right := selectorArea.Min.X + (index+1)*selectorArea.Dx()/zStatsDossierCharacterSelectors
		d.layout.characterSelectorRects[index] = image.Rect(left, selectorArea.Min.Y, right, selectorArea.Max.Y)
	}
}

func (d *ZStatsDossierDialog) calculateGlobalLayout(overlap int) {
	body := d.layout.bodyRect
	d.layout.spellSearchRect = image.Rect(0, 0, 0, 0)
	if d.activeTab == ZStatsDossierSpellsTab {
		d.layout.spellSearchRect = image.Rect(body.Min.X, body.Min.Y, body.Max.X, body.Min.Y+d.spellStripHeight())
		body.Min.Y = d.layout.spellSearchRect.Max.Y - overlap
	}
	split := body.Min.X + int(math.Round(float64(body.Dx())*zStatsDossierListRatio))
	d.layout.globalListRect = image.Rect(body.Min.X, body.Min.Y, split, body.Max.Y)
	d.layout.globalDetailRect = image.Rect(split-overlap, body.Min.Y, body.Max.X, body.Max.Y)
}

func (d *ZStatsDossierDialog) calculateTabRects() {
	area := d.layout.tabsRect
	for index := range zStatsDossierTabCount {
		row, column := index/zStatsDossierTabColumns, index%zStatsDossierTabColumns
		d.layout.tabRects[index] = image.Rect(
			area.Min.X+column*area.Dx()/zStatsDossierTabColumns,
			area.Min.Y+row*area.Dy()/zStatsDossierTabRows,
			area.Min.X+(column+1)*area.Dx()/zStatsDossierTabColumns,
			area.Min.Y+(row+1)*area.Dy()/zStatsDossierTabRows,
		)
	}
}

func (d *ZStatsDossierDialog) calculateFooterControls() {
	content := insetZStatsRect(d.layout.footerRect, d.layout.padding)
	controlWidth := max(1, content.Dx()/zStatsFooterControlWidthDivisor)
	d.layout.pageLabelRect = image.Rect(content.Max.X-controlWidth, content.Min.Y, content.Max.X, content.Max.Y)
}

func (d *ZStatsDossierDialog) configureOutputs() {
	d.titleOutput.SetMaxCharsPerLine(max(1, d.titleFont.CharsForWidth(d.layout.headerRect.Dx())))
	d.footerOutput.SetMaxCharsPerLine(max(
		1,
		d.bodyFont.CharsForWidth(d.layout.footerRect.Dx()-d.layout.padding*2),
	))
}

func (d *ZStatsDossierDialog) drawHeader(screen *ebiten.Image) {
	d.titleOutput.Reset()
	d.titleOutput.AppendLine().WithAlignment(etext.AlignCenter).White("Z-STATS").Done()
	d.titleOutput.DrawPanelContent(screen, d.layout.titleRect.Min, d.layout.titleRect.Dx(), etext.AlignCenter)
	for index, rect := range d.layout.tabRects {
		d.drawTabTitle(screen, ZStatsDossierTab(index), rect)
	}
}

func (d *ZStatsDossierDialog) drawTabTitle(
	screen *ebiten.Image,
	tab ZStatsDossierTab,
	rect image.Rectangle,
) {
	textColor := DialogEmptyColor
	if tab == d.activeTab {
		textColor = pkgcolor.EGAYellow
	}
	prepareSpellOutput(d.tabOutput, rect)
	d.tabOutput.AppendLine().Text(d.tabTitle(tab), textColor).Done()
	top := rect.Min.Y + max(
		0,
		(rect.Dy()-int(math.Ceil(d.tabOutput.GetFontHeight())))/zStatsDossierCenterDivisor,
	)
	d.tabOutput.DrawPanelContent(screen, image.Pt(rect.Min.X, top), rect.Dx(), etext.AlignCenter)
}

func (d *ZStatsDossierDialog) tabTitle(tab ZStatsDossierTab) string {
	switch tab {
	case ZStatsDossierCharacterTab:
		return "Character"
	case ZStatsDossierProvisionsTab:
		return "Supplies"
	case ZStatsDossierArmoryTab:
		return "Armory"
	case ZStatsDossierSpellsTab:
		return "Spells"
	case ZStatsDossierReagentsTab:
		return "Reagents"
	case ZStatsDossierPotionsTab:
		return "Potions"
	case ZStatsDossierScrollsTab:
		return "Scrolls"
	case ZStatsDossierItemsTab:
		return "Items"
	default:
		return ""
	}
}

func (d *ZStatsDossierDialog) drawCharacterSection(screen *ebiten.Image) {
	d.drawCharacterPortrait(screen)
	d.drawListPane(screen, d.layout.characterListRect)
	d.drawCharacterStats(screen)
	d.drawCharacterItemDetail(screen)
}

func (d *ZStatsDossierDialog) drawCharacterPortrait(screen *ebiten.Image) {
	rect := d.layout.characterPortraitRect
	character := d.selectedCharacterModel()
	header := "CHARACTER"
	if character != nil {
		header = character.Name
	}
	d.drawPanelHeader(screen, rect, header)
	body := d.panelBodyRect(rect)
	if character == nil {
		d.drawOutputLine(screen, body, "No active character", DialogEmptyColor, etext.AlignCenter)

		return
	}
	lineHeight := int(math.Ceil(d.rowOutput.GetLineSpacing()))
	portraitArea := image.Rect(
		body.Min.X,
		body.Min.Y,
		body.Max.X,
		max(body.Min.Y, body.Max.Y-lineHeight-d.layout.padding),
	)
	portraitSize := min(
		portraitArea.Dx()-d.layout.padding*zStatsDossierPaneInsetMultiplier,
		portraitArea.Dy()-d.layout.padding*zStatsDossierPaneInsetMultiplier,
	)
	portraitSize = max(1, portraitSize)
	portraitRect := image.Rect(
		portraitArea.Min.X+(portraitArea.Dx()-portraitSize)/zStatsDossierCenterDivisor,
		portraitArea.Min.Y+(portraitArea.Dy()-portraitSize)/zStatsDossierCenterDivisor,
		portraitArea.Min.X+(portraitArea.Dx()+portraitSize)/zStatsDossierCenterDivisor,
		portraitArea.Min.Y+(portraitArea.Dy()+portraitSize)/zStatsDossierCenterDivisor,
	)
	d.drawFramedSprite(screen, portraitRect, character.SpriteIndex)

	for index, selectorRect := range d.layout.characterSelectorRects {
		if index >= len(d.dossier.Characters) {
			continue
		}
		selectorColor := DialogNavHintColor
		if index == d.selectedCharacter {
			selectorColor = pkgcolor.EGAYellow
		}
		d.drawOutputLine(screen, selectorRect, strconv.Itoa(index+1), selectorColor, etext.AlignCenter)
	}
}

func (d *ZStatsDossierDialog) characterSelectorArea() image.Rectangle {
	body := d.panelBodyRect(d.layout.characterPortraitRect)
	lineHeight := max(1, int(math.Ceil(d.rowOutput.GetLineSpacing())))

	return image.Rect(body.Min.X, max(body.Min.Y, body.Max.Y-lineHeight), body.Max.X, body.Max.Y)
}

func (d *ZStatsDossierDialog) drawCharacterStats(screen *ebiten.Image) {
	content := d.prepareCharacterStats()
	d.drawPagedPanelOutput(screen, d.detailOutput, content)
}

func (d *ZStatsDossierDialog) prepareCharacterStats() image.Rectangle {
	rect := d.layout.characterStatsRect
	content := insetZStatsRect(rect, d.layout.padding*zStatsDossierPaneInsetMultiplier)
	d.detailOutput.SetMaxCharsPerLine(max(1, d.bodyFont.CharsForWidth(content.Dx())))
	d.detailOutput.Reset()
	character := d.selectedCharacterModel()
	if character == nil {
		d.appendDetailTextTo(d.detailOutput, "No active character.", DialogEmptyColor)
	} else {
		d.appendCharacterStats(d.detailOutput, character)
	}
	d.preparePanelPage(d.detailOutput, content)

	return content
}

func (d *ZStatsDossierDialog) appendCharacterStats(
	output *text.Output,
	character *game_state.ZStatsCharacter,
) {
	d.appendDetailHeaderTo(output, character.Name)
	d.appendDetailPairTo(
		output,
		"Identity",
		strings.TrimSpace(character.Gender+" "+character.Class),
		DialogValueColor,
	)
	d.appendDetailPairTo(output, "Status", character.Status, zStatsStatusColor(character.Status))
	d.appendDetailPairTo(output, "HP", fmt.Sprintf("%d/%d", character.CurrentHP, character.MaximumHP), DialogValueColor)
	d.appendDetailPairTo(output, "MP", fmt.Sprintf("%d/%d", character.CurrentMP, character.MaximumMP), DialogValueColor)
	d.appendDetailPairTo(output, "STR", strconv.Itoa(character.Strength), DialogValueColor)
	d.appendDetailPairTo(output, "DEX", strconv.Itoa(character.Dexterity), DialogValueColor)
	d.appendDetailPairTo(output, "INT", strconv.Itoa(character.Intelligence), DialogValueColor)
	d.appendDetailPairTo(
		output,
		"Level / XP",
		fmt.Sprintf("%d / %d", character.Level, character.Experience),
		DialogValueColor,
	)
	if character.ReadyToAdvance {
		d.appendDetailTextTo(output, "READY TO ADVANCE", DialogStatusOKColor)
	} else if character.ExperienceForNextLevel > 0 {
		d.appendDetailPairTo(output, "XP progress", fmt.Sprintf(
			"%d/%d (%d%%), %d remaining",
			character.ExperienceIntoLevel,
			character.ExperienceForNextLevel,
			int(math.Round(character.NextLevelProgress*zStatsDossierPercentMultiplier)),
			character.ExperienceToNextLevel,
		), DialogValueColor)
	}
	if !character.CanCast {
		d.appendDetailPairTo(output, "Highest spell", "Cannot cast", DialogValueColor)
	} else if character.HighestUsableSpellCircle != character.Level {
		d.appendDetailPairTo(
			output,
			"Spell circle cap",
			strconv.Itoa(character.HighestUsableSpellCircle),
			DialogValueColor,
		)
	}
	d.appendDetailPairTo(output, "Attack", strconv.Itoa(character.EquipmentAttack), DialogValueColor)
	d.appendDetailPairTo(output, "Defense", strconv.Itoa(character.EquipmentDefense), DialogValueColor)
}

func (d *ZStatsDossierDialog) drawCharacterItemDetail(screen *ebiten.Image) {
	character := d.selectedCharacterModel()
	if character == nil || len(character.Equipped) == 0 {
		d.drawEmptyDetail(screen, d.layout.characterItemRect, "No equipment selected.")

		return
	}
	item := character.Equipped[d.tabSelections[ZStatsDossierCharacterTab]]
	spriteIndex := indexes.NoSprites
	if armory := d.armoryItemByID(item.ItemID); armory != nil {
		spriteIndex = armory.SpriteIndex
	}
	d.drawPreparedCatalogDetail(screen, spriteIndex, d.prepareCharacterItemDetail())
}

func (d *ZStatsDossierDialog) prepareCharacterItemDetail() zStatsCatalogDetailLayout {
	character := d.selectedCharacterModel()
	if character == nil || len(character.Equipped) == 0 {
		d.summaryOutput.Reset()
		d.detailOutput.Reset()

		var empty zStatsCatalogDetailLayout

		return empty
	}
	item := character.Equipped[d.tabSelections[ZStatsDossierCharacterTab]]

	return d.prepareCatalogDetail(
		d.layout.characterItemRect,
		func(output *text.Output) {
			d.appendDetailHeaderTo(output, item.Slot+": "+item.Name)
			d.appendDetailPairTo(output, "Category", item.Category, DialogValueColor)
			d.appendDetailPairTo(output, "Attack", strconv.Itoa(item.Attack), DialogValueColor)
			d.appendDetailPairTo(output, "Defense", strconv.Itoa(item.Defense), DialogValueColor)
		},
		func(output *text.Output) {
			d.appendDetailPairTo(output, "Strength required", strconv.Itoa(item.StrengthRequired), DialogValueColor)
			d.appendDetailPairTo(output, "Range", strconv.Itoa(item.Range), DialogValueColor)
			d.appendDetailPairTo(output, "Hands required", zStatsHandsRequired(item.TwoHanded), DialogValueColor)
			if item.RequiresAmmo {
				d.appendDetailPairTo(output, "Ammo", item.AmmoName, DialogValueColor)
			}
			d.appendDetailPairTo(output, "Abilities", zStatsJoinedOrNone(item.Abilities), DialogValueColor)
		},
	)
}

func (d *ZStatsDossierDialog) drawListPane(screen *ebiten.Image, rect image.Rectangle) {
	d.drawPanelHeader(screen, rect, d.listHeader())
	body := d.panelBodyRect(rect)
	if d.activeTab == ZStatsDossierSpellsTab {
		heading := image.Rect(body.Min.X, rect.Min.Y+d.layout.padding,
			body.Max.X-d.scrollIndicatorWidth(), body.Min.Y)
		d.spellView.drawPreparedHeading(screen, heading)
	}
	count := d.currentItemCount()
	if count == 0 {
		d.drawOutputLine(screen, body, d.emptyListText(), DialogEmptyColor, etext.AlignStart)

		return
	}

	selection := d.currentSelection()
	rowHeight := d.listRowHeight(rect)
	visibleRows := max(1, body.Dy()/rowHeight)
	start, end := zStatsVisibleRange(selection, count, visibleRows)
	for index := start; index < end; index++ {
		rowTop := body.Min.Y + (index-start)*rowHeight
		rowRect := image.Rect(body.Min.X, rowTop, body.Max.X, min(body.Max.Y, rowTop+rowHeight))
		d.drawListRow(screen, rowRect, index, index == selection)
	}
	drawMasterDetailScrollIndicators(screen, body, start, end, count, rowHeight,
		d.scrollIndicatorWidth(), d.bodyFont, d.rowOutput)
}

func (d *ZStatsDossierDialog) drawSpellFilter(screen *ebiten.Image, rect image.Rectangle) {
	d.spellView.drawSearch(screen, d.spellBrowser, rect)
}

func (d *ZStatsDossierDialog) drawListRow(
	screen *ebiten.Image,
	rect image.Rectangle,
	index int,
	selected bool,
) {
	if d.activeTab == ZStatsDossierSpellsTab {
		if spell := d.spellAtFilteredPosition(index); spell != nil {
			drawSpellCatalogRow(screen, d.rowOutput, d.spellView.count,
				image.Rect(rect.Min.X+d.layout.padding, rect.Min.Y, rect.Max.X-d.scrollIndicatorWidth(), rect.Max.Y),
				*spell, selected, d.spellBrowser.filterMode != spellFilterMixable || spellMixable(*spell))
		}

		return
	}
	if selected {
		fillSelectedRow(screen, rect)
	}
	d.prepareListRow(rect, index, selected)
	clippedRect := rect.Intersect(screen.Bounds())
	if clippedRect.Empty() {
		return
	}
	clipped, ok := screen.SubImage(clippedRect).(*ebiten.Image)
	if !ok {
		panic("ZStatsDossierDialog: Ebitengine list clip is not an image")
	}
	left := rect.Min.X + d.layout.padding
	if d.rowIconWidth > 0 {
		d.rowIconOutput.DrawPanelContent(clipped, image.Pt(left, rect.Min.Y), d.rowIconWidth, etext.AlignStart)
		left += d.rowIconWidth + d.pixelUnit()
	}
	d.rowOutput.DrawPanelContent(
		clipped,
		image.Pt(left, rect.Min.Y),
		d.rowWidth,
		etext.AlignStart,
	)
}

func (d *ZStatsDossierDialog) prepareListRow(rect image.Rectangle, index int, selected bool) {
	if d.activeTab == ZStatsDossierSpellsTab {
		if spell := d.spellAtFilteredPosition(index); spell != nil {
			prepareSpellCatalogRow(d.rowOutput, d.spellView.count,
				image.Rect(rect.Min.X+d.layout.padding, rect.Min.Y, rect.Max.X-d.scrollIndicatorWidth(), rect.Max.Y),
				*spell, selected, d.spellBrowser.filterMode != spellFilterMixable || spellMixable(*spell))
		}

		return
	}
	textColor := DialogItemNameColor
	var iconColor stdcolor.Color = pkgcolor.EGAYellow
	secondaryColor := DialogValueColor
	if selected {
		textColor = DialogSelectedText
		iconColor = pkgcolor.EGABlack
		secondaryColor = DialogSelectedText
	}
	d.rowWidth = rect.Dx() - d.layout.padding*zStatsDossierPaneInsetMultiplier
	if d.activeTab != ZStatsDossierCharacterTab {
		d.rowWidth -= d.scrollIndicatorWidth()
	}
	d.rowWidth = max(1, d.rowWidth)
	d.rowOutput.SetMaxCharsPerLine(max(
		1,
		d.rowOutput.Font.CharsForWidth(d.rowWidth),
	))
	d.rowOutput.Reset()
	d.rowIconOutput.Reset()
	d.rowIconOutput.SetFont(d.bodyFont, d.rowOutput.GetLineSpacing())
	d.rowIconWidth = 0
	line := d.rowOutput.AppendLine()

	switch d.activeTab {
	case ZStatsDossierCharacterTab:
		d.appendCharacterListRow(line, index, textColor, iconColor, secondaryColor)
	case ZStatsDossierProvisionsTab:
		d.appendProvisionListRow(line, index, selected, textColor)
	case ZStatsDossierArmoryTab:
		d.appendArmoryListRow(line, index, selected, textColor, iconColor)
	case ZStatsDossierSpellsTab:
		// Two-line spell rows are rendered by the shared catalog path above.
	case ZStatsDossierReagentsTab, ZStatsDossierPotionsTab, ZStatsDossierScrollsTab:
		d.appendMagicInventoryListRow(line, index, selected, textColor)
	case ZStatsDossierItemsTab:
		d.appendItemListRow(line, index, textColor)
	}
	line.Done()
}

func (d *ZStatsDossierDialog) appendProvisionListRow(
	line *text.LineBuilder,
	index int,
	selected bool,
	textColor stdcolor.Color,
) {
	if index < 0 || index >= len(d.dossier.Provisions) {
		return
	}
	item := d.dossier.Provisions[index]
	if item.Quantity == 0 && !selected {
		textColor = DialogEmptyColor
	}
	line.Text(d.countedRowLabel(item.Name, fmt.Sprintf(" (%d)", item.Quantity)), textColor)
}

func (d *ZStatsDossierDialog) appendCharacterListRow(
	line *text.LineBuilder,
	index int,
	textColor stdcolor.Color,
	iconColor stdcolor.Color,
	secondaryColor stdcolor.Color,
) {
	character := d.selectedCharacterModel()
	if character == nil || index >= len(character.Equipped) {
		return
	}
	item := character.Equipped[index]
	if equipment, ok := zStatsEquipmentByID(item.ItemID); ok {
		d.prepareRowIcon(d.rowIconOutput.AppendLine().EquipmentIconWithColor(equipment, iconColor))
		d.appendFullNameRow(line, item.Name, "", textColor)

		return
	}
	if slot, ok := zStatsEquipmentSlot(item.Slot); ok {
		d.prepareRowIcon(d.rowIconOutput.AppendLine().SlotIconWithColor(slot, secondaryColor))
		d.appendFullNameRow(line, zStatsNoneLabel, "", secondaryColor)
	}
}

func (d *ZStatsDossierDialog) appendArmoryListRow(
	line *text.LineBuilder,
	index int,
	selected bool,
	textColor stdcolor.Color,
	iconColor stdcolor.Color,
) {
	item := d.dossier.Armory[index]
	if item.Available == 0 && item.Readied == 0 && !selected {
		textColor = DialogEmptyColor
		iconColor = DialogEmptyColor
	}
	if equipment, ok := zStatsEquipmentByID(item.ID); ok {
		d.prepareRowIcon(d.rowIconOutput.AppendLine().EquipmentIconWithColor(equipment, iconColor))
	}
	d.appendFullNameRow(line, item.Name, "", textColor)
}

func (d *ZStatsDossierDialog) appendMagicInventoryListRow(
	line *text.LineBuilder,
	index int,
	selected bool,
	textColor stdcolor.Color,
) {
	item := d.magicInventoryItem(index)
	if item == nil {
		return
	}
	if item.Quantity == 0 && !selected {
		textColor = DialogEmptyColor
	}
	line.Text(d.countedRowLabel(item.Name, fmt.Sprintf(" (%d)", item.Quantity)), textColor)
}

func (d *ZStatsDossierDialog) appendItemListRow(
	line *text.LineBuilder,
	index int,
	textColor stdcolor.Color,
) {
	item := d.dossier.Items[d.itemIndices[index]]
	if item.MoonPhase.Present {
		scale := max(1, int(d.rowOutput.GetLineSpacing())/text.BitmapCharHeight)
		d.rowIconOutput.SetFont(d.bodyFont, float64(scale*text.BitmapCharHeight))
		d.prepareRowIcon(appendMoonPhaseGlyph(d.rowIconOutput.AppendLine(), item.MoonPhase.Phase, textColor))
	}
	d.appendFullNameRow(line, item.Name, fmt.Sprintf(" (%d)", item.Quantity), textColor)
}

func (d *ZStatsDossierDialog) prepareRowIcon(line *text.LineBuilder) {
	d.rowIconWidth = int(math.Ceil(d.rowIconOutput.MeasureLineSegments(line.GetSegments())))
	line.Done()
	d.rowWidth = max(1, d.rowWidth-d.rowIconWidth-d.pixelUnit())
}

// Full names wrap at measured word boundaries. Quantity remains on the first
// row; its advance is reserved before fitting any name text. The icon has its
// own Output so a bitmap cell can never wrap away the beginning of the name.
func (d *ZStatsDossierDialog) appendFullNameRow(
	line *text.LineBuilder, name, suffix string, foreground stdcolor.Color,
) {
	suffixWidth, _ := d.bodyFont.MeasureString(suffix)
	rows := text.WrapJournalHistory(name, d.bodyFont, max(1, d.rowWidth-int(math.Ceil(suffixWidth))))
	d.rowOutput.SetMaxCharsPerLine(max(1, utf8.RuneCountInString(name+suffix)))
	for index, row := range rows {
		if index > 0 {
			line.Text("\n", foreground)
		}
		line.Text(row.Text, foreground)
		if index == 0 {
			line.Text(suffix, foreground)
		}
	}
}

// Fit the name before insertion: wrapping a one-row Output overwrites its prefix.
// Reserve the quantity suffix first. Equipment and Items use full wrapping rows.
func (d *ZStatsDossierDialog) countedRowLabel(name, suffix string) string {
	font := d.rowOutput.Font
	capacity := max(1, font.CharsForWidth(d.rowWidth))
	reservedWidth, _ := font.MeasureString(suffix)
	name = FitDialogLine(name, font, max(0, d.rowWidth-int(math.Ceil(reservedWidth))))
	name = truncateRunes(name, max(0, capacity-utf8.RuneCountInString(suffix)))
	label := name + suffix
	if utf8.RuneCountInString(label) > capacity {
		label = truncateRunes(label, capacity)
	}

	return label
}

func (d *ZStatsDossierDialog) scrollIndicatorWidth() int {
	return max(d.layout.padding*zStatsDossierPaneInsetMultiplier, int(math.Ceil(d.bodyFont.TextFace.Size)))
}

func (d *ZStatsDossierDialog) drawCatalogDetailPane(screen *ebiten.Image, rect image.Rectangle) {
	switch d.activeTab {
	case ZStatsDossierProvisionsTab:
		d.drawProvisionDetail(screen, rect)
	case ZStatsDossierArmoryTab:
		d.drawArmoryDetail(screen, rect)
	case ZStatsDossierSpellsTab:
		d.drawSpellDetail(screen, rect)
	case ZStatsDossierReagentsTab, ZStatsDossierPotionsTab, ZStatsDossierScrollsTab:
		d.drawMagicInventoryDetail(screen, rect)
	case ZStatsDossierItemsTab:
		d.drawItemDetail(screen, rect)
	case ZStatsDossierCharacterTab:
	}
}

func (d *ZStatsDossierDialog) drawProvisionDetail(screen *ebiten.Image, rect image.Rectangle) {
	selection := d.tabSelections[ZStatsDossierProvisionsTab]
	if selection < 0 || selection >= len(d.dossier.Provisions) {
		d.drawEmptyDetail(screen, rect, "No provision records.")

		return
	}
	item := d.dossier.Provisions[selection]
	d.drawPreparedCatalogDetail(screen, item.SpriteIndex, d.prepareProvisionDetail(rect))
}

func (d *ZStatsDossierDialog) prepareProvisionDetail(rect image.Rectangle) zStatsCatalogDetailLayout {
	selection := d.tabSelections[ZStatsDossierProvisionsTab]
	if selection < 0 || selection >= len(d.dossier.Provisions) {
		d.summaryOutput.Reset()
		d.detailOutput.Reset()

		var empty zStatsCatalogDetailLayout

		return empty
	}
	item := d.dossier.Provisions[selection]

	return d.prepareCatalogDetail(rect, func(output *text.Output) {
		d.appendDetailHeaderTo(output, item.Name)
		d.appendDetailPairTo(
			output,
			"Quantity",
			strconv.FormatUint(uint64(item.Quantity), zStatsDecimalBase),
			DialogValueColor,
		)
	}, func(output *text.Output) {
		d.appendDetailPairTo(output, "Description", item.Description, DialogValueColor)
	})
}

func (d *ZStatsDossierDialog) drawArmoryDetail(screen *ebiten.Image, rect image.Rectangle) {
	if len(d.dossier.Armory) == 0 {
		d.drawEmptyDetail(screen, rect, "No armory records.")

		return
	}
	item := d.dossier.Armory[d.tabSelections[ZStatsDossierArmoryTab]]
	d.drawPreparedCatalogDetail(screen, item.SpriteIndex, d.prepareArmoryDetail(rect))
}

func (d *ZStatsDossierDialog) prepareArmoryDetail(rect image.Rectangle) zStatsCatalogDetailLayout {
	if len(d.dossier.Armory) == 0 {
		d.summaryOutput.Reset()
		d.detailOutput.Reset()

		var empty zStatsCatalogDetailLayout

		return empty
	}
	item := d.dossier.Armory[d.tabSelections[ZStatsDossierArmoryTab]]

	return d.prepareCatalogDetail(rect, func(output *text.Output) {
		d.appendDetailHeaderTo(output, item.Name)
		d.appendDetailPairTo(output, "Category", item.Category, DialogValueColor)
		d.appendDetailPairTo(
			output,
			"Available / Readied",
			fmt.Sprintf("%d / %d", item.Available, item.Readied),
			DialogValueColor,
		)
	}, func(output *text.Output) {
		d.appendDetailPairTo(output, "Attack", strconv.Itoa(item.Attack), DialogValueColor)
		d.appendDetailPairTo(output, "Defense", strconv.Itoa(item.Defense), DialogValueColor)
		d.appendDetailPairTo(output, "Strength required", strconv.Itoa(item.StrengthRequired), DialogValueColor)
		d.appendDetailPairTo(output, "Range", strconv.Itoa(item.Range), DialogValueColor)
		d.appendDetailPairTo(output, "Hands required", zStatsHandsRequired(item.TwoHanded), DialogValueColor)
		if item.RequiresAmmo {
			d.appendDetailPairTo(output, "Ammo", item.AmmoName, DialogValueColor)
		}
		d.appendDetailPairTo(output, "Abilities", zStatsJoinedOrNone(item.Abilities), DialogValueColor)
	})
}

func (d *ZStatsDossierDialog) drawSpellDetail(screen *ebiten.Image, rect image.Rectangle) {
	prepared := d.prepareSpellDetail(rect)
	if spell := d.selectedSpell(); spell != nil {
		d.drawPreparedCatalogDetail(screen, spell.SpriteIndex, prepared)
	} else {
		d.drawEmptyDetail(screen, rect, "No matching spells.")
	}
}

func (d *ZStatsDossierDialog) prepareSpellDetail(rect image.Rectangle) zStatsCatalogDetailLayout {
	spell := d.selectedSpell()
	if spell == nil {
		d.summaryOutput.Reset()
		d.detailOutput.Reset()

		var empty zStatsCatalogDetailLayout

		return empty
	}

	return d.prepareCatalogDetail(rect, func(output *text.Output) {
		d.appendDetailHeaderTo(output, zStatsDisplayName(spell.Name, spell.Syllables))
		d.appendDetailTextTo(output, spell.EffectLabel, DialogControlHintColor)
		appendSpellMetrics(output, *spell)
	}, func(output *text.Output) {
		d.appendDetailTextTo(output, spell.Description, DialogValueColor)
		appendSpellFormula(output, *spell)
		if spell.Hidden {
			if source := zStatsLearningSource(spell.LearningSourceKind, spell.LearningSource); source != "" {
				d.appendDetailPairTo(output, "Learned from", source, DialogValueColor)
			}
			if spell.LearningDate != "" {
				d.appendDetailPairTo(output, "Learned on", spell.LearningDate, DialogValueColor)
			}
		}
	})
}

func (d *ZStatsDossierDialog) drawMagicInventoryDetail(screen *ebiten.Image, rect image.Rectangle) {
	item := d.magicInventoryItem(d.tabSelections[d.activeTab])
	if item == nil {
		d.drawEmptyDetail(screen, rect, "No "+strings.ToLower(d.tabTitle(d.activeTab))+".")

		return
	}
	d.drawPreparedCatalogDetail(screen, item.SpriteIndex, d.prepareMagicInventoryDetail(rect))
}

func (d *ZStatsDossierDialog) prepareMagicInventoryDetail(rect image.Rectangle) zStatsCatalogDetailLayout {
	item := d.magicInventoryItem(d.tabSelections[d.activeTab])
	if item == nil {
		d.summaryOutput.Reset()
		d.detailOutput.Reset()

		var empty zStatsCatalogDetailLayout

		return empty
	}

	return d.prepareCatalogDetail(rect, func(output *text.Output) {
		d.appendDetailHeaderTo(output, item.Name)
		d.appendDetailPairTo(output, "Section", d.tabTitle(d.activeTab), DialogValueColor)
		d.appendDetailPairTo(
			output,
			"Quantity",
			strconv.FormatUint(uint64(item.Quantity), zStatsDecimalBase),
			DialogValueColor,
		)
	}, func(output *text.Output) {
		d.appendDetailPairTo(output, "Description", item.Description, DialogValueColor)
	})
}

func (d *ZStatsDossierDialog) drawItemDetail(screen *ebiten.Image, rect image.Rectangle) {
	if len(d.itemIndices) == 0 {
		d.drawEmptyDetail(screen, rect, "No special possessions or carried moonstones.")

		return
	}
	item := d.dossier.Items[d.itemIndices[d.tabSelections[ZStatsDossierItemsTab]]]
	d.drawPreparedCatalogDetail(screen, item.SpriteIndex, d.prepareItemDetail(rect))
}

func (d *ZStatsDossierDialog) prepareItemDetail(rect image.Rectangle) zStatsCatalogDetailLayout {
	if len(d.itemIndices) == 0 {
		d.summaryOutput.Reset()
		d.detailOutput.Reset()

		var empty zStatsCatalogDetailLayout

		return empty
	}
	item := d.dossier.Items[d.itemIndices[d.tabSelections[ZStatsDossierItemsTab]]]

	if item.MoonPhase.Present {
		return d.prepareMoonstoneDetail(rect, item)
	}

	return d.prepareCatalogDetail(rect, func(output *text.Output) {
		d.appendDetailHeaderTo(output, item.Name)
		d.appendDetailPairTo(output, "Category", item.Category, DialogValueColor)
		d.appendDetailPairTo(
			output,
			"Quantity",
			strconv.FormatUint(uint64(item.Quantity), zStatsDecimalBase),
			DialogValueColor,
		)
	}, func(output *text.Output) {
		d.appendDetailPairTo(output, "Description", item.Description, DialogValueColor)
	})
}

type zStatsCatalogDetailLayout struct {
	portrait, summary, body image.Rectangle
	phaseIcon               image.Rectangle
}

// Bound the phase art by two body lines and the existing UI pixel unit, never
// by the detail pane's remaining width/height. Output's bitmap scale derives
// from its line spacing, so an 8-pixel multiple gives nearest-neighbour integers.
func (d *ZStatsDossierDialog) moonstoneIconScale() int {
	padding := d.pixelUnit() * zStatsDossierPortraitInsetUnits
	height := int(d.rowOutput.GetTotalHeightForLines(zStatsMoonstoneHeaderLines))

	return max(1, min(d.pixelUnit(), (height-padding*zStatsDossierPaneInsetMultiplier)/text.BitmapCharHeight))
}

func (d *ZStatsDossierDialog) prepareMoonstoneDetail(
	rect image.Rectangle, item game_state.ZStatsOwnedItem,
) zStatsCatalogDetailLayout {
	content := insetZStatsRect(rect, d.layout.padding*zStatsDossierPaneInsetMultiplier)
	unit := d.pixelUnit()
	// Round the small frame inward to the UI grid; the outer dossier is unchanged.
	left := (content.Min.X + unit - 1) / unit * unit
	top := (content.Min.Y + unit - 1) / unit * unit
	iconSize := d.moonstoneIconScale()*text.BitmapCharHeight +
		unit*zStatsDossierPortraitInsetUnits*zStatsDossierPaneInsetMultiplier
	icon := image.Rect(left, top, left+iconSize, top+iconSize)
	summary := image.Rect(icon.Max.X+d.layout.padding, top, content.Max.X, content.Max.Y)
	d.summaryOutput.SetMaxCharsPerLine(max(1, d.bodyFont.CharsForWidth(summary.Dx())))
	d.summaryOutput.Reset()
	d.appendDetailHeaderTo(d.summaryOutput, item.Name)
	d.summaryOutput.AppendLine().Text(item.Category+" · ", DialogValueColor).
		Text("Quantity: ", DialogLabelColor).Text(strconv.Itoa(int(item.Quantity)), DialogValueColor).WordWrap().Done()
	summary.Max.Y = summary.Min.Y + int(math.Ceil(d.summaryOutput.GetTotalHeightForLines(
		d.summaryOutput.GetPanelContentLineCount(),
	)))
	body := image.Rect(content.Min.X, max(icon.Max.Y, summary.Max.Y)+d.layout.padding, content.Max.X, content.Max.Y)
	d.detailOutput.SetMaxCharsPerLine(max(1, d.bodyFont.CharsForWidth(body.Dx())))
	d.detailOutput.Reset()
	d.appendDetailPairTo(d.detailOutput, "Description", item.Description, DialogValueColor)
	d.preparePanelPage(d.detailOutput, body)
	d.phaseOutput.Reset()
	appendMoonPhaseGlyph(d.phaseOutput.AppendLine(), item.MoonPhase.Phase, DialogHeaderColor).Done()

	return zStatsCatalogDetailLayout{portrait: image.Rect(0, 0, 0, 0), phaseIcon: icon, summary: summary, body: body}
}

func (d *ZStatsDossierDialog) prepareCatalogDetail(
	rect image.Rectangle, populateSummary, populateBody func(*text.Output),
) zStatsCatalogDetailLayout {
	portraitRect, summaryRect, bodyRect := d.catalogDetailRects(rect)

	d.summaryOutput.SetMaxCharsPerLine(max(1, d.bodyFont.CharsForWidth(summaryRect.Dx())))
	d.summaryOutput.Reset()
	populateSummary(d.summaryOutput)
	if d.activeTab == ZStatsDossierSpellsTab {
		// Spell summaries remain fixed; only genuinely oversized body content pages.
		lines := d.summaryOutput.GetPanelContentLineCount()
		summaryRect.Max.Y = summaryRect.Min.Y + int(math.Ceil(d.summaryOutput.GetTotalHeightForLines(lines)))
		bodyRect.Min.Y = min(bodyRect.Max.Y, max(portraitRect.Max.Y, summaryRect.Max.Y)+d.layout.padding)
		summaryRect = summaryRect.Intersect(rect)
	} else {
		d.preparePanelPage(d.summaryOutput, summaryRect)
	}

	d.detailOutput.SetMaxCharsPerLine(max(1, d.bodyFont.CharsForWidth(bodyRect.Dx())))
	d.detailOutput.Reset()
	populateBody(d.detailOutput)
	d.preparePanelPage(d.detailOutput, bodyRect)

	return zStatsCatalogDetailLayout{
		portrait: portraitRect, phaseIcon: image.Rect(0, 0, 0, 0), summary: summaryRect, body: bodyRect,
	}
}

func (d *ZStatsDossierDialog) drawPreparedCatalogDetail(
	screen *ebiten.Image, spriteIndex indexes.SpriteIndex, prepared zStatsCatalogDetailLayout,
) {
	if !prepared.phaseIcon.Empty() {
		DrawClassicPixelRoundedFrame(screen, prepared.phaseIcon, d.pixelUnit(), DialogRuleLightColor)
		inset := d.pixelUnit() * zStatsDossierPortraitInsetUnits
		glyph := prepared.phaseIcon.Inset(inset)
		d.phaseOutput.DrawPanelContent(screen, glyph.Min, glyph.Dx(), etext.AlignStart)
	} else {
		d.drawFramedSprite(screen, prepared.portrait, spriteIndex)
	}
	if d.activeTab == ZStatsDossierSpellsTab || !prepared.phaseIcon.Empty() {
		d.drawPanelOutputRange(screen, d.summaryOutput, prepared.summary, 0, d.summaryOutput.GetPanelContentLineCount())
	} else {
		d.drawPagedPanelOutput(screen, d.summaryOutput, prepared.summary)
	}
	d.drawPagedPanelOutput(screen, d.detailOutput, prepared.body)
}

func (d *ZStatsDossierDialog) drawPagedPanelOutput(
	screen *ebiten.Image,
	output *text.Output,
	rect image.Rectangle,
) {
	start, capacity := d.preparePanelPage(output, rect)
	d.drawPanelOutputRange(screen, output, rect, start, capacity)
}

func (d *ZStatsDossierDialog) preparePanelPage(output *text.Output, rect image.Rectangle) (int, int) {
	capacity := max(1, output.GetLineCapacity(max(1, rect.Dy())))
	lineCount := output.GetPanelContentLineCount()
	pageCount := max(1, (lineCount+capacity-1)/capacity)
	d.detailPages = max(d.detailPages, pageCount)
	page := min(d.detailPage, pageCount-1)

	return page * capacity, capacity
}

func (d *ZStatsDossierDialog) drawPanelOutputRange(
	screen *ebiten.Image, output *text.Output, rect image.Rectangle, start, count int,
) {
	if screen == nil || count == 0 || rect.Empty() {
		return
	}
	clippedRect := rect.Intersect(screen.Bounds())
	if clippedRect.Empty() {
		return
	}
	clipped, ok := screen.SubImage(clippedRect).(*ebiten.Image)
	if !ok {
		panic("ZStatsDossierDialog: Ebitengine panel clip is not an image")
	}
	output.DrawPanelContentRange(
		clipped,
		rect.Min,
		max(1, rect.Dx()),
		etext.AlignStart,
		start,
		count,
	)
}

func (d *ZStatsDossierDialog) catalogDetailRects(
	rect image.Rectangle,
) (image.Rectangle, image.Rectangle, image.Rectangle) {
	content := insetZStatsRect(rect, d.layout.padding*zStatsDossierPaneInsetMultiplier)
	portraitSize := min(
		content.Dx()/zStatsDossierPortraitWidthDivisor,
		int(math.Ceil(d.rowOutput.GetTotalHeightForLines(zStatsDossierPortraitLines)))+
			d.layout.padding*zStatsDossierPaneInsetMultiplier,
	)
	portraitSize = min(portraitSize, content.Dy()/zStatsDossierCenterDivisor)
	portraitSize = max(1, portraitSize)
	portraitRect := image.Rect(
		content.Min.X,
		content.Min.Y,
		content.Min.X+portraitSize,
		content.Min.Y+portraitSize,
	)
	summaryRect := image.Rect(
		portraitRect.Max.X+d.layout.padding,
		content.Min.Y,
		content.Max.X,
		portraitRect.Max.Y,
	)
	bodyRect := image.Rect(content.Min.X, portraitRect.Max.Y+d.layout.padding, content.Max.X, content.Max.Y)

	return portraitRect, summaryRect, bodyRect
}

func (d *ZStatsDossierDialog) drawFramedSprite(
	screen *ebiten.Image,
	rect image.Rectangle,
	spriteIndex indexes.SpriteIndex,
) {
	DrawClassicPixelRoundedFrame(screen, rect, d.pixelUnit(), DialogRuleLightColor)
	if spriteIndex == indexes.NoSprites {
		return
	}
	sprite := d.spriteSheet.GetRawSprite(spriteIndex)
	if sprite == nil || sprite.Bounds().Empty() {
		return
	}
	inset := max(1, d.pixelUnit()*zStatsDossierPortraitInsetUnits)
	inner := insetZStatsRect(rect, inset)
	scale := max(1, min(inner.Dx()/sprite.Bounds().Dx(), inner.Dy()/sprite.Bounds().Dy()))
	drawWidth := sprite.Bounds().Dx() * scale
	drawHeight := sprite.Bounds().Dy() * scale
	d.drawOp.GeoM.Reset()
	d.drawOp.Filter = ebiten.FilterNearest
	d.drawOp.GeoM.Translate(float64(-sprite.Bounds().Min.X), float64(-sprite.Bounds().Min.Y))
	d.drawOp.GeoM.Scale(float64(scale), float64(scale))
	d.drawOp.GeoM.Translate(
		float64(inner.Min.X+(inner.Dx()-drawWidth)/zStatsDossierCenterDivisor),
		float64(inner.Min.Y+(inner.Dy()-drawHeight)/zStatsDossierCenterDivisor),
	)
	screen.DrawImage(sprite, d.drawOp)
}

func (d *ZStatsDossierDialog) drawEmptyDetail(screen *ebiten.Image, rect image.Rectangle, value string) {
	d.summaryOutput.Reset()
	d.detailOutput.Reset()
	content := insetZStatsRect(rect, d.layout.padding*zStatsDossierPaneInsetMultiplier)
	d.drawOutputLine(screen, content, value, DialogEmptyColor, etext.AlignCenter)
}

func (d *ZStatsDossierDialog) drawPanelHeader(screen *ebiten.Image, rect image.Rectangle, value string) {
	headerRect := d.preparePanelHeader(rect, value)
	d.titleOutput.DrawPanelContent(screen, headerRect.Min, headerRect.Dx(), etext.AlignStart)
}

func (d *ZStatsDossierDialog) preparePanelHeader(rect image.Rectangle, value string) image.Rectangle {
	content := insetZStatsRect(rect, d.layout.padding)
	headerRect := image.Rect(
		content.Min.X,
		content.Min.Y,
		content.Max.X,
		min(content.Max.Y, content.Min.Y+int(math.Ceil(d.titleOutput.GetLineSpacing()))),
	)
	d.titleOutput.SetMaxCharsPerLine(max(1, d.titleFont.CharsForWidth(headerRect.Dx())))
	d.titleOutput.Reset()
	d.titleOutput.AppendLine().Text(value, DialogHeaderColor).Done()

	return headerRect
}

func (d *ZStatsDossierDialog) panelBodyRect(rect image.Rectangle) image.Rectangle {
	content := insetZStatsRect(rect, d.layout.padding*zStatsDossierPaneInsetMultiplier)
	content.Min.Y = min(content.Max.Y, content.Min.Y+int(math.Ceil(d.titleOutput.GetLineSpacing())))

	return content
}

func (d *ZStatsDossierDialog) listHeader() string {
	switch d.activeTab {
	case ZStatsDossierCharacterTab:
		return "READIED"
	case ZStatsDossierProvisionsTab:
		return "PROVISIONS"
	case ZStatsDossierArmoryTab:
		return "ARMORY"
	case ZStatsDossierSpellsTab,
		ZStatsDossierReagentsTab,
		ZStatsDossierPotionsTab,
		ZStatsDossierScrollsTab:
		return strings.ToUpper(d.tabTitle(d.activeTab))
	case ZStatsDossierItemsTab:
		return "POSSESSIONS"
	default:
		return ""
	}
}

func (d *ZStatsDossierDialog) emptyListText() string {
	switch d.activeTab {
	case ZStatsDossierCharacterTab:
		return "No character equipment."
	case ZStatsDossierProvisionsTab:
		return "No provision records."
	case ZStatsDossierArmoryTab:
		return "No armory records."
	case ZStatsDossierSpellsTab:
		return "No matching spells."
	case ZStatsDossierReagentsTab, ZStatsDossierPotionsTab, ZStatsDossierScrollsTab:
		return "No " + strings.ToLower(d.tabTitle(d.activeTab)) + "."
	case ZStatsDossierItemsTab:
		return "No special possessions or carried moonstones."
	default:
		return "No records."
	}
}

func (d *ZStatsDossierDialog) magicInventoryItem(index int) *game_state.ZStatsInventoryItem {
	var items []game_state.ZStatsInventoryItem
	switch d.activeTab {
	case ZStatsDossierReagentsTab:
		items = d.dossier.Magic.Reagents
	case ZStatsDossierPotionsTab:
		items = d.dossier.Magic.Potions
	case ZStatsDossierScrollsTab:
		items = d.dossier.Magic.Scrolls
	case ZStatsDossierCharacterTab, ZStatsDossierProvisionsTab, ZStatsDossierArmoryTab,
		ZStatsDossierSpellsTab, ZStatsDossierItemsTab:
		return nil
	default:
		return nil
	}
	if index < 0 || index >= len(items) {
		return nil
	}

	return &items[index]
}

func (d *ZStatsDossierDialog) spellAtFilteredPosition(position int) *game_state.ZStatsSpell {
	if position < 0 || position >= len(d.spellBrowser.filtered) {
		return nil
	}
	index := d.spellBrowser.filtered[position]

	return &d.dossier.Magic.Spells[index]
}

func (d *ZStatsDossierDialog) prepareFooter() image.Rectangle {
	footerContent := insetZStatsRect(d.layout.footerRect, d.layout.padding)
	textRect := footerContent
	if d.detailPageCount() > 1 {
		textRect.Max.X = max(textRect.Min.X, d.layout.pageLabelRect.Min.X-d.layout.padding)
	}
	d.footerOutput.Reset()
	searching := d.activeTab == ZStatsDossierSpellsTab && d.spellBrowser.searching
	hints := []ControlHint{{hintKeyTabCycle, "Section"}}
	switch d.activeTab {
	case ZStatsDossierCharacterTab:
		hints = append(hints, ControlHint{"Left/Right/1-6", "Character"}, ControlHint{hintKeyUpDown, "Equipment"})
	case ZStatsDossierSpellsTab:
		if searching {
			hints = append(hints, ControlHint{"Ctrl+U", "Clear search"})
		} else {
			hints = append(hints, ControlHint{"A-Z/Backspace", "Initials"}, ControlHint{"/", hintActionSearch})
		}
		hints = append(hints, ControlHint{"Ctrl+O", "Filter"}, ControlHint{"Ctrl+M", "Mix"},
			ControlHint{hintKeyUpDown, "Spell"})
	case ZStatsDossierProvisionsTab, ZStatsDossierArmoryTab, ZStatsDossierReagentsTab,
		ZStatsDossierPotionsTab, ZStatsDossierScrollsTab, ZStatsDossierItemsTab:
		hints = append(hints, ControlHint{hintKeyUpDown, hintActionItem})
	}
	hints = append(hints, ControlHint{hintKeyShiftPage, "List page"})
	if d.detailPageCount() > 1 && !searching {
		hints = append(hints, ControlHint{hintKeySpace, hintActionPage})
	}
	escape := ControlHint{hintKeyEsc, hintActionClose}
	if searching {
		escape.Action = "Leave search"
	}
	PackControlHints(d.footerOutput, append(hints, escape), textRect.Dx(), etext.AlignCenter)

	return textRect
}

func (d *ZStatsDossierDialog) drawFooter(screen *ebiten.Image) {
	textRect := d.prepareFooter()
	footerContent := insetZStatsRect(d.layout.footerRect, d.layout.padding)
	if d.detailPageCount() > 1 {
		d.drawOutputLine(
			screen,
			d.layout.pageLabelRect,
			fmt.Sprintf("Page %d/%d", d.detailPage+1, d.detailPageCount()),
			DialogControlHintColor,
			etext.AlignCenter,
		)
	}
	top := footerContent.Min.Y + max(
		0,
		(footerContent.Dy()-int(math.Ceil(d.footerOutput.GetTotalHeightForLines(
			d.footerOutput.GetPanelContentLineCount(),
		))))/zStatsDossierCenterDivisor,
	)
	clipped, ok := screen.SubImage(textRect.Intersect(screen.Bounds())).(*ebiten.Image)
	if !ok {
		panic("ZStatsDossierDialog: Ebitengine footer clip is not an image")
	}
	d.footerOutput.DrawPanelContent(clipped, image.Pt(textRect.Min.X, top), textRect.Dx(), etext.AlignCenter)
}

func (d *ZStatsDossierDialog) drawFrames(screen *ebiten.Image) {
	for _, rect := range d.frameRects() {
		DrawClassicPixelRoundedFrame(screen, rect, d.pixelUnit(), DialogRuleColor)
	}
}

func (d *ZStatsDossierDialog) frameRects() []image.Rectangle {
	rects := []image.Rectangle{d.layout.headerRect, d.layout.footerRect}
	if d.activeTab == ZStatsDossierSpellsTab {
		rects = append(rects, d.layout.spellSearchRect)
	}
	switch d.activeTab {
	case ZStatsDossierCharacterTab:
		rects = append(rects,
			d.layout.characterPortraitRect,
			d.layout.characterListRect,
			d.layout.characterStatsRect,
			d.layout.characterItemRect,
		)
	case ZStatsDossierProvisionsTab,
		ZStatsDossierArmoryTab,
		ZStatsDossierSpellsTab,
		ZStatsDossierReagentsTab,
		ZStatsDossierPotionsTab,
		ZStatsDossierScrollsTab,
		ZStatsDossierItemsTab:
		rects = append(rects, d.layout.globalListRect, d.layout.globalDetailRect)
	}

	return rects
}

func (d *ZStatsDossierDialog) drawOutputLine(
	screen *ebiten.Image,
	rect image.Rectangle,
	value string,
	textColor stdcolor.Color,
	align etext.Align,
) {
	d.rowOutput.SetMaxCharsPerLine(max(1, d.bodyFont.CharsForWidth(max(1, rect.Dx()))))
	d.rowOutput.Reset()
	d.rowOutput.AppendLine().Text(value, textColor).Done()
	top := rect.Min.Y + max(
		0,
		(rect.Dy()-int(math.Ceil(d.rowOutput.GetFontHeight())))/zStatsDossierCenterDivisor,
	)
	d.rowOutput.DrawPanelContent(screen, image.Pt(rect.Min.X, top), max(1, rect.Dx()), align)
}

func (d *ZStatsDossierDialog) appendDetailHeaderTo(output *text.Output, value string) {
	output.AppendLine().Text(value, DialogHeaderColor).WordWrap().Done()
}

func (d *ZStatsDossierDialog) appendDetailPairTo(
	output *text.Output,
	label string,
	value string,
	valueColor stdcolor.Color,
) {
	output.AppendLine().
		Text(strings.TrimSpace(label)+": ", DialogLabelColor).
		Text(strings.TrimSpace(value), valueColor).
		WordWrap().
		Done()
}

func (d *ZStatsDossierDialog) appendDetailTextTo(output *text.Output, value string, valueColor stdcolor.Color) {
	output.AppendLine().Text(value, valueColor).WordWrap().Done()
}

func (d *ZStatsDossierDialog) visibleListRows(rect image.Rectangle) int {
	body := d.panelBodyRect(rect)

	return max(zStatsDossierMinimumRows, body.Dy()/d.listRowHeight(rect))
}

func (d *ZStatsDossierDialog) listRowHeight(rect image.Rectangle) int {
	rowHeight := max(1, int(math.Ceil(d.rowOutput.GetLineSpacing())))
	if d.activeTab == ZStatsDossierSpellsTab {
		return spellRowHeight(d.rowOutput)
	}
	if d.activeTab != ZStatsDossierCharacterTab && d.activeTab != ZStatsDossierArmoryTab &&
		d.activeTab != ZStatsDossierItemsTab {
		return rowHeight
	}
	// Use the same measured pitch for traversal and drawing. Short catalogs keep
	// one-line density; full names gain rows only when the actual width needs them.
	body := d.panelBodyRect(rect)
	for index := range d.currentItemCount() {
		d.prepareListRow(body, index, false)
		height := int(math.Ceil(d.rowOutput.GetTotalHeightForLines(d.rowOutput.GetPanelContentLineCount())))
		rowHeight = max(rowHeight, height, int(math.Ceil(d.rowIconOutput.GetLineSpacing())))
	}

	return rowHeight
}

func (d *ZStatsDossierDialog) activeListRect() image.Rectangle {
	switch d.activeTab {
	case ZStatsDossierCharacterTab:
		return d.layout.characterListRect
	case ZStatsDossierProvisionsTab,
		ZStatsDossierArmoryTab,
		ZStatsDossierSpellsTab,
		ZStatsDossierReagentsTab,
		ZStatsDossierPotionsTab,
		ZStatsDossierScrollsTab,
		ZStatsDossierItemsTab:
		return d.layout.globalListRect
	default:
		return image.Rect(0, 0, 0, 0)
	}
}

func (d *ZStatsDossierDialog) armoryItemByID(itemID int) *game_state.ZStatsArmoryItem {
	for index := range d.dossier.Armory {
		if d.dossier.Armory[index].ID == itemID {
			return &d.dossier.Armory[index]
		}
	}

	return nil
}

func (d *ZStatsDossierDialog) pixelUnit() int {
	return getClassicUIPixelUnit(d.lastScreenWidth, d.lastScreenHeight)
}

func initialZStatsCharacterPosition(dossier game_state.ZStatsDossier) int {
	for position, character := range dossier.Characters {
		if character.RosterIndex == dossier.InitialCharacterIndex {
			return position
		}
	}
	if len(dossier.Characters) > 0 {
		return 0
	}

	return -1
}

func zStatsDossierPlacementIsZero(placement sprites.PercentBasedPlacement) bool {
	return placement.StartPercentX == 0 && placement.EndPercentX == 0 &&
		placement.StartPercentY == 0 && placement.EndPercentY == 0
}

func zStatsDossierDefaultPlacement() sprites.PercentBasedPlacement {
	return sprites.PercentBasedPlacement{
		StartPercentX: zStatsDossierDefaultMarginX,
		EndPercentX:   1 - zStatsDossierDefaultMarginX,
		StartPercentY: zStatsDossierDefaultMarginY,
		EndPercentY:   1 - zStatsDossierDefaultMarginY,
	}
}

func insetZStatsRect(rect image.Rectangle, inset int) image.Rectangle {
	inset = max(0, inset)
	insetX := min(inset, max(0, rect.Dx()/zStatsDossierCenterDivisor))
	insetY := min(inset, max(0, rect.Dy()/zStatsDossierCenterDivisor))

	return image.Rect(rect.Min.X+insetX, rect.Min.Y+insetY, rect.Max.X-insetX, rect.Max.Y-insetY)
}

func zStatsVisibleRange(selection, count, visibleRows int) (int, int) {
	if count <= 0 {
		return 0, 0
	}
	visibleRows = max(1, min(visibleRows, count))
	start := max(0, selection-visibleRows/zStatsDossierCenterDivisor)
	start = min(start, count-visibleRows)

	return start, start + visibleRows
}

func zStatsEquipmentByID(itemID int) (references.Equipment, bool) {
	item, valid := references.ItemFromIdentity(references.ItemTypeEquipment, itemID)
	if !valid {
		return references.NoEquipment, false
	}
	equipment, valid := item.(references.Equipment)

	return equipment, valid
}

func zStatsEquipmentSlot(slotName string) (references.EquipmentSlot, bool) {
	switch strings.ToLower(strings.TrimSpace(slotName)) {
	case "helm":
		return references.SlotHelm, true
	case "armour", "armor":
		return references.SlotArmour, true
	case "weapon":
		return references.SlotWeapon, true
	case "shield":
		return references.SlotShield, true
	case "ring":
		return references.SlotRing, true
	case "amulet":
		return references.SlotAmulet, true
	default:
		return references.SlotNone, false
	}
}

func zStatsStatusColor(status string) stdcolor.Color {
	if zStatsHealthyStatus(status) {
		return DialogStatusOKColor
	}
	lower := strings.ToLower(strings.TrimSpace(status))
	if strings.Contains(lower, "dead") || strings.Contains(lower, "poison") {
		return DialogWarningColor
	}

	return DialogLabelColor
}

func zStatsHealthyStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "good", "healthy", "fine", "ok":
		return true
	default:
		return false
	}
}

func zStatsDisplayName(name, fallback string) string {
	if strings.TrimSpace(name) != "" {
		return name
	}

	return fallback
}

func zStatsLearningSource(kind, source string) string {
	source = strings.TrimSpace(source)
	switch game_state.KnowledgeSourceKind(strings.TrimSpace(kind)) {
	case game_state.KnowledgeSourceDialogue, game_state.KnowledgeSourceExperimentation,
		game_state.KnowledgeSourceObservation, game_state.KnowledgeSourceRevelation:
		return source
	case game_state.KnowledgeSourceMigration:
		return "Previously learned"
	default:
		return source
	}
}

func zStatsJoinedOrNone(values []string) string {
	if len(values) == 0 {
		return zStatsNoneLabel
	}

	return strings.Join(values, ", ")
}

func zStatsHandsRequired(twoHanded bool) string {
	if twoHanded {
		return "Two"
	}

	return "One"
}

func zStatsOwnedItemIndices(items []game_state.ZStatsOwnedItem) []int {
	indices := make([]int, 0, len(items))
	for index, item := range items {
		if item.Quantity == 0 {
			continue
		}
		if strings.EqualFold(item.Category, "Quest Item") || strings.EqualFold(item.Category, "Moonstone") {
			indices = append(indices, index)
		}
	}

	return indices
}

func cloneZStatsDossier(dossier game_state.ZStatsDossier) game_state.ZStatsDossier {
	cloned := dossier
	cloned.Characters = make([]game_state.ZStatsCharacter, len(dossier.Characters))
	for index, character := range dossier.Characters {
		cloned.Characters[index] = character
		cloned.Characters[index].Equipped = make([]game_state.ZStatsEquippedItem, len(character.Equipped))
		for itemIndex, item := range character.Equipped {
			cloned.Characters[index].Equipped[itemIndex] = item
			cloned.Characters[index].Equipped[itemIndex].Abilities = append([]string(nil), item.Abilities...)
		}
	}
	cloned.Provisions = append([]game_state.ZStatsProvision(nil), dossier.Provisions...)
	cloned.Armory = make([]game_state.ZStatsArmoryItem, len(dossier.Armory))
	for index, item := range dossier.Armory {
		cloned.Armory[index] = item
		cloned.Armory[index].Abilities = append([]string(nil), item.Abilities...)
	}
	cloned.Magic.Spells = cloneSpellCatalog(dossier.Magic.Spells)
	cloned.Magic.Reagents = append([]game_state.ZStatsInventoryItem(nil), dossier.Magic.Reagents...)
	cloned.Magic.Potions = append([]game_state.ZStatsInventoryItem(nil), dossier.Magic.Potions...)
	cloned.Magic.Scrolls = append([]game_state.ZStatsInventoryItem(nil), dossier.Magic.Scrolls...)
	cloned.Items = append([]game_state.ZStatsOwnedItem(nil), dossier.Items...)

	return cloned
}
