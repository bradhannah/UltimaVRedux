package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

type gameplaySettingDefinition struct {
	label  string
	detail string
	// defaultLine closes a detail-only row; rows with helpLines carry their own.
	defaultLine string
	helpLines   []string
	numeric     *gameplayNumericSetting
}

type gameplayNumericSetting struct {
	spec   config.NumericSettingSpec
	get    func(config.GameplayConfiguration) float64
	set    func(*config.GameplayConfiguration, float64)
	format func(float64) string
}

const (
	gameplaySettingOffLabel = "Off"
	gameplayDefaultOnLine   = "Default: On"
	gameplayDefaultOffLine  = "Default: Off"
)

const (
	gameplayRepeatedDivisionIndex = iota
	gameplayHiddenDiscoveriesIndex
	gameplayUnlitDungeonsIndex
	gameplayBatchMerchantTheftIndex
	gameplaySmoothSurfaceMovementIndex
	gameplayConcealedTurnSlotsIndex
	gameplayFlexibleSextantUseIndex
	gameplayAnkhWandererBonusIndex
	gameplayDeedExperienceIndex
	gameplayShareKillExperienceIndex
	gameplayCombatSpeedIndex
	gameplayCombatKeyRepeatIndex
	gameplayCombatPanelsIndex
	gameplayConfirmQuickSaveIndex
	gameplayShowCombatHPBarsIndex
	gameplayBattleFrameIndex
	gameplayBattleExperienceIndex
	gameplayMaxActionPopupsIndex
	gameplaySimpleNotesModeIndex
)

//nolint:exhaustruct_v5,funlen // Explicit row catalog; boolean rows have no numeric accessors.
func gameplaySettings() [audioSettingsGameplayCount]gameplaySettingDefinition {
	return [...]gameplaySettingDefinition{
		{
			label: "Repeated Monster Division", defaultLine: gameplayDefaultOffLine,
			detail: "On: a Slime or Gargoyle that survives can divide again and again, as in the original. " +
				"Off: each divides once, and its child never does.",
		},
		{
			label: "Remember Hidden Dungeon Discoveries", defaultLine: gameplayDefaultOnLine,
			detail: "On: hidden doors and pits you find stay found. " +
				"Off: they are hidden again on your next visit.",
		},
		{
			label: "Show Subdued Unlit Dungeons", defaultLine: gameplayDefaultOnLine,
			detail: "On: with no light you still make out dim walls and passages, but nothing is lit. " +
				"Off: no light, no view, as in the original.",
		},
		{
			label: "Batch Merchant Theft", defaultLine: gameplayDefaultOnLine,
			detail: "On: when gold goes missing in a shop, it happens once for the whole purchase. " +
				"Off: as in the original, once for each item, bundle or patient, which can cut a purchase short.",
		},
		{
			label: "Smooth Surface Movement", defaultLine: gameplayDefaultOffLine,
			detail: "On: the map glides as you walk, ride, fly or sail. " +
				"Only the look changes; play, battles and dungeons run as before.",
		},
		{
			label: "Hidden Enemy Turns", defaultLine: gameplayDefaultOnLine,
			detail: "On: an enemy you cannot see (invisible, under an arch or in the dark) " +
				"still takes its place in Turn Order, as ??? unseen. Off: it is left out. " +
				"The turns themselves are the same either way.",
		},
		{
			label: "Flexible Sextant Use", defaultLine: gameplayDefaultOffLine,
			detail: "On: the sextant works by day or night, on the world map and inside surface towns. " +
				"Off: only at night on the world map, as in the original. " +
				"It never works in dungeons or the Underworld.",
		},
		{
			label: "Ankh Visit Bonus", defaultLine: gameplayDefaultOffLine,
			detail: "An old players' tale says an Ankh draws Lord British to your camp; " +
				"the original shows no sign of it. On: each living member wearing an Ankh " +
				"makes his visit a little more likely. Off: the original odds.",
		},
		{
			label:     "Deed Experience",
			helpLines: deedExperienceHelpLines(),
			numeric: &gameplayNumericSetting{
				spec:   config.DeedExperienceSpec(),
				get:    func(s config.GameplayConfiguration) float64 { return float64(s.DeedExperience) },
				set:    func(s *config.GameplayConfiguration, value float64) { s.DeedExperience = int(value) },
				format: config.FormatDeedExperience,
			},
		},
		{
			label: "Share Kill Experience", defaultLine: gameplayDefaultOffLine,
			detail: "On: every living member shares a kill's experience equally. " +
				"Off: it all goes to whoever struck the killing blow, as in the original.",
		},
		{
			label:     "Combat Speed",
			helpLines: combatSpeedHelpLines(config.CombatSpeedSpec()),
			numeric: &gameplayNumericSetting{
				spec:   config.CombatSpeedSpec(),
				get:    func(s config.GameplayConfiguration) float64 { return s.CombatSpeedFactor },
				set:    func(s *config.GameplayConfiguration, value float64) { s.CombatSpeedFactor = value },
				format: config.FormatCombatSpeed,
			},
		},
		{
			label:     "Combat Key Repeat",
			helpLines: combatKeyRepeatHelpLines(),
			numeric: &gameplayNumericSetting{
				spec:   config.CombatKeyRepeatSpec(),
				get:    func(s config.GameplayConfiguration) float64 { return float64(s.CombatKeyRepeat) },
				set:    func(s *config.GameplayConfiguration, value float64) { s.CombatKeyRepeat = int(value) },
				format: formatCombatKeyRepeat,
			},
		},
		{
			label:     "Combat Panels",
			helpLines: combatPanelsHelpLines(),
			numeric: &gameplayNumericSetting{
				spec:   config.CombatPanelsSpec(),
				get:    func(s config.GameplayConfiguration) float64 { return float64(s.CombatPanels) },
				set:    func(s *config.GameplayConfiguration, value float64) { s.CombatPanels = int(value) },
				format: config.FormatCombatPanels,
			},
		},
		{
			label: "Confirm Quick Save",
			helpLines: []string{
				"On: Q asks before it saves, and Ctrl+Q before it loads.", "Off: Q saves and Ctrl+Q loads at once.", "",
				"Either way they use the slot you last saved or loaded.", "",
				gameplayDefaultOnLine,
			},
		},
		{
			label: "Show Combat HP Bars",
			helpLines: []string{
				"Show a health bar under each injured figure in combat.", "",
				"Off hides the bars and adds injury descriptions to the battle report.", "",
				"Damage numbers and fleeing indicators remain visible.", "",
				gameplayDefaultOnLine,
			},
		},
		{
			label: "Battle Frame",
			helpLines: []string{
				"Frame the battlefield.", "",
				"Red while the battle is undecided.", "Blue once it is won.", "",
				"Off: no frame.", "",
				gameplayDefaultOnLine,
			},
		},
		{
			label: "Battle Experience",
			helpLines: []string{
				"List experience earned after each battle.", "",
				"On: Battle complete shows each member's share.",
				"Off: it lists only what was collected.", "",
				gameplayDefaultOnLine,
			},
		},
		{
			label:     "Maximum Action Popups",
			helpLines: actionPopupsHelpLines(config.ActionPopupsSpec()),
			numeric: &gameplayNumericSetting{
				spec: config.ActionPopupsSpec(),
				get:  func(s config.GameplayConfiguration) float64 { return float64(s.MaxVisibleActionPopups) },
				set:  func(s *config.GameplayConfiguration, value float64) { s.MaxVisibleActionPopups = int(value) },
				format: func(value float64) string {
					if value == 0 {
						return gameplaySettingOffLabel
					}

					return strconv.Itoa(int(value))
				},
			},
		},
		{
			label: "Simple Notes Mode",
			helpLines: []string{
				"On: the Journal shows only My Notes.", "Off: every tab.", "",
				"Nothing is deleted.", "",
				gameplayDefaultOffLine,
			},
		},
	}
}

