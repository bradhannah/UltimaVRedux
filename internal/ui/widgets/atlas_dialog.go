package widgets

import (
	"fmt"
	"image"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
)

// AtlasDialog borrows the application texture. Closing never disposes geography.
type AtlasDialog struct {
	navigation
	keyboard                                   *input.Keyboard
	dossier                                    journal.AtlasDossier
	entries                                    []journal.AtlasEntry
	view                                       *AtlasViewState
	texture                                    *ebiten.Image
	onDismiss                                  func()
	onNoteIntent                               func(AtlasNoteIntent)
	focused, closed, panning                   bool
	selection, detailPage                      int
	status                                     string
	width, height, rowHeight                   int
	mapRect, listRect, detailRect, footerRect  image.Rectangle
	placesHeaderRect, positionRect, statusRect image.Rectangle
	font                                       *text.UltimaFont
	header, row, details, footer, label        *text.Output
	panelHeader, position, zoom                *text.Output
	tabs                                       [journal.AtlasPlaceFilterCount]image.Rectangle
	tabText                                    *text.Output
	lighting                                   AtlasLighting
	lightPool                                  *ebiten.Image
	lightPoolCore                              float32
	lightFrames                                int64
}

var _ Widget = (*AtlasDialog)(nil)

// NewAtlasDialog creates a detached, keyboard-only reader with scene-owned settings.
func NewAtlasDialog(dossier journal.AtlasDossier, texture *ebiten.Image, keyboard *input.Keyboard,
	view *AtlasViewState, onDismiss func()) *AtlasDialog {
	if keyboard == nil || view == nil || onDismiss == nil {
		panic("AtlasDialog requires keyboard, view state and dismissal owner")
	}
	if !view.Initialized {
		*view = NewAtlasViewState()
	}
	//nolint:exhaustruct_v5 // Refresh and ensureLayout initialize detached data and measured panels.
	d := &AtlasDialog{navigation: newNavigation(keyboard.IsKeyHeld), keyboard: keyboard,
		view: view, texture: texture, onDismiss: onDismiss, focused: true, lighting: AtlasDaylight()}
	d.Refresh(dossier)
	d.ensureLayout()

	return d
}

// Refresh discards a stale fix without changing the manually chosen camera.
func (d *AtlasDialog) Refresh(dossier journal.AtlasDossier) {
	dossier.Entries = append([]journal.AtlasEntry(nil), dossier.Entries...)
	dossier.Symbols = append([]journal.AtlasSymbol(nil), dossier.Symbols...)
	d.dossier = dossier
	d.refilter()
}

func (d *AtlasDialog) refilter() {
	oldSelection := d.selection
	d.entries = journal.FilterAtlasEntries(d.dossier.Entries, d.view.Filters)
	d.selection = max(0, min(oldSelection, len(d.entries)-1))
	if (d.view.Overview || len(d.entries) == 0) && d.offersOverview() {
		d.view.Overview = true
		d.selection = len(d.entries)

		return
	}
	d.view.Overview = false
	for i, e := range d.entries {
		if e.Key == d.view.SelectedKey {
			d.selection = i

			break
		}
	}
	if len(d.entries) > 0 {
		d.view.SelectedKey = d.entries[d.selection].Key
	} else {
		d.view.SelectedKey = ""
	}
}

func (d *AtlasDialog) onDialogFocusChanged(focused bool) {
	d.focused = focused
	d.navigation.onDialogFocusChanged(focused)
}

// AtlasFunctionKey opens and closes the map exactly like Shift+M.
const AtlasFunctionKey = ebiten.KeyF6

// IsAtlasChord is exact: extra Ctrl/Alt/Meta modifiers retain their own routing.
func IsAtlasChord(keyboard *input.Keyboard) bool {
	return keyboard.IsModifierComboPressed(input.ModifierShift, ebiten.KeyM) &&
		!keyboard.IsModifierPressed(input.ModifierCtrl) && !keyboard.IsModifierPressed(input.ModifierAlt) &&
		!keyboard.IsModifierPressed(input.ModifierMeta)
}

