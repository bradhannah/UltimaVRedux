package widgets

import (
	"fmt"
	"image"
	stdcolor "image/color"
	"math"
	"slices"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	pkgcolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	questJournalTabCount              = 8
	questJournalMarginX               = 0.06
	questJournalMarginY               = 0.04
	questJournalFooterLines           = 2
	questJournalListRatio             = 0.38
	questJournalMinimumPadding        = 4
	questJournalPaddingDivisor        = 64
	questJournalLineSpacingMultiplier = 1.15
	questJournalMinimumTextChars      = 8
	questJournalDetailMaxLines        = 128
	questJournalCenterDivisor         = 2
	questJournalVerticalRegionDivisor = 3
	questJournalPaneInsetMultiplier   = 2
	questJournalStateLabel            = "State"
	questJournalNotLearned            = "Not learned"
)

var _ Widget = (*QuestJournalDialog)(nil)

// QuestJournalTab identifies one section of the Quest Journal.
type QuestJournalTab int

const (
	// QuestJournalChronicleTab displays durable semantic events in insertion order.
	QuestJournalChronicleTab QuestJournalTab = iota
	// QuestJournalVirtuesTab displays current shrine progress authority.
	QuestJournalVirtuesTab
	// QuestJournalShadowlordsTab displays current Shadowlord life authority.
	QuestJournalShadowlordsTab
	// QuestJournalDungeonsTab shows discovered dungeon facts.
	QuestJournalDungeonsTab
	// QuestJournalConversationTab retains the last actually presented conversation.
	QuestJournalConversationTab
	// QuestJournalExcerptsTab contains deliberate player selections.
	QuestJournalExcerptsTab
	// QuestJournalMoonstonesTab separates charted origins from observed destinations.
	QuestJournalMoonstonesTab
	// QuestJournalNotesTab holds the player's own notes, filed in sections.
	QuestJournalNotesTab
)

// QuestJournalDialogConfig configures the presentation-owned journal modal.
type QuestJournalDialogConfig struct {
	Keyboard         *input.Keyboard
	Placement        sprites.PercentBasedPlacement
	OnDismiss        func()
	NewEntries       [3][]bool
	ShadowlordSprite *ebiten.Image
	LastConversation journal.Conversation
	Excerpts         []journal.Exchange
	KeepExcerpt      func(journal.Exchange) ([]journal.Exchange, error)
	DeleteExcerpt    func(string) ([]journal.Exchange, error)
	// ArchiveExcerpt is optional for read-only consumers; nil reports storage unavailable.
	ArchiveExcerpt func(string, bool) ([]journal.Exchange, error)
	ExcerptError   string
	// Notes is a detached notebook; OnNotesIntent asks the scene to change it.
	Notes         journal.NotesState
	OnNotesIntent func(NotesIntent)
}

type questJournalLayout struct {
	panelRect  image.Rectangle
	headerRect image.Rectangle
	titleRect  image.Rectangle
	tabsRect   image.Rectangle
	bodyRect   image.Rectangle
	listRect   image.Rectangle
	detailRect image.Rectangle
	footerRect image.Rectangle
	tabRects   [questJournalTabCount]image.Rectangle
	padding    int
}

// QuestJournalDialog presents a detached read-only quest dossier.
type QuestJournalDialog struct {
	navigation
	dossier           game_state.QuestJournalDossier
	keyboard          *input.Keyboard
	placement         sprites.PercentBasedPlacement
	onDismiss         func()
	dismissed         bool
	focused           bool
	activeTab         QuestJournalTab
	selections        [questJournalTabCount]int
	detailPage        int
	newEntries        [3][]bool
	visitedTabs       [3]bool
	shadowlordSprite  *ebiten.Image
	searching         bool
	searchQuery       string
	readingQuery      string
	searchEntries     []questJournalSearchEntry
	searchResults     []int
	searchSelection   int
	searchInput       *TextInput
	cachedDetailPages [][]detailLine

	outerBorder            *Border
	font                   *text.UltimaFont
	titleOutput            *text.Output
	tabOutput              *text.Output
	rowOutput              *text.Output
	detailOutput           *text.Output
	footerOutput           *text.Output
	moonstoneRowOutput     *text.Output
	moonstoneHeadingOutput *text.Output
	layout                 questJournalLayout
	frameRects             []image.Rectangle
	lastConversation       journal.Conversation
	excerpts               []journal.Exchange
	visibleExcerpts        []int
	showArchived           bool
	keepExcerpt            func(journal.Exchange) ([]journal.Exchange, error)
	deleteExcerpt          func(string) ([]journal.Exchange, error)
	archiveExcerpt         func(string, bool) ([]journal.Exchange, error)
	excerptStatus          string
	deletePending          bool
	passageRow             int
	exchangeOutput         *text.Output
	conversationRows       []int
	runicReferenceOpen     bool
	runicReference         RunicReferencePanel
	notesView              notesState
	tabs                   []QuestJournalTab

	lastScreenWidth  int
	lastScreenHeight int
	lastFooterLines  int
}