// Update processes input and updates dialog state.
func (d *ConfigurationDialog) Update() {
	if !d.isVisible || !d.hasFocus {
		return
	}
	if d.updateDisplayWorkflow() {
		return
	}
	if d.updateAudioDiagnostics() {
		return
	}
	if d.confirmingDeedExperience {
		d.updateDeedExperienceConfirmation()

		return
	}
	if key, owned := d.navigation.Poll(d.keyboard); owned {
		if key != nil {
			d.navigateConfigurationRows(*key)
		}

		return
	}
	switch d.currentTab {
	case audioSettingsGameplayTab:
		d.updateGameplayTab()
	case audioSettingsAudioTab:
		d.updateAudioTab()
	case audioSettingsDisplayTab:
		d.updateDisplayTab()
	default:
		panic("unsupported Configuration section")
	}
}

func (d *ConfigurationDialog) navigateConfigurationRows(key ebiten.Key) {
	if key != ebiten.KeyUp && key != ebiten.KeyDown {
		return // These compact setting lists have no paging action.
	}
	switch d.currentTab {
	case audioSettingsGameplayTab:
		d.updateGameplayControl(key)
	case audioSettingsAudioTab:
		d.updateAudioControl(key)
	case audioSettingsDisplayTab:
		if key == ebiten.KeyUp {
			d.focusedDisplayIndex = max(0, d.focusedDisplayIndex-1)
		} else if d.stagedDisplay.Mode == config.ModeWindowed {
			d.focusedDisplayIndex = min(configurationDisplayRowCount-1, d.focusedDisplayIndex+1)
		}
	}
}

func (d *ConfigurationDialog) updateGameplayTab() {
	boundKeys := []ebiten.Key{
		ebiten.KeyEscape,
		ebiten.KeyEnter,
		ebiten.KeyNumpadEnter,
		ebiten.KeyTab,
		ebiten.KeySpace,
		ebiten.KeyLeft,
		ebiten.KeyRight,
	}
	boundKey := d.keyboard.GetBoundKeyPressed(&boundKeys)
	if boundKey == nil || !d.keyboard.TryToRegisterKeyPress(*boundKey) {
		return
	}
	d.updateGameplayControl(*boundKey)
}

func (d *ConfigurationDialog) updateGameplayControl(key ebiten.Key) {
	//nolint:exhaustive // The keyboard lookup restricts input to this tab's bindings.
	switch key {
	case ebiten.KeyEscape:
		d.onCancelClicked()
	case ebiten.KeyEnter, ebiten.KeyNumpadEnter:
		d.onOKClicked()
	case ebiten.KeyTab:
		d.cycleConfigurationTab()
	case ebiten.KeyUp:
		if d.focusedGameplayIndex > 0 {
			d.focusedGameplayIndex--
		}
	case ebiten.KeyDown:
		if d.focusedGameplayIndex < audioSettingsGameplayCount-1 {
			d.focusedGameplayIndex++
		}
	case ebiten.KeySpace:
		d.toggleFocusedGameplaySetting()
	case ebiten.KeyLeft:
		d.adjustGameplaySetting(-1)
	case ebiten.KeyRight:
		d.adjustGameplaySetting(1)
	default:
		return
	}
	d.ensureGameplaySelectionVisible()
}

func (d *ConfigurationDialog) adjustGameplaySetting(direction int) {
	numeric := gameplaySettings()[d.focusedGameplayIndex].numeric
	if numeric == nil {
		return
	}
	numeric.set(&d.stagedGameplay, numeric.spec.Advance(numeric.get(d.stagedGameplay), direction))
	d.saveError = ""
}

