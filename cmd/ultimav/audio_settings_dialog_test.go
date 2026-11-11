package main

import (
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/audio"
	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	gametext "github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigurationDialogBasicCreation(t *testing.T) {
	keyboard := newPresentationTestKeyboard()
	placement := &sprites.PercentBasedPlacement{
		StartPercentX: 0.1325,
		EndPercentX:   0.6325,
		StartPercentY: 0.1875,
		EndPercentY:   0.8125,
	}

	onClose := func() {
		// Test callback
	}

	audioConfig := &config.AudioConfiguration{
		MusicVolume:       7, // Slider value 7
		SoundEffectVolume: 6, // Slider value 6
		AmbientVolume:     5, // Slider value 5
	}

	// Create mock UltimaVConfiguration for testing
	ultimaConfig := &config.UltimaVConfiguration{
		AudioConfiguration: audioConfig,
		DisplayConfiguration: &config.DisplayConfiguration{
			Mode:              config.ModeWindowed,
			WindowWidth:       config.MinimumWindowWidth,
			WindowHeight:      config.MinimumWindowHeight,
			TargetFPS:         30,
			EnableFpsLimiting: true,
		},
		GameplayConfiguration: defaultDialogGameplayConfiguration(),
	}

	manager := &audioSettingsManagerStub{}
	dialog := NewConfigurationDialog(placement, keyboard, onClose, ultimaConfig, manager)

	// Test dialog creation
	require.NotNil(t, dialog, "ConfigurationDialog should be created successfully")
	assert.True(t, dialog.IsVisible(), "Dialog should be visible by default")
	assert.True(t, dialog.HasFocus(), "Dialog should have focus by default")

	// Test slider creation and initial values (master volume removed)
	require.NotNil(t, dialog.GetMusicVolumeSlider(), "Music volume slider should exist")
	require.NotNil(t, dialog.GetSFXVolumeSlider(), "SFX volume slider should exist")
	require.NotNil(t, dialog.GetAmbientVolumeSlider(), "Ambient volume slider should exist")
	assert.Equal(t, 7, dialog.GetMusicVolumeSlider().GetValue())
	assert.Equal(t, 6, dialog.GetSFXVolumeSlider().GetValue())
	assert.Equal(t, 5, dialog.GetAmbientVolumeSlider().GetValue())

	// Test tab system
	assert.Equal(t, audioSettingsGameplayTab, dialog.GetActiveTab(), "Gameplay should be immediately actionable")
	assert.Equal(t, []string{"Gameplay", "Sound", "Display"}, dialog.tabNames)
	assert.Len(t, gameplaySettings(), audioSettingsGameplayCount)
	assert.False(t, dialog.GetMusicVolumeSlider().HasFocus())
	assert.False(t, dialog.GetSFXVolumeSlider().HasFocus())
	assert.False(t, dialog.GetAmbientVolumeSlider().HasFocus())
	assert.Empty(t, manager.enabledStates, "Opening Gameplay must not start audio preview")
	assert.Equal(t, dialog.labelOutput.Font.TextFace.Size, dialog.tabOutput.Font.TextFace.Size)
	assert.Equal(t, dialog.labelOutput.Font.TextFace.Size, dialog.gameplayOutput.Font.TextFace.Size)
	assert.Equal(t, dialog.labelOutput.Font.TextFace.Size, dialog.footerOutput.Font.TextFace.Size)
	assert.Equal(t,
		gametext.GetScaledNumberToWidth(dialog.lastWidth, float64(widgets.FontSizeBody)),
		dialog.labelOutput.Font.TextFace.Size,
	)
	assert.Equal(t,
		gametext.GetScaledNumberToWidth(dialog.lastWidth, float64(widgets.FontSizeTitle)),
		dialog.titleOutput.Font.TextFace.Size,
	)
	assert.True(t, placementContainsRect(*placement, dialog.layout.cardRect, dialog.lastWidth, dialog.lastHeight),
		"content-measured card should remain inside the supplied placement")
	placementRect := audioSettingsPercentRect(*placement, dialog.lastWidth, dialog.lastHeight)
	assert.LessOrEqual(t, dialog.layout.cardRect.Dy(), placementRect.Dy(),
		"the Gameplay card must stay within the supplied dialog height")

}

func TestConfigurationReverseTabPreservesStagedSettings(t *testing.T) {
	for _, shift := range []ebiten.Key{ebiten.KeyShiftLeft, ebiten.KeyShiftRight} {
		held := map[ebiten.Key]bool{shift: true}
		keyboard := input.NewKeyboardWithKeyStateSource(-1, func(key ebiten.Key) bool { return held[key] })
		configuration := dialogConfiguration(&config.AudioConfiguration{})
		dialog := newConfigurationDialogForTab(&sprites.PercentBasedPlacement{
			StartPercentX: 0.1, EndPercentX: 0.9, StartPercentY: 0.1, EndPercentY: 0.9,
		}, keyboard, func() {}, configuration, nil, audioSettingsGameplayTab)
		staged := dialog.stagedDisplay
		pressAudioSettingsKey(dialog, ebiten.KeyTab)
		require.Equal(t, audioSettingsDisplayTab, dialog.currentTab)
		require.Equal(t, staged, dialog.stagedDisplay)
	}
}

func TestConfigurationDialogCanOpenOnDisplayWithoutStartingAudioPreview(t *testing.T) {
	audioManager := &audioSettingsManagerStub{}
	dialog := newConfigurationDialogForTab(
		&sprites.PercentBasedPlacement{StartPercentX: 0.1, EndPercentX: 0.9, StartPercentY: 0.1, EndPercentY: 0.9},
		newPresentationTestKeyboard(),
		func() {},
		dialogConfiguration(&config.AudioConfiguration{}),
		audioManager,
		audioSettingsDisplayTab,
	)

	assert.Equal(t, audioSettingsDisplayTab, dialog.GetActiveTab())
	assert.False(t, dialog.musicSlider.HasFocus())
	assert.False(t, dialog.sfxSlider.HasFocus())
	assert.False(t, dialog.ambientSlider.HasFocus())
	assert.Empty(t, audioManager.enabledStates)
	assert.Empty(t, audioManager.musicVolumes)
	assert.Empty(t, audioManager.sfxVolumes)
	assert.Empty(t, audioManager.ambientVolumes)
}

func TestConfigurationDialogRejectsUnsupportedInitialTab(t *testing.T) {
	assert.Panics(t, func() {
		newConfigurationDialogForTab(
			&sprites.PercentBasedPlacement{StartPercentX: 0.1, EndPercentX: 0.9, StartPercentY: 0.1, EndPercentY: 0.9},
			newPresentationTestKeyboard(),
			func() {},
			dialogConfiguration(&config.AudioConfiguration{}),
			nil,
			audioSettingsDisplayTab+1,
		)
	})
}

func TestConfigurationDialogDrawsClassicCardAndTopTabs(t *testing.T) {
	placement := &sprites.PercentBasedPlacement{
		StartPercentX: 0.1,
		EndPercentX:   0.9,
		StartPercentY: 0.1,
		EndPercentY:   0.9,
	}
	dialog := NewConfigurationDialog(
		placement,
		newPresentationTestKeyboard(),
		func() {},
		dialogConfiguration(&config.AudioConfiguration{}),
		nil,
	)
	screen := newPresentationRaster(dialog.lastWidth, dialog.lastHeight)
	t.Cleanup(screen.Deallocate)
	screen.Fill(ucolor.EGAMagenta)
	dialog.Draw(screen)

	card := dialog.layout.cardRect
	activeTab := dialog.layout.tabRects[audioSettingsGameplayTab]
	assert.Equal(t,
		color.RGBAModel.Convert(widgets.DialogRuleColor),
		color.RGBAModel.Convert(screen.At(card.Min.X+card.Dx()/2, card.Min.Y)),
	)
	assert.Equal(t,
		color.RGBAModel.Convert(ucolor.DialogBackground.RGBA()),
		color.RGBAModel.Convert(screen.At(card.Min.X+dialog.layout.pixelUnit*3, dialog.layout.titleRect.Min.Y)),
	)
	assert.Equal(t,
		color.RGBAModel.Convert(widgets.DialogSelectedBackground),
		color.RGBAModel.Convert(screen.At(activeTab.Min.X+dialog.layout.pixelUnit, activeTab.Min.Y+activeTab.Dy()/2)),
	)
	assert.Less(t, activeTab.Max.Y, dialog.layout.bodyRect.Min.Y)
}

