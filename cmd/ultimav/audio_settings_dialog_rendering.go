package main

import (
	"fmt"
	"image"
	"math"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

type gameplaySettingDefinition struct {
	label     string
	detail    string
	helpLines []string
	numeric   *gameplayNumericSetting
}

type gameplayNumericSetting struct {
	spec   config.NumericSettingSpec
	get    func(config.GameplayConfiguration) float64
	set    func(*config.GameplayConfiguration, float64)
	format func(float64) string
}

const (
	gameplaySettingOffLabel = "Off"
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
	gameplayCombatSpeedIndex
	gameplayConfirmQuickSaveIndex
	gameplayShowCombatHPBarsIndex
	gameplayMaxActionPopupsIndex
	gameplaySimpleNotesModeIndex
)

//nolint:exhaustruct_v5,funlen // Explicit row catalog; boolean rows have no numeric accessors.
func gameplaySettings() [audioSettingsGameplayCount]gameplaySettingDefinition {
	return [...]gameplaySettingDefinition{
		{
			label: "Repeated Monster Division",
			detail: "Original: surviving Slimes and Gargoyles can divide repeatedly. " +
				"Off keeps Redux's one-child split limit; On restores repeated division.",
		},
		{
			label: "Remember Hidden Dungeon Discoveries",
			detail: "Remember discovered hidden doors and pits between visits. " +
				"On: keep discoveries. Off: reset each fresh visit.",
		},
		{
			label: "Show Subdued Unlit Dungeons",
			detail: "Original: an unlit dungeon viewport is black. " +
				"On shows subdued geometry without granting light; Off restores binary darkness.",
		},
		{
			label: "Batch Merchant Theft",
			detail: "On: one Faulinei theft per paid batch. Off: original theft per item, bundle or patient; " +
				"theft can stop a batch early. Charity requires a separate review.",
		},
		{
			label: "Smooth Surface Movement",
			detail: "Scroll while walking, riding, flying or boating. " +
				"Mounts animate 25% faster; automatic sailing follows its cadence. " +
				"Gameplay timing, combat and dungeons are unchanged.",
		},
		{
			label: "Concealed Turn Slots",
			detail: "On reserves blank NEXT slots for concealed enemies. " +
				"Off compacts the four-action forecast to avoid revealing hidden actors. " +
				"Scheduling is unchanged.",
		},
		{
			label: "Flexible Sextant Use",
			detail: "Redux: day or night, including surface settlement interiors. Sextant required. " +
				"No dungeons or Underworld. Off: night world-map use.",
		},
		{
			label: "Ankh Visit Bonus",
			detail: "Folk tale: an Ankh draws Lord British to camp. No evidence the original did this. " +
				"On: each living Ankh wearer adds 5% to the 25% visit chance. Off: the original 25%.",
		},
		{
			label:     "Combat Speed",
			helpLines: combatSpeedHelpLines(config.CombatSpeedSpec()),
			numeric: &gameplayNumericSetting{
				spec:   config.CombatSpeedSpec(),
				get:    func(s config.GameplayConfiguration) float64 { return s.CombatSpeedFactor },
				set:    func(s *config.GameplayConfiguration, value float64) { s.CombatSpeedFactor = value },
				format: formatCombatSpeed,
			},
		},
		{
			label: "Confirm Quick Save",
			helpLines: []string{
				"Ask before saving with Q.", "",
				"On: Ask before saving.", "Off: Save immediately.", "",
				"Q saves to the last slot you loaded or saved.", "",
				"Default: On",
			},
		},
		{
			label: "Show Combat HP Bars",
			helpLines: []string{
				"Show health bars during combat.", "",
				"Off hides the bars and adds injury descriptions to the battle report.", "",
				"Damage numbers and fleeing indicators remain visible.", "",
				"Default: On",
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
				"Journal shows only My Notes.", "",
				"On: My Notes only.", "Off: all tabs.", "",
				"Nothing is deleted.", "",
				"Default: Off",
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
		d.openAudioDiagnostics()
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
	d.presetOutput.Reset()
	d.presetOutput.AppendLine().Text("WINDOW", widgets.DialogHeaderColor).Done()
	d.presetOutput.DrawPanelContent(screen, d.layout.displayHeading.Min, d.layout.displayHeading.Dx(), etext.AlignStart)
	labels, values := d.displayContentRows()
	for index := range d.layout.displayRows {
		selected := index == d.focusedDisplayIndex &&
			(index == configurationDisplayModeRow || d.stagedDisplay.Mode == config.ModeWindowed)
		textColor := widgets.DialogValueColor
		labelColor := widgets.DialogLabelColor
		if selected {
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
			screen,
			image.Pt(labelRect.Min.X, centeredAudioOutputY(labelRect, d.labelOutput)),
			labelRect.Dx(),
			etext.AlignStart,
		)
		valueRect := d.layout.displayValues[index]
		d.valueOutput.Reset()
		d.valueOutput.AppendLine().Text(values[index], textColor).Done()
		d.valueOutput.DrawPanelContent(
			screen,
			image.Pt(valueRect.Min.X, centeredAudioOutputY(valueRect, d.valueOutput)),
			valueRect.Dx(),
			etext.AlignStart,
		)
	}
	d.drawDisplayHelp(screen)
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
	rect := d.layout.displayHelp
	d.detailOutput.SetMaxCharsPerLine(max(1, d.detailOutput.Font.CharsForWidth(rect.Dx())))
	d.detailOutput.Reset()
	lines := make([]string, 0, audioSettingsDetailLines)
	lines = append(lines, "Display mode", "", "Windowed: choose a size or resize normally.", "",
		"Maximized: use available desktop space.", "", "Fullscreen: fill the display.")
	if d.focusedDisplayIndex == configurationDisplaySizeRow && d.stagedDisplay.Mode == config.ModeWindowed {
		lines = append(lines[:0], "Window size", "", "Choose the image size on your desktop.",
			"You can also resize the window normally.", "",
			"Presets reserve desktop space. Use Maximize for the largest window.", "",
			"About means pixel rounding at this display scale.")
	}
	lines = append(lines, "", "Automatic pixel density; 720p to 4K.",
		"Game shape stays 16:9; black bars may appear.")
	for index, line := range lines {
		color := widgets.DialogControlHintColor
		if index == 0 {
			color = widgets.DialogLabelColor
		}
		d.detailOutput.AppendLine().Text(line, color).WordWrap().Done()
	}
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
	d.detailOutput.AppendLine().Text(
		fmt.Sprintf(
			"Settings will revert in %d seconds.",
			(d.displayConfirmationTicks+configurationTicksPerSecond-1)/configurationTicksPerSecond,
		),
		widgets.DialogStatusWarningColor,
	).Done()
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
		textColor := widgets.DialogControlHintColor
		if index == d.currentTab {
			widgets.FillClassicPixelRoundedRect(
				screen,
				d.layout.tabRects[index],
				d.layout.pixelUnit,
				widgets.DialogSelectedBackground,
			)
			textColor = widgets.DialogSelectedText
		}
		d.tabOutput.Reset()
		d.tabOutput.AppendLine().Text(tabName, textColor).Done()
		d.tabOutput.DrawPanelContent(
			screen,
			image.Pt(
				d.layout.tabRects[index].Min.X,
				centeredAudioOutputY(d.layout.tabRects[index], d.tabOutput),
			),
			d.layout.tabRects[index].Dx(),
			etext.AlignCenter,
		)
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
		d.gameplayOutput.Reset()
		d.gameplayOutput.AppendLine().Text(settings[index].label, textColor).WordWrap().Done()
		inset := max(1, d.layout.pixelUnit*audioSettingsCenterDivisor)
		top := centeredAudioOutputY(rect, d.gameplayOutput)
		d.labelOutput.Reset()
		if numeric := settings[index].numeric; numeric != nil {
			d.labelOutput.AppendLine().Text(numeric.format(numeric.get(d.stagedGameplay)), textColor).Done()
		} else {
			d.labelOutput.AppendLine().Text("[", textColor).
				Text(d.gameplayCheckmark(index), markColor).Text("]", textColor).Done()
		}
		d.labelOutput.DrawPanelContent(clipped, image.Pt(rect.Min.X+inset, top), d.gameplayCheckboxWidth(), etext.AlignStart)
		d.gameplayOutput.DrawPanelContent(
			clipped,
			image.Pt(rect.Min.X+inset+d.gameplayCheckboxWidth(), top),
			d.gameplayLabelWidth(d.layout.gameplayListRect.Dx(), d.layout.pixelUnit),
			etext.AlignStart,
		)
	}

	d.drawGameplayScrollIndicator(screen)
	d.prepareGameplayDetail()
	inset := max(1, d.layout.pixelUnit*audioSettingsCenterDivisor)
	d.detailOutput.DrawPanelContent(
		screen,
		image.Pt(d.layout.gameplayDetail.Min.X+inset, d.layout.gameplayDetail.Min.Y+inset),
		max(1, d.layout.gameplayDetail.Dx()-inset*audioSettingsCenterDivisor),
		etext.AlignStart,
	)
	widgets.DrawClassicPixelRoundedFrame(
		screen, d.layout.gameplayListRect, d.layout.pixelUnit, widgets.DialogRuleColor,
	)
	widgets.DrawClassicPixelRoundedFrame(
		screen, d.layout.gameplayDetail, d.layout.pixelUnit, widgets.DialogRuleColor,
	)
}

// prepareGameplayDetail uses the measured layout's wrapping capacity without
// rasterizing. Selection and staged settings remain owned by Update.
func (d *ConfigurationDialog) prepareGameplayDetail() {
	inset := max(1, d.layout.pixelUnit*audioSettingsCenterDivisor)
	d.detailOutput.SetMaxCharsPerLine(max(1,
		d.detailOutput.Font.CharsForWidth(d.layout.gameplayDetail.Dx()-inset*audioSettingsCenterDivisor)))
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

func (d *ConfigurationDialog) drawAudioContent(screen *ebiten.Image) {
	sliders := []*widgets.Slider{d.musicSlider, d.sfxSlider, d.ambientSlider}
	labels := []string{"Music Volume", "SFX Volume", "Ambient Volume"}
	for index, slider := range sliders {
		d.labelOutput.Reset()
		d.labelOutput.AppendLine().Text(labels[index], widgets.DialogLabelColor).Done()
		d.labelOutput.DrawPanelContent(
			screen,
			d.layout.sliderLabelRects[index].Min,
			d.layout.sliderLabelRects[index].Dx(),
			etext.AlignStart,
		)

		d.valueOutput.Reset()
		d.valueOutput.AppendLine().Text(strconv.Itoa(slider.GetValue()), widgets.DialogValueColor).Done()
		d.valueOutput.DrawPanelContent(
			screen,
			d.layout.sliderValueRects[index].Min,
			d.layout.sliderValueRects[index].Dx(),
			etext.AlignEnd,
		)
		slider.Draw(screen)
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
	default:
		var adjust string
		switch d.currentTab {
		case audioSettingsGameplayTab:
			adjust = "[Space] Toggle   [Left/Right] Adjust"
		case audioSettingsAudioTab:
			adjust = "[Left/Right] Adjust   [D] Diagnostics"
		case audioSettingsDisplayTab:
			adjust = "[Left/Right] Change"
		default:
			panic("unsupported Configuration section")
		}
		hints := []string{"[Tab/Shift+Tab] Section   [Up/Down] Select", adjust, "[Enter] Apply   [Esc] Cancel"}
		if d.currentTab == audioSettingsDisplayTab {
			hints = []string{
				"[Tab/Shift+Tab] Section   [Up/Down] Select", "[Left/Right] Change", "",
				"[Enter] Apply   [Esc] Cancel",
			}
		}
		for _, hint := range hints {
			widgets.AppendControlHints(d.footerOutput.AppendLine(), hint).Done()
		}
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

func (d *ConfigurationDialog) gameplaySettingValue(index int) bool {
	switch index {
	case gameplayRepeatedDivisionIndex:
		return d.stagedGameplay.AllowRepeatedMonsterDivision
	case gameplayHiddenDiscoveriesIndex:
		return d.stagedGameplay.PersistHiddenDungeonDiscoveries
	case gameplayUnlitDungeonsIndex:
		return d.stagedGameplay.ShowSubduedUnlitDungeons
	case gameplaySmoothSurfaceMovementIndex:
		return d.stagedGameplay.SmoothSurfaceMovement
	case gameplayBatchMerchantTheftIndex:
		return d.stagedGameplay.BatchMerchantTheft
	case gameplayConcealedTurnSlotsIndex:
		return d.stagedGameplay.ShowConcealedTurnOrderSlots
	case gameplayFlexibleSextantUseIndex:
		return d.stagedGameplay.FlexibleSextantUse
	case gameplayAnkhWandererBonusIndex:
		return d.stagedGameplay.AnkhWandererBonus
	case gameplayConfirmQuickSaveIndex:
		return d.stagedGameplay.ConfirmQuickSave
	case gameplayShowCombatHPBarsIndex:
		return d.stagedGameplay.ShowCombatHPBars
	case gameplaySimpleNotesModeIndex:
		return d.stagedGameplay.SimpleNotesMode
	default:
		panic("unsupported Gameplay setting index")
	}
}

func (d *ConfigurationDialog) toggleFocusedGameplaySetting() {
	switch d.focusedGameplayIndex {
	case gameplayRepeatedDivisionIndex:
		d.stagedGameplay.AllowRepeatedMonsterDivision = !d.stagedGameplay.AllowRepeatedMonsterDivision
	case gameplayHiddenDiscoveriesIndex:
		d.stagedGameplay.PersistHiddenDungeonDiscoveries = !d.stagedGameplay.PersistHiddenDungeonDiscoveries
	case gameplayUnlitDungeonsIndex:
		d.stagedGameplay.ShowSubduedUnlitDungeons = !d.stagedGameplay.ShowSubduedUnlitDungeons
	case gameplaySmoothSurfaceMovementIndex:
		d.stagedGameplay.SmoothSurfaceMovement = !d.stagedGameplay.SmoothSurfaceMovement
	case gameplayBatchMerchantTheftIndex:
		d.stagedGameplay.BatchMerchantTheft = !d.stagedGameplay.BatchMerchantTheft
	case gameplayConcealedTurnSlotsIndex:
		d.stagedGameplay.ShowConcealedTurnOrderSlots = !d.stagedGameplay.ShowConcealedTurnOrderSlots
	case gameplayFlexibleSextantUseIndex:
		d.stagedGameplay.FlexibleSextantUse = !d.stagedGameplay.FlexibleSextantUse
	case gameplayAnkhWandererBonusIndex:
		d.stagedGameplay.AnkhWandererBonus = !d.stagedGameplay.AnkhWandererBonus
	case gameplayConfirmQuickSaveIndex:
		d.stagedGameplay.ConfirmQuickSave = !d.stagedGameplay.ConfirmQuickSave
	case gameplayShowCombatHPBarsIndex:
		d.stagedGameplay.ShowCombatHPBars = !d.stagedGameplay.ShowCombatHPBars
	case gameplaySimpleNotesModeIndex:
		d.stagedGameplay.SimpleNotesMode = !d.stagedGameplay.SimpleNotesMode
	case gameplayCombatSpeedIndex, gameplayMaxActionPopupsIndex:
		return
	default:
		panic("unsupported Gameplay setting index")
	}
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