func (d *ConfigurationDialog) updateAudioTab() {
	boundKeys := []ebiten.Key{
		ebiten.KeyD,
		ebiten.KeyEscape,
		ebiten.KeyEnter,
		ebiten.KeyNumpadEnter,
		ebiten.KeyLeft,
		ebiten.KeyRight,
		ebiten.KeyTab,
	}
	boundKey := d.keyboard.GetBoundKeyPressed(&boundKeys)
	if boundKey == nil || !d.keyboard.TryToRegisterKeyPress(*boundKey) {
		return
	}

	//nolint:exhaustive // The keyboard lookup above restricts input to this tab's bindings.
	switch *boundKey {
	case ebiten.KeyD:
		if d.ultimaConfig.DeveloperTools {
			d.openAudioDiagnostics()
		}
	case ebiten.KeyEscape:
		d.onCancelClicked()
	case ebiten.KeyEnter, ebiten.KeyNumpadEnter:
		d.onOKClicked()
	case ebiten.KeyTab:
		d.cycleConfigurationTab()
	default:
		d.updateAudioControl(*boundKey)
	}
}

func (d *ConfigurationDialog) cycleConfigurationTab() {
	d.navigation.Reset()
	previous := d.currentTab
	direction := 1
	if d.keyboard.IsModifierPressed(input.ModifierShift) {
		direction = -1
	}
	d.currentTab = (d.currentTab + direction + len(d.tabNames)) % len(d.tabNames)
	if previous == audioSettingsAudioTab {
		d.leaveAudioTab()
	}
	if d.currentTab == audioSettingsAudioTab {
		d.enablePreviewAudio()
		d.updateSliderFocus()
	}
}

func (d *ConfigurationDialog) updateAudioControl(key ebiten.Key) {
	//nolint:exhaustive // This helper receives only directional keys from updateAudioTab.
	switch key {
	case ebiten.KeyUp:
		if d.focusedSliderIndex > 0 {
			d.focusedSliderIndex--
			d.updateSliderFocus()
		}
	case ebiten.KeyDown:
		if d.focusedSliderIndex < audioSettingsSliderCount-1 {
			d.focusedSliderIndex++
			d.updateSliderFocus()
		}
	case ebiten.KeyLeft:
		d.adjustFocusedSlider(-1)
	case ebiten.KeyRight:
		d.adjustFocusedSlider(1)
	default:
		return
	}
}

// Draw renders the dialog to the screen.
func (d *ConfigurationDialog) Draw(screen *ebiten.Image) {
	if !d.isVisible || screen == nil {
		return
	}
	d.ensureLayout(screen.Bounds().Dx(), screen.Bounds().Dy())
	if d.displayPhase == configurationDisplayConfirming {
		d.drawDisplayConfirmation(screen)

		return
	}
	widgets.FillClassicPixelRoundedRect(
		screen,
		d.layout.cardRect,
		d.layout.pixelUnit,
		ucolor.DialogBackground.RGBA(),
	)
	d.titleOutput.Reset()
	d.titleOutput.AppendLine().Text("Configuration", widgets.DialogHeaderColor).Done()
	d.titleOutput.DrawPanelContent(
		screen,
		image.Pt(d.layout.titleRect.Min.X, centeredAudioOutputY(d.layout.titleRect, d.titleOutput)),
		d.layout.titleRect.Dx(),
		etext.AlignCenter,
	)
	d.drawTabs(screen)
	switch d.currentTab {
	case audioSettingsGameplayTab:
		d.drawGameplayContent(screen)
	case audioSettingsAudioTab:
		if d.audioReport != nil {
			d.drawAudioDiagnostics(screen)
		} else {
			d.drawAudioContent(screen)
		}
	case audioSettingsDisplayTab:
		d.drawDisplayContent(screen)
	default:
		panic("unsupported Configuration section")
	}
	d.drawFooter(screen)
	widgets.DrawClassicPixelRoundedFrame(
		screen,
		d.layout.cardRect,
		d.layout.pixelUnit,
		widgets.DialogRuleColor,
	)
}

func (d *ConfigurationDialog) drawDisplayContent(screen *ebiten.Image) {
	labels, values := d.displayContentRows()
	d.drawDisplayHeading(screen, d.layout.displayHeading, "DISPLAY MODE")
	d.drawDisplayModeCards(screen)
	d.drawDisplayHeading(screen, d.layout.displaySizeHeading, strings.ToUpper(labels[configurationDisplaySizeRow]))
	index := configurationDisplaySizeRow
	textColor := widgets.DialogValueColor
	labelColor := widgets.DialogLabelColor
	if index == d.focusedDisplayIndex && d.stagedDisplay.Mode == config.ModeWindowed {
		widgets.FillClassicPixelRoundedRect(
			screen, d.layout.displaySelections[index], d.layout.pixelUnit, widgets.DialogSelectedBackground,
		)
		textColor = widgets.DialogSelectedText
		labelColor = widgets.DialogSelectedLabelColor
	}
	labelRect := d.layout.displayLabels[index]
	d.labelOutput.Reset()
	d.labelOutput.AppendLine().Text(labels[index], labelColor).Done()
	d.labelOutput.DrawPanelContent(
		screen, image.Pt(labelRect.Min.X, centeredAudioOutputY(labelRect, d.labelOutput)), labelRect.Dx(), etext.AlignStart,
	)
	valueRect := d.layout.displayValues[index]
	d.valueOutput.Reset()
	d.valueOutput.AppendLine().Text(values[index], textColor).Done()
	d.valueOutput.DrawPanelContent(
		screen, image.Pt(valueRect.Min.X, centeredAudioOutputY(valueRect, d.valueOutput)), valueRect.Dx(), etext.AlignStart,
	)
	d.drawDisplayHelp(screen)
}

func (d *ConfigurationDialog) drawDisplayHeading(screen *ebiten.Image, rect image.Rectangle, heading string) {
	d.presetOutput.Reset()
	d.presetOutput.AppendLine().Text(heading, widgets.DialogHeaderColor).Done()
	d.presetOutput.DrawPanelContent(screen, rect.Min, rect.Dx(), etext.AlignStart)
}

// displayModeCard is one display mode as a card: a small picture of the game
// on a display, its name and what it does.
type displayModeCard struct {
	mode config.Mode
	name string
	note string
	// gapEighths is how far the game's box stands in from the display's edge,
	// in eighths of the picture's height; zero fills the display.
	gapEighths int
}