func TestConfigurationDialogDisplayConfirmationChoicesHaveContrast(t *testing.T) {
	for _, selection := range []struct {
		name string
		keep bool
		text string
	}{
		{name: "Keep", keep: true, text: "[ Keep ]        Revert"},
		{name: "Revert", keep: false, text: "Keep        [ Revert ]"},
	} {
		t.Run(selection.name, func(t *testing.T) {
			dialog, _ := newDisplayTestDialog(newConfigurationDisplayManagerStub(), func() {})
			dialog.displayPhase = configurationDisplayConfirming
			dialog.keepDisplaySettings = selection.keep
			screen := newPresentationRaster(dialog.lastWidth, dialog.lastHeight)
			t.Cleanup(screen.Deallocate)
			screen.Fill(ucolor.EGAMagenta)
			dialog.Draw(screen)
			require.Contains(t, dialog.detailOutput.GetOutputStr(false), selection.text)
			selectionRow := dialog.detailOutput.GetPanelContentLineCount() - 1
			y := centeredAudioOutputY(dialog.layout.bodyRect, dialog.detailOutput) +
				int(math.Ceil(float64(selectionRow)*dialog.detailOutput.GetLineSpacing()))
			whitePixels, blackPixels := 0, 0
			for row := y; row < y+int(dialog.detailOutput.GetFontHeight()); row++ {
				for x := dialog.layout.bodyRect.Min.X; x < dialog.layout.bodyRect.Max.X; x++ {
					switch color.RGBAModel.Convert(screen.At(x, row)) {
					case color.RGBA{R: 255, G: 255, B: 255, A: 255}:
						whitePixels++
					case color.RGBA{A: 255}:
						blackPixels++
					}
				}
			}
			require.Positive(t, whitePixels, "choices must render white text on the black card")
			require.Positive(t, blackPixels, "choices retain an opaque black background")
		})
	}
}

func TestConfigurationDialogUsesPreferredCardWidthAndEqualTabs(t *testing.T) {
	placement := gameScreenPercents
	dialog := NewConfigurationDialog(
		&placement,
		newPresentationTestKeyboard(),
		func() {},
		dialogConfiguration(&config.AudioConfiguration{}),
		nil,
	)
	dialog.ensureLayout(1280, 720)

	assert.Equal(t, int(math.Round(audioSettingsPreferredWidthPercent*1280)), dialog.layout.cardRect.Dx())
	naturalMetrics := dialog.measureLayout(dialog.layout.pixelUnit, dialog.layout.bodyRect.Dx())
	// The preferred height is capped by the 720-pixel screen; the Gameplay list scrolls.
	assert.Equal(t,
		min(int(math.Ceil(float64(naturalMetrics.cardHeight)*audioSettingsPreferredHeightScale)), 720),
		dialog.layout.cardRect.Dy(),
	)
	require.Len(t, dialog.layout.tabRects, 3)
	assert.Equal(t, dialog.layout.tabRects[0].Dx(), dialog.layout.tabRects[1].Dx())
	assert.Equal(t, dialog.layout.tabRects[1].Dx(), dialog.layout.tabRects[2].Dx())
	assert.Equal(t,
		dialog.layout.tabRects[1].Min.X-dialog.layout.tabRects[0].Max.X,
		dialog.layout.tabRects[2].Min.X-dialog.layout.tabRects[1].Max.X,
	)
	for index, tabRect := range dialog.layout.tabRects {
		labelWidth, _ := dialog.tabOutput.Font.MeasureString(dialog.tabNames[index])
		assert.LessOrEqual(t, int(math.Ceil(labelWidth)), tabRect.Dx())
	}
}

func TestConfigurationDialogDisplayTextFitsAllocatedColumns(t *testing.T) {
	displayManager := newConfigurationDisplayManagerStub()
	dialog, _ := newDisplayTestDialog(displayManager, func() {})
	placement := gameScreenPercents
	dialog.placement = &placement

	for _, resolution := range config.SupportedWindowResolutions() {
		logicalBounds := logicalGameBounds(resolution.X, resolution.Y)
		dialog.ensureLayout(logicalBounds.Dx(), logicalBounds.Dy())
		for _, mode := range []config.Mode{config.ModeWindowed, config.ModeMaximized, config.ModeFullscreen} {
			dialog.stagedDisplay.Mode = mode
			labels, values := dialog.displayContentRows()
			for index := range labels {
				labelWidth, _ := dialog.labelOutput.Font.MeasureString(labels[index])
				valueWidth, _ := dialog.valueOutput.Font.MeasureString(values[index])
				assert.LessOrEqual(t, int(math.Ceil(labelWidth)), dialog.layout.displayLabels[index].Dx(), labels[index])
				assert.LessOrEqual(t, int(math.Ceil(valueWidth)), dialog.layout.displayValues[index].Dx(), values[index])
				assert.Less(t, dialog.layout.displayLabels[index].Max.X, dialog.layout.displayValues[index].Min.X)
			}
		}
	}
}

func TestConfigurationDialogSelectionsUseNaturalTextHeight(t *testing.T) {
	placement := gameScreenPercents
	dialog := NewConfigurationDialog(
		&placement,
		newPresentationTestKeyboard(),
		func() {},
		dialogConfiguration(&config.AudioConfiguration{}),
		nil,
	)
	dialog.ensureLayout(1280, 720)

	for index, setting := range gameplaySettings() {
		textWidth := dialog.gameplayLabelWidth(dialog.layout.gameplayListRect.Dx(), dialog.layout.pixelUnit, setting)
		measurement := gametext.MeasureText(
			setting.label,
			dialog.gameplayOutput.Font,
			dialog.gameplayOutput.GetLineSpacing(),
			dialog.gameplayOutput.Font.CharsForWidth(textWidth),
		)
		expectedHeight := int(math.Ceil(measurement.TotalHeight)) +
			dialog.layout.pixelUnit*audioSettingsGameplayRowPaddingUnits
		assert.Equal(t, expectedHeight, dialog.layout.gameplayRows[index].Dy())
	}
	assert.Greater(t, dialog.layout.gameplayRows[0].Min.Y, dialog.layout.gameplayListRect.Min.Y+dialog.layout.pixelUnit)
	assert.True(t, dialog.gameplayVisibleRow(dialog.focusedGameplayIndex).In(dialog.gameplayViewport()))

	expectedDisplayHeight := int(math.Ceil(dialog.labelOutput.GetFontHeight())) +
		dialog.layout.pixelUnit*audioSettingsCenterDivisor
	for index, selection := range dialog.layout.displaySelections {
		assert.Equal(t, expectedDisplayHeight, selection.Dy())
		assert.Less(t, selection.Dy(), dialog.layout.displayRows[index].Dy())
		assert.Equal(t, dialog.layout.displayRows[index].Min.Y+dialog.layout.displayRows[index].Dy()/2,
			selection.Min.Y+selection.Dy()/2)
	}
	assert.Greater(t, dialog.layout.displayRows[0].Min.Y, dialog.layout.displayHeading.Max.Y)
	assert.Less(t, dialog.layout.displayRows[len(dialog.layout.displayRows)-1].Max.Y,
		dialog.layout.bodyRect.Max.Y)
}

func TestConfigurationDialogKeepsStableCardSizeAcrossTabs(t *testing.T) {
	dialog := NewConfigurationDialog(
		&sprites.PercentBasedPlacement{StartPercentX: 0.1, EndPercentX: 0.9, StartPercentY: 0.1, EndPercentY: 0.9},
		newPresentationTestKeyboard(),
		func() {},
		dialogConfiguration(&config.AudioConfiguration{}),
		nil,
	)
	gameplayHeight := dialog.layout.cardRect.Dy()

	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	dialog.ensureLayout(dialog.lastWidth, dialog.lastHeight)

	assert.Equal(t, audioSettingsAudioTab, dialog.GetActiveTab())
	assert.Equal(t, gameplayHeight, dialog.layout.cardRect.Dy())
	assert.Len(t, dialog.layout.sliderRects, audioSettingsSliderCount)
	assert.Len(t, dialog.layout.gameplayRows, audioSettingsGameplayCount)
	assert.Len(t, dialog.layout.displayRows, 2)
	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	assert.Equal(t, audioSettingsDisplayTab, dialog.GetActiveTab())
	assert.Equal(t, gameplayHeight, dialog.layout.cardRect.Dy())
}