// NewQuestJournalDialog creates a modal over a detached journal dossier.
func NewQuestJournalDialog(
	dossier game_state.QuestJournalDossier,
	config QuestJournalDialogConfig,
) *QuestJournalDialog {
	if config.Keyboard == nil {
		panic("QuestJournalDialog: keyboard is required")
	}
	if config.OnDismiss == nil {
		panic("QuestJournalDialog: OnDismiss is required")
	}

	placement := config.Placement
	if questJournalPlacementIsZero(placement) {
		placement = questJournalDefaultPlacement()
	}
	if placement.EndPercentX <= placement.StartPercentX || placement.EndPercentY <= placement.StartPercentY {
		panic("QuestJournalDialog: placement must have positive width and height")
	}

	//nolint:exhaustruct_v5 // Outputs and responsive layout are initialized by ensureLayout.
	dialog := &QuestJournalDialog{
		navigation:       newNavigation(config.Keyboard.IsKeyHeld),
		dossier:          cloneQuestJournalDossier(dossier),
		keyboard:         config.Keyboard,
		placement:        placement,
		onDismiss:        config.OnDismiss,
		activeTab:        QuestJournalChronicleTab,
		focused:          true,
		selections:       [questJournalTabCount]int{},
		shadowlordSprite: config.ShadowlordSprite,
		lastConversation: config.LastConversation.Clone(),
		excerpts:         cloneJournalExchanges(config.Excerpts),
		keepExcerpt:      config.KeepExcerpt,
		deleteExcerpt:    config.DeleteExcerpt,
		archiveExcerpt:   config.ArchiveExcerpt,
		excerptStatus:    config.ExcerptError,
		visitedTabs:      [3]bool{true, false, false},
	}
	for tab := range dialog.newEntries {
		dialog.newEntries[tab] = append([]bool(nil), config.NewEntries[tab]...)
	}
	dialog.notesView.notes, dialog.notesView.onIntent = config.Notes.Normalized(), config.OnNotesIntent
	for tab := QuestJournalChronicleTab; tab < questJournalTabCount; tab++ {
		dialog.tabs = append(dialog.tabs, tab)
	}
	dialog.rebuildNoteRows()
	dialog.refreshVisibleExcerpts("")
	dialog.clampAllSelections()
	dialog.ensureLayout()
	dialog.buildSearchEntries()

	return dialog
}

// Update processes one keyboard action while the journal owns modal focus.
func (d *QuestJournalDialog) Update() {
	if d.dismissed || !d.focused {
		return
	}
	d.ensureLayout()

	if d.handleRunicReferenceInput() {
		return
	}
	if d.handleExcerptInput() || d.handleNotesInput() {
		return
	}
	if d.handleDismissInput() {
		return
	}
	if d.searching {
		d.handleSearchInput()

		return
	}
	if d.handleTabInput() {
		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeySlash) {
		d.beginSearch()

		return
	}
	d.handleBrowseInput()
}

func (d *QuestJournalDialog) handleDismissInput() bool {
	shiftPressed := d.keyboard.IsModifierPressed(input.ModifierShift)
	escape := questJournalDismissShortcut(ebiten.KeyEscape, shiftPressed) &&
		d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape)
	toggle := questJournalDismissShortcut(ebiten.KeyJ, shiftPressed) &&
		d.keyboard.IsKeyPressedAndRegister(ebiten.KeyJ) ||
		d.keyboard.IsKeyPressedAndRegister(QuestJournalFunctionKey)
	if !escape && !toggle {
		return false
	}
	if escape && !toggle && d.searching {
		d.leaveSearch()

		return true
	}
	d.Dismiss()

	return true
}

func (d *QuestJournalDialog) handleTabInput() bool {
	if !d.keyboard.IsKeyPressedAndRegister(ebiten.KeyTab) {
		return false
	}
	d.switchTabBy(questJournalTabDelta(d.keyboard.IsModifierPressed(input.ModifierShift)))

	return true
}

func (d *QuestJournalDialog) handleBrowseInput() {
	if d.handleNavigationInput() {
		return
	}
	if !d.keyboard.IsKeyPressedAndRegister(ebiten.KeySpace) {
		return
	}
	if d.isConversationTab() {
		if d.keyboard.IsModifierPressed(input.ModifierCtrl) || d.keyboard.IsModifierPressed(input.ModifierAlt) ||
			d.keyboard.IsModifierPressed(input.ModifierMeta) {
			return
		}
		d.pageSelectedExchange(questJournalTabDelta(d.keyboard.IsModifierPressed(input.ModifierShift)))

		return
	}
	pageCount := d.GetDetailPageCount()
	if pageCount > 1 {
		d.detailPage = (d.detailPage + 1) % pageCount
	}
}