func displayModeCards() [3]displayModeCard {
	const windowedGap, maximizedGap = 2, 1

	return [...]displayModeCard{
		{mode: config.ModeWindowed, name: "Windowed", note: "Pick a size", gapEighths: windowedGap},
		{mode: config.ModeMaximized, name: "Maximized", note: "Fill the desktop", gapEighths: maximizedGap},
		{mode: config.ModeFullscreen, name: "Fullscreen", note: "Fill the display", gapEighths: 0},
	}
}

// drawDisplayModeCards shows every display mode at once. The chosen mode's
// card is framed in white, or filled while the cards have the cursor; the
// others are dim.
func (d *ConfigurationDialog) drawDisplayModeCards(screen *ebiten.Image) {
	row := d.layout.displayRows[configurationDisplayModeRow]
	unit := d.layout.pixelUnit
	gap := unit * audioSettingsPaddingUnits
	cards := displayModeCards()
	width := (row.Dx() - gap*(len(cards)-1)) / len(cards)
	for index, card := range cards {
		left := row.Min.X + index*(width+gap)
		rect := image.Rect(left, row.Min.Y, left+width, row.Max.Y)
		ink, noteInk := widgets.DialogEmptyColor, widgets.DialogEmptyColor
		switch {
		case card.mode != d.stagedDisplay.Mode:
			widgets.DrawClassicPixelRoundedFrame(screen, rect, unit, widgets.DialogEmptyColor)
		case d.focusedDisplayIndex == configurationDisplayModeRow:
			widgets.FillClassicPixelRoundedRect(screen, rect, unit, widgets.DialogSelectedBackground)
			ink, noteInk = widgets.DialogSelectedText, widgets.DialogSelectedText
		default:
			widgets.DrawClassicPixelRoundedFrame(screen, rect, unit, widgets.DialogRuleColor)
			ink, noteInk = widgets.DialogValueColor, widgets.DialogControlHintColor
		}
		d.drawDisplayModeCard(screen, rect, card, ink, noteInk)
	}
}

func (d *ConfigurationDialog) drawDisplayModeCard(
	screen *ebiten.Image, rect image.Rectangle, card displayModeCard, ink, noteInk color.Color,
) {
	unit := d.layout.pixelUnit
	inset := unit * audioSettingsPaddingUnits
	line := int(math.Ceil(d.detailOutput.GetTotalHeightForLines(1)))
	// The picture: a display's outline with the game's box inside it.
	const pictureWidthLines, pictureHeightLines = 3, 2
	picture := image.Rect(0, 0, line*pictureWidthLines, line*pictureHeightLines).
		Add(image.Pt(rect.Min.X+(rect.Dx()-line*pictureWidthLines)/audioSettingsCenterDivisor, rect.Min.Y+inset))
	fillAudioRect(screen, picture, ink)
	fillAudioRect(screen, picture.Inset(unit), ucolor.EGABlack)
	if d.cardIsFilled(card) {
		fillAudioRect(screen, picture.Inset(unit), widgets.DialogSelectedBackground)
	}
	const pictureGapDivisor = 8
	gameGap := card.gapEighths * picture.Dy() / pictureGapDivisor
	if gameGap > 0 {
		gameGap += unit // Clear of the display's outline.
	}
	fillAudioRect(screen, picture.Inset(gameGap), ink)
	textWidth := rect.Dx() - inset*audioSettingsCenterDivisor
	d.detailOutput.SetMaxCharsPerLine(max(1, d.detailOutput.Font.CharsForWidth(textWidth)))
	d.detailOutput.Reset()
	d.detailOutput.AppendLine().WithAlignment(etext.AlignCenter).Text(card.name, ink).Done()
	d.detailOutput.AppendLine().WithAlignment(etext.AlignCenter).Text(card.note, noteInk).WordWrap().Done()
	d.detailOutput.DrawPanelContent(
		screen, image.Pt(rect.Min.X+inset, picture.Max.Y+inset), textWidth, etext.AlignCenter,
	)
}

// cardIsFilled reports the chosen mode's card while the cards have the cursor.
func (d *ConfigurationDialog) cardIsFilled(card displayModeCard) bool {
	return card.mode == d.stagedDisplay.Mode && d.focusedDisplayIndex == configurationDisplayModeRow
}

func fillAudioRect(screen *ebiten.Image, rect image.Rectangle, clr color.Color) {
	if rect.Empty() {
		return
	}
	vector.FillRect(screen, float32(rect.Min.X), float32(rect.Min.Y), float32(rect.Dx()), float32(rect.Dy()), clr, false)
}

func (d *ConfigurationDialog) displayContentRows() ([2]string, [2]string) {
	labels := [...]string{"Display mode", "Window size"}
	values := [...]string{
		"<  " + displayModeLabel(d.stagedDisplay.Mode) + "  >",
		"<  " + d.displaySizeLabel() + "  >",
	}
	if d.stagedDisplay.Mode != config.ModeWindowed {
		labels[configurationDisplaySizeRow] = "Output size"
		values[configurationDisplaySizeRow] = d.displaySizeLabel()
	}

	return labels, values
}