func TestConfigurationDialogKeepsSliderBoundsInLogicalLayout(t *testing.T) {
	dialog := NewConfigurationDialog(
		&sprites.PercentBasedPlacement{StartPercentX: 0.1, EndPercentX: 0.9, StartPercentY: 0.1, EndPercentY: 0.9},
		newPresentationTestKeyboard(),
		func() {},
		dialogConfiguration(&config.AudioConfiguration{}),
		nil,
	)
	dialog.ensureLayout(1365, 768)
	card := dialog.layout.cardRect

	dialog.Update()

	assert.Equal(t, 1365, dialog.lastWidth)
	assert.Equal(t, 768, dialog.lastHeight)
	assert.Equal(t, card, dialog.layout.cardRect)
	assert.Equal(t, dialog.layout.sliderRects[0], dialog.musicSlider.GetBounds())
	assert.Equal(t, dialog.layout.sliderRects[1], dialog.sfxSlider.GetBounds())
	assert.Equal(t, dialog.layout.sliderRects[2], dialog.ambientSlider.GetBounds())
}

func TestConfigurationDialogNoSoundPreviewAndCancelAreSafe(t *testing.T) {
	audioConfig := &config.AudioConfiguration{
		MusicVolume:       7,
		SoundEffectVolume: 6,
		AmbientVolume:     5,
	}
	dialog := NewConfigurationDialog(
		&sprites.PercentBasedPlacement{StartPercentX: 0.1, EndPercentX: 0.9, StartPercentY: 0.1, EndPercentY: 0.9},
		newPresentationTestKeyboard(),
		func() {},
		dialogConfiguration(audioConfig),
		nil,
	)

	require.NotPanics(t, func() {
		dialog.previewMusicVolume()
		dialog.adjustMusicPreviewVolume()
		dialog.previewSoundEffectVolume()
		dialog.previewAmbientVolume()
		dialog.adjustAmbientPreviewVolume()
		dialog.previewAudioChanges()
		dialog.stopPreviewAudio()
		dialog.onCancelClicked()
	})
}

func TestConfigurationDialogKeyboardNavigationAppliesAndPersists(t *testing.T) {
	ultimaConfig, configPath := initializeDialogConfigFile(t, "audio_settings")

	audioConfig := &config.AudioConfiguration{
		MusicVolume:       7,
		SoundEffectVolume: 6,
		AmbientVolume:     5,
	}
	dismissCount := 0
	manager := &audioSettingsManagerStub{}
	ultimaConfig.AudioConfiguration = audioConfig
	dialog := NewConfigurationDialog(
		&sprites.PercentBasedPlacement{StartPercentX: 0.1, EndPercentX: 0.9, StartPercentY: 0.1, EndPercentY: 0.9},
		newPresentationTestKeyboard(),
		func() { dismissCount++ },
		ultimaConfig,
		manager,
	)

	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	assert.Equal(t, audioSettingsAudioTab, dialog.GetActiveTab())
	assert.True(t, dialog.GetMusicVolumeSlider().HasFocus())
	assert.False(t, dialog.GetSFXVolumeSlider().HasFocus())
	assert.False(t, dialog.GetAmbientVolumeSlider().HasFocus())

	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	assert.Equal(t, audioSettingsDisplayTab, dialog.GetActiveTab())
	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	assert.Equal(t, audioSettingsGameplayTab, dialog.GetActiveTab())
	pressAudioSettingsKey(dialog, ebiten.KeySpace)
	assert.True(t, dialog.stagedGameplay.AllowRepeatedMonsterDivision)
	assert.False(t, ultimaConfig.GameplayConfiguration.AllowRepeatedMonsterDivision,
		"Gameplay editing must remain detached until Apply")
	pressAudioSettingsKey(dialog, ebiten.KeyRight)
	assert.Equal(t, audioSettingsGameplayTab, dialog.GetActiveTab(),
		"Left/Right must not switch configure sections")
	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	assert.Equal(t, audioSettingsAudioTab, dialog.GetActiveTab())
	assert.True(t, dialog.GetMusicVolumeSlider().HasFocus())

	pressAudioSettingsKey(dialog, ebiten.KeyDown)
	assert.True(t, dialog.GetSFXVolumeSlider().HasFocus())
	pressAudioSettingsKey(dialog, ebiten.KeyRight)
	assert.Equal(t, 6, audioConfig.SoundEffectVolume, "editing must remain detached until Apply")
	assert.Equal(t, 7, dialog.stagedAudio.SoundEffectVolume)
	assert.Len(t, manager.playedEffects, 1, "one adjustment should play one SFX preview")

	pressAudioSettingsKey(dialog, ebiten.KeyDown)
	assert.True(t, dialog.GetAmbientVolumeSlider().HasFocus())
	assert.Equal(t, 1, manager.ambientPlayCount)
	ambientStops := manager.ambientStopCount
	pressAudioSettingsKey(dialog, ebiten.KeyUp)
	assert.Equal(t, ambientStops+1, manager.ambientStopCount,
		"leaving ambient should stop its dedicated preview")

	pressAudioSettingsKey(dialog, ebiten.KeyEnter)
	assert.False(t, dialog.IsVisible())
	assert.Equal(t, 7, audioConfig.SoundEffectVolume)
	assert.True(t, ultimaConfig.GameplayConfiguration.AllowRepeatedMonsterDivision)
	assert.Equal(t, 1, dismissCount)
	require.NotEmpty(t, manager.enabledStates)
	assert.False(t, manager.enabledStates[len(manager.enabledStates)-1], "Escape menu audio must remain paused")
	dialog.Update()
	assert.Equal(t, 1, dismissCount, "dismissal must dispatch once")

	persisted := viper.New()
	persisted.SetConfigFile(configPath)
	require.NoError(t, persisted.ReadInConfig())
	assert.Equal(t, 7, persisted.GetInt("Audio.SoundEffectVolume"))
	assert.True(t, persisted.GetBool("Gameplay.Combat.AllowRepeatedMonsterDivision"))
}

func TestConfigurationDialogKeyboardEscapeRevertsAndDismissesOnce(t *testing.T) {
	audioConfig := &config.AudioConfiguration{
		MusicVolume:       7,
		SoundEffectVolume: 6,
		AmbientVolume:     5,
	}
	dismissCount := 0
	manager := &audioSettingsManagerStub{}
	dialog := NewConfigurationDialog(
		&sprites.PercentBasedPlacement{StartPercentX: 0.1, EndPercentX: 0.9, StartPercentY: 0.1, EndPercentY: 0.9},
		newPresentationTestKeyboard(),
		func() { dismissCount++ },
		dialogConfiguration(audioConfig),
		manager,
	)

	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	pressAudioSettingsKey(dialog, ebiten.KeyRight)
	assert.Equal(t, 7, audioConfig.MusicVolume)
	assert.Equal(t, 8, dialog.stagedAudio.MusicVolume)

	pressAudioSettingsKey(dialog, ebiten.KeyEscape)
	assert.Equal(t, 7, audioConfig.MusicVolume)
	assert.Equal(t, 6, audioConfig.SoundEffectVolume)
	assert.Equal(t, 5, audioConfig.AmbientVolume)
	assert.Equal(t, 1, dismissCount)
	assert.False(t, dialog.IsVisible())
	require.NotEmpty(t, manager.musicVolumes)
	assert.InDelta(t, audio.ConvertSliderToVolume(7), manager.musicVolumes[len(manager.musicVolumes)-1], 0.0001)
	require.NotEmpty(t, manager.sfxVolumes)
	assert.InDelta(t, audio.ConvertSliderToVolume(6), manager.sfxVolumes[len(manager.sfxVolumes)-1], 0.0001)
	require.NotEmpty(t, manager.ambientVolumes)
	assert.InDelta(t, audio.ConvertSliderToVolume(5), manager.ambientVolumes[len(manager.ambientVolumes)-1], 0.0001)

	dialog.Update()
	assert.Equal(t, 1, dismissCount, "cancel must not dispatch again after dismissal")
}

