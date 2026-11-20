package widgets

import (
	"image"
	"math"
	"strconv"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
)

const (
	commandHelpMargin       = 0.08
	commandHelpPadding      = 12
	commandHelpTabGap       = 6 // Room for the open tab's plate under the title.
	commandHelpEntryRows    = 20
	commandHelpFooterRows   = 8
	commandHelpInitialChars = 80
	commandHelpSpacing      = 1.2
	commandHelpIconRows     = 2
	commandHelpSides        = 2
)

// CommandHelpEntry is detached guidance, never an executable command. Group
// gathers related keys on the Right now page.
type CommandHelpEntry struct {
	Key         string
	Purpose     string
	NextInput   string
	Unavailable string
	Group       string
}

// Help opens on Right now: the keys that work at this moment.
const (
	helpPageNow = iota
	helpPageCommands
	helpPageStatus
	helpPageCount
)

// StatusHelpEntry describes an already visible status-rail icon.
type StatusHelpEntry struct {
	Name        string
	Description string
	Icon        *ebiten.Image
	Tint        [3]float32
}

// HelpStatusView is an opening snapshot with no hidden countdowns or actor identities.
type HelpStatusView struct {
	Worn    []StatusHelpEntry
	Effects []StatusHelpEntry
	Food    uint16
}

type statusHelpIcon struct {
	entry StatusHelpEntry
	row   int
}

// CommandHelpDialog is a keyboard-only, transient command reference.
type CommandHelpDialog struct {
	navigation
	entries  []CommandHelpEntry
	keyboard *input.Keyboard
	onClose  func()
	output   *text.Output
	footer   *text.Output
	border   *Border
	row      int
	width    int
	height   int
	title    *text.Output
	header   *text.Output
	status   HelpStatusView
	page     int
	pageRows [helpPageCount]int
	icons    []statusHelpIcon
}

// NewCommandHelpDialog creates a detached, paged command reference.
func NewCommandHelpDialog(
	entries []CommandHelpEntry, status HelpStatusView, keyboard *input.Keyboard, onClose func(),
) *CommandHelpDialog {
	if keyboard == nil || onClose == nil {
		panic("command help requires keyboard and close callback")
	}
	dialog := &CommandHelpDialog{
		navigation: newNavigation(keyboard.IsKeyHeld),
		entries:    append([]CommandHelpEntry(nil), entries...), keyboard: keyboard, onClose: onClose,
		output: nil, footer: nil, border: nil, row: 0, width: 0, height: 0,
		title: nil, header: nil, status: HelpStatusView{Worn: append([]StatusHelpEntry(nil), status.Worn...),
			Effects: append([]StatusHelpEntry(nil), status.Effects...), Food: status.Food},
		page: helpPageNow, pageRows: [helpPageCount]int{}, icons: nil,
	}
	dialog.ensureLayout()
	keyboard.PrepareForTransition(KeyboardTransitionDelayMs)

	return dialog
}

func (d *CommandHelpDialog) ensureLayout() {
	width, height := display.GetManager().GetCurrentSize()
	if d.output != nil && width == d.width && height == d.height {
		return
	}
	d.width, d.height = width, height
	d.border = newDialogBorder(sprites.PercentBasedPlacement{
		StartPercentX: commandHelpMargin, EndPercentX: 1 - commandHelpMargin,
		StartPercentY: commandHelpMargin, EndPercentY: 1 - commandHelpMargin,
	})
	font, output := GetScaledFontWithOutput(FontConfig{
		Size: FontSizeBody, MaxLines: 1, MaxChars: commandHelpInitialChars,
		LineSpacing: 0, LineSpacingMultiplier: commandHelpSpacing,
	})
	chars := max(1, font.CharsForWidth(d.border.GetInteriorBounds().Dx()-2*commandHelpPadding))
	d.output = text.NewOutput(font, output.GetLineSpacing(), max(1,
		(len(d.entries)+len(d.status.Worn)+len(d.status.Effects)+1)*commandHelpEntryRows), chars)
	_, d.title = GetScaledFontWithOutput(FontConfig{
		Size: FontSizeTitle, MaxLines: 1, MaxChars: commandHelpInitialChars,
		LineSpacing: 0, LineSpacingMultiplier: commandHelpSpacing,
	})
	d.title.AppendLine().Text("HELP", DialogHeaderColor).Done()
	d.header = text.NewOutput(font, output.GetLineSpacing(), 1, chars)
	d.footer = text.NewOutput(font, output.GetLineSpacing(), commandHelpFooterRows, chars)
	d.populatePage()
}