// Update handles reader input without acquiring or modifying gameplay knowledge.
func (d *AtlasDialog) Update() {
	d.lightFrames++
	if d.closed || !d.focused {
		return
	}
	d.ensureLayout()
	if IsAtlasChord(d.keyboard) && d.keyboard.IsKeyPressedAndRegister(ebiten.KeyM) ||
		d.keyboard.IsKeyPressedAndRegister(AtlasFunctionKey) {
		d.close()

		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.close()

		return
	}
	// A released category tap is immediate; gameplay repeat delays must not
	// swallow quick Tab/Shift+Tab presses. Held keys remain consumed until release.
	if d.keyboard.IsTextKeyPressedAndRegister(ebiten.KeyTab) {
		d.handleKey(ebiten.KeyTab)

		return
	}
	if d.keyboard.IsTextKeyPressedAndRegister(ebiten.KeyP) {
		d.handleKey(ebiten.KeyP)

		return
	}
	if key, owned := d.Poll(d.keyboard, ebiten.KeyUp, ebiten.KeyDown, ebiten.KeyLeft, ebiten.KeyRight); owned {
		if key != nil {
			d.navigate(*key)
		}

		return
	}
	for _, key := range []ebiten.Key{ebiten.KeyDigit0, ebiten.KeyNumpad0, ebiten.KeyEqual, ebiten.KeyMinus,
		ebiten.KeyNumpadAdd, ebiten.KeyNumpadSubtract, ebiten.KeyEnter, ebiten.KeyNumpadEnter, ebiten.KeyL,
		ebiten.KeyG, ebiten.KeyA, ebiten.KeyPageUp, ebiten.KeyPageDown, ebiten.KeySpace,
		ebiten.KeyN, ebiten.KeyE, ebiten.KeyD} {
		if d.keyboard.IsTextKeyPressedAndRegister(key) {
			d.handleKey(key)

			return
		}
	}
}

func (d *AtlasDialog) close() {
	if !d.closed {
		d.closed = true
		d.onDismiss()
	}
}

func (d *AtlasDialog) navigate(key ebiten.Key) {
	d.status = ""
	if d.panning {
		d.panMap(key)

		return
	}
	previous := d.selection
	capacity := max(1, d.listRect.Dy()/d.directoryRowHeight())
	d.selection = max(0, min(d.presentationRowCount()-1, d.selection+listNavigationDelta(key, capacity)))
	d.view.Overview = d.offersOverview() && d.selection == len(d.entries)
	if !d.view.Overview && len(d.entries) > 0 {
		d.view.SelectedKey = d.entries[d.selection].Key
	}
	if previous != d.selection {
		d.detailPage = 0
	}
}

//nolint:mnd // Panning advances one sixteenth of the visible world span.
func (d *AtlasDialog) panMap(key ebiten.Key) {
	if d.view.Zoom == 1 {
		d.status = "Zoom in to pan."

		return
	}
	step := d.view.span() / 16
	switch key { //nolint:exhaustive // Navigation admits only these directions.
	case ebiten.KeyUp:
		d.view.CenterY -= step
	case ebiten.KeyDown:
		d.view.CenterY += step
	case ebiten.KeyLeft:
		d.view.CenterX -= step
	case ebiten.KeyRight:
		d.view.CenterX += step
	}
	d.view.CenterX = atlasModulo(d.view.CenterX)
	d.view.CenterY = atlasModulo(d.view.CenterY)
}