// Draw renders the dimmed surround and journal panels.
func (d *QuestJournalDialog) Draw(screen *ebiten.Image) {
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
	fillClassicPaneBackgrounds(screen, d.frameRects, d.pixelUnit(), MerchantDialogBackgroundColor)
	d.drawHeader(screen)
	switch {
	case d.searching:
		d.drawSearch(screen)
	case d.isConversationTab():
		if d.runicReferenceOpen {
			d.runicReference.Draw(screen, d.layout.listRect, d.font, d.rowOutput.GetLineSpacing())
		} else {
			d.drawList(screen)
		}
		d.drawExchange(screen)
	default:
		d.drawList(screen)
		d.drawDetail(screen)
	}
	d.drawFooter(screen)
	drawClassicPaneFrames(screen, d.frameRects, d.pixelUnit(), DialogRuleColor)
}

// Dismiss closes the journal and invokes its callback at most once.
func (d *QuestJournalDialog) Dismiss() {
	if d.dismissed {
		return
	}
	d.dismissed = true
	d.onDismiss()
}

// IsDismissed reports whether the journal has closed.
func (d *QuestJournalDialog) IsDismissed() bool { return d.dismissed }

func (d *QuestJournalDialog) onDialogFocusChanged(focused bool) {
	d.focused = focused
	if focused {
		d.keyboard.PrepareForTransition(0)
	}
}

// VisitedTabs returns the curated tabs actually opened during this session.
// The scene acknowledges these only through its captured opening revision.
func (d *QuestJournalDialog) VisitedTabs() [3]bool { return d.visitedTabs }

// GetDetailPageCount returns the page count calculated from current panel capacity.
func (d *QuestJournalDialog) GetDetailPageCount() int {
	d.ensureLayout()
	if d.isConversationTab() {
		d.prepareExchangeReader()

		return d.exchangePageCount()
	}

	return len(d.detailPages())
}

func (d *QuestJournalDialog) switchTabBy(delta int) {
	d.Reset()
	count := len(d.tabs)
	next := (max(0, slices.Index(d.tabs, d.activeTab)) + delta%count + count) % count
	d.activeTab = d.tabs[next]
	d.clampCurrentSelection()
	d.detailPage = 0
	d.enterTab()
}

func (d *QuestJournalDialog) enterTab() {
	d.readingQuery = ""
	d.runicReferenceOpen = false
	d.ensureLayout()
	if d.activeTab < QuestJournalDungeonsTab {
		d.visitedTabs[d.activeTab] = true
	}
	d.passageRow = 0
	d.exchangeOutput = nil
	d.cachedDetailPages = nil
	d.rebuildFrames()
}

func (d *QuestJournalDialog) moveSelection(delta int) {
	count := d.currentItemCount()
	if count == 0 {
		return
	}
	selection := max(0, min(d.selections[d.activeTab]+delta, count-1))
	if selection == d.selections[d.activeTab] {
		return
	}
	d.selections[d.activeTab] = selection
	d.notesView.status = ""
	d.readingQuery = ""
	d.runicReferenceOpen = false
	d.detailPage = 0
	d.passageRow = 0
	d.exchangeOutput = nil
	d.cachedDetailPages = nil
}

func (d *QuestJournalDialog) clampAllSelections() {
	active := d.activeTab
	for tab := QuestJournalChronicleTab; tab < questJournalTabCount; tab++ {
		d.activeTab = tab
		d.clampCurrentSelection()
	}
	d.activeTab = active
}

func (d *QuestJournalDialog) clampCurrentSelection() {
	count := d.currentItemCount()
	if count == 0 {
		d.selections[d.activeTab] = 0

		return
	}
	d.selections[d.activeTab] = max(0, min(d.selections[d.activeTab], count-1))
}

func (d *QuestJournalDialog) currentItemCount() int {
	switch d.activeTab {
	case QuestJournalChronicleTab:
		return len(d.dossier.Chronicle)
	case QuestJournalVirtuesTab:
		return len(d.dossier.Virtues)
	case QuestJournalShadowlordsTab:
		return len(d.dossier.Shadowlords)
	case QuestJournalConversationTab:
		return len(d.lastConversation.Exchanges)
	case QuestJournalExcerptsTab:
		return len(d.visibleExcerpts)
	case QuestJournalDungeonsTab:
		return len(d.dossier.Dungeons)
	case QuestJournalMoonstonesTab:
		return len(d.dossier.Moonstones)
	case QuestJournalNotesTab:
		return len(d.notesView.rows)
	default:
		return 0
	}
}