func (d *CommandHelpDialog) populatePage() {
	d.output.Reset()
	d.icons = nil
	d.header.Reset()
	tabs := d.header.AppendLine()
	for page, label := range helpPageTitles() {
		colour := DialogTabColor
		if page == d.page {
			colour = DialogActiveTabText
		}
		if page > 0 {
			tabs.Text(helpTabGap, colour)
		}
		tabs.Text(label, colour)
	}
	tabs.Done()
	width := d.border.GetInteriorBounds().Dx() - commandHelpSides*commandHelpPadding
	if d.page == helpPageStatus {
		width -= d.iconColumnWidth()
	}
	d.output.SetMaxCharsPerLine(max(1, d.output.Font.CharsForWidth(width)))
	switch d.page {
	case helpPageStatus:
		d.populateStatus()
	case helpPageCommands:
		d.populateCommands()
	default:
		d.populateNow(width)
	}
	d.footer.Reset()
	title := [helpPageCount]string{"KEYS THAT WORK RIGHT NOW", "COMMAND HELP", "STATUS HELP"}[d.page]
	d.footer.AppendLine().Text(title, DialogHeaderColor).
		Text("   No time or resources spent", DialogControlHintColor).WordWrap().Done()
	d.row = min(d.row, d.lastRow())
	d.appendFooterHints()
}

// appendFooterHints adds the three keys a lost player needs. The other paging
// keys keep working and are listed with the Help command on Commands. Scroll is
// dimmed when the page already fits.
func (d *CommandHelpDialog) appendFooterHints() {
	scrollKey, scrollHint := DialogControlKeyColor, DialogControlHintColor
	if d.lastRow() == 0 {
		scrollKey, scrollHint = DialogUnavailableItemColor, DialogUnavailableItemColor
	}
	d.footer.AppendLine().Text("[Tab]", DialogControlKeyColor).Text(" Page of Help   ", DialogControlHintColor).
		Text("[Up/Down]", scrollKey).Text(" Scroll   ", scrollHint).
		Text("["+hintKeyEsc+"]", DialogControlKeyColor).Text(" "+hintActionClose, DialogControlHintColor).Done()
}

// populateNow lists the keys usable at this moment, one group to a heading and
// several keys to a row, so the whole answer normally fits one screen. Keys that
// cannot be used here are named last under the reason Commands gives for them.
func (d *CommandHelpDialog) populateNow(width int) {
	var groups, reasons []string
	hints, unavailable := map[string][]ControlHint{}, map[string][]ControlHint{}
	for _, entry := range d.entries {
		if entry.Unavailable != "" {
			if _, known := unavailable[entry.Unavailable]; !known {
				reasons = append(reasons, entry.Unavailable)
			}
			unavailable[entry.Unavailable] = append(unavailable[entry.Unavailable],
				ControlHint{Key: entry.Key, Action: entry.Purpose})

			continue
		}
		if _, known := hints[entry.Group]; !known {
			groups = append(groups, entry.Group)
		}
		hints[entry.Group] = append(hints[entry.Group], ControlHint{Key: entry.Key, Action: entry.Purpose})
	}
	chars := max(1, d.output.Font.CharsForWidth(width))
	for _, group := range groups {
		d.output.SetMaxCharsPerLine(chars)
		d.output.AppendLine().Text(group, DialogLabelColor).Done()
		PackControlHints(d.output, hints[group], width, etext.AlignStart)
		d.output.AppendLine().White("").Done()
	}
	d.output.SetMaxCharsPerLine(chars)
	if len(groups) == 0 {
		d.output.AppendLine().Text("No command key works at this moment.", DialogValueColor).WordWrap().Done()
	}
	for _, reason := range reasons {
		d.output.SetMaxCharsPerLine(chars)
		d.output.AppendLine().Text(reason, DialogWarningColor).WordWrap().Done()
		PackControlHints(d.output, unavailable[reason], width, etext.AlignStart)
	}
}