//nolint:mnd // The approved zoom ladder is 1, 2, 4; label modes have two states.
func (d *AtlasDialog) handleKey(key ebiten.Key) {
	d.status = ""
	switch key { //nolint:exhaustive // Only reader intents enter this dispatcher.
	case ebiten.KeyN, ebiten.KeyE, ebiten.KeyD:
		d.requestNoteIntent(key)
	case ebiten.KeyTab:
		d.cycleCategory()
	case ebiten.KeyP:
		d.panning = !d.panning
		d.BlockUntilRelease(d.keyboard.IsKeyHeld)
	case ebiten.KeyDigit0, ebiten.KeyNumpad0:
		d.view.Zoom = 1
		d.view.CenterX = 128
		d.view.CenterY = 128
	case ebiten.KeyEqual, ebiten.KeyNumpadAdd:
		d.view.Zoom = min(4, d.view.Zoom*2)
	case ebiten.KeyMinus, ebiten.KeyNumpadSubtract:
		d.view.Zoom = max(1, d.view.Zoom/2)
	case ebiten.KeyL:
		d.view.Labels = (d.view.Labels + 1) % 2
		d.status = []string{"Place labels shown.", "Place labels hidden."}[d.view.Labels]
	case ebiten.KeyG:
		d.view.Grid = !d.view.Grid
	case ebiten.KeyA:
		d.centerOnCurrentFix()
	case ebiten.KeyEnter, ebiten.KeyNumpadEnter:
		d.centerOnSelection()
	case ebiten.KeyPageUp:
		d.detailPage = max(0, d.detailPage-1)
	case ebiten.KeyPageDown:
		d.detailPage++
	case ebiten.KeySpace:
		_, pages := d.prepareDetails()
		d.detailPage = (d.detailPage + 1) % pages
	}
}

// centerOnCurrentFix centres the map on the measured party position, or
// explains why no position fix is available.
func (d *AtlasDialog) centerOnCurrentFix() {
	if d.dossier.CurrentFix.Measured {
		d.center(d.dossier.CurrentFix.Location)
	} else {
		d.status = d.dossier.FixUnavailable
	}
}

// centerOnSelection centres the map on the selected list place when its
// position is known; the overview and an empty list have no selection.
func (d *AtlasDialog) centerOnSelection() {
	if d.view.Overview || len(d.entries) == 0 {
		return
	}
	if position, ok := d.entries[d.selection].PlotPosition(); ok {
		d.center(position)
	} else {
		d.status = "Measure this place to locate it from the list."
	}
}

func (d *AtlasDialog) cycleCategory() {
	direction := 1
	if d.keyboard.IsModifierPressed(input.ModifierShift) {
		direction = -1
	}
	count := int(journal.AtlasPlaceFilterCount)
	//nolint:gosec // Positive modulo bounds the selected category to 0..7.
	d.view.Filters.Places = journal.AtlasPlaceFilter((int(d.view.Filters.Places) + direction + count) % count)
	d.view.Overview = false
	d.selection = 0
	d.refilter()
	d.detailPage = 0
	d.BlockUntilRelease(d.keyboard.IsKeyHeld)
}

func (d *AtlasDialog) center(p journal.SiteLocation) {
	d.view.CenterX = float64(p.X)
	d.view.CenterY = float64(p.Y)
	if d.view.Zoom == 1 {
		d.view.Zoom = 2
	}
	d.status = ""
}