func (d *ConfigurationDialog) drawDisplayHelp(screen *ebiten.Image) {
	// The help sits in a framed box, like the explanation on the Gameplay tab.
	inset := max(1, d.layout.pixelUnit*audioSettingsCenterDivisor) * audioSettingsCenterDivisor
	frame := d.layout.displayHelp
	rect := frame.Inset(inset)
	d.detailOutput.SetMaxCharsPerLine(max(1, d.detailOutput.Font.CharsForWidth(rect.Dx())))
	d.detailOutput.Reset()
	lines := make([]string, 0, audioSettingsDetailLines)
	lines = append(lines, "Display mode", "", "Windowed: choose a size or resize normally.", "",
		"Maximized: use available desktop space.", "", "Fullscreen: fill the display.")
	if d.focusedDisplayIndex == configurationDisplaySizeRow && d.stagedDisplay.Mode == config.ModeWindowed {
		lines = append(lines[:0], "Window size", "", "Choose the image size on your desktop.",
			"You can also resize the window normally.", "",
			"Presets leave room on the desktop. Maximize gives the largest window.", "",
			"About means pixel rounding at this display scale.")
	}
	lines = append(lines, "", "Fits any size from 720p to 4K.",
		"The picture stays 16:9, so black bars may appear.")
	for index, line := range lines {
		color := widgets.DialogControlHintColor
		if index == 0 {
			color = widgets.DialogLabelColor
		}
		d.detailOutput.AppendLine().Text(line, color).WordWrap().Done()
	}
	written := int(math.Ceil(d.detailOutput.GetTotalHeightForLines(d.detailOutput.GetPanelContentLineCount())))
	frame.Max.Y = min(frame.Max.Y, rect.Min.Y+written+inset)
	widgets.DrawClassicPixelRoundedFrame(screen, frame, d.layout.pixelUnit, widgets.DialogRuleColor)
	d.detailOutput.DrawPanelContent(screen, rect.Min, rect.Dx(), etext.AlignStart)
}

func (d *ConfigurationDialog) drawDisplayConfirmation(screen *ebiten.Image) {
	// This view owns the whole body, not the Gameplay detail column.
	d.detailOutput.SetMaxCharsPerLine(max(1, d.detailOutput.Font.CharsForWidth(d.layout.bodyRect.Dx())))
	widgets.FillClassicPixelRoundedRect(
		screen, d.layout.cardRect, d.layout.pixelUnit, ucolor.DialogBackground.RGBA(),
	)
	d.titleOutput.Reset()
	d.titleOutput.AppendLine().Text("Keep Display Settings?", widgets.DialogHeaderColor).Done()
	d.titleOutput.DrawPanelContent(
		screen,
		image.Pt(d.layout.titleRect.Min.X, centeredAudioOutputY(d.layout.titleRect, d.titleOutput)),
		d.layout.titleRect.Dx(),
		etext.AlignCenter,
	)
	d.detailOutput.Reset()
	d.detailOutput.AppendLine().
		Text("Display Mode: "+displayModeLabel(d.stagedDisplay.Mode), widgets.DialogValueColor).
		Done()
	logicalWidth, logicalHeight := d.displayManager.LogicalSize()
	windowWidth, windowHeight := display.WindowPixels(
		d.stagedDisplay.WindowWidth, d.stagedDisplay.WindowHeight, d.displayManager.DeviceScaleFactor(),
	)
	if d.stagedDisplay.Mode == config.ModeWindowed {
		d.detailOutput.AppendLine().Text(
			fmt.Sprintf("Window: %d x %d pixels", windowWidth, windowHeight), widgets.DialogValueColor,
		).Done()
	}
	d.detailOutput.AppendLine().Text(
		fmt.Sprintf("Game Area: %d x %d (16:9)", logicalWidth, logicalHeight),
		widgets.DialogValueColor,
	).Done()
	secondsLeft := (d.displayConfirmationTicks + configurationTicksPerSecond - 1) / configurationTicksPerSecond
	revertNotice := fmt.Sprintf("Settings will revert in %d seconds.", secondsLeft)
	if secondsLeft == 1 {
		revertNotice = "Settings will revert in 1 second."
	}
	d.detailOutput.AppendLine().Text(revertNotice, widgets.DialogStatusWarningColor).Done()
	selection := "[ Keep ]        Revert"
	if !d.keepDisplaySettings {
		selection = "Keep        [ Revert ]"
	}
	d.detailOutput.AppendLine().Text(selection, widgets.DialogValueColor).Done()
	d.detailOutput.DrawPanelContent(
		screen,
		image.Pt(d.layout.bodyRect.Min.X, centeredAudioOutputY(d.layout.bodyRect, d.detailOutput)),
		d.layout.bodyRect.Dx(),
		etext.AlignCenter,
	)
	d.footerOutput.Reset()
	widgets.AppendControlHints(d.footerOutput.AppendLine(), "[Left/Right] Select").Done()
	widgets.AppendControlHints(d.footerOutput.AppendLine(), "[Enter] Confirm   [Esc] Revert").Done()
	d.footerOutput.DrawPanelContent(
		screen,
		image.Pt(d.layout.footerRect.Min.X, centeredAudioOutputY(d.layout.footerRect, d.footerOutput)),
		d.layout.footerRect.Dx(),
		etext.AlignCenter,
	)
	widgets.DrawClassicPixelRoundedFrame(
		screen, d.layout.cardRect, d.layout.pixelUnit, widgets.DialogRuleColor,
	)
}

func (d *ConfigurationDialog) drawTabs(screen *ebiten.Image) {
	for index, tabName := range d.tabNames {
		rect, textColor := d.layout.tabRects[index], widgets.DialogTabColor
		if index == d.currentTab {
			textColor = widgets.DialogActiveTabText
		}
		d.tabOutput.Reset()
		d.tabOutput.AppendLine().Text(tabName, textColor).Done()
		top := centeredAudioOutputY(rect, d.tabOutput)
		if index == d.currentTab {
			width, _ := d.tabOutput.Font.MeasureString(tabName)
			widgets.DrawActiveTabPlate(screen, rect, width, top,
				int(math.Ceil(d.tabOutput.GetFontHeight())), d.layout.pixelUnit)
		}
		d.tabOutput.DrawPanelContent(screen, image.Pt(rect.Min.X, top), rect.Dx(), etext.AlignCenter)
	}
}