func (d *CommandHelpDialog) populateCommands() {
	for _, entry := range d.entries {
		d.output.AppendLine().Text(entry.Key+"  ", DialogControlKeyColor).Text(entry.Purpose, DialogValueColor).Done()
		if entry.Unavailable != "" {
			d.output.AppendLine().LightRed("Unavailable: " + entry.Unavailable).Done()
		} else {
			d.output.AppendLine().LightGray("Next: " + entry.NextInput).Done()
		}
		d.output.AppendLine().White("").Done()
	}
}

func (d *CommandHelpDialog) populateStatus() {
	d.output.AppendLine().Text("Status rail: worn items on the left, active effects on the right.",
		DialogControlHintColor).WordWrap().Done()
	d.appendStatusGroup("WORN", "None worn.", d.status.Worn)
	d.appendStatusGroup("ACTIVE EFFECTS", "None active.", d.status.Effects)
	d.output.AppendLine().Text("Food remaining: ", DialogLabelColor).
		Text(strconv.Itoa(int(d.status.Food)), DialogValueColor).Done()
}

func (d *CommandHelpDialog) appendStatusGroup(title, empty string, entries []StatusHelpEntry) {
	d.output.AppendLine().Text(title, DialogHeaderColor).Done()
	if len(entries) == 0 {
		d.output.AppendLine().Text(empty, DialogEmptyColor).Done()
	}
	for _, entry := range entries {
		d.icons = append(d.icons, statusHelpIcon{entry: entry, row: d.output.GetPanelContentLineCount()})
		d.output.AppendLine().Text(entry.Name, DialogLabelColor).Done()
		d.output.AppendLine().Text(entry.Description, DialogValueColor).WordWrap().Done()
		d.output.AppendLine().Done()
	}
	d.output.AppendLine().Done()
}

func (d *CommandHelpDialog) contentBounds() image.Rectangle {
	bounds := d.border.GetInteriorBounds().Inset(commandHelpPadding)
	bounds.Min.Y += int(d.title.GetTotalHeightForLines(1)+d.header.GetTotalHeightForLines(1)) +
		commandHelpTabGap + commandHelpPadding
	bounds.Max.Y -= int(d.footer.GetTotalHeightForLines(d.footer.GetPanelContentLineCount())) + commandHelpPadding

	return bounds
}

func (d *CommandHelpDialog) iconColumnWidth() int {
	return int(d.output.GetTotalHeightForLines(commandHelpIconRows)) + commandHelpPadding
}

func (d *CommandHelpDialog) pageSize() int {
	return max(1, d.output.GetLineCapacity(d.contentBounds().Dy()))
}

func (d *CommandHelpDialog) lastRow() int {
	return max(0, d.output.GetPanelContentLineCount()-d.pageSize())
}

// Update handles paging and dismissal without dispatching gameplay commands.
func (d *CommandHelpDialog) Update() {
	d.ensureLayout()
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.keyboard.PrepareForTransition(KeyboardTransitionDelayMs)
		d.onClose()

		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyTab) {
		step := 1
		if d.keyboard.IsModifierPressed(input.ModifierShift) {
			step = helpPageCount - 1
		}
		d.showPage((d.page + step) % helpPageCount)

		return
	}
	key, owned := d.Poll(d.keyboard,
		ebiten.KeyUp, ebiten.KeyDown, ebiten.KeyPageUp, ebiten.KeyPageDown, ebiten.KeyHome, ebiten.KeyEnd)
	if key != nil {
		switch *key { //nolint:exhaustive // Only navigation keys reach this switch.
		case ebiten.KeyHome:
			d.row = 0
		case ebiten.KeyEnd:
			d.row = d.lastRow()
		default:
			d.row = max(0, min(d.lastRow(), d.row+listNavigationDelta(*key, d.pageSize())))
		}
	}
	if !owned && d.keyboard.IsKeyPressedAndRegister(ebiten.KeySpace) {
		d.row = min(d.lastRow(), d.row+d.pageSize())
	}
}

// helpTabGap separates the page titles on the one centred tab line.
const helpTabGap = "    "