func (d *QuestJournalDialog) ensureLayout() {
	screenWidth, screenHeight := display.GetManager().GetCurrentSize()
	if screenWidth <= 0 || screenHeight <= 0 {
		return
	}
	if d.outerBorder != nil && screenWidth == d.lastScreenWidth && screenHeight == d.lastScreenHeight &&
		d.lastFooterLines == d.footerLineCount() {
		return
	}

	previousPage := d.detailPage
	d.lastScreenWidth = screenWidth
	d.lastScreenHeight = screenHeight
	d.lastFooterLines = d.footerLineCount()
	d.outerBorder = NewOpaqueBorder(d.placement, BorderWidthStandard, MerchantDialogBackgroundColor)
	d.rebuildOutputs()
	d.calculateLayout()
	d.cachedDetailPages = nil
	d.clampAllSelections()
	if d.isConversationTab() {
		d.detailPage = previousPage
		d.prepareExchangeReader()
	} else {
		pageCount := len(d.detailPages())
		d.detailPage = min(previousPage, max(0, pageCount-1))
	}
	d.buildSearchEntries()
}

func (d *QuestJournalDialog) rebuildOutputs() {
	d.exchangeOutput = nil
	config := FontConfig{
		Size:                  FontSizeBody,
		LineSpacing:           0,
		LineSpacingMultiplier: questJournalLineSpacingMultiplier,
		MaxLines:              1,
		MaxChars:              questJournalMinimumTextChars,
	}
	d.font, d.rowOutput = GetScaledFontWithOutput(config)
	_, d.tabOutput = GetScaledFontWithOutput(config)
	config.Size = FontSizeTitle
	_, d.titleOutput = GetScaledFontWithOutput(config)
	config.Size = FontSizeBody
	config.MaxLines = questJournalDetailMaxLines
	_, d.detailOutput = GetScaledFontWithOutput(config)
	config.MaxLines = d.footerLineCount()
	_, d.footerOutput = GetScaledFontWithOutput(config)
}

func (d *QuestJournalDialog) calculateLayout() {
	interior := d.outerBorder.GetInteriorBounds()
	padding := max(questJournalMinimumPadding, min(interior.Dx(), interior.Dy())/questJournalPaddingDivisor)
	content := insetRectangle(interior, padding)
	lineHeight := int(math.Ceil(d.rowOutput.GetLineSpacing()))
	titleHeight := int(math.Ceil(d.titleOutput.GetFontHeight()))
	headerHeight := max(1, titleHeight+lineHeight*2+padding*3)
	footerHeight := max(1, lineHeight*d.footerLineCount()+padding*2)
	headerHeight = min(headerHeight, max(1, content.Dy()/questJournalVerticalRegionDivisor))
	footerHeight = min(footerHeight, max(1, content.Dy()/questJournalVerticalRegionDivisor))

	d.layout = questJournalLayout{ //nolint:exhaustruct_v5 // Remaining rectangles are calculated below.
		panelRect:  interior,
		headerRect: image.Rect(content.Min.X, content.Min.Y, content.Max.X, content.Min.Y+headerHeight),
		footerRect: image.Rect(content.Min.X, content.Max.Y-footerHeight, content.Max.X, content.Max.Y),
		padding:    padding,
	}
	d.layout.bodyRect = image.Rect(
		content.Min.X,
		d.layout.headerRect.Max.Y+padding,
		content.Max.X,
		d.layout.footerRect.Min.Y-padding,
	)
	d.layout.titleRect = image.Rect(
		d.layout.headerRect.Min.X+padding,
		d.layout.headerRect.Min.Y+padding,
		d.layout.headerRect.Max.X-padding,
		min(d.layout.headerRect.Max.Y, d.layout.headerRect.Min.Y+padding+titleHeight),
	)
	d.layout.tabsRect = image.Rect(
		d.layout.headerRect.Min.X+padding,
		d.layout.titleRect.Max.Y,
		d.layout.headerRect.Max.X-padding,
		d.layout.headerRect.Max.Y-padding,
	)

	usableWidth := max(1, d.layout.bodyRect.Dx()-padding)
	listWidth := max(1, int(math.Round(float64(usableWidth)*questJournalListRatio)))
	d.layout.listRect = image.Rect(
		d.layout.bodyRect.Min.X,
		d.layout.bodyRect.Min.Y,
		d.layout.bodyRect.Min.X+listWidth,
		d.layout.bodyRect.Max.Y,
	)
	d.layout.detailRect = image.Rect(
		d.layout.listRect.Max.X+padding,
		d.layout.bodyRect.Min.Y,
		d.layout.bodyRect.Max.X,
		d.layout.bodyRect.Max.Y,
	)
	d.rebuildFrames()
	d.calculateTabs()
	d.rebuildMoonstoneOutputs()

	d.titleOutput.SetMaxCharsPerLine(len("Quest Journal"))
	d.tabOutput.SetMaxCharsPerLine(max(1, d.font.CharsForWidth(d.layout.tabRects[0].Dx())))
	d.footerOutput.SetMaxCharsPerLine(max(1, d.font.CharsForWidth(d.footerContentRect().Dx())))
}