//nolint:mnd,varnamelen // Responsive panel ratios and padding are measured against logical w/h.
func (d *AtlasDialog) ensureLayout() {
	w, h := display.GetManager().GetCurrentSize()
	if w == d.width && h == d.height && d.font != nil {
		return
	}
	d.width, d.height = w, h
	config := FontConfig{Size: FontSizeBody, LineSpacing: 0,
		LineSpacingMultiplier: 1.15, MaxLines: journal.MaxAtlasNoteDescriptionRunes + 32, MaxChars: 128}
	d.font, d.details = GetScaledFontWithOutput(config)
	_, d.footer = GetScaledFontWithOutput(config)
	_, d.header = GetScaledFontWithOutput(FontConfig{Size: FontSizeTitle, LineSpacing: 0,
		LineSpacingMultiplier: config.LineSpacingMultiplier, MaxLines: 1, MaxChars: config.MaxChars})
	config.MaxLines = 4
	_, d.row = GetScaledFontWithOutput(config)
	_, d.label = GetScaledFontWithOutput(config)
	_, d.panelHeader = GetScaledFontWithOutput(config)
	config.MaxLines = 16
	_, d.position = GetScaledFontWithOutput(config)
	config.MaxLines = 4
	_, d.zoom = GetScaledFontWithOutput(config)
	_, d.tabText = GetScaledFontWithOutput(config)
	d.rowHeight = int(math.Ceil(d.row.GetTotalHeightForLines(1))) + 2
	margin := max(8, w/80)
	// Exactly two physical help rows. Position and transient status belong to
	// the right pane and never resize the map or reserve invisible footer rows.
	footerHeight := int(math.Ceil(d.footer.GetTotalHeightForLines(2))) + margin
	d.footerRect = image.Rect(margin, h-footerHeight, w-margin, h-margin)
	top := d.layoutCategoryTabs(margin) + margin/2
	placeWidth, _ := d.font.MeasureString("Crescent Waxing (last seen)")
	sideWidth := int(math.Ceil(placeWidth)) + d.rowHeight*2 + margin
	size := min(d.footerRect.Min.Y-top-margin, w-3*margin-sideWidth)
	d.mapRect = image.Rect(margin, top, margin+size, top+size)
	right := d.mapRect.Max.X + margin
	d.placesHeaderRect = image.Rect(right, top, w-margin, top+2*d.rowHeight)
	d.layoutPositionStatus(right, margin)
	listTop := d.placesHeaderRect.Max.Y + margin/2
	bodyBottom := d.positionRect.Min.Y - margin/2
	d.listRect = image.Rect(right, listTop, w-margin, listTop+(bodyBottom-listTop)/2)
	d.detailRect = image.Rect(right, d.listRect.Max.Y+margin, w-margin, bodyBottom)
	d.details.SetMaxCharsPerLine(max(1, d.font.CharsForWidth(d.detailRect.Dx()-8)))
}

// Draw uses borrowed geography and presentation buffers, without generation or I/O.
func (d *AtlasDialog) Draw(screen *ebiten.Image) {
	if d.closed {
		return
	}
	screen.Fill(ucolor.DialogBackground.RGBA())
	d.drawHeading(screen)
	d.drawCategoryTabs(screen)
	d.drawGeography(screen)
	d.drawPlacesHeading(screen)
	d.drawDirectory(screen)
	d.drawDetails(screen)
	d.drawPositionStatus(screen)
	d.drawFooter(screen)
}

//nolint:varnamelen // Row geometry uses a measured height and small text inset.
func (d *AtlasDialog) drawDirectory(screen *ebiten.Image) {
	rowHeight := d.directoryRowHeight()
	capacity := max(1, d.listRect.Dy()/rowHeight)
	start := max(0, min(d.selection-capacity/2, d.presentationRowCount()-capacity))
	if d.presentationRowCount() == 0 {
		d.row.Reset()
		d.row.SetMaxCharsPerLine(max(1, d.font.CharsForWidth(d.listRect.Dx())))
		d.row.AppendLine().Text("No known places in this category.", DialogControlHintColor).Done()
		d.row.DrawPanelContent(screen, d.listRect.Min, d.listRect.Dx(), etext.AlignStart)

		return
	}
	for i := start; i < min(d.presentationRowCount(), start+capacity); i++ {
		r := image.Rect(d.listRect.Min.X, d.listRect.Min.Y+(i-start)*rowHeight,
			d.listRect.Max.X, d.listRect.Min.Y+(i-start+1)*rowHeight)
		if i == d.selection {
			fillAtlasRect(screen, r, DialogSelectedBackground)
		}
		if i == len(d.entries) {
			d.drawOverviewRow(screen, r, i == d.selection)
		} else {
			d.drawPlaceRow(screen, r, d.entries[i], i == d.selection)
		}
	}
	count := d.presentationRowCount()
	drawMasterDetailScrollIndicators(screen, d.listRect, start, min(count, start+capacity), count,
		rowHeight, int(math.Ceil(d.font.TextFace.Size)), d.font, d.row)
}