func TestConfigurationDialogLeavingSoundStopsPreviewAndClearsFocus(t *testing.T) {
	manager := &audioSettingsManagerStub{}
	dialog := NewConfigurationDialog(
		&sprites.PercentBasedPlacement{StartPercentX: 0.1, EndPercentX: 0.9, StartPercentY: 0.1, EndPercentY: 0.9},
		newPresentationTestKeyboard(),
		func() {},
		dialogConfiguration(&config.AudioConfiguration{}),
		manager,
	)

	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	pressAudioSettingsKey(dialog, ebiten.KeyDown)
	pressAudioSettingsKey(dialog, ebiten.KeyDown)
	require.True(t, dialog.GetAmbientVolumeSlider().HasFocus())
	require.Equal(t, 1, manager.ambientPlayCount)

	pressAudioSettingsKey(dialog, ebiten.KeyTab)

	assert.Equal(t, audioSettingsDisplayTab, dialog.GetActiveTab())
	assert.False(t, dialog.GetMusicVolumeSlider().HasFocus())
	assert.False(t, dialog.GetSFXVolumeSlider().HasFocus())
	assert.False(t, dialog.GetAmbientVolumeSlider().HasFocus())
	assert.Equal(t, 1, manager.ambientStopCount)
	assert.GreaterOrEqual(t, manager.musicStopCount, 1)
	require.NotEmpty(t, manager.enabledStates)
	assert.False(t, manager.enabledStates[len(manager.enabledStates)-1])
}

func TestConfigurationDialogStagesGameplayAndPreservesSectionSelections(t *testing.T) {
	configuration := dialogConfiguration(&config.AudioConfiguration{})
	manager := &audioSettingsManagerStub{}
	dialog := NewConfigurationDialog(
		&sprites.PercentBasedPlacement{StartPercentX: 0.1, EndPercentX: 0.9, StartPercentY: 0.1, EndPercentY: 0.9},
		newPresentationTestKeyboard(),
		func() {},
		configuration,
		manager,
	)

	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	pressAudioSettingsKey(dialog, ebiten.KeyDown)
	assert.Equal(t, 1, dialog.focusedSliderIndex)
	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	pressAudioSettingsKey(dialog, ebiten.KeyDown)
	pressAudioSettingsKey(dialog, ebiten.KeySpace)
	assert.Equal(t, 1, dialog.focusedGameplayIndex)
	assert.False(t, dialog.stagedGameplay.PersistHiddenDungeonDiscoveries)
	assert.True(t, configuration.GameplayConfiguration.PersistHiddenDungeonDiscoveries)

	dialog.ensureLayout(1920, 1080)
	assert.Equal(t, 1, dialog.focusedGameplayIndex)
	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	assert.Equal(t, 1, dialog.focusedSliderIndex)
	assert.True(t, dialog.sfxSlider.HasFocus())
	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	assert.Equal(t, audioSettingsDisplayTab, dialog.GetActiveTab())
	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	assert.Equal(t, 1, dialog.focusedGameplayIndex)

	dialog.prepareGameplayDetail()
	assert.Contains(t, strings.Join(strings.Fields(dialog.detailOutput.GetOutputStr(false)), " "), "hidden doors and pits between visits")
}

func TestConfigurationDialogFitsEverySupportedResolution(t *testing.T) {
	placement := gameScreenPercents
	dialog := NewConfigurationDialog(
		&placement,
		newPresentationTestKeyboard(),
		func() {},
		dialogConfiguration(&config.AudioConfiguration{}),
		nil,
	)
	pressAudioSettingsKey(dialog, ebiten.KeyDown)

	for _, resolution := range config.SupportedWindowResolutions() {
		dialog.ensureLayout(resolution.X, resolution.Y)

		assert.True(t, placementContainsRect(placement, dialog.layout.cardRect, resolution.X, resolution.Y),
			"card must fit %dx%d", resolution.X, resolution.Y)
		assert.True(t, dialog.layout.gameplayListRect.Min.In(dialog.layout.bodyRect))
		assert.True(t, dialog.layout.gameplayDetail.Min.In(dialog.layout.bodyRect))
		assert.Len(t, dialog.layout.gameplayRows, audioSettingsGameplayCount)
		assert.True(t, dialog.gameplayVisibleRow(dialog.focusedGameplayIndex).In(dialog.gameplayViewport()))
		assert.Greater(t, dialog.layout.gameplayRows[0].Min.Y, dialog.layout.gameplayListRect.Min.Y+dialog.layout.pixelUnit)
		assert.LessOrEqual(t, dialog.layout.displayRows[len(dialog.layout.displayRows)-1].Max.Y,
			dialog.layout.bodyRect.Max.Y)
		assert.Equal(t, 1, dialog.focusedGameplayIndex)

		detailInset := max(1, dialog.layout.pixelUnit*audioSettingsCenterDivisor)
		detailChars := dialog.detailOutput.Font.CharsForWidth(
			dialog.layout.gameplayDetail.Dx() - detailInset*audioSettingsCenterDivisor,
		)
		for index, setting := range gameplaySettings() {
			titleLines := gametext.MeasureText(
				setting.label,
				dialog.detailOutput.Font,
				dialog.detailOutput.GetLineSpacing(),
				detailChars,
			).Lines
			detailLines := 0
			for _, line := range setting.detailLines() {
				detailLines += max(1, gametext.MeasureText(line, dialog.detailOutput.Font,
					dialog.detailOutput.GetLineSpacing(), detailChars).Lines)
			}
			assert.LessOrEqual(t, titleLines+detailLines, audioSettingsDetailLines, setting.label)
			dialog.focusedGameplayIndex = index
			dialog.prepareGameplayDetail()
			expected := setting.label + " " + strings.Join(setting.detailLines(), " ") + " Value: " + dialog.gameplaySettingDisplayValue()
			assert.Equal(t, strings.Fields(expected), strings.Fields(dialog.detailOutput.GetOutputStr(false)), "preparation must retain the complete setting text")
			assert.LessOrEqual(t, int(math.Ceil(dialog.detailOutput.GetTotalHeightForLines(dialog.detailOutput.GetPanelContentLineCount()))), dialog.layout.gameplayDetail.Dy()-detailInset*audioSettingsCenterDivisor)
			assert.LessOrEqual(t, dialog.detailOutput.GetPanelContentLineCount(), audioSettingsDetailLines, setting.label)
		}
		dialog.focusedGameplayIndex = 1
	}
}

func TestConfigurationDialogEscapeDiscardsStagedSettingsAndWritesNothing(t *testing.T) {
	configuration, configPath := initializeDialogConfigFile(t, "configuration_cancel")
	beforeFile, err := os.ReadFile(configPath)
	require.NoError(t, err)
	configuration.AudioConfiguration = &config.AudioConfiguration{MusicVolume: 7, SoundEffectVolume: 6, AmbientVolume: 5}
	dialog := NewConfigurationDialog(
		&sprites.PercentBasedPlacement{StartPercentX: 0.1, EndPercentX: 0.9, StartPercentY: 0.1, EndPercentY: 0.9},
		newPresentationTestKeyboard(),
		func() {},
		configuration,
		&audioSettingsManagerStub{},
	)

	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	pressAudioSettingsKey(dialog, ebiten.KeyRight)
	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	pressAudioSettingsKey(dialog, ebiten.KeySpace)
	pressAudioSettingsKey(dialog, ebiten.KeyEscape)

	assert.Equal(t, config.AudioConfiguration{MusicVolume: 7, SoundEffectVolume: 6, AmbientVolume: 5},
		*configuration.AudioConfiguration)
	assert.False(t, configuration.GameplayConfiguration.AllowRepeatedMonsterDivision)
	afterFile, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, beforeFile, afterFile)
}