func (d *QuestJournalDialog) drawHeader(screen *ebiten.Image) {
	d.titleOutput.Reset()
	d.titleOutput.AppendLine().WithAlignment(etext.AlignCenter).Text("Quest Journal", DialogHeaderColor).Done()
	d.titleOutput.DrawPanelContent(screen, d.layout.titleRect.Min, d.layout.titleRect.Dx(), etext.AlignCenter)

	for _, index := range d.tabs {
		rect := d.layout.tabRects[index]
		textColor := DialogEmptyColor
		if index == d.activeTab && !d.searching {
			textColor = pkgcolor.EGAYellow
		}
		d.tabOutput.SetMaxCharsPerLine(len(d.tabTitle(index)))
		d.tabOutput.Reset()
		builder := d.tabOutput.AppendLine().
			WithAlignment(etext.AlignCenter).
			Text(questJournalTabTitle(index), textColor)
		if d.tabIsNew(index) {
			builder.Text(" *", DialogLabelColor)
		}
		builder.Done()
		top := rect.Min.Y + max(0, (rect.Dy()-int(math.Ceil(d.tabOutput.GetFontHeight())))/questJournalCenterDivisor)
		d.tabOutput.DrawPanelContent(screen, image.Pt(rect.Min.X, top), rect.Dx(), etext.AlignCenter)
	}
}

func (d *QuestJournalDialog) drawList(screen *ebiten.Image) {
	if d.activeTab == QuestJournalNotesTab && d.notesView.moving {
		d.drawNotesChooser(screen)

		return
	}
	content := d.listContentRect()
	count := d.currentItemCount()
	if count == 0 {
		drawMasterDetailCenteredLine(
			screen, content, d.emptyListText(), DialogEmptyColor, d.font, d.rowOutput,
		)

		return
	}

	rowHeight := max(1, int(math.Ceil(d.rowOutput.GetLineSpacing())))
	visibleRows := max(1, content.Dy()/rowHeight)
	selection := d.selections[d.activeTab]
	start, end := zStatsVisibleRange(selection, count, visibleRows)
	for index := start; index < end; index++ {
		rowTop := content.Min.Y + (index-start)*rowHeight
		rowRect := image.Rect(content.Min.X, rowTop, content.Max.X, min(content.Max.Y, rowTop+rowHeight))
		d.drawListRow(screen, rowRect, index, index == selection)
	}
	d.drawScrollIndicators(screen, content, start, end, count, rowHeight)
}

func (d *QuestJournalDialog) drawListRow(screen *ebiten.Image, rect image.Rectangle, index int, selected bool) {
	textColor, background := d.journalRowColors(d.activeTab, index, selected)
	if selected {
		vector.FillRect(
			screen,
			float32(rect.Min.X),
			float32(rect.Min.Y),
			float32(rect.Dx()),
			float32(rect.Dy()),
			background,
			false,
		)
	}

	if d.activeTab == QuestJournalMoonstonesTab {
		d.drawMoonstoneRow(screen, rect, index, selected, textColor)
	} else {
		d.drawEntryTitle(screen, rect, d.listRowText(index), d.entryIsNew(d.activeTab, index), textColor)
	}
}

func (d *QuestJournalDialog) drawScrollIndicators(
	screen *ebiten.Image,
	content image.Rectangle,
	start int,
	end int,
	count int,
	rowHeight int,
) {
	indicatorWidth := max(
		d.layout.padding*questJournalPaneInsetMultiplier,
		int(math.Ceil(d.font.TextFace.Size)),
	)
	drawMasterDetailScrollIndicators(
		screen, content, start, end, count, rowHeight, indicatorWidth, d.font, d.rowOutput,
	)
}

func (d *QuestJournalDialog) drawDetail(screen *ebiten.Image) {
	if d.activeTab == QuestJournalMoonstonesTab {
		d.drawMoonstoneHeading(screen)
	}
	content := d.detailContentRect()
	pages := d.detailPages()
	if d.detailPage >= len(pages) {
		d.detailPage = max(0, len(pages)-1)
	}

	d.detailOutput.SetMaxCharsPerLine(max(1, d.font.CharsForWidth(content.Dx())))
	d.detailOutput.Reset()
	for _, line := range pages[d.detailPage] {
		d.appendDetailLine(line)
	}
	d.detailOutput.DrawPanelContent(screen, content.Min, content.Dx(), etext.AlignStart)
	if d.activeTab == QuestJournalShadowlordsTab && d.detailPage == 0 {
		d.drawShadowlordSprite(screen)
	}
}

func (d *QuestJournalDialog) drawFooter(screen *ebiten.Image) {
	content := d.footerContentRect()
	drawModifierFooter(screen, content, d.footerGuidance(), d.footerOutput)
}