func (d *ConfigurationDialog) drawGameplayContent(screen *ebiten.Image) {
	settings := gameplaySettings()
	viewport := d.gameplayViewport()
	clipped, ok := screen.SubImage(viewport).(*ebiten.Image)
	if !ok {
		panic("Ebitengine SubImage must return an image")
	}
	for index := range d.layout.gameplayRows {
		rect := d.gameplayVisibleRow(index)
		if !rect.Overlaps(viewport) {
			continue
		}
		textColor := widgets.DialogValueColor
		markColor := widgets.DialogCheckedMarkColor
		if index == d.focusedGameplayIndex {
			widgets.FillClassicPixelRoundedRect(
				clipped, rect, d.layout.pixelUnit, widgets.DialogSelectedBackground,
			)
			textColor = widgets.DialogSelectedText
			markColor = widgets.DialogSelectedCheckedMarkColor
		}
		labelWidth := d.gameplayLabelWidth(d.layout.gameplayListRect.Dx(), d.layout.pixelUnit, settings[index])
		d.gameplayOutput.SetMaxCharsPerLine(d.gameplayLabelChars(labelWidth))
		d.gameplayOutput.Reset()
		d.gameplayOutput.AppendLine().Text(settings[index].label, textColor).WordWrap().Done()
		inset := max(1, d.layout.pixelUnit*audioSettingsCenterDivisor)
		// A value too wide to sit beside the label takes the line under it.
		valueLine := d.gameplayValueLineHeight(d.layout.gameplayListRect.Dx(), d.layout.pixelUnit, settings[index])
		top := centeredAudioOutputY(rect, d.gameplayOutput) - valueLine/audioSettingsCenterDivisor
		d.labelOutput.Reset()
		if settings[index].numeric != nil {
			valueTop := top
			if valueLine > 0 {
				valueTop += int(math.Ceil(d.gameplayOutput.GetTotalHeightForLines(d.gameplayOutput.GetPanelContentLineCount())))
			}
			d.drawGameplayValue(clipped, image.Pt(rect.Max.X-inset, valueTop), settings[index], textColor,
				index == d.focusedGameplayIndex)
		} else {
			d.labelOutput.AppendLine().Text("[", textColor).
				Text(d.gameplayCheckmark(index), markColor).Text("]", textColor).Done()
			d.labelOutput.DrawPanelContent(clipped, image.Pt(rect.Min.X+inset, top), d.gameplayCheckboxWidth(), etext.AlignStart)
		}
		d.gameplayOutput.DrawPanelContent(
			clipped, image.Pt(rect.Min.X+inset+d.gameplayCheckboxWidth(), top), labelWidth, etext.AlignStart,
		)
	}

	d.drawGameplayScrollIndicator(screen)
	d.prepareGameplayDetail()
	margin := gameplayDetailMargin(d.layout.pixelUnit)
	d.detailOutput.DrawPanelContent(
		screen,
		image.Pt(d.layout.gameplayDetail.Min.X+margin, d.layout.gameplayDetail.Min.Y+margin),
		max(1, d.layout.gameplayDetail.Dx()-margin*audioSettingsCenterDivisor),
		etext.AlignStart,
	)
	widgets.DrawClassicPixelRoundedFrame(
		screen, d.layout.gameplayListRect, d.layout.pixelUnit, widgets.DialogRuleColor,
	)
	widgets.DrawClassicPixelRoundedFrame(
		screen, d.layout.gameplayDetail, d.layout.pixelUnit, widgets.DialogRuleColor,
	)
}

// drawGameplayValue draws a numeric row's value between arrows, ending at the row's right edge.
func (d *ConfigurationDialog) drawGameplayValue(
	target *ebiten.Image, topRight image.Point, setting gameplaySettingDefinition, textColor color.Color, selected bool,
) {
	arrowColor := widgets.DialogControlHintColor
	if selected {
		arrowColor = textColor
	}
	valueWidth := d.gameplayValueWidth(setting)
	d.labelOutput.AppendLine().Text("< ", arrowColor).
		Text(setting.numeric.format(setting.numeric.get(d.stagedGameplay)), textColor).Text(" >", arrowColor).Done()
	d.labelOutput.DrawPanelContent(target, image.Pt(topRight.X-valueWidth, topRight.Y), valueWidth, etext.AlignEnd)
}

// prepareGameplayDetail uses the measured layout's wrapping capacity without
// rasterizing. Selection and staged settings remain owned by Update.
func (d *ConfigurationDialog) prepareGameplayDetail() {
	margin := gameplayDetailMargin(d.layout.pixelUnit)
	d.detailOutput.SetMaxCharsPerLine(max(1,
		d.detailOutput.Font.CharsForWidth(d.layout.gameplayDetail.Dx()-margin*audioSettingsCenterDivisor)))
	if d.confirmingDeedExperience {
		d.populateDeedExperienceConfirmation(d.detailOutput)

		return
	}
	selected := gameplaySettings()[d.focusedGameplayIndex]
	populateGameplayDetail(d.detailOutput, selected, d.gameplaySettingDisplayValue())
}

func (d *ConfigurationDialog) gameplaySettingDisplayValue() string {
	if numeric := gameplaySettings()[d.focusedGameplayIndex].numeric; numeric != nil {
		return numeric.format(numeric.get(d.stagedGameplay))
	}
	if d.gameplaySettingValue(d.focusedGameplayIndex) {
		return "On"
	}

	return gameplaySettingOffLabel
}

func (d *ConfigurationDialog) drawGameplayScrollIndicator(screen *ebiten.Image) {
	viewport := d.gameplayViewport()
	last := d.layout.gameplayRows[len(d.layout.gameplayRows)-1]
	contentHeight := last.Max.Y - d.layout.gameplayRows[0].Min.Y
	if contentHeight <= viewport.Dy() {
		return
	}
	width := max(1, d.layout.pixelUnit)
	height := max(width, viewport.Dy()*viewport.Dy()/contentHeight)
	offset := d.layout.gameplayRows[d.firstGameplayIndex].Min.Y - d.layout.gameplayRows[0].Min.Y
	top := viewport.Min.Y + min(viewport.Dy()-height, offset*viewport.Dy()/contentHeight)
	widgets.FillClassicPixelRoundedRect(screen,
		image.Rect(viewport.Max.X, top, viewport.Max.X+width, top+height), 1, widgets.DialogRuleColor)
}

// audioVolumes names each volume row and says where that channel is heard.
func audioVolumes() [audioSettingsSliderCount][2]string {
	return [...][2]string{
		{"Music", "Music: town, travel and battle music."},
		{"Effects", "Effects: blows, spells, doors and other actions."},
		{"Ambient", "Ambient: the background sounds of a place."},
	}
}