func helpPageTitles() [helpPageCount]string {
	return [helpPageCount]string{"Right now", "Commands", "Status"}
}

// The tab line is one centred row of text, so the open page's place on it is measured.
func (d *CommandHelpDialog) drawActiveTabPlate(screen *ebiten.Image, bounds image.Rectangle, pixelUnit int) {
	titles := helpPageTitles()
	line, _ := d.header.Font.MeasureString(strings.Join(titles[:], helpTabGap))
	before, _ := d.header.Font.MeasureString(strings.Join(titles[:d.page+1], helpTabGap))
	width, _ := d.header.Font.MeasureString(titles[d.page])
	left := bounds.Min.X + (bounds.Dx()-int(math.Ceil(line)))/commandHelpSides + int(math.Round(before-width))
	cell := image.Rect(left, bounds.Min.Y, left+int(math.Ceil(width)), bounds.Max.Y)
	DrawActiveTabPlate(screen, cell, width, bounds.Min.Y, int(math.Ceil(d.header.GetFontHeight())), pixelUnit)
}

// showPage changes page and restores the reading position last left on it.
func (d *CommandHelpDialog) showPage(page int) {
	d.pageRows[d.page] = d.row
	d.page = page
	d.row = d.pageRows[page]
	d.Reset()
	d.populatePage()
}

// Draw renders the visible command page through Output.
func (d *CommandHelpDialog) Draw(screen *ebiten.Image) {
	d.ensureLayout()
	pixelUnit := getClassicUIPixelUnit(screen.Bounds().Dx(), screen.Bounds().Dy())
	FillClassicPixelRoundedRect(screen, d.border.GetBounds(), pixelUnit, ucolor.DialogBackground.RGBA())
	DrawClassicPixelRoundedFrame(screen, d.border.GetBounds(), pixelUnit, DialogRuleColor)
	bounds := d.border.GetInteriorBounds().Inset(commandHelpPadding)
	d.title.DrawPanelContent(screen, bounds.Min, bounds.Dx(), etext.AlignCenter)
	bounds.Min.Y += int(d.title.GetTotalHeightForLines(1)) + commandHelpTabGap
	d.drawActiveTabPlate(screen, bounds, pixelUnit)
	d.header.DrawPanelContent(screen, bounds.Min, bounds.Dx(), etext.AlignCenter)
	content := d.contentBounds()
	if d.page == helpPageStatus {
		d.drawStatusIcons(screen, content)
		content.Min.X += d.iconColumnWidth()
	}
	d.output.DrawPanelContentRange(screen, content.Min, content.Dx(), etext.AlignStart, d.row, d.pageSize())
	footerY := bounds.Max.Y - int(d.footer.GetTotalHeightForLines(d.footer.GetPanelContentLineCount()))
	d.footer.DrawPanelContent(screen, image.Pt(bounds.Min.X, footerY), bounds.Dx(), etext.AlignStart)
}

func (d *CommandHelpDialog) drawStatusIcons(screen *ebiten.Image, bounds image.Rectangle) {
	clipBounds := bounds.Intersect(screen.Bounds())
	if clipBounds.Empty() {
		return
	}
	clipped, ok := screen.SubImage(clipBounds).(*ebiten.Image)
	if !ok {
		panic("help icon clip is not an image")
	}
	size := d.iconColumnWidth() - commandHelpPadding
	for _, icon := range d.icons {
		if icon.entry.Icon == nil || icon.row+commandHelpIconRows <= d.row || icon.row >= d.row+d.pageSize() {
			continue
		}
		art := icon.entry.Icon
		scale := min(float64(size)/float64(art.Bounds().Dx()), float64(size)/float64(art.Bounds().Dy()))
		op := &ebiten.DrawImageOptions{Filter: ebiten.FilterNearest}
		op.GeoM.Scale(scale, scale)
		op.GeoM.Translate(float64(bounds.Min.X), float64(bounds.Min.Y)+float64(icon.row-d.row)*d.output.GetLineSpacing())
		op.ColorScale.Scale(icon.entry.Tint[0], icon.entry.Tint[1], icon.entry.Tint[2], 1)
		clipped.DrawImage(art, op)
	}
}