func (d *QuestJournalDialog) footerLineCount() int {
	lines := questJournalFooterLines
	if d.activeTab == QuestJournalExcerptsTab || d.activeTab == QuestJournalNotesTab {
		// Archive and notebook management need one extra row without hiding navigation hints.
		lines++
	}
	if d.hasRunicExchange() {
		lines++
	}

	return lines
}

func (d *QuestJournalDialog) detailPages() [][]detailLine {
	if d.cachedDetailPages == nil {
		d.cachedDetailPages = d.buildDetailPages()
	}

	return d.cachedDetailPages
}

func (d *QuestJournalDialog) buildDetailPages() [][]detailLine {
	content := d.detailContentRect()
	maxChars := max(1, d.font.CharsForWidth(max(1, content.Dx())))
	lineCapacity := max(1, d.detailOutput.GetLineCapacity(max(1, content.Dy())))
	d.detailOutput.SetMaxCharsPerLine(maxChars)
	lines := d.detailLines()
	if len(lines) == 0 {
		return [][]detailLine{{{
			label: "", value: "No entry selected.", color: DialogEmptyColor, header: false,
		}}}
	}

	pages := make([][]detailLine, 0, 1)
	page := make([]detailLine, 0, lineCapacity)
	for len(lines) > 0 {
		candidate := append(append([]detailLine(nil), page...), lines[0])
		if d.detailLineCount(candidate) <= lineCapacity {
			page = candidate
			lines = lines[1:]

			continue
		}
		if len(page) > 0 {
			pages = append(pages, page)
			page = nil

			continue
		}

		first, remainder := splitDetailLine(lines[0], lineCapacity, d.detailLineCount)
		pages = append(pages, []detailLine{first})
		if remainder == nil {
			lines = lines[1:]
		} else {
			lines[0] = *remainder
		}
	}
	if len(page) > 0 {
		pages = append(pages, page)
	}
	if len(pages) == 0 {
		return [][]detailLine{{}}
	}

	return pages
}

func (d *QuestJournalDialog) detailLineCount(lines []detailLine) int {
	d.detailOutput.Reset()
	for _, line := range lines {
		d.appendDetailLine(line)
	}

	return d.detailOutput.GetPanelContentLineCount()
}

func (d *QuestJournalDialog) appendDetailLine(line detailLine) {
	builder := d.detailOutput.AppendLine()
	query := d.readingQuery
	if line.value == questJournalNotLearned {
		query = ""
	}
	if line.header {
		appendJournalMatches(builder, line.value, query, DialogHeaderColor, false)
		builder.WordWrap().Done()

		return
	}
	if line.label != "" {
		builder.Text(strings.TrimSpace(line.label)+": ", DialogLabelColor)
	}
	appendJournalMatches(builder, line.value, query, line.color, false)
	builder.WordWrap().Done()
}

func (d *QuestJournalDialog) detailLines() []detailLine {
	if d.currentItemCount() == 0 {
		return nil
	}
	selection := d.selections[d.activeTab]

	switch d.activeTab {
	case QuestJournalChronicleTab:
		return questJournalChronicleDetail(d.dossier.Chronicle[selection])
	case QuestJournalVirtuesTab:
		return questJournalVirtueDetail(d.dossier.Virtues[selection])
	case QuestJournalShadowlordsTab:
		return questJournalShadowlordDetail(d.dossier.Shadowlords[selection])
	case QuestJournalDungeonsTab:
		return questJournalDungeonDetail(d.dossier.Dungeons[selection])
	case QuestJournalMoonstonesTab:
		return questJournalMoonstoneDetail(d.dossier.Moonstones[selection])
	case QuestJournalConversationTab, QuestJournalExcerptsTab:
		return nil // Conversation readers own their measured exchange output.
	case QuestJournalNotesTab:
		return d.notesDetailLines()
	default:
		return nil
	}
}

func (d *QuestJournalDialog) listRowText(index int) string {
	switch d.activeTab {
	case QuestJournalChronicleTab:
		return questJournalEventTitle(d.dossier.Chronicle[index])
	case QuestJournalVirtuesTab:
		return d.dossier.Virtues[index].Name
	case QuestJournalShadowlordsTab:
		return d.dossier.Shadowlords[index].DisplayName
	case QuestJournalConversationTab:
		exchange := d.lastConversation.Exchanges[index]
		title := exchange.Title()
		if d.exchangeKept(exchange.ID) {
			title += " [Kept]"
		}

		return title
	case QuestJournalExcerptsTab:
		return excerptTitle(d.excerpts[d.visibleExcerpts[index]])
	case QuestJournalDungeonsTab:
		return d.dossier.Dungeons[index].Name
	case QuestJournalMoonstonesTab:
		return d.dossier.Moonstones[index].Phase.String()
	case QuestJournalNotesTab:
		return d.notesRowText(index)
	default:
		return ""
	}
}

func (d *QuestJournalDialog) listContentRect() image.Rectangle {
	return insetRectangle(d.layout.listRect, d.layout.padding*questJournalPaneInsetMultiplier)
}