func TestConfigurationDialogSmoothMovementKeyboardStagingAndCancel(t *testing.T) {
	configuration := dialogConfiguration(&config.AudioConfiguration{})
	placement := gameScreenPercents
	dialog := newConfigurationDialogForTab(&placement, newPresentationTestKeyboard(), func() {},
		configuration, nil, audioSettingsGameplayTab)
	for range gameplaySmoothSurfaceMovementIndex {
		pressAudioSettingsKey(dialog, ebiten.KeyDown)
	}
	assert.Equal(t, gameplaySmoothSurfaceMovementIndex, dialog.focusedGameplayIndex)
	pressAudioSettingsKey(dialog, ebiten.KeySpace)
	assert.True(t, dialog.stagedGameplay.SmoothSurfaceMovement)
	assert.False(t, configuration.GameplayConfiguration.SmoothSurfaceMovement)
	for _, resolution := range config.SupportedWindowResolutions() {
		dialog.ensureLayout(resolution.X, resolution.Y)
		assert.Equal(t, gameplaySmoothSurfaceMovementIndex, dialog.focusedGameplayIndex)
		assert.Equal(t, "X", dialog.gameplayCheckmark(gameplaySmoothSurfaceMovementIndex))
	}
	pressAudioSettingsKey(dialog, ebiten.KeyDown)
	assert.Equal(t, gameplayConcealedTurnSlotsIndex, dialog.focusedGameplayIndex)
	pressAudioSettingsKey(dialog, ebiten.KeyUp)
	assert.Equal(t, gameplaySmoothSurfaceMovementIndex, dialog.focusedGameplayIndex)
	pressAudioSettingsKey(dialog, ebiten.KeySpace)
	assert.False(t, dialog.stagedGameplay.SmoothSurfaceMovement)
	pressAudioSettingsKey(dialog, ebiten.KeySpace)
	pressAudioSettingsKey(dialog, ebiten.KeyEscape)
	assert.False(t, configuration.GameplayConfiguration.SmoothSurfaceMovement)
}

func TestConfigurationDialogConcealedTurnSlotsStageAndPersist(t *testing.T) {
	configuration, configPath := initializeDialogConfigFile(t, "concealed_turn_slots")
	placement := gameScreenPercents
	dialog := newConfigurationDialogForTab(&placement, newPresentationTestKeyboard(), func() {},
		configuration, nil, audioSettingsGameplayTab)
	for range gameplayConcealedTurnSlotsIndex {
		pressAudioSettingsKey(dialog, ebiten.KeyDown)
	}

	assert.Equal(t, gameplayConcealedTurnSlotsIndex, dialog.focusedGameplayIndex)
	assert.Equal(t, "X", dialog.gameplayCheckmark(gameplayConcealedTurnSlotsIndex))
	pressAudioSettingsKey(dialog, ebiten.KeySpace)
	assert.False(t, dialog.stagedGameplay.ShowConcealedTurnOrderSlots)
	assert.True(t, configuration.GameplayConfiguration.ShowConcealedTurnOrderSlots)

	pressAudioSettingsKey(dialog, ebiten.KeyEnter)
	assert.False(t, configuration.GameplayConfiguration.ShowConcealedTurnOrderSlots)
	persisted, err := config.LoadUltimaVConfiguration(configPath, filepath.Dir(configPath))
	require.NoError(t, err)
	assert.False(t, persisted.GameplayConfiguration.ShowConcealedTurnOrderSlots)
}

func TestConfigurationDialogFlexibleSextantKeyboardApplyCancelAndReload(t *testing.T) {
	configuration, configPath := initializeDialogConfigFile(t, "flexible_sextant")
	placement := gameScreenPercents
	open := func() *ConfigurationDialog {
		dialog := newConfigurationDialogForTab(&placement, newPresentationTestKeyboard(), func() {},
			configuration, nil, audioSettingsGameplayTab)
		for range gameplayFlexibleSextantUseIndex {
			pressAudioSettingsKey(dialog, ebiten.KeyDown)
		}
		require.Equal(t, gameplayFlexibleSextantUseIndex, dialog.focusedGameplayIndex)
		pressAudioSettingsKey(dialog, ebiten.KeyDown)
		require.Equal(t, gameplayAnkhWandererBonusIndex, dialog.focusedGameplayIndex)
		pressAudioSettingsKey(dialog, ebiten.KeyUp)

		return dialog
	}
	beforeFile, err := os.ReadFile(configPath)
	require.NoError(t, err)
	dialog := open()
	require.False(t, dialog.gameplaySettingValue(gameplayFlexibleSextantUseIndex), "original rules are the default")
	pressAudioSettingsKey(dialog, ebiten.KeySpace)
	require.True(t, dialog.stagedGameplay.FlexibleSextantUse)
	require.False(t, configuration.GameplayConfiguration.FlexibleSextantUse, "staging must not change live navigation")
	for _, resolution := range config.SupportedWindowResolutions() {
		dialog.ensureLayout(resolution.X, resolution.Y)
		dialog.prepareGameplayDetail()
		require.Equal(t, "X", dialog.gameplayCheckmark(gameplayFlexibleSextantUseIndex))
		require.Contains(t, dialog.detailOutput.GetOutputStr(false), "Flexible Sextant Use")
		require.Contains(t, dialog.detailOutput.GetOutputStr(false), "Redux:")
		require.LessOrEqual(t, dialog.detailOutput.GetPanelContentLineCount(), audioSettingsDetailLines)
	}
	pressAudioSettingsKey(dialog, ebiten.KeyEscape)
	require.False(t, configuration.GameplayConfiguration.FlexibleSextantUse)
	afterFile, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Equal(t, beforeFile, afterFile, "cancel writes nothing")

	dialog = open()
	pressAudioSettingsKey(dialog, ebiten.KeySpace)
	pressAudioSettingsKey(dialog, ebiten.KeyEnter)
	require.False(t, dialog.IsVisible())
	require.True(t, configuration.GameplayConfiguration.FlexibleSextantUse)
	loaded, err := config.LoadUltimaVConfiguration(configPath, filepath.Dir(configPath))
	require.NoError(t, err)
	require.True(t, loaded.GameplayConfiguration.FlexibleSextantUse)

	dialog = open()
	require.True(t, dialog.gameplaySettingValue(gameplayFlexibleSextantUseIndex))
	pressAudioSettingsKey(dialog, ebiten.KeySpace)
	pressAudioSettingsKey(dialog, ebiten.KeyEnter)
	loaded, err = config.LoadUltimaVConfiguration(configPath, filepath.Dir(configPath))
	require.NoError(t, err)
	require.False(t, loaded.GameplayConfiguration.FlexibleSextantUse, "original policy can be restored and persisted")
}

func TestConfigurationDialogSaveFailureStaysVisibleAndCanRetry(t *testing.T) {
	configuration, configPath := initializeDialogConfigFile(t, "configuration_retry")
	configuration.AudioConfiguration = &config.AudioConfiguration{MusicVolume: 7, SoundEffectVolume: 6, AmbientVolume: 5}
	require.NoError(t, os.Remove(configPath))
	require.NoError(t, os.Mkdir(configPath, 0o700))
	dismissCount := 0
	dialog := NewConfigurationDialog(
		&sprites.PercentBasedPlacement{StartPercentX: 0.1, EndPercentX: 0.9, StartPercentY: 0.1, EndPercentY: 0.9},
		newPresentationTestKeyboard(),
		func() { dismissCount++ },
		configuration,
		&audioSettingsManagerStub{},
	)
	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	pressAudioSettingsKey(dialog, ebiten.KeyRight)
	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	pressAudioSettingsKey(dialog, ebiten.KeySpace)
	for range gameplaySmoothSurfaceMovementIndex {
		pressAudioSettingsKey(dialog, ebiten.KeyDown)
	}
	pressAudioSettingsKey(dialog, ebiten.KeySpace)
	for range gameplayFlexibleSextantUseIndex - gameplaySmoothSurfaceMovementIndex {
		pressAudioSettingsKey(dialog, ebiten.KeyDown)
	}
	pressAudioSettingsKey(dialog, ebiten.KeySpace)
	gameplay := configuration.GameplayConfiguration

	pressAudioSettingsKey(dialog, ebiten.KeyEnter)

	assert.True(t, dialog.IsVisible())
	assert.Zero(t, dismissCount)
	assert.NotEmpty(t, dialog.saveError)
	assert.Equal(t, 7, configuration.AudioConfiguration.MusicVolume)
	assert.False(t, configuration.GameplayConfiguration.AllowRepeatedMonsterDivision)
	assert.Equal(t, 8, dialog.stagedAudio.MusicVolume)
	assert.True(t, dialog.stagedGameplay.AllowRepeatedMonsterDivision)
	assert.True(t, dialog.stagedGameplay.SmoothSurfaceMovement)
	assert.False(t, gameplay.SmoothSurfaceMovement)
	assert.True(t, dialog.stagedGameplay.FlexibleSextantUse)
	assert.False(t, gameplay.FlexibleSextantUse)
	assert.Same(t, gameplay, configuration.GameplayConfiguration)

	require.NoError(t, os.Remove(configPath))
	pressAudioSettingsKey(dialog, ebiten.KeyEnter)
	assert.False(t, dialog.IsVisible())
	assert.Equal(t, 1, dismissCount)
	assert.Equal(t, 8, configuration.AudioConfiguration.MusicVolume)
	assert.True(t, configuration.GameplayConfiguration.AllowRepeatedMonsterDivision)
	assert.True(t, gameplay.SmoothSurfaceMovement)
	assert.True(t, gameplay.FlexibleSextantUse)
	assert.Same(t, gameplay, configuration.GameplayConfiguration)
	persisted := viper.New()
	persisted.SetConfigFile(configPath)
	require.NoError(t, persisted.ReadInConfig())
	assert.True(t, persisted.GetBool("Gameplay.Presentation.SmoothSurfaceMovement"))
	assert.True(t, persisted.GetBool("Gameplay.Navigation.FlexibleSextantUse"))
}

