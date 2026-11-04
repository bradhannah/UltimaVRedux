package widgets

import (
	"fmt"
	"image"
	stdcolor "image/color"
	"math"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	pkgcolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

const (
	characterRosterDialogMaximumLogicalWidth = 170
	characterRosterDialogMarginUnits         = 4
	characterRosterDialogHeaderUnits         = 15
	characterRosterDialogFooterUnits         = 12
	characterRosterDialogPaddingUnits        = 2
	characterRosterDialogPaneHeaderUnits     = 7
	characterRosterDialogMinimumBodyUnits    = 58
	characterRosterDialogRosterRatio         = 0.48
	characterHealthRosterRatio               = 0.66
	characterHealthDetailLines               = 16
	characterRosterDialogLineSpacing         = 1.1
	characterRosterDialogRowSpacingChars     = 0.5
	characterRosterDialogCenterDivisor       = 2
	characterRosterDialogSectionMaxDivisor   = 4
	characterRosterDialogMapColumns          = 19
	characterRosterDialogMapRows             = 13
	characterRosterDialogMaximumChars        = 80
	characterRosterDialogDetailMaximumLines  = 12
	characterRosterDialogFooterMaximumLines  = 3
	characterRosterDialogFooterMaximumChars  = 100
)

// CharacterSelectionState describes whether one inspected row can be confirmed.
type CharacterSelectionState int

const (
	// CharacterSelectionAvailable identifies an ordinary confirmable row.
	CharacterSelectionAvailable CharacterSelectionState = iota
	// CharacterSelectionWarning identifies a confirmable row with consequences.
	CharacterSelectionWarning
	// CharacterSelectionUnavailable identifies an inspectable row that cannot confirm.
	CharacterSelectionUnavailable
	// CharacterSelectionLocked identifies the first New Order selection during stage two.
	CharacterSelectionLocked
)

// CharacterSelectionFact is one decision-relevant label/value pair.
type CharacterSelectionFact struct {
	Label string
	Value string
}

// CharacterSelectionHealth exposes current health, never a predicted treatment result.
type CharacterSelectionHealth struct {
	Current uint16
	Maximum uint16
	Missing uint16
	Dead    bool
}

// CharacterSelectionOption is a detached presentation projection for one roster row.
type CharacterSelectionOption struct {
	CharacterIndex int
	PartyPosition  int
	Name           string
	Status         string
	StatusKind     party_state.CharacterStatus
	PortraitSprite indexes.SpriteIndex
	State          CharacterSelectionState
	Active         bool
	Facts          []CharacterSelectionFact
	Details        []string
	Health         *CharacterSelectionHealth
	Resources      *CharacterSelectionResources
}

func (o CharacterSelectionOption) confirmable() bool {
	return o.State == CharacterSelectionAvailable || o.State == CharacterSelectionWarning
}

// statusColor keeps condition cues independent of focus and target eligibility.
func (o CharacterSelectionOption) statusColor(fallback stdcolor.Color) stdcolor.Color {
	switch o.StatusKind {
	case party_state.Poisoned:
		return pkgcolor.EGAGreen
	case party_state.Sleep:
		return pkgcolor.EGAMagenta
	case party_state.Charmed:
		// Dark EGA blue is unreadable as text on black; portraits keep it.
		return pkgcolor.EGALightBlue
	case party_state.Dead:
		return pkgcolor.EGARed
	case party_state.Good:
		return fallback
	default:
		return fallback
	}
}

type characterRosterDialogView struct {
	title       string
	rosterTitle string
	options     []CharacterSelectionOption
	selected    int
	footer      string
}

type characterRosterDialogLayout struct {
	panelRect      image.Rectangle
	headerRect     image.Rectangle
	rosterRect     image.Rectangle
	detailRect     image.Rectangle
	footerRect     image.Rectangle
	rosterBodyRect image.Rectangle
	detailBodyRect image.Rectangle
	portraitRect   image.Rectangle
	detailTextRect image.Rectangle
	rowRects       []image.Rectangle
	padding        int
	pixelUnit      int
}

// characterRosterDialogShell draws the shared roster/detail/footer family.
// Selection state and gameplay behavior remain with the concrete dialog owner.
type characterRosterDialogShell struct {
	guardResources        bool
	guardResourcesCompact bool
	healthRoster          bool
	placement             sprites.PercentBasedPlacement
	spriteSheet           *sprites.SpriteSheet

	titleOutput  *text.Output
	headerOutput *text.Output
	rowOutput    *text.Output
	detailOutput *text.Output
	footerOutput *text.Output
	bodyFont     *text.UltimaFont
	drawOp       *ebiten.DrawImageOptions
	layout       characterRosterDialogLayout

	lastScreenWidth  int
	lastScreenHeight int
	lastFooter       string
}

func newCharacterRosterDialogShell(
	placement sprites.PercentBasedPlacement,
	spriteSheet *sprites.SpriteSheet,
) *characterRosterDialogShell {
	if spriteSheet == nil {
		panic("character roster dialog: SpriteSheet is required")
	}
	if placement.EndPercentX <= placement.StartPercentX || placement.EndPercentY <= placement.StartPercentY {
		panic("character roster dialog: placement must have positive width and height")
	}

	//nolint:exhaustruct_v5 // Layout modes default off; outputs and geometry are lazy display-size caches.
	return &characterRosterDialogShell{
		placement:   placement,
		spriteSheet: spriteSheet,
		drawOp:      new(ebiten.DrawImageOptions),
	}
}

func (s *characterRosterDialogShell) draw(screen *ebiten.Image, view characterRosterDialogView) {
	if screen == nil {
		return
	}
	if s.guardResources {
		s.drawGuardResources(screen, view)

		return
	}
	bounds := screen.Bounds()
	s.ensureLayout(bounds.Dx(), bounds.Dy(), len(view.options), view.footer)
	vector.FillRect(
		screen,
		float32(s.layout.panelRect.Min.X),
		float32(s.layout.panelRect.Min.Y),
		float32(s.layout.panelRect.Dx()),
		float32(s.layout.panelRect.Dy()),
		pkgcolor.DialogBackground.RGBA(),
		false,
	)
	s.drawHeader(screen, view.title)
	s.drawRoster(screen, view)
	s.drawDetail(screen, view)
	s.drawFooter(screen, view.footer)
	s.drawFrames(screen)
}

func (s *characterRosterDialogShell) ensureLayout(width, height, rowCount int, footer string) {
	if width <= 0 || height <= 0 {
		return
	}
	if s.titleOutput != nil && width == s.lastScreenWidth && height == s.lastScreenHeight &&
		len(s.layout.rowRects) == rowCount && footer == s.lastFooter {
		return
	}
	s.lastScreenWidth = width
	s.lastScreenHeight = height
	s.lastFooter = footer
	s.calculateLayout(width, height, rowCount, footer)
	s.createOutputs()
}

// calculateLayout keeps responsive pane geometry together for bounds review.
//
//nolint:funlen // Keep interdependent roster, portrait, and detail bounds in one geometry calculation.
func (s *characterRosterDialogShell) calculateLayout(width, height, rowCount int, guidance string) {
	workspace := image.Rect(
		int(float64(width)*s.placement.StartPercentX),
		int(float64(height)*s.placement.StartPercentY),
		int(float64(width)*s.placement.EndPercentX),
		int(float64(height)*s.placement.EndPercentY),
	)
	pixelUnit := getClassicUIPixelUnit(width, height)
	margin := max(pixelUnit*characterRosterDialogMarginUnits, 1)
	availableWidth := max(1, workspace.Dx()-margin*2)
	availableHeight := max(1, workspace.Dy()-margin*characterRosterDialogCenterDivisor)
	panelWidth := min(availableWidth, characterRosterDialogMaximumLogicalWidth*pixelUnit)
	if s.healthRoster {
		panelWidth = availableWidth
	}
	headerHeight := max(pixelUnit*characterRosterDialogHeaderUnits, 1)
	paneHeaderHeight := max(1, pixelUnit*characterRosterDialogPaneHeaderUnits)
	padding := max(1, pixelUnit*characterRosterDialogPaddingUnits)
	// Reserve every packed hint row so no control hint is scrolled out of the footer.
	footerRows := len(packControlHintRows(GetScaledFont(FontSizeBody), ParseControlHints(guidance),
		panelWidth-padding*characterRosterDialogCenterDivisor, controlHintSeparator))
	footerHeight := max(pixelUnit*characterRosterDialogFooterUnits,
		int(math.Ceil(float64(footerRows)*GetScaledLineHeight(FontSizeBody)*characterRosterDialogLineSpacing))+
			padding*characterRosterDialogCenterDivisor)
	_, rowCharacterHeight := GetScaledFont(FontSizeCompact).MeasureString("M")
	rowHeight := max(1, int(math.Ceil(rowCharacterHeight*(1+characterRosterDialogRowSpacingChars))))
	desiredBodyHeight := max(
		pixelUnit*characterRosterDialogMinimumBodyUnits,
		rowCount*rowHeight+paneHeaderHeight+padding*characterRosterDialogSectionMaxDivisor,
	)
	if s.healthRoster {
		detailHeight := int(math.Ceil(rowCharacterHeight * characterRosterDialogLineSpacing * characterHealthDetailLines))
		desiredBodyHeight = max(desiredBodyHeight, detailHeight+paneHeaderHeight+padding*characterRosterDialogCenterDivisor)
	}
	panelHeight := min(availableHeight, headerHeight+desiredBodyHeight+footerHeight)
	panel := image.Rect(
		workspace.Min.X+(workspace.Dx()-panelWidth)/2,
		workspace.Min.Y+(workspace.Dy()-panelHeight)/2,
		workspace.Min.X+(workspace.Dx()+panelWidth)/2,
		workspace.Min.Y+(workspace.Dy()+panelHeight)/2,
	)
	headerHeight = min(headerHeight, max(1, panel.Dy()/characterRosterDialogSectionMaxDivisor))
	footerHeight = min(footerHeight, max(1, panel.Dy()/characterRosterDialogSectionMaxDivisor))
	bodyTop := panel.Min.Y + headerHeight
	bodyBottom := panel.Max.Y - footerHeight
	if bodyBottom <= bodyTop {
		bodyBottom = min(panel.Max.Y, bodyTop+1)
	}
	rosterRatio := characterRosterDialogRosterRatio
	if s.healthRoster {
		rosterRatio = characterHealthRosterRatio
	}
	split := panel.Min.X + int(math.Round(float64(panel.Dx())*rosterRatio))
	header := image.Rect(panel.Min.X, panel.Min.Y, panel.Max.X, bodyTop)
	roster := image.Rect(panel.Min.X, bodyTop, split, bodyBottom)
	detail := image.Rect(split, bodyTop, panel.Max.X, bodyBottom)
	footer := image.Rect(panel.Min.X, bodyBottom, panel.Max.X, panel.Max.Y)
	rosterBody := insetCharacterRosterRect(roster, padding*characterRosterDialogCenterDivisor)
	detailBody := insetCharacterRosterRect(detail, padding*characterRosterDialogCenterDivisor)
	rosterBody.Min.Y = min(rosterBody.Max.Y, rosterBody.Min.Y+paneHeaderHeight)
	detailBody.Min.Y = min(detailBody.Max.Y, detailBody.Min.Y+paneHeaderHeight)
	portraitWidth := max(1, min(workspace.Dx()/characterRosterDialogMapColumns, detailBody.Dx()))
	portraitHeight := max(1, min(workspace.Dy()/characterRosterDialogMapRows, detailBody.Dy()))
	portrait := image.Rect(
		detailBody.Min.X,
		detailBody.Min.Y,
		detailBody.Min.X+portraitWidth,
		detailBody.Min.Y+portraitHeight,
	)
	detailText := image.Rect(
		min(detailBody.Max.X, portrait.Max.X+padding*characterRosterDialogCenterDivisor),
		detailBody.Min.Y,
		detailBody.Max.X,
		detailBody.Max.Y,
	)
	if rowCount > 0 && rowHeight*rowCount > rosterBody.Dy() {
		rowHeight = max(1, rosterBody.Dy()/rowCount)
	}
	rows := make([]image.Rectangle, rowCount)
	for index := range rowCount {
		top := rosterBody.Min.Y + index*rowHeight
		bottom := min(rosterBody.Max.Y, top+rowHeight)
		rows[index] = image.Rect(rosterBody.Min.X, top, rosterBody.Max.X, bottom)
	}
	s.layout = characterRosterDialogLayout{
		panelRect: panel, headerRect: header, rosterRect: roster, detailRect: detail, footerRect: footer,
		rosterBodyRect: rosterBody, detailBodyRect: detailBody, portraitRect: portrait,
		detailTextRect: detailText, rowRects: rows, padding: padding, pixelUnit: pixelUnit,
	}
}

func (s *characterRosterDialogShell) createOutputs() {
	detailLines := characterRosterDialogDetailMaximumLines
	if s.healthRoster {
		detailLines = characterHealthDetailLines
	}
	_, s.titleOutput = GetScaledFontWithOutput(FontConfig{
		Size: FontSizeTitle, LineSpacing: 0, LineSpacingMultiplier: characterRosterDialogLineSpacing,
		MaxLines: 1, MaxChars: characterRosterDialogMaximumChars,
	})
	s.bodyFont, s.rowOutput = GetScaledFontWithOutput(FontConfig{
		Size: FontSizeCompact, LineSpacing: 0, LineSpacingMultiplier: characterRosterDialogLineSpacing,
		MaxLines: 1, MaxChars: characterRosterDialogMaximumChars,
	})
	_, s.headerOutput = GetScaledFontWithOutput(FontConfig{
		Size: FontSizeCompact, LineSpacing: 0, LineSpacingMultiplier: characterRosterDialogLineSpacing,
		MaxLines: 1, MaxChars: characterRosterDialogMaximumChars,
	})
	_, s.detailOutput = GetScaledFontWithOutput(FontConfig{
		Size: FontSizeCompact, LineSpacing: 0, LineSpacingMultiplier: characterRosterDialogLineSpacing,
		MaxLines: detailLines, MaxChars: characterRosterDialogMaximumChars,
	})
	_, s.footerOutput = GetScaledFontWithOutput(FontConfig{
		Size: FontSizeBody, LineSpacing: 0, LineSpacingMultiplier: characterRosterDialogLineSpacing,
		MaxLines: characterRosterDialogFooterMaximumLines, MaxChars: characterRosterDialogFooterMaximumChars,
	})
	s.titleOutput.SetMaxCharsPerLine(max(
		1,
		s.titleOutput.Font.CharsForWidth(
			s.layout.headerRect.Dx()-s.layout.padding*characterRosterDialogCenterDivisor,
		),
	))
	s.rowOutput.SetMaxCharsPerLine(max(1, s.bodyFont.CharsForWidth(s.layout.rosterBodyRect.Dx())))
	s.detailOutput.SetMaxCharsPerLine(max(1, s.detailOutput.Font.CharsForWidth(s.layout.detailTextRect.Dx())))
	s.footerOutput.SetMaxCharsPerLine(max(
		1,
		s.footerOutput.Font.CharsForWidth(
			s.layout.footerRect.Dx()-s.layout.padding*characterRosterDialogCenterDivisor,
		),
	))
}

func (s *characterRosterDialogShell) drawFrames(screen *ebiten.Image) {
	DrawClassicPixelRoundedFrame(
		screen, s.layout.panelRect, s.layout.pixelUnit, DialogRuleLightColor,
	)
	separatorSize := float32(max(1, s.layout.pixelUnit))
	vector.FillRect(
		screen,
		float32(s.layout.panelRect.Min.X),
		float32(s.layout.headerRect.Max.Y),
		float32(s.layout.panelRect.Dx()),
		separatorSize,
		DialogRuleColor,
		false,
	)
	vector.FillRect(
		screen,
		float32(s.layout.panelRect.Min.X),
		float32(s.layout.footerRect.Min.Y),
		float32(s.layout.panelRect.Dx()),
		separatorSize,
		DialogRuleColor,
		false,
	)
	vector.FillRect(
		screen,
		float32(s.layout.detailRect.Min.X),
		float32(s.layout.detailRect.Min.Y),
		separatorSize,
		float32(s.layout.detailRect.Dy()),
		DialogRuleColor,
		false,
	)
}

func (s *characterRosterDialogShell) drawHeader(screen *ebiten.Image, title string) {
	s.titleOutput.Reset()
	s.titleOutput.AppendLine().Text(title, DialogHeaderColor).Done()
	textHeight := int(math.Ceil(s.titleOutput.GetFontHeight()))
	y := s.layout.headerRect.Min.Y + max(
		0, (s.layout.headerRect.Dy()-textHeight)/characterRosterDialogCenterDivisor,
	)
	s.titleOutput.DrawPanelContent(
		screen, image.Pt(s.layout.headerRect.Min.X, y), s.layout.headerRect.Dx(), etext.AlignCenter,
	)
}

func (s *characterRosterDialogShell) drawRoster(screen *ebiten.Image, view characterRosterDialogView) {
	if s.healthRoster {
		s.drawHealthRoster(screen, view)

		return
	}
	s.drawPaneHeader(screen, s.layout.rosterRect, view.rosterTitle)
	for index, option := range view.options {
		row := s.layout.rowRects[index]
		nameRect, statusRect := s.rosterRowTextRects(row, option.Status)
		selected := index == view.selected
		color := characterRosterRowColor(option.State, selected)
		s.appendRosterRow(option, selected, nameRect.Dx())
		textHeight := int(math.Ceil(s.rowOutput.GetFontHeight()))
		y := row.Min.Y + max(0, (row.Dy()-textHeight)/characterRosterDialogCenterDivisor)
		s.rowOutput.DrawPanelContent(screen, image.Pt(nameRect.Min.X, y), nameRect.Dx(), etext.AlignStart)
		if option.Status != "" {
			s.rowOutput.Reset()
			s.rowOutput.AppendLine().Text(option.Status, option.statusColor(color)).Done()
			s.rowOutput.DrawPanelContent(screen, image.Pt(statusRect.Min.X, y), statusRect.Dx(), etext.AlignStart)
		}
	}
}

func (s *characterRosterDialogShell) appendRosterRow(option CharacterSelectionOption, selected bool, width int) {
	color := characterRosterRowColor(option.State, selected)
	focus, focusColor := " ", color
	if selected {
		focus, focusColor = ">", pkgcolor.FocusIndicator.RGBA()
	}
	marker := characterRosterStateMarker(option.State)
	position := ""
	if option.PartyPosition >= 0 {
		position = fmt.Sprintf("%d ", option.PartyPosition)
	}
	active := ""
	if option.Active {
		active = "*"
	}
	// Reserve all markers before fitting the name; panel width alone does not clip text.
	name := option.Name
	for count := utf8.RuneCountInString(name); ; count-- {
		textWidth, _ := s.bodyFont.MeasureString(focus + marker + position + name + active)
		if textWidth <= float64(width) || count == 0 {
			break
		}
		name = truncateRunes(option.Name, count-1)
	}
	s.rowOutput.Reset()
	s.rowOutput.AppendLine().Text(focus, focusColor).
		Text(marker, characterRosterStateColor(option.State)).Text(position, color).
		Text(name, color).Text(active, pkgcolor.EGALightGreen).Done()
}

func (s *characterRosterDialogShell) rosterRowTextRects(
	row image.Rectangle,
	status string,
) (image.Rectangle, image.Rectangle) {
	statusWidth, _ := s.bodyFont.MeasureString(status)
	characterWidth, _ := s.bodyFont.MeasureString("M")
	gap := int(math.Ceil(characterWidth * characterRosterDialogRowSpacingChars))
	statusLeft := max(row.Min.X, row.Max.X-int(math.Ceil(statusWidth)))
	nameRight := max(row.Min.X+1, statusLeft-gap)

	return image.Rect(row.Min.X, row.Min.Y, nameRight, row.Max.Y),
		image.Rect(statusLeft, row.Min.Y, row.Max.X, row.Max.Y)
}

func (s *characterRosterDialogShell) drawDetail(screen *ebiten.Image, view characterRosterDialogView) {
	if view.selected < 0 || view.selected >= len(view.options) {
		s.drawPaneHeader(screen, s.layout.detailRect, "DETAILS")

		return
	}
	option := view.options[view.selected]
	s.drawPaneHeader(screen, s.layout.detailRect, option.Name)
	detailTextRect := s.layout.detailTextRect
	if option.PortraitSprite == indexes.NoSprites || s.healthRoster {
		detailTextRect = s.layout.detailBodyRect
	} else {
		s.drawPortrait(screen, option.PortraitSprite)
	}
	s.detailOutput.Reset()
	s.detailOutput.SetMaxCharsPerLine(max(1, s.detailOutput.Font.CharsForWidth(detailTextRect.Dx())))
	s.appendDetailText(option)
	s.detailOutput.DrawPanelContent(
		screen,
		detailTextRect.Min,
		max(1, detailTextRect.Dx()),
		etext.AlignStart,
	)
}

func (s *characterRosterDialogShell) appendDetailText(option CharacterSelectionOption) {
	if option.Status != "" {
		s.detailOutput.AppendLine().Text("Status: ", DialogLabelColor).
			Text(option.Status, option.statusColor(DialogValueColor)).Done()
	}
	for _, fact := range option.Facts {
		s.detailOutput.AppendLine().Text(fact.Label+": ", DialogLabelColor).Text(fact.Value, DialogValueColor).Done()
	}
	if len(option.Details) > 0 && (option.Status != "" || len(option.Facts) > 0) {
		s.detailOutput.AppendLine().White("").Done()
	}
	for _, detail := range option.Details {
		s.detailOutput.AppendLine().Text(detail, characterRosterDetailColor(option.State)).WordWrap().Done()
	}
}

func characterRosterDetailColor(state CharacterSelectionState) stdcolor.Color {
	switch state {
	case CharacterSelectionWarning, CharacterSelectionUnavailable:
		return DialogStatusWarningColor
	case CharacterSelectionLocked:
		return pkgcolor.EGALightCyan
	case CharacterSelectionAvailable:
		return DialogValueColor
	default:
		panic("unknown character selection state")
	}
}

func (s *characterRosterDialogShell) drawFooter(screen *ebiten.Image, footer string) {
	s.footerOutput.Reset()
	content := insetCharacterRosterRect(s.layout.footerRect, s.layout.padding)
	PackControlHints(s.footerOutput, ParseControlHints(footer), content.Dx(), etext.AlignCenter)
	totalHeight := int(math.Ceil(s.footerOutput.GetTotalHeightForLines(s.footerOutput.GetPanelContentLineCount())))
	y := content.Min.Y + max(0, (content.Dy()-totalHeight)/characterRosterDialogCenterDivisor)
	s.footerOutput.DrawPanelContent(screen, image.Pt(content.Min.X, y), content.Dx(), etext.AlignCenter)
}

func (s *characterRosterDialogShell) drawPaneHeader(screen *ebiten.Image, rect image.Rectangle, title string) {
	s.headerOutput.Reset()
	s.headerOutput.AppendLine().Text(title, DialogHeaderColor).Done()
	content := insetCharacterRosterRect(rect, s.layout.padding*characterRosterDialogCenterDivisor)
	s.headerOutput.DrawPanelContent(screen, content.Min, content.Dx(), etext.AlignStart)
}

func (s *characterRosterDialogShell) drawPortrait(screen *ebiten.Image, spriteIndex indexes.SpriteIndex) {
	if s.spriteSheet.SpriteImage == nil || spriteIndex == indexes.NoSprites {
		return
	}
	sprite := s.spriteSheet.GetRawSprite(spriteIndex)
	if sprite == nil || sprite.Bounds().Empty() {
		return
	}
	frameInset := max(1, s.layout.pixelUnit)
	DrawClassicPixelRoundedFrame(
		screen, s.layout.portraitRect.Inset(-frameInset), s.layout.pixelUnit, DialogRuleLightColor,
	)
	scaleX := float64(s.layout.portraitRect.Dx()) / float64(sprite.Bounds().Dx())
	scaleY := float64(s.layout.portraitRect.Dy()) / float64(sprite.Bounds().Dy())
	s.drawOp.GeoM.Reset()
	s.drawOp.Filter = ebiten.FilterNearest
	s.drawOp.GeoM.Translate(float64(-sprite.Bounds().Min.X), float64(-sprite.Bounds().Min.Y))
	s.drawOp.GeoM.Scale(scaleX, scaleY)
	s.drawOp.GeoM.Translate(
		float64(s.layout.portraitRect.Min.X),
		float64(s.layout.portraitRect.Min.Y),
	)
	screen.DrawImage(sprite, s.drawOp)
}

func characterRosterRowColor(state CharacterSelectionState, selected bool) stdcolor.Color {
	if selected {
		return DialogValueColor
	}
	switch state {
	case CharacterSelectionWarning:
		return pkgcolor.EGAYellow
	case CharacterSelectionUnavailable:
		return pkgcolor.UITextDisabled.RGBA()
	case CharacterSelectionLocked:
		return pkgcolor.EGALightCyan
	case CharacterSelectionAvailable:
		return DialogControlHintColor
	default:
		panic("unknown character selection state")
	}
}

func characterRosterStateMarker(state CharacterSelectionState) string {
	switch state {
	case CharacterSelectionUnavailable:
		return "X"
	case CharacterSelectionLocked:
		return "L"
	case CharacterSelectionWarning:
		return "!"
	case CharacterSelectionAvailable:
		return " "
	default:
		panic("unknown character selection state")
	}
}

func characterRosterStateColor(state CharacterSelectionState) stdcolor.Color {
	switch state {
	case CharacterSelectionUnavailable:
		return DialogStatusWarningColor
	case CharacterSelectionLocked:
		return pkgcolor.EGALightCyan
	case CharacterSelectionAvailable, CharacterSelectionWarning:
		return DialogValueColor
	default:
		panic("unknown character selection state")
	}
}

func insetCharacterRosterRect(rect image.Rectangle, inset int) image.Rectangle {
	inset = max(0, min(inset, min(rect.Dx(), rect.Dy())/characterRosterDialogCenterDivisor))

	return image.Rect(rect.Min.X+inset, rect.Min.Y+inset, rect.Max.X-inset, rect.Max.Y-inset)
}