func (d *QuestJournalDialog) detailContentRect() image.Rectangle {
	content := insetRectangle(d.layout.detailRect, d.layout.padding*questJournalPaneInsetMultiplier)
	if d.activeTab == QuestJournalMoonstonesTab && d.moonstoneHeadingOutput != nil {
		content.Min.Y += int(math.Ceil(d.moonstoneHeadingOutput.GetLineSpacing())) + d.layout.padding
	}
	if sprite := d.shadowlordSpriteRect(); !sprite.Empty() {
		content.Max.X = sprite.Min.X - d.layout.padding
	}

	return content
}

func (d *QuestJournalDialog) footerContentRect() image.Rectangle {
	return insetRectangle(d.layout.footerRect, d.layout.padding)
}

func (d *QuestJournalDialog) pixelUnit() int {
	return getClassicUIPixelUnit(d.lastScreenWidth, d.lastScreenHeight)
}

func questJournalTabTitle(tab QuestJournalTab) string {
	switch tab {
	case QuestJournalChronicleTab:
		return "Chronicle"
	case QuestJournalVirtuesTab:
		return "Virtues"
	case QuestJournalShadowlordsTab:
		return "Shadowlords"
	case QuestJournalDungeonsTab:
		return "Dungeons"
	case QuestJournalConversationTab:
		return "Last Conversation"
	case QuestJournalExcerptsTab:
		return "Excerpts"
	case QuestJournalMoonstonesTab:
		return "Moonstones"
	case QuestJournalNotesTab:
		return "My Notes"
	default:
		return ""
	}
}

func questJournalTabDelta(shiftPressed bool) int {
	if shiftPressed {
		return -1
	}

	return 1
}

// QuestJournalFunctionKey opens and closes the journal exactly like Shift+J.
const QuestJournalFunctionKey = ebiten.KeyF5

func questJournalDismissShortcut(key ebiten.Key, shiftPressed bool) bool {
	return key == ebiten.KeyEscape || key == ebiten.KeyJ && shiftPressed
}

func questJournalChronicleDetail(event game_state.QuestJournalChronicleEntry) []detailLine {
	if event.Title != "" {
		lines := []detailLine{
			{label: "", value: event.Title, color: DialogHeaderColor, header: true},
			{label: "", value: event.Summary, color: DialogValueColor, header: false},
		}
		if event.Source != "" {
			lines = append(lines, detailLine{
				label: "Source", value: event.Source, color: DialogValueColor, header: false,
			})
		}
		if event.OccurredOn != nil {
			lines = append(lines, detailLine{
				label: "Date", color: DialogValueColor, header: false,
				value: fmt.Sprintf("%d-%d-%d", event.OccurredOn.Month, event.OccurredOn.Day, event.OccurredOn.Year),
			})
		}

		return lines
	}
	title := event.DisplayName
	defeated := "The Shadowlord " + title + " was permanently defeated."
	if strings.Contains(title, "Shadowlord") {
		defeated = title + " was permanently defeated."
	}
	lines := []detailLine{
		{label: "", value: title, color: DialogHeaderColor, header: true},
		{
			label: "", value: defeated,
			color: DialogValueColor, header: false,
		},
	}
	if event.OccurredOn == nil {
		return append(lines, detailLine{
			label: "", value: "Previously defeated; date unknown",
			color: DialogNavHintColor, header: false,
		})
	}
	lines = append(lines, detailLine{
		label: "Date",
		value: fmt.Sprintf(
			"%d-%d-%d",
			event.OccurredOn.Month,
			event.OccurredOn.Day,
			event.OccurredOn.Year,
		),
		color: DialogValueColor, header: false,
	})

	return lines
}

func questJournalVirtueDetail(virtue game_state.QuestJournalVirtue) []detailLine {
	state, stateColor := questJournalVirtueStateText(virtue.QuestState)
	answer := questJournalNotLearned
	answerColor := DialogEmptyColor
	if virtue.CodexAnswerLearned {
		answer = virtue.CodexAnswer
		answerColor = DialogValueColor
	}
	mantra := questJournalNotLearned
	mantraColor := DialogEmptyColor
	if virtue.MantraLearned {
		mantra = virtue.Mantra
		mantraColor = DialogValueColor
	}

	lines := []detailLine{
		{label: "", value: virtue.Name, color: DialogHeaderColor, header: true},
		{label: "Shrine quest", value: state, color: stateColor, header: false},
	}
	if virtue.Ruined {
		lines = append(lines, detailLine{
			label: "Shrine", value: "Destroyed", color: DialogStatusWarningColor, header: false,
		})
	}

	return append(lines,
		detailLine{label: "Mantra", value: mantra, color: mantraColor, header: false},
		detailLine{label: "Codex answer", value: answer, color: answerColor, header: false},
	)
}