func TestConfigurationDialogProcessesOnlyOneSimultaneousAction(t *testing.T) {
	pressed := map[ebiten.Key]bool{ebiten.KeyRight: true, ebiten.KeyTab: true}
	keyboard := input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool { return pressed[key] })
	dialog := newConfigurationDialogForTab(
		&sprites.PercentBasedPlacement{StartPercentX: 0.1, EndPercentX: 0.9, StartPercentY: 0.1, EndPercentY: 0.9},
		keyboard,
		func() {},
		dialogConfiguration(&config.AudioConfiguration{}),
		nil,
		audioSettingsAudioTab,
	)

	dialog.Update()

	assert.Equal(t, 1, dialog.stagedAudio.MusicVolume)
	assert.Equal(t, audioSettingsAudioTab, dialog.currentTab)
}

func TestConfigurationDialogDismissalIsIdempotent(t *testing.T) {
	dismissCount := 0
	manager := &audioSettingsManagerStub{}
	dialog := NewConfigurationDialog(
		&sprites.PercentBasedPlacement{StartPercentX: 0.1, EndPercentX: 0.9, StartPercentY: 0.1, EndPercentY: 0.9},
		newPresentationTestKeyboard(),
		func() { dismissCount++ },
		dialogConfiguration(&config.AudioConfiguration{}),
		manager,
	)

	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	dialog.dismiss()
	dialog.dismiss()

	assert.Equal(t, 1, dismissCount)
	assert.Equal(t, []bool{true, false}, manager.enabledStates)
}

func TestConfigurationDialogDisplayNavigationStagesModesAndAvailableSizes(t *testing.T) {
	displayManager := newConfigurationDisplayManagerStub()
	displayManager.monitorWidth = 1920
	displayManager.monitorHeight = 1080
	dialog, configuration := newDisplayTestDialog(displayManager, func() {})

	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	require.Equal(t, audioSettingsDisplayTab, dialog.currentTab)
	pressAudioSettingsKey(dialog, ebiten.KeyRight)
	assert.Equal(t, config.ModeMaximized, dialog.stagedDisplay.Mode)
	assert.Equal(t, config.ModeWindowed, configuration.DisplayConfiguration.Mode)

	pressAudioSettingsKey(dialog, ebiten.KeyRight)
	assert.Equal(t, config.ModeFullscreen, dialog.stagedDisplay.Mode)
	pressAudioSettingsKey(dialog, ebiten.KeyRight)
	assert.Equal(t, config.ModeWindowed, dialog.stagedDisplay.Mode)
	pressAudioSettingsKey(dialog, ebiten.KeyDown)
	pressAudioSettingsKey(dialog, ebiten.KeyRight)
	assert.Equal(t, 1366, dialog.stagedDisplay.WindowWidth)
	assert.Equal(t, 768, dialog.stagedDisplay.WindowHeight)
	assert.Equal(t, []config.ScreenResolution{
		{X: 1280, Y: 720},
		{X: 1366, Y: 768},
		{X: 1600, Y: 900},
	}, dialog.availableWindowSizes())
	choices := dialog.windowSizeChoices()
	require.Len(t, choices, 6)
	assert.True(t, choices[2].available)
	assert.False(t, choices[3].available)
	assert.False(t, choices[4].available)
}

func TestConfigurationDialogTracksDraggedWindowUntilSizeIsStaged(t *testing.T) {
	displayManager := newConfigurationDisplayManagerStub()
	dialog, _ := newDisplayTestDialog(displayManager, func() {})
	pressAudioSettingsKey(dialog, ebiten.KeyTab)

	displayManager.dragWindow(1700, 950)
	dialog.Update()
	assert.Equal(t, 1700, dialog.stagedDisplay.WindowWidth)
	assert.Equal(t, 950, dialog.stagedDisplay.WindowHeight)
	assert.Equal(t, "Custom: 1700 x 950 pixels", dialog.displaySizeLabel())

	pressAudioSettingsKey(dialog, ebiten.KeyDown)
	pressAudioSettingsKey(dialog, ebiten.KeyLeft)
	displayManager.dragWindow(1800, 1000)
	dialog.Update()
	assert.NotEqual(t, 1800, dialog.stagedDisplay.WindowWidth)
}

func TestConfigurationDialogTracksDraggedSizeAfterStagingNonWindowedMode(t *testing.T) {
	displayManager := newConfigurationDisplayManagerStub()
	dialog, _ := newDisplayTestDialog(displayManager, func() {})
	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	pressAudioSettingsKey(dialog, ebiten.KeyRight)
	require.Equal(t, config.ModeMaximized, dialog.stagedDisplay.Mode)

	displayManager.dragWindow(1700, 950)
	dialog.Update()
	assert.Equal(t, 1700, dialog.stagedDisplay.WindowWidth)
	assert.Equal(t, 950, dialog.stagedDisplay.WindowHeight)
}

func TestConfigurationDialogRetainsStagedSizeWhenApplyingNonWindowedMode(t *testing.T) {
	configuration, _ := initializeDialogConfigFile(t, "display_staged_size")
	displayManager := newConfigurationDisplayManagerStub()
	dialog, _ := newDisplayTestDialog(displayManager, func() {})
	dialog.ultimaConfig = configuration
	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	pressAudioSettingsKey(dialog, ebiten.KeyDown)
	pressAudioSettingsKey(dialog, ebiten.KeyRight)
	pressAudioSettingsKey(dialog, ebiten.KeyUp)
	pressAudioSettingsKey(dialog, ebiten.KeyRight)

	pressAudioSettingsKey(dialog, ebiten.KeyEnter)
	assert.Equal(t, config.ModeMaximized, displayManager.requestedMode)
	assert.Equal(t, 1366, displayManager.requestWidth)
	assert.Equal(t, 768, displayManager.requestHeight)
	displayManager.completeTransition()
	dialog.Update()
	assert.Equal(t, config.MinimumWindowWidth, displayManager.persistWidth)
	assert.Equal(t, config.MinimumWindowHeight, displayManager.persistHeight)
	pressAudioSettingsKey(dialog, ebiten.KeyEnter)

	assert.Equal(t, config.ModeMaximized, configuration.DisplayConfiguration.Mode)
	assert.Equal(t, 1366, configuration.DisplayConfiguration.WindowWidth)
	assert.Equal(t, 768, configuration.DisplayConfiguration.WindowHeight)
	assert.Equal(t, 1366, displayManager.persistWidth)
	assert.Equal(t, 768, displayManager.persistHeight)
}