// drawAudioContent frames the three volumes as rows of ten blocks under a
// VOLUME heading, with a line saying where the selected channel is heard.
func (d *ConfigurationDialog) drawAudioContent(screen *ebiten.Image) {
	sliders := []*widgets.Slider{d.musicSlider, d.sfxSlider, d.ambientSlider}
	unit := d.layout.pixelUnit
	inset := max(1, unit*audioSettingsCenterDivisor)
	body := d.layout.bodyRect
	line := int(math.Ceil(d.detailOutput.GetTotalHeightForLines(1)))
	last := d.layout.sliderRects[len(sliders)-1]
	ruleTop := last.Max.Y + inset*audioSettingsCenterDivisor
	frame := image.Rect(body.Min.X, body.Min.Y, body.Max.X, ruleTop+inset+line*audioSettingsCenterDivisor+inset)
	widgets.DrawClassicPixelRoundedFrame(screen, frame, unit, widgets.DialogRuleColor)
	heading := image.Rect(body.Min.X+inset*audioSettingsCenterDivisor, body.Min.Y+inset, body.Max.X, body.Max.Y)
	d.drawDisplayHeading(screen, heading, "VOLUME")
	volumes := audioVolumes()
	for index, slider := range sliders {
		row := image.Rect(d.layout.sliderLabelRects[index].Min.X, d.layout.sliderLabelRects[index].Min.Y,
			d.layout.sliderLabelRects[index].Max.X, d.layout.sliderRects[index].Max.Y)
		d.drawVolumeRow(screen, row, volumes[index][0], slider.GetValue(), index == d.focusedSliderIndex)
	}
	left, right := d.layout.sliderLabelRects[0].Min.X, d.layout.sliderLabelRects[0].Max.X
	rule := image.Rect(left, ruleTop, right, ruleTop+max(1, unit/audioSettingsCenterDivisor))
	fillAudioRect(screen, rule, widgets.DialogEmptyColor)
	d.detailOutput.SetMaxCharsPerLine(max(1, d.detailOutput.Font.CharsForWidth(right-left)))
	d.detailOutput.Reset()
	d.detailOutput.AppendLine().Text(volumes[d.focusedSliderIndex][1], widgets.DialogControlHintColor).WordWrap().Done()
	d.detailOutput.DrawPanelContent(screen, image.Pt(left, ruleTop+inset), right-left, etext.AlignStart)
}

// drawVolumeRow draws one volume: its name, ten blocks and its figure. The
// selected row is a filled plate, like a selected row on the other tabs.
func (d *ConfigurationDialog) drawVolumeRow(
	screen *ebiten.Image, row image.Rectangle, name string, value int, selected bool,
) {
	unit := d.layout.pixelUnit
	inset := max(1, unit*audioSettingsCenterDivisor)
	textColor, labelColor := widgets.DialogValueColor, widgets.DialogLabelColor
	var filled, empty color.Color = ucolor.EGALightCyan, ucolor.EGADarkGray
	if selected {
		widgets.FillClassicPixelRoundedRect(screen, row, unit, widgets.DialogSelectedBackground)
		textColor, labelColor = widgets.DialogSelectedText, widgets.DialogSelectedLabelColor
		filled, empty = ucolor.EGABlue, ucolor.EGALightGray
	}
	d.labelOutput.Reset()
	d.labelOutput.AppendLine().Text(name, labelColor).Done()
	d.labelOutput.DrawPanelContent(
		screen, image.Pt(row.Min.X+inset, centeredAudioOutputY(row, d.labelOutput)), row.Dx(), etext.AlignStart,
	)
	valueWidth, _ := d.valueOutput.Font.MeasureString(strconv.Itoa(audioSettingsVolumeSteps))
	d.valueOutput.Reset()
	d.valueOutput.AppendLine().Text(strconv.Itoa(value), textColor).Done()
	d.valueOutput.DrawPanelContent(
		screen, image.Pt(row.Max.X-inset-int(math.Ceil(valueWidth)), centeredAudioOutputY(row, d.valueOutput)),
		int(math.Ceil(valueWidth)), etext.AlignEnd,
	)
	// The blocks fill the space between the widest name and the figure.
	nameWidth := 0.0
	for _, volume := range audioVolumes() {
		width, _ := d.labelOutput.Font.MeasureString(volume[0] + " ")
		nameWidth = max(nameWidth, width)
	}
	blocksLeft := row.Min.X + inset + int(math.Ceil(nameWidth))
	blocksRight := row.Max.X - inset*audioSettingsCenterDivisor - int(math.Ceil(valueWidth))
	blockWidth := (blocksRight - blocksLeft - unit*(audioSettingsVolumeSteps-1)) / audioSettingsVolumeSteps
	blockHeight := row.Dy() / audioSettingsCenterDivisor
	top := row.Min.Y + (row.Dy()-blockHeight)/audioSettingsCenterDivisor
	for step := range audioSettingsVolumeSteps {
		clr := empty
		if step < value {
			clr = filled
		}
		left := blocksLeft + step*(blockWidth+unit)
		fillAudioRect(screen, image.Rect(left, top, left+blockWidth, top+blockHeight), clr)
	}
}