func questJournalShadowlordDetail(shadowlord game_state.QuestJournalShadowlord) []detailLine {
	state := "Defeated"
	stateColor := DialogStatusOKColor
	if shadowlord.State == game_state.QuestJournalShadowlordLiving {
		state = "Living"
		stateColor = DialogStatusWarningColor
	}

	return []detailLine{
		{label: "", value: shadowlord.DisplayName, color: DialogHeaderColor, header: true},
		questJournalLearnedField("Embodies", shadowlord.Association),
		questJournalLearnedField("Opposes", shadowlord.Opposes),
		{label: "", value: "", color: DialogValueColor, header: false},
		questJournalLearnedField("Flame", shadowlord.Flame),
		questJournalLearnedField("Location", shadowlord.Location),
		{label: "", value: "", color: DialogValueColor, header: false},
		{label: questJournalStateLabel, value: state, color: stateColor, header: false},
	}
}

func questJournalLearnedField(label, value string) detailLine {
	fieldColor := DialogValueColor
	if value == "" {
		value, fieldColor = questJournalNotLearned, DialogEmptyColor
	}

	return detailLine{label: label, value: value, color: fieldColor, header: false}
}

func questJournalDungeonDetail(entry journal.DungeonEntry) []detailLine {
	if entry.Gate {
		return questJournalGateDetail(entry)
	}
	lines := []detailLine{
		questJournalLearnedField("Dungeon", entry.Name),
		questJournalLearnedField("Entrance", entry.Entrance),
	}
	if entry.Location != "" {
		lines = append(lines, questJournalLearnedField("Location", entry.Location))
	}
	if entry.Word != "" {
		label := "Word of Power"
		if entry.WordIsClue {
			label = "Spoken clue"
		}
		lines = append(lines, questJournalLearnedField(label, entry.Word))
		if entry.Source != "" {
			lines = append(lines, questJournalLearnedField("Source", entry.Source))
		}
	}

	return lines
}

// questJournalGateDetail presents a learned clue without entrance or location
// fields: the gate has no observed site, so those would assert false facts.
func questJournalGateDetail(entry journal.DungeonEntry) []detailLine {
	lines := []detailLine{
		{label: "", value: entry.Name, color: DialogHeaderColor, header: true},
		questJournalLearnedField("Word of Power", entry.Word),
		questJournalLearnedField("Source", entry.Source),
	}
	// Separate paragraphs so paging can break between them.
	for paragraph := range strings.SplitSeq(entry.Description, "\n\n") {
		lines = append(lines,
			detailLine{label: "", value: "", color: DialogValueColor, header: false},
			detailLine{label: "", value: paragraph, color: DialogValueColor, header: false})
	}

	return lines
}

func questJournalVirtueStateText(state game_state.ShrineProgressState) (string, stdcolor.Color) {
	switch state {
	case game_state.ShrineProgressNotAssigned:
		return "Not started", DialogEmptyColor
	case game_state.ShrineProgressAssigned:
		return "In progress", DialogLabelColor
	case game_state.ShrineProgressReadyToComplete:
		return "Ready to complete", DialogStatusOKColor
	case game_state.ShrineProgressOffering:
		return "Complete", DialogValueColor
	default:
		panic("QuestJournalDialog: unsupported shrine progress state")
	}
}

func questJournalEventTitle(event game_state.QuestJournalChronicleEntry) string {
	if event.Title != "" {
		return event.Title
	}

	return event.DisplayName + " defeated"
}

func cloneQuestJournalDossier(dossier game_state.QuestJournalDossier) game_state.QuestJournalDossier {
	cloned := dossier
	cloned.Chronicle = make([]game_state.QuestJournalChronicleEntry, len(dossier.Chronicle))
	for index, event := range dossier.Chronicle {
		cloned.Chronicle[index] = event
		if event.OccurredOn != nil {
			date := *event.OccurredOn
			cloned.Chronicle[index].OccurredOn = &date
		}
	}
	cloned.Virtues = append([]game_state.QuestJournalVirtue(nil), dossier.Virtues...)
	cloned.Shadowlords = append([]game_state.QuestJournalShadowlord(nil), dossier.Shadowlords...)
	cloned.Dungeons = append([]journal.DungeonEntry(nil), dossier.Dungeons...)

	return cloned
}

func questJournalPlacementIsZero(placement sprites.PercentBasedPlacement) bool {
	return placement.StartPercentX == 0 && placement.EndPercentX == 0 &&
		placement.StartPercentY == 0 && placement.EndPercentY == 0
}

func questJournalDefaultPlacement() sprites.PercentBasedPlacement {
	return sprites.PercentBasedPlacement{
		StartPercentX: questJournalMarginX,
		EndPercentX:   1 - questJournalMarginX,
		StartPercentY: questJournalMarginY,
		EndPercentY:   1 - questJournalMarginY,
	}
}