func TestConfigurationDialogDisplayApplyKeepsAndPersists(t *testing.T) {
	configuration, configPath := initializeDialogConfigFile(t, "display_keep")
	displayManager := newConfigurationDisplayManagerStub()
	dismissCount := 0
	dialog, _ := newDisplayTestDialog(displayManager, func() { dismissCount++ })
	dialog.ultimaConfig = configuration

	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	pressAudioSettingsKey(dialog, ebiten.KeyRight)
	pressAudioSettingsKey(dialog, ebiten.KeyEnter)
	require.Equal(t, configurationDisplayApplying, dialog.displayPhase)
	assert.Equal(t, config.ModeMaximized, displayManager.requestedMode)

	displayManager.completeTransition()
	dialog.Update()
	require.Equal(t, configurationDisplayConfirming, dialog.displayPhase)
	assert.Equal(t, configurationDisplayConfirmationTime, dialog.displayConfirmationTicks)
	pressAudioSettingsKey(dialog, ebiten.KeyEnter)

	assert.False(t, dialog.IsVisible())
	assert.Equal(t, 1, dismissCount)
	assert.Equal(t, config.ModeMaximized, configuration.DisplayConfiguration.Mode)

	persisted := viper.New()
	persisted.SetConfigFile(configPath)
	require.NoError(t, persisted.ReadInConfig())
	assert.Equal(t, config.ModeMaximized.String(), persisted.GetString("Display.Mode"))
}

func TestConfigurationDialogDisplayRevertRestoresActualState(t *testing.T) {
	displayManager := newConfigurationDisplayManagerStub()
	dialog, configuration := newDisplayTestDialog(displayManager, func() {})

	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	pressAudioSettingsKey(dialog, ebiten.KeyRight)
	pressAudioSettingsKey(dialog, ebiten.KeyEnter)
	displayManager.completeTransition()
	dialog.Update()
	pressAudioSettingsKey(dialog, ebiten.KeyRight)
	pressAudioSettingsKey(dialog, ebiten.KeyEnter)
	require.Equal(t, configurationDisplayReverting, dialog.displayPhase)
	assert.Equal(t, config.ModeWindowed, displayManager.requestedMode)

	displayManager.completeTransition()
	dialog.Update()
	assert.Equal(t, configurationDisplayEditing, dialog.displayPhase)
	assert.True(t, dialog.IsVisible())
	assert.Equal(t, config.ModeWindowed, dialog.stagedDisplay.Mode)
	assert.Equal(t, config.ModeWindowed, configuration.DisplayConfiguration.Mode)
}

func TestConfigurationDialogRevertStagesCapturedActualState(t *testing.T) {
	displayManager := newConfigurationDisplayManagerStub()
	displayManager.mode = config.ModeMaximized
	displayManager.requestedMode = config.ModeMaximized
	displayManager.normalWidth = 1700
	displayManager.normalHeight = 950
	dialog, configuration := newDisplayTestDialog(displayManager, func() {})
	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	pressAudioSettingsKey(dialog, ebiten.KeyLeft)

	pressAudioSettingsKey(dialog, ebiten.KeyEnter)
	assert.Equal(t, config.ModeWindowed, displayManager.requestedMode)
	displayManager.completeTransition()
	dialog.Update()
	pressAudioSettingsKey(dialog, ebiten.KeyRight)
	pressAudioSettingsKey(dialog, ebiten.KeyEnter)
	displayManager.completeTransition()
	dialog.Update()

	assert.Equal(t, config.ModeMaximized, dialog.stagedDisplay.Mode)
	assert.Equal(t, 1700, dialog.stagedDisplay.WindowWidth)
	assert.Equal(t, 950, dialog.stagedDisplay.WindowHeight)
	assert.Equal(t, config.ModeWindowed, configuration.DisplayConfiguration.Mode)
}

func TestConfigurationDialogDisplayConfirmationTimesOutAndReverts(t *testing.T) {
	displayManager := newConfigurationDisplayManagerStub()
	dialog, _ := newDisplayTestDialog(displayManager, func() {})

	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	pressAudioSettingsKey(dialog, ebiten.KeyRight)
	pressAudioSettingsKey(dialog, ebiten.KeyEnter)
	displayManager.completeTransition()
	dialog.Update()
	for range configurationDisplayConfirmationTime {
		dialog.Update()
	}
	require.Equal(t, configurationDisplayReverting, dialog.displayPhase)

	displayManager.completeTransition()
	dialog.Update()
	assert.Equal(t, configurationDisplayEditing, dialog.displayPhase)
	assert.Equal(t, config.ModeWindowed, dialog.stagedDisplay.Mode)
}

func TestConfigurationDialogDisplaySaveFailureRollsBackButKeepsStagedValues(t *testing.T) {
	displayManager := newConfigurationDisplayManagerStub()
	dialog, configuration := newDisplayTestDialog(displayManager, func() {})

	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	pressAudioSettingsKey(dialog, ebiten.KeyDown)
	pressAudioSettingsKey(dialog, ebiten.KeyRight)
	pressAudioSettingsKey(dialog, ebiten.KeyUp)
	pressAudioSettingsKey(dialog, ebiten.KeyRight)
	pressAudioSettingsKey(dialog, ebiten.KeyEnter)
	displayManager.completeTransition()
	dialog.Update()
	pressAudioSettingsKey(dialog, ebiten.KeyEnter)
	require.Equal(t, configurationDisplayReverting, dialog.displayPhase)
	assert.NotEmpty(t, dialog.saveError)

	displayManager.completeTransition()
	dialog.Update()
	assert.Equal(t, configurationDisplayEditing, dialog.displayPhase)
	assert.Equal(t, config.ModeMaximized, dialog.stagedDisplay.Mode)
	assert.Equal(t, config.ModeWindowed, configuration.DisplayConfiguration.Mode)
	assert.Equal(t, config.ModeWindowed, displayManager.mode)
	assert.Equal(t, 1366, dialog.stagedDisplay.WindowWidth)
	assert.Equal(t, config.MinimumWindowWidth, displayManager.persistWidth)
	assert.Equal(t, config.MinimumWindowHeight, displayManager.persistHeight)
}

func TestConfigurationDialogDisplayTransitionFailureReturnsToEditing(t *testing.T) {
	displayManager := newConfigurationDisplayManagerStub()
	dialog, _ := newDisplayTestDialog(displayManager, func() {})

	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	pressAudioSettingsKey(dialog, ebiten.KeyRight)
	pressAudioSettingsKey(dialog, ebiten.KeyEnter)
	for range configurationDisplayTransitionTimeout {
		dialog.Update()
	}
	require.Equal(t, configurationDisplayReverting, dialog.displayPhase)
	assert.NotEmpty(t, dialog.saveError)

	displayManager.completeTransition()
	dialog.Update()
	assert.Equal(t, configurationDisplayEditing, dialog.displayPhase)
}

func TestConfigurationDialogRetriesFailedRollbackInsteadOfPersistingCandidate(t *testing.T) {
	displayManager := newConfigurationDisplayManagerStub()
	dialog, configuration := newDisplayTestDialog(displayManager, func() {})
	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	pressAudioSettingsKey(dialog, ebiten.KeyRight)
	pressAudioSettingsKey(dialog, ebiten.KeyEnter)
	displayManager.completeTransition()
	dialog.Update()
	pressAudioSettingsKey(dialog, ebiten.KeyRight)
	pressAudioSettingsKey(dialog, ebiten.KeyEnter)
	require.Equal(t, configurationDisplayReverting, dialog.displayPhase)

	displayManager.pending = false
	displayManager.mode = config.ModeMaximized
	dialog.Update()
	require.Equal(t, configurationDisplayEditing, dialog.displayPhase)
	require.True(t, dialog.displayRollbackRetry)
	assert.Equal(t, "[Enter] Retry restore   [Esc] Cancel", dialog.configurationErrorGuidance())

	pressAudioSettingsKey(dialog, ebiten.KeyEnter)
	assert.Equal(t, configurationDisplayReverting, dialog.displayPhase)
	assert.Equal(t, config.ModeWindowed, displayManager.requestedMode)
	assert.Equal(t, config.ModeWindowed, configuration.DisplayConfiguration.Mode)
}