func atlasCoordinates(p journal.SiteLocation) string {
	//nolint:gosec // The DTO contains validated survey coordinates 0..255.
	return references.FormatBritannianCoordinates(references.Coordinate(p.X), references.Coordinate(p.Y))
}

const (
	atlasCoordinatesLabel     = "Coordinates"
	atlasMoonstoneKind        = "moonstone"
	atlasNoteDescriptionLabel = "Description"
)

//nolint:varnamelen // e is the single selected detached entry.
func (d *AtlasDialog) detailFields() [][2]string {
	if d.view.Overview {
		return d.overviewFields()
	}
	if len(d.entries) == 0 {
		return [][2]string{{"", d.emptyCategoryText()}}
	}
	e := d.entries[d.selection]
	var fields [][2]string
	if e.Note.ID != 0 {
		label := "Map label"
		if e.Kind == AtlasJournalNoteKind {
			label = "Journal note"
		}

		return [][2]string{{label, e.Name}, {atlasNoteDescriptionLabel, e.Note.Description},
			{atlasCoordinatesLabel, atlasCoordinates(e.Note.Position)},
			{"Recorded", moonstoneDate(journal.OptionalGameTime{Known: true, Time: e.Note.RecordedAt})}}
	}
	if e.Vessel.ID != 0 {
		return [][2]string{{"Type", e.Kind}, {"Status", "Parked"},
			{atlasCoordinatesLabel, atlasCoordinates(e.Vessel.Position)}}
	}
	if e.OriginReference != "" {
		fields = append(fields, [2]string{"Original region", e.OriginReference})
	}
	if e.Carried {
		return append(fields, [2]string{"", "Carried by the party."})
	}
	if !e.Discovered {
		return append(fields, [2]string{"", "Not yet visited."})
	}
	if e.Survey.Measured {
		fields = append(fields,
			[2]string{atlasCoordinatesLabel, atlasCoordinates(e.Survey.Location)},
			[2]string{"Measured", moonstoneDate(e.Survey.RecordedAt)})
	} else {
		fields = append(fields, [2]string{atlasCoordinatesLabel, "Not measured"})
	}
	if e.LastObservedDestination {
		fields = append(fields, [2]string{"Last seen", moonstoneDate(e.ObservedAt)})
	}

	return fields
}

func (d *AtlasDialog) offersOverview() bool { return d.view.Filters.Places != journal.AtlasAllPlaces }
func (d *AtlasDialog) presentationRowCount() int {
	if d.offersOverview() {
		return len(d.entries) + 1
	}

	return len(d.entries)
}
func (d *AtlasDialog) mappedCount() int {
	count := 0
	for _, entry := range d.entries {
		if _, ok := entry.PlotPosition(); ok {
			count++
		}
	}

	return count
}
func (d *AtlasDialog) overviewFields() [][2]string {
	mapped, visible := d.mappedCount(), 0
	for _, entry := range d.entries {
		if _, ok := d.atlasMarkerCenter(entry); ok {
			visible++
		}
	}
	fields := [][2]string{{"", fmt.Sprintf("%d mapped; %d in view.", mapped, visible)}}
	if len(d.entries) == 0 {
		fields = append(fields, [2]string{"", d.emptyCategoryText()})
	}
	switch {
	case mapped == 0:
		fields = append(fields, [2]string{"", "No coordinates recorded in this category."})
	case d.view.Labels == AtlasLabelsNone:
		fields = append(fields, [2]string{"", "Labels hidden. L shows labels."})
	case visible == 0:
		fields = append(fields, [2]string{"", "No mapped places in view. 0 World."})
	default:
		fields = append(fields, [2]string{"", fmt.Sprintf("Showing all %d visible labels.", visible)})
	}
	missing := len(d.entries) - mapped
	noun := "places have"
	if missing == 1 {
		noun = "place has"
	}

	return append(fields, [2]string{"", fmt.Sprintf("%d %s no coordinates.", missing, noun)})
}
func (d *AtlasDialog) emptyCategoryText() string {
	switch d.view.Filters.Places { //nolint:exhaustive // Ordinary place families share the generic empty-state copy.
	case journal.AtlasVesselPlaces:
		if !d.dossier.VesselsUnlocked {
			return "Requires a sextant."
		}

		return "No boarded boats parked on Britannia. Recording begins when you use boats with a sextant."
	case journal.AtlasNotePlaces:
		return "No personal notes yet."
	default:
		return "No known places in this category."
	}
}

