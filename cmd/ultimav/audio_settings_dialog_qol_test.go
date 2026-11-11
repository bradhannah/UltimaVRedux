package main

import (
	"math"
	"os"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	gametext "github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

func TestConfigurationQoLKeyboardDomainsAndScrollableReachability(t *testing.T) {
	configuration := dialogConfiguration(&config.AudioConfiguration{})
	placement := gameScreenPercents
	dialog := NewConfigurationDialog(&placement, newPresentationTestKeyboard(), func() {}, configuration, nil)
	for _, resolution := range config.SupportedWindowResolutions() {
		dialog.ensureLayout(resolution.X, resolution.Y)
		for index := 0; index < audioSettingsGameplayCount; index++ {
			dialog.focusedGameplayIndex = index
			dialog.ensureGameplaySelectionVisible()
			assert.True(t, dialog.gameplayVisibleRow(index).In(dialog.gameplayViewport()), "row %d at %dx%d", index, resolution.X, resolution.Y)
			dialog.prepareGameplayDetail()
			inset := max(1, dialog.layout.pixelUnit*audioSettingsCenterDivisor)
			assert.LessOrEqual(t, int(math.Ceil(dialog.detailOutput.GetTotalHeightForLines(dialog.detailOutput.GetPanelContentLineCount()))), dialog.layout.gameplayDetail.Dy()-inset*audioSettingsCenterDivisor)
		}
		first := dialog.firstGameplayIndex
		pressAudioSettingsKey(dialog, ebiten.KeyTab)
		pressAudioSettingsKey(dialog, ebiten.KeyTab)
		pressAudioSettingsKey(dialog, ebiten.KeyTab)
		assert.Equal(t, first, dialog.firstGameplayIndex)
		pressAudioSettingsKey(dialog, ebiten.KeyDown)
		assert.Equal(t, audioSettingsGameplayCount-1, dialog.focusedGameplayIndex)
	}
	dialog.focusedGameplayIndex = gameplayCombatSpeedIndex
	for range 20 {
		pressAudioSettingsKey(dialog, ebiten.KeyLeft)
	}
	for _, want := range []float64{0.5, 1, 1.5, 2, 3, 4, 6, 8, 12, 16, 20, config.CombatSpeedLudicrous} {
		assert.Equal(t, want, dialog.stagedGameplay.CombatSpeedFactor)
		pressAudioSettingsKey(dialog, ebiten.KeyRight)
	}
	assert.Equal(t, config.CombatSpeedLudicrous, dialog.stagedGameplay.CombatSpeedFactor)
	pressAudioSettingsKey(dialog, ebiten.KeySpace)
	assert.Equal(t, config.CombatSpeedLudicrous, dialog.stagedGameplay.CombatSpeedFactor)
	dialog.focusedGameplayIndex = gameplayMaxActionPopupsIndex
	for range 5 {
		pressAudioSettingsKey(dialog, ebiten.KeyLeft)
	}
	for want := 0; want <= 3; want++ {
		assert.Equal(t, want, dialog.stagedGameplay.MaxVisibleActionPopups)
		pressAudioSettingsKey(dialog, ebiten.KeyRight)
	}
	dialog.focusedGameplayIndex = gameplayConfirmQuickSaveIndex
	pressAudioSettingsKey(dialog, ebiten.KeyLeft)
	assert.True(t, dialog.stagedGameplay.ConfirmQuickSave, "arrows only change numeric rows")
	pressAudioSettingsKey(dialog, ebiten.KeySpace)
	assert.False(t, dialog.stagedGameplay.ConfirmQuickSave)
	assert.Equal(t, 1.0, configuration.GetCombatSpeedFactor())
	assert.True(t, configuration.GetConfirmQuickSave())
	pressAudioSettingsKey(dialog, ebiten.KeyEscape)
	assert.False(t, configuration.ConfigurationEditing())
	assert.Equal(t, 1.0, configuration.GetCombatSpeedFactor())
}

func TestConfigurationQoLHelpUsesApprovedFontSafeParagraphs(t *testing.T) {
	expected := map[int][]string{
		gameplayCombatSpeedIndex: {
			"Change how quickly combat actions and animations play.", "",
			"1x is normal speed.", "Higher values make combat faster.",
			"Ludicrous also skips movement and missile animation.", "",
			"Choose 0.5x, 1x, 1.5x, 2x, 3x, 4x, 6x, 8x, 12x, 16x, 20x or Ludicrous.", "",
			"Default: 1x",
		},
		gameplayMaxActionPopupsIndex: {
			"Choose how many action messages can appear over the map at once.", "",
			"Off hides these popups.", "Messages still appear in the lower-right text area.", "",
			"Choose Off, 1, 2 or 3.", "", "Default: 3",
		},
		gameplayConfirmQuickSaveIndex: {
			"Ask before saving with Q.", "",
			"On: Ask before saving.", "Off: Save immediately.", "",
			"Q saves to the last slot you loaded or saved.", "", "Default: On",
		},
		gameplayShowCombatHPBarsIndex: {
			"Show health bars during combat.", "",
			"Off hides the bars and adds injury descriptions to the battle report.", "",
			"Damage numbers and fleeing indicators remain visible.", "", "Default: On",
		},
	}
	placement := gameScreenPercents
	dialog := NewConfigurationDialog(&placement, newPresentationTestKeyboard(), func() {},
		dialogConfiguration(&config.AudioConfiguration{}), nil)
	t.Cleanup(dialog.onCancelClicked)
	for index, lines := range expected {
		setting := gameplaySettings()[index]
		require.Equal(t, lines, setting.detailLines(), setting.label)
		for _, line := range lines {
			for _, character := range line {
				assert.True(t, character >= ' ' && character <= '~',
					"%s: non-printable-ASCII glyph %U in %q", setting.label, character, line)
			}
			assert.NotContains(t, line, ";", "plain-language guidance must not return to compact technical syntax")
		}
		// A wide Output removes wrapping as a variable: assert each authored
		// sentence and blank paragraph separator occupies its own physical row.
		dialog.focusedGameplayIndex = index
		dialog.detailOutput.SetMaxCharsPerLine(1000)
		populateGameplayDetail(dialog.detailOutput, setting, dialog.gameplaySettingDisplayValue())
		require.Equal(t, len(lines)+2, dialog.detailOutput.GetPanelContentLineCount())
		actualRows := strings.Split(strings.TrimSpace(dialog.detailOutput.GetOutputStr(true)), "\n")
		for row := range actualRows {
			actualRows[row] = strings.TrimSpace(actualRows[row])
		}
		wantRows := append([]string{setting.label}, lines...)
		wantRows = append(wantRows, "Value: "+dialog.gameplaySettingDisplayValue())
		assert.Equal(t, wantRows, actualRows)
	}

	for _, resolution := range config.SupportedWindowResolutions() {
		dialog.ensureLayout(resolution.X, resolution.Y)
		inset := max(1, dialog.layout.pixelUnit*audioSettingsCenterDivisor)
		width := dialog.layout.gameplayDetail.Dx() - inset*audioSettingsCenterDivisor
		chars := dialog.detailOutput.Font.CharsForWidth(width)
		assert.Equal(t, gametext.GetScaledNumberToWidth(resolution.X, float64(widgets.FontSizeBody)),
			dialog.detailOutput.Font.TextFace.Size, "help retains normal typography")
		for index, lines := range expected {
			dialog.focusedGameplayIndex = index
			dialog.prepareGameplayDetail()
			setting := gameplaySettings()[index]
			rows := gametext.MeasureText(setting.label, dialog.detailOutput.Font,
				dialog.detailOutput.GetLineSpacing(), chars).Lines + 1
			for _, line := range lines {
				rows += max(1, gametext.MeasureText(line, dialog.detailOutput.Font,
					dialog.detailOutput.GetLineSpacing(), chars).Lines)
			}
			assert.Equal(t, rows, dialog.detailOutput.GetPanelContentLineCount(), setting.label)
			assert.LessOrEqual(t, rows, audioSettingsDetailLines, "help must not wrap the circular buffer")
			assert.LessOrEqual(t, int(math.Ceil(dialog.detailOutput.GetTotalHeightForLines(rows))),
				dialog.layout.gameplayDetail.Dy()-inset*audioSettingsCenterDivisor,
				"complete paragraphs for %s must fit %dx%d", setting.label, resolution.X, resolution.Y)
			assert.Equal(t, strings.Fields(setting.label+" "+strings.Join(lines, " ")+" Value: "+dialog.gameplaySettingDisplayValue()),
				strings.Fields(dialog.detailOutput.GetOutputStr(false)))
			blankRows := 0
			for _, row := range strings.Split(strings.TrimSpace(dialog.detailOutput.GetOutputStr(true)), "\n") {
				row = strings.TrimSpace(row)
				if row == "" {
					blankRows++
				}
				rowWidth, _ := dialog.detailOutput.Font.MeasureString(row)
				assert.LessOrEqual(t, int(math.Ceil(rowWidth)), width, "wrapped help row must fit: %q", row)
			}
			assert.Equal(t, 3, blankRows, "paragraph separators survive wrapping for %s", setting.label)
		}
	}
}

func TestConfigurationNumericHelpFollowsSpecifications(t *testing.T) {
	assert.Equal(t, []string{
		"Change how quickly combat actions and animations play.", "",
		"2x is normal speed.", "Higher values make combat faster.", "",
		"Choose 1x, 2x or 3x.", "", "Default: 2x",
	}, combatSpeedHelpLines(config.NumericSettingSpec{Min: 1, Max: 3, Step: 1, Default: 2}))
	assert.Contains(t, combatSpeedHelpLines(config.NumericSettingSpec{
		Min: 0.5, Max: 8, Step: 0.5, Default: 1, Values: []float64{0.5, 1, 8},
	}), "Choose 0.5x, 1x or 8x.")
	assert.Contains(t, actionPopupsHelpLines(config.NumericSettingSpec{Min: 0, Max: 4, Step: 2, Default: 2}),
		"Choose Off, 2 or 4.")
	assert.Contains(t, actionPopupsHelpLines(config.NumericSettingSpec{Min: 0, Max: 4, Step: 2, Default: 2}), "Default: 2")
}

func TestConfigurationQoLApplyFailureRetryAndConflict(t *testing.T) {
	configuration, file := initializeDialogConfigFile(t, "qol_dialog")
	placement := gameScreenPercents
	dialog := NewConfigurationDialog(&placement, newPresentationTestKeyboard(), func() {}, configuration, nil)
	before := *configuration.GameplayConfiguration
	pointer := configuration.GameplayConfiguration
	require.ErrorIs(t, configuration.SetCombatSpeedFactor(2), config.ErrConfigurationEditing)
	dialog.stagedGameplay.CombatSpeedFactor = 4
	dialog.stagedGameplay.MaxVisibleActionPopups = 0
	dialog.stagedGameplay.ConfirmQuickSave = false
	dialog.stagedGameplay.ShowCombatHPBars = false
	require.NoError(t, os.Remove(file))
	require.NoError(t, os.Mkdir(file, 0o700))
	assert.False(t, dialog.persistConfiguration())
	assert.Equal(t, before, *configuration.GameplayConfiguration)
	assert.True(t, configuration.ConfigurationEditing())
	assert.True(t, dialog.IsVisible())
	require.NoError(t, os.Remove(file))
	assert.True(t, dialog.persistConfiguration())
	assert.Same(t, pointer, configuration.GameplayConfiguration)
	assert.Equal(t, dialog.stagedGameplay, *pointer)
	assert.False(t, configuration.ConfigurationEditing())
	assert.False(t, dialog.IsVisible())
}