func TestConfigurationDialogDoesNotConfirmUnobservedWindowSize(t *testing.T) {
	displayManager := newConfigurationDisplayManagerStub()
	dialog, _ := newDisplayTestDialog(displayManager, func() {})
	pressAudioSettingsKey(dialog, ebiten.KeyTab)
	pressAudioSettingsKey(dialog, ebiten.KeyDown)
	pressAudioSettingsKey(dialog, ebiten.KeyRight)
	pressAudioSettingsKey(dialog, ebiten.KeyEnter)
	require.Equal(t, configurationDisplayApplying, dialog.displayPhase)

	displayManager.pending = false
	displayManager.normalWidth = config.MinimumWindowWidth
	displayManager.normalHeight = config.MinimumWindowHeight
	dialog.Update()

	assert.Equal(t, configurationDisplayReverting, dialog.displayPhase)
	assert.NotEmpty(t, dialog.saveError)
}

func TestConfigurationDialogNonWindowedRowsKeepAspectLockedAndDisableResizing(t *testing.T) {
	displayManager := newConfigurationDisplayManagerStub()
	dialog, _ := newDisplayTestDialog(displayManager, func() {})
	dialog.stagedDisplay.Mode = config.ModeFullscreen

	labels, values := dialog.displayContentRows()
	assert.Equal(t, "Output size", labels[configurationDisplaySizeRow])
	assert.Equal(t, "Automatic after Apply", values[configurationDisplaySizeRow])
	displayManager.mode = config.ModeFullscreen
	displayManager.outputWidth, displayManager.outputHeight = 2560, 1440
	_, values = dialog.displayContentRows()
	assert.Equal(t, "2560 x 1440 pixels (Auto)", values[configurationDisplaySizeRow])
}

func pressAudioSettingsKey(dialog *ConfigurationDialog, key ebiten.Key) {
	dialog.keyboard.SimulateKeyPress(key)
	dialog.Update()
}

func dialogConfiguration(audioConfig *config.AudioConfiguration) *config.UltimaVConfiguration {
	return &config.UltimaVConfiguration{
		AudioConfiguration: audioConfig,
		DisplayConfiguration: &config.DisplayConfiguration{
			Mode:              config.ModeWindowed,
			WindowWidth:       config.MinimumWindowWidth,
			WindowHeight:      config.MinimumWindowHeight,
			TargetFPS:         30,
			EnableFpsLimiting: true,
		},
		GameplayConfiguration: defaultDialogGameplayConfiguration(),
	}
}

func defaultDialogGameplayConfiguration() *config.GameplayConfiguration {
	settings := config.DefaultGameplayConfiguration()
	settings.BatchMerchantTheft = false
	return settings
}

func placementContainsRect(placement sprites.PercentBasedPlacement, rect image.Rectangle, width, height int) bool {
	placementRect := audioSettingsPercentRect(placement, width, height)

	return rect.Min.X >= placementRect.Min.X && rect.Max.X <= placementRect.Max.X &&
		rect.Min.Y >= placementRect.Min.Y && rect.Max.Y <= placementRect.Max.Y
}

type audioSettingsManagerStub struct {
	musicVolumes     []float64
	sfxVolumes       []float64
	ambientVolumes   []float64
	playedEffects    []game_state.SoundEffect
	enabledStates    []bool
	ambientPlayCount int
	ambientStopCount int
	musicStopCount   int
}

type configurationDisplayManagerStub struct {
	mode          config.Mode
	requestedMode config.Mode
	pending       bool
	scale         float64
	outputWidth   int
	outputHeight  int
	normalWidth   int
	normalHeight  int
	monitorWidth  int
	monitorHeight int
	logicalWidth  int
	logicalHeight int
	persistWidth  int
	persistHeight int
	requestWidth  int
	requestHeight int
}

func newConfigurationDisplayManagerStub() *configurationDisplayManagerStub {
	return &configurationDisplayManagerStub{
		mode:          config.ModeWindowed,
		requestedMode: config.ModeWindowed,
		normalWidth:   config.MinimumWindowWidth,
		normalHeight:  config.MinimumWindowHeight,
		monitorWidth:  2560,
		monitorHeight: 1440,
		logicalWidth:  config.MinimumWindowWidth,
		logicalHeight: config.MinimumWindowHeight,
		persistWidth:  config.MinimumWindowWidth,
		persistHeight: config.MinimumWindowHeight,
	}
}

func (m *configurationDisplayManagerStub) Mode() config.Mode { return m.mode }
func (m *configurationDisplayManagerStub) DeviceScaleFactor() float64 {
	return display.ValidScale(m.scale)
}
func (m *configurationDisplayManagerStub) OutputSize() (int, int) {
	return m.outputWidth, m.outputHeight
}

func (m *configurationDisplayManagerStub) MonitorSize() (int, int) {
	return m.monitorWidth, m.monitorHeight
}

func (m *configurationDisplayManagerStub) LogicalSize() (int, int) {
	return m.logicalWidth, m.logicalHeight
}

func (m *configurationDisplayManagerStub) LastNormalWindowSize() (int, int) {
	return m.normalWidth, m.normalHeight
}

func (m *configurationDisplayManagerStub) PersistableWindowSize() (int, int) {
	return m.persistWidth, m.persistHeight
}

func (m *configurationDisplayManagerStub) CommitNormalWindowSize(width, height int) {
	m.persistWidth, m.persistHeight = width, height
}

func (m *configurationDisplayManagerStub) RequestMode(mode config.Mode, width, height int) error {
	m.requestedMode = mode
	m.requestWidth = width
	m.requestHeight = height
	m.normalWidth = width
	m.normalHeight = height
	m.pending = true

	return nil
}

func (m *configurationDisplayManagerStub) IsTransitionPending() bool { return m.pending }

func (m *configurationDisplayManagerStub) completeTransition() {
	m.mode = m.requestedMode
	m.pending = false
}

func (m *configurationDisplayManagerStub) dragWindow(width, height int) {
	m.mode = config.ModeWindowed
	m.requestedMode = config.ModeWindowed
	m.normalWidth = width
	m.normalHeight = height
	m.persistWidth = width
	m.persistHeight = height
}

func newDisplayTestDialog(
	displayManager *configurationDisplayManagerStub,
	onDismiss func(),
) (*ConfigurationDialog, *config.UltimaVConfiguration) {
	configuration := dialogConfiguration(&config.AudioConfiguration{})
	dialog := newConfigurationDialogForTab(
		&sprites.PercentBasedPlacement{StartPercentX: 0.1, EndPercentX: 0.9, StartPercentY: 0.1, EndPercentY: 0.9},
		newPresentationTestKeyboard(),
		onDismiss,
		configuration,
		nil,
		audioSettingsAudioTab,
	)
	dialog.displayManager = displayManager
	dialog.originalDisplay = *configuration.DisplayConfiguration
	dialog.stagedDisplay = *configuration.DisplayConfiguration

	return dialog, configuration
}

func initializeDialogConfigFile(t *testing.T, name string) (*config.UltimaVConfiguration, string) {
	t.Helper()
	directory := t.TempDir()
	configPath := filepath.Join(directory, name+".yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("existing: true\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "DATA.OVL"), nil, 0o600))
	configuration, err := config.LoadUltimaVConfiguration(configPath, directory)
	require.NoError(t, err)

	return configuration, configPath
}

func (m *audioSettingsManagerStub) SetMusicVolume(volume float64) error {
	m.musicVolumes = append(m.musicVolumes, volume)
	return nil
}

func (m *audioSettingsManagerStub) SetSoundEffectVolume(volume float64) error {
	m.sfxVolumes = append(m.sfxVolumes, volume)
	return nil
}

func (m *audioSettingsManagerStub) SetAmbientVolume(volume float64) error {
	m.ambientVolumes = append(m.ambientVolumes, volume)
	return nil
}

func (m *audioSettingsManagerStub) PlayMusicPreview(string) error { return nil }

func (m *audioSettingsManagerStub) StopBackgroundMusic() error {
	m.musicStopCount++
	return nil
}

func (m *audioSettingsManagerStub) PlaySoundEffect(effect game_state.SoundEffect) error {
	m.playedEffects = append(m.playedEffects, effect)
	return nil
}

func (m *audioSettingsManagerStub) PlayAmbientSoundPreview() error {
	m.ambientPlayCount++
	return nil
}

func (m *audioSettingsManagerStub) StopAmbientSoundPreview() error {
	m.ambientStopCount++
	return nil
}

func (m *audioSettingsManagerStub) SetEnabled(enabled bool) {
	m.enabledStates = append(m.enabledStates, enabled)
}