func (d *ConfigurationDialog) drawFooter(screen *ebiten.Image) {
	d.footerOutput.Reset()
	switch {
	case d.audioReport != nil:
		hints := []string{"[Up/Down] Scroll   [PgUp/PgDn] Page", "[R] Refresh captured state", "[D/Esc] Back to Sound"}
		for _, hint := range hints {
			widgets.AppendControlHints(d.footerOutput.AppendLine(), hint).Done()
		}
	case d.displayPhase == configurationDisplayApplying:
		d.footerOutput.AppendLine().
			Text("Applying display settings...", widgets.DialogStatusWarningColor).
			Done()
	case d.displayPhase == configurationDisplayReverting:
		d.footerOutput.AppendLine().
			Text("Restoring previous display settings...", widgets.DialogStatusWarningColor).
			Done()
	case d.saveError != "":
		d.footerOutput.AppendLine().
			Text(d.saveError, widgets.DialogStatusWarningColor).
			WordWrap().
			Done()
		widgets.AppendControlHints(d.footerOutput.AppendLine(), d.configurationErrorGuidance()).Done()
	case d.confirmingDeedExperience:
		widgets.AppendControlHints(d.footerOutput.AppendLine(), deedExperienceConfirmHint).Done()
	default:
		d.appendConfigurationHints()
	}
	d.footerOutput.DrawPanelContent(
		screen,
		image.Pt(
			d.layout.footerRect.Min.X,
			centeredAudioOutputY(d.layout.footerRect, d.footerOutput),
		),
		d.layout.footerRect.Dx(),
		etext.AlignCenter,
	)
}

// appendConfigurationHints adds the two hint rows: what moves and changes a
// setting, then how to leave. A key that does nothing on the selected row is dimmed.
func (d *ConfigurationDialog) appendConfigurationHints() {
	onCheckbox := d.currentTab == audioSettingsGameplayTab && d.gameplayCheckbox(d.focusedGameplayIndex) != nil
	onStepped := d.currentTab == audioSettingsGameplayTab && !onCheckbox
	const leftRight, tab = "Left/Right", "Tab"
	first := []hintKey{{"Up/Down", "Select", true}}
	switch d.currentTab {
	case audioSettingsGameplayTab:
		first = append(first, hintKey{"Space", "Toggle", !onStepped}, hintKey{leftRight, "Adjust", !onCheckbox})
	case audioSettingsAudioTab:
		first = append(first, hintKey{leftRight, "Adjust", true})
		if d.ultimaConfig.DeveloperTools {
			first = append(first, hintKey{"D", "Diagnostics", true})
		}
	case audioSettingsDisplayTab:
		first = append(first, hintKey{leftRight, "Change", true})
	default:
		panic("unsupported Configuration section")
	}
	first = append(first, hintKey{tab, "Section", true})
	rows := [][]hintKey{first, {{"Enter", "Apply", true}, {"Esc", "Cancel", true}}}
	for _, row := range rows {
		line := d.footerOutput.AppendLine()
		for index, hint := range row {
			keyColor, hintColor := widgets.DialogControlKeyColor, widgets.DialogControlHintColor
			if !hint.works {
				keyColor, hintColor = widgets.DialogUnavailableItemColor, widgets.DialogUnavailableItemColor
			}
			if index > 0 {
				line.Text("  ", hintColor)
			}
			line.Text("["+hint.key+"]", keyColor).Text(" "+hint.action, hintColor)
		}
		line.Done()
	}
}

// hintKey is one footer hint; works is false when the key does nothing now.
type hintKey struct {
	key, action string
	works       bool
}

func (d *ConfigurationDialog) configurationErrorGuidance() string {
	if d.displayRollbackRetry {
		return "[Enter] Retry restore   [Esc] Cancel"
	}

	return "[Enter] Retry   [Esc] Cancel"
}

func (d *ConfigurationDialog) gameplayCheckmark(index int) string {
	if d.gameplaySettingValue(index) {
		return "X"
	}

	return " "
}

// gameplayCheckbox returns the staged value behind a checkbox row, or nil for a
// stepped row. One table serves both reading and toggling.
func (d *ConfigurationDialog) gameplayCheckbox(index int) *bool {
	staged := &d.stagedGameplay

	return map[int]*bool{
		gameplayRepeatedDivisionIndex:      &staged.AllowRepeatedMonsterDivision,
		gameplayHiddenDiscoveriesIndex:     &staged.PersistHiddenDungeonDiscoveries,
		gameplayUnlitDungeonsIndex:         &staged.ShowSubduedUnlitDungeons,
		gameplaySmoothSurfaceMovementIndex: &staged.SmoothSurfaceMovement,
		gameplayBatchMerchantTheftIndex:    &staged.BatchMerchantTheft,
		gameplayConcealedTurnSlotsIndex:    &staged.ShowConcealedTurnOrderSlots,
		gameplayFlexibleSextantUseIndex:    &staged.FlexibleSextantUse,
		gameplayAnkhWandererBonusIndex:     &staged.AnkhWandererBonus,
		gameplayShareKillExperienceIndex:   &staged.ShareKillExperience,
		gameplayConfirmQuickSaveIndex:      &staged.ConfirmQuickSave,
		gameplayShowCombatHPBarsIndex:      &staged.ShowCombatHPBars,
		gameplayBattleFrameIndex:           &staged.ShowCombatBattleFrame,
		gameplayBattleExperienceIndex:      &staged.ShowBattleExperience,
		gameplaySimpleNotesModeIndex:       &staged.SimpleNotesMode,
	}[index]
}

func (d *ConfigurationDialog) gameplaySettingValue(index int) bool {
	checkbox := d.gameplayCheckbox(index)
	if checkbox == nil {
		panic("unsupported Gameplay setting index")
	}

	return *checkbox
}

func (d *ConfigurationDialog) toggleFocusedGameplaySetting() {
	checkbox := d.gameplayCheckbox(d.focusedGameplayIndex)
	if checkbox == nil {
		if d.focusedGameplayIndex < 0 || d.focusedGameplayIndex >= audioSettingsGameplayCount ||
			gameplaySettings()[d.focusedGameplayIndex].numeric == nil {
			panic("unsupported Gameplay setting index")
		}

		return
	}
	*checkbox = !*checkbox
	d.saveError = ""
}

func centeredAudioOutputY(rect image.Rectangle, output *text.Output) int {
	lineCount := output.GetPanelContentLineCount()
	if lineCount == 0 {
		lineCount = 1
	}
	contentHeight := int(math.Ceil(output.GetTotalHeightForLines(lineCount)))

	return rect.Min.Y + max(0, (rect.Dy()-contentHeight)/audioSettingsCenterDivisor)
}