func (d *AtlasDialog) drawDetails(screen *ebiten.Image) {
	capacity, pages := d.prepareDetails()
	d.details.DrawPanelContentRange(screen, d.detailRect.Min, d.detailRect.Dx(), etext.AlignStart,
		d.detailPage*capacity, capacity)
	if pages > 1 {
		d.row.DrawPanelContent(screen, image.Pt(d.detailRect.Min.X, d.detailRect.Max.Y-d.rowHeight),
			d.detailRect.Dx(), etext.AlignStart)
	}
}

func (d *AtlasDialog) prepareDetails() (int, int) {
	d.details.Reset()
	for _, field := range d.detailFields() {
		if d.selectedNoteID() != 0 && field[0] == atlasNoteDescriptionLabel {
			d.appendNoteDescription(field[1])

			continue
		}
		line := d.details.AppendLine()
		if field[0] != "" {
			line.Text(field[0]+": ", DialogLabelColor)
		}
		line.Text(field[1], DialogValueColor).Done()
	}
	capacity := max(1, d.details.GetLineCapacity(d.detailRect.Dy()))
	// Output owns measured wrapping; page the actual physical wrapped rows.
	count := d.details.GetPanelContentLineCount()
	if count > capacity {
		capacity = max(1, d.details.GetLineCapacity(d.detailRect.Dy()-d.rowHeight))
	}
	pages := max(1, (count+capacity-1)/capacity)
	d.detailPage = min(d.detailPage, pages-1)
	if pages == 1 {
		return capacity, pages
	}
	d.row.Reset()
	d.row.SetMaxCharsPerLine(max(1, d.font.CharsForWidth(d.detailRect.Dx())))
	AppendControlHints(d.row.AppendLine(), fmt.Sprintf("%d/%d   [Space] More", d.detailPage+1, pages)).Done()

	return capacity, pages
}

// Preserve authored paragraphs and spacing rather than allowing the general
// styled wrapper to merge punctuation across an explicit player-authored LF.
//
//nolint:mnd // Match the detail pane's established eight-pixel text inset.
func (d *AtlasDialog) appendNoteDescription(description string) {
	d.details.AppendLine().Text(atlasNoteDescriptionLabel+":", DialogLabelColor).Done()
	width := d.detailRect.Dx() - 8
	for paragraph := range strings.SplitSeq(description, "\n") {
		if paragraph == "" {
			d.details.AppendLine().Text("", DialogValueColor).Done()

			continue
		}
		for _, row := range text.WrapJournalHistory(paragraph, d.font, width) {
			// Rows are already measured. Separate builders prohibit punctuation
			// merging, and this exact budget prevents a second whitespace reflow.
			d.details.SetMaxCharsPerLine(max(1, utf8.RuneCountInString(row.Text)))
			d.details.AppendLine().Text(row.Text, DialogValueColor).Done()
		}
	}
	d.details.SetMaxCharsPerLine(max(1, d.font.CharsForWidth(width)))
}

func (d *AtlasDialog) drawFooter(screen *ebiten.Image) {
	d.buildFooter()
	d.footer.DrawPanelContent(screen, d.footerRect.Min, d.footerRect.Dx(), etext.AlignStart)
}
